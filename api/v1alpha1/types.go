package v1alpha1

// Common condition types for all resources
const (
	// ConditionReady indicates the resource has been successfully created in the external system
	ConditionReady = "Ready"
)

// TransportProtocol represents protocols for transport layer
type TransportProtocol string

const (
	ProtocolTCP TransportProtocol = "TCP"
	ProtocolUDP TransportProtocol = "UDP"
)

// PortConfig defines a port or port range configuration
type PortConfig struct {
	// Port or port range (e.g., "80", "8080-9090")
	Port string `json:"port"`

	// Description of this port configuration
	// +optional
	Description string `json:"description,omitempty"`
}

// ProtocolPorts defines ports by protocol
type ProtocolPorts struct {
	// TCP ports
	// +optional
	TCP []PortConfig `json:"TCP,omitempty"`

	// UDP ports
	// +optional
	UDP []PortConfig `json:"UDP,omitempty"`
}
