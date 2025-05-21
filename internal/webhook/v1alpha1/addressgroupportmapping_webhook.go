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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var addressgroupportmappinglog = logf.Log.WithName("addressgroupportmapping-resource")

// SetupAddressGroupPortMappingWebhookWithManager registers the webhook for AddressGroupPortMapping in the manager.
func SetupAddressGroupPortMappingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.AddressGroupPortMapping{}).
		WithValidator(&AddressGroupPortMappingCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-addressgroupportmapping,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=addressgroupportmappings,verbs=create;update,versions=v1alpha1,name=vaddressgroupportmapping-v1alpha1.kb.io,admissionReviewVersions=v1

// AddressGroupPortMappingCustomValidator struct is responsible for validating the AddressGroupPortMapping resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type AddressGroupPortMappingCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &AddressGroupPortMappingCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupPortMapping.
func (v *AddressGroupPortMappingCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	addressgroupportmapping, ok := obj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object but got %T", obj)
	}
	addressgroupportmappinglog.Info("Validation for AddressGroupPortMapping upon creation", "name", addressgroupportmapping.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupPortMapping.
func (v *AddressGroupPortMappingCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	addressgroupportmapping, ok := newObj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object for the newObj but got %T", newObj)
	}
	addressgroupportmappinglog.Info("Validation for AddressGroupPortMapping upon update", "name", addressgroupportmapping.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupPortMapping.
func (v *AddressGroupPortMappingCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	addressgroupportmapping, ok := obj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object but got %T", obj)
	}
	addressgroupportmappinglog.Info("Validation for AddressGroupPortMapping upon deletion", "name", addressgroupportmapping.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
