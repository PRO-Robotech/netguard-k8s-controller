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

var _ = Describe("AddressGroupBinding Webhook", func() {
	// Краткое описание: Тесты для вебхука AddressGroupBinding
	// Детальное описание: Проверяет валидацию создания, обновления и удаления ресурса AddressGroupBinding.
	// Тесты проверяют корректность ссылок на Service и AddressGroup, проверку пересечения портов,
	// и правила для кросс-неймспейс привязок.
	var (
		obj              *netguardv1alpha1.AddressGroupBinding
		oldObj           *netguardv1alpha1.AddressGroupBinding
		validator        AddressGroupBindingCustomValidator
		ctx              context.Context
		service          *netguardv1alpha1.Service
		addressGroup     *providerv1alpha1.AddressGroup
		portMapping      *netguardv1alpha1.AddressGroupPortMapping
		bindingPolicy    *netguardv1alpha1.AddressGroupBindingPolicy
		defaultNamespace string
		crossNamespace   string
		serviceRef       netguardv1alpha1.ObjectReference
		addressGroupRef  netguardv1alpha1.NamespacedObjectReference
		fakeClient       client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		defaultNamespace = "default"
		crossNamespace = "other-namespace"

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
					{
						Protocol:    netguardv1alpha1.ProtocolUDP,
						Port:        "53",
						Description: "DNS",
					},
				},
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

		// Create address group in cross namespace
		crossAddressGroup := &providerv1alpha1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cross-address-group",
				Namespace: crossNamespace,
			},
			Spec: providerv1alpha1.AddressGroupSpec{
				DefaultAction: providerv1alpha1.ActionAccept,
			},
		}

		// Create address group in namespace without policy
		namespaceWithoutPolicy := "namespace-without-policy"
		addressGroupWithoutPolicy := &providerv1alpha1.AddressGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-address-group",
				Namespace: namespaceWithoutPolicy,
			},
			Spec: providerv1alpha1.AddressGroupSpec{
				DefaultAction: providerv1alpha1.ActionAccept,
			},
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
									Description: "Existing HTTP",
								},
							},
						},
					},
				},
			},
		}

		// Create cross namespace port mapping
		crossPortMapping := &netguardv1alpha1.AddressGroupPortMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cross-address-group",
				Namespace: crossNamespace,
			},
		}

		// Create binding policy for cross-namespace binding
		bindingPolicy = &netguardv1alpha1.AddressGroupBindingPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-binding-policy",
				Namespace: crossNamespace,
			},
			Spec: netguardv1alpha1.AddressGroupBindingPolicySpec{
				AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
					ObjectReference: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "AddressGroup",
						Name:       "cross-address-group",
					},
				},
				ServiceRef: netguardv1alpha1.NamespacedObjectReference{
					ObjectReference: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       "test-service",
					},
					Namespace: defaultNamespace,
				},
			},
		}

		// Create fake client with the objects
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(service, addressGroup, crossAddressGroup, addressGroupWithoutPolicy, portMapping, crossPortMapping, bindingPolicy).
			Build()

		// Initialize validator with fake client
		validator = AddressGroupBindingCustomValidator{
			Client: fakeClient,
		}

		// Initialize references
		serviceRef = netguardv1alpha1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1alpha1",
			Kind:       "Service",
			Name:       "test-service",
		}

		addressGroupRef = netguardv1alpha1.NamespacedObjectReference{
			ObjectReference: netguardv1alpha1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1alpha1",
				Kind:       "AddressGroup",
				Name:       "test-address-group",
			},
		}

		// Initialize objects
		obj = &netguardv1alpha1.AddressGroupBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-binding",
				Namespace: defaultNamespace,
			},
			Spec: netguardv1alpha1.AddressGroupBindingSpec{
				ServiceRef:      serviceRef,
				AddressGroupRef: addressGroupRef,
			},
		}

		oldObj = obj.DeepCopy()
	})

	AfterEach(func() {
		// No teardown needed for fake client
	})

	Context("When validating AddressGroupBinding creation", func() {
		// Краткое описание: Тесты валидации при создании AddressGroupBinding
		// Детальное описание: Проверяет различные сценарии создания ресурса, включая корректные и некорректные ссылки,
		// несуществующие ресурсы и кросс-неймспейс привязки.
		It("Should allow creation with valid references", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with missing ServiceRef name", func() {
			obj.Spec.ServiceRef.Name = ""
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Service.name cannot be empty"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with invalid ServiceRef kind", func() {
			obj.Spec.ServiceRef.Kind = "InvalidKind"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reference must be to a Service resource"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with missing AddressGroupRef name", func() {
			obj.Spec.AddressGroupRef.Name = ""
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("addressGroupRef.name cannot be empty"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with invalid AddressGroupRef kind", func() {
			obj.Spec.AddressGroupRef.Kind = "InvalidKind"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("addressGroupRef must be to an AddressGroup resource"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent Service", func() {
			obj.Spec.ServiceRef.Name = "non-existent-service"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service non-existent-service not found"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny creation with non-existent AddressGroup", func() {
			obj.Spec.AddressGroupRef.Name = "non-existent-address-group"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("addressGroup non-existent-address-group not found"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should allow cross-namespace binding with policy", func() {
			obj.Spec.AddressGroupRef.Name = "cross-address-group"
			obj.Spec.AddressGroupRef.Namespace = crossNamespace
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny cross-namespace binding without policy", func() {
			obj.Spec.AddressGroupRef.Name = "test-address-group"
			obj.Spec.AddressGroupRef.Namespace = "namespace-without-policy"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cross-namespace binding not allowed"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating AddressGroupBinding updates", func() {
		// Краткое описание: Тесты валидации при обновлении AddressGroupBinding
		// Детальное описание: Проверяет, что нельзя изменять ключевые поля после создания ресурса,
		// и что валидация пропускается для ресурсов, помеченных на удаление.
		It("Should allow updates without changing references", func() {
			obj.Labels = map[string]string{"updated": "true"}
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change ServiceRef", func() {
			obj.Spec.ServiceRef.Name = "another-service"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot change spec.serviceRef.name after creation"))
			Expect(warnings).To(BeEmpty())
		})

		It("Should deny updates that change AddressGroupRef", func() {
			obj.Spec.AddressGroupRef.Name = "another-address-group"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot change spec.addressGroupRef.name after creation"))
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

	Context("When validating AddressGroupBinding deletion", func() {
		// Краткое описание: Тесты валидации при удалении AddressGroupBinding
		// Детальное описание: Проверяет, что удаление ресурса разрешено без дополнительных проверок.
		It("Should allow deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

})
