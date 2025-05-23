# Сценарий 9: Создание правила RuleS2S для взаимодействия между сервисами

## Описание

В этом сценарии пользователь создает правило RuleS2S, которое определяет взаимодействие между двумя сервисами в разных неймспейсах. На основе этого правила система автоматически создает соответствующие правила IEAgAgRule в провайдере, которые определяют правила доступа между группами адресов.

## Последовательность действий

```mermaid
sequenceDiagram
    actor User
    participant API as Kubernetes API Server
    participant RuleS2SWebhook as RuleS2S Webhook
    participant Client as K8s Client
    participant RuleS2SController as RuleS2S Controller
    participant ServiceA as Service A
    participant ServiceB as Service B
    participant Provider as Provider API

    User->>API: Создать RuleS2S
    API->>RuleS2SWebhook: Запрос на валидацию (ValidateCreate)
    RuleS2SWebhook->>Client: Получить ServiceAlias (serviceLocalRef)
    Client-->>RuleS2SWebhook: ServiceAlias
    RuleS2SWebhook->>Client: Получить ServiceAlias (serviceRef)
    Client-->>RuleS2SWebhook: ServiceAlias

    Note over RuleS2SWebhook: Проверка существования ресурсов

    alt Ресурсы не существуют
        RuleS2SWebhook-->>API: Ошибка: ресурсы не найдены
        API-->>User: Ошибка создания ресурса
    else Ресурсы существуют
        RuleS2SWebhook-->>API: Валидация успешна
        API->>API: Создать RuleS2S
        API-->>User: RuleS2S создан
        API->>RuleS2SController: Событие создания RuleS2S
        RuleS2SController->>Client: Получить RuleS2S
        Client-->>RuleS2SController: RuleS2S
        RuleS2SController->>Client: Получить ServiceAlias (serviceLocalRef)
        Client-->>RuleS2SController: ServiceAlias
        RuleS2SController->>Client: Получить ServiceAlias (serviceRef)
        Client-->>RuleS2SController: ServiceAlias
        RuleS2SController->>Client: Получить Service A
        Client-->>RuleS2SController: Service A
        RuleS2SController->>Client: Получить Service B
        Client-->>RuleS2SController: Service B
        
        Note over RuleS2SController: Обработка кросс-неймспейс ссылок
        
        alt Кросс-неймспейс ссылка
            RuleS2SController->>ServiceB: Добавить ссылку в RuleS2SDstOwnRef
            ServiceB-->>RuleS2SController: Обновлено
        else Тот же неймспейс
            RuleS2SController->>RuleS2S: Установить владельца (OwnerReference)
            RuleS2S-->>RuleS2SController: Обновлено
        end
        
        Note over RuleS2SController: Создание правил IEAgAgRule
        
        RuleS2SController->>Provider: Создать IEAgAgRule
        Provider-->>RuleS2SController: IEAgAgRule создан
        RuleS2SController->>RuleS2S: Обновить статус (Ready)
        RuleS2S-->>RuleS2SController: Статус обновлен
    end
```

## Детали реализации

1. Пользователь отправляет запрос на создание ресурса RuleS2S через Kubernetes API.
2. API-сервер вызывает валидационный вебхук для RuleS2S.
3. Вебхук проверяет:
   - Существование ServiceAlias, указанного в serviceLocalRef
   - Существование ServiceAlias, указанного в serviceRef
4. Если все проверки пройдены успешно, ресурс создается.
5. Контроллер RuleS2S обрабатывает событие создания:
   - Получает связанные ServiceAlias и Service
   - Обрабатывает кросс-неймспейс ссылки
   - Создает правила IEAgAgRule на основе адресных групп и портов сервисов
   - Обновляет статус RuleS2S

## Примеры

### Пример 1: Правило ingress

```yaml
apiVersion: netguard.sgroups.io/v1alpha1
kind: RuleS2S
metadata:
  name: ingress-example
  namespace: database
spec:
  traffic: ingress
  serviceLocalRef:
    apiVersion: netguard.sgroups.io/v1alpha1
    kind: ServiceAlias
    name: service-B
  serviceRef:
    apiVersion: netguard.sgroups.io/v1alpha1
    kind: ServiceAlias
    name: service-A
```

Результирующий IEAgAgRule:

```yaml
apiVersion: provider.sgroups.io/v1alpha1
kind: IEAgAgRule
metadata:
  name: ingress-database-backend-tcp
  namespace: client-B
spec:
  transport: TCP
  addressGroup:
    apiVersion: provider.sgroups.io/v1alpha1
    kind: AddressGroup
    name: backend
    namespace: client-A
  addressGroupLocal:
    apiVersion: provider.sgroups.io/v1alpha1
    kind: AddressGroup
    name: database
    namespace: client-B
  traffic: INGRESS
  ports:
    - d: "5432"
  action: ACCEPT
  logs: true
  priority:
    value: 100
```

### Пример 2: Правило egress

```yaml
apiVersion: netguard.sgroups.io/v1alpha1
kind: RuleS2S
metadata:
  name: egress-example
  namespace: database
spec:
  traffic: egress
  serviceLocalRef:
    apiVersion: netguard.sgroups.io/v1alpha1
    kind: ServiceAlias
    name: service-B
  serviceRef:
    apiVersion: netguard.sgroups.io/v1alpha1
    kind: ServiceAlias
    name: service-A
```

Результирующий IEAgAgRule:

```yaml
apiVersion: provider.sgroups.io/v1alpha1
kind: IEAgAgRule
metadata:
  name: egress-database-backend-tcp
  namespace: client-A
spec:
  transport: TCP
  addressGroup:
    apiVersion: provider.sgroups.io/v1alpha1
    kind: AddressGroup
    name: backend
    namespace: client-A
  addressGroupLocal:
    apiVersion: provider.sgroups.io/v1alpha1
    kind: AddressGroup
    name: database
    namespace: client-B
  traffic: EGRESS
  ports:
    - d: "80,443"
  action: ACCEPT
  logs: true
  priority:
    value: 100
```

## Особенности

1. **Направление трафика**:
   - **ingress**: Локальный сервис (serviceLocalRef) является получателем трафика
   - **egress**: Локальный сервис (serviceLocalRef) является отправителем трафика

2. **Порты**:
   - Для правил используются порты сервиса-получателя
   - Поддерживаются различные форматы портов: одиночные, списки, диапазоны

3. **Кросс-неймспейс ссылки**:
   - Если RuleS2S находится в другом неймспейсе, чем целевой сервис, создается запись в RuleS2SDstOwnRef
   - При удалении сервиса связанные правила RuleS2S удаляются автоматически