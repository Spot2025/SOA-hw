package grpc

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	flightv1 "soa-hw/proto/gen/go/flight/v1"
	"soa-hw/flight-service/internal/cache"
	"soa-hw/flight-service/internal/repository"
)

type Server struct {
	flightv1.UnimplementedFlightServiceServer
	repo  *repository.FlightRepo
	cache *cache.Cache
	log   *slog.Logger
}

func NewServer(db *sqlx.DB, c *cache.Cache, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		repo:  repository.NewFlightRepo(db),
		cache: c,
		log:   log,
	}
}

func flightToProto(f *repository.Flight) *flightv1.Flight {
	if f == nil {
		return nil
	}
	st := flightStatusToProto(f.Status)
	return &flightv1.Flight{
		Id:             f.ID,
		FlightNumber:   f.FlightNumber,
		Airline:        f.Airline,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureTime:  timestamppb.New(f.DepartureTime),
		ArrivalTime:    timestamppb.New(f.ArrivalTime),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		Price:          f.Price,
		Status:         st,
	}
}

func flightStatusToProto(s string) flightv1.FlightStatus {
	switch s {
	case "SCHEDULED":
		return flightv1.FlightStatus_FLIGHT_STATUS_SCHEDULED
	case "DEPARTED":
		return flightv1.FlightStatus_FLIGHT_STATUS_DEPARTED
	case "CANCELLED":
		return flightv1.FlightStatus_FLIGHT_STATUS_CANCELLED
	case "COMPLETED":
		return flightv1.FlightStatus_FLIGHT_STATUS_COMPLETED
	default:
		return flightv1.FlightStatus_FLIGHT_STATUS_UNSPECIFIED
	}
}

func reservationStatusToProto(s string) flightv1.ReservationStatus {
	switch s {
	case "ACTIVE":
		return flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE
	case "RELEASED":
		return flightv1.ReservationStatus_RESERVATION_STATUS_RELEASED
	case "EXPIRED":
		return flightv1.ReservationStatus_RESERVATION_STATUS_EXPIRED
	default:
		return flightv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED
	}
}

func (s *Server) SearchFlights(ctx context.Context, req *flightv1.SearchFlightsRequest) (*flightv1.SearchFlightsResponse, error) {
	if req.Origin == "" || req.Destination == "" {
		return nil, status.Error(codes.InvalidArgument, "origin and destination are required")
	}
	date := req.Date

	if s.cache != nil {
		if data, ok, _ := s.cache.GetSearch(ctx, req.Origin, req.Destination, date); ok && len(data) > 0 {
			var list []repository.Flight
			if err := cache.UnmarshalFlight(data, &list); err == nil {
				flights := make([]*flightv1.Flight, len(list))
				for i := range list {
					flights[i] = flightToProto(&list[i])
				}
				return &flightv1.SearchFlightsResponse{Flights: flights}, nil
			}
		}
	}

	list, err := s.repo.Search(ctx, req.Origin, req.Destination, date)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	flights := make([]*flightv1.Flight, len(list))
	for i := range list {
		flights[i] = flightToProto(&list[i])
	}

	if s.cache != nil && len(list) > 0 {
		if data, err := cache.MarshalFlight(list); err == nil {
			_ = s.cache.SetSearch(ctx, req.Origin, req.Destination, date, data)
		}
	}
	return &flightv1.SearchFlightsResponse{Flights: flights}, nil
}

func (s *Server) GetFlight(ctx context.Context, req *flightv1.GetFlightRequest) (*flightv1.GetFlightResponse, error) {
	if req.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid flight id")
	}

	if s.cache != nil {
		if data, ok, _ := s.cache.GetFlight(ctx, req.Id); ok && len(data) > 0 {
			var f repository.Flight
			if err := cache.UnmarshalFlight(data, &f); err == nil {
				return &flightv1.GetFlightResponse{Flight: flightToProto(&f)}, nil
			}
		}
	}

	f, err := s.repo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if f == nil {
		return nil, status.Error(codes.NotFound, "flight not found")
	}

	if s.cache != nil {
		if data, err := cache.MarshalFlight(f); err == nil {
			_ = s.cache.SetFlight(ctx, f.ID, data)
		}
	}
	return &flightv1.GetFlightResponse{Flight: flightToProto(f)}, nil
}

func (s *Server) ReserveSeats(ctx context.Context, req *flightv1.ReserveSeatsRequest) (*flightv1.ReserveSeatsResponse, error) {
	if req.FlightId <= 0 || req.SeatCount <= 0 || req.BookingId == "" {
		return nil, status.Error(codes.InvalidArgument, "flight_id, seat_count and booking_id are required")
	}

	// Получаем рейс для инвалидации кеша после мутации
	var origin, dest string
	var depTime time.Time
	if s.cache != nil {
		f, _ := s.repo.GetByID(ctx, req.FlightId)
		if f != nil {
			origin, dest, depTime = f.Origin, f.Destination, f.DepartureTime
		}
	}

	reservationID, existed, err := s.repo.ReserveSeats(ctx, req.FlightId, req.SeatCount, req.BookingId)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, status.Error(codes.NotFound, "flight not found")
		}
		if err == repository.ErrResourceExhausted {
			return nil, status.Error(codes.ResourceExhausted, "not enough seats")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if s.cache != nil && (origin != "" || dest != "") {
		_ = s.cache.InvalidateFlightAndSearch(ctx, req.FlightId, origin, dest, depTime)
	}

	st := flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE
	if existed {
		st = flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE
	}
	return &flightv1.ReserveSeatsResponse{
		ReservationId: reservationID,
		BookingId:     req.BookingId,
		FlightId:      req.FlightId,
		SeatCount:     req.SeatCount,
		Status:        st,
	}, nil
}

func (s *Server) ReleaseReservation(ctx context.Context, req *flightv1.ReleaseReservationRequest) (*flightv1.ReleaseReservationResponse, error) {
	if req.BookingId == "" {
		return nil, status.Error(codes.InvalidArgument, "booking_id is required")
	}

	result, err := s.repo.ReleaseReservation(ctx, req.BookingId)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, status.Error(codes.NotFound, "reservation not found or already released")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if s.cache != nil {
		_ = s.cache.InvalidateFlightAndSearch(ctx, result.FlightID, result.Origin, result.Destination, result.DepartureTime)
	}
	return &flightv1.ReleaseReservationResponse{
		BookingId:      req.BookingId,
		PreviousStatus: reservationStatusToProto(result.PrevStatus),
	}, nil
}
