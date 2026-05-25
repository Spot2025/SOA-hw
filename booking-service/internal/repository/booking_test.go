package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"soa-hw/booking-service/internal/repository"
)

func setupBookingDB(t *testing.T) (*sqlx.DB, func()) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("booking_db"),
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
		t.Fatalf("migrate: %v", err)
	}
	return db, func() {
		db.Close()
		_ = pg.Terminate(ctx)
	}
}

func TestBookingRepoCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	db, cleanup := setupBookingDB(t)
	defer cleanup()

	repo := repository.NewBookingRepo(db)
	ctx := context.Background()
	id := uuid.New()
	b := &repository.Booking{
		ID:             id,
		UserID:         "user-1",
		FlightID:       1,
		PassengerName:  "Alice",
		PassengerEmail: "alice@test.com",
		SeatCount:      2,
		TotalPrice:     "10000.00",
	}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.UserID != "user-1" {
		t.Fatalf("unexpected booking: %+v", got)
	}

	list, err := repo.ListByUserID(ctx, "user-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByUserID: len=%d err=%v", len(list), err)
	}

	if err := repo.UpdateStatus(ctx, id, "CANCELLED"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ = repo.GetByID(ctx, id)
	if got.Status != "CANCELLED" {
		t.Fatalf("expected CANCELLED, got %s", got.Status)
	}
}
