package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	api "marketplace/internal/api"
	"marketplace/internal/config"
	"marketplace/internal/handler"
	"marketplace/internal/middleware"
	"marketplace/internal/repository"
	"marketplace/internal/service"
)

func main() {
	cfg := config.Load()
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to ping database")
	}
	logger.Info().Msg("connected to database")

	productRepo := repository.NewProductRepo(pool)
	orderRepo := repository.NewOrderRepo(pool)
	userRepo := repository.NewUserRepo(pool)
	promoRepo := repository.NewPromoRepo(pool)
	userOpRepo := repository.NewUserOpRepo(pool)

	authSvc := service.NewAuthService(userRepo, cfg)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, productRepo, promoRepo, userOpRepo, cfg)
	promoSvc := service.NewPromoService(promoRepo)

	h := handler.New(productSvc, orderSvc, authSvc, promoSvc, logger)

	authMw := middleware.NewAuthMiddleware(cfg.JWTSecret)

	validator, err := middleware.NewValidator()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create validator")
	}

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(authMw.Authenticate)
	r.Use(middleware.Logging(logger))

	r.Route("/api/v1", func(sub chi.Router) {
		sub.Use(validator)
		api.HandlerFromMux(h, sub)
	})

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: r,
	}

	go func() {
		logger.Info().Str("port", cfg.ServerPort).Msg("starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}
