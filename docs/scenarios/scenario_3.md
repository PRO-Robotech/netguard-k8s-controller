# Сценарий 3: Создание политики AddressGroupBindingPolicy

## Описание
В этом сценарии пользователь создает политику, разрешающую кросс-неймспейс привязки. Политика должна быть создана в том же неймспейсе, что и AddressGroup, и ссылаться на существующие Service и AddressGroup.

## Последовательность действий

```mermaid
sequenceDiagram
    actor User
    participant API as Kubernetes API Server
    participant AGBPWebhook as AddressGroupBindingPolicy Webhook
    participant AGBPController as AddressGroupBindingPolicy Controller
    participant Client as K8s Client
    participant Service as Service Resource
    participant AG as AddressGroup Resource
    participant AGPM as AddressGroupPortMapping

    User->>API: Создать AddressGroupBindingPolicy
    API->>AGBPWebhook: Запрос на валидацию (ValidateCreate)

    Note over AGBPWebhook: Проверка, что политика создается в том же неймспейсе, что и AddressGroup

    AGBPWebhook->>Client: Получить AddressGroup
    Client-->>AGBPWebhook: AddressGroup

    AGBPWebhook->>Client: Получить AddressGroupPortMapping (для обратной совместимости)
    Client-->>AGBPWebhook: AddressGroupPortMapping

    AGBPWebhook->>Client: Получить Service
    Client-->>AGBPWebhook: Service

    AGBPWebhook->>Client: Проверить наличие дубликатов политик
    Client-->>AGBPWebhook: Список существующих политик

    alt Дубликат найден
        AGBPWebhook-->>API: Ошибка: дублирующая политика
        API-->>User: Ошибка создания ресурса
    else Дубликат не найден
        AGBPWebhook-->>API: Валидация успешна
        API->>API: Создать AddressGroupBindingPolicy
        API-->>User: AddressGroupBindingPolicy создана

        Note over AGBPController: Контроллер запускается после создания ресурса

        API->>AGBPController: Событие создания ресурса
        AGBPController->>Client: Получить AddressGroup
        Client-->>AGBPController: AddressGroup
        AGBPController->>Client: Получить Service
        Client-->>AGBPController: Service

        AGBPController->>API: Обновить статус политики (Ready=True)
        API-->>AGBPController: Статус обновлен
    end
```

## Детали реализации

1. Пользователь отправляет запрос на создание ресурса AddressGroupBindingPolicy через Kubernetes API.
2. API-сервер вызывает валидационный вебхук для AddressGroupBindingPolicy.
3. Вебхук проверяет:
   - Что политика создается в том же неймспейсе, что и AddressGroup
   - Существование AddressGroup в неймспейсе политики
   - Существование AddressGroupPortMapping в неймспейсе политики (для обратной совместимости)
   - Существование Service в указанном неймспейсе
   - Отсутствие дублирующих политик для той же пары Service-AddressGroup
4. Если все проверки пройдены успешно, ресурс создается.
5. Если обнаружены дубликаты или отсутствуют необходимые ресурсы, возвращается ошибка.
6. После создания ресурса контроллер AddressGroupBindingPolicy:
   - Проверяет существование Service и AddressGroup напрямую (без использования промежуточных ресурсов)
   - Обновляет статус политики, устанавливая условия (conditions):
     - AddressGroupFound: указывает, найдена ли группа адресов
     - ServiceFound: указывает, найден ли сервис
     - Ready: указывает, что политика валидна и готова к использованию
   - Не создает ресурсы AddressGroupBinding напрямую (это делается через контроллер AddressGroupBinding)

## Особенности безопасности

1. Политика должна быть создана в том же неймспейсе, что и AddressGroup, чтобы обеспечить контроль доступа со стороны владельцев AddressGroup.
2. Система предотвращает создание дублирующих политик, чтобы избежать неоднозначности в правилах доступа.
3. Проверка существования ресурсов гарантирует, что политика не будет создана для несуществующих объектов.
