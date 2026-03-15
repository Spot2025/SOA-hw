package grpcclient

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	flightv1 "soa-hw/proto/gen/go/flight/v1"
)

type Client struct {
	conn   *grpc.ClientConn
	client flightv1.FlightServiceClient
	apiKey string
	log    *slog.Logger
}

func New(ctx context.Context, addr, apiKey string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		client: flightv1.NewFlightServiceClient(conn),
		apiKey: apiKey,
		log:    slog.Default(),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) withAuth(ctx context.Context) context.Context {
	if c.apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.apiKey, "x-api-key", c.apiKey)
}

func (c *Client) SearchFlights(ctx context.Context, origin, destination, date string) (*flightv1.SearchFlightsResponse, error) {
	return c.client.SearchFlights(c.withAuth(ctx), &flightv1.SearchFlightsRequest{
		Origin:      origin,
		Destination: destination,
		Date:        date,
	})
}

func (c *Client) GetFlight(ctx context.Context, flightID int64) (*flightv1.GetFlightResponse, error) {
	return c.client.GetFlight(c.withAuth(ctx), &flightv1.GetFlightRequest{Id: flightID})
}

func (c *Client) ReserveSeats(ctx context.Context, flightID int64, seatCount int32, bookingID string) (*flightv1.ReserveSeatsResponse, error) {
	return c.client.ReserveSeats(c.withAuth(ctx), &flightv1.ReserveSeatsRequest{
		FlightId:   flightID,
		SeatCount:  seatCount,
		BookingId:  bookingID,
	})
}

func (c *Client) ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error) {
	return c.client.ReleaseReservation(c.withAuth(ctx), &flightv1.ReleaseReservationRequest{
		BookingId: bookingID,
	})
}

// Retry and Circuit Breaker will be added as dial/interceptor options when building the client from main.
