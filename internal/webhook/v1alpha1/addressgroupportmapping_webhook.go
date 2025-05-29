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
)

// nolint:unused
// log is for logging in this package.
var addressgroupportmappinglog = logf.Log.WithName("addressgroupportmapping-resource")

// SetupAddressGroupPortMappingWebhookWithManager registers the webhook for AddressGroupPortMapping in the manager.
func SetupAddressGroupPortMappingWebhookWithManager(mgr ctrl.Manager) error {
	addressgroupportmappinglog.Info("setting up manager", "webhook", "AddressGroupPortMapping")
	return ctrl.NewWebhookManagedBy(mgr).For(&netguardv1alpha1.AddressGroupPortMapping{}).
		WithValidator(&AddressGroupPortMappingCustomValidator{
			Client: mgr.GetClient(),
		}).
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
	Client client.Client
}

var _ webhook.CustomValidator = &AddressGroupPortMappingCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupPortMapping.
func (v *AddressGroupPortMappingCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	portMapping, ok := obj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object but got %T", obj)
	}
	addressgroupportmappinglog.Info("Validation for AddressGroupPortMapping upon creation", "name", portMapping.GetName())

	// Check for internal port overlaps (between services in this mapping)
	if err := v.checkInternalPortOverlaps(portMapping); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupPortMapping.
func (v *AddressGroupPortMappingCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldPortMapping, ok := oldObj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object for oldObj but got %T", oldObj)
	}

	newPortMapping, ok := newObj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object for newObj but got %T", newObj)
	}
	addressgroupportmappinglog.Info("Validation for AddressGroupPortMapping upon update", "name", newPortMapping.GetName())

	// Skip validation for resources being deleted
	if !newPortMapping.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// Check that spec hasn't changed when Ready condition is true
	if err := ValidateSpecNotChangedWhenReady(oldObj, newObj, oldPortMapping.Spec, newPortMapping.Spec); err != nil {
		return nil, err
	}

	// Check for internal port overlaps
	if err := v.checkInternalPortOverlaps(newPortMapping); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AddressGroupPortMapping.
func (v *AddressGroupPortMappingCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	addressgroupportmapping, ok := obj.(*netguardv1alpha1.AddressGroupPortMapping)
	if !ok {
		return nil, fmt.Errorf("expected a AddressGroupPortMapping object but got %T", obj)
	}
	addressgroupportmappinglog.Info("Validation for AddressGroupPortMapping upon deletion", "name", addressgroupportmapping.GetName())

	// No validation needed for deletion
	return nil, nil
}

// checkInternalPortOverlaps checks for port overlaps between services in the port mapping
func (v *AddressGroupPortMappingCustomValidator) checkInternalPortOverlaps(portMapping *netguardv1alpha1.AddressGroupPortMapping) error {
	// Create maps to store port ranges by protocol
	tcpRanges := make(map[string][]PortRange)
	udpRanges := make(map[string][]PortRange)

	// Check each service in the port mapping
	for _, servicePortRef := range portMapping.AccessPorts.Items {
		serviceName := servicePortRef.GetName()

		// Check for duplicate TCP ports within the same service
		if err := ValidateNoDuplicatePortsInPortConfig(servicePortRef.Ports.TCP); err != nil {
			return fmt.Errorf("service %s: %w", serviceName, err)
		}

		// Check for duplicate UDP ports within the same service
		if err := ValidateNoDuplicatePortsInPortConfig(servicePortRef.Ports.UDP); err != nil {
			return fmt.Errorf("service %s: %w", serviceName, err)
		}

		// Check TCP ports
		for _, tcpPort := range servicePortRef.Ports.TCP {
			portRange, err := ParsePortRange(tcpPort.Port)
			if err != nil {
				return fmt.Errorf("invalid TCP port %s for service %s: %w",
					tcpPort.Port, serviceName, err)
			}

			// Check for overlaps with other services' TCP ports
			for otherService, ranges := range tcpRanges {
				if otherService == serviceName {
					continue // Skip checking against the same service
				}

				for _, existingRange := range ranges {
					if DoPortRangesOverlap(portRange, existingRange) {
						return fmt.Errorf("TCP port range %s for service %s overlaps with existing port range %d-%d for service %s",
							tcpPort.Port, serviceName, existingRange.Start, existingRange.End, otherService)
					}
				}
			}

			// Add this port range to the map
			tcpRanges[serviceName] = append(tcpRanges[serviceName], portRange)
		}

		// Check UDP ports
		for _, udpPort := range servicePortRef.Ports.UDP {
			portRange, err := ParsePortRange(udpPort.Port)
			if err != nil {
				return fmt.Errorf("invalid UDP port %s for service %s: %w",
					udpPort.Port, serviceName, err)
			}

			// Check for overlaps with other services' UDP ports
			for otherService, ranges := range udpRanges {
				if otherService == serviceName {
					continue // Skip checking against the same service
				}

				for _, existingRange := range ranges {
					if DoPortRangesOverlap(portRange, existingRange) {
						return fmt.Errorf("UDP port range %s for service %s overlaps with existing port range %d-%d for service %s",
							udpPort.Port, serviceName, existingRange.Start, existingRange.End, otherService)
					}
				}
			}

			// Add this port range to the map
			udpRanges[serviceName] = append(udpRanges[serviceName], portRange)
		}
	}

	return nil
}
