.PHONY: proto build migrate-up migrate-down docker-up docker-down test test-unit test-integration test-e2e load-test validate-metrics

PROTO_DIR := proto
PROTO_GEN := proto/gen/go/flight/v1
THIRD_PARTY := third_party

proto:
	@command -v protoc >/dev/null 2>&1 || (echo "install protoc: https://grpc.io/docs/protoc-installation/" && exit 1)
	@command -v protoc-gen-go >/dev/null 2>&1 || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@command -v protoc-gen-go-grpc >/dev/null 2>&1 || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@mkdir -p $(PROTO_GEN)
	protoc -I $(PROTO_DIR) -I $(THIRD_PARTY) \
		--go_out=. --go_opt=module=soa-hw \
		--go-grpc_out=. --go-grpc_opt=module=soa-hw \
		$(PROTO_DIR)/flight.proto

build: proto
	go build -o bin/flight-service ./flight-service/cmd/server
	go build -o bin/booking-service ./booking-service/cmd/server

test-unit:
	go test -short -count=1 ./booking-service/... ./flight-service/... ./pkg/...

test-integration:
	go test -count=1 ./booking-service/test/integration/...

test-e2e:
	go test -count=1 ./tests/e2e/...

test: test-unit test-integration

load-test:
	k6 run loadtests/booking.js

validate-metrics:
	bash scripts/validate-metrics.sh

# Локальный запуск миграций (Postgres на localhost). Использует образ migrate/migrate.
migrate-up:
	docker run --rm -v $(PWD)/flight-service/migrations:/migrations --network host \
		migrate/migrate:v4.17.0 -path=/migrations -database "postgres://postgres:postgres@localhost:5434/flight_db?sslmode=disable" up
	docker run --rm -v $(PWD)/booking-service/migrations:/migrations --network host \
		migrate/migrate:v4.17.0 -path=/migrations -database "postgres://postgres:postgres@localhost:5435/booking_db?sslmode=disable" up

docker-up:
	docker compose up -d

docker-down:
	docker compose down
