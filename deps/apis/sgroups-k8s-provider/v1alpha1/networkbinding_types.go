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

// NetworkBindingSpec defines the desired state of NetworkBinding.
type NetworkBindingSpec struct {
	// NetworkRef is a reference to the Network resource
	NetworkRef ObjectReference `json:"networkRef"`

	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef ObjectReference `json:"addressGroupRef"`
}

// NetworkBindingStatus defines the observed state of NetworkBinding.
type NetworkBindingStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkBinding is the Schema for the networkbindings API.
type NetworkBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkBindingSpec   `json:"spec,omitempty"`
	Status NetworkBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkBindingList contains a list of NetworkBinding.
type NetworkBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkBinding{}, &NetworkBindingList{})
}
