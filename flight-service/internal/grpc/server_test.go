package grpc_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	grpchandlers "soa-hw/flight-service/internal/grpc"
	flightv1 "soa-hw/proto/gen/go/flight/v1"
)

func setupFlightDB(t *testing.T) (*sqlx.DB, func()) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("flight_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_, err = db.Exec(`
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
			booking_id UUID NOT NULL,
			seat_count INT NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (booking_id)
		);
		INSERT INTO flights (flight_number, airline, origin, destination, departure_time, arrival_time, total_seats, available_seats, price, status)
		VALUES ('SU1234', 'Aeroflot', 'SVO', 'LED', '2026-04-01 10:00:00+00', '2026-04-01 11:30:00+00', 100, 100, 5000.00, 'SCHEDULED');
	`)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cleanup := func() {
		db.Close()
		_ = pg.Terminate(ctx)
	}
	return db, cleanup
}

func TestSearchFlightsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	db, cleanup := setupFlightDB(t)
	defer cleanup()

	srv := grpchandlers.NewServer(db, nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	resp, err := srv.SearchFlights(context.Background(), &flightv1.SearchFlightsRequest{
		Origin:      "SVO",
		Destination: "LED",
		Date:        "2026-04-01",
	})
	if err != nil {
		t.Fatalf("SearchFlights: %v", err)
	}
	if len(resp.Flights) != 1 {
		t.Fatalf("expected 1 flight, got %d", len(resp.Flights))
	}
	if resp.Flights[0].FlightNumber != "SU1234" {
		t.Fatalf("unexpected flight: %+v", resp.Flights[0])
	}
}

func TestReserveSeatsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	db, cleanup := setupFlightDB(t)
	defer cleanup()

	srv := grpchandlers.NewServer(db, nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx := context.Background()
	_, err := srv.ReserveSeats(ctx, &flightv1.ReserveSeatsRequest{
		FlightId:  1,
		SeatCount: 2,
		BookingId: "550e8400-e29b-41d4-a716-446655440000",
	})
	if err != nil {
		t.Fatalf("ReserveSeats: %v", err)
	}

	var available int32
	if err := db.Get(&available, `SELECT available_seats FROM flights WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if available != 98 {
		t.Fatalf("expected 98 available seats, got %d", available)
	}

	// Idempotent second call
	_, err = srv.ReserveSeats(ctx, &flightv1.ReserveSeatsRequest{
		FlightId:  1,
		SeatCount: 2,
		BookingId: "550e8400-e29b-41d4-a716-446655440000",
	})
	if err != nil {
		t.Fatalf("idempotent ReserveSeats: %v", err)
	}
	if err := db.Get(&available, `SELECT available_seats FROM flights WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if available != 98 {
		t.Fatalf("idempotent call changed seats: %d", available)
	}
}

func TestReleaseReservationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	db, cleanup := setupFlightDB(t)
	defer cleanup()

	srv := grpchandlers.NewServer(db, nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx := context.Background()
	bookingID := "550e8400-e29b-41d4-a716-446655440000"
	_, err := srv.ReserveSeats(ctx, &flightv1.ReserveSeatsRequest{
		FlightId:  1, SeatCount: 3, BookingId: bookingID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.ReleaseReservation(ctx, &flightv1.ReleaseReservationRequest{BookingId: bookingID})
	if err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}
	var available int32
	if err := db.Get(&available, `SELECT available_seats FROM flights WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if available != 100 {
		t.Fatalf("expected seats restored to 100, got %d", available)
	}
	_ = time.Now() // keep time import used
}
