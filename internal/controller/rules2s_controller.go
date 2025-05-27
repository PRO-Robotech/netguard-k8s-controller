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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	Log    logr.Logger
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
	log := log.FromContext(ctx)
	log.Info("Reconciling RuleS2S", "request", req)

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

	// Check if the resource is being deleted
	if !ruleS2S.DeletionTimestamp.IsZero() {
		// Delete related IEAgAgRules
		if err := r.deleteRelatedIEAgAgRules(ctx, ruleS2S); err != nil {
			log.Error(err, "Failed to delete related IEAgAgRules")
			return ctrl.Result{}, err
		}

		// Resource is being deleted, no need to do anything else
		return ctrl.Result{}, nil
	}

	// Get the ServiceAlias objects
	localServiceAlias := &netguardv1alpha1.ServiceAlias{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: ruleS2S.Namespace,
		Name:      ruleS2S.Spec.ServiceLocalRef.Name,
	}, localServiceAlias); err != nil {
		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceAliasNotFound",
			Message: fmt.Sprintf("Local service alias not found: %v", err),
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	targetServiceAlias := &netguardv1alpha1.ServiceAlias{}
	targetNamespace := ruleS2S.Spec.ServiceRef.ResolveNamespace(ruleS2S.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: targetNamespace,
		Name:      ruleS2S.Spec.ServiceRef.Name,
	}, targetServiceAlias); err != nil {
		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceAliasNotFound",
			Message: fmt.Sprintf("Target service alias not found: %v", err),
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Get the actual Service objects
	localService := &netguardv1alpha1.Service{}
	localServiceNamespace := localServiceAlias.Spec.ServiceRef.ResolveNamespace(localServiceAlias.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: localServiceNamespace,
		Name:      localServiceAlias.Spec.ServiceRef.Name,
	}, localService); err != nil {
		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceNotFound",
			Message: fmt.Sprintf("Local service not found: %v", err),
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	targetService := &netguardv1alpha1.Service{}
	targetServiceNamespace := targetServiceAlias.Spec.ServiceRef.ResolveNamespace(targetServiceAlias.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: targetServiceNamespace,
		Name:      targetServiceAlias.Spec.ServiceRef.Name,
	}, targetService); err != nil {
		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ServiceNotFound",
			Message: fmt.Sprintf("Target service not found: %v", err),
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
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

			if err := r.Update(ctx, targetService); err != nil {
				log.Error(err, "Failed to update target service RuleS2SDstOwnRef")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	} else {
		// For rules in the same namespace, use owner references
		if err := controllerutil.SetControllerReference(targetService, ruleS2S, r.Scheme); err != nil {
			log.Error(err, "Failed to set owner reference")
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
		if err := r.Update(ctx, ruleS2S); err != nil {
			log.Error(err, "Failed to update RuleS2S with owner reference")
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
	}

	// Get address groups from services
	localAddressGroups := localService.AddressGroups.Items
	targetAddressGroups := targetService.AddressGroups.Items

	if len(localAddressGroups) == 0 || len(targetAddressGroups) == 0 {
		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NoAddressGroups",
			Message: "One or both services have no address groups",
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, fmt.Errorf("one or both services have no address groups")
	}

	// Determine which ports to use based on traffic direction
	// In both cases, we use ports from the service that receives the traffic
	var ports []netguardv1alpha1.IngressPort
	if ruleS2S.Spec.Traffic == "ingress" {
		// For ingress, local service is the receiver
		ports = localService.Spec.IngressPorts
	} else {
		// For egress, target service is the receiver
		ports = targetService.Spec.IngressPorts
	}

	if len(ports) == 0 {
		// Update status with error condition
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NoPorts",
			Message: "No ports defined for the service",
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, fmt.Errorf("no ports defined for the service")
	}

	// Create IEAgAgRule resources for each combination of address groups and ports
	createdRules := []string{}
	for _, localAG := range localAddressGroups {
		for _, targetAG := range targetAddressGroups {
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

			// Create TCP rule if there are TCP ports
			if len(tcpPorts) > 0 {
				// Combine all TCP ports into a single comma-separated string
				combinedTcpPorts := strings.Join(tcpPorts, ",")

				// Create or update the rule
				ruleName, err := r.createOrUpdateIEAgAgRule(ctx, ruleS2S, localAG, targetAG,
					netguardv1alpha1.ProtocolTCP, combinedTcpPorts)
				if err != nil {
					log.Error(err, "Failed to create/update TCP rule")
					continue
				}
				createdRules = append(createdRules, ruleName)
			}

			// Create UDP rule if there are UDP ports
			if len(udpPorts) > 0 {
				// Combine all UDP ports into a single comma-separated string
				combinedUdpPorts := strings.Join(udpPorts, ",")

				// Create or update the rule
				ruleName, err := r.createOrUpdateIEAgAgRule(ctx, ruleS2S, localAG, targetAG,
					netguardv1alpha1.ProtocolUDP, combinedUdpPorts)
				if err != nil {
					log.Error(err, "Failed to create/update UDP rule")
					continue
				}
				createdRules = append(createdRules, ruleName)
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
			log.Error(err, "Failed to update RuleS2S status")
			return ctrl.Result{}, err
		}
	} else {
		meta.SetStatusCondition(&ruleS2S.Status.Conditions, metav1.Condition{
			Type:    netguardv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NoRulesCreated",
			Message: "Failed to create any rules",
		})
		if err := UpdateStatusWithRetry(ctx, r.Client, ruleS2S, DefaultMaxRetries); err != nil {
			log.Error(err, "Failed to update RuleS2S status")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, fmt.Errorf("failed to create any rules")
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
	// Determine namespace for the rule based on traffic direction
	var ruleNamespace string
	if ruleS2S.Spec.Traffic == "ingress" {
		// For ingress, rule goes in the local AG namespace (receiver)
		ruleNamespace = localAG.ResolveNamespace(ruleS2S.GetNamespace())
	} else {
		// For egress, rule goes in the target AG namespace (receiver)
		ruleNamespace = targetAG.ResolveNamespace(ruleS2S.GetNamespace())
	}

	// Ensure namespace is not empty
	if ruleNamespace == "" {
		return "", fmt.Errorf("cannot create rule with empty namespace")
	}

	// Generate rule name using the helper function
	ruleName := r.generateRuleName(
		ruleS2S.Name,
		ruleS2S.Spec.Traffic,
		localAG.Name,
		targetAG.Name,
		string(protocol))

	r.Log.Info("Creating rule", "namespace", ruleNamespace, "ruleName", ruleName, "traffic", ruleS2S.Spec.Traffic)

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
		// Rule doesn't exist, create it with retry
		r.Log.Info("Creating new IEAgAgRule", "namespace", ruleNamespace, "name", ruleName)

		// Create the rule
		newRule := &providerv1alpha1.IEAgAgRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ruleName,
				Namespace: ruleNamespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(ruleS2S, netguardv1alpha1.GroupVersion.WithKind("RuleS2S")),
				},
			},
			Spec: ruleSpec,
		}

		// Try to create with retries
		for i := 0; i < DefaultMaxRetries; i++ {
			if err := r.Create(ctx, newRule); err != nil {
				if errors.IsAlreadyExists(err) {
					// Rule was created concurrently, get it and update
					if err := r.Get(ctx, types.NamespacedName{
						Namespace: ruleNamespace,
						Name:      ruleName,
					}, existingRule); err != nil {
						if errors.IsNotFound(err) {
							// Strange situation, try again
							continue
						}
						return "", err
					}
					// Found the rule, break out to update it
					break
				} else if errors.IsConflict(err) {
					// Conflict, wait and retry
					time.Sleep(DefaultRetryInterval)
					continue
				} else {
					// Other error
					return "", err
				}
			} else {
				// Successfully created
				return ruleName, nil
			}
		}
	} else if err != nil {
		// Error getting the rule
		return "", err
	}

	// Rule exists, update it using patch with retry
	r.Log.Info("Updating existing IEAgAgRule", "namespace", ruleNamespace, "name", ruleName)

	// Get the latest version of the rule to avoid conflicts
	latestRule := &providerv1alpha1.IEAgAgRule{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: ruleNamespace,
		Name:      ruleName,
	}, latestRule); err != nil {
		return "", err
	}

	// Create a copy for patching
	original := latestRule.DeepCopy()

	// Update the spec
	latestRule.Spec = ruleSpec

	// Apply patch with retry
	patch := client.MergeFrom(original)
	if err := PatchWithRetry(ctx, r.Client, latestRule, patch, DefaultMaxRetries); err != nil {
		return "", err
	}

	return ruleName, nil
}

// generateRuleName creates a deterministic rule name based on input parameters
func (r *RuleS2SReconciler) generateRuleName(
	ruleName string,
	trafficDirection string,
	localAGName string,
	targetAGName string,
	protocol string,
) string {
	// Generate deterministic UUID based on input parameters
	input := fmt.Sprintf("%s-%s-%s-%s-%s",
		ruleName,
		strings.ToLower(trafficDirection),
		localAGName,
		targetAGName,
		strings.ToLower(protocol))

	h := sha256.New()
	h.Write([]byte(input))
	hash := h.Sum(nil)

	// Format first 16 bytes as UUID v5 (8-4-4-4-12 format)
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])

	// Use traffic direction prefix and UUID
	return fmt.Sprintf("%s-%s",
		strings.ToLower(trafficDirection)[:3],
		uuid)
}

// deleteRelatedIEAgAgRules deletes all IEAgAgRules that have an OwnerReference to the given RuleS2S
func (r *RuleS2SReconciler) deleteRelatedIEAgAgRules(ctx context.Context, ruleS2S *netguardv1alpha1.RuleS2S) error {
	logger := log.FromContext(ctx)

	// Get all IEAgAgRules across all namespaces
	ieAgAgRuleList := &providerv1alpha1.IEAgAgRuleList{}
	if err := r.List(ctx, ieAgAgRuleList); err != nil {
		return err
	}

	// Check each rule for an OwnerReference to this RuleS2S
	for _, rule := range ieAgAgRuleList.Items {
		for _, ownerRef := range rule.GetOwnerReferences() {
			if ownerRef.UID == ruleS2S.UID &&
				ownerRef.Kind == "RuleS2S" &&
				ownerRef.APIVersion == netguardv1alpha1.GroupVersion.String() {

				// Found a rule that references this RuleS2S
				logger.Info("Deleting related IEAgAgRule", "name", rule.Name, "namespace", rule.Namespace)

				// Delete the rule
				if err := r.Delete(ctx, &rule); err != nil {
					if !errors.IsNotFound(err) {
						logger.Error(err, "Failed to delete related IEAgAgRule",
							"name", rule.Name, "namespace", rule.Namespace)
						return err
					}
				}
			}
		}
	}

	return nil
}

// findRuleS2SForService finds all RuleS2S resources that reference a Service through ServiceAlias
func (r *RuleS2SReconciler) findRuleS2SForService(ctx context.Context, obj client.Object) []reconcile.Request {
	service, ok := obj.(*netguardv1alpha1.Service)
	if !ok {
		return nil
	}

	logger := r.Log.WithValues("service", service.Name, "namespace", service.Namespace)
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

	logger := r.Log.WithValues("serviceAlias", serviceAlias.Name, "namespace", serviceAlias.Namespace)
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

// findRuleS2SForAddressGroupBinding finds all RuleS2S resources that may be affected by changes to an AddressGroupBinding
func (r *RuleS2SReconciler) findRuleS2SForAddressGroupBinding(ctx context.Context, obj client.Object) []reconcile.Request {
	binding, ok := obj.(*netguardv1alpha1.AddressGroupBinding)
	if !ok {
		return nil
	}

	logger := r.Log.WithValues("binding", binding.Name, "namespace", binding.Namespace)
	logger.Info("Finding RuleS2S resources for AddressGroupBinding")

	// Get the Service referenced by the binding
	service := &netguardv1alpha1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.ServiceRef.Name,
		Namespace: binding.Namespace,
	}, service); err != nil {
		logger.Error(err, "Failed to get Service referenced by AddressGroupBinding")
		return nil
	}

	// Use the findRuleS2SForService function to find affected RuleS2S resources
	return r.findRuleS2SForService(ctx, service)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RuleS2SReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add indexes for faster lookups
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&netguardv1alpha1.ServiceAlias{}, "spec.serviceRef.name",
		func(obj client.Object) []string {
			serviceAlias := obj.(*netguardv1alpha1.ServiceAlias)
			return []string{serviceAlias.Spec.ServiceRef.Name}
		}); err != nil {
		return err
	}

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
		// Watch for changes to AddressGroupBinding resources
		Watches(
			&netguardv1alpha1.AddressGroupBinding{},
			handler.EnqueueRequestsFromMapFunc(r.findRuleS2SForAddressGroupBinding),
		).
		Named("rules2s").
		Complete(r)
}
