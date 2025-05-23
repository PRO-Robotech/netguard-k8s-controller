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

// RuleS2SSpec defines the desired state of RuleS2S.
type RuleS2SSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of RuleS2S. Edit rules2s_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// RuleS2SStatus defines the observed state of RuleS2S.
type RuleS2SStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RuleS2S is the Schema for the rules2s API.
type RuleS2S struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuleS2SSpec   `json:"spec,omitempty"`
	Status RuleS2SStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RuleS2SList contains a list of RuleS2S.
type RuleS2SList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuleS2S `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RuleS2S{}, &RuleS2SList{})
}
