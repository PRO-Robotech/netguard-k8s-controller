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
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
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
// +kubebuilder:rbac:groups=sgroups.io,resources=addressgroups,verbs=get;list;watch

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

	// Log current state of the resource
	logger.Info("AddressGroupBinding current state",
		"name", binding.Name,
		"namespace", binding.Namespace,
		"deletionTimestamp", binding.DeletionTimestamp,
		"finalizers", binding.Finalizers,
		"ownerReferences", formatOwnerReferences(binding.OwnerReferences),
		"serviceRef", formatObjectReference(binding.Spec.ServiceRef),
		"addressGroupRef", formatNamespacedObjectReference(binding.Spec.AddressGroupRef),
		"conditions", formatConditions(binding.Status.Conditions),
		"generation", binding.Generation,
		"resourceVersion", binding.ResourceVersion)

	// TEMPORARY-DEBUG-CODE: Detailed logging for problematic resources
	if binding.Name == "dynamic-2rx8z" || binding.Name == "dynamic-7dls7" ||
		binding.Name == "dynamic-fb5qw" || binding.Name == "dynamic-g6jfj" ||
		binding.Name == "dynamic-jd2b7" || binding.Name == "dynamic-lsjlt" {

		logger.Info("TEMPORARY-DEBUG-CODE: Detailed state of problematic binding",
			"name", binding.Name,
			"namespace", binding.Namespace,
			"generation", binding.Generation,
			"resourceVersion", binding.ResourceVersion,
			"finalizers", binding.Finalizers,
			"ownerReferences", formatOwnerReferences(binding.OwnerReferences),
			"serviceRef", formatObjectReference(binding.Spec.ServiceRef),
			"addressGroupRef", formatNamespacedObjectReference(binding.Spec.AddressGroupRef),
			"conditions", formatConditions(binding.Status.Conditions))
	}

	// Add finalizer if it doesn't exist
	const finalizer = "addressgroupbinding.netguard.sgroups.io/finalizer"
	if !controllerutil.ContainsFinalizer(binding, finalizer) {
		controllerutil.AddFinalizer(binding, finalizer)
		if err := UpdateWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
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
	logger.Info("Starting normal reconciliation for AddressGroupBinding",
		"name", binding.Name,
		"namespace", binding.Namespace)

	// 1. Get the Service
	serviceRef := binding.Spec.ServiceRef
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: binding.GetNamespace(), // Service is in the same namespace as the binding
	}
	logger.Info("Looking up Service",
		"serviceName", serviceRef.GetName(),
		"serviceNamespace", binding.GetNamespace())

	if err := r.Get(ctx, serviceKey, service); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Service not found, will requeue after 1 minute",
				"serviceName", serviceRef.GetName(),
				"serviceNamespace", binding.GetNamespace())

			// Set condition to indicate that the Service was not found
			setCondition(binding, netguardv1alpha1.ConditionReady, metav1.ConditionFalse, "ServiceNotFound",
				fmt.Sprintf("Service %s not found", serviceRef.GetName()))
			if err := UpdateStatusWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
				logger.Error(err, "Failed to update AddressGroupBinding status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	logger.Info("Service found",
		"serviceName", service.Name,
		"serviceUID", service.UID,
		"addressGroups", len(service.AddressGroups.Items))

	// 2. Get the AddressGroup
	addressGroupRef := binding.Spec.AddressGroupRef
	addressGroupNamespace := v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), binding.GetNamespace())

	logger.Info("Looking up AddressGroup",
		"addressGroupName", addressGroupRef.GetName(),
		"addressGroupNamespace", addressGroupNamespace,
		"originalNamespace", addressGroupRef.GetNamespace())

	addressGroup := &providerv1alpha1.AddressGroup{}
	addressGroupKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}
	if err := r.Get(ctx, addressGroupKey, addressGroup); err != nil {
		if apierrors.IsNotFound(err) {
			// Check if we already have a condition for AddressGroupNotFound with the same generation
			var existingCondition *metav1.Condition
			for i := range binding.Status.Conditions {
				if binding.Status.Conditions[i].Type == netguardv1alpha1.ConditionReady &&
					binding.Status.Conditions[i].Reason == "AddressGroupNotFound" &&
					binding.Status.Conditions[i].ObservedGeneration == binding.Generation {
					existingCondition = &binding.Status.Conditions[i]
					break
				}
			}

			// If condition already exists with the same generation, update with detailed message and don't requeue
			if existingCondition != nil {
				logger.Info("AddressGroup not found, not requeuing until resource changes",
					"addressGroupName", addressGroupRef.GetName(),
					"addressGroupNamespace", addressGroupNamespace)

				// Update the message with more detailed information
				meta.SetStatusCondition(&binding.Status.Conditions, metav1.Condition{
					Type:   netguardv1alpha1.ConditionReady,
					Status: metav1.ConditionFalse,
					Reason: "AddressGroupNotFound",
					Message: fmt.Sprintf("AddressGroup %s not found in namespace %s. This binding will not be reconciled until the AddressGroup is created or the resource is modified.",
						addressGroupRef.GetName(), addressGroupNamespace),
					ObservedGeneration: binding.Generation,
					LastTransitionTime: existingCondition.LastTransitionTime,
				})

				if err := UpdateStatusWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
					logger.Error(err, "Failed to update AddressGroupBinding status")
					return ctrl.Result{}, err
				}

				// Don't requeue
				return ctrl.Result{}, nil
			}

			// First time seeing this issue or generation changed, set condition and requeue once
			logger.Info("AddressGroup not found, will requeue once to update status",
				"addressGroupName", addressGroupRef.GetName(),
				"addressGroupNamespace", addressGroupNamespace)

			meta.SetStatusCondition(&binding.Status.Conditions, metav1.Condition{
				Type:   netguardv1alpha1.ConditionReady,
				Status: metav1.ConditionFalse,
				Reason: "AddressGroupNotFound",
				Message: fmt.Sprintf("AddressGroup %s not found in namespace %s. This binding will be reconciled once more to update status.",
					addressGroupRef.GetName(), addressGroupNamespace),
				ObservedGeneration: binding.Generation,
				LastTransitionTime: metav1.Now(),
			})

			if err := UpdateStatusWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
				logger.Error(err, "Failed to update AddressGroupBinding status")
				return ctrl.Result{}, err
			}

			// Requeue after a short time to update the status with the final message
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		logger.Error(err, "Failed to get AddressGroup")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroup found",
		"addressGroupName", addressGroup.Name,
		"addressGroupUID", addressGroup.UID,
		"addressGroupNamespace", addressGroup.Namespace)

	// 2.1 Get the AddressGroupPortMapping for port information
	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	portMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(), // Port mapping has the same name as the address group
		Namespace: addressGroupNamespace,
	}

	logger.Info("Looking up AddressGroupPortMapping",
		"portMappingName", portMappingKey.Name,
		"portMappingNamespace", portMappingKey.Namespace)

	if err := r.Get(ctx, portMappingKey, portMapping); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AddressGroupPortMapping not found, creating a new one",
				"portMappingName", portMappingKey.Name,
				"portMappingNamespace", portMappingKey.Namespace)

			// Create a new AddressGroupPortMapping if it doesn't exist
			portMapping = &netguardv1alpha1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addressGroupRef.GetName(),
					Namespace: addressGroupNamespace,
				},
				Spec: netguardv1alpha1.AddressGroupPortMappingSpec{},
				AccessPorts: netguardv1alpha1.AccessPortsSpec{
					Items: []netguardv1alpha1.ServicePortsRef{},
				},
			}

			// Add OwnerReference to AddressGroup
			if err := controllerutil.SetControllerReference(addressGroup, portMapping, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference on AddressGroupPortMapping")
				return ctrl.Result{}, err
			}

			if err := r.Create(ctx, portMapping); err != nil {
				logger.Error(err, "Failed to create AddressGroupPortMapping")
				return ctrl.Result{}, err
			}
			logger.Info("Created new AddressGroupPortMapping",
				"name", portMapping.GetName(),
				"namespace", portMapping.GetNamespace(),
				"ownerReference", formatOwnerReferences(portMapping.OwnerReferences))
		} else {
			logger.Error(err, "Failed to get AddressGroupPortMapping")
			return ctrl.Result{}, err
		}
	} else {
		logger.Info("AddressGroupPortMapping found",
			"name", portMapping.GetName(),
			"namespace", portMapping.GetNamespace(),
			"servicePortsCount", len(portMapping.AccessPorts.Items))
	}

	// Add OwnerReferences to the binding for Service and AddressGroup
	logger.Info("Checking owner references",
		"currentOwnerRefs", formatOwnerReferences(binding.OwnerReferences))

	ownerRefsUpdated := false

	// Add OwnerReference to Service
	serviceOwnerRef := metav1.OwnerReference{
		APIVersion:         service.APIVersion,
		Kind:               service.Kind,
		Name:               service.Name,
		UID:                service.UID,
		BlockOwnerDeletion: pointer.Bool(false),
		Controller:         pointer.Bool(false),
	}
	if !containsOwnerReference(binding.GetOwnerReferences(), serviceOwnerRef) {
		logger.Info("Adding Service owner reference",
			"service", fmt.Sprintf("%s/%s", service.Kind, service.Name),
			"serviceUID", service.UID)

		binding.OwnerReferences = append(binding.OwnerReferences, serviceOwnerRef)
		ownerRefsUpdated = true
	}

	// Add OwnerReference to AddressGroup
	agOwnerRef := metav1.OwnerReference{
		APIVersion:         addressGroup.APIVersion,
		Kind:               addressGroup.Kind,
		Name:               addressGroup.Name,
		UID:                addressGroup.UID,
		BlockOwnerDeletion: pointer.Bool(false),
		Controller:         pointer.Bool(false),
	}
	if !containsOwnerReference(binding.GetOwnerReferences(), agOwnerRef) {
		logger.Info("Adding AddressGroup owner reference",
			"addressGroup", fmt.Sprintf("%s/%s", addressGroup.Kind, addressGroup.Name),
			"addressGroupUID", addressGroup.UID)

		binding.OwnerReferences = append(binding.OwnerReferences, agOwnerRef)
		ownerRefsUpdated = true
	}

	// If owner references were updated, update the binding
	if ownerRefsUpdated {
		logger.Info("Updating binding with new owner references",
			"updatedOwnerRefs", formatOwnerReferences(binding.OwnerReferences))

		if err := UpdateWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update AddressGroupBinding with owner references")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully updated binding with owner references")
	} else {
		logger.Info("No owner reference updates needed")
	}

	// 3. Update Service.AddressGroups
	logger.Info("Checking if AddressGroup is already in Service.AddressGroups",
		"serviceAddressGroupsCount", len(service.AddressGroups.Items),
		"addressGroupToAdd", formatNamespacedObjectReference(addressGroupRef))

	addressGroupFound := false
	for _, ag := range service.AddressGroups.Items {
		if ag.GetName() == addressGroupRef.GetName() &&
			ag.GetNamespace() == addressGroupRef.GetNamespace() {
			logger.Info("AddressGroup already exists in Service.AddressGroups",
				"addressGroup", formatNamespacedObjectReference(ag))
			addressGroupFound = true
			break
		}
	}

	if !addressGroupFound {
		logger.Info("AddressGroup not found in Service.AddressGroups, adding it",
			"addressGroup", formatNamespacedObjectReference(addressGroupRef))

		service.AddressGroups.Items = append(service.AddressGroups.Items, addressGroupRef)
		if err := UpdateWithRetry(ctx, r.Client, service, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update Service.AddressGroups")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully added AddressGroup to Service.AddressGroups",
			"service", service.GetName(),
			"addressGroup", addressGroupRef.GetName(),
			"updatedAddressGroupsCount", len(service.AddressGroups.Items))
	} else {
		logger.Info("No Service.AddressGroups update needed")
	}

	// 4. Update AddressGroupPortMapping.AccessPorts
	logger.Info("Preparing to update AddressGroupPortMapping.AccessPorts")

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

	logger.Info("Created ServicePortsRef",
		"service", fmt.Sprintf("%s/%s", service.GetNamespace(), service.GetName()))

	servicePortsFound := false
	for i, sp := range portMapping.AccessPorts.Items {
		if sp.GetName() == service.GetName() &&
			sp.GetNamespace() == service.GetNamespace() {
			logger.Info("Found existing ServicePortsRef in AddressGroupPortMapping",
				"service", fmt.Sprintf("%s/%s", sp.GetNamespace(), sp.GetName()))

			// Update ports if they've changed
			if !reflect.DeepEqual(sp.Ports, servicePortsRef.Ports) {
				logger.Info("Ports have changed, updating ServicePortsRef",
					"service", fmt.Sprintf("%s/%s", sp.GetNamespace(), sp.GetName()))

				// Create a copy for patching
				original := portMapping.DeepCopy()

				// Update the ports
				portMapping.AccessPorts.Items[i].Ports = servicePortsRef.Ports

				// Apply patch with retry
				patch := client.MergeFrom(original)
				if err := PatchWithRetry(ctx, r.Client, portMapping, patch, DefaultMaxRetries); err != nil {
					logger.Error(err, "Failed to update AddressGroupPortMapping.AccessPorts")
					return ctrl.Result{}, err
				}
				logger.Info("Successfully updated Service ports in AddressGroupPortMapping",
					"service", service.GetName(),
					"addressGroup", addressGroupRef.GetName())
			} else {
				logger.Info("Ports have not changed, no update needed")
			}
			servicePortsFound = true
			break
		}
	}

	if !servicePortsFound {
		logger.Info("Service not found in AddressGroupPortMapping.AccessPorts, adding it",
			"service", fmt.Sprintf("%s/%s", service.GetNamespace(), service.GetName()),
			"currentItemsCount", len(portMapping.AccessPorts.Items))

		// Create a copy for patching
		original := portMapping.DeepCopy()

		// Add the service to the list
		portMapping.AccessPorts.Items = append(portMapping.AccessPorts.Items, servicePortsRef)

		// Apply patch with retry
		patch := client.MergeFrom(original)
		if err := PatchWithRetry(ctx, r.Client, portMapping, patch, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to add Service to AddressGroupPortMapping.AccessPorts")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully added Service to AddressGroupPortMapping.AccessPorts",
			"service", service.GetName(),
			"addressGroup", addressGroupRef.GetName(),
			"updatedItemsCount", len(portMapping.AccessPorts.Items))
	}

	// 5. Update status
	logger.Info("Updating AddressGroupBinding status to Ready")

	setCondition(binding, netguardv1alpha1.ConditionReady, metav1.ConditionTrue, "BindingCreated",
		"AddressGroupBinding successfully created")

	// Log the updated conditions before saving
	logger.Info("Updated conditions",
		"conditions", formatConditions(binding.Status.Conditions))

	if err := UpdateStatusWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to update AddressGroupBinding status")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupBinding reconciled successfully",
		"name", binding.Name,
		"namespace", binding.Namespace,
		"generation", binding.Generation,
		"resourceVersion", binding.ResourceVersion)

	// TEMPORARY-DEBUG-CODE: Final state logging for problematic resources
	if binding.Name == "dynamic-2rx8z" || binding.Name == "dynamic-7dls7" ||
		binding.Name == "dynamic-fb5qw" || binding.Name == "dynamic-g6jfj" ||
		binding.Name == "dynamic-jd2b7" || binding.Name == "dynamic-lsjlt" {

		logger.Info("TEMPORARY-DEBUG-CODE: Final state of problematic binding after successful reconciliation",
			"name", binding.Name,
			"namespace", binding.Namespace,
			"generation", binding.Generation,
			"resourceVersion", binding.ResourceVersion,
			"finalizers", binding.Finalizers,
			"ownerReferences", formatOwnerReferences(binding.OwnerReferences),
			"conditions", formatConditions(binding.Status.Conditions))
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles the deletion of an AddressGroupBinding
func (r *AddressGroupBindingReconciler) reconcileDelete(ctx context.Context, binding *netguardv1alpha1.AddressGroupBinding, finalizer string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting AddressGroupBinding",
		"name", binding.GetName(),
		"namespace", binding.GetNamespace(),
		"finalizers", binding.Finalizers,
		"conditions", formatConditions(binding.Status.Conditions))

	// TEMPORARY-DEBUG-CODE: Detailed logging for problematic resources being deleted
	if binding.Name == "dynamic-2rx8z" || binding.Name == "dynamic-7dls7" ||
		binding.Name == "dynamic-fb5qw" || binding.Name == "dynamic-g6jfj" ||
		binding.Name == "dynamic-jd2b7" || binding.Name == "dynamic-lsjlt" {

		logger.Info("TEMPORARY-DEBUG-CODE: Detailed state of problematic binding being deleted",
			"name", binding.Name,
			"namespace", binding.Namespace,
			"generation", binding.Generation,
			"resourceVersion", binding.ResourceVersion,
			"finalizers", binding.Finalizers,
			"ownerReferences", formatOwnerReferences(binding.OwnerReferences),
			"serviceRef", formatObjectReference(binding.Spec.ServiceRef),
			"addressGroupRef", formatNamespacedObjectReference(binding.Spec.AddressGroupRef),
			"conditions", formatConditions(binding.Status.Conditions))
	}

	// 1. Remove AddressGroup from Service.AddressGroups
	serviceRef := binding.Spec.ServiceRef
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: binding.GetNamespace(),
	}

	logger.Info("Looking up Service for deletion cleanup",
		"serviceName", serviceRef.GetName(),
		"serviceNamespace", binding.GetNamespace())

	err := r.Get(ctx, serviceKey, service)
	if err == nil {
		logger.Info("Service found, checking for AddressGroup to remove",
			"serviceName", service.GetName(),
			"serviceUID", service.UID,
			"addressGroupsCount", len(service.AddressGroups.Items))

		// Service exists, remove AddressGroup from its list
		addressGroupRef := binding.Spec.AddressGroupRef
		addressGroupFound := false

		for i, ag := range service.AddressGroups.Items {
			if ag.GetName() == addressGroupRef.GetName() &&
				ag.GetNamespace() == addressGroupRef.GetNamespace() {
				addressGroupFound = true
				logger.Info("Found AddressGroup in Service.AddressGroups, removing it",
					"addressGroup", formatNamespacedObjectReference(ag),
					"index", i)

				// Remove the item from the slice
				service.AddressGroups.Items = append(
					service.AddressGroups.Items[:i],
					service.AddressGroups.Items[i+1:]...)

				if err := UpdateWithRetry(ctx, r.Client, service, DefaultMaxRetries); err != nil {
					logger.Error(err, "Failed to remove AddressGroup from Service.AddressGroups")
					return ctrl.Result{}, err
				}
				logger.Info("Successfully removed AddressGroup from Service.AddressGroups",
					"service", service.GetName(),
					"addressGroup", addressGroupRef.GetName(),
					"remainingAddressGroups", len(service.AddressGroups.Items))
				break
			}
		}

		if !addressGroupFound {
			logger.Info("AddressGroup not found in Service.AddressGroups, nothing to remove",
				"addressGroup", formatNamespacedObjectReference(addressGroupRef))
		}
	} else if apierrors.IsNotFound(err) {
		// Service not found
		logger.Info("Service not found, skipping Service.AddressGroups cleanup",
			"serviceName", serviceRef.GetName())
	} else {
		// Error other than "not found"
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// 2. Remove Service from AddressGroupPortMapping.AccessPorts
	addressGroupRef := binding.Spec.AddressGroupRef
	addressGroupNamespace := v1alpha1.ResolveNamespace(addressGroupRef.GetNamespace(), binding.GetNamespace())

	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	portMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}

	logger.Info("Looking up AddressGroupPortMapping for deletion cleanup",
		"portMappingName", portMappingKey.Name,
		"portMappingNamespace", portMappingKey.Namespace,
		"originalNamespace", addressGroupRef.GetNamespace())

	err = r.Get(ctx, portMappingKey, portMapping)
	if err == nil {
		logger.Info("AddressGroupPortMapping found, checking for Service to remove",
			"portMappingName", portMapping.GetName(),
			"portMappingNamespace", portMapping.GetNamespace(),
			"servicePortsCount", len(portMapping.AccessPorts.Items))

		// PortMapping exists, remove Service from its list
		serviceFound := false

		for i, sp := range portMapping.AccessPorts.Items {
			if sp.GetName() == serviceRef.GetName() &&
				sp.GetNamespace() == binding.GetNamespace() {
				serviceFound = true
				logger.Info("Found Service in AddressGroupPortMapping.AccessPorts, removing it",
					"service", fmt.Sprintf("%s/%s", sp.GetNamespace(), sp.GetName()),
					"index", i)

				// Create a copy for patching
				original := portMapping.DeepCopy()

				// Remove the item from the slice
				portMapping.AccessPorts.Items = append(
					portMapping.AccessPorts.Items[:i],
					portMapping.AccessPorts.Items[i+1:]...)

				// Apply patch with retry
				patch := client.MergeFrom(original)
				if err := PatchWithRetry(ctx, r.Client, portMapping, patch, DefaultMaxRetries); err != nil {
					logger.Error(err, "Failed to remove Service from AddressGroupPortMapping.AccessPorts")
					return ctrl.Result{}, err
				}
				logger.Info("Successfully removed Service from AddressGroupPortMapping.AccessPorts",
					"service", serviceRef.GetName(),
					"addressGroup", addressGroupRef.GetName(),
					"remainingServicePorts", len(portMapping.AccessPorts.Items))
				break
			}
		}

		if !serviceFound {
			logger.Info("Service not found in AddressGroupPortMapping.AccessPorts, nothing to remove",
				"service", fmt.Sprintf("%s/%s", binding.GetNamespace(), serviceRef.GetName()))
		}
	} else if apierrors.IsNotFound(err) {
		// PortMapping not found
		logger.Info("AddressGroupPortMapping not found, skipping AccessPorts cleanup",
			"portMappingName", portMappingKey.Name,
			"portMappingNamespace", portMappingKey.Namespace)
	} else {
		// Error other than "not found"
		logger.Error(err, "Failed to get AddressGroupPortMapping")
		return ctrl.Result{}, err
	}

	// 3. Remove finalizer
	logger.Info("Removing finalizer",
		"name", binding.GetName(),
		"finalizer", finalizer,
		"currentFinalizers", binding.Finalizers)

	controllerutil.RemoveFinalizer(binding, finalizer)
	if err := UpdateWithRetry(ctx, r.Client, binding, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to remove finalizer from AddressGroupBinding")
		return ctrl.Result{}, err
	}

	logger.Info("AddressGroupBinding deleted successfully",
		"name", binding.GetName(),
		"namespace", binding.GetNamespace())

	// TEMPORARY-DEBUG-CODE: Final state logging for problematic resources being deleted
	if binding.Name == "dynamic-2rx8z" || binding.Name == "dynamic-7dls7" ||
		binding.Name == "dynamic-fb5qw" || binding.Name == "dynamic-g6jfj" ||
		binding.Name == "dynamic-jd2b7" || binding.Name == "dynamic-lsjlt" {

		logger.Info("TEMPORARY-DEBUG-CODE: Final state of problematic binding after successful deletion",
			"name", binding.Name,
			"namespace", binding.Namespace,
			"generation", binding.Generation,
			"resourceVersion", binding.ResourceVersion,
			"finalizers", binding.Finalizers)
	}

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

// containsOwnerReference checks if the list of owner references contains a reference with the same UID
func containsOwnerReference(refs []metav1.OwnerReference, ref metav1.OwnerReference) bool {
	for _, r := range refs {
		if r.UID == ref.UID {
			return true
		}
	}
	return false
}

// formatConditions formats a slice of conditions into a readable string
func formatConditions(conditions []metav1.Condition) string {
	var result []string
	for _, c := range conditions {
		result = append(result, fmt.Sprintf("%s=%s(%s)", c.Type, c.Status, c.Reason))
	}
	return strings.Join(result, ", ")
}

// formatOwnerReferences formats a slice of owner references into a readable string
func formatOwnerReferences(refs []metav1.OwnerReference) string {
	var result []string
	for _, ref := range refs {
		result = append(result, fmt.Sprintf("%s/%s(%s)", ref.Kind, ref.Name, ref.UID))
	}
	return strings.Join(result, ", ")
}

// formatObjectReference formats an ObjectReference into a readable string
func formatObjectReference(ref netguardv1alpha1.ObjectReference) string {
	return fmt.Sprintf("%s/%s/%s", ref.APIVersion, ref.Kind, ref.Name)
}

// formatNamespacedObjectReference formats a NamespacedObjectReference into a readable string
func formatNamespacedObjectReference(ref netguardv1alpha1.NamespacedObjectReference) string {
	return fmt.Sprintf("%s/%s/%s/%s", ref.APIVersion, ref.Kind, ref.GetNamespace(), ref.Name)
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

// findBindingsForAddressGroup finds bindings that reference an address group
func (r *AddressGroupBindingReconciler) findBindingsForAddressGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	addressGroup, ok := obj.(*providerv1alpha1.AddressGroup)
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
		if binding.Spec.AddressGroupRef.GetName() == addressGroup.GetName() &&
			(binding.Spec.AddressGroupRef.GetNamespace() == addressGroup.GetNamespace() ||
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
		// Watch for changes to AddressGroup
		Watches(
			&providerv1alpha1.AddressGroup{},
			handler.EnqueueRequestsFromMapFunc(r.findBindingsForAddressGroup),
		).
		// Watch for changes to AddressGroupPortMapping (for backward compatibility)
		Watches(
			&netguardv1alpha1.AddressGroupPortMapping{},
			handler.EnqueueRequestsFromMapFunc(r.findBindingsForPortMapping),
		).
		Named("addressgroupbinding").
		Complete(r)
}
