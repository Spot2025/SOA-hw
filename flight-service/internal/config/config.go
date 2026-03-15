package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	GRPCAddr     string
	DBConn       string
	RedisAddr    string
	RedisTTL     time.Duration
	APIKey       string // для межсервисной аутентификации
}

func Load() *Config {
	ttlMin := 5
	if v := os.Getenv("CACHE_TTL_MIN"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			ttlMin = n
		}
	}
	return &Config{
		GRPCAddr:  getEnv("GRPC_ADDR", ":50051"),
		DBConn:    getEnv("FLIGHT_DB_DSN", "postgres://postgres:postgres@localhost:5432/flight_db?sslmode=disable"),
		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		RedisTTL:  time.Duration(ttlMin) * time.Minute,
		APIKey:    os.Getenv("FLIGHT_GRPC_API_KEY"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
