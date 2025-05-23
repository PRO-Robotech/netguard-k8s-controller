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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// ServiceAliasReconciler reconciles a ServiceAlias object
type ServiceAliasReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=servicealiases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=servicealiases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=servicealiases/finalizers,verbs=update
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *ServiceAliasReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ServiceAlias", "request", req)

	// Get the ServiceAlias resource
	serviceAlias := &netguardv1alpha1.ServiceAlias{}
	if err := r.Get(ctx, req.NamespacedName, serviceAlias); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, likely deleted
			logger.Info("ServiceAlias not found, it may have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ServiceAlias")
		return ctrl.Result{}, err
	}

	// Check if the referenced Service exists
	service := &netguardv1alpha1.Service{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      serviceAlias.Spec.ServiceRef.GetName(),
		Namespace: serviceAlias.Spec.ServiceRef.ResolveNamespace(serviceAlias.GetNamespace()),
	}, service)

	if apierrors.IsNotFound(err) {
		// Referenced Service doesn't exist
		setServiceAliasCondition(serviceAlias, netguardv1alpha1.ConditionReady, metav1.ConditionFalse,
			"ServiceNotFound", "Referenced Service does not exist")
		if err := r.Status().Update(ctx, serviceAlias); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get referenced Service")
		return ctrl.Result{}, err
	}

	// Set owner reference to the Service
	// This will ensure that when the Service is deleted, this ServiceAlias will be automatically deleted
	if err := r.setOwnerReference(ctx, serviceAlias, service); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return ctrl.Result{}, err
	}

	// Service exists, set Ready condition to true
	setServiceAliasCondition(serviceAlias, netguardv1alpha1.ConditionReady, metav1.ConditionTrue,
		"ServiceAliasValid", "Referenced Service exists")
	if err := r.Status().Update(ctx, serviceAlias); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("ServiceAlias reconciled successfully")
	return ctrl.Result{}, nil
}

// setOwnerReference sets the owner reference of the ServiceAlias to the Service
func (r *ServiceAliasReconciler) setOwnerReference(ctx context.Context, serviceAlias *netguardv1alpha1.ServiceAlias, service *netguardv1alpha1.Service) error {
	// Check if owner reference already exists
	for _, ownerRef := range serviceAlias.GetOwnerReferences() {
		if ownerRef.UID == service.GetUID() {
			// Owner reference already exists
			return nil
		}
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(service, serviceAlias, r.Scheme); err != nil {
		return err
	}

	// Update the ServiceAlias
	return r.Update(ctx, serviceAlias)
}

// setServiceAliasCondition updates a condition in the status
func setServiceAliasCondition(serviceAlias *netguardv1alpha1.ServiceAlias, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Find and update existing condition or append new one
	for i, cond := range serviceAlias.Status.Conditions {
		if cond.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if cond.Status != status {
				serviceAlias.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Condition not found, append it
	serviceAlias.Status.Conditions = append(serviceAlias.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceAliasReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.ServiceAlias{}).
		Named("servicealias").
		Complete(r)
}
