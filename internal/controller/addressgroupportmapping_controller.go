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
	if err := EnsureFinalizer(ctx, r.Client, portMapping, finalizer); err != nil {
		logger.Error(err, "Failed to add finalizer to AddressGroupPortMapping")
		return ctrl.Result{}, err
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
	SetCondition(&portMapping.Status.Conditions, netguardv1alpha1.ConditionReady, metav1.ConditionTrue,
		"PortMappingValid", "All port mappings are valid")
	if err := UpdateStatusWithRetry(ctx, r.Client, portMapping, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupPortMapping reconciled successfully")
	return ctrl.Result{}, nil
}

// reconcileDelete handles the deletion of an AddressGroupPortMapping
func (r *AddressGroupPortMappingReconciler) reconcileDelete(ctx context.Context, portMapping *netguardv1alpha1.AddressGroupPortMapping, finalizer string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting AddressGroupPortMapping", "name", portMapping.Name)

	// Get the latest version of the resource to avoid conflicts
	freshPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      portMapping.Name,
		Namespace: portMapping.Namespace,
	}, freshPortMapping); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource is already gone, nothing to do
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get latest version of AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	// Check if finalizer exists
	if !controllerutil.ContainsFinalizer(freshPortMapping, finalizer) {
		// Finalizer already removed, nothing to do
		return ctrl.Result{}, nil
	}

	// Create a patch for removing the finalizer
	patch := client.MergeFrom(freshPortMapping.DeepCopy())
	controllerutil.RemoveFinalizer(freshPortMapping, finalizer)

	// Apply the patch with retry
	if err := PatchWithRetry(ctx, r.Client, freshPortMapping, patch, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to remove finalizer from AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupPortMapping deleted successfully")
	return ctrl.Result{}, nil
}

// cleanupStalePortMappings removes port mappings for services that no longer exist
func (r *AddressGroupPortMappingReconciler) cleanupStalePortMappings(ctx context.Context, portMapping *netguardv1alpha1.AddressGroupPortMapping) error {
	logger := log.FromContext(ctx)

	// If there are no port mappings, nothing to clean up
	if len(portMapping.AccessPorts.Items) == 0 {
		return nil
	}

	// Create a map of existing services for faster lookup
	existingServices := make(map[string]bool)
	serviceList := &netguardv1alpha1.ServiceList{}

	// List all services in all namespaces to catch cross-namespace references
	if err := r.List(ctx, serviceList); err != nil {
		return err
	}

	for _, svc := range serviceList.Items {
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		existingServices[key] = true
	}

	// Filter out stale port mappings in one pass
	validPorts := make([]netguardv1alpha1.ServicePortsRef, 0, len(portMapping.AccessPorts.Items))
	modified := false

	for _, serviceRef := range portMapping.AccessPorts.Items {
		namespace := serviceRef.GetNamespace()
		if namespace == "" {
			namespace = portMapping.Namespace // Default to the same namespace if not specified
		}

		key := fmt.Sprintf("%s/%s", namespace, serviceRef.GetName())
		if existingServices[key] {
			validPorts = append(validPorts, serviceRef)
		} else {
			logger.Info("Removing stale port mapping for deleted service",
				"service", serviceRef.GetName(),
				"namespace", namespace)
			modified = true
		}
	}

	// Update only if there were changes
	if modified {
		// Get the latest version of the resource to avoid conflicts
		latest := &netguardv1alpha1.AddressGroupPortMapping{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      portMapping.Name,
			Namespace: portMapping.Namespace,
		}, latest); err != nil {
			return err
		}

		// Apply our changes to the latest version
		latest.AccessPorts.Items = validPorts

		// Use patch with retry to update the resource
		original := latest.DeepCopy()
		patch := client.MergeFrom(original)
		if err := PatchWithRetry(ctx, r.Client, latest, patch, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update port mappings after cleanup")
			return err
		}

		// Update our local copy to reflect the changes
		portMapping.AccessPorts.Items = validPorts
	}

	return nil
}

// This function has been replaced by the SetCondition utility function

// SetupWithManager sets up the controller with the Manager.
func (r *AddressGroupPortMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.AddressGroupPortMapping{}).
		Named("addressgroupportmapping").
		Complete(r)
}
