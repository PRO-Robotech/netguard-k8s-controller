package v1alpha1

import (
	"fmt"
	"strconv"
	"strings"
)

// RuleAction represents allowed actions for SgSgIcmpRule
type RuleAction string

const (
	ActionAccept RuleAction = "ACCEPT"
	ActionDrop   RuleAction = "DROP"
)

type IpAddrFamily string

const (
	IPv4 IpAddrFamily = "IPv4"
	IPv6 IpAddrFamily = "IPv6"
)

// TransportProtocol represents protocols for transport layer
type TransportProtocol string

const (
	ProtocolTCP TransportProtocol = "TCP"
	ProtocolUDP TransportProtocol = "UDP"
)

// TrafficDirection represents direction of traffic flow
type TrafficDirection string

const (
	TrafficIngress TrafficDirection = "INGRESS"
	TrafficEgress  TrafficDirection = "EGRESS"
)

// ICMPSpec defines the ICMP protocol specification
type ICMPSpec struct {
	// IPv4 or IPv6
	// +kubebuilder:validation:Enum=IPv4;IPv6
	IPv IpAddrFamily `json:"ipv"`

	// ICMP type
	Type []uint32 `json:"type"`
}

// RulePrioritySpec defines the priority of the rule
type RulePrioritySpec struct {
	// Value of the priority
	// +optional
	Value int32 `json:"value,omitempty"`
}

// AccPorts defines source and destination ports
type AccPorts struct {
	// Source port or port range
	// +optional
	S string `json:"s,omitempty"`

	// Destination port or port range
	D string `json:"d"`
}

// validatePort validates a port string, which can be a single port, a port range, or a comma-separated list of ports/ranges
func validatePort(port string) error {
	// Allow empty port string
	if port == "" {
		return nil
	}

	// Split by comma to handle comma-separated list
	portItems := strings.Split(port, ",")
	for _, item := range portItems {
		item = strings.TrimSpace(item)

		// Check if it's a port range (format: "start-end")
		if strings.Contains(item, "-") && !strings.HasPrefix(item, "-") {
			parts := strings.Split(item, "-")
			if len(parts) != 2 {
				return fmt.Errorf("invalid port range format")
			}

			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return fmt.Errorf("invalid start port")
			}

			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return fmt.Errorf("invalid end port")
			}

			if start < 0 || start > 65535 {
				return fmt.Errorf("start port must be between 0 and 65535")
			}

			if end < 0 || end > 65535 {
				return fmt.Errorf("end port must be between 0 and 65535")
			}

			if start > end {
				return fmt.Errorf("start port must be less than or equal to end port")
			}
		} else {
			// Check if it's a single port
			p, err := strconv.Atoi(item)
			if err != nil {
				return fmt.Errorf("invalid port")
			}

			if p < 0 {
				return fmt.Errorf("invalid port")
			}
			if p > 65535 {
				return fmt.Errorf("port must be between 0 and 65535")
			}
		}
	}

	return nil
}

// ObjectReference contains enough information to let you locate the referenced object
type ObjectReference struct {
	// APIVersion of the referenced object
	APIVersion string `json:"apiVersion"`

	// Kind of the referenced object
	Kind string `json:"kind"`

	// Name of the referenced object
	Name string `json:"name"`
}

// NamespacedObjectReference extends ObjectReference with a Namespace field
type NamespacedObjectReference struct {
	// Embedded ObjectReference
	ObjectReference `json:",inline"`

	// Namespace of the referenced object
	Namespace string `json:"namespace,omitempty"`
}

// Common condition types for all resources
const (
	// ConditionReady indicates the resource has been successfully created in the external system
	ConditionReady = "Ready"

	// ConditionLinked indicates the NetworkBinding has successfully linked a Network to an AddressGroup
	ConditionLinked = "Linked"
)
