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

// IECidrAgRuleSpec defines the desired state of IECidrAgRule.
type IECidrAgRuleSpec struct {
	// Transport protocol for the rule
	// +kubebuilder:validation:Enum=TCP;UDP
	Transport TransportProtocol `json:"transport"`

	// CIDR notation for IP range, e.g., "192.168.0.0/16"
	CIDR string `json:"cidr"`

	// Reference to the address group
	AddressGroup NamespacedObjectReference `json:"addressGroup"`

	// Direction of traffic flow
	// +kubebuilder:validation:Enum=INGRESS;EGRESS
	Traffic TrafficDirection `json:"traffic"`

	// List of source and destination ports
	Ports []AccPorts `json:"ports"`

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

// IECidrAgRuleStatus defines the observed state of IECidrAgRule.
type IECidrAgRuleStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IECidrAgRule is the Schema for the iecidragrules API.
type IECidrAgRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IECidrAgRuleSpec   `json:"spec,omitempty"`
	Status IECidrAgRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IECidrAgRuleList contains a list of IECidrAgRule.
type IECidrAgRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IECidrAgRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IECidrAgRule{}, &IECidrAgRuleList{})
}
