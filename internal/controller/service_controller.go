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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	"sgroups.io/netguard/internal/webhook/v1alpha1"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services/finalizers,verbs=update
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupportmappings,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Service", "request", req)

	// Get the Service resource
	service := &netguardv1alpha1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, likely deleted
			logger.Info("Service not found, it may have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// Add finalizer if it doesn't exist
	const finalizer = "service.netguard.sgroups.io/finalizer"
	if err := EnsureFinalizer(ctx, r.Client, service, finalizer); err != nil {
		logger.Error(err, "Failed to add finalizer to Service")
		return ctrl.Result{}, err
	}
	// Continue processing without requeue

	// Check if the resource is being deleted
	if !service.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, service, finalizer)
	}

	// Normal reconciliation
	return r.reconcileNormal(ctx, service)
}

// reconcileNormal handles the normal reconciliation of a Service
func (r *ServiceReconciler) reconcileNormal(ctx context.Context, service *netguardv1alpha1.Service) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update status to Ready only if it's not already set
	if !meta.IsStatusConditionTrue(service.Status.Conditions, netguardv1alpha1.ConditionReady) {
		setServiceCondition(service, netguardv1alpha1.ConditionReady, metav1.ConditionTrue, "ServiceCreated",
			"Service successfully created")
		if err := UpdateStatusWithRetry(ctx, r.Client, service, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update Service status")
			return ctrl.Result{}, err
		}
	}

	// If the service has ports and is bound to AddressGroups, update the port mappings
	if len(service.Spec.IngressPorts) > 0 && len(service.AddressGroups.Items) > 0 {
		// For each AddressGroup, update the port mapping
		for _, addressGroupRef := range service.AddressGroups.Items {
			// Get the AddressGroupPortMapping
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			portMappingKey := client.ObjectKey{
				Name:      addressGroupRef.GetName(),
				Namespace: v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), service.GetNamespace()),
			}
			if err := r.Get(ctx, portMappingKey, portMapping); err != nil {
				if apierrors.IsNotFound(err) {
					// PortMapping not found, log and continue
					logger.Info("AddressGroupPortMapping not found",
						"addressGroup", addressGroupRef.GetName())
					continue
				}
				logger.Error(err, "Failed to get AddressGroupPortMapping")
				return ctrl.Result{}, err
			}

			// Create ServicePortsRef
			servicePortsRef := netguardv1alpha1.ServicePortsRef{
				NamespacedObjectReference: netguardv1alpha1.NamespacedObjectReference{
					ObjectReference: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       service.GetName(),
					},
					Namespace: service.GetNamespace(),
				},
				Ports: v1alpha1.ConvertIngressPortsToProtocolPorts(service.Spec.IngressPorts),
			}

			// Check if the service is already in the port mapping
			serviceFound := false
			for _, sp := range portMapping.AccessPorts.Items {
				if sp.GetName() == service.GetName() &&
					sp.GetNamespace() == service.GetNamespace() {
					// Update ports if they've changed
					if !reflect.DeepEqual(sp.Ports, servicePortsRef.Ports) {
						// Get the latest version of the port mapping before updating
						updatedPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
						if err := r.Get(ctx, portMappingKey, updatedPortMapping); err != nil {
							logger.Error(err, "Failed to get latest AddressGroupPortMapping")
							return ctrl.Result{}, err
						}

						// Find the service in the updated port mapping
						for j, updatedSp := range updatedPortMapping.AccessPorts.Items {
							if updatedSp.GetName() == service.GetName() &&
								updatedSp.GetNamespace() == service.GetNamespace() {
								// Create a copy for patching
								original := updatedPortMapping.DeepCopy()
								updatedPortMapping.AccessPorts.Items[j].Ports = servicePortsRef.Ports

								// Apply patch with retry
								patch := client.MergeFrom(original)
								if err := PatchWithRetry(ctx, r.Client, updatedPortMapping, patch, DefaultMaxRetries); err != nil {
									logger.Error(err, "Failed to update AddressGroupPortMapping.AccessPorts")
									return ctrl.Result{}, err
								}
								logger.Info("Updated Service ports in AddressGroupPortMapping",
									"service", service.GetName(),
									"addressGroup", addressGroupRef.GetName())
								break
							}
						}
					}
					serviceFound = true
					break
				}
			}

			// If the service is not in the port mapping, add it
			if !serviceFound {
				// Get the latest version of the port mapping before updating
				updatedPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
				if err := r.Get(ctx, portMappingKey, updatedPortMapping); err != nil {
					logger.Error(err, "Failed to get latest AddressGroupPortMapping")
					return ctrl.Result{}, err
				}

				// Check if the service is already in the updated port mapping
				serviceAlreadyAdded := false
				for _, updatedSp := range updatedPortMapping.AccessPorts.Items {
					if updatedSp.GetName() == service.GetName() &&
						updatedSp.GetNamespace() == service.GetNamespace() {
						serviceAlreadyAdded = true
						break
					}
				}

				// Add the service if it's not already there
				if !serviceAlreadyAdded {
					// Create a copy for patching
					original := updatedPortMapping.DeepCopy()
					updatedPortMapping.AccessPorts.Items = append(updatedPortMapping.AccessPorts.Items, servicePortsRef)

					// Apply patch with retry
					patch := client.MergeFrom(original)
					if err := PatchWithRetry(ctx, r.Client, updatedPortMapping, patch, DefaultMaxRetries); err != nil {
						logger.Error(err, "Failed to add Service to AddressGroupPortMapping.AccessPorts")
						return ctrl.Result{}, err
					}
					logger.Info("Added Service to AddressGroupPortMapping.AccessPorts",
						"service", service.GetName(),
						"addressGroup", addressGroupRef.GetName())
				}
			}
		}
	}

	logger.Info("Service reconciled successfully")
	return ctrl.Result{}, nil
}

// reconcileDelete handles the deletion of a Service
func (r *ServiceReconciler) reconcileDelete(ctx context.Context, service *netguardv1alpha1.Service, finalizer string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting Service", "name", service.GetName())

	// 1. Get all AddressGroupBindings that reference this Service
	bindingList := &netguardv1alpha1.AddressGroupBindingList{}
	if err := r.List(ctx, bindingList, client.InNamespace(service.GetNamespace())); err != nil {
		logger.Error(err, "Failed to list AddressGroupBindings")
		return ctrl.Result{}, err
	}

	// 2. Delete all AddressGroupBindings that reference this Service
	for _, binding := range bindingList.Items {
		if binding.Spec.ServiceRef.GetName() == service.GetName() {
			if err := r.Delete(ctx, &binding); err != nil {
				logger.Error(err, "Failed to delete AddressGroupBinding",
					"binding", binding.GetName())
				return ctrl.Result{}, err
			}
			logger.Info("Deleted AddressGroupBinding for Service",
				"binding", binding.GetName(),
				"service", service.GetName())
		}
	}

	// 3. For each AddressGroup in Service.AddressGroups, remove the Service from the port mapping
	for _, addressGroupRef := range service.AddressGroups.Items {
		// Get the AddressGroupPortMapping
		portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
		portMappingKey := client.ObjectKey{
			Name:      addressGroupRef.GetName(),
			Namespace: v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), service.GetNamespace()),
		}

		if err := r.Get(ctx, portMappingKey, portMapping); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to get AddressGroupPortMapping")
				return ctrl.Result{}, err
			}
			continue // If not found, continue to the next
		}

		// Remove the Service from the port mapping
		for _, sp := range portMapping.AccessPorts.Items {
			if sp.GetName() == service.GetName() &&
				sp.GetNamespace() == service.GetNamespace() {
				// Get the latest version of the port mapping before updating
				updatedPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
				if err := r.Get(ctx, portMappingKey, updatedPortMapping); err != nil {
					if apierrors.IsNotFound(err) {
						// Port mapping no longer exists, nothing to do
						break
					}
					logger.Error(err, "Failed to get latest AddressGroupPortMapping")
					return ctrl.Result{}, err
				}

				// Find the service in the updated port mapping
				serviceFound := false
				for j, updatedSp := range updatedPortMapping.AccessPorts.Items {
					if updatedSp.GetName() == service.GetName() &&
						updatedSp.GetNamespace() == service.GetNamespace() {
						// Remove the item from the slice
						// Create a copy for patching
						original := updatedPortMapping.DeepCopy()
						updatedPortMapping.AccessPorts.Items = append(
							updatedPortMapping.AccessPorts.Items[:j],
							updatedPortMapping.AccessPorts.Items[j+1:]...)

						// Apply patch with retry
						patch := client.MergeFrom(original)
						if err := PatchWithRetry(ctx, r.Client, updatedPortMapping, patch, DefaultMaxRetries); err != nil {
							logger.Error(err, "Failed to remove Service from AddressGroupPortMapping.AccessPorts")
							return ctrl.Result{}, err
						}
						logger.Info("Removed Service from AddressGroupPortMapping.AccessPorts",
							"service", service.GetName(),
							"addressGroup", addressGroupRef.GetName())
						serviceFound = true
						break
					}
				}

				// If service not found in the updated port mapping, it's already been removed
				if !serviceFound {
					logger.Info("Service already removed from AddressGroupPortMapping.AccessPorts",
						"service", service.GetName(),
						"addressGroup", addressGroupRef.GetName())
				}
				break
			}
		}
	}

	// 4. Get the latest version of the service before removing finalizer
	updatedService := &netguardv1alpha1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, updatedService); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource already deleted, nothing to do
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get updated Service")
		return ctrl.Result{}, err
	}

	// Remove finalizer with retry
	if err := RemoveFinalizer(ctx, r.Client, updatedService, finalizer); err != nil {
		logger.Error(err, "Failed to remove finalizer from Service")
		return ctrl.Result{}, err
	}

	logger.Info("Service deleted successfully")
	return ctrl.Result{}, nil
}

// setServiceCondition updates a condition in the status
func setServiceCondition(service *netguardv1alpha1.Service, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Find and update existing condition or append new one
	for i, cond := range service.Status.Conditions {
		if cond.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if cond.Status != status {
				service.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Condition not found, append it
	service.Status.Conditions = append(service.Status.Conditions, condition)
}

// findServicesForPortMapping finds services that are referenced in an AddressGroupPortMapping
func (r *ServiceReconciler) findServicesForPortMapping(ctx context.Context, obj client.Object) []reconcile.Request {
	portMapping, ok := obj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	// For each service in AccessPorts, create a reconcile request
	for _, serviceRef := range portMapping.AccessPorts.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      serviceRef.GetName(),
				Namespace: serviceRef.GetNamespace(),
			},
		})
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.Service{}).
		Owns(&netguardv1alpha1.AddressGroupBinding{}).
		// Watch for changes to AddressGroupPortMapping
		Watches(
			&netguardv1alpha1.AddressGroupPortMapping{},
			handler.EnqueueRequestsFromMapFunc(r.findServicesForPortMapping),
		).
		Named("service").
		Complete(r)
}
