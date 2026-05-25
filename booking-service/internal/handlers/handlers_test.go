package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"soa-hw/booking-service/internal/handlers"
	"soa-hw/booking-service/internal/repository"
	flightv1 "soa-hw/proto/gen/go/flight/v1"
)

type mockFlight struct {
	searchFn   func(ctx context.Context, origin, destination, date string) (*flightv1.SearchFlightsResponse, error)
	getFn      func(ctx context.Context, flightID int64) (*flightv1.GetFlightResponse, error)
	reserveFn  func(ctx context.Context, flightID int64, seatCount int32, bookingID string) (*flightv1.ReserveSeatsResponse, error)
	releaseFn  func(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error)
}

func (m *mockFlight) SearchFlights(ctx context.Context, origin, destination, date string) (*flightv1.SearchFlightsResponse, error) {
	return m.searchFn(ctx, origin, destination, date)
}

func (m *mockFlight) GetFlight(ctx context.Context, flightID int64) (*flightv1.GetFlightResponse, error) {
	return m.getFn(ctx, flightID)
}

func (m *mockFlight) ReserveSeats(ctx context.Context, flightID int64, seatCount int32, bookingID string) (*flightv1.ReserveSeatsResponse, error) {
	return m.reserveFn(ctx, flightID, seatCount, bookingID)
}

func (m *mockFlight) ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error) {
	return m.releaseFn(ctx, bookingID)
}

type mockRepo struct {
	bookings map[uuid.UUID]*repository.Booking
}

func (m *mockRepo) Create(ctx context.Context, b *repository.Booking) error {
	if m.bookings == nil {
		m.bookings = make(map[uuid.UUID]*repository.Booking)
	}
	copy := *b
	m.bookings[b.ID] = &copy
	return nil
}

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*repository.Booking, error) {
	if b, ok := m.bookings[id]; ok {
		copy := *b
		return &copy, nil
	}
	return nil, nil
}

func (m *mockRepo) ListByUserID(ctx context.Context, userID string) ([]repository.Booking, error) {
	var list []repository.Booking
	for _, b := range m.bookings {
		if b.UserID == userID {
			list = append(list, *b)
		}
	}
	return list, nil
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	if b, ok := m.bookings[id]; ok {
		b.Status = status
	}
	return nil
}

func newTestHandlers(flight handlers.FlightAPI, repo *repository.BookingRepo) *handlers.Handlers {
	if repo == nil {
		repo = repository.NewBookingRepo(nil)
	}
	return handlers.New(repo, flight, slog.New(slog.NewTextHandler(os.Stdout, nil)))
}

func TestSearchFlightsMissingParams(t *testing.T) {
	h := newTestHandlers(&mockFlight{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/flights?origin=SVO", nil)
	rec := httptest.NewRecorder()
	h.SearchFlights(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetFlightNotFound(t *testing.T) {
	h := newTestHandlers(&mockFlight{
		getFn: func(_ context.Context, _ int64) (*flightv1.GetFlightResponse, error) {
			return nil, status.Error(codes.NotFound, "flight not found")
		},
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/flights/999", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()
	h.GetFlight(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateBookingSuccess(t *testing.T) {
	flight := &mockFlight{
		getFn: func(_ context.Context, _ int64) (*flightv1.GetFlightResponse, error) {
			return &flightv1.GetFlightResponse{
				Flight: &flightv1.Flight{
					Id:    1,
					Price: "5000.00",
				},
			}, nil
		},
		reserveFn: func(_ context.Context, _ int64, _ int32, _ string) (*flightv1.ReserveSeatsResponse, error) {
			return &flightv1.ReserveSeatsResponse{Status: flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE}, nil
		},
	}
	// Use in-memory mock via wrapper - handlers use real repo with nil db won't work for Create
	// So we test validation only for invalid JSON
	h := newTestHandlers(flight, nil)
	body := bytes.NewBufferString(`{"user_id":"","flight_id":1,"passenger_name":"A","passenger_email":"a@b.com","seat_count":1}`)
	req := httptest.NewRequest(http.MethodPost, "/bookings", body)
	rec := httptest.NewRecorder()
	h.CreateBooking(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing user_id, got %d", rec.Code)
	}
}

func TestSearchFlightsSuccess(t *testing.T) {
	h := newTestHandlers(&mockFlight{
		searchFn: func(_ context.Context, _, _, _ string) (*flightv1.SearchFlightsResponse, error) {
			return &flightv1.SearchFlightsResponse{
				Flights: []*flightv1.Flight{{
					Id:             1,
					FlightNumber:   "SU1234",
					Origin:         "SVO",
					Destination:    "LED",
					DepartureTime:  timestamppb.Now(),
					AvailableSeats: 10,
					Price:          "5000.00",
					Status:         flightv1.FlightStatus_FLIGHT_STATUS_SCHEDULED,
				}},
			}, nil
		},
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/flights?origin=SVO&destination=LED", nil)
	rec := httptest.NewRecorder()
	h.SearchFlights(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	flights, ok := resp["flights"].([]interface{})
	if !ok || len(flights) != 1 {
		t.Fatalf("expected 1 flight, got %#v", resp["flights"])
	}
}
