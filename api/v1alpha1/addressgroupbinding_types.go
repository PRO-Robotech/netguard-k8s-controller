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

// AddressGroupBindingSpec defines the desired state of AddressGroupBinding.
type AddressGroupBindingSpec struct {
	// ServiceRef is a reference to the Service resource
	ServiceRef ObjectReference `json:"serviceRef"`

	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef NamespacedObjectReference `json:"addressGroupRef"`
}

// AddressGroupBindingStatus defines the observed state of AddressGroupBinding.
type AddressGroupBindingStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AddressGroupBinding is the Schema for the addressgroupbindings API.
type AddressGroupBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddressGroupBindingSpec   `json:"spec,omitempty"`
	Status AddressGroupBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddressGroupBindingList contains a list of AddressGroupBinding.
type AddressGroupBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroupBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddressGroupBinding{}, &AddressGroupBindingList{})
}
