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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

var _ = Describe("RuleS2S Webhook", func() {
	// Краткое описание: Тесты для вебхука RuleS2S
	// Детальное описание: Проверяет валидацию создания, обновления и удаления ресурса RuleS2S.
	// Тесты проверяют корректность ссылок на ServiceAlias, существование ссылаемых ресурсов,
	// и невозможность изменения спецификации после создания.
	var (
		obj                *netguardv1alpha1.RuleS2S
		oldObj             *netguardv1alpha1.RuleS2S
		validator          RuleS2SCustomValidator
		ctx                context.Context
		defaultNamespace   string
		otherNamespace     string
		localServiceAlias  *netguardv1alpha1.ServiceAlias
		targetServiceAlias *netguardv1alpha1.ServiceAlias
		fakeClient         client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		defaultNamespace = "default"
		otherNamespace = "other-namespace"

		// Create a scheme with all the required types
		scheme := runtime.NewScheme()
		Expect(netguardv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create local service alias
		localServiceAlias = &netguardv1alpha1.ServiceAlias{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-service-alias",
				Namespace: defaultNamespace,
			},
			Spec: netguardv1alpha1.ServiceAliasSpec{
				ServiceRef: netguardv1alpha1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1alpha1",
					Kind:       "Service",
					Name:       "local-service",
				},
			},
		}

		// Create target service alias
		targetServiceAlias = &netguardv1alpha1.ServiceAlias{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-service-alias",
				Namespace: otherNamespace,
			},
			Spec: netguardv1alpha1.ServiceAliasSpec{
				ServiceRef: netguardv1alpha1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1alpha1",
					Kind:       "Service",
					Name:       "target-service",
				},
			},
		}

		// Create fake client with the objects
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(localServiceAlias, targetServiceAlias).
			Build()

		// Initialize validator with fake client
		validator = RuleS2SCustomValidator{
			Client: fakeClient,
		}

		// Initialize objects
		obj = &netguardv1alpha1.RuleS2S{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule",
				Namespace: defaultNamespace,
			},
			Spec: netguardv1alpha1.RuleS2SSpec{
				Traffic: "ingress",
				ServiceLocalRef: netguardv1alpha1.NamespacedObjectReference{
					ObjectReference: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "ServiceAlias",
						Name:       "local-service-alias",
					},
					Namespace: defaultNamespace,
				},
				ServiceRef: netguardv1alpha1.NamespacedObjectReference{
					ObjectReference: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "ServiceAlias",
						Name:       "target-service-alias",
					},
					Namespace: otherNamespace,
				},
			},
		}

		oldObj = obj.DeepCopy()
	})

	AfterEach(func() {
		// No teardown needed for fake client
	})

	Context("When validating RuleS2S creation", func() {
		// Краткое описание: Тесты валидации при создании RuleS2S
		// Детальное описание: Проверяет различные сценарии создания ресурса, включая корректные ссылки на ServiceAlias
		// и случаи, когда ссылаемые ресурсы не существуют.
		It("Should allow creation with valid references", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent serviceLocalRef", func() {
			obj.Spec.ServiceLocalRef.Name = "non-existent-service-alias"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serviceLocalRef default/non-existent-service-alias does not exist"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent serviceRef", func() {
			obj.Spec.ServiceRef.Name = "non-existent-service-alias"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serviceRef other-namespace/non-existent-service-alias does not exist"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating RuleS2S updates", func() {
		// Краткое описание: Тесты валидации при обновлении RuleS2S
		// Детальное описание: Проверяет, что нельзя изменять спецификацию после создания ресурса,
		// и что валидация пропускается для ресурсов, помеченных на удаление.
		It("Should allow updates without changing spec", func() {
			obj.Labels = map[string]string{"updated": "true"}
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change spec", func() {
			obj.Spec.Traffic = "egress"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec of RuleS2S cannot be changed"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should allow updates for resources being deleted", func() {
			now := metav1.NewTime(time.Now())
			obj.DeletionTimestamp = &now
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating RuleS2S deletion", func() {
		// Краткое описание: Тесты валидации при удалении RuleS2S
		// Детальное описание: Проверяет, что удаление ресурса разрешено без дополнительных проверок.
		It("Should allow deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

})
