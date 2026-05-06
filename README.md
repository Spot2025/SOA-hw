# Smart Warehouse (Kafka + Cassandra) — баллы 1–7

Одна команда поднимает Zookeeper, Kafka, Schema Registry, Cassandra, регистрацию Avro-схемы, инициализацию keyspace, сервисы **producer** (WMS) и **consumer** (состояние склада).

```bash
docker compose up --build
```

Автоматические и ручные E2E-сценарии (баллы 1–7): см. [docs/E2E.md](docs/E2E.md). Кратко: `./scripts/e2e_verify.sh` после `docker compose up -d`.

После старта:

- Kafka (с хоста): `localhost:19092` (внутри Docker-сети брокер: `kafka:29092`)
- Schema Registry: `http://localhost:8081`
- Cassandra CQL: `localhost:9042`, keyspace `warehouse`

## Модель данных в Cassandra (запросо-ориентированная)

JOIN нет; таблицы под конкретные read-паттерны из задания.

1. **`inventory_by_product_zone`** — первичный ключ `(product_id, zone_id)` (составная партиция). Один запрос: остаток SKU в конкретной зоне. Поле `last_event_ms` хранит время последнего **применённого** события для этой пары (и денормализовано согласованно с `inventory_by_zone`).

2. **`inventory_by_product`** — партиция `product_id`. Агрегированные `total_available` / `total_reserved` по всем зонам без скана кластера по зонам.

3. **`inventory_by_zone`** — партиция `zone_id`, кластерный ключ `product_id`. Один запрос: все SKU в зоне.

Дополнительно: `processed_events` (идемпотентность по `event_id`), `orders`, `event_history` (аудит, append-only по сущности).

Обоснование ключей: в Cassandra партиция должна соответствовать «одному логическому запросу», чтобы избежать scatter/gather и фильтрации по всему кластеру.

## Семантика consumer (1 балл)

- Топик: `warehouse-events`, группа: `warehouse-state-consumer`.
- `enable.auto.commit=false`: **commit выполняется только после** успешной обработки (включая запись в Cassandra или отправку в DLQ для «ядовитых» сообщений, чтобы не зациклиться).
- В логах: `event_id`, `event_type`, `partition`, `offset` для `processed`, `skipped_duplicate`, `skipped_stale`, `sent_to_dlq`.

## Идемпотентность (4 балл)

Перед применением изменений проверяется `processed_events`. Повтор с тем же `event_id` не меняет остатки.

## Атомарность денормализации (5 балл)

Для каждого успешно применяемого события все связанные UPDATE/INSERT по инвентарю (и запись в `processed_events` + аудит) выполняются одним **logged `BATCH`** в Cassandra, без частичного обновления одной таблицы без остальных.

## События вне порядка (6 балл)

Используется **время события** `occurred_at` (epoch ms): событие отбрасывается как устаревшее, если `occurred_at` не строго больше `last_event_ms` всех затронутых строк инвентаря (для `PRODUCT_MOVED` проверяются обе зоны). Так «поздно пришедшее, но более старое» событие не перезаписывает новое состояние.

## Dead Letter Queue (7 балл)

При ошибке валидации/обработки событие уходит в топик `warehouse-events-dlq` в JSON-обёртке: полный `original_event`, `error_code`, `error_reason`, `failed_at`, `kafka_metadata` (partition, offset), опционально `stacktrace`. Consumer не падает; следующие сообщения обрабатываются.

Пример гарантированного DLQ: `PRODUCT_SHIPPED` с `quantity = -5` (см. демо producer).

## Producer

Контейнер `producer` в режиме `PRODUCER_MODE=demo` отправляет последовательность событий (базовый цикл склада, дубликат `event_id`, out-of-order, невалидное количество, затем валидное).

## Структура репозитория

- `docker-compose.yml` — вся инфраструктура и сервисы
- `cassandra/schema.cql` — миграции (применяются job-контейнером `cassandra-init`)
- `schemas/WarehouseEvent.avsc` — Avro-схема, регистрируется в Schema Registry (`schema-init`)
- `consumer/` — stateful consumer
- `producer/` — WMS producer
- `scripts/register_schema.py` — регистрация схемы через REST API
