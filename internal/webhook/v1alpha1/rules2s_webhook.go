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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var rules2slog = logf.Log.WithName("rules2s-resource")

// SetupRuleS2SWebhookWithManager registers the webhook for RuleS2S in the manager.
func SetupRuleS2SWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.RuleS2S{}).
		WithValidator(&RuleS2SCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-rules2s,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=rules2s,verbs=create;update,versions=v1alpha1,name=vrules2s-v1alpha1.kb.io,admissionReviewVersions=v1

// RuleS2SCustomValidator struct is responsible for validating the RuleS2S resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type RuleS2SCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &RuleS2SCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type RuleS2S.
func (v *RuleS2SCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	rule, ok := obj.(*netguardv1alpha1.RuleS2S)
	if !ok {
		return nil, fmt.Errorf("expected a RuleS2S object but got %T", obj)
	}
	rules2slog.Info("Validation for RuleS2S upon creation", "name", rule.GetName())

	// Validate that serviceLocalRef exists
	localServiceAlias := &netguardv1alpha1.ServiceAlias{}
	localServiceNamespace := rule.Namespace
	localServiceName := rule.Spec.ServiceLocalRef.Name

	if err := v.Client.Get(ctx, types.NamespacedName{
		Namespace: localServiceNamespace,
		Name:      localServiceName,
	}, localServiceAlias); err != nil {
		return nil, fmt.Errorf("serviceLocalRef %s/%s does not exist: %w",
			localServiceNamespace, localServiceName, err)
	}

	// Check if local ServiceAlias is Ready
	if err := ValidateReferencedObjectIsReady(localServiceAlias, localServiceName, "ServiceAlias"); err != nil {
		return nil, err
	}

	// Validate that serviceRef exists
	targetServiceAlias := &netguardv1alpha1.ServiceAlias{}
	targetServiceNamespace := rule.Spec.ServiceRef.ResolveNamespace(rule.Namespace)
	targetServiceName := rule.Spec.ServiceRef.Name

	if err := v.Client.Get(ctx, types.NamespacedName{
		Namespace: targetServiceNamespace,
		Name:      targetServiceName,
	}, targetServiceAlias); err != nil {
		return nil, fmt.Errorf("serviceRef %s/%s does not exist: %w",
			targetServiceNamespace, targetServiceName, err)
	}

	// Check if target ServiceAlias is Ready
	if err := ValidateReferencedObjectIsReady(targetServiceAlias, targetServiceName, "ServiceAlias"); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type RuleS2S.
func (v *RuleS2SCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldRule, ok := oldObj.(*netguardv1alpha1.RuleS2S)
	if !ok {
		return nil, fmt.Errorf("expected a RuleS2S object for oldObj but got %T", oldObj)
	}

	newRule, ok := newObj.(*netguardv1alpha1.RuleS2S)
	if !ok {
		return nil, fmt.Errorf("expected a RuleS2S object for newObj but got %T", newObj)
	}
	rules2slog.Info("Validation for RuleS2S upon update", "name", newRule.GetName())

	// Skip validation for resources being deleted
	if !newRule.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// Check that spec hasn't changed when Ready condition is true
	if err := ValidateSpecNotChangedWhenReady(oldObj, newObj, oldRule.Spec, newRule.Spec); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type RuleS2S.
func (v *RuleS2SCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	rules2s, ok := obj.(*netguardv1alpha1.RuleS2S)
	if !ok {
		return nil, fmt.Errorf("expected a RuleS2S object but got %T", obj)
	}
	rules2slog.Info("Validation for RuleS2S upon deletion", "name", rules2s.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
