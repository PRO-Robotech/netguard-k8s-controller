# Сценарий 6: Проверка перекрытия портов при создании привязки

## Описание
Этот сценарий детализирует процесс проверки перекрытия портов при создании привязки между Service и AddressGroup. Это критически важная проверка, которая предотвращает конфликты портов между различными сервисами в одной группе адресов.

## Последовательность действий

```mermaid
sequenceDiagram
    participant AGBWebhook as AddressGroupBinding Webhook
    participant Service as Service Resource
    participant AGPM as AddressGroupPortMapping

    AGBWebhook->>AGBWebhook: CheckPortOverlaps(service, portMapping)

    Note over AGBWebhook: Извлечение портов из Service

    loop Для каждого IngressPort в Service
        AGBWebhook->>AGBWebhook: ParsePortRange(ingressPort.Port)
    end

    Note over AGBWebhook: Проверка перекрытий с существующими сервисами

    loop Для каждого ServicePortRef в portMapping
        Note over AGBWebhook: Пропуск текущего сервиса (при обновлении)

        loop Для каждого TCP порта
            AGBWebhook->>AGBWebhook: ParsePortRange(tcpPort.Port)
            AGBWebhook->>AGBWebhook: DoPortRangesOverlap(portRange, serviceRange)
        end

        loop Для каждого UDP порта
            AGBWebhook->>AGBWebhook: ParsePortRange(udpPort.Port)
            AGBWebhook->>AGBWebhook: DoPortRangesOverlap(portRange, serviceRange)
        end
    end

    alt Перекрытие найдено
        AGBWebhook-->>AGBWebhook: Возврат ошибки
    else Перекрытие не найдено
        AGBWebhook-->>AGBWebhook: Возврат nil (успех)
    end
```

## Детали реализации

1. Функция `CheckPortOverlaps` принимает два аргумента:
   - `service`: Сервис, для которого создается привязка
   - `portMapping`: Существующий AddressGroupPortMapping для AddressGroup

2. Процесс проверки включает следующие шаги:
   - Извлечение всех портов из Service и преобразование их в структуры PortRange
   - Создание карты портов по протоколам (TCP/UDP)
   - Перебор всех сервисов в portMapping (кроме текущего сервиса при обновлении)
   - Для каждого сервиса проверка перекрытия его TCP и UDP портов с портами текущего сервиса

3. Функция `ParsePortRange` преобразует строковое представление порта (например, "80" или "8080-9090") в структуру с началом и концом диапазона.

4. Функция `DoPortRangesOverlap` проверяет, перекрываются ли два диапазона портов, используя простое условие:
   ```
   a.Start <= b.End && a.End >= b.Start
   ```

## Технические особенности

1. Система поддерживает как одиночные порты, так и диапазоны портов.
2. Проверка перекрытий учитывает протокол (TCP/UDP), поэтому одинаковые порты разных протоколов не считаются конфликтующими.
3. При обновлении привязки текущий сервис исключается из проверки, чтобы избежать ложных срабатываний.
4. Если обнаружено перекрытие, функция возвращает подробную ошибку с указанием конфликтующих портов и сервисов.
