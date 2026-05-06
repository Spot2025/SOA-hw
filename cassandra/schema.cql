CREATE KEYSPACE IF NOT EXISTS warehouse
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE warehouse;

-- Query: SELECT * FROM inventory_by_product_zone WHERE product_id = ? AND zone_id = ?;
-- Partition: (product_id, zone_id) — одна партиция на ячейку хранения, быстрый point lookup.
CREATE TABLE IF NOT EXISTS inventory_by_product_zone (
    product_id text,
    zone_id text,
    available int,
    reserved int,
    last_event_ms bigint,
    PRIMARY KEY ((product_id, zone_id))
);

-- Query: SELECT * FROM inventory_by_product WHERE product_id = ?;
-- Partition: product_id — агрегаты по SKU в одной партиции (без сканирования кластера).
CREATE TABLE IF NOT EXISTS inventory_by_product (
    product_id text PRIMARY KEY,
    total_available int,
    total_reserved int
);

-- Query: SELECT * FROM inventory_by_zone WHERE zone_id = ?;
-- Partition: zone_id, clustering: product_id — все SKU в зоне одним запросом, product_id как clustering для сортировки/уникальности.
CREATE TABLE IF NOT EXISTS inventory_by_zone (
    zone_id text,
    product_id text,
    available int,
    reserved int,
    last_event_ms bigint,
    PRIMARY KEY (zone_id, product_id)
);

CREATE TABLE IF NOT EXISTS processed_events (
    event_id text PRIMARY KEY,
    processed_at timestamp
);

CREATE TABLE IF NOT EXISTS orders (
    order_id text PRIMARY KEY,
    status text,
    last_event_ms bigint,
    lines_json text
);

-- Опциональная история для аудита (append-only по времени внутри партиции заказа / товара)
CREATE TABLE IF NOT EXISTS event_history (
    entity_type text,
    entity_id text,
    event_id text,
    event_type text,
    occurred_at_ms bigint,
    payload_json text,
    PRIMARY KEY ((entity_type, entity_id), occurred_at_ms, event_id)
) WITH CLUSTERING ORDER BY (occurred_at_ms DESC, event_id ASC);
