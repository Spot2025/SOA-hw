package handlers

import (
	"context"

	flightv1 "soa-hw/proto/gen/go/flight/v1"
)

// FlightAPI abstracts gRPC flight client for testing.
type FlightAPI interface {
	SearchFlights(ctx context.Context, origin, destination, date string) (*flightv1.SearchFlightsResponse, error)
	GetFlight(ctx context.Context, flightID int64) (*flightv1.GetFlightResponse, error)
	ReserveSeats(ctx context.Context, flightID int64, seatCount int32, bookingID string) (*flightv1.ReserveSeatsResponse, error)
	ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error)
}
