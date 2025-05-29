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
var addressgroupbindingpolicylog = logf.Log.WithName("addressgroupbindingpolicy-resource")

// SetupAddressGroupBindingPolicyWebhookWithManager registers the webhook for AddressGroupBindingPolicy in the manager.
func SetupAddressGroupBindingPolicyWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.AddressGroupBindingPolicy{}).
		WithValidator(&AddressGroupBindingPolicyCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-addressgroupbindingpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=addressgroupbindingpolicies,verbs=create;update;delete,versions=v1alpha1,name=vaddressgroupbindingpolicy-v1alpha1.kb.io,admissionReviewVersions=v1

// AddressGroupBindingPolicyCustomValidator struct is responsible for validating the AddressGroupBindingPolicy resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type AddressGroupBindingPolicyCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &AddressGroupBindingPolicyCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupBindingPolicy.
func (v *AddressGroupBindingPolicyCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	policy, ok := obj.(*netguardv1alpha1.AddressGroupBindingPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBindingPolicy object but got %T", obj)
	}
	addressgroupbindingpolicylog.Info("Validation for AddressGroupBindingPolicy upon creation", "name", policy.GetName())

	// 1.1 Check that an AddressGroup with the same name exists in the same namespace
	addressGroupRef := policy.Spec.AddressGroupRef

	// Check that AddressGroupRef name is not empty
	if addressGroupRef.GetName() == "" {
		return nil, fmt.Errorf("AddressGroup.name cannot be empty")
	}

	// Check that AddressGroupRef kind is correct
	if addressGroupRef.GetKind() != "AddressGroup" {
		return nil, fmt.Errorf("addressGroupRef must be to an AddressGroup resource, got %s", addressGroupRef.GetKind())
	}

	// Check that AddressGroupRef apiVersion is correct
	if addressGroupRef.GetAPIVersion() != "netguard.sgroups.io/v1alpha1" {
		return nil, fmt.Errorf("addressGroupRef must be to a resource with APIVersion netguard.sgroups.io/v1alpha1, got %s", addressGroupRef.GetAPIVersion())
	}

	addressGroupNamespace := ResolveNamespace(addressGroupRef.GetNamespace(), policy.GetNamespace())

	// Validate that the policy is created in the same namespace as the address group
	if addressGroupNamespace != policy.GetNamespace() {
		return nil, fmt.Errorf("policy must be created in the same namespace as the referenced address group")
	}

	// Check if AddressGroup exists directly
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

	// For backward compatibility, also check if AddressGroupPortMapping exists
	addressGroupPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	addressGroupPortMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}
	_ = v.Client.Get(ctx, addressGroupPortMappingKey, addressGroupPortMapping)

	// 1.2 Check that onRef (ServiceRef) exists
	serviceRef := policy.Spec.ServiceRef
	if serviceRef.GetName() == "" {
		return nil, fmt.Errorf("serviceRef.name cannot be empty")
	}
	if serviceRef.GetKind() != "Service" {
		return nil, fmt.Errorf("serviceRef must be to a Service resource, got %s", serviceRef.GetKind())
	}
	if serviceRef.GetAPIVersion() != "netguard.sgroups.io/v1alpha1" {
		return nil, fmt.Errorf("serviceRef must be to a resource with APIVersion netguard.sgroups.io/v1alpha1, got %s", serviceRef.GetAPIVersion())
	}

	// Check if Service exists
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: ResolveNamespace(serviceRef.GetNamespace(), policy.GetNamespace()),
	}
	if err := v.Client.Get(ctx, serviceKey, service); err != nil {
		return nil, fmt.Errorf("service %s not found in namespace %s: %w",
			serviceRef.GetName(),
			ResolveNamespace(serviceRef.GetNamespace(), policy.GetNamespace()),
			err)
	}

	// 1.2 Check that onRef (AddressGroupRef) exists
	if addressGroupRef.GetName() == "" {
		return nil, fmt.Errorf("addressGroupRef.name cannot be empty")
	}
	if addressGroupRef.GetKind() != "AddressGroup" {
		return nil, fmt.Errorf("addressGroupRef must be to an AddressGroup resource, got %s", addressGroupRef.GetKind())
	}
	if addressGroupRef.GetAPIVersion() != "netguard.sgroups.io/v1alpha1" {
		return nil, fmt.Errorf("addressGroupRef must be to a resource with APIVersion netguard.sgroups.io/v1alpha1, got %s", addressGroupRef.GetAPIVersion())
	}

	// 1.3 Check that there's no duplicate AddressGroupBindingPolicy
	policyList := &netguardv1alpha1.AddressGroupBindingPolicyList{}
	if err := v.Client.List(ctx, policyList, client.InNamespace(policy.GetNamespace())); err != nil {
		return nil, fmt.Errorf("failed to list existing policies: %w", err)
	}

	for _, existingPolicy := range policyList.Items {
		// Skip the current policy
		if existingPolicy.GetName() == policy.GetName() && existingPolicy.GetNamespace() == policy.GetNamespace() {
			continue
		}

		// Check if there's a policy with the same ServiceRef and AddressGroupRef
		if existingPolicy.Spec.ServiceRef.GetName() == policy.Spec.ServiceRef.GetName() &&
			existingPolicy.Spec.ServiceRef.GetNamespace() == policy.Spec.ServiceRef.GetNamespace() &&
			existingPolicy.Spec.AddressGroupRef.GetName() == policy.Spec.AddressGroupRef.GetName() &&
			existingPolicy.Spec.AddressGroupRef.GetNamespace() == policy.Spec.AddressGroupRef.GetNamespace() {
			return nil, fmt.Errorf("duplicate policy found: policy %s already binds service %s to address group %s",
				existingPolicy.GetName(),
				policy.Spec.ServiceRef.GetName(),
				policy.Spec.AddressGroupRef.GetName())
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupBindingPolicy.
func (v *AddressGroupBindingPolicyCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldPolicy, ok := oldObj.(*netguardv1alpha1.AddressGroupBindingPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBindingPolicy object for oldObj but got %T", oldObj)
	}

	newPolicy, ok := newObj.(*netguardv1alpha1.AddressGroupBindingPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBindingPolicy object for newObj but got %T", newObj)
	}
	addressgroupbindingpolicylog.Info("Validation for AddressGroupBindingPolicy upon update", "name", newPolicy.GetName())

	// Skip validation for resources being deleted
	if SkipValidationForDeletion(ctx, newPolicy) {
		return nil, nil
	}

	// Check that spec hasn't changed when Ready condition is true
	if err := ValidateSpecNotChangedWhenReady(oldObj, newObj, oldPolicy.Spec, newPolicy.Spec); err != nil {
		return nil, err
	}

	// 1.2 Check that onRef (ServiceRef) exists
	serviceRef := newPolicy.Spec.ServiceRef
	service := &netguardv1alpha1.Service{}
	serviceKey := client.ObjectKey{
		Name:      serviceRef.GetName(),
		Namespace: ResolveNamespace(serviceRef.GetNamespace(), newPolicy.GetNamespace()),
	}
	if err := v.Client.Get(ctx, serviceKey, service); err != nil {
		return nil, fmt.Errorf("service %s not found: %w", serviceRef.GetName(), err)
	}

	// 1.2 Check that onRef (AddressGroupRef) exists
	addressGroupRef := newPolicy.Spec.AddressGroupRef
	addressGroupNamespace := ResolveNamespace(addressGroupRef.GetNamespace(), newPolicy.GetNamespace())

	// Validate that the policy is in the same namespace as the address group
	if addressGroupNamespace != newPolicy.GetNamespace() {
		return nil, fmt.Errorf("policy must be in the same namespace as the referenced address group")
	}

	// Check if AddressGroup exists directly
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

	// For backward compatibility, also check if AddressGroupPortMapping exists
	addressGroupPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
	addressGroupPortMappingKey := client.ObjectKey{
		Name:      addressGroupRef.GetName(),
		Namespace: addressGroupNamespace,
	}
	_ = v.Client.Get(ctx, addressGroupPortMappingKey, addressGroupPortMapping)

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupBindingPolicy.
func (v *AddressGroupBindingPolicyCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	policy, ok := obj.(*netguardv1alpha1.AddressGroupBindingPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupBindingPolicy object but got %T", obj)
	}
	addressgroupbindingpolicylog.Info("Validation for AddressGroupBindingPolicy upon deletion", "name", policy.GetName())

	// Get the address group namespace (policy is in the same namespace as the address group)
	addressGroupNamespace := policy.GetNamespace()

	// Get the service namespace from the policy
	serviceNamespace := ResolveNamespace(policy.Spec.ServiceRef.GetNamespace(), addressGroupNamespace)

	// 1.1 Check that there are no active addressGroupBindings related to this policy
	// Only list bindings in the service namespace since AddressGroupBinding is always created in the same namespace as the Service
	bindingList := &netguardv1alpha1.AddressGroupBindingList{}
	if err := v.Client.List(ctx, bindingList, client.InNamespace(serviceNamespace)); err != nil {
		return nil, fmt.Errorf("failed to list AddressGroupBindings in namespace %s: %w", serviceNamespace, err)
	}

	// Check if any binding references the same service and address group as the policy
	for _, binding := range bindingList.Items {
		// For cross-namespace bindings:
		// 1. The binding is in the service namespace
		// 2. The binding references the address group in the policy's namespace
		// 3. The binding references the service in the policy's spec
		if binding.GetNamespace() == serviceNamespace &&
			binding.Spec.ServiceRef.GetName() == policy.Spec.ServiceRef.GetName() &&
			binding.Spec.AddressGroupRef.GetName() == policy.Spec.AddressGroupRef.GetName() &&
			ResolveNamespace(binding.Spec.AddressGroupRef.GetNamespace(), binding.GetNamespace()) == addressGroupNamespace {

			return nil, fmt.Errorf("cannot delete policy while active AddressGroupBinding %s exists in namespace %s",
				binding.GetName(), binding.GetNamespace())
		}
	}

	return nil, nil
}
