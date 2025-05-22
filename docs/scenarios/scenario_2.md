# Сценарий 2: Создание привязки AddressGroupBinding между разными неймспейсами

## Описание
В этом сценарии пользователь пытается создать привязку между Service и AddressGroup, находящимися в разных неймспейсах. Для успешного создания такой привязки должна существовать соответствующая политика (AddressGroupBindingPolicy) в неймспейсе AddressGroup.

## Последовательность действий

```mermaid
sequenceDiagram
    actor User
    participant API as Kubernetes API Server
    participant AGBWebhook as AddressGroupBinding Webhook
    participant Client as K8s Client
    participant Service as Service Resource
    participant AGPM as AddressGroupPortMapping
    participant AGBPolicy as AddressGroupBindingPolicy
    
    User->>API: Создать AddressGroupBinding (cross-namespace)
    API->>AGBWebhook: Запрос на валидацию (ValidateCreate)
    AGBWebhook->>Client: Получить Service
    Client-->>AGBWebhook: Service
    
    Note over AGBWebhook: Определение неймспейса AddressGroup
    
    AGBWebhook->>Client: Получить AddressGroupPortMapping
    Client-->>AGBWebhook: AddressGroupPortMapping
    
    AGBWebhook->>AGBWebhook: Проверка портов на перекрытие
    
    Note over AGBWebhook: Обнаружена кросс-неймспейс привязка
    
    AGBWebhook->>Client: Получить список AddressGroupBindingPolicy в неймспейсе AddressGroup
    Client-->>AGBWebhook: Список политик
    
    alt Политика найдена
        AGBWebhook-->>API: Валидация успешна
        API->>API: Создать AddressGroupBinding
        API-->>User: AddressGroupBinding создан
    else Политика не найдена
        AGBWebhook-->>API: Ошибка: отсутствует разрешающая политика
        API-->>User: Ошибка создания ресурса
    end
```

## Детали реализации

1. Пользователь отправляет запрос на создание ресурса AddressGroupBinding, указывая AddressGroup из другого неймспейса.
2. API-сервер вызывает валидационный вебхук для AddressGroupBinding.
3. Вебхук проверяет:
   - Существование Service в неймспейсе привязки
   - Существование AddressGroupPortMapping в неймспейсе AddressGroup
   - Отсутствие перекрытий портов между Service и другими сервисами
   - Наличие AddressGroupBindingPolicy в неймспейсе AddressGroup, разрешающей данную привязку
4. Если все проверки пройдены успешно, ресурс создается.
5. Если отсутствует разрешающая политика или не пройдены другие проверки, возвращается ошибка.

## Особенности безопасности

Механизм политик (AddressGroupBindingPolicy) обеспечивает контроль доступа между неймспейсами, предотвращая несанкционированное использование AddressGroup из других неймспейсов. Политика должна быть создана администратором неймспейса, содержащего AddressGroup.