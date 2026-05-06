from __future__ import annotations

import logging
import os
import time
import uuid
from datetime import datetime, timezone
from typing import Any

from confluent_kafka import Producer
from confluent_kafka.schema_registry import SchemaRegistryClient
from confluent_kafka.schema_registry.avro import AvroSerializer
from confluent_kafka.serialization import MessageField, SerializationContext

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("wms-producer")


def _env(name: str, default: str | None = None) -> str:
    v = os.environ.get(name, default)
    if v is None or v == "":
        raise RuntimeError(f"Missing env var: {name}")
    return v


def _load_schema(path: str) -> str:
    with open(path, encoding="utf-8") as f:
        return f.read()


def _now_ms() -> int:
    return int(datetime.now(timezone.utc).timestamp() * 1000)


def make_event(event_type: str, occurred_at: int, **fields: Any) -> dict[str, Any]:
    e: dict[str, Any] = {
        "event_id": str(uuid.uuid4()),
        "event_type": event_type,
        "occurred_at": occurred_at,
        "product_id": None,
        "zone_id": None,
        "from_zone_id": None,
        "to_zone_id": None,
        "quantity": None,
        "counted_quantity": None,
        "order_id": None,
        "order_lines": None,
    }
    for k, v in fields.items():
        if k in e:
            e[k] = v
        else:
            e[k] = v
    return e


def main() -> None:
    bootstrap = _env("KAFKA_BOOTSTRAP_SERVERS")
    topic = _env("KAFKA_TOPIC", "warehouse-events")
    sr_url = _env("SCHEMA_REGISTRY_URL")
    schema_path = _env("AVRO_SCHEMA_PATH", "/schemas/WarehouseEvent.avsc")
    mode = _env("PRODUCER_MODE", "demo")

    schema_str = _load_schema(schema_path)
    sr = SchemaRegistryClient({"url": sr_url})
    serializer = AvroSerializer(
        sr,
        schema_str,
        lambda obj, sc: obj,
        conf={"auto.register.schemas": False},
    )

    producer = Producer({"bootstrap.servers": bootstrap, "client.id": "wms-producer"})

    def delivery_report(err: Any, msg: Any) -> None:
        if err is not None:
            log.error("Delivery failed: %s", err)
        else:
            log.debug("Delivered to %s [%s] @ %s", msg.topic(), msg.partition(), msg.offset())

    ctx = SerializationContext(topic, MessageField.VALUE)

    def send(ev: dict[str, Any]) -> None:
        payload = serializer(ev, ctx)
        producer.produce(topic, value=payload, on_delivery=delivery_report)
        producer.poll(0)
        producer.flush(10)

    if mode == "demo":
        log.info("Producer demo mode: scenario events + periodic traffic")
        base = _now_ms()
        # Scenario 1 style flow
        send(
            make_event(
                "PRODUCT_RECEIVED",
                base,
                product_id="SKU-001",
                zone_id="ZONE-A",
                quantity=100,
            )
        )
        time.sleep(0.2)
        send(
            make_event(
                "PRODUCT_RESERVED",
                base + 1000,
                product_id="SKU-001",
                zone_id="ZONE-A",
                quantity=30,
            )
        )
        time.sleep(0.2)
        send(
            make_event(
                "PRODUCT_MOVED",
                base + 2000,
                product_id="SKU-001",
                from_zone_id="ZONE-A",
                to_zone_id="ZONE-B",
                quantity=20,
            )
        )
        time.sleep(0.2)
        send(
            make_event(
                "PRODUCT_SHIPPED",
                base + 3000,
                product_id="SKU-001",
                zone_id="ZONE-A",
                quantity=10,
            )
        )
        time.sleep(0.2)
        oid = str(uuid.uuid4())
        send(
            make_event(
                "ORDER_CREATED",
                base + 4000,
                order_id=oid,
                order_lines=[
                    {"product_id": "SKU-001", "zone_id": "ZONE-A", "quantity": 15},
                ],
            )
        )
        time.sleep(0.2)
        send(
            make_event(
                "ORDER_COMPLETED",
                base + 5000,
                order_id=oid,
            )
        )

        # Idempotency / duplicate (same event_id)
        dup = make_event(
            "PRODUCT_RECEIVED",
            base + 6000,
            product_id="SKU-002",
            zone_id="ZONE-A",
            quantity=50,
        )
        dup_id = dup["event_id"]
        send(dup)
        dup2 = dict(dup)
        dup2["event_id"] = dup_id
        send(dup2)

        # Out-of-order: newer first, then stale
        t0 = base + 10_000
        send(
            make_event(
                "PRODUCT_RECEIVED",
                t0,
                product_id="SKU-004",
                zone_id="ZONE-A",
                quantity=100,
            )
        )
        send(
            make_event(
                "PRODUCT_SHIPPED",
                t0 + 300_000,
                product_id="SKU-004",
                zone_id="ZONE-A",
                quantity=20,
            )
        )
        send(
            make_event(
                "PRODUCT_RECEIVED",
                t0 + 120_000,
                product_id="SKU-004",
                zone_id="ZONE-A",
                quantity=50,
            )
        )

        # Invalid -> DLQ
        send(
            make_event(
                "PRODUCT_SHIPPED",
                t0 + 400_000,
                product_id="SKU-005",
                zone_id="ZONE-A",
                quantity=-5,
            )
        )

        # Valid after invalid
        send(
            make_event(
                "PRODUCT_RECEIVED",
                t0 + 500_000,
                product_id="SKU-005",
                zone_id="ZONE-A",
                quantity=10,
            )
        )

        log.info("Demo seed complete; sleeping (producer stays up for manual tests)")
        while True:
            time.sleep(3600)
    else:
        log.info("Producer mode=%s (only demo implemented)", mode)


if __name__ == "__main__":
    main()
