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

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var addressgroupbindinglog = logf.Log.WithName("addressgroupbinding-resource")

// SetupAddressGroupBindingWebhookWithManager registers the webhook for AddressGroupBinding in the manager.
func SetupAddressGroupBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.AddressGroupBinding{}).
		WithValidator(&AddressGroupBindingCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-addressgroupbinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=addressgroupbindings,verbs=create;update,versions=v1alpha1,name=vaddressgroupbinding-v1alpha1.kb.io,admissionReviewVersions=v1

// AddressGroupBindingCustomValidator struct is responsible for validating the AddressGroupBinding resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type AddressGroupBindingCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &AddressGroupBindingCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupBinding.
func (v *AddressGroupBindingCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	binding, ok := obj.(*netguardv1alpha1.AddressGroupBinding)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBinding object but got %T", obj)
	}

	// TEMPORARY-DEBUG-CODE: Enhanced logging for problematic resources
	if binding.Name == "dynamic-2rx8z" || binding.Name == "dynamic-7dls7" ||
		binding.Name == "dynamic-fb5qw" || binding.Name == "dynamic-g6jfj" ||
		binding.Name == "dynamic-jd2b7" || binding.Name == "dynamic-lsjlt" {

		addressgroupbindinglog.Info("TEMPORARY-DEBUG-CODE: Detailed validation for problematic AddressGroupBinding creation",
			"name", binding.Name,
			"namespace", binding.Namespace,
			"generation", binding.Generation,
			"resourceVersion", binding.ResourceVersion,
			"serviceRef", fmt.Sprintf("%s/%s", binding.Spec.ServiceRef.GetAPIVersion(), binding.Spec.ServiceRef.GetName()),
			"addressGroupRef", fmt.Sprintf("%s/%s/%s", binding.Spec.AddressGroupRef.GetAPIVersion(),
				binding.Spec.AddressGroupRef.GetNamespace(), binding.Spec.AddressGroupRef.GetName()))
	} else {
		addressgroupbindinglog.Info("Validation for AddressGroupBinding upon creation",
			"name", binding.GetName(),
			"namespace", binding.GetNamespace(),
			"generation", binding.Generation)
	}

	// 1.1 Validate ServiceRef
	serviceRef := binding.Spec.ServiceRef
	if err := ValidateObjectReference(serviceRef, "Service", "netguard.sgroups.io/v1alpha1"); err != nil {
		return nil, err
	}

	// Check if Service exists
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: binding.GetNamespace(), // Service is in the same namespace as the binding
	}
	if err := v.Client.Get(ctx, serviceKey, service); err != nil {
		return nil, fmt.Errorf("service %s not found: %w", serviceRef.GetName(), err)
	}

	// 1.1 Validate AddressGroupRef
	addressGroupRef := binding.Spec.AddressGroupRef
	if addressGroupRef.GetName() == "" {
		return nil, fmt.Errorf("addressGroupRef.name cannot be empty")
	}
	if addressGroupRef.GetKind() != "AddressGroup" {
		return nil, fmt.Errorf("addressGroupRef must be to an AddressGroup resource, got %s", addressGroupRef.GetKind())
	}
	if addressGroupRef.GetAPIVersion() != "provider.sgroups.io/v1alpha1" {
		return nil, fmt.Errorf("addressGroupRef must be to a resource with APIVersion provider.sgroups.io/v1alpha1, got %s", addressGroupRef.GetAPIVersion())
	}

	// 1.2 Check if AddressGroup exists directly
	addressGroupNamespace := ResolveNamespace(addressGroupRef.GetNamespace(), binding.GetNamespace())
	addressGroup := &providerv1alpha1.AddressGroup{}
	addressGroupKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}
	if err := v.Client.Get(ctx, addressGroupKey, addressGroup); err != nil {
		return nil, fmt.Errorf("addressGroup %s not found in namespace %s: %w",
			addressGroupRef.GetName(),
			addressGroupNamespace,
			err)
	}

	// 1.3 Get AddressGroupPortMapping for port information
	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	portMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(), // Port mapping has the same name as the address group
		Namespace: addressGroupNamespace,
	}
	if err := v.Client.Get(ctx, portMappingKey, portMapping); err != nil {
		// If port mapping doesn't exist, we can't check port overlaps
		// This is not a critical error as the port mapping might be created later
		addressgroupbindinglog.Info("AddressGroupPortMapping not found, skipping port overlap check",
			"addressGroup", addressGroupRef.GetName(),
			"namespace", addressGroupNamespace)
	} else {
		// 1.4 Check for port overlaps if port mapping exists
		if err := CheckPortOverlaps(service, portMapping); err != nil {
			return nil, err
		}
	}

	// 1.5 Check cross-namespace policy rule

	// If the address group is in a different namespace than the binding/service
	if addressGroupNamespace != binding.GetNamespace() {
		// Check if there's a policy in the address group's namespace that allows this binding
		policyList := &netguardv1alpha1.AddressGroupBindingPolicyList{}
		if err := v.Client.List(ctx, policyList, client.InNamespace(addressGroupNamespace)); err != nil {
			return nil, fmt.Errorf("failed to list policies in namespace %s: %w", addressGroupNamespace, err)
		}

		// Look for a policy that references both the address group and service
		policyFound := false
		for _, policy := range policyList.Items {
			if policy.Spec.AddressGroupRef.GetName() == addressGroupRef.GetName() &&
				policy.Spec.ServiceRef.GetName() == binding.Spec.ServiceRef.GetName() &&
				ResolveNamespace(policy.Spec.ServiceRef.GetNamespace(), addressGroupNamespace) == binding.GetNamespace() {
				policyFound = true
				break
			}
		}

		if !policyFound {
			return nil, fmt.Errorf("cross-namespace binding not allowed: no AddressGroupBindingPolicy found in namespace %s that references both AddressGroup %s and Service %s",
				addressGroupNamespace, addressGroupRef.GetName(), binding.Spec.ServiceRef.GetName())
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupBinding.
func (v *AddressGroupBindingCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldBinding, ok := oldObj.(*netguardv1alpha1.AddressGroupBinding)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBinding object for oldObj but got %T", oldObj)
	}

	newBinding, ok := newObj.(*netguardv1alpha1.AddressGroupBinding)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBinding object for newObj but got %T", newObj)
	}

	// TEMPORARY-DEBUG-CODE: Enhanced logging for problematic resources
	if newBinding.Name == "dynamic-2rx8z" || newBinding.Name == "dynamic-7dls7" ||
		newBinding.Name == "dynamic-fb5qw" || newBinding.Name == "dynamic-g6jfj" ||
		newBinding.Name == "dynamic-jd2b7" || newBinding.Name == "dynamic-lsjlt" {

		addressgroupbindinglog.Info("TEMPORARY-DEBUG-CODE: Detailed validation for problematic AddressGroupBinding",
			"name", newBinding.Name,
			"namespace", newBinding.Namespace,
			"oldGeneration", oldBinding.Generation,
			"newGeneration", newBinding.Generation,
			"oldResourceVersion", oldBinding.ResourceVersion,
			"newResourceVersion", newBinding.ResourceVersion,
			"oldDeletionTimestamp", oldBinding.DeletionTimestamp,
			"newDeletionTimestamp", newBinding.DeletionTimestamp,
			"oldFinalizers", oldBinding.Finalizers,
			"newFinalizers", newBinding.Finalizers)
	} else {
		addressgroupbindinglog.Info("Validation for AddressGroupBinding upon update",
			"name", newBinding.GetName(),
			"namespace", newBinding.GetNamespace(),
			"generation", newBinding.Generation,
			"resourceVersion", newBinding.ResourceVersion)
	}

	// Skip validation for resources being deleted
	if SkipValidationForDeletion(ctx, newBinding) {
		addressgroupbindinglog.Info("Validation skipped for resource being deleted",
			"name", newBinding.GetName(),
			"namespace", newBinding.GetNamespace())
		return nil, nil
	}

	// 1.1 Ensure spec is immutable
	// Check that ServiceRef hasn't changed
	if err := ValidateObjectReferenceNotChanged(
		&oldBinding.Spec.ServiceRef,
		&newBinding.Spec.ServiceRef,
		"spec.serviceRef"); err != nil {
		return nil, err
	}

	// Check that AddressGroupRef hasn't changed
	if err := ValidateObjectReferenceNotChanged(
		&oldBinding.Spec.AddressGroupRef,
		&newBinding.Spec.AddressGroupRef,
		"spec.addressGroupRef"); err != nil {
		return nil, err
	}

	// 1.2 Check if Service exists
	serviceRef := newBinding.Spec.ServiceRef
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: newBinding.GetNamespace(),
	}
	if err := v.Client.Get(ctx, serviceKey, service); err != nil {
		return nil, fmt.Errorf("service %s not found: %w", serviceRef.GetName(), err)
	}

	// 1.2 Check if AddressGroup exists directly
	addressGroupRef := newBinding.Spec.AddressGroupRef
	addressGroupNamespace := ResolveNamespace(addressGroupRef.GetNamespace(), newBinding.GetNamespace())
	addressGroup := &providerv1alpha1.AddressGroup{}
	addressGroupKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}
	if err := v.Client.Get(ctx, addressGroupKey, addressGroup); err != nil {
		return nil, fmt.Errorf("addressGroup %s not found in namespace %s: %w",
			addressGroupRef.GetName(),
			addressGroupNamespace,
			err)
	}

	// 1.3 Get AddressGroupPortMapping for port information
	portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	portMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(), // Port mapping has the same name as the address group
		Namespace: addressGroupNamespace,
	}
	if err := v.Client.Get(ctx, portMappingKey, portMapping); err != nil {
		// If port mapping doesn't exist, we can't check port overlaps
		// This is not a critical error as the port mapping might be created later
		addressgroupbindinglog.Info("AddressGroupPortMapping not found, skipping port overlap check",
			"addressGroup", addressGroupRef.GetName(),
			"namespace", addressGroupNamespace)
	} else {
		// 1.4 Check for port overlaps if port mapping exists
		if err := CheckPortOverlaps(service, portMapping); err != nil {
			return nil, err
		}
	}

	// 1.5 Check cross-namespace policy rule

	// If the address group is in a different namespace than the binding/service
	if addressGroupNamespace != newBinding.GetNamespace() {
		// Check if there's a policy in the address group's namespace that allows this binding
		policyList := &netguardv1alpha1.AddressGroupBindingPolicyList{}
		if err := v.Client.List(ctx, policyList, client.InNamespace(addressGroupNamespace)); err != nil {
			return nil, fmt.Errorf("failed to list policies in namespace %s: %w", addressGroupNamespace, err)
		}

		// Look for a policy that references both the address group and service
		policyFound := false
		for _, policy := range policyList.Items {
			if policy.Spec.AddressGroupRef.GetName() == addressGroupRef.GetName() &&
				policy.Spec.ServiceRef.GetName() == newBinding.Spec.ServiceRef.GetName() &&
				ResolveNamespace(policy.Spec.ServiceRef.GetNamespace(), addressGroupNamespace) == newBinding.GetNamespace() {
				policyFound = true
				break
			}
		}

		if !policyFound {
			return nil, fmt.Errorf("cross-namespace binding not allowed: no AddressGroupBindingPolicy found in namespace %s that references both AddressGroup %s and Service %s",
				addressGroupNamespace, addressGroupRef.GetName(), newBinding.Spec.ServiceRef.GetName())
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupBinding.
func (v *AddressGroupBindingCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	binding, ok := obj.(*netguardv1alpha1.AddressGroupBinding)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBinding object but got %T", obj)
	}

	// TEMPORARY-DEBUG-CODE: Enhanced logging for problematic resources
	if binding.Name == "dynamic-2rx8z" || binding.Name == "dynamic-7dls7" ||
		binding.Name == "dynamic-fb5qw" || binding.Name == "dynamic-g6jfj" ||
		binding.Name == "dynamic-jd2b7" || binding.Name == "dynamic-lsjlt" {

		addressgroupbindinglog.Info("TEMPORARY-DEBUG-CODE: Detailed validation for problematic AddressGroupBinding deletion",
			"name", binding.Name,
			"namespace", binding.Namespace,
			"generation", binding.Generation,
			"resourceVersion", binding.ResourceVersion,
			"deletionTimestamp", binding.DeletionTimestamp,
			"finalizers", binding.Finalizers)
	} else {
		addressgroupbindinglog.Info("Validation for AddressGroupBinding upon deletion",
			"name", binding.GetName(),
			"namespace", binding.GetNamespace(),
			"deletionTimestamp", binding.DeletionTimestamp)
	}

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
