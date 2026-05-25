package testutil

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"soa-hw/flight-service/internal/cache"
	grpchandlers "soa-hw/flight-service/internal/grpc"
	flightv1 "soa-hw/proto/gen/go/flight/v1"
	pkgmetrics "soa-hw/pkg/metrics"
)

// StartGRPCServer runs flight-service gRPC in-process for integration tests.
func StartGRPCServer(t *testing.T, db *sqlx.DB, redisAddr, apiKey string) (string, func()) {
	t.Helper()

	var c *cache.Cache
	if redisAddr != "" {
		client := redis.NewClient(&redis.Options{Addr: redisAddr})
		if err := client.Ping(context.Background()).Err(); err == nil {
			c = cache.New(client, time.Minute, slog.Default())
		}
	}

	srv := grpchandlers.NewServer(db, c, slog.Default())
	authOpt := grpchandlers.RequireAPIKey(apiKey)
	gs := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			pkgmetrics.UnaryServerInterceptor(),
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
				if err := authOpt(ctx); err != nil {
					return nil, err
				}
				return handler(ctx, req)
			},
		),
	)
	flightv1.RegisterFlightServiceServer(gs, srv)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go gs.Serve(lis)

	return lis.Addr().String(), func() {
		gs.GracefulStop()
	}
}
