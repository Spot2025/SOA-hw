package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	grpcgoogle "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"soa-hw/flight-service/internal/cache"
	"soa-hw/flight-service/internal/config"
	grpchandlers "soa-hw/flight-service/internal/grpc"
	flightv1 "soa-hw/proto/gen/go/flight/v1"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	db, err := sqlx.Connect("postgres", cfg.DBConn)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	var redisClient redis.UniversalClient
	if cfg.RedisAddr != "" {
		redisClient = redis.NewUniversalClient(&redis.UniversalOptions{
			Addrs: []string{cfg.RedisAddr},
		})
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			log.Warn("redis ping failed, continuing without cache", "err", err)
			redisClient = nil
		}
	}

	var c *cache.Cache
	if redisClient != nil {
		c = cache.New(redisClient, cfg.RedisTTL, log)
	}

	srv := grpchandlers.NewServer(db, c, log)
	authOpt := grpchandlers.RequireAPIKey(cfg.APIKey)

	gs := grpcgoogle.NewServer(
		grpcgoogle.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpcgoogle.UnaryServerInfo, handler grpcgoogle.UnaryHandler) (interface{}, error) {
			if err := authOpt(ctx); err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}),
	)
	flightv1.RegisterFlightServiceServer(gs, srv)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	defer lis.Close()

	log.Info("flight service listening", "addr", cfg.GRPCAddr)
	go func() {
		if err := gs.Serve(lis); err != nil {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	gs.GracefulStop()
}
