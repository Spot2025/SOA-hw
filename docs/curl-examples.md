# Примеры curl для проверки API

Booking Service слушает на **http://localhost:8080**. Выполняйте запросы после `docker compose up`.

---

## 1. Поиск рейсов

**Обязательные параметры:** `origin`, `destination`. **Опционально:** `date` (YYYY-MM-DD).

```bash
# С датой (после seed есть рейс SVO → LED на 2026-04-01)
curl -s "http://localhost:8080/flights?origin=SVO&destination=LED&date=2026-04-01" | jq

# Без даты — все рейсы по маршруту
curl -s "http://localhost:8080/flights?origin=SVO&destination=LED" | jq
```

Ожидание: массив `flights` (для seed — один рейс с `id: 1`).

---

## 2. Получить рейс по ID

```bash
# ID рейса из seed = 1
curl -s "http://localhost:8080/flights/1" | jq
```

Несуществующий рейс:
```bash
curl -s "http://localhost:8080/flights/999"
# 404, {"error":"flight not found"}
```

---

## 3. Создать бронирование

Подставьте свой `flight_id` (обычно `1` после seed).

```bash
curl -s -X POST http://localhost:8080/bookings \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "flight_id": 1,
    "passenger_name": "Ivan Ivanov",
    "passenger_email": "ivan@example.com",
    "seat_count": 2
  }' | jq
```

В ответе будет `id` бронирования (UUID) — сохраните его для следующих шагов.

---

## 4. Получить бронирование по ID

Подставьте `BOOKING_ID` из ответа создания.

```bash
export BOOKING_ID="<uuid-из-шага-3>"
curl -s "http://localhost:8080/bookings/$BOOKING_ID" | jq
```

---

## 5. Список бронирований пользователя

```bash
curl -s "http://localhost:8080/bookings?user_id=user-123" | jq
```

---

## 6. Отменить бронирование

```bash
curl -s -X POST "http://localhost:8080/bookings/$BOOKING_ID/cancel" | jq
# {"status":"CANCELLED"}
```

Повторная отмена:
```bash
curl -s -X POST "http://localhost:8080/bookings/$BOOKING_ID/cancel"
# 400, booking is not in CONFIRMED status
```

---

## Полный сценарий (копировать целиком)

```bash
# Поиск рейсов
curl -s "http://localhost:8080/flights?origin=SVO&destination=LED&date=2026-04-01" | jq

# Создание бронирования
RESP=$(curl -s -X POST http://localhost:8080/bookings \
  -H "Content-Type: application/json" \
  -d '{"user_id":"alice","flight_id":1,"passenger_name":"Alice","passenger_email":"alice@test.com","seat_count":1}')
echo "$RESP" | jq
BOOKING_ID=$(echo "$RESP" | jq -r '.id')
echo "Booking ID: $BOOKING_ID"

# Получить бронирование
curl -s "http://localhost:8080/bookings/$BOOKING_ID" | jq

# Список бронирований пользователя alice
curl -s "http://localhost:8080/bookings?user_id=alice" | jq

# Отмена
curl -s -X POST "http://localhost:8080/bookings/$BOOKING_ID/cancel" | jq

# Проверка: снова получить бронирование (status = CANCELLED)
curl -s "http://localhost:8080/bookings/$BOOKING_ID" | jq
```

`jq` опционален — без него ответ будет сырой JSON. Установка: `apt install jq` / `brew install jq`.
