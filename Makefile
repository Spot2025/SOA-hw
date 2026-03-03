.PHONY: generate build run clean docker-up docker-down migrate-up migrate-down

OAPI_CODEGEN := $(shell go env GOPATH)/bin/oapi-codegen

install-tools:
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

generate:
	$(OAPI_CODEGEN) --package api \
		-generate types,chi-server,spec \
		-o internal/api/api.gen.go \
		api/openapi.yaml

build: generate
	go build -o bin/server ./cmd/server

run: build
	./bin/server

clean:
	rm -rf bin/ internal/api/api.gen.go

docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down -v

migrate-up:
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$DATABASE_URL" down
