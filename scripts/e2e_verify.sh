#!/usr/bin/env bash
# End-to-end проверки сценариев 1–5 (баллы 1–7). Сценарии 6–8 — для блока 8–10 (кластер, Grafana).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose)

publish_json() {
  local tmp="${ROOT}/scripts/.e2e_last_event.json"
  cat > "$tmp"
  "${COMPOSE[@]}" run --rm --no-deps \
    -v "${ROOT}/schemas:/schemas:ro" \
    -v "${ROOT}/scripts:/evt:ro" \
    --entrypoint python \
    producer -m app.publish_cli "/evt/.e2e_last_event.json"
  rm -f "$tmp"
}

cql() {
  "${COMPOSE[@]}" exec -T cassandra cqlsh -e "$1"
}

# Парсим строку данных cqlsh вида "       100 |        0"
parse_row_two_ints() {
  awk -F'|' '
    NF >= 2 {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", $1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
      if ($1 ~ /^[0-9]+$/ && $2 ~ /^[0-9]+$/) { print $1, $2; exit }
    }'
}

get_pz() {
  local sku="$1" zone="$2"
  cql "SELECT available, reserved FROM warehouse.inventory_by_product_zone WHERE product_id='${sku}' AND zone_id='${zone}';" | parse_row_two_ints
}

get_agg() {
  local sku="$1"
  cql "SELECT total_available, total_reserved FROM warehouse.inventory_by_product WHERE product_id='${sku}';" | parse_row_two_ints
}

get_zinv() {
  local zone="$1" sku="$2"
  cql "SELECT available, reserved FROM warehouse.inventory_by_zone WHERE zone_id='${zone}' AND product_id='${sku}';" | parse_row_two_ints
}

assert_zinv() {
  local zone="$1" sku="$2" want_a="$3" want_r="$4"
  read -r a r <<< "$(get_zinv "$zone" "$sku")"
  if [[ "$a" != "$want_a" || "$r" != "$want_r" ]]; then
    echo "FAIL inventory_by_zone $zone $sku: expected available=$want_a reserved=$want_r, got available=$a reserved=$r" >&2
    exit 1
  fi
}

assert_pz() {
  local sku="$1" zone="$2" want_a="$3" want_r="$4"
  read -r a r <<< "$(get_pz "$sku" "$zone")"
  if [[ "$a" != "$want_a" || "$r" != "$want_r" ]]; then
    echo "FAIL inventory_by_product_zone $sku $zone: expected available=$want_a reserved=$want_r, got available=$a reserved=$r" >&2
    exit 1
  fi
}

assert_agg() {
  local sku="$1" want_a="$2" want_r="$3"
  read -r a r <<< "$(get_agg "$sku")"
  if [[ "$a" != "$want_a" || "$r" != "$want_r" ]]; then
    echo "FAIL inventory_by_product $sku: expected total_available=$want_a total_reserved=$want_r, got $a $r" >&2
    exit 1
  fi
}

echo "== Остановка producer (чтобы демо не мешало событиям) =="
"${COMPOSE[@]}" stop producer 2>/dev/null || true

RUN_ID="$(date +%s)-$$"

echo "== Ожидание Cassandra =="
for _ in $(seq 1 60); do
  if "${COMPOSE[@]}" exec -T cassandra cqlsh -e "DESCRIBE KEYSPACES" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

echo ""
echo "=== Сценарий 1: базовый цикл склада ==="
S1="E2E-S1-${RUN_ID}-SKU"
OID="$(uuidgen)"
T="$(date +%s)000"

publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_RECEIVED","occurred_at":$T,"product_id":"$S1","zone_id":"ZONE-A","quantity":100}
EOF

sleep 0.5
assert_pz "$S1" "ZONE-A" 100 0
assert_agg "$S1" 100 0

T=$((T + 1000))
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_RESERVED","occurred_at":$T,"product_id":"$S1","zone_id":"ZONE-A","quantity":30}
EOF
sleep 0.5
assert_pz "$S1" "ZONE-A" 70 30

T=$((T + 1000))
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_MOVED","occurred_at":$T,"product_id":"$S1","from_zone_id":"ZONE-A","to_zone_id":"ZONE-B","quantity":20}
EOF
sleep 0.5
assert_pz "$S1" "ZONE-A" 50 30
assert_pz "$S1" "ZONE-B" 20 0
assert_agg "$S1" 70 30

T=$((T + 1000))
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_SHIPPED","occurred_at":$T,"product_id":"$S1","zone_id":"ZONE-A","quantity":10}
EOF
sleep 0.5
assert_pz "$S1" "ZONE-A" 40 30

T=$((T + 1000))
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"ORDER_CREATED","occurred_at":$T,"order_id":"$OID","order_lines":[{"product_id":"$S1","zone_id":"ZONE-A","quantity":15}]}
EOF
sleep 0.5
assert_pz "$S1" "ZONE-A" 25 45

T=$((T + 1000))
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"ORDER_COMPLETED","occurred_at":$T,"order_id":"$OID"}
EOF
sleep 0.5
assert_pz "$S1" "ZONE-A" 25 30
echo "Сценарий 1: OK"

echo ""
echo "=== Сценарий 2: идемпотентность ==="
S2="E2E-S2-${RUN_ID}-SKU"
EID="$(uuidgen)"
T="$(date +%s)000"
publish_json <<EOF
{"event_id":"$EID","event_type":"PRODUCT_RECEIVED","occurred_at":$T,"product_id":"$S2","zone_id":"ZONE-A","quantity":50}
EOF
sleep 0.5
assert_pz "$S2" "ZONE-A" 50 0
publish_json <<EOF
{"event_id":"$EID","event_type":"PRODUCT_RECEIVED","occurred_at":$T,"product_id":"$S2","zone_id":"ZONE-A","quantity":50}
EOF
sleep 0.5
assert_pz "$S2" "ZONE-A" 50 0
echo "Сценарий 2: OK"

echo ""
echo "=== Сценарий 3: три таблицы согласованы ==="
S3="E2E-S3-${RUN_ID}-SKU"
T="$(date +%s)000"
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_RECEIVED","occurred_at":$T,"product_id":"$S3","zone_id":"ZONE-A","quantity":100}
EOF
sleep 0.5
assert_pz "$S3" "ZONE-A" 100 0
assert_agg "$S3" 100 0
assert_zinv "ZONE-A" "$S3" 100 0
echo "Сценарий 3: OK"

echo ""
echo "=== Сценарий 4: события вне порядка (timestamp) ==="
S4="E2E-S4-${RUN_ID}-SKU"
BASE=1700000000000
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_RECEIVED","occurred_at":$BASE,"product_id":"$S4","zone_id":"ZONE-A","quantity":100}
EOF
sleep 0.3
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_SHIPPED","occurred_at":$((BASE + 300000)),"product_id":"$S4","zone_id":"ZONE-A","quantity":20}
EOF
sleep 0.5
assert_pz "$S4" "ZONE-A" 80 0
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_RECEIVED","occurred_at":$((BASE + 120000)),"product_id":"$S4","zone_id":"ZONE-A","quantity":50}
EOF
sleep 0.5
assert_pz "$S4" "ZONE-A" 80 0
echo "Сценарий 4: OK"

echo ""
echo "=== Сценарий 5: DLQ + восстановление после ошибки ==="
S5="E2E-S5-${RUN_ID}-SKU"
T="$(date +%s)000"
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_SHIPPED","occurred_at":$T,"product_id":"$S5","zone_id":"ZONE-A","quantity":-5}
EOF
sleep 1
publish_json <<EOF
{"event_id":"$(uuidgen)","event_type":"PRODUCT_RECEIVED","occurred_at":$((T + 1000)),"product_id":"$S5","zone_id":"ZONE-A","quantity":10}
EOF
sleep 0.5
assert_pz "$S5" "ZONE-A" 10 0
if ! "${COMPOSE[@]}" exec -T kafka kafka-console-consumer \
  --bootstrap-server kafka:29092 \
  --topic warehouse-events-dlq \
  --from-beginning \
  --max-messages 500 \
  --timeout-ms 15000 2>/dev/null | grep -q "$S5"; then
  echo "FAIL: не найдено сообщение DLQ для $S5 (проверьте, что consumer обработал invalid событие)" >&2
  exit 1
fi
echo "Сценарий 5: OK"

echo ""
echo "Все сценарии 1–5 прошли успешно."
