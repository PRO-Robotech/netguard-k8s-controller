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

var _ = Describe("ServiceAlias Webhook", func() {
	// Краткое описание: Тесты для вебхука ServiceAlias
	// Детальное описание: Проверяет валидацию создания, обновления и удаления ресурса ServiceAlias.
	// Тесты проверяют корректность ссылок на Service, существование ссылаемого сервиса,
	// и невозможность изменения спецификации после создания.
	var (
		obj              *netguardv1alpha1.ServiceAlias
		oldObj           *netguardv1alpha1.ServiceAlias
		validator        ServiceAliasCustomValidator
		ctx              context.Context
		defaultNamespace string
		service          *netguardv1alpha1.Service
		fakeClient       client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		defaultNamespace = "default"

		// Create a scheme with all the required types
		scheme := runtime.NewScheme()
		Expect(netguardv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create service
		service = &netguardv1alpha1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: defaultNamespace,
			},
			Spec: netguardv1alpha1.ServiceSpec{
				Description: "Test Service",
				IngressPorts: []netguardv1alpha1.IngressPort{
					{
						Protocol:    netguardv1alpha1.ProtocolTCP,
						Port:        "80",
						Description: "HTTP",
					},
				},
			},
		}

		// Create fake client with the objects
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(service).
			Build()

		// Initialize validator with fake client
		validator = ServiceAliasCustomValidator{
			Client: fakeClient,
		}

		// Initialize objects
		obj = &netguardv1alpha1.ServiceAlias{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service-alias",
				Namespace: defaultNamespace,
			},
			Spec: netguardv1alpha1.ServiceAliasSpec{
				ServiceRef: netguardv1alpha1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1alpha1",
					Kind:       "Service",
					Name:       "test-service",
				},
			},
		}

		oldObj = obj.DeepCopy()
	})

	AfterEach(func() {
		// No teardown needed for fake client
	})

	Context("When validating ServiceAlias creation", func() {
		// Краткое описание: Тесты валидации при создании ServiceAlias
		// Детальное описание: Проверяет различные сценарии создания ресурса, включая корректные ссылки на сервис
		// и случаи, когда ссылаемый сервис не существует.
		It("Should allow creation with valid service reference", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent service", func() {
			obj.Spec.ServiceRef.Name = "non-existent-service"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced Service does not exist"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating ServiceAlias updates", func() {
		// Краткое описание: Тесты валидации при обновлении ServiceAlias
		// Детальное описание: Проверяет, что нельзя изменять спецификацию после создания ресурса,
		// и что валидация пропускается для ресурсов, помеченных на удаление.
		It("Should allow updates without changing spec", func() {
			obj.Labels = map[string]string{"updated": "true"}
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change spec", func() {
			obj.Spec.ServiceRef.Name = "another-service"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec of ServiceAlias cannot be changed"))
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

	Context("When validating ServiceAlias deletion", func() {
		// Краткое описание: Тесты валидации при удалении ServiceAlias
		// Детальное описание: Проверяет, что удаление ресурса разрешено без дополнительных проверок.
		It("Should allow deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

})
