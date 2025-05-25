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

// NetworkSpec defines the desired state of Network.
type NetworkSpec struct {
	// CIDR notation for IP range, e.g., "192.168.0.0/16"
	CIDR string `json:"cidr"`
}

// NetworkStatus defines the observed state of Network.
type NetworkStatus struct {
	// NetworkName is a name in external resource
	NetworkName string `json:"networkName,omitempty"`

	// IsBound indicates whether this network is bound to an address group
	IsBound bool `json:"isBound"`

	// BindingRef is a reference to the SGBinding that binds this network to an address group
	BindingRef *ObjectReference `json:"bindingRef,omitempty"`

	// AddressGroupRef is a reference to the AddressGroup this network is bound to
	AddressGroupRef *ObjectReference `json:"addressGroupRef,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Network is the Schema for the networks API.
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec,omitempty"`
	Status NetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkList contains a list of Network.
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Network `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Network{}, &NetworkList{})
}
