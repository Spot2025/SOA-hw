# Шпаргалка для защиты ДЗ №7 (9 баллов)

Система: распределённое бронирование авиабилетов — `booking-service` (REST) + `flight-service` (gRPC) + PostgreSQL ×2 + Redis + Prometheus + Grafana + Alertmanager.

---

## 1. Перед защитой — что подготовить

- Репозиторий с веткой, где есть все файлы CI/CD и мониторинга
- Docker и Docker Compose установлены локально
- Аккаунт GitHub с доступом к Actions (для демонстрации CI)
- Браузер с закладками:
  - Grafana: http://localhost:3000 (admin / admin)
  - Prometheus: http://localhost:9090
  - Alertmanager: http://localhost:9093
  - API: http://localhost:8080

---

## 2. Демонстрация E2E (обязательно на защите)

### Шаг 1: Поднять систему одной командой

```bash
cd /path/to/SOA-hw
docker compose up --build
```

**Что показать ассистенту:**
- В логах стартуют postgres, redis, migrate, flight-service, booking-service, prometheus, grafana, alertmanager
- `booking-service` слушает `:8080`, `flight-service` gRPC `:50051`, метрики flight на `:9090`

### Шаг 2: Проверить health и метрики

```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics | head
curl http://localhost:9090/metrics | head
# Prometheus UI: http://localhost:9090/targets
```

### Шаг 3: Полный пользовательский сценарий (curl)

```bash
curl -s "http://localhost:8080/flights?origin=SVO&destination=LED&date=2026-04-01" | jq

RESP=$(curl -s -X POST http://localhost:8080/bookings \
  -H "Content-Type: application/json" \
  -d '{"user_id":"demo","flight_id":1,"passenger_name":"Demo","passenger_email":"demo@test.com","seat_count":1}')
echo "$RESP" | jq
BOOKING_ID=$(echo "$RESP" | jq -r '.id')

docker compose exec postgres-booking psql -U postgres -d booking_db -c "SELECT id, status FROM bookings WHERE id='$BOOKING_ID';"
docker compose exec postgres-flight psql -U postgres -d flight_db -c "SELECT status FROM seat_reservations WHERE booking_id='$BOOKING_ID';"

curl -s -X POST "http://localhost:8080/bookings/$BOOKING_ID/cancel" | jq
```

---

## 3. Демонстрация CI pipeline

### Локально

```bash
go test -short ./booking-service/... ./flight-service/... ./pkg/...
go test ./tests/integration/...
docker compose up -d --build --wait
bash scripts/wait-for-services.sh
go test ./tests/e2e/...
k6 run loadtests/booking.js
bash scripts/validate-metrics.sh
```

### На GitHub Actions

1. Вкладка **Actions** → workflow **CI**
2. Jobs: `build`, `unit-tests`, `integration-tests`, `e2e-load-metrics`

**Как показать падение CI:** временно ужесточить порог в `loadtests/booking.js` или сломать тест → push → красный pipeline.

---

## 4. Prometheus и Grafana

### Prometheus (http://localhost:9090)

- **Status → Targets** — все UP
- Примеры PromQL:

```promql
sum(rate(soa_hw_http_requests_total[1m]))
sum(rate(soa_hw_http_request_errors_total[1m])) / sum(rate(soa_hw_http_requests_total[1m]))
histogram_quantile(0.95, sum(rate(soa_hw_http_request_duration_seconds_bucket[1m])) by (le))
```

### Grafana (http://localhost:3000, admin/admin)

Дашборды в папке **SOA-HW**:
1. Booking Service
2. Flight Service
3. Infrastructure

Трафик для графиков: `k6 run loadtests/booking.js`

---

## 5. Alertmanager — firing alert

```bash
docker compose stop booking-service
# ~30 сек → http://localhost:9090/alerts → BookingServiceDown Firing
# http://localhost:9093 — alert в UI
docker compose start booking-service
```

Файлы: `monitoring/prometheus/alerts.yml`, `monitoring/alertmanager/alertmanager.yml`

---

## 6. Структура CI

| Job | Что делает |
|-----|-----------|
| `build` | go build обоих сервисов |
| `unit-tests` | handlers, repository (-short) |
| `integration-tests` | HTTP → gRPC → 2×PG + Redis |
| `e2e-load-metrics` | compose → E2E → k6 → validate-metrics |

**k6:** 10 VU, 30s, p95 < 500ms, errors < 5%  
**validate-metrics.sh:** error rate < 1%, p95 < 500ms

---

## 7. Ключевые файлы для разбора кода

| Файл | Назначение |
|------|-----------|
| `.github/workflows/ci.yml` | CI pipeline |
| `pkg/metrics/` | метрики + middleware |
| `tests/integration/` | интеграционные тесты |
| `tests/e2e/` | E2E тест |
| `loadtests/booking.js` | нагрузка |
| `scripts/validate-metrics.sh` | проверка метрик в CI |
| `monitoring/` | Prometheus, Grafana, alerts |
| `docker-compose.yml` | весь стек одной командой |

---

## 8. Метрики

**booking-service (HTTP):** `soa_hw_http_requests_total`, `soa_hw_http_request_errors_total`, `soa_hw_http_request_duration_seconds`

**flight-service (gRPC):** `soa_hw_grpc_requests_total`, `soa_hw_grpc_request_errors_total`, `soa_hw_grpc_request_duration_seconds`

---

## 9. Теория — кратко

- **Unit / Integration / E2E** — изоляция → межсервисное → полный сценарий
- **Counter / Histogram** — requests vs latency percentiles
- **Prometheus pull** — scrape `/metrics`
- **Alert `for:`** — задержка перед firing
- **k6 VU** — виртуальные пользователи
- **Testcontainers** — Docker в тестах

---

## 10. Порты

| Сервис | Порт |
|--------|------|
| booking-service | 8080 |
| flight-service gRPC | 50051 |
| flight-service metrics (host) | 9101 |
| PostgreSQL flight / booking | 5434 / 5435 |
| Prometheus | 9090 |
| Alertmanager | 9093 |
| Grafana | 3000 |
