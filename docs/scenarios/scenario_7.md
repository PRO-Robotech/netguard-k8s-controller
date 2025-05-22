# Сценарий 7: Обновление привязки AddressGroupBinding

## Описание
В этом сценарии система проверяет, что при обновлении привязки не изменяются ключевые поля и сохраняются все необходимые условия. Это обеспечивает целостность системы и предотвращает непредвиденные изменения в конфигурации сети.

## Последовательность действий

```mermaid
sequenceDiagram
    actor User
    participant API as Kubernetes API Server
    participant AGBWebhook as AddressGroupBinding Webhook
    participant Client as K8s Client
    
    User->>API: Обновить AddressGroupBinding
    API->>AGBWebhook: Запрос на валидацию (ValidateUpdate)
    
    Note over AGBWebhook: Проверка, что ресурс не удаляется
    
    AGBWebhook->>AGBWebhook: Проверка неизменности ServiceRef
    AGBWebhook->>AGBWebhook: Проверка неизменности AddressGroupRef
    
    AGBWebhook->>Client: Получить Service
    Client-->>AGBWebhook: Service
    
    AGBWebhook->>Client: Получить AddressGroupPortMapping
    Client-->>AGBWebhook: AddressGroupPortMapping
    
    AGBWebhook->>AGBWebhook: Проверка портов на перекрытие
    
    Note over AGBWebhook: Проверка кросс-неймспейс политики (если применимо)
    
    alt Неймспейсы различаются
        AGBWebhook->>Client: Получить список AddressGroupBindingPolicy
        Client-->>AGBWebhook: Список политик
        
        alt Политика найдена
            AGBWebhook-->>API: Валидация успешна
        else Политика не найдена
            AGBWebhook-->>API: Ошибка: отсутствует разрешающая политика
            API-->>User: Ошибка обновления ресурса
        end
    else Неймспейсы совпадают
        AGBWebhook-->>API: Валидация успешна
    end
    
    API->>API: Обновить AddressGroupBinding
    API-->>User: AddressGroupBinding обновлен
```

## Детали реализации

1. Пользователь отправляет запрос на обновление ресурса AddressGroupBinding через Kubernetes API.
2. API-сервер вызывает валидационный вебхук для AddressGroupBinding.
3. Вебхук проверяет:
   - Что ресурс не находится в процессе удаления (DeletionTimestamp не установлен)
   - Что ключевые поля (ServiceRef и AddressGroupRef) не изменились
   - Существование Service в неймспейсе привязки
   - Существование AddressGroupPortMapping в неймспейсе AddressGroup
   - Отсутствие перекрытий портов между Service и другими сервисами
   - Наличие AddressGroupBindingPolicy в неймспейсе AddressGroup (для кросс-неймспейс привязок)
4. Если все проверки пройдены успешно, ресурс обновляется.
5. Если какая-либо проверка не пройдена, возвращается ошибка.

## Технические особенности

1. Неизменность ключевых полей (ServiceRef и AddressGroupRef) после создания ресурса обеспечивает стабильность конфигурации.
2. Повторная проверка существования ресурсов и отсутствия перекрытий портов гарантирует, что обновление не нарушит работу системы.
3. Для кросс-неймспейс привязок проверяется наличие разрешающей политики, даже если она была проверена при создании (политика могла быть удалена).
4. Проверка на удаление (DeletionTimestamp) позволяет пропустить валидацию для ресурсов, которые находятся в процессе удаления.