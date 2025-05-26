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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
)

var _ = Describe("Service Controller", func() {
	// Краткое описание: Тесты для контроллера Service
	// Детальное описание: Проверяет логику согласования ресурса Service,
	// включая создание зависимых ресурсов, обновление статуса и обработку ошибок.

	Context("При создании сервиса с портами", func() {
		// Краткое описание: Тесты для обработки создания сервиса с портами
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые сервисы с портами, добавляет финализатор и устанавливает статус Ready.

		const serviceName = "test-service-with-ports"
		const testNamespace = "default"

		ctx := context.Background()

		serviceNamespacedName := types.NamespacedName{
			Name:      serviceName,
			Namespace: testNamespace,
		}

		BeforeEach(func() {
			// Create the Service
			By("creating the Service resource with ports")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: testNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service with Ports",
					IngressPorts: []netguardv1alpha1.IngressPort{
						{
							Protocol:    netguardv1alpha1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP",
						},
						{
							Protocol:    netguardv1alpha1.ProtocolTCP,
							Port:        "443",
							Description: "HTTPS",
						},
					},
				},
			}
			err := k8sClient.Get(ctx, serviceNamespacedName, &netguardv1alpha1.Service{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, service)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the service
				existingService := &netguardv1alpha1.Service{}
				Expect(k8sClient.Get(ctx, serviceNamespacedName, existingService)).To(Succeed())

				// Update the service fields
				existingService.Spec = service.Spec

				// Update the service
				Expect(k8sClient.Update(ctx, existingService)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup the Service resource
			By("Cleanup the Service resource")
			service := &netguardv1alpha1.Service{}
			err := k8sClient.Get(ctx, serviceNamespacedName, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}
		})

		It("должен установить статус Ready и добавить финализатор", func() {
			// Краткое описание: Тест успешного создания сервиса с портами
			// Детальное описание: Проверяет, что контроллер успешно создает сервис с портами,
			// добавляет финализатор и устанавливает статус Ready в True

			By("Reconciling the created resource")
			controllerReconciler := &ServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: serviceNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Check that the Service has been updated with the Ready condition
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceNamespacedName, updatedService)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedService.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())

			// Check that the finalizer has been added
			Expect(controllerutil.ContainsFinalizer(updatedService, "service.netguard.sgroups.io/finalizer")).To(BeTrue())
		})
	})

	Context("При создании сервиса с портами и привязке к адресным группам", func() {
		// Краткое описание: Тесты для обработки создания сервиса с портами и привязке к адресным группам
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// сервисы с портами и адресными группами, обновляет маппинги портов.

		const serviceName = "test-service-with-address-groups"
		const addressGroupName = "test-address-group"
		const testNamespace = "default"

		ctx := context.Background()

		serviceNamespacedName := types.NamespacedName{
			Name:      serviceName,
			Namespace: testNamespace,
		}

		addressGroupNamespacedName := types.NamespacedName{
			Name:      addressGroupName,
			Namespace: testNamespace,
		}

		portMappingNamespacedName := types.NamespacedName{
			Name:      addressGroupName, // Port mapping has the same name as the address group
			Namespace: testNamespace,
		}

		BeforeEach(func() {
			// Create the AddressGroup
			By("creating the AddressGroup resource")
			addressGroup := &providerv1alpha1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addressGroupName,
					Namespace: testNamespace,
				},
				Spec: providerv1alpha1.AddressGroupSpec{
					DefaultAction: providerv1alpha1.ActionAccept,
				},
			}
			err := k8sClient.Get(ctx, addressGroupNamespacedName, &providerv1alpha1.AddressGroup{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, addressGroup)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the address group
				existingAddressGroup := &providerv1alpha1.AddressGroup{}
				Expect(k8sClient.Get(ctx, addressGroupNamespacedName, existingAddressGroup)).To(Succeed())

				// Update the address group fields
				existingAddressGroup.Spec = addressGroup.Spec

				// Update the address group
				Expect(k8sClient.Update(ctx, existingAddressGroup)).To(Succeed())
			}

			// Create the AddressGroupPortMapping
			By("creating the AddressGroupPortMapping resource")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addressGroupName,
					Namespace: testNamespace,
				},
				Spec:        netguardv1alpha1.AddressGroupPortMappingSpec{},
				AccessPorts: netguardv1alpha1.AccessPortsSpec{},
			}
			err = k8sClient.Get(ctx, portMappingNamespacedName, &netguardv1alpha1.AddressGroupPortMapping{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, portMapping)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the port mapping
				existingPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
				Expect(k8sClient.Get(ctx, portMappingNamespacedName, existingPortMapping)).To(Succeed())

				// Update the port mapping fields
				existingPortMapping.Spec = portMapping.Spec
				existingPortMapping.AccessPorts = portMapping.AccessPorts

				// Update the port mapping
				Expect(k8sClient.Update(ctx, existingPortMapping)).To(Succeed())
			}

			// Create the Service with address groups
			By("creating the Service resource with ports and address groups")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: testNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service with Ports and Address Groups",
					IngressPorts: []netguardv1alpha1.IngressPort{
						{
							Protocol:    netguardv1alpha1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP",
						},
					},
				},
				AddressGroups: netguardv1alpha1.AddressGroupsSpec{
					Items: []netguardv1alpha1.NamespacedObjectReference{
						{
							ObjectReference: netguardv1alpha1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1alpha1",
								Kind:       "AddressGroup",
								Name:       addressGroupName,
							},
							Namespace: testNamespace,
						},
					},
				},
			}
			err = k8sClient.Get(ctx, serviceNamespacedName, &netguardv1alpha1.Service{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, service)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the service
				existingService := &netguardv1alpha1.Service{}
				Expect(k8sClient.Get(ctx, serviceNamespacedName, existingService)).To(Succeed())

				// Update the service fields
				existingService.Spec = service.Spec
				existingService.AddressGroups = service.AddressGroups

				// Update the service
				Expect(k8sClient.Update(ctx, existingService)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup the Service resource
			By("Cleanup the Service resource")
			service := &netguardv1alpha1.Service{}
			err := k8sClient.Get(ctx, serviceNamespacedName, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Cleanup the AddressGroup resource
			By("Cleanup the AddressGroup resource")
			addressGroup := &providerv1alpha1.AddressGroup{}
			err = k8sClient.Get(ctx, addressGroupNamespacedName, addressGroup)
			if err == nil {
				Expect(k8sClient.Delete(ctx, addressGroup)).To(Succeed())
			}

			// Cleanup the AddressGroupPortMapping resource
			By("Cleanup the AddressGroupPortMapping resource")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, portMappingNamespacedName, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}
		})

		It("должен обновить маппинг портов для адресной группы", func() {
			// Краткое описание: Тест обновления маппинга портов
			// Детальное описание: Проверяет, что контроллер обновляет маппинг портов
			// для адресной группы при создании сервиса с портами и адресными группами

			By("Reconciling the created resource")
			controllerReconciler := &ServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: serviceNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Check that the Service has been updated with the Ready condition
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceNamespacedName, updatedService)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedService.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())

			// Check that the port mapping has been updated with the service's ports
			updatedPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, portMappingNamespacedName, updatedPortMapping)
				if err != nil {
					return false
				}

				// Should have one service reference
				if len(updatedPortMapping.AccessPorts.Items) != 1 {
					return false
				}

				// The service reference should be for our service
				serviceRef := updatedPortMapping.AccessPorts.Items[0]
				if serviceRef.GetName() != serviceName || serviceRef.GetNamespace() != testNamespace {
					return false
				}

				// The service reference should have the correct ports
				if len(serviceRef.Ports.TCP) != 1 || serviceRef.Ports.TCP[0].Port != "80" {
					return false
				}

				return true
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())
		})
	})

	Context("При удалении сервиса", func() {
		// Краткое описание: Тесты для обработки удаления сервиса
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// удаление сервиса, удаляет связанные ресурсы и очищает маппинги портов.

		const serviceName = "test-service-delete"
		const addressGroupName = "test-address-group-delete"
		const bindingName = "test-binding-delete"
		const testNamespace = "default"

		ctx := context.Background()

		serviceNamespacedName := types.NamespacedName{
			Name:      serviceName,
			Namespace: testNamespace,
		}

		addressGroupNamespacedName := types.NamespacedName{
			Name:      addressGroupName,
			Namespace: testNamespace,
		}

		portMappingNamespacedName := types.NamespacedName{
			Name:      addressGroupName, // Port mapping has the same name as the address group
			Namespace: testNamespace,
		}

		bindingNamespacedName := types.NamespacedName{
			Name:      bindingName,
			Namespace: testNamespace,
		}

		BeforeEach(func() {
			// Create the AddressGroup
			By("creating the AddressGroup resource")
			addressGroup := &providerv1alpha1.AddressGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addressGroupName,
					Namespace: testNamespace,
				},
				Spec: providerv1alpha1.AddressGroupSpec{
					DefaultAction: providerv1alpha1.ActionAccept,
				},
			}
			err := k8sClient.Get(ctx, addressGroupNamespacedName, &providerv1alpha1.AddressGroup{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, addressGroup)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the address group
				existingAddressGroup := &providerv1alpha1.AddressGroup{}
				Expect(k8sClient.Get(ctx, addressGroupNamespacedName, existingAddressGroup)).To(Succeed())

				// Update the address group fields
				existingAddressGroup.Spec = addressGroup.Spec

				// Update the address group
				Expect(k8sClient.Update(ctx, existingAddressGroup)).To(Succeed())
			}

			// Create the Service
			By("creating the Service resource")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: testNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service for Deletion",
					IngressPorts: []netguardv1alpha1.IngressPort{
						{
							Protocol:    netguardv1alpha1.ProtocolTCP,
							Port:        "80",
							Description: "HTTP",
						},
					},
				},
				AddressGroups: netguardv1alpha1.AddressGroupsSpec{
					Items: []netguardv1alpha1.NamespacedObjectReference{
						{
							ObjectReference: netguardv1alpha1.ObjectReference{
								APIVersion: "netguard.sgroups.io/v1alpha1",
								Kind:       "AddressGroup",
								Name:       addressGroupName,
							},
							Namespace: testNamespace,
						},
					},
				},
			}
			err = k8sClient.Get(ctx, serviceNamespacedName, &netguardv1alpha1.Service{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, service)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the service
				existingService := &netguardv1alpha1.Service{}
				Expect(k8sClient.Get(ctx, serviceNamespacedName, existingService)).To(Succeed())

				// Update the service fields
				existingService.Spec = service.Spec
				existingService.AddressGroups = service.AddressGroups

				// Update the service
				Expect(k8sClient.Update(ctx, existingService)).To(Succeed())
			}

			// Create the AddressGroupPortMapping
			By("creating the AddressGroupPortMapping resource")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addressGroupName,
					Namespace: testNamespace,
				},
				Spec: netguardv1alpha1.AddressGroupPortMappingSpec{},
				AccessPorts: netguardv1alpha1.AccessPortsSpec{
					Items: []netguardv1alpha1.ServicePortsRef{
						{
							NamespacedObjectReference: netguardv1alpha1.NamespacedObjectReference{
								ObjectReference: netguardv1alpha1.ObjectReference{
									APIVersion: "netguard.sgroups.io/v1alpha1",
									Kind:       "Service",
									Name:       serviceName,
								},
								Namespace: testNamespace,
							},
							Ports: netguardv1alpha1.ProtocolPorts{
								TCP: []netguardv1alpha1.PortConfig{
									{
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
						},
					},
				},
			}
			err = k8sClient.Get(ctx, portMappingNamespacedName, &netguardv1alpha1.AddressGroupPortMapping{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, portMapping)).To(Succeed())
			} else if err == nil {
				// Get the latest version of the port mapping
				existingPortMapping := &netguardv1alpha1.AddressGroupPortMapping{}
				Expect(k8sClient.Get(ctx, portMappingNamespacedName, existingPortMapping)).To(Succeed())

				// Update the port mapping fields
				existingPortMapping.Spec = portMapping.Spec
				existingPortMapping.AccessPorts = portMapping.AccessPorts

				// Update the port mapping
				Expect(k8sClient.Update(ctx, existingPortMapping)).To(Succeed())
			}

			// Create the AddressGroupBinding
			By("creating the AddressGroupBinding resource")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: testNamespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "netguard.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
					},
				},
			}
			err = k8sClient.Get(ctx, bindingNamespacedName, binding)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, binding)).To(Succeed())
			} else if err == nil {
				// Update the binding if it exists
				Expect(k8sClient.Update(ctx, binding)).To(Succeed())
			}

			// Reconcile to add the finalizer
			controllerReconciler := &ServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: serviceNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify that the finalizer was added
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceNamespacedName, updatedService)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updatedService, "service.netguard.sgroups.io/finalizer")
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())
		})

		AfterEach(func() {
			// Ensure all resources are deleted
			By("Cleanup all resources")

			// Cleanup the AddressGroupBinding resource
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, bindingNamespacedName, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			// Cleanup the Service resource
			service := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, serviceNamespacedName, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Cleanup the AddressGroup resource
			addressGroup := &providerv1alpha1.AddressGroup{}
			err = k8sClient.Get(ctx, addressGroupNamespacedName, addressGroup)
			if err == nil {
				Expect(k8sClient.Delete(ctx, addressGroup)).To(Succeed())
			}

			// Cleanup the AddressGroupPortMapping resource
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, portMappingNamespacedName, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}
		})

		It("должен удалить связанные ресурсы и очистить маппинги портов", func() {
			// Краткое описание: Тест удаления сервиса
			// Детальное описание: Проверяет, что контроллер удаляет связанные ресурсы
			// и очищает маппинги портов при удалении сервиса

			// Get the current service
			service := &netguardv1alpha1.Service{}
			err := k8sClient.Get(ctx, serviceNamespacedName, service)
			Expect(err).NotTo(HaveOccurred())

			// Delete the service
			By("Deleting the Service resource")
			Expect(k8sClient.Delete(ctx, service)).To(Succeed())

			// Mark the service for deletion by setting DeletionTimestamp
			now := metav1.Now()
			service.DeletionTimestamp = &now
			Expect(k8sClient.Update(ctx, service)).To(Succeed())

			// Reconcile the deletion
			controllerReconciler := &ServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: serviceNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Check that the AddressGroupBinding has been deleted
			binding := &netguardv1alpha1.AddressGroupBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, bindingNamespacedName, binding)
				return errors.IsNotFound(err)
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())

			// Check that the service has been removed from the port mapping
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, portMappingNamespacedName, portMapping)
				if err != nil {
					return false
				}

				// Should have no service references
				return len(portMapping.AccessPorts.Items) == 0
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())

			// Check that the finalizer has been removed
			updatedService := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, serviceNamespacedName, updatedService)

			// The service might be completely gone, or it might still exist but without the finalizer
			if !errors.IsNotFound(err) {
				Expect(controllerutil.ContainsFinalizer(updatedService, "service.netguard.sgroups.io/finalizer")).To(BeFalse())
			}
		})
	})

	Context("При обработке сервиса без портов", func() {
		// Краткое описание: Тесты для обработки сервиса без портов
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// сервисы без портов, устанавливая статус Ready.

		const serviceName = "test-service-no-ports"
		const testNamespace = "default"

		ctx := context.Background()

		serviceNamespacedName := types.NamespacedName{
			Name:      serviceName,
			Namespace: testNamespace,
		}

		BeforeEach(func() {
			// Create the Service without ports
			By("creating the Service resource without ports")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: testNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description:  "Test Service without Ports",
					IngressPorts: []netguardv1alpha1.IngressPort{},
				},
			}
			err := k8sClient.Get(ctx, serviceNamespacedName, service)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, service)).To(Succeed())
			} else if err == nil {
				// Update the service if it exists
				Expect(k8sClient.Update(ctx, service)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup the Service resource
			By("Cleanup the Service resource")
			service := &netguardv1alpha1.Service{}
			err := k8sClient.Get(ctx, serviceNamespacedName, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}
		})

		It("должен установить статус Ready для сервиса без портов", func() {
			// Краткое описание: Тест обработки сервиса без портов
			// Детальное описание: Проверяет, что контроллер устанавливает статус Ready
			// для сервиса без портов

			By("Reconciling the created resource")
			controllerReconciler := &ServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: serviceNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Check that the Service has been updated with the Ready condition
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceNamespacedName, updatedService)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedService.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, time.Second*30, time.Millisecond*500).Should(BeTrue())

			// Check that the finalizer has been added
			Expect(controllerutil.ContainsFinalizer(updatedService, "service.netguard.sgroups.io/finalizer")).To(BeTrue())
		})
	})
})
