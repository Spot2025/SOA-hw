// Package integration tests cross-service interaction: HTTP -> gRPC -> PostgreSQL.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"soa-hw/booking-service/internal/grpcclient"
	"soa-hw/booking-service/internal/handlers"
	"soa-hw/booking-service/internal/repository"
	"soa-hw/flight-service/testutil"
	pkgmetrics "soa-hw/pkg/metrics"
)

const apiKey = "test-api-key"

type stack struct {
	flightDB  *sqlx.DB
	bookingDB *sqlx.DB
	baseURL   string
	cleanup   func()
}

func setupStack(t *testing.T) *stack {
	t.Helper()
	ctx := context.Background()

	flightPG, err := postgres.Run(ctx, "postgres:15-alpine",
		postgres.WithDatabase("flight_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		t.Fatalf("flight postgres: %v", err)
	}
	bookingPG, err := postgres.Run(ctx, "postgres:15-alpine",
		postgres.WithDatabase("booking_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		t.Fatalf("booking postgres: %v", err)
	}
	redisC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("redis: %v", err)
	}

	flightConn, _ := flightPG.ConnectionString(ctx, "sslmode=disable")
	bookingConn, _ := bookingPG.ConnectionString(ctx, "sslmode=disable")
	flightDB, _ := sqlx.Connect("postgres", flightConn)
	bookingDB, _ := sqlx.Connect("postgres", bookingConn)

	migrateFlight(t, flightDB)
	migrateBooking(t, bookingDB)

	redisAddr, _ := redisC.Endpoint(ctx, "")
	flightAddr, stopFlight := testutil.StartGRPCServer(t, flightDB, redisAddr, apiKey)

	flightClient, err := grpcclient.New(context.Background(), flightAddr, apiKey,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}

	repo := repository.NewBookingRepo(bookingDB)
	h := handlers.New(repo, flightClient, slog.Default())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /flights", h.SearchFlights)
	mux.HandleFunc("GET /flights/{id}", h.GetFlight)
	mux.HandleFunc("POST /bookings", h.CreateBooking)
	mux.HandleFunc("GET /bookings/{id}", h.GetBooking)
	mux.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	mux.HandleFunc("GET /bookings", h.ListBookings)

	httpLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: pkgmetrics.Middleware(mux)}
	go httpSrv.Serve(httpLis)

	s := &stack{
		flightDB:  flightDB,
		bookingDB: bookingDB,
		baseURL:   fmt.Sprintf("http://%s", httpLis.Addr().String()),
	}
	s.cleanup = func() {
		_ = httpSrv.Shutdown(context.Background())
		stopFlight()
		_ = flightClient.Close()
		flightDB.Close()
		bookingDB.Close()
		_ = flightPG.Terminate(ctx)
		_ = bookingPG.Terminate(ctx)
		_ = redisC.Terminate(ctx)
	}
	return s
}

func migrateFlight(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE flights (
			id BIGSERIAL PRIMARY KEY,
			flight_number VARCHAR(20) NOT NULL,
			airline VARCHAR(100) NOT NULL,
			origin VARCHAR(3) NOT NULL,
			destination VARCHAR(3) NOT NULL,
			departure_time TIMESTAMPTZ NOT NULL,
			arrival_time TIMESTAMPTZ NOT NULL,
			total_seats INT NOT NULL,
			available_seats INT NOT NULL,
			price DECIMAL(10,2) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE seat_reservations (
			id BIGSERIAL PRIMARY KEY,
			flight_id BIGINT NOT NULL REFERENCES flights(id),
			booking_id UUID NOT NULL UNIQUE,
			seat_count INT NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		INSERT INTO flights (flight_number, airline, origin, destination, departure_time, arrival_time, total_seats, available_seats, price, status)
		VALUES ('SU1234', 'Aeroflot', 'SVO', 'LED', '2026-04-01 10:00:00+00', '2026-04-01 11:30:00+00', 100, 100, 5000.00, 'SCHEDULED');
	`)
	if err != nil {
		t.Fatalf("flight schema: %v", err)
	}
}

func migrateBooking(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE bookings (
			id UUID PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			flight_id BIGINT NOT NULL,
			passenger_name VARCHAR(255) NOT NULL,
			passenger_email VARCHAR(255) NOT NULL,
			seat_count INT NOT NULL,
			total_price DECIMAL(10,2) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'CONFIRMED',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		t.Fatalf("booking schema: %v", err)
	}
}

func TestBookingFlowCrossService(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	s := setupStack(t)
	defer s.cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(s.baseURL + "/flights?origin=SVO&destination=LED&date=2026-04-01")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search status=%d body=%s", resp.StatusCode, body)
	}

	payload := map[string]interface{}{
		"user_id":         "integration-user",
		"flight_id":       1,
		"passenger_name":  "Integration Test",
		"passenger_email": "test@example.com",
		"seat_count":      2,
	}
	data, _ := json.Marshal(payload)
	resp, err = client.Post(s.baseURL+"/bookings", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var created map[string]interface{}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	bookingID, _ := created["id"].(string)
	if bookingID == "" {
		t.Fatalf("missing booking id: %s", body)
	}

	var reserved int
	if err := s.flightDB.Get(&reserved, `SELECT seat_count FROM seat_reservations WHERE booking_id = $1 AND status = 'ACTIVE'`, bookingID); err != nil {
		t.Fatalf("reservation in flight DB: %v", err)
	}
	if reserved != 2 {
		t.Fatalf("expected 2 reserved seats, got %d", reserved)
	}

	var status string
	if err := s.bookingDB.Get(&status, `SELECT status FROM bookings WHERE id = $1`, bookingID); err != nil {
		t.Fatalf("booking in booking DB: %v", err)
	}
	if status != "CONFIRMED" {
		t.Fatalf("expected CONFIRMED, got %s", status)
	}

	resp, err = client.Post(s.baseURL+"/bookings/"+bookingID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status=%d", resp.StatusCode)
	}

	var resStatus string
	if err := s.flightDB.Get(&resStatus, `SELECT status FROM seat_reservations WHERE booking_id = $1`, bookingID); err != nil {
		t.Fatal(err)
	}
	if resStatus != "RELEASED" {
		t.Fatalf("expected RELEASED reservation, got %s", resStatus)
	}
}
