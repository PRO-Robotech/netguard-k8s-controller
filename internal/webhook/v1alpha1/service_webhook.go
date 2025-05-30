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
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var servicelog = logf.Log.WithName("service-resource")

// SetupServiceWebhookWithManager registers the webhook for Service in the manager.
func SetupServiceWebhookWithManager(mgr ctrl.Manager) error {
	servicelog.Info("setting up manager", "webhook", "Service")
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.Service{}).
		WithValidator(&ServiceCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-service,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=services,verbs=create;update;delete,versions=v1alpha1,name=vservice-v1alpha1.kb.io,admissionReviewVersions=v1

// ServiceCustomValidator struct is responsible for validating the Service resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ServiceCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &ServiceCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Service.
func (v *ServiceCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	service, ok := obj.(*netguardv1alpha1.Service)
	if !ok {
		return nil, fmt.Errorf("expected a Service object but got %T", obj)
	}
	servicelog.Info("Validation for Service upon creation", "name", service.GetName())

	// Check for duplicate ports
	if err := ValidateNoDuplicatePorts(service.Spec.IngressPorts); err != nil {
		return nil, err
	}

	// Validate all ports in the service
	for _, ingressPort := range service.Spec.IngressPorts {
		if err := ValidatePorts(ingressPort); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Service.
func (v *ServiceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldService, ok := oldObj.(*netguardv1alpha1.Service)
	if !ok {
		return nil, fmt.Errorf("expected a Service object for oldObj but got %T", oldObj)
	}

	newService, ok := newObj.(*netguardv1alpha1.Service)
	if !ok {
		return nil, fmt.Errorf("expected a Service object for newObj but got %T", newObj)
	}
	servicelog.Info("Validation for Service upon update", "name", newService.GetName())

	// Skip validation for resources being deleted
	if SkipValidationForDeletion(ctx, newService) {
		return nil, nil
	}

	// Check for duplicate ports
	if err := ValidateNoDuplicatePorts(newService.Spec.IngressPorts); err != nil {
		return nil, err
	}

	// Validate all ports in the service
	for _, ingressPort := range newService.Spec.IngressPorts {
		if err := ValidatePorts(ingressPort); err != nil {
			return nil, err
		}
	}

	// Check if ports have been added or modified
	if len(newService.Spec.IngressPorts) > len(oldService.Spec.IngressPorts) || !reflect.DeepEqual(oldService.Spec.IngressPorts, newService.Spec.IngressPorts) {
		// Check if the service is bound to any AddressGroups
		if len(newService.AddressGroups.Items) > 0 {
			// For each AddressGroup, check for port overlaps
			for _, addressGroupRef := range newService.AddressGroups.Items {
				// Get the AddressGroupPortMapping
				portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
				portMappingKey := client.ObjectKey{
					Name:      addressGroupRef.GetName(),
					Namespace: ResolveNamespace(addressGroupRef.GetNamespace(), newService.GetNamespace()),
				}
				if err := v.Client.Get(ctx, portMappingKey, portMapping); err != nil {
					return nil, fmt.Errorf("addressGroupPortMapping for addressGroup %s not found: %w", addressGroupRef.GetName(), err)
				}

				// Create a temporary copy of portMapping for validation
				tempPortMapping := portMapping.DeepCopy()

				// Remove the current service from the temporary copy
				for i := 0; i < len(tempPortMapping.AccessPorts.Items); i++ {
					if tempPortMapping.AccessPorts.Items[i].GetName() == newService.GetName() &&
						tempPortMapping.AccessPorts.Items[i].GetNamespace() == newService.GetNamespace() {
						tempPortMapping.AccessPorts.Items = append(
							tempPortMapping.AccessPorts.Items[:i],
							tempPortMapping.AccessPorts.Items[i+1:]...)
						break
					}
				}

				// Check for port overlaps with the updated service
				if err := CheckPortOverlaps(newService, tempPortMapping); err != nil {
					return nil, err
				}
			}
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Service.
func (v *ServiceCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	service, ok := obj.(*netguardv1alpha1.Service)
	if !ok {
		return nil, fmt.Errorf("expected a Service object but got %T", obj)
	}
	servicelog.Info("Validation for Service upon deletion", "name", service.GetName())

	// 1. Get all ServiceAlias resources in the same namespace
	serviceAliasList := &netguardv1alpha1.ServiceAliasList{}
	if err := v.Client.List(ctx, serviceAliasList, client.InNamespace(service.GetNamespace())); err != nil {
		servicelog.Error(err, "Failed to list ServiceAlias objects")
		return nil, fmt.Errorf("failed to list ServiceAlias objects: %w", err)
	}

	// 2. Filter ServiceAlias resources that have this Service as owner
	var ownedAliases []netguardv1alpha1.ServiceAlias
	for _, alias := range serviceAliasList.Items {
		for _, ownerRef := range alias.GetOwnerReferences() {
			if ownerRef.UID == service.GetUID() {
				ownedAliases = append(ownedAliases, alias)
				break
			}
		}
	}

	// If no aliases are owned by this service, allow deletion
	if len(ownedAliases) == 0 {
		return nil, nil
	}

	// 3. Check if any of the owned aliases are used in active RuleS2S resources
	ruleS2SList := &netguardv1alpha1.RuleS2SList{}
	if err := v.Client.List(ctx, ruleS2SList); err != nil {
		servicelog.Error(err, "Failed to list RuleS2S objects")
		return nil, fmt.Errorf("failed to list RuleS2S objects: %w", err)
	}

	// Map to store aliases referenced by rules
	aliasesWithRules := make(map[string][]string)

	// Check each rule for references to any of our owned aliases
	for _, rule := range ruleS2SList.Items {
		for _, alias := range ownedAliases {
			// Check if the rule references this alias as local service
			if rule.Spec.ServiceLocalRef.Name == alias.Name &&
				rule.Namespace == alias.Namespace {
				aliasesWithRules[alias.Name] = append(aliasesWithRules[alias.Name],
					fmt.Sprintf("RuleS2S '%s' in namespace '%s' (as local service)", rule.Name, rule.Namespace))
			}

			// Check if the rule references this alias as target service
			targetNamespace := rule.Spec.ServiceRef.ResolveNamespace(rule.Namespace)
			if rule.Spec.ServiceRef.Name == alias.Name &&
				targetNamespace == alias.Namespace {
				aliasesWithRules[alias.Name] = append(aliasesWithRules[alias.Name],
					fmt.Sprintf("RuleS2S '%s' in namespace '%s' (as target service)", rule.Name, rule.Namespace))
			}
		}
	}

	// If there are any aliases with rules, prevent deletion
	if len(aliasesWithRules) > 0 {
		var errorMsgs []string
		errorMsgs = append(errorMsgs, fmt.Sprintf("Cannot delete Service '%s' because it has ServiceAliases that are referenced by active RuleS2S resources:", service.Name))

		for aliasName, rules := range aliasesWithRules {
			errorMsgs = append(errorMsgs, fmt.Sprintf("  ServiceAlias '%s' is referenced by:", aliasName))
			for _, rule := range rules {
				errorMsgs = append(errorMsgs, fmt.Sprintf("    - %s", rule))
			}
		}

		errorMsg := strings.Join(errorMsgs, "\n")
		servicelog.Info("Preventing Service deletion due to active RuleS2S references",
			"service", service.Name, "errorMsg", errorMsg)
		return nil, fmt.Errorf(errorMsg)
	}

	return nil, nil
}
