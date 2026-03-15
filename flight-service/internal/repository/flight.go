package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type Flight struct {
	ID              int64     `db:"id"`
	FlightNumber    string    `db:"flight_number"`
	Airline         string    `db:"airline"`
	Origin          string    `db:"origin"`
	Destination     string    `db:"destination"`
	DepartureTime   time.Time `db:"departure_time"`
	ArrivalTime     time.Time `db:"arrival_time"`
	TotalSeats      int32     `db:"total_seats"`
	AvailableSeats  int32     `db:"available_seats"`
	Price           string    `db:"price"`
	Status          string    `db:"status"`
}

type SeatReservation struct {
	ID         int64     `db:"id"`
	FlightID   int64     `db:"flight_id"`
	BookingID  string    `db:"booking_id"`
	SeatCount  int32     `db:"seat_count"`
	Status     string    `db:"status"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type FlightRepo struct {
	db *sqlx.DB
}

func NewFlightRepo(db *sqlx.DB) *FlightRepo {
	return &FlightRepo{db: db}
}

func (r *FlightRepo) Search(ctx context.Context, origin, destination, date string) ([]Flight, error) {
	q := `SELECT id, flight_number, airline, origin, destination, departure_time, arrival_time,
	       total_seats, available_seats, price, status
	       FROM flights WHERE origin = $1 AND destination = $2 AND status = 'SCHEDULED'`
	args := []interface{}{origin, destination}
	if date != "" {
		q += ` AND departure_time::date = $3::date`
		args = append(args, date)
	}
	q += ` ORDER BY departure_time`
	var list []Flight
	err := sqlx.SelectContext(ctx, r.db, &list, q, args...)
	return list, err
}

func (r *FlightRepo) GetByID(ctx context.Context, id int64) (*Flight, error) {
	var f Flight
	err := r.db.GetContext(ctx, &f,
		`SELECT id, flight_number, airline, origin, destination, departure_time, arrival_time,
		 total_seats, available_seats, price, status FROM flights WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ReserveSeats атомарно: SELECT FOR UPDATE, уменьшение available_seats, вставка seat_reservations.
// Идемпотентность: если резервация с таким booking_id уже есть (ACTIVE) — возвращаем её без дубликата.
func (r *FlightRepo) ReserveSeats(ctx context.Context, flightID int64, seatCount int32, bookingID string) (reservationID int64, alreadyExisted bool, err error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()

	// Идемпотентность: уже есть активная резервация с этим booking_id
	var existingID int64
	err = tx.GetContext(ctx, &existingID,
		`SELECT id FROM seat_reservations WHERE booking_id = $1 AND status = 'ACTIVE'`, bookingID)
	if err == nil {
		_ = tx.Commit()
		return existingID, true, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}

	// Блокировка строки рейса для предотвращения race condition
	var available int32
	err = tx.GetContext(ctx, &available,
		`SELECT available_seats FROM flights WHERE id = $1 FOR UPDATE`, flightID)
	if err == sql.ErrNoRows {
		return 0, false, ErrNotFound
	}
	if err != nil {
		return 0, false, err
	}
	if available < seatCount {
		return 0, false, ErrResourceExhausted
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE flights SET available_seats = available_seats - $1, updated_at = NOW() WHERE id = $2`,
		seatCount, flightID)
	if err != nil {
		return 0, false, err
	}

	err = tx.QueryRowContext(ctx,
		`INSERT INTO seat_reservations (flight_id, booking_id, seat_count, status) VALUES ($1, $2, $3, 'ACTIVE') RETURNING id`,
		flightID, bookingID, seatCount).Scan(&reservationID)
	if err != nil {
		return 0, false, err
	}

	if err = tx.Commit(); err != nil {
		return 0, false, err
	}
	return reservationID, false, nil
}

// ReleaseResult данные для инвалидации кеша после отмены резервации
type ReleaseResult struct {
	PrevStatus     string
	FlightID       int64
	Origin         string
	Destination    string
	DepartureTime  time.Time
}

// ReleaseReservation возврат мест и смена статуса в одной транзакции
func (r *FlightRepo) ReleaseReservation(ctx context.Context, bookingID string) (*ReleaseResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var res SeatReservation
	err = tx.GetContext(ctx, &res,
		`SELECT id, flight_id, seat_count, status FROM seat_reservations WHERE booking_id = $1 AND status = 'ACTIVE' FOR UPDATE`,
		bookingID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var f Flight
	err = tx.GetContext(ctx, &f, `SELECT id, origin, destination, departure_time FROM flights WHERE id = $1`, res.FlightID)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE flights SET available_seats = available_seats + $1, updated_at = NOW() WHERE id = $2`,
		res.SeatCount, res.FlightID)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE seat_reservations SET status = 'RELEASED', updated_at = NOW() WHERE id = $1`, res.ID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &ReleaseResult{
		PrevStatus:    res.Status,
		FlightID:      res.FlightID,
		Origin:        f.Origin,
		Destination:   f.Destination,
		DepartureTime: f.DepartureTime,
	}, nil
}
