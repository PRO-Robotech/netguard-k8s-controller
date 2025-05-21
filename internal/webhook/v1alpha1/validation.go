package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

// ValidateObjectReference checks the basic validity of an object reference
func ValidateObjectReference(ref netguardv1alpha1.ObjectReference, expectedKind, expectedAPIVersion string) error {
	if ref.GetName() == "" {
		return fmt.Errorf("%s.name cannot be empty", expectedKind)
	}

	if expectedKind != "" && ref.GetKind() != expectedKind {
		return fmt.Errorf("reference must be to a %s resource, got %s", expectedKind, ref.GetKind())
	}

	if expectedAPIVersion != "" && ref.GetAPIVersion() != expectedAPIVersion {
		return fmt.Errorf("reference must be to a resource with APIVersion %s, got %s",
			expectedAPIVersion, ref.GetAPIVersion())
	}

	return nil
}

// ValidateObjectReferenceNotChanged checks that a reference hasn't changed during an update
func ValidateObjectReferenceNotChanged(oldRef, newRef netguardv1alpha1.ObjectReferencer, fieldName string) error {
	if oldRef.GetName() != newRef.GetName() {
		return fmt.Errorf("cannot change %s.name after creation", fieldName)
	}

	if oldRef.GetKind() != newRef.GetKind() {
		return fmt.Errorf("cannot change %s.kind after creation", fieldName)
	}

	if oldRef.GetAPIVersion() != newRef.GetAPIVersion() {
		return fmt.Errorf("cannot change %s.apiVersion after creation", fieldName)
	}

	// Check namespace only if both objects support namespaces
	if oldRef.IsNamespaced() && newRef.IsNamespaced() {
		if oldRef.GetNamespace() != newRef.GetNamespace() {
			return fmt.Errorf("cannot change %s.namespace after creation", fieldName)
		}
	}

	return nil
}

// ValidateObjectReferenceNotChangedWhenReady checks that a reference hasn't changed during an update
// if the Ready condition is true
func ValidateObjectReferenceNotChangedWhenReady(oldObj, newObj runtime.Object, oldRef, newRef netguardv1alpha1.ObjectReferencer, fieldName string) error {
	// Check if any reference fields have changed
	if oldRef.GetName() != newRef.GetName() ||
		oldRef.GetKind() != newRef.GetKind() ||
		oldRef.GetAPIVersion() != newRef.GetAPIVersion() ||
		(oldRef.IsNamespaced() && newRef.IsNamespaced() && oldRef.GetNamespace() != newRef.GetNamespace()) {

		// Check if the Ready condition is true in the old object
		if IsReadyConditionTrue(oldObj) {
			// Determine which field changed for a more specific error message
			if oldRef.GetName() != newRef.GetName() {
				return fmt.Errorf("cannot change %s.name when Ready condition is true", fieldName)
			}
			if oldRef.GetKind() != newRef.GetKind() {
				return fmt.Errorf("cannot change %s.kind when Ready condition is true", fieldName)
			}
			if oldRef.GetAPIVersion() != newRef.GetAPIVersion() {
				return fmt.Errorf("cannot change %s.apiVersion when Ready condition is true", fieldName)
			}
			if oldRef.IsNamespaced() && newRef.IsNamespaced() && oldRef.GetNamespace() != newRef.GetNamespace() {
				return fmt.Errorf("cannot change %s.namespace when Ready condition is true", fieldName)
			}

			// Generic fallback error
			return fmt.Errorf("cannot change %s when Ready condition is true", fieldName)
		}
	}

	return nil
}
