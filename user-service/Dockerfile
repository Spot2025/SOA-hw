# --- Build stage ---
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o /user-service main.go

# --- Runtime stage ---
FROM alpine:3.20

COPY --from=builder /user-service /user-service

EXPOSE 8080

ENTRYPOINT ["/user-service"]
