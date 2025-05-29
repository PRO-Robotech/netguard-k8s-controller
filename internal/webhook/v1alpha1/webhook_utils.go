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
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ResolveNamespace resolves the namespace, using the default if not specified
func ResolveNamespace(specNamespace, defaultNamespace string) string {
	if specNamespace == "" {
		return defaultNamespace
	}
	return specNamespace
}

// ValidatePorts validates that all ports in a list are valid
func ValidatePorts(p netguardv1alpha1.IngressPort) error {
	if err := validatePort(p.Port); err != nil {
		return fmt.Errorf("invalid destination port %v: %v", p, err)
	}
	return nil
}

// ValidatePortsNotChanged validates that ports haven't changed during an update
func ValidatePortsNotChanged(oldPort, newPort netguardv1alpha1.IngressPort) error {
	if newPort.Port != oldPort.Port || newPort.Protocol != oldPort.Protocol || newPort.Description != oldPort.Description {
		return fmt.Errorf("cannot change ports after creation")
	}

	return nil
}

// ValidateFieldNotChanged validates that a field hasn't changed during an update
func ValidateFieldNotChanged(fieldName string, oldValue, newValue interface{}) error {
	if oldValue != newValue {
		return fmt.Errorf("cannot change %s after creation", fieldName)
	}
	return nil
}

// SkipValidationForDeletion checks if validation should be skipped for a resource being deleted
func SkipValidationForDeletion(ctx context.Context, obj metav1.Object) bool {
	logger := log.FromContext(ctx)

	// TEMPORARY-DEBUG-CODE: Enhanced logging for problematic resources
	problematicNames := []string{"dynamic-2rx8z", "dynamic-7dls7", "dynamic-fb5qw", "dynamic-g6jfj", "dynamic-jd2b7", "dynamic-lsjlt"}
	isProblematic := false

	for _, name := range problematicNames {
		if obj.GetName() == name {
			isProblematic = true
			break
		}
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		if isProblematic {
			logger.Info("TEMPORARY-DEBUG-CODE: Skipping validation for problematic resource being deleted",
				"name", obj.GetName(),
				"namespace", obj.GetNamespace(),
				"deletionTimestamp", obj.GetDeletionTimestamp(),
				"finalizers", obj.GetFinalizers(),
				"resourceVersion", obj.GetResourceVersion())
		} else {
			logger.Info("Skipping validation for resource being deleted",
				"name", obj.GetName(),
				"namespace", obj.GetNamespace(),
				"deletionTimestamp", obj.GetDeletionTimestamp())
		}
		return true
	}

	return false
}

// IsReadyConditionTrue checks if the Ready condition is true for the given object
func IsReadyConditionTrue(obj runtime.Object) bool {
	switch o := obj.(type) {
	case *netguardv1alpha1.AddressGroupBinding:
		return isConditionTrue(o.Status.Conditions, netguardv1alpha1.ConditionReady)
	case *netguardv1alpha1.Service:
		return isConditionTrue(o.Status.Conditions, netguardv1alpha1.ConditionReady)
	case *netguardv1alpha1.AddressGroupPortMapping:
		return isConditionTrue(o.Status.Conditions, netguardv1alpha1.ConditionReady)
	case *netguardv1alpha1.AddressGroupBindingPolicy:
		return isConditionTrue(o.Status.Conditions, netguardv1alpha1.ConditionReady)
	case *providerv1alpha1.AddressGroup:
		return isConditionTrue(o.Status.Conditions, providerv1alpha1.ConditionReady)
	default:
		// If we don't know how to check the condition, assume it's not ready
		return false
	}
}

// isConditionTrue checks if a specific condition is true in the given conditions list
func isConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionTrue
		}
	}
	// If the condition is not found, assume it's not true
	return false
}

// ValidateFieldNotChangedWhenReady validates that a field hasn't changed during an update
// if the Ready condition is true
func ValidateFieldNotChangedWhenReady(fieldName string, oldObj, newObj runtime.Object, oldValue, newValue interface{}) error {
	if oldValue != newValue {
		// Check if the Ready condition is true in the old object
		if IsReadyConditionTrue(oldObj) {
			return fmt.Errorf("cannot change %s when Ready condition is true", fieldName)
		}
	}
	return nil
}

// ValidateSpecNotChangedWhenReady validates that the Spec hasn't changed during an update
// if the Ready condition is true
func ValidateSpecNotChangedWhenReady(oldObj, newObj runtime.Object, oldSpec, newSpec interface{}) error {
	// Check if specs are different
	if !reflect.DeepEqual(oldSpec, newSpec) {
		// Check if the Ready condition is true in the old object
		if IsReadyConditionTrue(oldObj) {
			return fmt.Errorf("spec cannot be changed when Ready condition is true")
		}
	}
	return nil
}

// validatePort validates a port string, which can be a single port, a port range, or a comma-separated list of ports/ranges
func validatePort(port string) error {
	// Allow empty port string
	if port == "" {
		return nil
	}

	item := strings.TrimSpace(port)

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

	return nil
}

// ValidateNoDuplicatePorts validates that there are no duplicate or overlapping port configurations in the list
func ValidateNoDuplicatePorts(ingressPorts []netguardv1alpha1.IngressPort) error {
	// Create maps to store port ranges by protocol
	tcpRanges := []PortRange{}
	udpRanges := []PortRange{}

	for _, port := range ingressPorts {
		// Parse the port string to a PortRange
		portRange, err := ParsePortRange(port.Port)
		if err != nil {
			return fmt.Errorf("invalid port %s: %w", port.Port, err)
		}

		// Check for overlaps with existing ports of the same protocol
		var existingRanges []PortRange
		if port.Protocol == netguardv1alpha1.ProtocolTCP {
			existingRanges = tcpRanges
		} else if port.Protocol == netguardv1alpha1.ProtocolUDP {
			existingRanges = udpRanges
		}

		for _, existingRange := range existingRanges {
			if DoPortRangesOverlap(portRange, existingRange) {
				return fmt.Errorf("port conflict detected: %s port range %s overlaps with existing port range %d-%d",
					port.Protocol, port.Port, existingRange.Start, existingRange.End)
			}
		}

		// Add this port range to the appropriate map
		if port.Protocol == netguardv1alpha1.ProtocolTCP {
			tcpRanges = append(tcpRanges, portRange)
		} else if port.Protocol == netguardv1alpha1.ProtocolUDP {
			udpRanges = append(udpRanges, portRange)
		}
	}

	return nil
}

// ValidateNoDuplicatePortsInPortConfig validates that there are no duplicate or overlapping port configurations in the list
func ValidateNoDuplicatePortsInPortConfig(ports []netguardv1alpha1.PortConfig) error {
	// Create a slice to store port ranges
	ranges := []PortRange{}

	for _, port := range ports {
		// Parse the port string to a PortRange
		portRange, err := ParsePortRange(port.Port)
		if err != nil {
			return fmt.Errorf("invalid port %s: %w", port.Port, err)
		}

		// Check for overlaps with existing ports
		for _, existingRange := range ranges {
			if DoPortRangesOverlap(portRange, existingRange) {
				return fmt.Errorf("port conflict detected: port range %s overlaps with existing port range %d-%d",
					port.Port, existingRange.Start, existingRange.End)
			}
		}

		// Add this port range to the slice
		ranges = append(ranges, portRange)
	}

	return nil
}
