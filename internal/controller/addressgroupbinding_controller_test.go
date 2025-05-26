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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
	providerv1alpha1 "sgroups.io/netguard/deps/apis/sgroups-k8s-provider/v1alpha1"
)

// createMockAddressGroup создает мок объекта AddressGroup для тестирования
func createMockAddressGroup(name, namespace string) *providerv1alpha1.AddressGroup {
	return &providerv1alpha1.AddressGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "provider.sgroups.io/v1alpha1",
			Kind:       "AddressGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: providerv1alpha1.AddressGroupSpec{
			DefaultAction: providerv1alpha1.ActionAccept,
		},
	}
}

// createMockClient создает мок-клиент, который возвращает мок AddressGroup при запросе
func createMockClient() *MockClient {
	mockClient := &MockClient{
		Client: k8sClient,
	}

	// Устанавливаем функцию Get, которая будет возвращать мок AddressGroup при запросе
	mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		// Проверяем, является ли запрашиваемый объект AddressGroup
		addressGroup, isAddressGroup := obj.(*providerv1alpha1.AddressGroup)
		if isAddressGroup {
			// Создаем мок AddressGroup и копируем его в запрашиваемый объект
			mockAG := createMockAddressGroup(key.Name, key.Namespace)
			addressGroup.TypeMeta = mockAG.TypeMeta
			addressGroup.ObjectMeta = mockAG.ObjectMeta
			addressGroup.Spec = mockAG.Spec
			addressGroup.Status = mockAG.Status
			addressGroup.Networks = mockAG.Networks
			return nil
		}

		// Для всех остальных объектов используем реальный клиент
		return k8sClient.Get(ctx, key, obj)
	}

	return mockClient
}

var _ = Describe("AddressGroupBinding Controller", func() {
	// Краткое описание: Тесты для контроллера AddressGroupBinding
	// Детальное описание: Проверяет логику согласования ресурса AddressGroupBinding,
	// включая создание связей между Service и AddressGroup, обновление маппингов портов,
	// обработку кросс-namespace ссылок и обработку ошибок.

	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	ctx := context.Background()

	// Создаем контроллер для использования в тестах
	var addressGroupBindingReconciler *AddressGroupBindingReconciler

	BeforeEach(func() {
		// Создаем мок-клиент, который будет возвращать мок AddressGroup при запросе
		mockClient := createMockClient()

		// Инициализируем контроллер с мок-клиентом
		addressGroupBindingReconciler = &AddressGroupBindingReconciler{
			Client: mockClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("При создании AddressGroupBinding с корректными ссылками на Service и AddressGroup в том же namespace", func() {
		// Краткое описание: Тесты для обработки создания AddressGroupBinding с корректными ссылками
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые AddressGroupBinding с ссылками на существующие Service и AddressGroup в том же namespace,
		// добавляет финализатор, обновляет Service.AddressGroups, AddressGroupPortMapping и устанавливает статус Ready.

		// Генерируем уникальные имена для каждого теста
		var serviceName, addressGroupName, bindingName string
		var namespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			addressGroupName = fmt.Sprintf("test-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			namespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
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
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Примечание: AddressGroup не создается, т.к. используется мок

			// Создаем AddressGroupBinding
			By("Создание AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
						// Не указываем Namespace, т.к. используем тот же namespace
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: namespace,
			}, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: namespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Примечание: AddressGroup не удаляется, т.к. используется мок

			// Удаление AddressGroupPortMapping, если оно было создано
			By("Удаление AddressGroupPortMapping")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      addressGroupName, // Имя совпадает с именем AddressGroup
				Namespace: namespace,
			}, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен установить статус Ready, добавить финализатор, обновить Service.AddressGroups и AddressGroupPortMapping", func() {
			// Краткое описание: Тест успешного создания AddressGroupBinding
			// Детальное описание: Проверяет, что контроллер успешно создает AddressGroupBinding,
			// добавляет финализатор, обновляет Service.AddressGroups, AddressGroupPortMapping
			// и устанавливает статус Ready в True

			By("Запуск согласования (reconcile)")
			_, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что AddressGroupBinding получил статус Ready
			updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, updatedBinding)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedBinding.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что финализатор добавлен
			Expect(controllerutil.ContainsFinalizer(updatedBinding, "addressgroupbinding.netguard.sgroups.io/finalizer")).To(BeTrue())

			// Проверяем, что AddressGroup добавлен в Service.AddressGroups
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}, updatedService)
				if err != nil {
					return false
				}

				for _, ag := range updatedService.AddressGroups.Items {
					if ag.GetName() == addressGroupName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что создан AddressGroupPortMapping и в него добавлен Service
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      addressGroupName, // Имя совпадает с именем AddressGroup
					Namespace: namespace,
				}, portMapping)
				if err != nil {
					return false
				}

				for _, sp := range portMapping.AccessPorts.Items {
					if sp.GetName() == serviceName {
						// Проверяем, что порты добавлены корректно
						if len(sp.Ports.TCP) > 0 && sp.Ports.TCP[0].Port == "80" {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("При создании AddressGroupBinding с ссылкой на AddressGroup в другом namespace", func() {
		// Краткое описание: Тесты для обработки создания AddressGroupBinding с ссылкой на AddressGroup в другом namespace
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые AddressGroupBinding с ссылкой на AddressGroup в другом namespace,
		// добавляет финализатор, обновляет Service.AddressGroups, AddressGroupPortMapping и устанавливает статус Ready.

		// Генерируем уникальные имена для каждого теста
		var serviceName, addressGroupName, bindingName string
		var serviceNamespace, addressGroupNamespace, crossNamespaceName string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			addressGroupName = fmt.Sprintf("test-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			crossNamespaceName = fmt.Sprintf("test-cross-ns-%s", uid)
			serviceNamespace = "default"
			addressGroupNamespace = crossNamespaceName

			// Создаем второй namespace для теста
			By("Создание второго namespace")
			crossNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: crossNamespaceName,
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crossNamespaceName}, &corev1.Namespace{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, crossNamespace)).To(Succeed())
			}

			// Создаем Service в основном namespace
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: serviceNamespace,
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
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Примечание: AddressGroup не создается, т.к. используется мок

			// Создаем AddressGroupBinding с ссылкой на AddressGroup в другом namespace
			By("Создание AddressGroupBinding с ссылкой на AddressGroup в другом namespace")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
						Namespace: addressGroupNamespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: serviceNamespace,
			}, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: serviceNamespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Примечание: AddressGroup не удаляется, т.к. используется мок

			// Удаление AddressGroupPortMapping, если оно было создано
			By("Удаление AddressGroupPortMapping")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      addressGroupName,
				Namespace: addressGroupNamespace,
			}, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}

			By("Удаление namespace")
			namespace := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: crossNamespaceName}, namespace)
			if err == nil {
				Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен установить статус Ready, добавить финализатор, обновить Service.AddressGroups и AddressGroupPortMapping в другом namespace", func() {
			// Краткое описание: Тест успешного создания AddressGroupBinding с ссылкой на AddressGroup в другом namespace
			// Детальное описание: Проверяет, что контроллер успешно создает AddressGroupBinding с ссылкой на AddressGroup в другом namespace,
			// добавляет финализатор, обновляет Service.AddressGroups, AddressGroupPortMapping в другом namespace
			// и устанавливает статус Ready в True

			By("Запуск согласования (reconcile)")
			_, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что AddressGroupBinding получил статус Ready
			updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: serviceNamespace,
				}, updatedBinding)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedBinding.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что финализатор добавлен
			Expect(controllerutil.ContainsFinalizer(updatedBinding, "addressgroupbinding.netguard.sgroups.io/finalizer")).To(BeTrue())

			// Проверяем, что AddressGroup добавлен в Service.AddressGroups с указанием namespace
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: serviceNamespace,
				}, updatedService)
				if err != nil {
					return false
				}

				for _, ag := range updatedService.AddressGroups.Items {
					if ag.GetName() == addressGroupName && ag.GetNamespace() == addressGroupNamespace {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что создан AddressGroupPortMapping в другом namespace и в него добавлен Service
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      addressGroupName,
					Namespace: addressGroupNamespace,
				}, portMapping)
				if err != nil {
					return false
				}

				for _, sp := range portMapping.AccessPorts.Items {
					if sp.GetName() == serviceName && sp.GetNamespace() == serviceNamespace {
						// Проверяем, что порты добавлены корректно
						if len(sp.Ports.TCP) > 0 && sp.Ports.TCP[0].Port == "80" {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("При создании AddressGroupBinding с некорректной ссылкой на несуществующий Service", func() {
		// Краткое описание: Тесты для обработки создания AddressGroupBinding с некорректной ссылкой на Service
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые AddressGroupBinding с ссылкой на несуществующий Service,
		// устанавливает статус NotReady с соответствующей причиной.

		// Генерируем уникальные имена для каждого теста
		var nonExistentServiceName, addressGroupName, bindingName string
		var namespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			nonExistentServiceName = fmt.Sprintf("non-existent-service-%s", uid)
			addressGroupName = fmt.Sprintf("test-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			namespace = "default"

			// Примечание: AddressGroup не создается, т.к. используется мок

			// Создаем AddressGroupBinding с ссылкой на несуществующий Service
			By("Создание AddressGroupBinding с ссылкой на несуществующий Service")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       nonExistentServiceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: namespace,
			}, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			// Примечание: AddressGroup не удаляется, т.к. используется мок

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен установить статус NotReady с причиной ServiceNotFound", func() {
			// Краткое описание: Тест обработки AddressGroupBinding с ссылкой на несуществующий Service
			// Детальное описание: Проверяет, что контроллер устанавливает статус NotReady
			// с причиной ServiceNotFound при ссылке на несуществующий Service

			By("Запуск согласования (reconcile)")
			result, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})

			// Ожидаем, что reconcile завершится без ошибки, но с запросом на повторное выполнение
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Проверяем, что AddressGroupBinding получил статус NotReady с причиной ServiceNotFound
			updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, updatedBinding)
				if err != nil {
					return false
				}

				for _, condition := range updatedBinding.Status.Conditions {
					if condition.Type == netguardv1alpha1.ConditionReady &&
						condition.Status == metav1.ConditionFalse &&
						condition.Reason == "ServiceNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что финализатор добавлен, несмотря на ошибку
			Expect(controllerutil.ContainsFinalizer(updatedBinding, "addressgroupbinding.netguard.sgroups.io/finalizer")).To(BeTrue())
		})
	})

	Context("При создании AddressGroupBinding с некорректной ссылкой на несуществующий AddressGroup", func() {
		// Краткое описание: Тесты для обработки создания AddressGroupBinding с некорректной ссылкой на AddressGroup
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые AddressGroupBinding с ссылкой на несуществующий AddressGroup,
		// устанавливает статус NotReady с соответствующей причиной.

		// Генерируем уникальные имена для каждого теста
		var serviceName, nonExistentAddressGroupName, bindingName string
		var namespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			nonExistentAddressGroupName = fmt.Sprintf("non-existent-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			namespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
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
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Создаем AddressGroupBinding с ссылкой на несуществующий AddressGroup
			By("Создание AddressGroupBinding с ссылкой на несуществующий AddressGroup")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       nonExistentAddressGroupName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: namespace,
			}, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: namespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен установить статус NotReady с причиной AddressGroupNotFound", func() {
			// Краткое описание: Тест обработки AddressGroupBinding с ссылкой на несуществующий AddressGroup
			// Детальное описание: Проверяет, что контроллер устанавливает статус NotReady
			// с причиной AddressGroupNotFound при ссылке на несуществующий AddressGroup

			By("Запуск согласования (reconcile)")
			result, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})

			// Ожидаем, что reconcile завершится без ошибки, но с запросом на повторное выполнение
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Проверяем, что AddressGroupBinding получил статус NotReady с причиной AddressGroupNotFound
			updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, updatedBinding)
				if err != nil {
					return false
				}

				for _, condition := range updatedBinding.Status.Conditions {
					if condition.Type == netguardv1alpha1.ConditionReady &&
						condition.Status == metav1.ConditionFalse &&
						condition.Reason == "AddressGroupNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что финализатор добавлен, несмотря на ошибку
			Expect(controllerutil.ContainsFinalizer(updatedBinding, "addressgroupbinding.netguard.sgroups.io/finalizer")).To(BeTrue())
		})
	})

	Context("При удалении AddressGroupBinding", func() {
		// Краткое описание: Тесты для обработки удаления AddressGroupBinding
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// удаление AddressGroupBinding, удаляя связи между Service и AddressGroup,
		// обновляя AddressGroupPortMapping и удаляя финализатор.

		// Генерируем уникальные имена для каждого теста
		var serviceName, addressGroupName, bindingName string
		var namespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			addressGroupName = fmt.Sprintf("test-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			namespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
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
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Примечание: AddressGroup не создается, т.к. используется мок

			// Создаем AddressGroupBinding
			By("Создание AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())

			// Запускаем reconcile для установки связей
			_, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Ждем, пока AddressGroupBinding получит статус Ready и финализатор
			Eventually(func() bool {
				updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, updatedBinding)
				if err != nil {
					return false
				}

				// Проверяем наличие финализатора
				if !controllerutil.ContainsFinalizer(updatedBinding, "addressgroupbinding.netguard.sgroups.io/finalizer") {
					return false
				}

				// Проверяем статус Ready
				return meta.IsStatusConditionTrue(updatedBinding.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что AddressGroup добавлен в Service.AddressGroups
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}, updatedService)
				if err != nil {
					return false
				}

				for _, ag := range updatedService.AddressGroups.Items {
					if ag.GetName() == addressGroupName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что создан AddressGroupPortMapping и в него добавлен Service
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      addressGroupName,
					Namespace: namespace,
				}, portMapping)
				if err != nil {
					return false
				}

				for _, sp := range portMapping.AccessPorts.Items {
					if sp.GetName() == serviceName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: namespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Примечание: AddressGroup не удаляется, т.к. используется мок

			// Удаление AddressGroupPortMapping, если оно было создано
			By("Удаление AddressGroupPortMapping")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      addressGroupName,
				Namespace: namespace,
			}, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}, &netguardv1alpha1.Service{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен удалить связи между Service и AddressGroup, обновить AddressGroupPortMapping и удалить финализатор", func() {
			// Краткое описание: Тест удаления AddressGroupBinding
			// Детальное описание: Проверяет, что контроллер корректно обрабатывает удаление AddressGroupBinding,
			// удаляя связи между Service и AddressGroup, обновляя AddressGroupPortMapping и удаляя финализатор

			// Получаем текущий AddressGroupBinding
			binding := &netguardv1alpha1.AddressGroupBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: namespace,
			}, binding)).To(Succeed())

			// Удаляем AddressGroupBinding
			By("Удаление AddressGroupBinding")
			Expect(k8sClient.Delete(ctx, binding)).To(Succeed())

			// Получаем обновленный AddressGroupBinding с DeletionTimestamp
			updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, updatedBinding)
				if err != nil {
					return false
				}
				return !updatedBinding.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			// Запускаем reconcile для обработки удаления
			_, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что AddressGroupBinding удален
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что AddressGroup удален из Service.AddressGroups
			updatedService := &netguardv1alpha1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: namespace,
				}, updatedService)
				if err != nil {
					return false
				}

				for _, ag := range updatedService.AddressGroups.Items {
					if ag.GetName() == addressGroupName {
						return false // AddressGroup все еще присутствует
					}
				}
				return true // AddressGroup удален
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что Service удален из AddressGroupPortMapping
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      addressGroupName,
					Namespace: namespace,
				}, portMapping)
				if err != nil {
					if errors.IsNotFound(err) {
						// Если AddressGroupPortMapping удален полностью, это тоже успех
						return true
					}
					return false
				}

				for _, sp := range portMapping.AccessPorts.Items {
					if sp.GetName() == serviceName {
						return false // Service все еще присутствует
					}
				}
				return true // Service удален
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("При обработке конфликтов при обновлении", func() {
		// Краткое описание: Тесты для обработки конфликтов при обновлении
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// конфликты при обновлении ресурсов, используя механизм повторных попыток.

		// Генерируем уникальные имена для каждого теста
		var serviceName, addressGroupName, bindingName string
		var namespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			addressGroupName = fmt.Sprintf("test-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			namespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
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
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Примечание: AddressGroup не создается, т.к. используется мок

			// Создаем AddressGroupBinding
			By("Создание AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: namespace,
			}, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: namespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Примечание: AddressGroup не удаляется, т.к. используется мок

			// Удаление AddressGroupPortMapping, если оно было создано
			By("Удаление AddressGroupPortMapping")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      addressGroupName,
				Namespace: namespace,
			}, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен использовать механизм повторных попыток при конфликтах", func() {
			// Краткое описание: Тест обработки конфликтов при обновлении
			// Детальное описание: Проверяет, что контроллер использует механизм
			// повторных попыток при конфликтах обновления

			// Создаем мок-клиент, который будет возвращать ошибку конфликта при первом вызове
			// и успех при последующих вызовах
			mockClient := &MockClient{
				Client:      k8sClient,
				updateCount: 0,
			}

			// Устанавливаем функцию обновления, которая будет возвращать ошибку конфликта при первом вызове
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				// Возвращаем ошибку конфликта при первом вызове
				if mockClient.updateCount == 0 {
					mockClient.updateCount++
					return errors.NewConflict(
						schema.GroupResource{
							Group:    obj.GetObjectKind().GroupVersionKind().Group,
							Resource: obj.GetObjectKind().GroupVersionKind().Kind,
						},
						obj.GetName(),
						fmt.Errorf("conflict error"))
				}
				// При последующих вызовах используем реальный клиент
				return k8sClient.Update(ctx, obj, opts...)
			}

			// Создаем контроллер с мок-клиентом
			mockReconciler := &AddressGroupBindingReconciler{
				Client: mockClient,
				Scheme: k8sClient.Scheme(),
			}

			// Запускаем reconcile
			_, err := mockReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})

			// Проверяем, что ошибка обработана и не возвращена из reconcile
			// Это означает, что механизм повторных попыток сработал
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что счетчик обновлений увеличился
			Expect(mockClient.updateCount).To(BeNumerically(">", 0))
		})
	})

	Context("При интеграции с Service контроллером", func() {
		// Краткое описание: Тесты для интеграции с Service контроллером
		// Детальное описание: Проверяет, что AddressGroupBinding корректно взаимодействует
		// с Service контроллером при изменении Service.

		// Генерируем уникальные имена для каждого теста
		var serviceName, addressGroupName, bindingName string
		var namespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			addressGroupName = fmt.Sprintf("test-addressgroup-%s", uid)
			bindingName = fmt.Sprintf("test-binding-%s", uid)
			namespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
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
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Примечание: AddressGroup не создается, т.к. используется мок

			// Создаем AddressGroupBinding
			By("Создание AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: netguardv1alpha1.AddressGroupBindingSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
					AddressGroupRef: netguardv1alpha1.NamespacedObjectReference{
						ObjectReference: netguardv1alpha1.ObjectReference{
							APIVersion: "provider.sgroups.io/v1alpha1",
							Kind:       "AddressGroup",
							Name:       addressGroupName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())

			// Запускаем reconcile для установки связей
			_, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Ждем, пока AddressGroupBinding получит статус Ready
			Eventually(func() bool {
				updatedBinding := &netguardv1alpha1.AddressGroupBinding{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, updatedBinding)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedBinding.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление AddressGroupBinding")
			binding := &netguardv1alpha1.AddressGroupBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      bindingName,
				Namespace: namespace,
			}, binding)
			if err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}

			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: namespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Примечание: AddressGroup не удаляется, т.к. используется мок

			// Удаление AddressGroupPortMapping, если оно было создано
			By("Удаление AddressGroupPortMapping")
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      addressGroupName,
				Namespace: namespace,
			}, portMapping)
			if err == nil {
				Expect(k8sClient.Delete(ctx, portMapping)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				}, &netguardv1alpha1.AddressGroupBinding{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен обновить AddressGroupPortMapping при изменении портов Service", func() {
			// Краткое описание: Тест обновления AddressGroupPortMapping при изменении портов Service
			// Детальное описание: Проверяет, что AddressGroupBinding корректно обновляет
			// AddressGroupPortMapping при изменении портов Service

			// Получаем текущий Service
			service := &netguardv1alpha1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: namespace,
			}, service)).To(Succeed())

			// Изменяем порты Service
			By("Изменение портов Service")
			service.Spec.IngressPorts = []netguardv1alpha1.IngressPort{
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
			}
			Expect(k8sClient.Update(ctx, service)).To(Succeed())

			// Запускаем reconcile для обработки изменений
			_, err := addressGroupBindingReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bindingName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что AddressGroupPortMapping обновлен с новыми портами
			portMapping := &netguardv1alpha1.AddressGroupPortMapping{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      addressGroupName,
					Namespace: namespace,
				}, portMapping)
				if err != nil {
					return false
				}

				for _, sp := range portMapping.AccessPorts.Items {
					if sp.GetName() == serviceName {
						// Проверяем, что порты обновлены корректно
						if len(sp.Ports.TCP) == 2 &&
							sp.Ports.TCP[0].Port == "80" &&
							sp.Ports.TCP[1].Port == "443" {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
