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
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
)

var _ = Describe("AddressGroupBindingPolicy Webhook", func() {
	// Краткое описание: Тесты для вебхука AddressGroupBindingPolicy
	// Детальное описание: Проверяет валидацию создания, обновления и удаления ресурса AddressGroupBindingPolicy.
	// Тесты проверяют корректность ссылок на Service и AddressGroup, проверку нахождения ресурсов в одном неймспейсе,
	// и невозможность изменения ключевых полей после создания.
	var (
		obj              *netguardv1alpha1.AddressGroupBindingPolicy
		oldObj           *netguardv1alpha1.AddressGroupBindingPolicy
		validator        AddressGroupBindingPolicyCustomValidator
		ctx              context.Context
		service          *netguardv1alpha1.Service
		addressGroup     *providerv1alpha1.AddressGroup
		defaultNamespace string
		otherNamespace   string
		serviceRef       netguardv1alpha1.NamespacedObjectReference
		addressGroupRef  netguardv1alpha1.NamespacedObjectReference
		fakeClient       client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		defaultNamespace = "default"
		otherNamespace = "other-namespace"

		// Create a scheme with all the required types
		scheme := runtime.NewScheme()
		Expect(netguardv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(providerv1alpha1.AddToScheme(scheme)).To(Succeed())

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

		// Create service in other namespace
		otherService := &netguardv1alpha1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-service",
				Namespace: otherNamespace,
			},
			Spec: netguardv1alpha1.ServiceSpec{
				Description: "Other Service",
			},
		}

		// Create address group
		addressGroup = &providerv1alpha1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-address-group",
				Namespace: defaultNamespace,
			},
			Spec: providerv1alpha1.AddressGroupSpec{
				DefaultAction: providerv1alpha1.ActionAccept,
			},
		}

		// Create fake client with the objects
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(service, otherService, addressGroup).
			Build()

		// Initialize validator with fake client
		validator = AddressGroupBindingPolicyCustomValidator{
			Client: fakeClient,
		}

		// Initialize references
		serviceRef = netguardv1alpha1.NamespacedObjectReference{
			ObjectReference: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "Service",
				Name:       "test-service",
			},
			Namespace: defaultNamespace,
		}

		addressGroupRef = netguardv1alpha1.NamespacedObjectReference{
			ObjectReference: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "AddressGroup",
				Name:       "test-address-group",
			},
		}

		// Initialize objects
		obj = &netguardv1alpha1.AddressGroupBindingPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: defaultNamespace,
			},
			Spec: netguardv1alpha1.AddressGroupBindingPolicySpec{
				ServiceRef:      serviceRef,
				AddressGroupRef: addressGroupRef,
			},
		}

		oldObj = obj.DeepCopy()
	})

	AfterEach(func() {
		// No teardown needed for fake client
	})

	Context("When validating AddressGroupBindingPolicy creation", func() {
		// Краткое описание: Тесты валидации при создании AddressGroupBindingPolicy
		// Детальное описание: Проверяет различные сценарии создания ресурса, включая корректные и некорректные ссылки,
		// несуществующие ресурсы и проверку нахождения ресурсов в одном неймспейсе.
		It("Should allow creation with valid references", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with policy in different namespace than address group", func() {
			obj.Spec.AddressGroupRef.Namespace = otherNamespace
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("policy must be created in the same namespace as the referenced address group"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with missing AddressGroupRef name", func() {
			obj.Spec.AddressGroupRef.Name = ""
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AddressGroup.name cannot be empty"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent AddressGroup", func() {
			obj.Spec.AddressGroupRef.Name = "non-existent-address-group"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("addressGroup non-existent-address-group not found"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with missing ServiceRef name", func() {
			obj.Spec.ServiceRef.Name = ""
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serviceRef.name cannot be empty"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent Service", func() {
			obj.Spec.ServiceRef.Name = "non-existent-service"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service non-existent-service not found"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating AddressGroupBindingPolicy updates", func() {
		// Краткое описание: Тесты валидации при обновлении AddressGroupBindingPolicy
		// Детальное описание: Проверяет, что нельзя изменять ключевые поля после создания ресурса,
		// и что валидация пропускается для ресурсов, помеченных на удаление.
		It("Should allow updates without changing references", func() {
			obj.Labels = map[string]string{"updated": "true"}
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change AddressGroupRef", func() {
			obj.Spec.AddressGroupRef.Name = "another-address-group"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot change spec.addressGroupRef.name after creation"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change ServiceRef", func() {
			obj.Spec.ServiceRef.Name = "another-service"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot change spec.serviceRef.name after creation"))
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

	Context("When validating AddressGroupBindingPolicy deletion", func() {
		// Краткое описание: Тесты валидации при удалении AddressGroupBindingPolicy
		// Детальное описание: Проверяет, что удаление ресурса разрешено без дополнительных проверок.
		It("Should allow deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

})
