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

// AddressGroupSpec defines the desired state of AddressGroup.
type AddressGroupSpec struct {
	// Default action for the address group
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	DefaultAction RuleAction `json:"defaultAction"`

	// Whether to enable logs
	// +optional
	Logs bool `json:"logs,omitempty"`

	// Whether to enable trace
	// +optional
	Trace bool `json:"trace,omitempty"`
}

// NetworkItem defines a network associated with an AddressGroup.
type NetworkItem struct {
	// Name of the network
	Name string `json:"name"`

	// CIDR of the network
	CIDR string `json:"cidr"`
}

// NetworksSpec defines the networks associated with an AddressGroup.
type NetworksSpec struct {
	// Networks related to this address group
	Items []NetworkItem `json:"items,omitempty"`
}

// AddressGroupStatus defines the observed state of AddressGroup.
type AddressGroupStatus struct {
	// AddressGroupName is a name in external resource
	AddressGroupName string `json:"addressGroupName,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:networks

// AddressGroup is the Schema for the addressgroups API.
type AddressGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec     AddressGroupSpec   `json:"spec,omitempty"`
	Status   AddressGroupStatus `json:"status,omitempty"`
	Networks NetworksSpec       `json:"networks,omitempty"`
}

// +kubebuilder:object:root=true

// AddressGroupList contains a list of AddressGroup.
type AddressGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddressGroup{}, &AddressGroupList{})
}
