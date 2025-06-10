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
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
)

// RuleS2SReconciler reconciles a RuleS2S object
type RuleS2SReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=rules2s,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=rules2s/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=rules2s/finalizers,verbs=update
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=netguard.sgroups.io,resources=servicealiases,verbs=get;list;watch
// +kubebuilder:rbac:groups=provider.sgroups.io,resources=ieagagrules,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *RuleS2SReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling RuleS2S", "request", req)

	// Fetch the RuleS2S instance
	ruleS2S := &netguardv1alpha1.RuleS2S{}
	if err := r.Get(ctx, req.NamespacedName, ruleS2S); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, could have been deleted after reconcile request
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		return ctrl.Result{}, err
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(ruleS2S, "netguard.sgroups.io/finalizer") {
		logger.Info("Adding finalizer to RuleS2S", "name", ruleS2S.Name)
		controllerutil.AddFinalizer(ruleS2S, "netguard.sgroups.io/finalizer")
		if err := UpdateWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to add finalizer to RuleS2S")
			return ctrl.Result{}, err
		}
		// Return to avoid processing the same object twice in one reconciliation
		return ctrl.Result{}, nil
	}

	// Check if the resource is being deleted
	if !ruleS2S.DeletionTimestamp.IsZero() {
		// Delete related IEAgAgRules
		if err := r.deleteRelatedIEAgAgRules(ctx, ruleS2S); err != nil {
			// Check if this is our custom error type
			if failedErr, ok := err.(*FailedToDeleteRulesError); ok {
				// Update status with error condition
				errorMsg := fmt.Sprintf("Cannot delete RuleS2S because some IEAgAgRules could not be deleted: %s",
					strings.Join(failedErr.FailedRules, ", "))

				meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
					Type:    netguardv1alpha1.ConditionReady,
					Status:  metav1.ConditionFalse,
					Reason:  "FailedToDeleteRules",
					Message: errorMsg,
				})

				if updateErr := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); updateErr != nil {
					logger.Error(updateErr, "Failed to update RuleS2S status")
				}

				logger.Error(err, "Failed to delete all related IEAgAgRules",
					"failedRules", strings.Join(failedErr.FailedRules, ", "))

				return ctrl.Result{}, nil
			}

			// For other errors, log and return the error
			logger.Error(err, "Failed to delete related IEAgAgRules")
			return ctrl.Result{}, err
		}

		// All related IEAgAgRules have been deleted, now remove the finalizer
		logger.Info("Removing finalizer from RuleS2S", "name", ruleS2S.Name)
		controllerutil.RemoveFinalizer(ruleS2S, "netguard.sgroups.io/finalizer")
		if err := UpdateWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to remove finalizer from RuleS2S")
			return ctrl.Result{}, err
		}

		// Resource is being deleted and finalizer has been removed
		return ctrl.Result{}, nil
	}

	// Get the ServiceAlias objects
	localServiceAlias := &netguardv1alpha1.ServiceAlias{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: ruleS2S.Namespace,
		Name:      ruleS2S.Spec.ServiceLocalRef.Name,
	}, localServiceAlias); err != nil {
		errorMsg := fmt.Sprintf("Local service alias '%s' not found in namespace '%s': %v",
			ruleS2S.Spec.ServiceLocalRef.Name, ruleS2S.Namespace, err)

		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceAliasNotFound",
			Message: errorMsg,
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
		}

		// Логируем информацию
		logger.Info(errorMsg, "name", ruleS2S.Spec.ServiceLocalRef.Name)

		// Возвращаем пустой Result без RequeueAfter
		return ctrl.Result{}, nil
	}

	targetServiceAlias := &netguardv1alpha1.ServiceAlias{}
	targetNamespace := ruleS2S.Spec.ServiceRef.ResolveNamespace(ruleS2S.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: targetNamespace,
		Name:      ruleS2S.Spec.ServiceRef.Name,
	}, targetServiceAlias); err != nil {
		errorMsg := fmt.Sprintf("Target service alias '%s' not found in namespace '%s': %v",
			ruleS2S.Spec.ServiceRef.Name, targetNamespace, err)

		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceAliasNotFound",
			Message: errorMsg,
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
		}

		// Логируем информацию
		logger.Info(errorMsg, "name", ruleS2S.Spec.ServiceRef.Name, "namespace", targetNamespace)

		// Возвращаем пустой Result без RequeueAfter
		return ctrl.Result{}, nil
	}

	// Get the actual Service objects
	localService := &netguardv1alpha1.Service{}
	localServiceNamespace := localServiceAlias.Spec.ServiceRef.ResolveNamespace(localServiceAlias.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: localServiceNamespace,
		Name:      localServiceAlias.Spec.ServiceRef.Name,
	}, localService); err != nil {
		errorMsg := fmt.Sprintf("Local service '%s' not found in namespace '%s' (referenced by ServiceAlias '%s'): %v",
			localServiceAlias.Spec.ServiceRef.Name, localServiceNamespace, localServiceAlias.Name, err)

		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceNotFound",
			Message: errorMsg,
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
		}

		// Логируем информацию
		logger.Info(errorMsg, "name", localServiceAlias.Spec.ServiceRef.Name, "namespace", localServiceNamespace)

		// Возвращаем пустой Result без RequeueAfter
		return ctrl.Result{}, nil
	}

	targetService := &netguardv1alpha1.Service{}
	targetServiceNamespace := targetServiceAlias.Spec.ServiceRef.ResolveNamespace(targetServiceAlias.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: targetServiceNamespace,
		Name:      targetServiceAlias.Spec.ServiceRef.Name,
	}, targetService); err != nil {
		errorMsg := fmt.Sprintf("Target service '%s' not found in namespace '%s' (referenced by ServiceAlias '%s'): %v",
			targetServiceAlias.Spec.ServiceRef.Name, targetServiceNamespace, targetServiceAlias.Name, err)

		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceNotFound",
			Message: errorMsg,
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
		}

		// Логируем информацию
		logger.Info(errorMsg, "name", targetServiceAlias.Spec.ServiceRef.Name, "namespace", targetServiceNamespace)

		// Возвращаем пустой Result без RequeueAfter
		return ctrl.Result{}, nil
	}

	// Update RuleS2SDstOwnRef for cross-namespace references
	if ruleS2S.Namespace != targetServiceNamespace {
		// Add this rule to the target service's RuleS2SDstOwnRef
		found := false
		for _, ref := range targetService.RuleS2SDstOwnRef.Items {
			if ref.Name == ruleS2S.Name && ref.Namespace == ruleS2S.Namespace {
				found = true
				break
			}
		}

		if !found {
			targetService.RuleS2SDstOwnRef.Items = append(targetService.RuleS2SDstOwnRef.Items,
				netguardv1alpha1.NamespacedObjectReference{
					ObjectReference: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "RuleS2S",
						Name:       ruleS2S.Name,
					},
					Namespace: ruleS2S.Namespace,
				})

			if err := UpdateWithRetry(ctx, r.Client, targetService, DefaultMaxRetries); err != nil {
				errorMsg := fmt.Sprintf("Failed to update target service '%s' RuleS2SDstOwnRef: %v", targetService.Name, err)
				logger.Error(err, errorMsg)

				// Проверяем, нужна ли периодическая проверка для этого правила
				if val, ok := ruleS2S.Annotations["netguard.sgroups.io/periodic-reconcile"]; ok && val == "true" {
					return ctrl.Result{RequeueAfter: time.Minute}, err
				}

				return ctrl.Result{}, err
			}
		}
	} else {
		// For rules in the same namespace, use owner references
		if err := controllerutil.SetControllerReference(targetService, ruleS2S, r.Scheme); err != nil {
			errorMsg := fmt.Sprintf("Failed to set owner reference from target service '%s' to RuleS2S '%s': %v",
				targetService.Name, ruleS2S.Name, err)
			logger.Error(err, errorMsg)

			return ctrl.Result{}, err
		}
		if err := UpdateWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			errorMsg := fmt.Sprintf("Failed to update RuleS2S '%s' with owner reference to service '%s': %v",
				ruleS2S.Name, targetService.Name, err)
			logger.Error(err, errorMsg)

			return ctrl.Result{}, err
		}
	}

	// Get address groups from services
	localAddressGroups := localService.AddressGroups.Items
	targetAddressGroups := targetService.AddressGroups.Items

	// Determine which ports to use based on traffic direction
	// In both cases, we use ports from the service that receives the traffic
	var ports []netguardv1alpha1.IngressPort
	if strings.ToLower(ruleS2S.Spec.Traffic) == "ingress" {
		// For ingress, local service is the receiver
		ports = localService.Spec.IngressPorts
	} else {
		// For egress, target service is the receiver
		ports = targetService.Spec.IngressPorts
	}

	// Collect all inactive conditions
	var inactiveConditions []string

	// Check address groups
	if len(localAddressGroups) == 0 && len(targetAddressGroups) == 0 {
		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("Both services have no address groups: localService '%s', targetService '%s'",
				localService.Name, targetService.Name))
	} else if len(localAddressGroups) == 0 {
		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("LocalService '%s' has no address groups", localService.Name))
	} else if len(targetAddressGroups) == 0 {
		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("TargetService '%s' has no address groups", targetService.Name))
	}

	// Check ports
	if len(ports) == 0 {
		var serviceName string
		if strings.ToLower(ruleS2S.Spec.Traffic) == "ingress" {
			serviceName = fmt.Sprintf("local service '%s'", localService.Name)
		} else {
			serviceName = fmt.Sprintf("target service '%s'", targetService.Name)
		}

		inactiveConditions = append(inactiveConditions,
			fmt.Sprintf("No ports defined for the %s (traffic direction: %s)",
				serviceName, ruleS2S.Spec.Traffic))
	}

	// If there are any inactive conditions, set status and delete related rules
	if len(inactiveConditions) > 0 {
		// Format the message without numbering and extra line breaks
		var formattedMessage strings.Builder
		formattedMessage.WriteString("Rule is valid but inactive due to the following reasons: ")

		for i, condition := range inactiveConditions {
			formattedMessage.WriteString(condition)
			if i < len(inactiveConditions)-1 {
				formattedMessage.WriteString("; ")
			}
		}

		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "ValidConfiguration",
			Message: formattedMessage.String(),
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
		}

		// Логируем информацию
		logger.Info("Rule is valid but inactive, deleting related IEAgAgRules",
			"conditions", strings.Join(inactiveConditions, "; "),
			"localService", localService.Name,
			"targetService", targetService.Name)

		// Удаляем связанные IEAgAgRules
		if err := r.deleteRelatedIEAgAgRules(ctx, ruleS2S); err != nil {
			// Если не удалось удалить некоторые правила, логируем ошибку, но продолжаем
			logger.Error(err, "Failed to delete some related IEAgAgRules")
		}

		// Удаляем само правило RuleS2S
		logger.Info("Auto-deleting inactive RuleS2S", "name", ruleS2S.Name, "namespace", ruleS2S.Namespace)
		if err := SafeDeleteAndWait(ctx, r.Client, ruleS2S, 30*time.Second); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "Failed to auto-delete inactive RuleS2S", "name", ruleS2S.Name)
				return ctrl.Result{}, err
			}
		}

		// Возвращаем пустой Result без RequeueAfter
		return ctrl.Result{}, nil
	}

	// Get all existing IEAgAgRules for this RuleS2S
	existingRules, err := r.getExistingIEAgAgRules(ctx, ruleS2S)
	if err != nil {
		logger.Error(err, "Failed to get existing IEAgAg rules")
		return ctrl.Result{}, err
	}

	// Create a map to track which rules should exist after reconciliation
	// The key is "namespace/name" to uniquely identify each rule
	expectedRules := make(map[string]bool)

	// Create IEAgAgRule resources for each combination of address groups and ports
	logger.Info("Starting rule creation for RuleS2S",
		"name", ruleS2S.Name,
		"namespace", ruleS2S.Namespace,
		"uid", ruleS2S.GetUID(),
		"localAddressGroups", len(localAddressGroups),
		"targetAddressGroups", len(targetAddressGroups),
		"ports", len(ports),
		"existingRules", len(existingRules))

	createdRules := []string{}
	for i, localAG := range localAddressGroups {
		for j, targetAG := range targetAddressGroups {
			logger.Info("Processing address group combination",
				"localAG", localAG.Name,
				"localAG.Namespace", localAG.GetNamespace(),
				"targetAG", targetAG.Name,
				"targetAG.Namespace", targetAG.GetNamespace(),
				"combination", fmt.Sprintf("%d/%d", i*len(targetAddressGroups)+j+1, len(localAddressGroups)*len(targetAddressGroups)))

			// Group ports by protocol
			tcpPorts := []string{}
			udpPorts := []string{}

			for _, port := range ports {
				if port.Protocol == netguardv1alpha1.ProtocolTCP {
					tcpPorts = append(tcpPorts, port.Port)
				} else if port.Protocol == netguardv1alpha1.ProtocolUDP {
					udpPorts = append(udpPorts, port.Port)
				}
			}

			logger.Info("Grouped ports by protocol",
				"tcpPorts", len(tcpPorts),
				"udpPorts", len(udpPorts))

			// Create TCP rule if there are TCP ports
			if len(tcpPorts) > 0 {
				// Combine all TCP ports into a single comma-separated string
				combinedTcpPorts := strings.Join(tcpPorts, ",")

				logger.Info("Creating/updating TCP rule",
					"localAG", localAG.Name,
					"targetAG", targetAG.Name,
					"ports", combinedTcpPorts)

				// Create or update the rule
				ruleName, err := r.createOrUpdateIEAgAgRule(ctx, ruleS2S, localAG, targetAG,
					netguardv1alpha1.ProtocolTCP, combinedTcpPorts)
				if err != nil {
					logger.Error(err, "Failed to create/update TCP rule",
						"localAG", localAG.Name,
						"targetAG", targetAG.Name,
						"errorType", fmt.Sprintf("%T", err))
					continue
				}

				// Determine namespace for the rule based on traffic direction
				var ruleNamespace string
				if ruleS2S.Spec.Traffic == "ingress" {
					// For ingress, rule goes in the local AG namespace (receiver)
					ruleNamespace = localAG.ResolveNamespace(ruleS2S.GetNamespace())
				} else {
					// For egress, rule goes in the target AG namespace (receiver)
					ruleNamespace = targetAG.ResolveNamespace(ruleS2S.GetNamespace())
				}

				// Add to expected rules map
				expectedRuleKey := fmt.Sprintf("%s/%s", ruleNamespace, ruleName)
				expectedRules[expectedRuleKey] = true

				logger.Info("Successfully created/updated TCP rule",
					"ruleName", ruleName,
					"ruleNamespace", ruleNamespace,
					"localAG", localAG.Name,
					"targetAG", targetAG.Name)
				createdRules = append(createdRules, ruleName)
			}

			// Create UDP rule if there are UDP ports
			if len(udpPorts) > 0 {
				// Combine all UDP ports into a single comma-separated string
				combinedUdpPorts := strings.Join(udpPorts, ",")

				logger.Info("Creating/updating UDP rule",
					"localAG", localAG.Name,
					"targetAG", targetAG.Name,
					"ports", combinedUdpPorts)

				// Create or update the rule
				ruleName, err := r.createOrUpdateIEAgAgRule(ctx, ruleS2S, localAG, targetAG,
					netguardv1alpha1.ProtocolUDP, combinedUdpPorts)
				if err != nil {
					logger.Error(err, "Failed to create/update UDP rule",
						"localAG", localAG.Name,
						"targetAG", targetAG.Name,
						"errorType", fmt.Sprintf("%T", err))
					continue
				}
				// Determine namespace for the rule based on traffic direction
				var ruleNamespace string
				if ruleS2S.Spec.Traffic == "ingress" {
					// For ingress, rule goes in the local AG namespace (receiver)
					ruleNamespace = localAG.ResolveNamespace(ruleS2S.GetNamespace())
				} else {
					// For egress, rule goes in the target AG namespace (receiver)
					ruleNamespace = targetAG.ResolveNamespace(ruleS2S.GetNamespace())
				}

				// Add to expected rules map
				expectedRuleKey := fmt.Sprintf("%s/%s", ruleNamespace, ruleName)
				expectedRules[expectedRuleKey] = true

				logger.Info("Successfully created/updated UDP rule",
					"ruleName", ruleName,
					"ruleNamespace", ruleNamespace,
					"localAG", localAG.Name,
					"targetAG", targetAG.Name)
				createdRules = append(createdRules, ruleName)
			}
		}
	}

	logger.Info("Completed rule creation",
		"name", ruleS2S.Name,
		"createdRules", len(createdRules),
		"ruleNames", strings.Join(createdRules, ", "))

	// Delete rules that are no longer needed
	for _, rule := range existingRules {
		key := fmt.Sprintf("%s/%s", rule.Namespace, rule.Name)
		if !expectedRules[key] {
			logger.Info("Deleting obsolete IEAgAg rule",
				"name", rule.Name,
				"namespace", rule.Namespace,
				"reason", "AddressGroup removed from Service or no longer needed")

			if err := r.Delete(ctx, &rule); err != nil {
				if !errors.IsNotFound(err) {
					logger.Error(err, "Failed to delete obsolete IEAgAg rule",
						"name", rule.Name, "namespace", rule.Namespace)
					// Don't return error to avoid blocking reconciliation of other rules
				}
			}
		}
	}

	// Update status to Ready if we created at least one rule
	if len(createdRules) > 0 {
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "RulesCreated",
			Message: fmt.Sprintf("Created rules: %s", strings.Join(createdRules, ", ")),
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
			return ctrl.Result{}, err
		}
	} else {
		errorMsg := fmt.Sprintf("Failed to create any rules for RuleS2S '%s' (local service: '%s', target service: '%s')",
			ruleS2S.Name, localService.Name, targetService.Name)

		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NoRulesCreated",
			Message: errorMsg,
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			logger.Error(err, "Failed to update RuleS2S status")
		}

		// Логируем информацию
		logger.Info(errorMsg,
			"localService", localService.Name,
			"targetService", targetService.Name,
			"localAddressGroups", len(localAddressGroups),
			"targetAddressGroups", len(targetAddressGroups),
			"ports", len(ports))

		// Возвращаем пустой Result без RequeueAfter
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// createOrUpdateIEAgAgRule creates or updates an IEAgAgRule
func (r *RuleS2SReconciler) createOrUpdateIEAgAgRule(
	ctx context.Context,
	ruleS2S *netguardv1alpha1.RuleS2S,
	localAG netguardv1alpha1.NamespacedObjectReference,
	targetAG netguardv1alpha1.NamespacedObjectReference,
	protocol netguardv1alpha1.TransportProtocol,
	portsStr string,
) (string, error) {
	logger := log.FromContext(ctx)

	// Логирование входных параметров
	logger.Info("Starting createOrUpdateIEAgAgRule",
		"ruleS2S", ruleS2S.Name,
		"ruleS2S.UID", ruleS2S.GetUID(),
		"localAG", localAG.Name,
		"localAG.Namespace", localAG.GetNamespace(),
		"targetAG", targetAG.Name,
		"targetAG.Namespace", targetAG.GetNamespace(),
		"protocol", protocol,
		"ports", portsStr)

	// Determine namespace for the rule based on traffic direction
	var ruleNamespace string
	if ruleS2S.Spec.Traffic == "ingress" {
		// For ingress, rule goes in the local AG namespace (receiver)
		ruleNamespace = localAG.ResolveNamespace(ruleS2S.GetNamespace())
		logger.Info("Using ingress namespace logic",
			"ruleNamespace", ruleNamespace,
			"localAG.Namespace", localAG.GetNamespace(),
			"ruleS2S.Namespace", ruleS2S.GetNamespace())
	} else {
		// For egress, rule goes in the target AG namespace (receiver)
		ruleNamespace = targetAG.ResolveNamespace(ruleS2S.GetNamespace())
		logger.Info("Using egress namespace logic",
			"ruleNamespace", ruleNamespace,
			"targetAG.Namespace", targetAG.GetNamespace(),
			"ruleS2S.Namespace", ruleS2S.GetNamespace())
	}

	// Ensure namespace is not empty
	if ruleNamespace == "" {
		logger.Error(fmt.Errorf("empty namespace"), "Cannot create rule with empty namespace")
		return "", fmt.Errorf("cannot create rule with empty namespace")
	}

	// Generate rule name using the helper function
	ruleName := r.generateRuleName(
		ruleS2S.Spec.Traffic,
		localAG.Name,
		targetAG.Name,
		string(protocol))

	logger.Info("Generated rule name",
		"ruleName", ruleName,
		"input", fmt.Sprintf("%s-%s-%s-%s",
			strings.ToLower(ruleS2S.Spec.Traffic),
			localAG.Name,
			targetAG.Name,
			strings.ToLower(string(protocol))))

	// Define the rule spec
	ruleSpec := providerv1alpha1.IEAgAgRuleSpec{
		Transport: providerv1alpha1.TransportProtocol(string(protocol)),
		Traffic:   providerv1alpha1.TrafficDirection(strings.ToUpper(ruleS2S.Spec.Traffic)),
		AddressGroupLocal: providerv1alpha1.NamespacedObjectReference{
			ObjectReference: providerv1alpha1.ObjectReference{
				APIVersion: localAG.APIVersion,
				Kind:       localAG.Kind,
				Name:       localAG.Name,
			},
			Namespace: localAG.ResolveNamespace(localAG.GetNamespace()),
		},
		AddressGroup: providerv1alpha1.NamespacedObjectReference{
			ObjectReference: providerv1alpha1.ObjectReference{
				APIVersion: targetAG.APIVersion,
				Kind:       targetAG.Kind,
				Name:       targetAG.Name,
			},
			Namespace: targetAG.ResolveNamespace(targetAG.GetNamespace()),
		},
		Ports: []providerv1alpha1.AccPorts{
			{
				D: portsStr,
			},
		},
		Action: providerv1alpha1.ActionAccept,
		Logs:   true,
		Priority: &providerv1alpha1.RulePrioritySpec{
			Value: 100,
		},
	}

	// Check if the rule already exists
	existingRule := &providerv1alpha1.IEAgAgRule{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: ruleNamespace,
		Name:      ruleName,
	}, existingRule)

	if err != nil && errors.IsNotFound(err) {
		logger.Info("Rule not found, will create new",
			"namespace", ruleNamespace,
			"name", ruleName,
			"error", err.Error())

		// Rule doesn't exist, create it with retry
		logger.Info("Creating new IEAgAgRule", "namespace", ruleNamespace, "name", ruleName)

		// Create the rule
		newRule := &providerv1alpha1.IEAgAgRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ruleName,
				Namespace: ruleNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         netguardv1alpha1.GroupVersion.String(),
						Kind:               "RuleS2S",
						Name:               ruleS2S.GetName(),
						UID:                ruleS2S.GetUID(),
						Controller:         pointer.Bool(false),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
			Spec: ruleSpec,
		}
		logger.Info("IEAgAgRule owner refs",
			"rule", newRule.Name,
			"refs", newRule.OwnerReferences,
			"ruleS2S.UID", ruleS2S.GetUID())

		// Try to create with retries
		for i := 0; i < DefaultMaxRetries; i++ {
			logger.Info("Attempting to create rule",
				"namespace", ruleNamespace,
				"name", ruleName,
				"attempt", i+1,
				"maxRetries", DefaultMaxRetries)

			if err := r.Create(ctx, newRule); err != nil {
				if errors.IsAlreadyExists(err) {
					logger.Info("Rule already exists (concurrent creation)",
						"namespace", ruleNamespace,
						"name", ruleName,
						"errorType", fmt.Sprintf("%T", err),
						"error", err.Error())

					// Rule was created concurrently, get it and update
					if err := r.Get(ctx, types.NamespacedName{
						Namespace: ruleNamespace,
						Name:      ruleName,
					}, existingRule); err != nil {
						if errors.IsNotFound(err) {
							// Strange situation, try again
							logger.Info("Strange situation: rule reported as existing but not found",
								"namespace", ruleNamespace,
								"name", ruleName)
							continue
						}
						logger.Error(err, "Failed to get existing rule after AlreadyExists error",
							"namespace", ruleNamespace,
							"name", ruleName)
						return "", err
					}

					logger.Info("Found existing rule after AlreadyExists error",
						"namespace", ruleNamespace,
						"name", ruleName,
						"existingUID", existingRule.GetUID(),
						"existingOwnerRefs", existingRule.GetOwnerReferences())

					// Found the rule, break out to update it
					break
				} else if errors.IsConflict(err) {
					// Conflict, wait and retry
					logger.Info("Conflict detected when creating rule",
						"namespace", ruleNamespace,
						"name", ruleName,
						"attempt", i+1,
						"error", err.Error())
					time.Sleep(DefaultRetryInterval)
					continue
				} else {
					// Other error
					logger.Error(err, "Failed to create rule",
						"namespace", ruleNamespace,
						"name", ruleName,
						"errorType", fmt.Sprintf("%T", err))
					return "", err
				}
			} else {
				// Successfully created
				logger.Info("Successfully created rule",
					"namespace", ruleNamespace,
					"name", ruleName)
				return ruleName, nil
			}
		}
	} else if err != nil {
		logger.Error(err, "Error checking if rule exists",
			"namespace", ruleNamespace,
			"name", ruleName,
			"errorType", fmt.Sprintf("%T", err))
		// Error getting the rule
		return "", err
	} else {
		// Правило существует
		logger.Info("Rule exists, will update",
			"namespace", ruleNamespace,
			"name", ruleName,
			"existingUID", existingRule.GetUID(),
			"existingOwnerRefs", existingRule.GetOwnerReferences())
	}

	// Rule exists, update it using patch with retry
	logger.Info("Updating existing IEAgAgRule", "namespace", ruleNamespace, "name", ruleName)

	// Get the latest version of the rule to avoid conflicts
	latestRule := &providerv1alpha1.IEAgAgRule{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: ruleNamespace,
		Name:      ruleName,
	}, latestRule); err != nil {
		logger.Error(err, "Failed to get latest version of rule for update",
			"namespace", ruleNamespace,
			"name", ruleName)
		return "", err
	}

	logger.Info("Got latest version of rule for update",
		"namespace", ruleNamespace,
		"name", ruleName,
		"resourceVersion", latestRule.GetResourceVersion(),
		"uid", latestRule.GetUID())

	// Create a copy for patching
	original := latestRule.DeepCopy()

	// Update the spec
	latestRule.Spec = ruleSpec

	// Логирование перед патчем
	logger.Info("Applying patch to rule",
		"namespace", ruleNamespace,
		"name", ruleName,
		"originalResourceVersion", original.GetResourceVersion(),
		"newResourceVersion", latestRule.GetResourceVersion())

	// Apply patch with retry
	patch := client.MergeFrom(original)
	if err := PatchWithRetry(ctx, r.Client, latestRule, patch, DefaultMaxRetries); err != nil {
		logger.Error(err, "Failed to patch rule",
			"namespace", ruleNamespace,
			"name", ruleName,
			"errorType", fmt.Sprintf("%T", err))
		return "", err
	}

	logger.Info("Successfully patched rule",
		"namespace", ruleNamespace,
		"name", ruleName)

	return ruleName, nil
}

// generateRuleName creates a deterministic rule name based on input parameters
func (r *RuleS2SReconciler) generateRuleName(
	trafficDirection string,
	localAGName string,
	targetAGName string,
	protocol string,
) string {
	// Generate deterministic UUID based on input parameters
	input := fmt.Sprintf("%s-%s-%s-%s",
		strings.ToLower(trafficDirection),
		localAGName,
		targetAGName,
		strings.ToLower(protocol))

	// Нет доступа к логгеру здесь, но мы логируем входные параметры и результат в вызывающей функции

	h := sha256.New()
	h.Write([]byte(input))
	hash := h.Sum(nil)

	// Format first 16 bytes as UUID v5 (8-4-4-4-12 format)
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])

	// Use traffic direction prefix and UUID
	result := fmt.Sprintf("%s-%s",
		strings.ToLower(trafficDirection)[:3],
		uuid)

	return result
}

// getExistingIEAgAgRules returns all IEAgAgRules that have an OwnerReference to the given RuleS2S
func (r *RuleS2SReconciler) getExistingIEAgAgRules(ctx context.Context, ruleS2S *netguardv1alpha1.RuleS2S) ([]providerv1alpha1.IEAgAgRule, error) {
	logger := log.FromContext(ctx)

	// Get all IEAgAgRules across all namespaces
	ieAgAgRuleList := &providerv1alpha1.IEAgAgRuleList{}
	if err := r.List(ctx, ieAgAgRuleList); err != nil {
		logger.Error(err, "Failed to list IEAgAgRules")
		return nil, err
	}

	var relatedRules []providerv1alpha1.IEAgAgRule

	// Check each rule for an OwnerReference to this RuleS2S
	for _, rule := range ieAgAgRuleList.Items {
		for _, ownerRef := range rule.GetOwnerReferences() {
			if ownerRef.UID == ruleS2S.GetUID() &&
				ownerRef.Kind == "RuleS2S" &&
				ownerRef.APIVersion == netguardv1alpha1.GroupVersion.String() {

				// Found a rule that references this RuleS2S
				relatedRules = append(relatedRules, rule)
				break
			}
		}
	}

	logger.Info("Found existing IEAgAg rules",
		"ruleS2S", ruleS2S.Name,
		"count", len(relatedRules))

	return relatedRules, nil
}

// FailedToDeleteRulesError is a custom error type for failed rule deletions
type FailedToDeleteRulesError struct {
	FailedRules []string
}

// Error implements the error interface
func (e *FailedToDeleteRulesError) Error() string {
	return fmt.Sprintf("failed to delete the following IEAgAgRules: %s", strings.Join(e.FailedRules, ", "))
}

// deleteRelatedIEAgAgRules deletes all IEAgAgRules that have an OwnerReference to the given RuleS2S
// Returns a FailedToDeleteRulesError if any rules could not be deleted
func (r *RuleS2SReconciler) deleteRelatedIEAgAgRules(ctx context.Context, ruleS2S *netguardv1alpha1.RuleS2S) error {
	logger := log.FromContext(ctx)

	// Get all IEAgAgRules across all namespaces
	ieAgAgRuleList := &providerv1alpha1.IEAgAgRuleList{}
	if err := r.List(ctx, ieAgAgRuleList); err != nil {
		return err
	}

	logger.Info("Deleting IEAgAgRules")
	logger.Info("Deleting rule", "ruleName", ruleS2S.GetName(), "uuid", ruleS2S.GetUID())
	logger.Info("ieAgAgRuleList", "listNumber", len(ieAgAgRuleList.Items))

	var failedRules []string

	// Check each rule for an OwnerReference to this RuleS2S
	for _, rule := range ieAgAgRuleList.Items {
		for _, ownerRef := range rule.GetOwnerReferences() {
			logger.Info("Checking rule", "Kind", ownerRef.Kind, "uuid", ownerRef.UID)
			if ownerRef.UID == ruleS2S.GetUID() &&
				ownerRef.Kind == "RuleS2S" &&
				ownerRef.APIVersion == netguardv1alpha1.GroupVersion.String() {

				// Found a rule that references this RuleS2S
				logger.Info("Deleting related IEAgAgRule", "name", rule.Name, "namespace", rule.Namespace)

				// First, remove the finalizer if it exists
				ruleCopy := rule.DeepCopy()
				if controllerutil.ContainsFinalizer(ruleCopy, "provider.sgroups.io/finalizer") {
					logger.Info("Removing finalizer from IEAgAgRule", "name", ruleCopy.Name, "namespace", ruleCopy.Namespace)
					controllerutil.RemoveFinalizer(ruleCopy, "provider.sgroups.io/finalizer")

					// Update the rule to remove the finalizer with retry
					if err := UpdateWithRetry(ctx, r.Client, ruleCopy, DefaultMaxRetries); err != nil {
						if !errors.IsNotFound(err) {
							logger.Error(err, "Failed to remove finalizer from IEAgAgRule",
								"name", ruleCopy.Name, "namespace", ruleCopy.Namespace)
							// Continue with deletion attempt even if finalizer removal fails
						}
					}
				}

				// Then delete the rule
				if err := r.Delete(ctx, ruleCopy); err != nil {
					if !errors.IsNotFound(err) {
						logger.Error(err, "Failed to delete related IEAgAgRule",
							"name", ruleCopy.Name, "namespace", ruleCopy.Namespace)
						// Add to failed rules list instead of returning immediately
						failedRules = append(failedRules, fmt.Sprintf("%s/%s", ruleCopy.Namespace, ruleCopy.Name))
					}
				}
			}
		}
	}

	// If any rules failed to delete, return a custom error
	if len(failedRules) > 0 {
		logger.Error(fmt.Errorf("failed to delete some IEAgAgRules"),
			"Some IEAgAgRules could not be deleted",
			"failedRules", strings.Join(failedRules, ", "))
		return &FailedToDeleteRulesError{FailedRules: failedRules}
	}

	return nil
}

// findRuleS2SForService finds all RuleS2S resources that reference a Service through ServiceAlias
func (r *RuleS2SReconciler) findRuleS2SForService(ctx context.Context, obj client.Object) []reconcile.Request {
	service, ok := obj.(*netguardv1alpha1.Service)
	if !ok {
		return nil
	}

	logger := log.FromContext(ctx).WithValues("service", service.Name, "namespace", service.Namespace)
	logger.Info("Finding RuleS2S resources for Service")

	// Find all ServiceAlias objects that reference this Service
	serviceAliasList := &netguardv1alpha1.ServiceAliasList{}
	if err := r.List(ctx, serviceAliasList); err != nil {
		logger.Error(err, "Failed to list ServiceAlias objects")
		return nil
	}

	var requests []reconcile.Request

	// For each ServiceAlias that references this Service
	for _, serviceAlias := range serviceAliasList.Items {
		if serviceAlias.Spec.ServiceRef.GetName() == service.Name &&
			(serviceAlias.Spec.ServiceRef.GetNamespace() == "" ||
				serviceAlias.Spec.ServiceRef.ResolveNamespace(serviceAlias.Namespace) == service.Namespace) {

			// Find all RuleS2S objects that reference this ServiceAlias
			ruleS2SList := &netguardv1alpha1.RuleS2SList{}
			if err := r.List(ctx, ruleS2SList); err != nil {
				logger.Error(err, "Failed to list RuleS2S objects")
				continue
			}

			for _, rule := range ruleS2SList.Items {
				// Check if the rule references this ServiceAlias as local service
				if rule.Spec.ServiceLocalRef.Name == serviceAlias.Name &&
					rule.Namespace == serviceAlias.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      rule.Name,
							Namespace: rule.Namespace,
						},
					})
					logger.Info("Found RuleS2S referencing ServiceAlias as local service",
						"rule", rule.Name, "serviceAlias", serviceAlias.Name)
				}

				// Check if the rule references this ServiceAlias as target service
				targetNamespace := rule.Spec.ServiceRef.ResolveNamespace(rule.Namespace)
				if rule.Spec.ServiceRef.Name == serviceAlias.Name &&
					targetNamespace == serviceAlias.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      rule.Name,
							Namespace: rule.Namespace,
						},
					})
					logger.Info("Found RuleS2S referencing ServiceAlias as target service",
						"rule", rule.Name, "serviceAlias", serviceAlias.Name)
				}
			}
		}
	}

	return requests
}

// findRuleS2SForServiceAlias finds all RuleS2S resources that reference a ServiceAlias
func (r *RuleS2SReconciler) findRuleS2SForServiceAlias(ctx context.Context, obj client.Object) []reconcile.Request {
	serviceAlias, ok := obj.(*netguardv1alpha1.ServiceAlias)
	if !ok {
		return nil
	}

	logger := log.FromContext(ctx).WithValues("serviceAlias", serviceAlias.Name, "namespace", serviceAlias.Namespace)
	logger.Info("Finding RuleS2S resources for ServiceAlias")

	var requests []reconcile.Request

	// Find all RuleS2S objects that reference this ServiceAlias
	ruleS2SList := &netguardv1alpha1.RuleS2SList{}
	if err := r.List(ctx, ruleS2SList); err != nil {
		logger.Error(err, "Failed to list RuleS2S objects")
		return nil
	}

	for _, rule := range ruleS2SList.Items {
		// Check if the rule references this ServiceAlias as local service
		if rule.Spec.ServiceLocalRef.Name == serviceAlias.Name &&
			rule.Namespace == serviceAlias.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rule.Name,
					Namespace: rule.Namespace,
				},
			})
			logger.Info("Found RuleS2S referencing ServiceAlias as local service", "rule", rule.Name)
		}

		// Check if the rule references this ServiceAlias as target service
		targetNamespace := rule.Spec.ServiceRef.ResolveNamespace(rule.Namespace)
		if rule.Spec.ServiceRef.Name == serviceAlias.Name &&
			targetNamespace == serviceAlias.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rule.Name,
					Namespace: rule.Namespace,
				},
			})
			logger.Info("Found RuleS2S referencing ServiceAlias as target service", "rule", rule.Name)
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
//  1. When an AddressGroupBinding is deleted, the AddressGroupBinding controller already updates the Service
//     by removing the AddressGroup from Service.AddressGroups
//  2. This controller is already watching for changes to Service resources, so it will be notified
//     when a Service's AddressGroups are modified
func (r *RuleS2SReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add indexes for faster lookups
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.RuleS2S{}, "spec.serviceLocalRef.name",
		func(obj client.Object) []string {
			rule := obj.(*netguardv1alpha1.RuleS2S)
			return []string{rule.Spec.ServiceLocalRef.Name}
		}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.RuleS2S{}, "spec.serviceRef.name",
		func(obj client.Object) []string {
			rule := obj.(*netguardv1alpha1.RuleS2S)
			return []string{rule.Spec.ServiceRef.Name}
		}); err != nil {
		return err
	}

	// Добавляем составной индекс для быстрого поиска дубликатов
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.RuleS2S{}, "spec.composite",
		func(obj client.Object) []string {
			rule := obj.(*netguardv1alpha1.RuleS2S)
			// Создаем уникальный ключ на основе полей спецификации
			composite := fmt.Sprintf("%s-%s-%s-%s",
				rule.Spec.Traffic,
				rule.Spec.ServiceLocalRef.Name,
				rule.Spec.ServiceRef.Name,
				rule.Spec.ServiceRef.GetNamespace())
			return []string{composite}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&netguardv1alpha1.RuleS2S{}).
		// Watch for changes to Service resources
		Watches(
			&netguardv1alpha1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findRuleS2SForService),
		).
		// Watch for changes to ServiceAlias resources
		Watches(
			&netguardv1alpha1.ServiceAlias{},
			handler.EnqueueRequestsFromMapFunc(r.findRuleS2SForServiceAlias),
		).
		Named("rules2s").
		Complete(r)
}
