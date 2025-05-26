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
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.ServiceAlias{}).
		WithValidator(&ServiceAliasCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-servicealias,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=servicealias,verbs=create;update,versions=v1alpha1,name=vservicealias-v1alpha1.kb.io,admissionReviewVersions=v1

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

	// Check that spec hasn't changed (should be immutable)
	if !reflect.DeepEqual(oldServiceAlias.Spec, newServiceAlias.Spec) {
		return nil, fmt.Errorf("spec of ServiceAlias cannot be changed")
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ServiceAlias.
func (v *ServiceAliasCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	servicealias, ok := obj.(*netguardv1alpha1.ServiceAlias)
	if !ok {
		return nil, fmt.Errorf("expected a ServiceAlias object but got %T", obj)
	}
	servicealiaslog.Info("Validation for ServiceAlias upon deletion", "name", servicealias.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
