// Package e2e runs end-to-end tests against a running docker-compose stack.
package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func baseURL() string {
	if v := os.Getenv("E2E_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func bookingDSN() string {
	if v := os.Getenv("E2E_BOOKING_DB_DSN"); v != "" {
		return v
	}
	return "postgres://postgres:postgres@localhost:5435/booking_db?sslmode=disable"
}

func flightDSN() string {
	if v := os.Getenv("E2E_FLIGHT_DB_DSN"); v != "" {
		return v
	}
	return "postgres://postgres:postgres@localhost:5434/flight_db?sslmode=disable"
}

func TestFullBookingScenario(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	base := baseURL()

	// 1. Search flights
	resp, err := client.Get(base + "/flights?origin=SVO&destination=LED&date=2026-04-01")
	if err != nil {
		t.Fatalf("search request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: status=%d body=%s", resp.StatusCode, body)
	}
	var searchResp struct {
		Flights []struct {
			ID             int64  `json:"id"`
			FlightNumber   string `json:"flight_number"`
			AvailableSeats int32  `json:"available_seats"`
		} `json:"flights"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		t.Fatal(err)
	}
	if len(searchResp.Flights) == 0 {
		t.Fatal("expected at least one flight")
	}
	flightID := searchResp.Flights[0].ID

	// 2. Create booking
	payload := map[string]interface{}{
		"user_id":         fmt.Sprintf("e2e-user-%d", time.Now().UnixNano()),
		"flight_id":       flightID,
		"passenger_name":  "E2E User",
		"passenger_email": "e2e@example.com",
		"seat_count":      1,
	}
	data, _ := json.Marshal(payload)
	resp, err = client.Post(base+"/bookings", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create booking: status=%d body=%s", resp.StatusCode, body)
	}
	var created struct {
		ID         string  `json:"id"`
		UserID     string  `json:"user_id"`
		FlightID   int64   `json:"flight_id"`
		SeatCount  int32   `json:"seat_count"`
		TotalPrice string  `json:"total_price"`
		Status     string  `json:"status"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Status != "CONFIRMED" {
		t.Fatalf("invalid create response: %s", body)
	}
	if created.TotalPrice == "" || created.TotalPrice == "0.00" {
		t.Fatalf("expected non-zero total_price, got %s", created.TotalPrice)
	}

	// 3. Verify booking in PostgreSQL (booking_db)
	bookingDB, err := sql.Open("postgres", bookingDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer bookingDB.Close()
	var dbStatus string
	var dbSeatCount int
	err = bookingDB.QueryRow(`SELECT status, seat_count FROM bookings WHERE id = $1`, created.ID).Scan(&dbStatus, &dbSeatCount)
	if err != nil {
		t.Fatalf("booking DB query: %v", err)
	}
	if dbStatus != "CONFIRMED" || dbSeatCount != 1 {
		t.Fatalf("booking DB: status=%s seats=%d", dbStatus, dbSeatCount)
	}

	// 4. Verify reservation in PostgreSQL (flight_db)
	flightDB, err := sql.Open("postgres", flightDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer flightDB.Close()
	var resStatus string
	err = flightDB.QueryRow(`SELECT status FROM seat_reservations WHERE booking_id = $1`, created.ID).Scan(&resStatus)
	if err != nil {
		t.Fatalf("flight DB reservation: %v", err)
	}
	if resStatus != "ACTIVE" {
		t.Fatalf("expected ACTIVE reservation, got %s", resStatus)
	}

	// 5. Get booking by ID
	resp, err = client.Get(base + "/bookings/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get booking: status=%d body=%s", resp.StatusCode, body)
	}

	// 6. List user bookings
	resp, err = client.Get(base + "/bookings?user_id=" + created.UserID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list bookings: status=%d body=%s", resp.StatusCode, body)
	}

	// 7. Cancel booking
	resp, err = client.Post(base+"/bookings/"+created.ID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel: status=%d body=%s", resp.StatusCode, body)
	}

	// 8. Verify cancelled in both DBs
	err = bookingDB.QueryRow(`SELECT status FROM bookings WHERE id = $1`, created.ID).Scan(&dbStatus)
	if err != nil || dbStatus != "CANCELLED" {
		t.Fatalf("booking not cancelled: status=%s err=%v", dbStatus, err)
	}
	err = flightDB.QueryRow(`SELECT status FROM seat_reservations WHERE booking_id = $1`, created.ID).Scan(&resStatus)
	if err != nil || resStatus != "RELEASED" {
		t.Fatalf("reservation not released: status=%s err=%v", resStatus, err)
	}
}

func TestHealthEndpoints(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", resp.StatusCode)
	}
}
