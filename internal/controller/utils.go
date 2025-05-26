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

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// DefaultMaxRetries is the default number of retries for operations
	DefaultMaxRetries = 3

	// DefaultRetryInterval is the default interval between retries
	DefaultRetryInterval = 100 * time.Millisecond

	// DefaultTimeout is the default timeout for operations
	DefaultTimeout = 30 * time.Second
)

// UpdateWithRetry updates a resource with retries on conflict
func UpdateWithRetry(ctx context.Context, c client.Client, obj client.Object, maxRetries int) error {
	logger := log.FromContext(ctx)
	name := obj.GetName()
	namespace := obj.GetNamespace()

	for i := 0; i < maxRetries; i++ {
		err := c.Update(ctx, obj)
		if err == nil {
			return nil
		}

		if !apierrors.IsConflict(err) {
			return err
		}

		// Get the latest version of the object
		latest := obj.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, latest); err != nil {
			return err
		}

		// Log the conflict and retry
		logger.Info("Conflict detected, retrying update",
			"resource", fmt.Sprintf("%s/%s", namespace, name),
			"attempt", i+1,
			"maxRetries", maxRetries)

		// Wait before retrying
		time.Sleep(DefaultRetryInterval)
	}

	return fmt.Errorf("failed to update resource after %d retries", maxRetries)
}

// PatchWithRetry patches a resource with retries on conflict
func PatchWithRetry(ctx context.Context, c client.Client, obj client.Object, patch client.Patch, maxRetries int) error {
	logger := log.FromContext(ctx)
	name := obj.GetName()
	namespace := obj.GetNamespace()

	for i := 0; i < maxRetries; i++ {
		err := c.Patch(ctx, obj, patch)
		if err == nil {
			return nil
		}

		if !apierrors.IsConflict(err) {
			return err
		}

		// Get the latest version of the object
		latest := obj.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, latest); err != nil {
			return err
		}

		// Log the conflict and retry
		logger.Info("Conflict detected, retrying patch",
			"resource", fmt.Sprintf("%s/%s", namespace, name),
			"attempt", i+1,
			"maxRetries", maxRetries)

		// Wait before retrying
		time.Sleep(DefaultRetryInterval)
	}

	return fmt.Errorf("failed to patch resource after %d retries", maxRetries)
}

// UpdateStatusWithRetry updates a resource's status with retries on conflict
func UpdateStatusWithRetry(ctx context.Context, c client.Client, obj client.Object, maxRetries int) error {
	logger := log.FromContext(ctx)
	name := obj.GetName()
	namespace := obj.GetNamespace()

	for i := 0; i < maxRetries; i++ {
		err := c.Status().Update(ctx, obj)
		if err == nil {
			return nil
		}

		if !apierrors.IsConflict(err) {
			return err
		}

		// Get the latest version of the object
		latest := obj.DeepCopyObject().(client.Object)
		if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, latest); err != nil {
			return err
		}

		// Log the conflict and retry
		logger.Info("Conflict detected, retrying status update",
			"resource", fmt.Sprintf("%s/%s", namespace, name),
			"attempt", i+1,
			"maxRetries", maxRetries)

		// Wait before retrying
		time.Sleep(DefaultRetryInterval)
	}

	return fmt.Errorf("failed to update resource status after %d retries", maxRetries)
}

// EnsureFinalizer ensures that a finalizer is added to an object
func EnsureFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
	if controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil // Finalizer already exists
	}

	// Create a copy for patching
	objCopy := obj.DeepCopyObject().(client.Object)
	controllerutil.AddFinalizer(objCopy, finalizer)

	// Apply patch
	patch := client.MergeFrom(obj)
	return PatchWithRetry(ctx, c, objCopy, patch, DefaultMaxRetries)
}

// RemoveFinalizer removes a finalizer from an object
func RemoveFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
	if !controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil // Finalizer already removed
	}

	// Create a copy for patching
	objCopy := obj.DeepCopyObject().(client.Object)
	controllerutil.RemoveFinalizer(objCopy, finalizer)

	// Apply patch
	patch := client.MergeFrom(obj)
	return PatchWithRetry(ctx, c, objCopy, patch, DefaultMaxRetries)
}

// SetCondition sets a condition on an object's status
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	}

	// Find and update existing condition or append new one
	meta.SetStatusCondition(conditions, condition)
}

// SafeDeleteAndWait safely deletes a resource and waits for it to be gone
func SafeDeleteAndWait(ctx context.Context, c client.Client, obj client.Object, timeout time.Duration) error {
	logger := log.FromContext(ctx)
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}

	// Check if resource exists
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Resource already deleted
		}
		return err
	}

	// Delete resource
	if err := c.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Resource already deleted
		}
		return err
	}

	logger.Info("Waiting for resource to be deleted",
		"resource", fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()),
		"timeout", timeout)

	// Wait for resource to be deleted
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	checkObj := obj.DeepCopyObject().(client.Object)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timeout waiting for resource deletion")
		case <-ticker.C:
			err := c.Get(ctx, key, checkObj)
			if apierrors.IsNotFound(err) {
				return nil // Resource successfully deleted
			}
		}
	}
}
