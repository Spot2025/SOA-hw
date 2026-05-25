#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-60}"

echo "Waiting for services at ${BASE_URL}..."
for i in $(seq 1 "$MAX_ATTEMPTS"); do
  if curl -sf "${BASE_URL}/health" >/dev/null; then
    echo "Services are ready"
    exit 0
  fi
  sleep 2
done

echo "Services did not become ready in time"
exit 1
