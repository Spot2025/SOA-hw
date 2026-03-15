-- Flight: рейс. Комбинация (flight_number, дата вылета) уникальна
CREATE TABLE IF NOT EXISTS flights (
    id BIGSERIAL PRIMARY KEY,
    flight_number VARCHAR(20) NOT NULL,
    airline VARCHAR(100) NOT NULL,
    origin CHAR(3) NOT NULL,
    destination CHAR(3) NOT NULL,
    departure_time TIMESTAMPTZ NOT NULL,
    arrival_time TIMESTAMPTZ NOT NULL,
    total_seats INT NOT NULL CHECK (total_seats > 0),
    available_seats INT NOT NULL CHECK (available_seats >= 0),
    price DECIMAL(12,2) NOT NULL CHECK (price > 0),
    status VARCHAR(20) NOT NULL CHECK (status IN ('SCHEDULED', 'DEPARTED', 'CANCELLED', 'COMPLETED')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_available_lte_total CHECK (available_seats <= total_seats)
);

-- Уникальность по номеру рейса и дате вылета (UTC); выражение должно быть IMMUTABLE для индекса
CREATE UNIQUE INDEX idx_flights_number_departure ON flights (flight_number, ((departure_time AT TIME ZONE 'UTC')::date));

-- SeatReservation: резервация мест на рейсе
CREATE TABLE IF NOT EXISTS seat_reservations (
    id BIGSERIAL PRIMARY KEY,
    flight_id BIGINT NOT NULL REFERENCES flights(id) ON DELETE CASCADE,
    booking_id VARCHAR(64) NOT NULL,
    seat_count INT NOT NULL CHECK (seat_count > 0),
    status VARCHAR(20) NOT NULL CHECK (status IN ('ACTIVE', 'RELEASED', 'EXPIRED')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_seat_reservations_booking_active ON seat_reservations (booking_id) WHERE status = 'ACTIVE';
CREATE INDEX idx_seat_reservations_flight ON seat_reservations(flight_id);
