package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"soa-hw/booking-service/internal/repository"
)

type Handlers struct {
	repo   *repository.BookingRepo
	flight FlightAPI
	log    *slog.Logger
}

func New(repo *repository.BookingRepo, flight FlightAPI, log *slog.Logger) *Handlers {
	if log == nil {
		log = slog.Default()
	}
	return &Handlers{repo: repo, flight: flight, log: log}
}

func (h *Handlers) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handlers) writeErr(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

// GET /flights?origin=SVO&destination=LED&date=2026-04-01
func (h *Handlers) SearchFlights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	origin := r.URL.Query().Get("origin")
	destination := r.URL.Query().Get("destination")
	date := r.URL.Query().Get("date")
	if origin == "" || destination == "" {
		h.writeErr(w, http.StatusBadRequest, "origin and destination are required")
		return
	}
	resp, err := h.flight.SearchFlights(r.Context(), origin, destination, date)
	if err != nil {
		h.log.Error("SearchFlights", "err", err)
		h.grpcErrToHTTP(w, err)
		return
	}
	// Преобразуем в простой JSON для REST
	type flightItem struct {
		ID             int64   `json:"id"`
		FlightNumber   string  `json:"flight_number"`
		Airline        string  `json:"airline"`
		Origin         string  `json:"origin"`
		Destination    string  `json:"destination"`
		DepartureTime  string  `json:"departure_time"`
		ArrivalTime    string  `json:"arrival_time"`
		TotalSeats     int32   `json:"total_seats"`
		AvailableSeats int32   `json:"available_seats"`
		Price          string  `json:"price"`
		Status         string  `json:"status"`
	}
	items := make([]flightItem, len(resp.Flights))
	for i, f := range resp.Flights {
		dep := ""
		if f.DepartureTime != nil {
			dep = f.DepartureTime.AsTime().Format("2006-01-02T15:04:05Z07:00")
		}
		arr := ""
		if f.ArrivalTime != nil {
			arr = f.ArrivalTime.AsTime().Format("2006-01-02T15:04:05Z07:00")
		}
		items[i] = flightItem{
			ID:             f.Id,
			FlightNumber:   f.FlightNumber,
			Airline:        f.Airline,
			Origin:         f.Origin,
			Destination:    f.Destination,
			DepartureTime:  dep,
			ArrivalTime:    arr,
			TotalSeats:     f.TotalSeats,
			AvailableSeats: f.AvailableSeats,
			Price:          f.Price,
			Status:         f.Status.String(),
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"flights": items})
}

// GET /flights/:id
func (h *Handlers) GetFlight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		h.writeErr(w, http.StatusBadRequest, "flight id required")
		return
	}
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		h.writeErr(w, http.StatusBadRequest, "invalid flight id")
		return
	}
	resp, err := h.flight.GetFlight(r.Context(), id)
	if err != nil {
		h.grpcErrToHTTP(w, err)
		return
	}
	if resp.Flight == nil {
		h.writeErr(w, http.StatusNotFound, "flight not found")
		return
	}
	f := resp.Flight
	dep := ""
	if f.DepartureTime != nil {
		dep = f.DepartureTime.AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	arr := ""
	if f.ArrivalTime != nil {
		arr = f.ArrivalTime.AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":              f.Id,
		"flight_number":   f.FlightNumber,
		"airline":         f.Airline,
		"origin":          f.Origin,
		"destination":     f.Destination,
		"departure_time":  dep,
		"arrival_time":    arr,
		"total_seats":     f.TotalSeats,
		"available_seats": f.AvailableSeats,
		"price":           f.Price,
		"status":          f.Status.String(),
	})
}

// POST /bookings
func (h *Handlers) CreateBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		UserID          string `json:"user_id"`
		FlightID        int64  `json:"flight_id"`
		PassengerName   string `json:"passenger_name"`
		PassengerEmail  string `json:"passenger_email"`
		SeatCount       int32  `json:"seat_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.UserID == "" || body.FlightID <= 0 || body.PassengerName == "" || body.PassengerEmail == "" || body.SeatCount <= 0 {
		h.writeErr(w, http.StatusBadRequest, "user_id, flight_id, passenger_name, passenger_email, seat_count required")
		return
	}

	// 1) GetFlight
	flightResp, err := h.flight.GetFlight(r.Context(), body.FlightID)
	if err != nil {
		h.log.Error("GetFlight", "err", err)
		h.grpcErrToHTTP(w, err)
		return
	}
	if flightResp.Flight == nil {
		h.writeErr(w, http.StatusNotFound, "flight not found")
		return
	}
	flight := flightResp.Flight

	// 2) Генерируем ID бронирования и резервируем места (идемпотентность по booking_id)
	bookingID := uuid.New().String()
	_, err = h.flight.ReserveSeats(r.Context(), body.FlightID, body.SeatCount, bookingID)
	if err != nil {
		h.log.Error("ReserveSeats", "err", err)
		h.grpcErrToHTTP(w, err)
		return
	}

	// 3) total_price = seat_count * flight.price (snapshot)
	pricePerSeatF, _ := strconv.ParseFloat(flight.Price, 64)
	if pricePerSeatF <= 0 {
		pricePerSeatF = 0
	}
	totalPrice := fmt.Sprintf("%.2f", pricePerSeatF*float64(body.SeatCount))

	// 4) Создаём бронирование в БД (id = bookingID для связи с резервацией)
	bid, _ := uuid.Parse(bookingID)
	b := &repository.Booking{
		ID:             bid,
		UserID:         body.UserID,
		FlightID:       body.FlightID,
		PassengerName:  body.PassengerName,
		PassengerEmail: body.PassengerEmail,
		SeatCount:     body.SeatCount,
		TotalPrice:     totalPrice,
	}
	if err := h.repo.Create(r.Context(), b); err != nil {
		h.log.Error("Create booking", "err", err)
		h.writeErr(w, http.StatusInternalServerError, "failed to create booking")
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         b.ID.String(),
		"user_id":    b.UserID,
		"flight_id":  b.FlightID,
		"passenger_name": b.PassengerName,
		"passenger_email": b.PassengerEmail,
		"seat_count":  b.SeatCount,
		"total_price": b.TotalPrice,
		"status":     "CONFIRMED",
	})
}

// GET /bookings/:id
func (h *Handlers) GetBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		h.writeErr(w, http.StatusBadRequest, "booking id required")
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	b, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		h.writeErr(w, http.StatusNotFound, "booking not found")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":               b.ID.String(),
		"user_id":          b.UserID,
		"flight_id":        b.FlightID,
		"passenger_name":   b.PassengerName,
		"passenger_email":  b.PassengerEmail,
		"seat_count":       b.SeatCount,
		"total_price":      b.TotalPrice,
		"status":           b.Status,
		"created_at":       b.CreatedAt,
	})
}

// POST /bookings/:id/cancel
func (h *Handlers) CancelBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		h.writeErr(w, http.StatusBadRequest, "booking id required")
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	b, err := h.repo.GetByID(r.Context(), id)
	if err != nil || b == nil {
		h.writeErr(w, http.StatusNotFound, "booking not found")
		return
	}
	if b.Status != "CONFIRMED" {
		h.writeErr(w, http.StatusBadRequest, "booking is not in CONFIRMED status")
		return
	}
	_, err = h.flight.ReleaseReservation(r.Context(), b.ID.String())
	if err != nil {
		h.log.Error("ReleaseReservation", "err", err)
		h.grpcErrToHTTP(w, err)
		return
	}
	if err := h.repo.UpdateStatus(r.Context(), id, "CANCELLED"); err != nil {
		h.writeErr(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "CANCELLED"})
}

// GET /bookings?user_id=X
func (h *Handlers) ListBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		h.writeErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	list, err := h.repo.ListByUserID(r.Context(), userID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]interface{}, len(list))
	for i, b := range list {
		items[i] = map[string]interface{}{
			"id":               b.ID.String(),
			"user_id":          b.UserID,
			"flight_id":        b.FlightID,
			"passenger_name":   b.PassengerName,
			"passenger_email":  b.PassengerEmail,
			"seat_count":       b.SeatCount,
			"total_price":      b.TotalPrice,
			"status":           b.Status,
			"created_at":       b.CreatedAt,
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"bookings": items})
}

func (h *Handlers) grpcErrToHTTP(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch st.Code() {
	case codes.NotFound:
		h.writeErr(w, http.StatusNotFound, st.Message())
	case codes.InvalidArgument, codes.FailedPrecondition:
		h.writeErr(w, http.StatusBadRequest, st.Message())
	case codes.ResourceExhausted:
		h.writeErr(w, http.StatusConflict, st.Message())
	case codes.Unavailable:
		h.writeErr(w, http.StatusServiceUnavailable, "service temporarily unavailable")
	case codes.Unauthenticated:
		h.writeErr(w, http.StatusUnauthorized, st.Message())
	default:
		h.writeErr(w, http.StatusInternalServerError, st.Message())
	}
}
