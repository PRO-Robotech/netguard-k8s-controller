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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

var _ = Describe("RuleS2S Controller", func() {
	Context("When generating rule names", func() {
		It("should generate consistent names for the same input", func() {
			reconciler := &RuleS2SReconciler{}

			// Generate name twice with the same input
			name1 := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
			name2 := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")

			// Names should be identical
			Expect(name1).To(Equal(name2))
		})

		It("should handle different traffic directions", func() {
			reconciler := &RuleS2SReconciler{}

			// Generate names for ingress and egress
			ingressName := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
			egressName := reconciler.generateRuleName("test-rule", "egress", "local-ag", "target-ag", "TCP")

			// Names should be different
			Expect(ingressName).NotTo(Equal(egressName))

			// Ingress name should start with "ing-"
			Expect(ingressName).To(HavePrefix("ing-"))

			// Egress name should start with "egr-"
			Expect(egressName).To(HavePrefix("egr-"))
		})

		It("should handle different protocols", func() {
			reconciler := &RuleS2SReconciler{}

			// Generate names for TCP and UDP
			tcpName := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
			udpName := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "UDP")

			// Names should be different
			Expect(tcpName).NotTo(Equal(udpName))
		})

		It("should handle very long address group names", func() {
			reconciler := &RuleS2SReconciler{}

			// Create very long address group names (over 63 characters)
			longLocalAGName := "very-long-local-address-group-name-that-exceeds-kubernetes-name-limit"
			longTargetAGName := "very-long-target-address-group-name-that-exceeds-kubernetes-name-limit"

			// Generate name with long address group names
			name := reconciler.generateRuleName("test-rule", "ingress", longLocalAGName, longTargetAGName, "TCP")

			// Name should be 63 characters or less (Kubernetes name limit)
			Expect(len(name)).To(BeNumerically("<=", 63))
		})

		It("should generate different names for different inputs", func() {
			reconciler := &RuleS2SReconciler{}

			// Generate names with different inputs
			name1 := reconciler.generateRuleName("rule1", "ingress", "local-ag", "target-ag", "TCP")
			name2 := reconciler.generateRuleName("rule2", "ingress", "local-ag", "target-ag", "TCP")
			name3 := reconciler.generateRuleName("rule1", "ingress", "different-local-ag", "target-ag", "TCP")
			name4 := reconciler.generateRuleName("rule1", "ingress", "local-ag", "different-target-ag", "TCP")

			// All names should be different
			Expect(name1).NotTo(Equal(name2))
			Expect(name1).NotTo(Equal(name3))
			Expect(name1).NotTo(Equal(name4))
			Expect(name2).NotTo(Equal(name3))
			Expect(name2).NotTo(Equal(name4))
			Expect(name3).NotTo(Equal(name4))
		})
	})
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		rules2s := &netguardv1alpha1.RuleS2S{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind RuleS2S")
			err := k8sClient.Get(ctx, typeNamespacedName, rules2s)
			if err != nil && errors.IsNotFound(err) {
				resource := &netguardv1alpha1.RuleS2S{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &netguardv1alpha1.RuleS2S{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RuleS2S")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &RuleS2SReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
