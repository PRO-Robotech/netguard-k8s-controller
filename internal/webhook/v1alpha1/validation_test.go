package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

func TestValidateObjectReference(t *testing.T) {
	tests := []struct {
		name               string
		ref                netguardv1alpha1.ObjectReference
		expectedKind       string
		expectedAPIVersion string
		wantErr            bool
		errContains        string
	}{
		{
			name: "Valid reference",
			ref: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			expectedKind:       "Service",
			expectedAPIVersion: "netguard.sgroups.io/v1alpha1",
			wantErr:            false,
		},
		{
			name: "Empty name",
			ref: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "",
			},
			expectedKind:       "Service",
			expectedAPIVersion: "netguard.sgroups.io/v1alpha1",
			wantErr:            true,
			errContains:        "Service.name cannot be empty",
		},
		{
			name: "Wrong kind",
			ref: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "WrongKind",
				Name:       "test-service",
			},
			expectedKind:       "Service",
			expectedAPIVersion: "netguard.sgroups.io/v1alpha1",
			wantErr:            true,
			errContains:        "reference must be to a Service resource",
		},
		{
			name: "Wrong API version",
			ref: netguardv1alpha1.ObjectReference{
				APIVersion: "wrong.api/v1",
				Kind:       "Service",
				Name:       "test-service",
			},
			expectedKind:       "Service",
			expectedAPIVersion: "netguard.sgroups.io/v1alpha1",
			wantErr:            true,
			errContains:        "reference must be to a resource with APIVersion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateObjectReference(tt.ref, tt.expectedKind, tt.expectedAPIVersion)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateObjectReferenceNotChanged(t *testing.T) {
	tests := []struct {
		name        string
		oldRef      netguardv1alpha1.ObjectReference
		newRef      netguardv1alpha1.ObjectReference
		fieldName   string
		wantErr     bool
		errContains string
	}{
		{
			name: "No changes",
			oldRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			newRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			fieldName: "spec.serviceRef",
			wantErr:   false,
		},
		{
			name: "Name changed",
			oldRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			newRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "changed-service",
			},
			fieldName:   "spec.serviceRef",
			wantErr:     true,
			errContains: "cannot change spec.serviceRef.name after creation",
		},
		{
			name: "Kind changed",
			oldRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			newRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "AddressGroup",
				Name:       "test-service",
			},
			fieldName:   "spec.serviceRef",
			wantErr:     true,
			errContains: "cannot change spec.serviceRef.kind after creation",
		},
		{
			name: "APIVersion changed",
			oldRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			newRef: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "test-service",
			},
			fieldName:   "spec.serviceRef",
			wantErr:     true,
			errContains: "cannot change spec.serviceRef.apiVersion after creation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateObjectReferenceNotChanged(&tt.oldRef, &tt.newRef, tt.fieldName)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Mock implementation of netguardv1alpha1.ObjectReferencer for testing
type mockObjectReferencer struct {
	name       string
	kind       string
	apiVersion string
	namespace  string
	namespaced bool
}

func (m *mockObjectReferencer) GetName() string {
	return m.name
}

func (m *mockObjectReferencer) GetKind() string {
	return m.kind
}

func (m *mockObjectReferencer) GetAPIVersion() string {
	return m.apiVersion
}

func (m *mockObjectReferencer) GetNamespace() string {
	return m.namespace
}

func (m *mockObjectReferencer) IsNamespaced() bool {
	return m.namespaced
}

func (m *mockObjectReferencer) ResolveNamespace(defaultNamespace string) string {
	if m.namespace == "" {
		return defaultNamespace
	}
	return m.namespace
}

func TestValidateObjectReferenceNotChangedWhenReady(t *testing.T) {
	// Create mock objects with Ready condition
	readyObj := &netguardv1alpha1.Service{
		Status: netguardv1alpha1.ServiceStatus{
			Conditions: []metav1.Condition{
				{
					Type:   netguardv1alpha1.ConditionReady,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	notReadyObj := &netguardv1alpha1.Service{
		Status: netguardv1alpha1.ServiceStatus{
			Conditions: []metav1.Condition{
				{
					Type:   netguardv1alpha1.ConditionReady,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	tests := []struct {
		name        string
		oldObj      runtime.Object
		newObj      runtime.Object
		oldRef      netguardv1alpha1.ObjectReferencer
		newRef      netguardv1alpha1.ObjectReferencer
		fieldName   string
		wantErr     bool
		errContains string
	}{
		{
			name:   "No changes",
			oldObj: readyObj,
			newObj: readyObj,
			oldRef: &mockObjectReferencer{
				name:       "test-service",
				kind:       "Service",
				apiVersion: "netguard.sgroups.io/v1alpha1",
				namespace:  "default",
				namespaced: true,
			},
			newRef: &mockObjectReferencer{
				name:       "test-service",
				kind:       "Service",
				apiVersion: "netguard.sgroups.io/v1alpha1",
				namespace:  "default",
				namespaced: true,
			},
			fieldName: "spec.serviceRef",
			wantErr:   false,
		},
		{
			name:   "Name changed when ready",
			oldObj: readyObj,
			newObj: readyObj,
			oldRef: &mockObjectReferencer{
				name:       "test-service",
				kind:       "Service",
				apiVersion: "netguard.sgroups.io/v1alpha1",
				namespace:  "default",
				namespaced: true,
			},
			newRef: &mockObjectReferencer{
				name:       "changed-service",
				kind:       "Service",
				apiVersion: "netguard.sgroups.io/v1alpha1",
				namespace:  "default",
				namespaced: true,
			},
			fieldName:   "spec.serviceRef",
			wantErr:     true,
			errContains: "cannot change spec.serviceRef.name when Ready condition is true",
		},
		{
			name:   "Name changed when not ready",
			oldObj: notReadyObj,
			newObj: notReadyObj,
			oldRef: &mockObjectReferencer{
				name:       "test-service",
				kind:       "Service",
				apiVersion: "netguard.sgroups.io/v1alpha1",
				namespace:  "default",
				namespaced: true,
			},
			newRef: &mockObjectReferencer{
				name:       "changed-service",
				kind:       "Service",
				apiVersion: "netguard.sgroups.io/v1alpha1",
				namespace:  "default",
				namespaced: true,
			},
			fieldName: "spec.serviceRef",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateObjectReferenceNotChangedWhenReady(tt.oldObj, tt.newObj, tt.oldRef, tt.newRef, tt.fieldName)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
