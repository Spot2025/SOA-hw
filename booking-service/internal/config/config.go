package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr         string
	DBConn           string
	FlightGRPCAddr   string
	FlightAPIKey     string
	// Circuit breaker
	CBThreshold     uint32        // порог ошибок для OPEN
	CBTimeout       time.Duration // таймаут перед HALF_OPEN
	CBInterval      time.Duration // окно для подсчёта ошибок (опционально)
	// Retry
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
}

func Load() *Config {
	threshold := uint32(5)
	if v := os.Getenv("CIRCUIT_BREAKER_THRESHOLD"); v != "" {
		if n, _ := strconv.ParseUint(v, 10, 32); n > 0 {
			threshold = uint32(n)
		}
	}
	cbTimeoutSec := 15
	if v := os.Getenv("CIRCUIT_BREAKER_TIMEOUT_SEC"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			cbTimeoutSec = n
		}
	}
	retryMax := 3
	if v := os.Getenv("RETRY_MAX_ATTEMPTS"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			retryMax = n
		}
	}
	return &Config{
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DBConn:           getEnv("BOOKING_DB_DSN", "postgres://postgres:postgres@localhost:5433/booking_db?sslmode=disable"),
		FlightGRPCAddr:   getEnv("FLIGHT_GRPC_ADDR", "localhost:50051"),
		FlightAPIKey:     os.Getenv("FLIGHT_GRPC_API_KEY"),
		CBThreshold:     threshold,
		CBTimeout:        time.Duration(cbTimeoutSec) * time.Second,
		CBInterval:       time.Minute,
		RetryMaxAttempts: retryMax,
		RetryBaseDelay:   100 * time.Millisecond,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
