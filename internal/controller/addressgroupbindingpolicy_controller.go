/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
	"sgroups.io/netguard/internal/webhook/v1alpha1"
)

// AddressGroupBindingPolicyReconciler reconciles a AddressGroupBindingPolicy object
type AddressGroupBindingPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindingpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindingpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindingpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupportmappings,verbs=get;list;watch
// +kubebuilder:rbac:groups=sgroups.io,resources=addressgroups,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AddressGroupBindingPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AddressGroupBindingPolicy", "request", req)

	// Get the AddressGroupBindingPolicy resource
	policy := &netguardv1alpha1.AddressGroupBindingPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, likely deleted
			logger.Info("AddressGroupBindingPolicy not found, it may have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AddressGroupBindingPolicy")
		return ctrl.Result{}, err
	}

	// Check if the resource is being deleted
	if !policy.DeletionTimestamp.IsZero() {
		logger.Info("AddressGroupBindingPolicy is being deleted, no action needed")
		return ctrl.Result{}, nil
	}

	// Verify that the referenced resources exist
	// This is already done by the webhook, but we do it again here for safety
	// and to update the status conditions

	// 1. Verify AddressGroup exists
	addressGroupRef := policy.Spec.AddressGroupRef
	addressGroupNamespace := v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), policy.GetNamespace())

	addressGroup := &providerv1alpha1.AddressGroup{}
	addressGroupKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}

	if err := r.Get(ctx, addressGroupKey, addressGroup); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AddressGroup not found, deleting policy",
				"addressGroup", addressGroupKey.String())
			// AddressGroup deleted, delete the policy with retry
			if err := DeleteWithRetry(ctx, r.Client, policy, DefaultMaxRetries); err != nil {
				logger.Error(err, "Failed to delete policy after AddressGroup deletion")
				return ctrl.Result{}, err
			}
			logger.Info("Policy deleted successfully after AddressGroup deletion")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AddressGroup")
		return ctrl.Result{}, err
	}

	// Check if the Service exists
	serviceRef := policy.Spec.ServiceRef
	serviceNamespace := v1alpha1.ResolveNamespace(serviceRef.GetNamespace(), policy.GetNamespace())
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: serviceNamespace,
	}

	service := &netguardv1alpha1.Service{}
	if err := r.Get(ctx, serviceKey, service); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Service not found, deleting policy",
				"service", serviceKey.String())
			// Service deleted, delete the policy with retry
			if err := DeleteWithRetry(ctx, r.Client, policy, DefaultMaxRetries); err != nil {
				logger.Error(err, "Failed to delete policy after Service deletion")
				return ctrl.Result{}, err
			}
			logger.Info("Policy deleted successfully after Service deletion")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// All resources exist, set Ready condition to true
	setAddressGroupBindingPolicyCondition(policy, "Ready", metav1.ConditionTrue, "PolicyValid",
		"AddressGroupBindingPolicy is valid and ready")
	if err := UpdateStatusWithRetry(ctx, r.Client, policy, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to update AddressGroupBindingPolicy status")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupBindingPolicy reconciled successfully")
	return ctrl.Result{}, nil
}

// setAddressGroupBindingPolicyCondition updates a condition in the policy status
func setAddressGroupBindingPolicyCondition(policy *netguardv1alpha1.AddressGroupBindingPolicy, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Find and update existing condition or append new one
	for i, cond := range policy.Status.Conditions {
		if cond.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if cond.Status != status {
				policy.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Condition not found, append it
	policy.Status.Conditions = append(policy.Status.Conditions, condition)
}

// findPoliciesForService finds policies that reference a specific service
func (r *AddressGroupBindingPolicyReconciler) findPoliciesForService(ctx context.Context, obj client.Object) []reconcile.Request {
	service, ok := obj.(*netguardv1alpha1.Service)
	if !ok {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Finding policies for Service",
		"service", service.GetName(),
		"namespace", service.GetNamespace())

	// Use index for faster lookup
	policyList := &netguardv1alpha1.AddressGroupBindingPolicyList{}
	if err := r.List(ctx, policyList, client.MatchingFields{"spec.serviceRef.name": service.GetName()}); err != nil {
		logger.Error(err, "Failed to list AddressGroupBindingPolicies by service index")
		return nil
	}

	var requests []reconcile.Request

	// Filter policies that reference this service (additional namespace check)
	for _, policy := range policyList.Items {
		// Resolve the namespace for the ServiceRef
		resolvedNamespace := v1alpha1.ResolveNamespace(policy.Spec.ServiceRef.GetNamespace(), policy.GetNamespace())

		if resolvedNamespace == service.GetNamespace() {
			logger.Info("Found policy that references this Service",
				"policy", policy.GetName(),
				"policyNamespace", policy.GetNamespace(),
				"serviceRef", policy.Spec.ServiceRef.GetName(),
				"resolvedNamespace", resolvedNamespace)

			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      policy.GetName(),
					Namespace: policy.GetNamespace(),
				},
			})
		}
	}

	logger.Info("Found policies for Service",
		"service", service.GetName(),
		"policiesCount", len(requests))

	return requests
}

// findPoliciesForAddressGroup finds policies that reference a specific address group
func (r *AddressGroupBindingPolicyReconciler) findPoliciesForAddressGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	addressGroup, ok := obj.(*providerv1alpha1.AddressGroup)
	if !ok {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Finding policies for AddressGroup",
		"addressGroup", addressGroup.GetName(),
		"namespace", addressGroup.GetNamespace())

	// Use index for faster lookup
	policyList := &netguardv1alpha1.AddressGroupBindingPolicyList{}
	if err := r.List(ctx, policyList, client.MatchingFields{"spec.addressGroupRef.name": addressGroup.GetName()}); err != nil {
		logger.Error(err, "Failed to list AddressGroupBindingPolicies by addressGroup index")
		return nil
	}

	var requests []reconcile.Request

	// Filter policies that reference this address group (additional namespace check)
	for _, policy := range policyList.Items {
		// Resolve the namespace for the AddressGroupRef
		resolvedNamespace := v1alpha1.ResolveNamespace(policy.Spec.AddressGroupRef.GetNamespace(), policy.GetNamespace())

		if resolvedNamespace == addressGroup.GetNamespace() {
			logger.Info("Found policy that references this AddressGroup",
				"policy", policy.GetName(),
				"policyNamespace", policy.GetNamespace(),
				"addressGroupRef", policy.Spec.AddressGroupRef.GetName(),
				"resolvedNamespace", resolvedNamespace)

			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      policy.GetName(),
					Namespace: policy.GetNamespace(),
				},
			})
		}
	}

	logger.Info("Found policies for AddressGroup",
		"addressGroup", addressGroup.GetName(),
		"policiesCount", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddressGroupBindingPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add indexes for faster lookups
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.AddressGroupBindingPolicy{}, "spec.serviceRef.name",
		func(obj client.Object) []string {
			policy := obj.(*netguardv1alpha1.AddressGroupBindingPolicy)
			return []string{policy.Spec.ServiceRef.Name}
		}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.AddressGroupBindingPolicy{}, "spec.addressGroupRef.name",
		func(obj client.Object) []string {
			policy := obj.(*netguardv1alpha1.AddressGroupBindingPolicy)
			return []string{policy.Spec.AddressGroupRef.Name}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.AddressGroupBindingPolicy{}).
		// Watch for changes to Service
		Watches(
			&netguardv1alpha1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForService),
		).
		// Watch for changes to AddressGroup
		Watches(
			&providerv1alpha1.AddressGroup{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForAddressGroup),
		).
		Named("addressgroupbindingpolicy").
		Complete(r)
}
