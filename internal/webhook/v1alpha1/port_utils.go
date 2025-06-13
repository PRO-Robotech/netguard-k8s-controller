package v1alpha1

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// PortRange represents a range of ports
type PortRange struct {
	Start int
	End   int
}

// ParsePortRange converts a string port representation to a PortRange
func ParsePortRange(port string) (PortRange, error) {
	if port == "" {
		return PortRange{}, fmt.Errorf("port cannot be empty")
	}

	// Check if it's a port range (format: "start-end")
	if strings.Contains(port, "-") && !strings.HasPrefix(port, "-") {
		parts := strings.Split(port, "-")
		if len(parts) != 2 {
			return PortRange{}, fmt.Errorf("invalid port range format '%s', expected format is 'start-end'", port)
		}

		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return PortRange{}, fmt.Errorf("invalid start port '%s': must be a number between 0 and 65535", parts[0])
		}

		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return PortRange{}, fmt.Errorf("invalid end port '%s': must be a number between 0 and 65535", parts[1])
		}

		if start < 0 || start > 65535 {
			return PortRange{}, fmt.Errorf("start port %d is out of valid range (0-65535)", start)
		}

		if end < 0 || end > 65535 {
			return PortRange{}, fmt.Errorf("end port %d is out of valid range (0-65535)", end)
		}

		if start > end {
			return PortRange{}, fmt.Errorf("start port %d cannot be greater than end port %d", start, end)
		}

		return PortRange{Start: start, End: end}, nil
	}

	// Single port
	p, err := strconv.Atoi(port)
	if err != nil {
		return PortRange{}, fmt.Errorf("invalid port '%s': must be a number between 0 and 65535", port)
	}

	if p < 0 || p > 65535 {
		return PortRange{}, fmt.Errorf("port %d is out of valid range (0-65535)", p)
	}

	return PortRange{Start: p, End: p}, nil
}

// DoPortRangesOverlap checks if two port ranges overlap
func DoPortRangesOverlap(a, b PortRange) bool {
	// Ranges overlap if the start of one is less than or equal to the end of the other
	// and the end of one is greater than or equal to the start of the other
	return a.Start <= b.End && a.End >= b.Start
}

// CheckPortOverlaps checks if there are any port overlaps between a service and existing services in a port mapping
func CheckPortOverlaps(
	service *netguardv1alpha1.Service,
	portMapping *netguardv1alpha1.AddressGroupPortMapping,
) error {
	// Create a map of service ports by protocol
	servicePorts := make(map[netguardv1alpha1.TransportProtocol][]PortRange)
	for _, ingressPort := range service.Spec.IngressPorts {
		portRange, err := ParsePortRange(ingressPort.Port)
		if err != nil {
			return fmt.Errorf("invalid port in service %s: %w", service.GetName(), err)
		}
		servicePorts[ingressPort.Protocol] = append(servicePorts[ingressPort.Protocol], portRange)
	}

	// Check for overlaps with existing services in the port mapping
	for _, servicePortRef := range portMapping.AccessPorts.Items {
		// Skip the current service (for updates)
		if servicePortRef.GetName() == service.GetName() {
			continue
		}

		// Check TCP ports
		for _, tcpPort := range servicePortRef.Ports.TCP {
			portRange, err := ParsePortRange(tcpPort.Port)
			if err != nil {
				return fmt.Errorf("invalid TCP port in portMapping: %w", err)
			}

			// Check for overlaps with service TCP ports
			for _, serviceRange := range servicePorts[netguardv1alpha1.ProtocolTCP] {
				if DoPortRangesOverlap(portRange, serviceRange) {
					return fmt.Errorf("TCP port range %s in service %s overlaps with existing port range %d-%d in service %s",
						tcpPort.Port, service.GetName(), portRange.Start, portRange.End, servicePortRef.GetName())
				}
			}
		}

		// Check UDP ports
		for _, udpPort := range servicePortRef.Ports.UDP {
			portRange, err := ParsePortRange(udpPort.Port)
			if err != nil {
				return fmt.Errorf("invalid UDP port in portMapping: %w", err)
			}

			// Check for overlaps with service UDP ports
			for _, serviceRange := range servicePorts[netguardv1alpha1.ProtocolUDP] {
				if DoPortRangesOverlap(portRange, serviceRange) {
					return fmt.Errorf("UDP port range %s in service %s overlaps with existing port range %d-%d in service %s",
						udpPort.Port, service.GetName(), portRange.Start, portRange.End, servicePortRef.GetName())
				}
			}
		}
	}

	return nil
}

// CheckPortOverlapsOptimized checks if there are any port overlaps using a more efficient algorithm
func CheckPortOverlapsOptimized(
	service *netguardv1alpha1.Service,
	portMapping *netguardv1alpha1.AddressGroupPortMapping,
) error {
	// Collect all port ranges by protocol
	tcpRanges := []PortRange{}
	udpRanges := []PortRange{}

	// Get port ranges from the service
	for _, ingressPort := range service.Spec.IngressPorts {
		portRange, err := ParsePortRange(ingressPort.Port)
		if err != nil {
			return fmt.Errorf("invalid port in service %s: %w", service.GetName(), err)
		}

		// Add service name to identify the source in error messages
		if ingressPort.Protocol == netguardv1alpha1.ProtocolTCP {
			tcpRanges = append(tcpRanges, portRange)
		} else if ingressPort.Protocol == netguardv1alpha1.ProtocolUDP {
			udpRanges = append(udpRanges, portRange)
		}
	}

	// Get port ranges from other services in the port mapping
	for _, servicePortRef := range portMapping.AccessPorts.Items {
		// Skip the current service (for updates)
		if servicePortRef.GetName() == service.GetName() {
			continue
		}

		// Get TCP port ranges
		for _, tcpPort := range servicePortRef.Ports.TCP {
			portRange, err := ParsePortRange(tcpPort.Port)
			if err != nil {
				return fmt.Errorf("invalid TCP port configuration '%s' in service '%s': %w",
					tcpPort.Port, servicePortRef.GetName(), err)
			}
			tcpRanges = append(tcpRanges, portRange)
		}

		// Get UDP port ranges
		for _, udpPort := range servicePortRef.Ports.UDP {
			portRange, err := ParsePortRange(udpPort.Port)
			if err != nil {
				return fmt.Errorf("invalid UDP port configuration '%s' in service '%s': %w",
					udpPort.Port, servicePortRef.GetName(), err)
			}
			udpRanges = append(udpRanges, portRange)
		}
	}

	// Check for TCP port overlaps
	if err := checkPortRangeOverlaps(tcpRanges, "TCP"); err != nil {
		return fmt.Errorf("port conflict in address group '%s': %w", portMapping.GetName(), err)
	}

	// Check for UDP port overlaps
	if err := checkPortRangeOverlaps(udpRanges, "UDP"); err != nil {
		return fmt.Errorf("port conflict in address group '%s': %w", portMapping.GetName(), err)
	}

	return nil
}

// checkPortRangeOverlaps checks for overlaps in a list of port ranges using sorting
func checkPortRangeOverlaps(ranges []PortRange, protocol string) error {
	if len(ranges) <= 1 {
		return nil
	}

	// Sort port ranges by start port
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start < ranges[j].Start
	})

	// Check for overlaps between adjacent ranges
	for i := 0; i < len(ranges)-1; i++ {
		if ranges[i].End >= ranges[i+1].Start {
			return fmt.Errorf("port conflict detected: %s port ranges %d-%d and %d-%d overlap. "+
				"Services in the same address group cannot have overlapping ports for the same protocol.",
				protocol, ranges[i].Start, ranges[i].End, ranges[i+1].Start, ranges[i+1].End)
		}
	}

	return nil
}

// ConvertIngressPortsToProtocolPorts converts IngressPorts to ProtocolPorts
func ConvertIngressPortsToProtocolPorts(ingressPorts []netguardv1alpha1.IngressPort) netguardv1alpha1.ProtocolPorts {
	result := netguardv1alpha1.ProtocolPorts{
		TCP: []netguardv1alpha1.PortConfig{},
		UDP: []netguardv1alpha1.PortConfig{},
	}

	for _, ingressPort := range ingressPorts {
		portConfig := netguardv1alpha1.PortConfig{
			Port:        ingressPort.Port,
			Description: ingressPort.Description,
		}

		if ingressPort.Protocol == netguardv1alpha1.ProtocolTCP {
			result.TCP = append(result.TCP, portConfig)
		} else if ingressPort.Protocol == netguardv1alpha1.ProtocolUDP {
			result.UDP = append(result.UDP, portConfig)
		}
	}

	return result
}
