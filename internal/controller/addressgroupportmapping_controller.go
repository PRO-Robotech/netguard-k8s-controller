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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// AddressGroupPortMappingReconciler reconciles a AddressGroupPortMapping object
type AddressGroupPortMappingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupportmappings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupportmappings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupportmappings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *AddressGroupPortMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AddressGroupPortMapping", "request", req)

	// Get the AddressGroupPortMapping resource
	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	if err := r.Get(ctx, req.NamespacedName, portMapping); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, likely deleted
			logger.Info("AddressGroupPortMapping not found, it may have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	// Add finalizer if it doesn't exist
	const finalizer = "addressgroupportmapping.netguard.sgroups.io/finalizer"
	if !controllerutil.ContainsFinalizer(portMapping, finalizer) {
		controllerutil.AddFinalizer(portMapping, finalizer)
		if err := r.Update(ctx, portMapping); err != nil {
			logger.Error(err, "Failed to add finalizer to AddressGroupPortMapping")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil // Requeue to continue reconciliation
	}

	// Check if the resource is being deleted
	if !portMapping.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, portMapping, finalizer)
	}

	// Clean up stale port mappings
	if err := r.cleanupStalePortMappings(ctx, portMapping); err != nil {
		logger.Error(err, "Failed to cleanup stale port mappings")
		return ctrl.Result{}, err
	}

	// Set Ready condition to true
	setPortMappingCondition(portMapping, netguardv1alpha1.ConditionReady, metav1.ConditionTrue,
		"PortMappingValid", "All port mappings are valid")
	if err := r.Status().Update(ctx, portMapping); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupPortMapping reconciled successfully")
	return ctrl.Result{}, nil
}

// reconcileDelete handles the deletion of an AddressGroupPortMapping
func (r *AddressGroupPortMappingReconciler) reconcileDelete(ctx context.Context, portMapping *netguardv1alpha1.AddressGroupPortMapping, finalizer string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Remove finalizer
	controllerutil.RemoveFinalizer(portMapping, finalizer)
	if err := r.Update(ctx, portMapping); err != nil {
		logger.Error(err, "Failed to remove finalizer from AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupPortMapping deleted successfully")
	return ctrl.Result{}, nil
}

// cleanupStalePortMappings removes port mappings for services that no longer exist
func (r *AddressGroupPortMappingReconciler) cleanupStalePortMappings(ctx context.Context, portMapping *netguardv1alpha1.AddressGroupPortMapping) error {
	logger := log.FromContext(ctx)

	for i := 0; i < len(portMapping.AccessPorts.Items); i++ {
		serviceRef := portMapping.AccessPorts.Items[i]

		// Check if the service still exists
		service := &netguardv1alpha1.Service{}
		err := r.Get(ctx, client.ObjectKey{
			Name:      serviceRef.GetName(),
			Namespace: serviceRef.GetNamespace(),
		}, service)

		if apierrors.IsNotFound(err) {
			// Service doesn't exist, remove this entry
			logger.Info("Removing stale port mapping for deleted service",
				"service", serviceRef.GetName(),
				"namespace", serviceRef.GetNamespace())

			// Remove the item from the slice
			portMapping.AccessPorts.Items = append(
				portMapping.AccessPorts.Items[:i],
				portMapping.AccessPorts.Items[i+1:]...)
			i-- // Adjust index after removal
		} else if err != nil {
			return err
		}
	}

	return nil
}

// setPortMappingCondition updates a condition in the status
func setPortMappingCondition(portMapping *netguardv1alpha1.AddressGroupPortMapping, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Find and update existing condition or append new one
	for i, cond := range portMapping.Status.Conditions {
		if cond.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if cond.Status != status {
				portMapping.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Condition not found, append it
	portMapping.Status.Conditions = append(portMapping.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddressGroupPortMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.AddressGroupPortMapping{}).
		Named("addressgroupportmapping").
		Complete(r)
}
