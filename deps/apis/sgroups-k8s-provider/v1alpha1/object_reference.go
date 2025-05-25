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
	"fmt"
)

// ObjectReferencer defines the interface for working with object references
// +k8s:deepcopy-gen=false
type ObjectReferencer interface {
	// GetAPIVersion returns the API version of the referenced object
	GetAPIVersion() string

	// GetKind returns the kind of the referenced object
	GetKind() string

	// GetName returns the name of the referenced object
	GetName() string

	// GetNamespace returns the namespace of the referenced object (may be empty)
	GetNamespace() string

	// IsNamespaced returns true if the reference contains a namespace
	IsNamespaced() bool

	// ResolveNamespace returns the namespace, using defaultNamespace if not specified
	ResolveNamespace(defaultNamespace string) string
}

// GetAPIVersion returns the API version of the referenced object
func (r *ObjectReference) GetAPIVersion() string {
	return r.APIVersion
}

// GetKind returns the kind of the referenced object
func (r *ObjectReference) GetKind() string {
	return r.Kind
}

// GetName returns the name of the referenced object
func (r *ObjectReference) GetName() string {
	return r.Name
}

// GetNamespace returns an empty string for ObjectReference
func (r *ObjectReference) GetNamespace() string {
	return ""
}

// IsNamespaced returns false for ObjectReference
func (r *ObjectReference) IsNamespaced() bool {
	return false
}

// ResolveNamespace returns defaultNamespace for ObjectReference
func (r *ObjectReference) ResolveNamespace(defaultNamespace string) string {
	return defaultNamespace
}

// GetAPIVersion returns the API version of the referenced object
func (r *NamespacedObjectReference) GetAPIVersion() string {
	return r.APIVersion
}

// GetKind returns the kind of the referenced object
func (r *NamespacedObjectReference) GetKind() string {
	return r.Kind
}

// GetName returns the name of the referenced object
func (r *NamespacedObjectReference) GetName() string {
	return r.Name
}

// GetNamespace returns the namespace of the referenced object
func (r *NamespacedObjectReference) GetNamespace() string {
	return r.Namespace
}

// IsNamespaced returns true for NamespacedObjectReference
func (r *NamespacedObjectReference) IsNamespaced() bool {
	return true
}

// ResolveNamespace returns the namespace, using defaultNamespace if not specified
func (r *NamespacedObjectReference) ResolveNamespace(defaultNamespace string) string {
	if r.Namespace == "" {
		return defaultNamespace
	}
	return r.Namespace
}

// ValidateObjectReference checks the basic validity of an object reference
func ValidateObjectReference(ref ObjectReferencer, expectedKind, expectedAPIVersion string) error {
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
func ValidateObjectReferenceNotChanged(oldRef, newRef ObjectReferencer, fieldName string) error {
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
