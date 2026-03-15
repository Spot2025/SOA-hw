package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"soa-hw/booking-service/internal/config"
	"soa-hw/booking-service/internal/grpcclient"
	"soa-hw/booking-service/internal/handlers"
	"soa-hw/booking-service/internal/middleware"
	"soa-hw/booking-service/internal/repository"
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

	// gRPC client with Circuit Breaker (outer) and Retry (inner)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			middleware.NewCircuitBreakerUnaryClientInterceptor(cfg.CBThreshold, cfg.CBTimeout, log),
			grpcclient.RetryUnaryClientInterceptor(cfg.RetryMaxAttempts, cfg.RetryBaseDelay),
		),
	}
	flightWrap, err := grpcclient.New(context.Background(), cfg.FlightGRPCAddr, cfg.FlightAPIKey, opts...)
	if err != nil {
		log.Error("flight client", "err", err)
		os.Exit(1)
	}
	defer flightWrap.Close()

	repo := repository.NewBookingRepo(db)
	h := handlers.New(repo, flightWrap, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /flights", h.SearchFlights)
	mux.HandleFunc("GET /flights/{id}", h.GetFlight)
	mux.HandleFunc("POST /bookings", h.CreateBooking)
	mux.HandleFunc("GET /bookings/{id}", h.GetBooking)
	mux.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	mux.HandleFunc("GET /bookings", h.ListBookings)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
	go func() {
		log.Info("booking service listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http serve", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	_ = srv.Shutdown(context.Background())
}
