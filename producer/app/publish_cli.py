"""Publish a single WarehouseEvent (JSON) to Kafka as Avro. Used for manual / E2E tests."""

from __future__ import annotations

import argparse
import json
import os
import sys

from confluent_kafka import Producer
from confluent_kafka.schema_registry import SchemaRegistryClient
from confluent_kafka.schema_registry.avro import AvroSerializer
from confluent_kafka.serialization import MessageField, SerializationContext


def _env(name: str, default: str | None = None) -> str:
    v = os.environ.get(name, default)
    if v is None or v == "":
        raise RuntimeError(f"Missing env var: {name}")
    return v


def main() -> None:
    ap = argparse.ArgumentParser(description="Publish one WarehouseEvent JSON to Kafka (Avro)")
    ap.add_argument(
        "path",
        nargs="?",
        default="-",
        help="Path to JSON file, or '-' / omit for stdin",
    )
    args = ap.parse_args()

    if args.path in ("-", ""):
        event = json.load(sys.stdin)
    else:
        with open(args.path, encoding="utf-8") as f:
            event = json.load(f)

    bootstrap = _env("KAFKA_BOOTSTRAP_SERVERS")
    topic = _env("KAFKA_TOPIC", "warehouse-events")
    sr_url = _env("SCHEMA_REGISTRY_URL")
    schema_path = _env("AVRO_SCHEMA_PATH", "/schemas/WarehouseEvent.avsc")

    with open(schema_path, encoding="utf-8") as f:
        schema_str = f.read()

    sr = SchemaRegistryClient({"url": sr_url})
    serializer = AvroSerializer(
        sr,
        schema_str,
        lambda obj, sc: obj,
        conf={"auto.register.schemas": False},
    )

    producer = Producer({"bootstrap.servers": bootstrap, "client.id": "wms-publish-cli"})
    ctx = SerializationContext(topic, MessageField.VALUE)
    payload = serializer(event, ctx)

    producer.produce(topic, value=payload)
    producer.poll(0)
    producer.flush(30)
    print(json.dumps({"ok": True, "event_id": event.get("event_id")}, ensure_ascii=False))


if __name__ == "__main__":
    main()
