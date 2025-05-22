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
	"reflect"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	"sgroups.io/netguard/internal/webhook/v1alpha1"
)

// AddressGroupBindingReconciler reconciles a AddressGroupBinding object
type AddressGroupBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=addressgroupportmappings,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *AddressGroupBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AddressGroupBinding", "request", req)

	// Get the AddressGroupBinding resource
	binding := &netguardv1alpha1.AddressGroupBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, likely deleted
			logger.Info("AddressGroupBinding not found, it may have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AddressGroupBinding")
		return ctrl.Result{}, err
	}

	// Add finalizer if it doesn't exist
	const finalizer = "addressgroupbinding.netguard.sgroups.io/finalizer"
	if !controllerutil.ContainsFinalizer(binding, finalizer) {
		controllerutil.AddFinalizer(binding, finalizer)
		if err := r.Update(ctx, binding); err != nil {
			logger.Error(err, "Failed to add finalizer to AddressGroupBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil // Requeue to continue reconciliation
	}

	// Check if the resource is being deleted
	if !binding.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, binding, finalizer)
	}

	// Normal reconciliation
	return r.reconcileNormal(ctx, binding)
}

// reconcileNormal handles the normal reconciliation of an AddressGroupBinding
func (r *AddressGroupBindingReconciler) reconcileNormal(ctx context.Context, binding *netguardv1alpha1.AddressGroupBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Get the Service
	serviceRef := binding.Spec.ServiceRef
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: binding.GetNamespace(), // Service is in the same namespace as the binding
	}
	if err := r.Get(ctx, serviceKey, service); err != nil {
		if apierrors.IsNotFound(err) {
			// Set condition to indicate that the Service was not found
			setCondition(binding, netguardv1alpha1.ConditionReady, metav1.ConditionFalse, "ServiceNotFound",
				fmt.Sprintf("Service %s not found", serviceRef.GetName()))
			if err := r.Status().Update(ctx, binding); err != nil {
				logger.Error(err, "Failed to update AddressGroupBinding status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// 2. Get the AddressGroupPortMapping
	addressGroupRef := binding.Spec.AddressGroupRef
	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	portMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(), // Port mapping has the same name as the address group
		Namespace: v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), binding.GetNamespace()),
	}
	if err := r.Get(ctx, portMappingKey, portMapping); err != nil {
		if apierrors.IsNotFound(err) {
			// Set condition to indicate that the AddressGroupPortMapping was not found
			setCondition(binding, netguardv1alpha1.ConditionReady, metav1.ConditionFalse, "PortMappingNotFound",
				fmt.Sprintf("AddressGroupPortMapping for AddressGroup %s not found", addressGroupRef.GetName()))
			if err := r.Status().Update(ctx, binding); err != nil {
				logger.Error(err, "Failed to update AddressGroupBinding status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		logger.Error(err, "Failed to get AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	// 3. Update Service.AddressGroups
	addressGroupFound := false
	for _, ag := range service.AddressGroups.Items {
		if ag.GetName() == addressGroupRef.GetName() &&
			ag.GetNamespace() == addressGroupRef.GetNamespace() {
			addressGroupFound = true
			break
		}
	}

	if !addressGroupFound {
		service.AddressGroups.Items = append(service.AddressGroups.Items, addressGroupRef)
		if err := r.Update(ctx, service); err != nil {
			logger.Error(err, "Failed to update Service.AddressGroups")
			return ctrl.Result{}, err
		}
		logger.Info("Added AddressGroup to Service.AddressGroups",
			"service", service.GetName(),
			"addressGroup", addressGroupRef.GetName())
	}

	// 4. Update AddressGroupPortMapping.AccessPorts
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

	servicePortsFound := false
	for i, sp := range portMapping.AccessPorts.Items {
		if sp.GetName() == service.GetName() &&
			sp.GetNamespace() == service.GetNamespace() {
			// Update ports if they've changed
			if !reflect.DeepEqual(sp.Ports, servicePortsRef.Ports) {
				portMapping.AccessPorts.Items[i].Ports = servicePortsRef.Ports
				if err := r.Update(ctx, portMapping); err != nil {
					logger.Error(err, "Failed to update AddressGroupPortMapping.AccessPorts")
					return ctrl.Result{}, err
				}
				logger.Info("Updated Service ports in AddressGroupPortMapping",
					"service", service.GetName(),
					"addressGroup", addressGroupRef.GetName())
			}
			servicePortsFound = true
			break
		}
	}

	if !servicePortsFound {
		portMapping.AccessPorts.Items = append(portMapping.AccessPorts.Items, servicePortsRef)
		if err := r.Update(ctx, portMapping); err != nil {
			logger.Error(err, "Failed to add Service to AddressGroupPortMapping.AccessPorts")
			return ctrl.Result{}, err
		}
		logger.Info("Added Service to AddressGroupPortMapping.AccessPorts",
			"service", service.GetName(),
			"addressGroup", addressGroupRef.GetName())
	}

	// 5. Update status
	setCondition(binding, netguardv1alpha1.ConditionReady, metav1.ConditionTrue, "BindingCreated",
		"AddressGroupBinding successfully created")
	if err := r.Status().Update(ctx, binding); err != nil {
		logger.Error(err, "Failed to update AddressGroupBinding status")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupBinding reconciled successfully")
	return ctrl.Result{}, nil
}

// reconcileDelete handles the deletion of an AddressGroupBinding
func (r *AddressGroupBindingReconciler) reconcileDelete(ctx context.Context, binding *netguardv1alpha1.AddressGroupBinding, finalizer string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting AddressGroupBinding", "name", binding.GetName())

	// 1. Remove AddressGroup from Service.AddressGroups
	serviceRef := binding.Spec.ServiceRef
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: binding.GetNamespace(),
	}

	err := r.Get(ctx, serviceKey, service)
	if err == nil {
		// Service exists, remove AddressGroup from its list
		addressGroupRef := binding.Spec.AddressGroupRef
		for i, ag := range service.AddressGroups.Items {
			if ag.GetName() == addressGroupRef.GetName() &&
				ag.GetNamespace() == addressGroupRef.GetNamespace() {
				// Remove the item from the slice
				service.AddressGroups.Items = append(
					service.AddressGroups.Items[:i],
					service.AddressGroups.Items[i+1:]...)

				if err := r.Update(ctx, service); err != nil {
					logger.Error(err, "Failed to remove AddressGroup from Service.AddressGroups")
					return ctrl.Result{}, err
				}
				logger.Info("Removed AddressGroup from Service.AddressGroups",
					"service", service.GetName(),
					"addressGroup", addressGroupRef.GetName())
				break
			}
		}
	} else if !apierrors.IsNotFound(err) {
		// Error other than "not found"
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// 2. Remove Service from AddressGroupPortMapping.AccessPorts
	addressGroupRef := binding.Spec.AddressGroupRef
	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	portMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), binding.GetNamespace()),
	}

	err = r.Get(ctx, portMappingKey, portMapping)
	if err == nil {
		// PortMapping exists, remove Service from its list
		for i, sp := range portMapping.AccessPorts.Items {
			if sp.GetName() == serviceRef.GetName() &&
				sp.GetNamespace() == binding.GetNamespace() {
				// Remove the item from the slice
				portMapping.AccessPorts.Items = append(
					portMapping.AccessPorts.Items[:i],
					portMapping.AccessPorts.Items[i+1:]...)

				if err := r.Update(ctx, portMapping); err != nil {
					logger.Error(err, "Failed to remove Service from AddressGroupPortMapping.AccessPorts")
					return ctrl.Result{}, err
				}
				logger.Info("Removed Service from AddressGroupPortMapping.AccessPorts",
					"service", serviceRef.GetName(),
					"addressGroup", addressGroupRef.GetName())
				break
			}
		}
	} else if !apierrors.IsNotFound(err) {
		// Error other than "not found"
		logger.Error(err, "Failed to get AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	// 3. Remove finalizer
	controllerutil.RemoveFinalizer(binding, finalizer)
	if err := r.Update(ctx, binding); err != nil {
		logger.Error(err, "Failed to remove finalizer from AddressGroupBinding")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupBinding deleted successfully")
	return ctrl.Result{}, nil
}

// setCondition updates a condition in the status
func setCondition(binding *netguardv1alpha1.AddressGroupBinding, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Find and update existing condition or append new one
	for i, cond := range binding.Status.Conditions {
		if cond.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if cond.Status != status {
				binding.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Condition not found, append it
	binding.Status.Conditions = append(binding.Status.Conditions, condition)
}

// findBindingsForService finds bindings that reference a specific service
func (r *AddressGroupBindingReconciler) findBindingsForService(ctx context.Context, obj client.Object) []reconcile.Request {
	service, ok := obj.(*netguardv1alpha1.Service)
	if !ok {
		return nil
	}

	// Get all AddressGroupBinding in the same namespace
	bindingList := &netguardv1alpha1.AddressGroupBindingList{}
	if err := r.List(ctx, bindingList, client.InNamespace(service.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request

	// Filter bindings that reference this service
	for _, binding := range bindingList.Items {
		if binding.Spec.ServiceRef.GetName() == service.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      binding.GetName(),
					Namespace: binding.GetNamespace(),
				},
			})
		}
	}

	return requests
}

// findBindingsForPortMapping finds bindings that reference an address group in a port mapping
func (r *AddressGroupBindingReconciler) findBindingsForPortMapping(ctx context.Context, obj client.Object) []reconcile.Request {
	portMapping, ok := obj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil
	}

	// Get all AddressGroupBinding
	bindingList := &netguardv1alpha1.AddressGroupBindingList{}
	if err := r.List(ctx, bindingList); err != nil {
		return nil
	}

	var requests []reconcile.Request

	// Filter bindings that reference this address group
	for _, binding := range bindingList.Items {
		if binding.Spec.AddressGroupRef.GetName() == portMapping.GetName() &&
			(binding.Spec.AddressGroupRef.GetNamespace() == portMapping.GetNamespace() ||
				binding.Spec.AddressGroupRef.GetNamespace() == "") {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      binding.GetName(),
					Namespace: binding.GetNamespace(),
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddressGroupBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.AddressGroupBinding{}).
		// Watch for changes to Service
		Watches(
			&netguardv1alpha1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findBindingsForService),
		).
		// Watch for changes to AddressGroupPortMapping
		Watches(
			&netguardv1alpha1.AddressGroupPortMapping{},
			handler.EnqueueRequestsFromMapFunc(r.findBindingsForPortMapping),
		).
		Named("addressgroupbinding").
		Complete(r)
}
