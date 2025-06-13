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
var servicealiaslog = logf.Log.WithName("servicealias-resource")

// SetupServiceAliasWebhookWithManager registers the webhook for ServiceAlias in the manager.
func SetupServiceAliasWebhookWithManager(mgr ctrl.Manager) error {
	servicealiaslog.Info("setting up manager", "webhook", "ServiceAlias")
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.ServiceAlias{}).
		WithValidator(&ServiceAliasCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-servicealias,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=servicealias,verbs=create;update;delete,versions=v1alpha1,name=vservicealias-v1alpha1.kb.io,admissionReviewVersions=v1

// ServiceAliasCustomValidator struct is responsible for validating the ServiceAlias resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ServiceAliasCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &ServiceAliasCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ServiceAlias.
func (v *ServiceAliasCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceAlias, ok := obj.(*netguardv1alpha1.ServiceAlias)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceAlias object but got %T", obj)
	}
	servicealiaslog.Info("Validation for ServiceAlias upon creation", "name", serviceAlias.GetName())

	// Validate that the referenced Service exists
	service := &netguardv1alpha1.Service{}
	err := v.Client.Get(ctx, client.ObjectKey{
		Name:      serviceAlias.Spec.ServiceRef.GetName(),
		Namespace: serviceAlias.GetNamespace(), // ServiceAlias can only reference Service in the same namespace
	}, service)

	if err != nil {
		return nil, fmt.Errorf("referenced Service does not exist: %w", err)
	}

	// Check if Service is Ready
	if err := ValidateReferencedObjectIsReady(service, serviceAlias.Spec.ServiceRef.GetName(), "Service"); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ServiceAlias.
func (v *ServiceAliasCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldServiceAlias, ok := oldObj.(*netguardv1alpha1.ServiceAlias)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceAlias object for oldObj but got %T", oldObj)
	}

	newServiceAlias, ok := newObj.(*netguardv1alpha1.ServiceAlias)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceAlias object for newObj but got %T", newObj)
	}
	servicealiaslog.Info("Validation for ServiceAlias upon update", "name", newServiceAlias.GetName())

	// Skip validation for resources being deleted
	if !newServiceAlias.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// Check that spec hasn't changed when Ready condition is true
	if err := ValidateSpecNotChangedWhenReady(oldObj, newObj, oldServiceAlias.Spec, newServiceAlias.Spec); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ServiceAlias.
func (v *ServiceAliasCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	serviceAlias, ok := obj.(*netguardv1alpha1.ServiceAlias)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceAlias object but got %T", obj)
	}
	servicealiaslog.Info("Validation for ServiceAlias upon deletion", "name", serviceAlias.GetName())

	// Check if there are any RuleS2S resources that reference this ServiceAlias
	ruleS2SList := &netguardv1alpha1.RuleS2SList{}
	if err := v.Client.List(ctx, ruleS2SList); err != nil {
		servicealiaslog.Error(err, "Failed to list RuleS2S objects")
		return nil, fmt.Errorf("failed to list RuleS2S objects: %w", err)
	}

	// Collect references by type
	localServiceRefs := make(map[string]struct{})
	targetServiceRefs := make(map[string]struct{})

	// Check if any rules reference this ServiceAlias
	for _, rule := range ruleS2SList.Items {
		// Check if the rule references this ServiceAlias as local service
		if rule.Spec.ServiceLocalRef.Name == serviceAlias.Name &&
			rule.Namespace == serviceAlias.Namespace {
			servicealiaslog.Info("ServiceAlias is referenced by RuleS2S as local service",
				"serviceAlias", serviceAlias.Name, "rule", rule.Name)
			localServiceRefs[rule.Name] = struct{}{}
		}

		// Check if the rule references this ServiceAlias as target service
		targetNamespace := rule.Spec.ServiceRef.ResolveNamespace(rule.Namespace)
		if rule.Spec.ServiceRef.Name == serviceAlias.Name &&
			targetNamespace == serviceAlias.Namespace {
			servicealiaslog.Info("ServiceAlias is referenced by RuleS2S as target service",
				"serviceAlias", serviceAlias.Name, "rule", rule.Name)
			targetServiceRefs[rule.Name] = struct{}{}
		}
	}

	// If there are any references, return a detailed error message
	if len(localServiceRefs) > 0 || len(targetServiceRefs) > 0 {
		var errorMsg string

		// Format local service references
		if len(localServiceRefs) > 0 {
			var localRulesWithNamespace []string
			for ruleName := range localServiceRefs {
				// Find the rule to get its namespace
				for _, rule := range ruleS2SList.Items {
					if rule.Name == ruleName {
						localRulesWithNamespace = append(localRulesWithNamespace,
							fmt.Sprintf("%s in namespace %s", ruleName, rule.Namespace))
						break
					}
				}
			}
			errorMsg += fmt.Sprintf("Cannot delete ServiceAlias %s: it is referenced by RuleS2S as local service:\n%s",
				serviceAlias.Name, strings.Join(localRulesWithNamespace, "\n"))
		}

		// Format target service references
		if len(targetServiceRefs) > 0 {
			if errorMsg != "" {
				errorMsg += "\n\n"
			}
			var targetRulesWithNamespace []string
			for ruleName := range targetServiceRefs {
				// Find the rule to get its namespace
				for _, rule := range ruleS2SList.Items {
					if rule.Name == ruleName {
						targetRulesWithNamespace = append(targetRulesWithNamespace,
							fmt.Sprintf("%s in namespace %s", ruleName, rule.Namespace))
						break
					}
				}
			}
			errorMsg += fmt.Sprintf("Cannot delete ServiceAlias %s: it is referenced by RuleS2S as target service:\n%s",
				serviceAlias.Name, strings.Join(targetRulesWithNamespace, "\n"))
		}

		servicealiaslog.Info("Cannot delete ServiceAlias: it is referenced by RuleS2S",
			"serviceAlias", serviceAlias.Name, "errorMsg", errorMsg)
		return nil, fmt.Errorf(errorMsg)
	}

	return nil, nil
}
