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

var _ = Describe("Service Webhook", func() {
	// Краткое описание: Тесты для вебхука Service
	// Детальное описание: Проверяет валидацию создания, обновления и удаления ресурса Service.
	// Тесты проверяют корректность конфигурации портов, включая форматы портов, диапазоны портов,
	// и проверку на пересечение портов с существующими сервисами.
	var (
		obj              *netguardv1alpha1.Service
		oldObj           *netguardv1alpha1.Service
		validator        ServiceCustomValidator
		ctx              context.Context
		defaultNamespace string
		addressGroup     *netguardv1alpha1.NamespacedObjectReference
		portMapping      *netguardv1alpha1.AddressGroupPortMapping
		fakeClient       client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		defaultNamespace = "default"

		// Create a scheme with all the required types
		scheme := runtime.NewScheme()
		Expect(netguardv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create address group reference
		addressGroup = &netguardv1alpha1.NamespacedObjectReference{
			ObjectReference: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "AddressGroup",
				Name:       "test-address-group",
			},
			Namespace: defaultNamespace,
		}

		// Create port mapping
		portMapping = &netguardv1alpha1.AddressGroupPortMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-address-group",
				Namespace: defaultNamespace,
			},
			AccessPorts: netguardv1alpha1.AccessPortsSpec{
				Items: []netguardv1alpha1.ServicePortsRef{
					{
						NamespacedObjectReference: netguardv1alpha1.NamespacedObjectReference{
							ObjectReference: netguardv1alpha1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1alpha1",
								Kind:       "Service",
								Name:       "existing-service",
							},
							Namespace: defaultNamespace,
						},
						Ports: netguardv1alpha1.ProtocolPorts{
							TCP: []netguardv1alpha1.PortConfig{
								{
									Port:        "8080",
									Description: "HTTP",
								},
							},
						},
					},
				},
			},
		}

		// Create fake client with the objects
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(portMapping).
			Build()

		// Initialize validator with fake client
		validator = ServiceCustomValidator{
			Client: fakeClient,
		}

		// Initialize objects
		obj = &netguardv1alpha1.Service{
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
					{
						Protocol:    netguardv1alpha1.ProtocolUDP,
						Port:        "53",
						Description: "DNS",
					},
				},
			},
			AddressGroups: netguardv1alpha1.AddressGroupsSpec{
				Items: []netguardv1alpha1.NamespacedObjectReference{
					*addressGroup,
				},
			},
		}

		oldObj = obj.DeepCopy()
	})

	AfterEach(func() {
		// No teardown needed for fake client
	})

	Context("When validating Service creation", func() {
		// Краткое описание: Тесты валидации при создании Service
		// Детальное описание: Проверяет различные сценарии создания ресурса, включая корректные и некорректные форматы портов,
		// порты вне допустимого диапазона, и корректные диапазоны портов.
		It("Should allow creation with valid ports", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with invalid port format", func() {
			obj.Spec.IngressPorts[0].Port = "invalid"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid port"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with port out of range", func() {
			obj.Spec.IngressPorts[0].Port = "70000"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("port must be between 0 and 65535"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should allow creation with valid port range", func() {
			obj.Spec.IngressPorts[0].Port = "8000-9000"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with invalid port range", func() {
			obj.Spec.IngressPorts[0].Port = "9000-8000"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("start port must be less than or equal to end port"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating Service updates", func() {
		// Краткое описание: Тесты валидации при обновлении Service
		// Детальное описание: Проверяет, что обновления с корректными портами разрешены,
		// обновления с некорректными портами запрещены, и что валидация пропускается для ресурсов, помеченных на удаление.
		It("Should allow updates with valid ports", func() {
			obj.Spec.Description = "Updated description"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates with invalid port format", func() {
			obj.Spec.IngressPorts[0].Port = "invalid"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid port"))
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

	Context("When validating Service deletion", func() {
		// Краткое описание: Тесты валидации при удалении Service
		// Детальное описание: Проверяет, что удаление ресурса разрешено без дополнительных проверок.
		It("Should allow deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

})
