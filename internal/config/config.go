package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL          string
	ServerPort           string
	JWTSecret            string
	JWTAccessTTL         time.Duration
	JWTRefreshTTL        time.Duration
	OrderRateLimitMinutes int
}

func Load() *Config {
	return &Config{
		DatabaseURL:          envOrDefault("DATABASE_URL", "postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable"),
		ServerPort:           envOrDefault("SERVER_PORT", "8080"),
		JWTSecret:            envOrDefault("JWT_SECRET", "super-secret-key-change-in-production"),
		JWTAccessTTL:         parseDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:        parseDuration("JWT_REFRESH_TTL", 720*time.Hour),
		OrderRateLimitMinutes: parseInt("ORDER_RATE_LIMIT_MINUTES", 1),
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

func parseInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}
