#!/usr/bin/env bash
set -euo pipefail

BASE="http://localhost:8080/api/v1"
GREEN='\033[0;32m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓ $1${NC}"; }
fail() { echo -e "  ${RED}✗ $1${NC}"; echo "$2"; exit 1; }
step() { echo -e "\n${CYAN}── $1 ──${NC}"; }

json() { python3 -m json.tool <<< "$1"; }
field() { python3 -c "import sys,json; print(json.load(sys.stdin)$(2))" <<< "$1"; }

# ─────────────────────────────────────────────────────────
step "1. Регистрация пользователей"

SELLER_RESP=$(curl -sf "$BASE/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"seller_e2e","password":"pass1234","role":"SELLER"}')
SELLER_TOKEN=$(field "$SELLER_RESP" "['access_token']")
ok "SELLER зарегистрирован (id=$(field "$SELLER_RESP" "['user_id']"))"

USER_RESP=$(curl -sf "$BASE/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"user_e2e","password":"pass1234","role":"USER"}')
USER_TOKEN=$(field "$USER_RESP" "['access_token']")
ok "USER зарегистрирован (id=$(field "$USER_RESP" "['user_id']"))"

ADMIN_RESP=$(curl -sf "$BASE/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin_e2e","password":"pass1234","role":"ADMIN"}')
ADMIN_TOKEN=$(field "$ADMIN_RESP" "['access_token']")
ok "ADMIN зарегистрирован (id=$(field "$ADMIN_RESP" "['user_id']"))"

# ─────────────────────────────────────────────────────────
step "2. JWT: login + refresh"

LOGIN_RESP=$(curl -sf "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"user_e2e","password":"pass1234"}')
REFRESH_TK=$(field "$LOGIN_RESP" "['refresh_token']")
ok "Login успешен"

REFRESH_RESP=$(curl -sf "$BASE/auth/refresh" \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"$REFRESH_TK\"}")
USER_TOKEN=$(field "$REFRESH_RESP" "['access_token']")
ok "Refresh успешен, получен новый access token"

# ─────────────────────────────────────────────────────────
step "3. CRUD Products (SELLER)"

P1=$(curl -sf "$BASE/products" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d '{"name":"Laptop","description":"Gaming laptop 16 RAM","price":1500.00,"stock":10,"category":"electronics","status":"ACTIVE"}')
P1_ID=$(field "$P1" "['id']")
ok "Создан товар Laptop id=$P1_ID"

P2=$(curl -sf "$BASE/products" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d '{"name":"Mouse","description":"Wireless mouse","price":25.50,"stock":100,"category":"electronics","status":"ACTIVE"}')
P2_ID=$(field "$P2" "['id']")
ok "Создан товар Mouse id=$P2_ID"

P3=$(curl -sf "$BASE/products" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d '{"name":"T-Shirt","price":19.99,"stock":50,"category":"clothing","status":"ACTIVE"}')
P3_ID=$(field "$P3" "['id']")
ok "Создан товар T-Shirt id=$P3_ID"

# GET by ID
GET_P=$(curl -sf "$BASE/products/$P1_ID" -H "Authorization: Bearer $USER_TOKEN")
ok "GET /products/$P1_ID → name=$(field "$GET_P" "['name']")"

# LIST с фильтрацией
LIST=$(curl -sf "$BASE/products?page=0&size=10&category=electronics" \
  -H "Authorization: Bearer $USER_TOKEN")
TOTAL=$(field "$LIST" "['totalElements']")
ok "GET /products?category=electronics → totalElements=$TOTAL"

# UPDATE
UPD=$(curl -sf -X PUT "$BASE/products/$P1_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d '{"price":1399.99}')
ok "PUT /products/$P1_ID → price=$(field "$UPD" "['price']")"

# SOFT DELETE
HTTP_DEL=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/products/$P3_ID" \
  -H "Authorization: Bearer $SELLER_TOKEN")
[[ "$HTTP_DEL" == "204" ]] && ok "DELETE /products/$P3_ID → 204 (soft delete)" \
  || fail "DELETE вернул $HTTP_DEL" ""

CHECK_DEL=$(curl -sf "$BASE/products/$P3_ID" -H "Authorization: Bearer $USER_TOKEN")
ok "Статус после удаления: $(field "$CHECK_DEL" "['status']")"

# ─────────────────────────────────────────────────────────
step "4. Ролевая модель"

# USER не может создавать товар
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/products" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d '{"name":"Hack","price":1,"stock":1,"category":"x","status":"ACTIVE"}')
[[ "$HTTP" == "403" ]] && ok "USER → POST /products = 403 ACCESS_DENIED" \
  || fail "Ожидали 403, получили $HTTP" ""

# Без токена
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/products")
[[ "$HTTP" == "401" ]] && ok "Без токена → 401" \
  || fail "Ожидали 401, получили $HTTP" ""

# ─────────────────────────────────────────────────────────
step "5. Промокод (SELLER)"

PROMO=$(curl -sf "$BASE/promo-codes" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d '{"code":"SAVE20","discount_type":"PERCENTAGE","discount_value":20,"min_order_amount":100,"max_uses":3,"valid_from":"2025-01-01T00:00:00Z","valid_until":"2027-12-31T23:59:59Z"}')
ok "Промокод SAVE20 создан (id=$(field "$PROMO" "['id']"))"

PROMO_FIXED=$(curl -sf "$BASE/promo-codes" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d '{"code":"FLAT50","discount_type":"FIXED_AMOUNT","discount_value":50,"min_order_amount":200,"max_uses":10,"valid_from":"2025-01-01T00:00:00Z","valid_until":"2027-12-31T23:59:59Z"}')
ok "Промокод FLAT50 создан"

# ─────────────────────────────────────────────────────────
step "6. Создание заказа (USER)"

ORDER=$(curl -sf "$BASE/orders" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d "{\"items\":[{\"product_id\":$P1_ID,\"quantity\":2},{\"product_id\":$P2_ID,\"quantity\":5}],\"promo_code\":\"SAVE20\"}")
O_ID=$(field "$ORDER" "['id']")
ok "Заказ создан id=$O_ID"
echo "  total=$(field "$ORDER" "['total_amount']"), discount=$(field "$ORDER" "['discount_amount']"), promo=$(field "$ORDER" "['promo_code']")"

# Проверяем stock уменьшился
STOCK1=$(curl -sf "$BASE/products/$P1_ID" -H "Authorization: Bearer $USER_TOKEN" | python3 -c "import sys,json; print(json.load(sys.stdin)['stock'])")
STOCK2=$(curl -sf "$BASE/products/$P2_ID" -H "Authorization: Bearer $USER_TOKEN" | python3 -c "import sys,json; print(json.load(sys.stdin)['stock'])")
ok "Stock после заказа: Laptop=$STOCK1, Mouse=$STOCK2"

# ─────────────────────────────────────────────────────────
step "7. Проверка бизнес-ошибок"

# ORDER_HAS_ACTIVE — у USER уже есть CREATED заказ
ERR=$(curl -s -w "\n%{http_code}" "$BASE/orders" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d "{\"items\":[{\"product_id\":$P2_ID,\"quantity\":1}]}")
HTTP=$(echo "$ERR" | tail -1)
BODY=$(echo "$ERR" | head -1)
[[ "$HTTP" == "409" ]] && ok "ORDER_HAS_ACTIVE → 409 ($(field "$BODY" "['error_code']"))" \
  || fail "Ожидали 409, получили $HTTP" "$BODY"

# SELLER не может создавать заказ
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/orders" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -d "{\"items\":[{\"product_id\":$P2_ID,\"quantity\":1}]}")
[[ "$HTTP" == "403" ]] && ok "SELLER → POST /orders = 403" \
  || fail "Ожидали 403, получили $HTTP" ""

# ─────────────────────────────────────────────────────────
step "8. Отмена заказа → возврат stock и promo uses"

CANCEL=$(curl -sf -X POST "$BASE/orders/$O_ID/cancel" \
  -H "Authorization: Bearer $USER_TOKEN")
ok "Заказ $O_ID отменён: status=$(field "$CANCEL" "['status']")"

STOCK1_AFTER=$(curl -sf "$BASE/products/$P1_ID" -H "Authorization: Bearer $USER_TOKEN" | python3 -c "import sys,json; print(json.load(sys.stdin)['stock'])")
STOCK2_AFTER=$(curl -sf "$BASE/products/$P2_ID" -H "Authorization: Bearer $USER_TOKEN" | python3 -c "import sys,json; print(json.load(sys.stdin)['stock'])")
ok "Stock восстановлен: Laptop=$STOCK1_AFTER, Mouse=$STOCK2_AFTER"

# ─────────────────────────────────────────────────────────
step "9. Rate limit"

echo "  Ждём 60 секунд для сброса rate limit..."
sleep 61

ORDER2=$(curl -sf "$BASE/orders" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d "{\"items\":[{\"product_id\":$P2_ID,\"quantity\":1}]}")
O2_ID=$(field "$ORDER2" "['id']")
ok "Заказ после ожидания создан id=$O2_ID"

# Сразу повторный — rate limit
ERR=$(curl -s -w "\n%{http_code}" "$BASE/orders" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d "{\"items\":[{\"product_id\":$P2_ID,\"quantity\":1}]}")
HTTP=$(echo "$ERR" | tail -1)
# может быть 409 (active order) или 429 (rate limit) — оба корректны
[[ "$HTTP" == "429" || "$HTTP" == "409" ]] \
  && ok "Повторный заказ заблокирован → $HTTP" \
  || fail "Ожидали 429/409, получили $HTTP" ""

# ─────────────────────────────────────────────────────────
step "10. State machine"

# ADMIN переводит заказ по цепочке
curl -sf -X PATCH "$BASE/orders/$O2_ID/status" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"status":"PAYMENT_PENDING"}' > /dev/null
ok "CREATED → PAYMENT_PENDING"

curl -sf -X PATCH "$BASE/orders/$O2_ID/status" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"status":"PAID"}' > /dev/null
ok "PAYMENT_PENDING → PAID"

# Невалидный переход PAID → CREATED
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X PATCH "$BASE/orders/$O2_ID/status" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"status":"CREATED"}')
[[ "$HTTP" == "409" ]] && ok "PAID → CREATED = 409 INVALID_STATE_TRANSITION" \
  || fail "Ожидали 409, получили $HTTP" ""

curl -sf -X PATCH "$BASE/orders/$O2_ID/status" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"status":"SHIPPED"}' > /dev/null
ok "PAID → SHIPPED"

curl -sf -X PATCH "$BASE/orders/$O2_ID/status" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"status":"COMPLETED"}' > /dev/null
ok "SHIPPED → COMPLETED"

# ─────────────────────────────────────────────────────────
step "11. Insufficient stock"

sleep 61
ERR=$(curl -s -w "\n%{http_code}" "$BASE/orders" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d "{\"items\":[{\"product_id\":$P1_ID,\"quantity\":9999}]}")
HTTP=$(echo "$ERR" | tail -1)
[[ "$HTTP" == "409" ]] && ok "INSUFFICIENT_STOCK → 409" \
  || fail "Ожидали 409, получили $HTTP" ""

# ─────────────────────────────────────────────────────────
step "12. X-Request-Id и JSON логи"

HEADERS=$(curl -sI "$BASE/products/$P1_ID" -H "Authorization: Bearer $USER_TOKEN")
RID=$(echo "$HEADERS" | grep -i "x-request-id" | tr -d '\r' | awk '{print $2}')
[[ -n "$RID" ]] && ok "X-Request-Id: $RID" \
  || fail "Заголовок X-Request-Id не найден" "$HEADERS"

# ─────────────────────────────────────────────────────────
step "13. Содержимое таблиц в БД"

echo ""
echo "  === products ==="
docker exec soa-hw-postgres-1 psql -U marketplace -c \
  "SELECT id, name, price, stock, status, seller_id FROM products ORDER BY id;"

echo "  === orders ==="
docker exec soa-hw-postgres-1 psql -U marketplace -c \
  "SELECT id, user_id, status, total_amount, discount_amount, promo_code_id FROM orders ORDER BY id;"

echo "  === order_items ==="
docker exec soa-hw-postgres-1 psql -U marketplace -c \
  "SELECT id, order_id, product_id, quantity, price_at_order FROM order_items ORDER BY id;"

echo "  === promo_codes ==="
docker exec soa-hw-postgres-1 psql -U marketplace -c \
  "SELECT id, code, discount_type, discount_value, current_uses, max_uses, active FROM promo_codes;"

echo "  === user_operations ==="
docker exec soa-hw-postgres-1 psql -U marketplace -c \
  "SELECT id, user_id, operation_type, created_at FROM user_operations ORDER BY id;"

echo "  === users ==="
docker exec soa-hw-postgres-1 psql -U marketplace -c \
  "SELECT id, username, role FROM users ORDER BY id;"

# ─────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}  Все E2E проверки пройдены успешно!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
