package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client redis.UniversalClient
	ttl   time.Duration
	log   *slog.Logger
}

func New(client redis.UniversalClient, ttl time.Duration, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.Default()
	}
	return &Cache{client: client, ttl: ttl, log: log}
}

func (c *Cache) GetFlight(ctx context.Context, id int64) ([]byte, bool, error) {
	key := fmt.Sprintf("flight:%d", id)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		c.log.Info("cache miss", "key", key)
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	c.log.Info("cache hit", "key", key)
	return data, true, nil
}

func (c *Cache) SetFlight(ctx context.Context, id int64, data []byte) error {
	key := fmt.Sprintf("flight:%d", id)
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return err
	}
	return nil
}

func (c *Cache) InvalidateFlight(ctx context.Context, id int64) error {
	key := fmt.Sprintf("flight:%d", id)
	return c.client.Del(ctx, key).Err()
}

func (c *Cache) GetSearch(ctx context.Context, origin, destination, date string) ([]byte, bool, error) {
	key := fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		c.log.Info("cache miss", "key", key)
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	c.log.Info("cache hit", "key", key)
	return data, true, nil
}

func (c *Cache) SetSearch(ctx context.Context, origin, destination, date string, data []byte) error {
	key := fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
	return c.client.Set(ctx, key, data, c.ttl).Err()
}

func (c *Cache) InvalidateSearch(ctx context.Context, origin, destination, date string) error {
	key := fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
	return c.client.Del(ctx, key).Err()
}

// InvalidateFlightAndSearch вызывается после мутаций (ReserveSeats, ReleaseReservation, UpdateFlight)
func (c *Cache) InvalidateFlightAndSearch(ctx context.Context, flightID int64, origin, destination string, departureDate time.Time) error {
	date := departureDate.Format("2006-01-02")
	keys := []string{
		fmt.Sprintf("flight:%d", flightID),
		fmt.Sprintf("search:%s:%s:%s", origin, destination, date),
	}
	for _, k := range keys {
		if err := c.client.Del(ctx, k).Err(); err != nil {
			return err
		}
	}
	return nil
}

func MarshalFlight(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func UnmarshalFlight(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
