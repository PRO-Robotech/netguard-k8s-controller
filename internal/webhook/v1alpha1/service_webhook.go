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
var servicelog = logf.Log.WithName("service-resource")

// SetupServiceWebhookWithManager registers the webhook for Service in the manager.
func SetupServiceWebhookWithManager(mgr ctrl.Manager) error {
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
// +kubebuilder:webhook:path=/validate-netguard-sgroups-io-v1alpha1-service,mutating=false,failurePolicy=fail,sideEffects=None,groups=netguard.sgroups.io,resources=services,verbs=create;update,versions=v1alpha1,name=vservice-v1alpha1.kb.io,admissionReviewVersions=v1

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

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
