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

// ObjectReference contains enough information to let you locate the referenced object
type ObjectReference struct {
	// APIVersion of the referenced object
	APIVersion string `json:"apiVersion"`

	// Kind of the referenced object
	Kind string `json:"kind"`

	// Name of the referenced object
	Name string `json:"name"`
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

// NamespacedObjectReference extends ObjectReference with a Namespace field
type NamespacedObjectReference struct {
	// Embedded ObjectReference
	ObjectReference `json:",inline"`

	// Namespace of the referenced object
	Namespace string `json:"namespace,omitempty"`
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
