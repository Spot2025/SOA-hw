#!/usr/bin/env python3
import json
import os
import sys
import urllib.error
import urllib.request

SR = os.environ.get("SCHEMA_REGISTRY_URL", "http://schema-registry:8081").rstrip("/")
SUBJECT = os.environ.get("SUBJECT", "warehouse-events-value")
PATH = os.environ.get("AVRO_SCHEMA_PATH", "/schemas/WarehouseEvent.avsc")


def main() -> None:
    with open(PATH, encoding="utf-8") as f:
        schema = json.load(f)
    payload = json.dumps({"schema": json.dumps(schema)})
    url = f"{SR}/subjects/{SUBJECT}/versions"
    req = urllib.request.Request(
        url,
        data=payload.encode("utf-8"),
        headers={"Content-Type": "application/vnd.schemaregistry.v1+json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read().decode("utf-8")
            print("Registered schema:", body)
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        if e.code in (409, 422) or "already exists" in body.lower() or "duplicate" in body.lower():
            print("Schema already registered or duplicate; continuing.", body)
            return
        print("Schema registration failed:", e.code, body, file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
