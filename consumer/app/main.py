from __future__ import annotations

import json
import logging
import os
import sys
import traceback
from datetime import datetime, timezone

from cassandra import ConsistencyLevel
from cassandra.cluster import Cluster
from confluent_kafka import Consumer, KafkaException, Producer
from confluent_kafka.schema_registry import SchemaRegistryClient
from confluent_kafka.schema_registry.avro import AvroDeserializer
from confluent_kafka.serialization import MessageField, SerializationContext

from app.processing import PreparedCQL, build_batch

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)
log = logging.getLogger("warehouse-consumer")


def _env(name: str, default: str | None = None) -> str:
    v = os.environ.get(name, default)
    if v is None or v == "":
        raise RuntimeError(f"Missing env var: {name}")
    return v


def _utc_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def main() -> None:
    bootstrap = _env("KAFKA_BOOTSTRAP_SERVERS")
    topic = _env("KAFKA_TOPIC", "warehouse-events")
    dlq_topic = _env("KAFKA_DLQ_TOPIC", "warehouse-events-dlq")
    group = _env("KAFKA_GROUP_ID", "warehouse-state-consumer")
    sr_url = _env("SCHEMA_REGISTRY_URL")
    cass_hosts = _env("CASSANDRA_HOSTS").split(",")
    cass_port = int(_env("CASSANDRA_PORT", "9042"))
    keyspace = _env("CASSANDRA_KEYSPACE", "warehouse")

    sr = SchemaRegistryClient({"url": sr_url})

    avro_deserializer = AvroDeserializer(
        sr,
        None,
        lambda obj, ctx: obj,
    )

    cluster = Cluster(cass_hosts, port=cass_port)
    session = cluster.connect()
    session.set_keyspace(keyspace)
    session.default_consistency_level = ConsistencyLevel.LOCAL_ONE

    prepared = PreparedCQL(session)

    consumer = Consumer(
        {
            "bootstrap.servers": bootstrap,
            "group.id": group,
            "enable.auto.commit": False,
            "auto.offset.reset": "earliest",
            "client.id": "warehouse-state-consumer-1",
        }
    )
    consumer.subscribe([topic])

    dlq_producer = Producer({"bootstrap.servers": bootstrap, "client.id": "warehouse-dlq-producer"})

    log.info("Consumer started: topic=%s group=%s", topic, group)

    try:
        while True:
            msg = consumer.poll(1.0)
            if msg is None:
                continue
            if msg.error():
                raise KafkaException(msg.error())

            partition = msg.partition()
            offset = msg.offset()
            ctx = SerializationContext(topic, MessageField.VALUE)

            try:
                event_obj = avro_deserializer(msg.value(), ctx)
                if not isinstance(event_obj, dict):
                    raise ValueError("Unexpected Avro payload type")
                event = {str(k): v for k, v in event_obj.items()}
            except Exception as e:  # noqa: BLE001 - DLQ path
                _send_dlq(
                    dlq_producer,
                    dlq_topic,
                    original=None,
                    error_code="DESERIALIZATION_ERROR",
                    error_reason=str(e),
                    partition=partition,
                    offset=offset,
                    stack=traceback.format_exc(),
                )
                consumer.commit(message=msg, asynchronous=False)
                continue

            event_id = str(event.get("event_id", ""))
            event_type = str(event.get("event_type", ""))

            try:
                status, batch = build_batch(session, prepared, event)
                if batch is not None:
                    session.execute(batch)
                if status == "duplicate":
                    log.info(
                        "skipped_duplicate event_id=%s event_type=%s partition=%s offset=%s",
                        event_id,
                        event_type,
                        partition,
                        offset,
                    )
                elif status == "stale":
                    log.info(
                        "skipped_stale event_id=%s event_type=%s partition=%s offset=%s",
                        event_id,
                        event_type,
                        partition,
                        offset,
                    )
                else:
                    log.info(
                        "processed event_id=%s event_type=%s partition=%s offset=%s",
                        event_id,
                        event_type,
                        partition,
                        offset,
                    )
                consumer.commit(message=msg, asynchronous=False)
            except ValueError as e:
                _send_dlq(
                    dlq_producer,
                    dlq_topic,
                    original=event,
                    error_code="VALIDATION_ERROR",
                    error_reason=str(e),
                    partition=partition,
                    offset=offset,
                    stack=traceback.format_exc(),
                )
                dlq_producer.flush(10)
                consumer.commit(message=msg, asynchronous=False)
                log.warning("sent_to_dlq event_id=%s reason=%s", event_id, e)
            except Exception as e:  # noqa: BLE001
                _send_dlq(
                    dlq_producer,
                    dlq_topic,
                    original=event,
                    error_code="PROCESSING_ERROR",
                    error_reason=str(e),
                    partition=partition,
                    offset=offset,
                    stack=traceback.format_exc(),
                )
                dlq_producer.flush(10)
                consumer.commit(message=msg, asynchronous=False)
                log.exception("sent_to_dlq event_id=%s", event_id)
    finally:
        consumer.close()
        cluster.shutdown()


def _send_dlq(
    producer: Producer,
    dlq_topic: str,
    *,
    original: dict | None,
    error_code: str,
    error_reason: str,
    partition: int,
    offset: int,
    stack: str | None = None,
) -> None:
    payload = {
        "original_event": original,
        "error_reason": error_reason,
        "error_code": error_code,
        "failed_at": _utc_iso(),
        "kafka_metadata": {"partition": partition, "offset": offset},
    }
    if stack:
        payload["stacktrace"] = stack
    producer.produce(
        dlq_topic,
        value=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
    )


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(0)
