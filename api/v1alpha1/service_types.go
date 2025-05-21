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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceSpec defines the desired state of Service.
type ServiceSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	// IngressPorts defines the ports that are allowed for ingress traffic
	// +optional
	IngressPorts []IngressPort `json:"ingressPorts,omitempty"`
}

// AddressGroupsSpec defines the address groups associated with a Service.
type AddressGroupsSpec struct {
	// Items contains the list of address groups
	Items []NamespacedObjectReference `json:"items,omitempty"`
}

// IngressPort defines a port configuration for ingress traffic
type IngressPort struct {
	// Transport protocol for the rule
	// +kubebuilder:validation:Enum=TCP;UDP
	Protocol TransportProtocol `json:"protocol"`

	// Port or port range (e.g., "80", "8080-9090")
	Port string `json:"port"`

	// Description of this port configuration
	// +optional
	Description string `json:"description,omitempty"`
}

// ServiceStatus defines the observed state of Service.
type ServiceStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:addressGroups

// Service is the Schema for the services API.
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec          ServiceSpec       `json:"spec,omitempty"`
	Status        ServiceStatus     `json:"status,omitempty"`
	AddressGroups AddressGroupsSpec `json:"addressGroups,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceList contains a list of Service.
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
