package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Booking struct {
	ID              uuid.UUID `db:"id"`
	UserID          string    `db:"user_id"`
	FlightID        int64     `db:"flight_id"`
	PassengerName   string    `db:"passenger_name"`
	PassengerEmail  string    `db:"passenger_email"`
	SeatCount       int32     `db:"seat_count"`
	TotalPrice      string    `db:"total_price"`
	Status          string    `db:"status"`
	CreatedAt       string    `db:"created_at"`
	UpdatedAt       string    `db:"updated_at"`
}

type BookingRepo struct {
	db *sqlx.DB
}

func NewBookingRepo(db *sqlx.DB) *BookingRepo {
	return &BookingRepo{db: db}
}

func (r *BookingRepo) Create(ctx context.Context, b *Booking) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO bookings (id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'CONFIRMED')`,
		b.ID, b.UserID, b.FlightID, b.PassengerName, b.PassengerEmail, b.SeatCount, b.TotalPrice)
	return err
}

func (r *BookingRepo) GetByID(ctx context.Context, id uuid.UUID) (*Booking, error) {
	var b Booking
	err := r.db.GetContext(ctx, &b,
		`SELECT id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status, created_at, updated_at
		 FROM bookings WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *BookingRepo) ListByUserID(ctx context.Context, userID string) ([]Booking, error) {
	var list []Booking
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status, created_at, updated_at
		 FROM bookings WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	return list, err
}

func (r *BookingRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}
