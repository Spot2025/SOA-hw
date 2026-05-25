#!/usr/bin/env bash
set -euo pipefail

PROMETHEUS_URL="${PROMETHEUS_URL:-http://localhost:9090}"
MAX_ERROR_RATE="${MAX_ERROR_RATE:-0.01}"
MAX_P95_LATENCY="${MAX_P95_LATENCY:-0.5}"
OUTPUT_DIR="${OUTPUT_DIR:-./artifacts}"

mkdir -p "$OUTPUT_DIR"

query() {
  local q="$1"
  curl -sfG --data-urlencode "query=${q}" "${PROMETHEUS_URL}/api/v1/query" | jq -r '.data.result[0].value[1] // "0"'
}

echo "Waiting for Prometheus metrics..."
for i in $(seq 1 30); do
  if curl -sf "${PROMETHEUS_URL}/-/ready" >/dev/null; then
    break
  fi
  sleep 2
done

ERROR_RATE=$(query 'sum(rate(soa_hw_http_request_errors_total[2m])) / clamp_min(sum(rate(soa_hw_http_requests_total[2m])), 0.001)')
P95_LATENCY=$(query 'histogram_quantile(0.95, sum(rate(soa_hw_http_request_duration_seconds_bucket[2m])) by (le))')
REQUESTS=$(query 'sum(soa_hw_http_requests_total)')

echo "error_rate=${ERROR_RATE}" | tee "${OUTPUT_DIR}/metrics-validation.txt"
echo "p95_latency=${P95_LATENCY}" | tee -a "${OUTPUT_DIR}/metrics-validation.txt"
echo "total_requests=${REQUESTS}" | tee -a "${OUTPUT_DIR}/metrics-validation.txt"

curl -sfG --data-urlencode 'query=sum(rate(soa_hw_http_requests_total[2m]))' \
  "${PROMETHEUS_URL}/api/v1/query" > "${OUTPUT_DIR}/prometheus-throughput.json"
curl -sfG --data-urlencode 'query=histogram_quantile(0.95, sum(rate(soa_hw_http_request_duration_seconds_bucket[2m])) by (le))' \
  "${PROMETHEUS_URL}/api/v1/query" > "${OUTPUT_DIR}/prometheus-p95.json"

fail=0
if awk -v e="$ERROR_RATE" -v max="$MAX_ERROR_RATE" 'BEGIN { exit !(e+0 > max+0) }'; then
  echo "FAIL: error rate ${ERROR_RATE} exceeds ${MAX_ERROR_RATE}"
  fail=1
fi
if awk -v l="$P95_LATENCY" -v max="$MAX_P95_LATENCY" 'BEGIN { exit !(l+0 > max+0) }'; then
  echo "FAIL: p95 latency ${P95_LATENCY}s exceeds ${MAX_P95_LATENCY}s"
  fail=1
fi
if awk -v r="$REQUESTS" 'BEGIN { exit !(r+0 <= 0) }'; then
  echo "FAIL: no HTTP requests recorded in Prometheus"
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "Metrics validation passed"
