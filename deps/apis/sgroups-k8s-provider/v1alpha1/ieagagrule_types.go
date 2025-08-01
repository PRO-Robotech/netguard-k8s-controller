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

// IEAgAgRuleSpec defines the desired state of IEAgAgRule.
type IEAgAgRuleSpec struct {
	// Transport protocol for the rule
	// +kubebuilder:validation:Enum=TCP;UDP
	Transport TransportProtocol `json:"transport"`

	// Reference to the address group
	AddressGroup NamespacedObjectReference `json:"addressGroup"`

	// Reference to the local address group
	AddressGroupLocal NamespacedObjectReference `json:"addressGroupLocal"`

	// List of source and destination ports
	Ports []AccPorts `json:"ports"`

	// Direction of traffic flow
	// +kubebuilder:validation:Enum=INGRESS;EGRESS
	Traffic TrafficDirection `json:"traffic"`

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

// IEAgAgRuleStatus defines the observed state of IEAgAgRule.
type IEAgAgRuleStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IEAgAgRule is the Schema for the ieagagrules API.
type IEAgAgRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IEAgAgRuleSpec   `json:"spec,omitempty"`
	Status IEAgAgRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IEAgAgRuleList contains a list of IEAgAgRule.
type IEAgAgRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IEAgAgRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IEAgAgRule{}, &IEAgAgRuleList{})
}
