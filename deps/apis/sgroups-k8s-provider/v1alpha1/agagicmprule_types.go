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

// AgAgIcmpRuleSpec defines the desired state of AgAgIcmpRule
type AgAgIcmpRuleSpec struct {
	// Source address group reference
	AddressGroupFrom NamespacedObjectReference `json:"addressGroupFrom"`

	// Destination address group reference
	AddressGroupTo NamespacedObjectReference `json:"addressGroupTo"`

	// ICMP specification
	ICMP ICMPSpec `json:"icmp"`

	// Whether to enable logs
	// +optional
	Logs bool `json:"logs,omitempty"`

	// Whether to enable trace
	// +optional
	Trace bool `json:"trace,omitempty"`

	// Rule action
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	Action RuleAction `json:"action"`

	// Rule priority
	// +optional
	Priority *RulePrioritySpec `json:"priority,omitempty"`
}

// AgAgIcmpRuleStatus defines the observed state of AgAgIcmpRule
type AgAgIcmpRuleStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AgAgIcmpRule is the Schema for the agagicmprules API
type AgAgIcmpRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgAgIcmpRuleSpec   `json:"spec,omitempty"`
	Status AgAgIcmpRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgAgIcmpRuleList contains a list of AgAgIcmpRule
type AgAgIcmpRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgAgIcmpRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgAgIcmpRule{}, &AgAgIcmpRuleList{})
}
