# E2E-сценарии (баллы 1–7): как запускать

Проверки покрывают сценарии 1–5 из методички. Сценарии 6–8 (кластер Cassandra, Grafana, эволюция схемы) относятся к блоку **8–10** и здесь не автоматизированы.

## Предварительные условия

- Docker и Docker Compose v2.
- Репозиторий: каталог с `docker-compose.yml` (ниже — переменная `SOA_HW`).

```bash
export SOA_HW=/absolute/path/to/SOA-hw   # подставьте свой путь
cd "$SOA_HW"
```

Поднять стек (после первого раза достаточно `docker compose up -d`):

```bash
docker compose up -d --build
```

Ожидание готовности Cassandra (опционально):

```bash
until docker compose exec -T cassandra cqlsh -e "DESCRIBE KEYSPACES" >/dev/null 2>&1; do sleep 2; done
```

Kafka с хоста (если нужен клиент на машине): **`localhost:19092`** (внутри Docker-сети брокер — `kafka:29092`). Schema Registry: `http://localhost:8081`.

---

## Автоматический прогон всех сценариев 1–5

Скрипт останавливает сервис **producer** (чтобы демо-поток не смешивался с тестовыми SKU), затем публикует события через образ `producer` и проверяет Cassandra.

```bash
cd "$SOA_HW"
docker compose stop producer    # если producer включён с PRODUCER_MODE=demo
./scripts/e2e_verify.sh
```

При успехе в конце будет: `Все сценарии 1–5 прошли успешно.`

Уникальные SKU формируются как `E2E-S<n>-<RUN_ID>-SKU`, так что повторный запуск не требует очистки БД.

---

## Ручная отправка одного события (Avro + Schema Registry)

Рабочий каталог — корень репозитория. Шаблон события — JSON по полям из `schemas/WarehouseEvent.avsc`.

### Вариант A: файл на хосте

```bash
cd "$SOA_HW"
cat > scripts/my_event.json <<'EOF'
{
  "event_id": "00000000-0000-4000-8000-000000000001",
  "event_type": "PRODUCT_RECEIVED",
  "occurred_at": 1735689600000,
  "product_id": "MANUAL-SKU-001",
  "zone_id": "ZONE-A",
  "quantity": 10,
  "from_zone_id": null,
  "to_zone_id": null,
  "counted_quantity": null,
  "order_id": null,
  "order_lines": null
}
EOF

docker compose run --rm --no-deps \
  -v "$SOA_HW/schemas:/schemas:ro" \
  -v "$SOA_HW/scripts:/evt:ro" \
  --entrypoint python \
  producer -m app.publish_cli /evt/my_event.json
```

### Вариант B: stdin в контейнер

```bash
cd "$SOA_HW"
docker compose run --rm --no-deps -i \
  -v "$SOA_HW/schemas:/schemas:ro" \
  --entrypoint python \
  producer -m app.publish_cli - <<'EOF'
{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"PRODUCT_RECEIVED","occurred_at":1735689600000,"product_id":"MANUAL-SKU-002","zone_id":"ZONE-A","quantity":5,"from_zone_id":null,"to_zone_id":null,"counted_quantity":null,"order_id":null,"order_lines":null}
EOF
```

Дождитесь обработки consumer (обычно менее секунды), затем проверка в Cassandra:

```bash
docker compose exec -T cassandra cqlsh -e \
  "SELECT * FROM warehouse.inventory_by_product_zone WHERE product_id='MANUAL-SKU-001' AND zone_id='ZONE-A';"
```

---

## Ручное воспроизведение сценариев из методички

Используйте **свои** `event_id` (UUID) и монотонно растущие `occurred_at` (epoch ms), если проверяете порядок по времени. Поля `null` в JSON можно опускать только если ваш клиент подставляет значения по умолчанию; для `publish_cli` безопаснее передавать все поля как в примерах выше.

### Сценарий 1 — базовый цикл

Последовательность событий (как в задании): `PRODUCT_RECEIVED` → `PRODUCT_RESERVED` → `PRODUCT_MOVED` → `PRODUCT_SHIPPED` → `ORDER_CREATED` → `ORDER_COMPLETED`. После каждого шага выполняйте `SELECT` в `inventory_by_product_zone`, `inventory_by_product`, при необходимости `inventory_by_zone`.

Пример `ORDER_CREATED`:

```json
{
  "event_id": "<uuid>",
  "event_type": "ORDER_CREATED",
  "occurred_at": <ms>,
  "order_id": "<uuid заказа>",
  "order_lines": [
    { "product_id": "<SKU>", "zone_id": "ZONE-A", "quantity": 15 }
  ],
  "product_id": null,
  "zone_id": null,
  "quantity": null,
  "from_zone_id": null,
  "to_zone_id": null,
  "counted_quantity": null
}
```

Затем `ORDER_COMPLETED` с тем же `order_id`.

### Сценарий 2 — идемпотентность

Отправьте два раза **одинаковый** JSON (включая тот же `event_id`). Второй раз остатки не изменятся; в логах consumer: `skipped_duplicate`.

### Сценарий 3 — три таблицы

После одного `PRODUCT_RECEIVED` проверьте три таблицы одним и тем же SKU и зоной:

```bash
docker compose exec -T cassandra cqlsh -e \
  "SELECT * FROM warehouse.inventory_by_product_zone WHERE product_id='<SKU>' AND zone_id='ZONE-A';"
docker compose exec -T cassandra cqlsh -e \
  "SELECT * FROM warehouse.inventory_by_product WHERE product_id='<SKU>';"
docker compose exec -T cassandra cqlsh -e \
  "SELECT * FROM warehouse.inventory_by_zone WHERE zone_id='ZONE-A' AND product_id='<SKU>';"
```

### Сценарий 4 — вне порядка

Задайте фиксированные `occurred_at`: сначала более новое событие, затем более старое (как в примере 12:00 / 12:05 / 12:02). Старое должно быть проигнорировано (`skipped_stale` в логах).

### Сценарий 5 — DLQ

Отправьте `PRODUCT_SHIPPED` с `quantity: -5`. Сообщение попадёт в топик `warehouse-events-dlq`. Просмотр:

```bash
docker compose exec -T kafka kafka-console-consumer \
  --bootstrap-server kafka:29092 \
  --topic warehouse-events-dlq \
  --from-beginning \
  --max-messages 20
```

Затем отправьте валидное событие и убедитесь, что оно обработано.

---

## Логи consumer

```bash
docker compose logs -f consumer
```

---

## Частые проблемы

| Проблема | Что сделать |
|----------|-------------|
| Демо-producer пишет свои SKU | `docker compose stop producer` перед ручными тестами |
| Порт 9092 на хосте занят | В compose Kafka проброшен как **19092** — см. `docker-compose.yml` |
| Скрипт E2E падает на assert | Убедитесь, что consumer запущен: `docker compose ps consumer` |
