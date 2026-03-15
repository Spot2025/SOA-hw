# Распределённая система бронирования авиабилетов

Два микросервиса на Go: **Booking Service** (REST) и **Flight Service** (gRPC), с кешированием в Redis.

## Запуск

```bash
# Сгенерировать код из .proto (нужен protoc и protoc-gen-go, protoc-gen-go-grpc)
make proto

# Поднять всё одной командой
docker compose up --build
```

Миграции применяются автоматически перед стартом приложений: контейнеры **migrate-flight** и **migrate-booking** (образ `migrate/migrate:v4.17.0`) поднимают БД и выполняют `migrate up` по папкам `flight-service/migrations` и `booking-service/migrations`; flight-service и booking-service стартуют только после успешного завершения миграций.

## Переменные окружения

- **FLIGHT_GRPC_API_KEY** — секретный ключ для аутентификации gRPC-вызовов (Booking → Flight). Один и тот же ключ должен быть задан в обоих сервисах.
  - **Через docker compose:** переменная берётся с хоста. Если не задана, подставляется `secret-api-key`.
  - Задать свой ключ: перед `docker compose up` выполнить `export FLIGHT_GRPC_API_KEY=твой-секрет` или создать файл `.env` в корне проекта с строкой `FLIGHT_GRPC_API_KEY=твой-секрет` (docker compose подхватывает `.env` автоматически).
  - Локальный запуск без Docker: в обоих процессах (flight-service и booking-service) выставить одну и ту же переменную окружения.
- **CIRCUIT_BREAKER_THRESHOLD**, **CIRCUIT_BREAKER_TIMEOUT_SEC** — параметры Circuit Breaker в Booking Service.
- **RETRY_MAX_ATTEMPTS** — число повторов вызовов Flight Service (по умолчанию 3).
- **CACHE_TTL_MIN** — TTL кеша в минутах во Flight Service (5–10).

## API

### Booking Service (REST), порт 8080

| Метод | Путь | Описание |
|-------|------|----------|
| GET | /flights?origin=SVO&destination=LED&date=2026-04-01 | Поиск рейсов |
| GET | /flights/{id} | Рейс по ID |
| POST | /bookings | Создать бронирование (body: user_id, flight_id, passenger_name, passenger_email, seat_count) |
| GET | /bookings/{id} | Бронирование по ID |
| POST | /bookings/{id}/cancel | Отменить бронирование |
| GET | /bookings?user_id=X | Список бронирований пользователя |

### Примеры curl

Подробный набор запросов для проверки всех эндпоинтов — в **[docs/curl-examples.md](docs/curl-examples.md)** (поиск рейсов, создание/получение/отмена бронирования, список по user_id и полный сценарий).

Быстрый старт:

```bash
# Поиск рейсов
curl "http://localhost:8080/flights?origin=SVO&destination=LED&date=2026-04-01"

# Создать бронирование (flight_id=1 после seed)
curl -X POST http://localhost:8080/bookings \
  -H "Content-Type: application/json" \
  -d '{"user_id":"u1","flight_id":1,"passenger_name":"Ivan","passenger_email":"i@example.com","seat_count":2}'

# Список бронирований
curl "http://localhost:8080/bookings?user_id=u1"
```

## Структура

- **proto/** — .proto файлы и сгенерированный Go-код для Flight Service.
- **flight-service/** — gRPC-сервер, PostgreSQL, Redis (Cache-Aside), аутентификация по API Key.
- **booking-service/** — REST API, PostgreSQL, gRPC-клиент с Retry и Circuit Breaker.
- **docs/er-diagram.md** — ER-диаграмма (Mermaid), 3NF.

## Реализованные пункты

- 1–4: gRPC-контракт, ER 3NF, оба сервиса, миграции, межсервисный gRPC.
- 5–7: транзакции и SELECT FOR UPDATE, аутентификация (API Key в metadata), Redis Cache-Aside с инвалидацией.
- 8–10: Retry с exponential backoff и идемпотентность ReserveSeats, Circuit Breaker в Booking Service. Redis в docker-compose (одиночный инстанс; для п.9 — Sentinel/Cluster отдельно).
