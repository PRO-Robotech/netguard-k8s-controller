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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
// reconcileDelete handles the deletion of a ServiceAlias
func (r *ServiceAliasReconciler) reconcileDelete(ctx context.Context, serviceAlias *netguardv1alpha1.ServiceAlias, finalizer string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting ServiceAlias", "name", serviceAlias.Name)

	// Get the latest version of the resource to avoid conflicts
	freshServiceAlias := &netguardv1alpha1.ServiceAlias{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      serviceAlias.Name,
		Namespace: serviceAlias.Namespace,
	}, freshServiceAlias); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource is already gone, nothing to do
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get latest version of ServiceAlias")
		return ctrl.Result{}, err
	}

	// Check if finalizer exists
	if !controllerutil.ContainsFinalizer(freshServiceAlias, finalizer) {
		// Finalizer already removed, nothing to do
		return ctrl.Result{}, nil
	}

	// Check if there are any RuleS2S resources that reference this ServiceAlias
	ruleS2SList := &netguardv1alpha1.RuleS2SList{}
	if err := r.List(ctx, ruleS2SList); err != nil {
		logger.Error(err, "Failed to list RuleS2S objects")
		return ctrl.Result{}, err
	}

	// Check if any rules reference this ServiceAlias
	for _, rule := range ruleS2SList.Items {
		// Check if the rule references this ServiceAlias as local service
		if rule.Spec.ServiceLocalRef.Name == freshServiceAlias.Name &&
			rule.Namespace == freshServiceAlias.Namespace {
			logger.Info("Cannot delete ServiceAlias: it is referenced by RuleS2S as local service",
				"serviceAlias", freshServiceAlias.Name, "rule", rule.Name)
			return ctrl.Result{}, fmt.Errorf("cannot delete ServiceAlias %s: it is referenced by RuleS2S %s as local service",
				freshServiceAlias.Name, rule.Name)
		}

		// Check if the rule references this ServiceAlias as target service
		targetNamespace := rule.Spec.ServiceRef.ResolveNamespace(rule.Namespace)
		if rule.Spec.ServiceRef.Name == freshServiceAlias.Name &&
			targetNamespace == freshServiceAlias.Namespace {
			logger.Info("Cannot delete ServiceAlias: it is referenced by RuleS2S as target service",
				"serviceAlias", freshServiceAlias.Name, "rule", rule.Name)
			return ctrl.Result{}, fmt.Errorf("cannot delete ServiceAlias %s: it is referenced by RuleS2S %s as target service",
				freshServiceAlias.Name, rule.Name)
		}
	}

	// Remove the finalizer
	controllerutil.RemoveFinalizer(freshServiceAlias, finalizer)

	// Update with retry to handle conflicts
	if err := UpdateWithRetry(ctx, r.Client, freshServiceAlias, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to remove finalizer from ServiceAlias")
		return ctrl.Result{}, err
	}

	logger.Info("ServiceAlias deleted successfully")
	return ctrl.Result{}, nil
}

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

	// Add finalizer if it doesn't exist
	const finalizer = "servicealias.netguard.sgroups.io/finalizer"
	if !controllerutil.ContainsFinalizer(serviceAlias, finalizer) {
		controllerutil.AddFinalizer(serviceAlias, finalizer)
		// Update with retry to handle conflicts
		if err := UpdateWithRetry(ctx, r.Client, serviceAlias, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to add finalizer to ServiceAlias")
			return ctrl.Result{}, err
		}
		// Get the updated ServiceAlias after adding the finalizer
		if err := r.Get(ctx, req.NamespacedName, serviceAlias); err != nil {
			logger.Error(err, "Failed to get updated ServiceAlias")
			return ctrl.Result{}, err
		}
	}

	// Check if the resource is being deleted
	if !serviceAlias.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, serviceAlias, finalizer)
	}

	// Check if the referenced Service exists
	service := &netguardv1alpha1.Service{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      serviceAlias.Spec.ServiceRef.GetName(),
		Namespace: serviceAlias.GetNamespace(), // ServiceAlias can only reference Service in the same namespace
	}, service)

	if apierrors.IsNotFound(err) {
		// Referenced Service doesn't exist
		SetCondition(&serviceAlias.Status.Conditions, netguardv1alpha1.ConditionReady, metav1.ConditionFalse,
			"ServiceNotFound", "Referenced Service does not exist")
		// Update status with retry to handle conflicts
		if err := UpdateStatusWithRetry(ctx, r.Client, serviceAlias, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		// Return an error to match the test expectation
		return ctrl.Result{}, fmt.Errorf("referenced Service %s does not exist", serviceAlias.Spec.ServiceRef.GetName())
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

	// Get the latest version of the ServiceAlias after setting owner reference
	if err := r.Get(ctx, req.NamespacedName, serviceAlias); err != nil {
		logger.Error(err, "Failed to get updated ServiceAlias")
		return ctrl.Result{}, err
	}

	// Service exists, set Ready condition to true
	SetCondition(&serviceAlias.Status.Conditions, netguardv1alpha1.ConditionReady, metav1.ConditionTrue,
		"ServiceAliasValid", "Referenced Service exists")
	// Update status with retry to handle conflicts
	if err := UpdateStatusWithRetry(ctx, r.Client, serviceAlias, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("ServiceAlias reconciled successfully")
	return ctrl.Result{}, nil
}

// setOwnerReference sets the owner reference of the ServiceAlias to the Service
func (r *ServiceAliasReconciler) setOwnerReference(ctx context.Context, serviceAlias *netguardv1alpha1.ServiceAlias, service *netguardv1alpha1.Service) error {
	logger := log.FromContext(ctx)

	// Get the latest version of the ServiceAlias to avoid conflicts
	freshServiceAlias := &netguardv1alpha1.ServiceAlias{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      serviceAlias.Name,
		Namespace: serviceAlias.Namespace,
	}, freshServiceAlias); err != nil {
		return err
	}

	// Check if owner reference already exists
	for _, ownerRef := range freshServiceAlias.GetOwnerReferences() {
		if ownerRef.UID == service.GetUID() {
			// Owner reference already exists, update our local copy
			serviceAlias.SetOwnerReferences(freshServiceAlias.GetOwnerReferences())
			return nil
		}
	}

	// Clear existing owner references if any
	if len(freshServiceAlias.GetOwnerReferences()) > 0 {
		freshServiceAlias.SetOwnerReferences([]metav1.OwnerReference{})
	}

	// Set controller reference (will handle deletion automatically)
	if err := controllerutil.SetControllerReference(service, freshServiceAlias, r.Scheme); err != nil {
		return err
	}

	// Update the ServiceAlias with retry on conflicts
	logger.Info("Setting owner reference on ServiceAlias",
		"serviceAlias", freshServiceAlias.Name,
		"service", service.Name)

	if err := UpdateWithRetry(ctx, r.Client, freshServiceAlias, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to update ServiceAlias with owner reference")
		return err
	}

	// Update our local copy to reflect the changes
	serviceAlias.SetOwnerReferences(freshServiceAlias.GetOwnerReferences())

	return nil
}

// This function has been replaced by the SetCondition utility function

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceAliasReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add index for faster lookups of ServiceAlias by Service name
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.ServiceAlias{},
		"spec.serviceRef.name",
		func(obj client.Object) []string {
			serviceAlias := obj.(*netguardv1alpha1.ServiceAlias)
			return []string{serviceAlias.Spec.ServiceRef.Name}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.ServiceAlias{}).
		Owns(&netguardv1alpha1.Service{}).
		Named("servicealias").
		Complete(r)
}
