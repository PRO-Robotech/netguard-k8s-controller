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

var _ = Describe("AddressGroupPortMapping Webhook", func() {
	// Краткое описание: Тесты для вебхука AddressGroupPortMapping
	// Детальное описание: Проверяет валидацию создания, обновления и удаления ресурса AddressGroupPortMapping.
	// Тесты проверяют корректность конфигурации портов, отсутствие пересечений портов между сервисами,
	// и невозможность изменения полей после создания.
	var (
		obj        *netguardv1alpha1.AddressGroupPortMapping
		oldObj     *netguardv1alpha1.AddressGroupPortMapping
		validator  AddressGroupPortMappingCustomValidator
		ctx        context.Context
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a scheme with all the required types
		scheme := runtime.NewScheme()
		Expect(netguardv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create fake client
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		// Initialize validator with fake client
		validator = AddressGroupPortMappingCustomValidator{
			Client: fakeClient,
		}

		// Initialize objects
		obj = &netguardv1alpha1.AddressGroupPortMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-port-mapping",
				Namespace: "default",
			},
			Spec: netguardv1alpha1.AddressGroupPortMappingSpec{},
			AccessPorts: netguardv1alpha1.AccessPortsSpec{
				Items: []netguardv1alpha1.ServicePortsRef{
					{
						NamespacedObjectReference: netguardv1alpha1.NamespacedObjectReference{
							ObjectReference: netguardv1alpha1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1alpha1",
								Kind:       "Service",
								Name:       "service-1",
							},
							Namespace: "default",
						},
						Ports: netguardv1alpha1.ProtocolPorts{
							TCP: []netguardv1alpha1.PortConfig{
								{
									Port:        "80",
									Description: "HTTP",
								},
							},
							UDP: []netguardv1alpha1.PortConfig{
								{
									Port:        "53",
									Description: "DNS",
								},
							},
						},
					},
					{
						NamespacedObjectReference: netguardv1alpha1.NamespacedObjectReference{
							ObjectReference: netguardv1alpha1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1alpha1",
								Kind:       "Service",
								Name:       "service-2",
							},
							Namespace: "default",
						},
						Ports: netguardv1alpha1.ProtocolPorts{
							TCP: []netguardv1alpha1.PortConfig{
								{
									Port:        "8080",
									Description: "HTTP Alt",
								},
							},
							UDP: []netguardv1alpha1.PortConfig{
								{
									Port:        "5353",
									Description: "DNS Alt",
								},
							},
						},
					},
				},
			},
		}

		oldObj = obj.DeepCopy()
	})

	AfterEach(func() {
		// No teardown needed for fake client
	})

	Context("When validating AddressGroupPortMapping creation", func() {
		// Краткое описание: Тесты валидации при создании AddressGroupPortMapping
		// Детальное описание: Проверяет различные сценарии создания ресурса, включая корректные и некорректные форматы портов,
		// пересечения портов между разными сервисами и допустимость пересечения портов для одного сервиса.
		It("Should allow creation with valid port configuration", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with invalid port format", func() {
			obj.AccessPorts.Items[0].Ports.TCP[0].Port = "invalid"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid TCP port"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with overlapping TCP ports", func() {
			obj.AccessPorts.Items[1].Ports.TCP[0].Port = "80"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TCP port range 80 for service service-2 overlaps with existing port range"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with overlapping UDP ports", func() {
			obj.AccessPorts.Items[1].Ports.UDP[0].Port = "53"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("UDP port range 53 for service service-2 overlaps with existing port range"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should allow creation with overlapping ports for the same service", func() {
			// Add another port to service-1 that overlaps with its existing port
			obj.AccessPorts.Items[0].Ports.TCP = append(obj.AccessPorts.Items[0].Ports.TCP, netguardv1alpha1.PortConfig{
				Port:        "80",
				Description: "HTTP Duplicate",
			})
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating AddressGroupPortMapping updates", func() {
		// Краткое описание: Тесты валидации при обновлении AddressGroupPortMapping
		// Детальное описание: Проверяет, что нельзя изменять поля после создания ресурса,
		// и что валидация пропускается для ресурсов, помеченных на удаление.
		It("Should allow updates without changing spec", func() {
			obj.Labels = map[string]string{"updated": "true"}
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change accessPorts", func() {
			// Clear the AccessPorts to make it different from the original
			obj.AccessPorts = netguardv1alpha1.AccessPortsSpec{
				Items: []netguardv1alpha1.ServicePortsRef{},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("accessPorts of AddressGroupPortMapping cannot be changed"))
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

	Context("When validating AddressGroupPortMapping deletion", func() {
		// Краткое описание: Тесты валидации при удалении AddressGroupPortMapping
		// Детальное описание: Проверяет, что удаление ресурса разрешено без дополнительных проверок.
		It("Should allow deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

})
