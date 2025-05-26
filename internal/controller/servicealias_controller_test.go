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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netguardv1alpha1 "sgroups.io/netguard/api/v1alpha1"
)

var _ = Describe("ServiceAlias Controller", func() {
	// Краткое описание: Тесты для контроллера ServiceAlias
	// Детальное описание: Проверяет логику согласования ресурса ServiceAlias,
	// включая создание ссылок на сервисы, обработку кросс-namespace ссылок,
	// обновление статуса и обработку ошибок.

	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	// Создаем контроллер для использования в тестах
	var serviceAliasReconciler *ServiceAliasReconciler

	BeforeEach(func() {
		// Инициализируем контроллер перед каждым тестом
		serviceAliasReconciler = &ServiceAliasReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("При создании ServiceAlias с корректной ссылкой на Service в том же namespace", func() {
		// Краткое описание: Тесты для обработки создания ServiceAlias с корректной ссылкой на Service
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые ServiceAlias с ссылкой на существующий Service в том же namespace,
		// добавляет финализатор, устанавливает owner reference и статус Ready.

		// Генерируем уникальные имена для каждого теста
		var serviceName, serviceAliasName string
		var serviceNamespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			serviceAliasName = fmt.Sprintf("test-servicealias-%s", uid)
			serviceNamespace = "default"

			// Создаем Service
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

			// Создаем ServiceAlias
			By("Создание ServiceAlias с ссылкой на Service")
			serviceAlias := &netguardv1alpha1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceAliasSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceAlias)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceNamespace,
			}, serviceAlias)
			if err == nil {
				Expect(k8sClient.Delete(ctx, serviceAlias)).To(Succeed())
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

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.ServiceAlias{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.Service{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен установить статус Ready, добавить финализатор и owner reference", func() {
			// Краткое описание: Тест успешного создания ServiceAlias с ссылкой на Service
			// Детальное описание: Проверяет, что контроллер успешно создает ServiceAlias с ссылкой на Service,
			// добавляет финализатор, устанавливает owner reference и статус Ready в True

			By("Запуск согласования (reconcile)")
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что ServiceAlias получил статус Ready
			updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedServiceAlias.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что финализатор добавлен
			Expect(controllerutil.ContainsFinalizer(updatedServiceAlias, "servicealias.netguard.sgroups.io/finalizer")).To(BeTrue())

			// Проверяем, что owner reference установлен
			Expect(updatedServiceAlias.OwnerReferences).NotTo(BeEmpty())
			found := false
			for _, ref := range updatedServiceAlias.OwnerReferences {
				if ref.Name == serviceName && ref.Kind == "Service" {
					found = true
					// Проверяем, что это controller reference
					Expect(ref.Controller).NotTo(BeNil())
					Expect(*ref.Controller).To(BeTrue())
					break
				}
			}
			Expect(found).To(BeTrue(), "Owner reference to Service not found")
		})
	})

	Context("При создании ServiceAlias с некорректной ссылкой на несуществующий Service", func() {
		// Краткое описание: Тесты для обработки создания ServiceAlias с некорректной ссылкой
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// новые ServiceAlias с ссылкой на несуществующий Service,
		// устанавливает статус NotReady с соответствующей причиной.

		// Генерируем уникальные имена для каждого теста
		var nonExistentServiceName, serviceAliasName string
		var serviceAliasNamespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			nonExistentServiceName = fmt.Sprintf("non-existent-service-%s", uid)
			serviceAliasName = fmt.Sprintf("test-servicealias-%s", uid)
			serviceAliasNamespace = "default"

			// Создаем ServiceAlias с ссылкой на несуществующий Service
			By("Создание ServiceAlias с ссылкой на несуществующий Service")
			serviceAlias := &netguardv1alpha1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAliasName,
					Namespace: serviceAliasNamespace,
				},
				Spec: netguardv1alpha1.ServiceAliasSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       nonExistentServiceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceAlias)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceAliasNamespace,
			}, serviceAlias)
			if err == nil {
				Expect(k8sClient.Delete(ctx, serviceAlias)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceAliasNamespace,
				}, &netguardv1alpha1.ServiceAlias{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен установить статус NotReady с причиной ServiceNotFound", func() {
			// Краткое описание: Тест обработки ServiceAlias с ссылкой на несуществующий Service
			// Детальное описание: Проверяет, что контроллер устанавливает статус NotReady
			// с причиной ServiceNotFound при ссылке на несуществующий Service

			By("Запуск согласования (reconcile)")
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceAliasNamespace,
				},
			})
			// Ожидаем ошибку, т.к. сервис не существует
			Expect(err).To(HaveOccurred())

			// Проверяем, что ServiceAlias получил статус NotReady с причиной ServiceNotFound
			updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceAliasNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}

				for _, condition := range updatedServiceAlias.Status.Conditions {
					if condition.Type == netguardv1alpha1.ConditionReady &&
						condition.Status == metav1.ConditionFalse &&
						condition.Reason == "ServiceNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что финализатор добавлен, несмотря на ошибку
			Expect(controllerutil.ContainsFinalizer(updatedServiceAlias, "servicealias.netguard.sgroups.io/finalizer")).To(BeTrue())
		})
	})

	Context("При обновлении ServiceAlias при изменении ссылки на Service", func() {
		// Краткое описание: Тесты для обработки обновления ServiceAlias
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// обновление ServiceAlias при изменении ссылки на Service,
		// обновляет owner reference и статус.

		// Генерируем уникальные имена для каждого теста
		var service1Name, service2Name, serviceAliasName string
		var serviceNamespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			service1Name = fmt.Sprintf("test-service-1-%s", uid)
			service2Name = fmt.Sprintf("test-service-2-%s", uid)
			serviceAliasName = fmt.Sprintf("test-servicealias-%s", uid)
			serviceNamespace = "default"

			// Создаем два Service
			By("Создание первого Service")
			service1 := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      service1Name,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service 1",
				},
			}
			Expect(k8sClient.Create(ctx, service1)).To(Succeed())

			By("Создание второго Service")
			service2 := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      service2Name,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service 2",
				},
			}
			Expect(k8sClient.Create(ctx, service2)).To(Succeed())

			// Создаем ServiceAlias с ссылкой на первый Service
			By("Создание ServiceAlias с ссылкой на первый Service")
			serviceAlias := &netguardv1alpha1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceAliasSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       service1Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceAlias)).To(Succeed())

			// Запускаем reconcile для установки owner reference
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Ждем, пока ServiceAlias получит статус Ready
			Eventually(func() bool {
				updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(updatedServiceAlias.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceNamespace,
			}, serviceAlias)
			if err == nil {
				Expect(k8sClient.Delete(ctx, serviceAlias)).To(Succeed())
			}

			By("Удаление первого Service")
			service1 := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      service1Name,
				Namespace: serviceNamespace,
			}, service1)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service1)).To(Succeed())
			}

			By("Удаление второго Service")
			service2 := &netguardv1alpha1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      service2Name,
				Namespace: serviceNamespace,
			}, service2)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service2)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.ServiceAlias{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      service1Name,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.Service{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      service2Name,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.Service{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен обновить owner reference при изменении ссылки на Service", func() {
			// Краткое описание: Тест обновления ServiceAlias при изменении ссылки на Service
			// Детальное описание: Проверяет, что контроллер обновляет owner reference
			// при изменении ссылки на Service

			// Получаем текущий ServiceAlias
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceNamespace,
			}, serviceAlias)).To(Succeed())

			// Проверяем, что owner reference установлен на первый Service
			Expect(serviceAlias.OwnerReferences).NotTo(BeEmpty())
			found := false
			for _, ref := range serviceAlias.OwnerReferences {
				if ref.Name == service1Name && ref.Kind == "Service" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Owner reference to first Service not found")

			// Обновляем ServiceAlias, меняя ссылку на второй Service
			By("Обновление ServiceAlias для ссылки на второй Service")
			serviceAlias.Spec.ServiceRef.Name = service2Name
			Expect(k8sClient.Update(ctx, serviceAlias)).To(Succeed())

			// Запускаем reconcile для обновления owner reference
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что owner reference обновился на второй Service
			updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}

				for _, ref := range updatedServiceAlias.OwnerReferences {
					if ref.Name == service2Name && ref.Kind == "Service" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Проверяем, что статус остался Ready
			Expect(meta.IsStatusConditionTrue(updatedServiceAlias.Status.Conditions, netguardv1alpha1.ConditionReady)).To(BeTrue())
		})
	})

	Context("При удалении ServiceAlias", func() {
		// Краткое описание: Тесты для обработки удаления ServiceAlias
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// удаление ServiceAlias, удаляя финализатор.

		// Генерируем уникальные имена для каждого теста
		var serviceName, serviceAliasName string
		var serviceNamespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			serviceAliasName = fmt.Sprintf("test-servicealias-%s", uid)
			serviceNamespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service",
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Создаем ServiceAlias
			By("Создание ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceAliasSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{

						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceAlias)).To(Succeed())

			// Запускаем reconcile для установки финализатора и owner reference
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Ждем, пока ServiceAlias получит статус Ready и финализатор
			Eventually(func() bool {
				updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}

				// Проверяем наличие финализатора
				if !controllerutil.ContainsFinalizer(updatedServiceAlias, "servicealias.netguard.sgroups.io/finalizer") {
					return false
				}

				// Проверяем статус Ready
				return meta.IsStatusConditionTrue(updatedServiceAlias.Status.Conditions, netguardv1alpha1.ConditionReady)
			}, timeout, interval).Should(BeTrue())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление Service")
			service := &netguardv1alpha1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: serviceNamespace,
			}, service)
			if err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.Service{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен корректно обработать удаление ServiceAlias", func() {
			// Краткое описание: Тест удаления ServiceAlias
			// Детальное описание: Проверяет, что контроллер корректно обрабатывает
			// удаление ServiceAlias, удаляя финализатор

			// Получаем текущий ServiceAlias
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceNamespace,
			}, serviceAlias)).To(Succeed())

			// Удаляем ServiceAlias
			By("Удаление ServiceAlias")
			Expect(k8sClient.Delete(ctx, serviceAlias)).To(Succeed())

			// Получаем обновленный ServiceAlias с DeletionTimestamp
			updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}
				return !updatedServiceAlias.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			// Запускаем reconcile для обработки удаления
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что ServiceAlias удален
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.ServiceAlias{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("При автоматическом удалении ServiceAlias при удалении связанного Service", func() {
		// Краткое описание: Тесты для автоматического удаления ServiceAlias
		// Детальное описание: Проверяет, что ServiceAlias автоматически удаляется
		// при удалении связанного Service благодаря owner reference.

		// Генерируем уникальные имена для каждого теста
		var serviceName, serviceAliasName string
		var serviceNamespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			serviceAliasName = fmt.Sprintf("test-servicealias-%s", uid)
			serviceNamespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service",
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Создаем ServiceAlias
			By("Создание ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceAliasSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceAlias)).To(Succeed())

			// Запускаем reconcile для установки owner reference
			_, err := serviceAliasReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Ждем, пока ServiceAlias получит owner reference
			Eventually(func() bool {
				updatedServiceAlias := &netguardv1alpha1.ServiceAlias{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, updatedServiceAlias)
				if err != nil {
					return false
				}

				// Проверяем наличие owner reference
				for _, ref := range updatedServiceAlias.OwnerReferences {
					if ref.Name == serviceName && ref.Kind == "Service" {
						// Проверяем, что это controller reference
						if ref.Controller != nil && *ref.Controller {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("должен автоматически удалить ServiceAlias при удалении Service", func() {
			// Краткое описание: Тест автоматического удаления ServiceAlias
			// Детальное описание: Проверяет, что ServiceAlias автоматически удаляется
			// при удалении связанного Service благодаря controller owner reference

			// Получаем текущий Service
			service := &netguardv1alpha1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: serviceNamespace,
			}, service)).To(Succeed())

			// Удаляем Service
			By("Удаление Service")
			Expect(k8sClient.Delete(ctx, service)).To(Succeed())

			// Проверяем, что ServiceAlias автоматически удален
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.ServiceAlias{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("При обработке конфликтов при обновлении", func() {
		// Краткое описание: Тесты для обработки конфликтов при обновлении
		// Детальное описание: Проверяет, что контроллер корректно обрабатывает
		// конфликты при обновлении ресурсов, используя механизм повторных попыток.

		// Генерируем уникальные имена для каждого теста
		var serviceName, serviceAliasName string
		var serviceNamespace string

		BeforeEach(func() {
			// Генерируем уникальные имена для ресурсов
			uid := uuid.New().String()[:8]
			serviceName = fmt.Sprintf("test-service-%s", uid)
			serviceAliasName = fmt.Sprintf("test-servicealias-%s", uid)
			serviceNamespace = "default"

			// Создаем Service
			By("Создание Service")
			service := &netguardv1alpha1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceSpec{
					Description: "Test Service",
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			// Создаем ServiceAlias
			By("Создание ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
				Spec: netguardv1alpha1.ServiceAliasSpec{
					ServiceRef: netguardv1alpha1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1alpha1",
						Kind:       "Service",
						Name:       serviceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceAlias)).To(Succeed())
		})

		AfterEach(func() {
			// Очистка ресурсов после теста
			By("Удаление ServiceAlias")
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceNamespace,
			}, serviceAlias)
			if err == nil {
				Expect(k8sClient.Delete(ctx, serviceAlias)).To(Succeed())
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

			// Ждем удаления ресурсов
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.ServiceAlias{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceName,
					Namespace: serviceNamespace,
				}, &netguardv1alpha1.Service{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("должен использовать механизм повторных попыток при конфликтах", func() {
			// Краткое описание: Тест обработки конфликтов при обновлении
			// Детальное описание: Проверяет, что контроллер использует механизм
			// повторных попыток при конфликтах обновления

			// Получаем текущий ServiceAlias
			serviceAlias := &netguardv1alpha1.ServiceAlias{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceAliasName,
				Namespace: serviceNamespace,
			}, serviceAlias)).To(Succeed())

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
			mockReconciler := &ServiceAliasReconciler{
				Client: mockClient,
				Scheme: k8sClient.Scheme(),
			}

			// Запускаем reconcile
			_, err := mockReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serviceAliasName,
					Namespace: serviceNamespace,
				},
			})

			// Проверяем, что ошибка обработана и не возвращена из reconcile
			// Это означает, что механизм повторных попыток сработал
			Expect(err).NotTo(HaveOccurred())

			// Проверяем, что счетчик обновлений увеличился
			Expect(mockClient.updateCount).To(BeNumerically(">", 0))
		})
	})
})

// MockClient - мок-клиент для тестирования обработки ошибок
type MockClient struct {
	client.Client
	updateCount int
	updateFunc  func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	getFunc     func(ctx context.Context, key client.ObjectKey, obj client.Object) error
	statusFunc  func() client.StatusWriter
}

// Update - переопределение метода Update для мок-клиента
func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, obj, opts...)
	}
	return m.Client.Update(ctx, obj, opts...)
}

// Get - переопределение метода Get для мок-клиента
func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, obj)
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

// Status - переопределение метода Status для мок-клиента
func (m *MockClient) Status() client.StatusWriter {
	if m.statusFunc != nil {
		return m.statusFunc()
	}
	return m.Client.Status()
}

// MockStatusWriter - мок для StatusWriter
type MockStatusWriter struct {
	client.StatusWriter
	updateFunc func(ctx context.Context, obj client.Object) error
}

// Update - переопределение метода Update для мок-StatusWriter
func (m *MockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, obj)
	}
	return m.StatusWriter.Update(ctx, obj, opts...)
}
