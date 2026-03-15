package middleware

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CircuitBreakerState для логов
type CircuitBreakerState string

const (
	StateClosed   CircuitBreakerState = "CLOSED"
	StateOpen     CircuitBreakerState = "OPEN"
	StateHalfOpen CircuitBreakerState = "HALF_OPEN"
)

type circuitBreaker struct {
	mu          sync.Mutex
	state       CircuitBreakerState
	failures    uint32
	threshold   uint32
	timeout     time.Duration
	lastFailure time.Time
	lastOpen    time.Time
	log         *slog.Logger
}

// NewCircuitBreakerUnaryClientInterceptor создаёт interceptor с Circuit Breaker.
// threshold — порог ошибок подряд для перехода в OPEN.
// timeout — время в OPEN перед переходом в HALF_OPEN.
func NewCircuitBreakerUnaryClientInterceptor(threshold uint32, timeout time.Duration, log *slog.Logger) grpc.UnaryClientInterceptor {
	cb := &circuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
		log:       log,
	}
	if log == nil {
		cb.log = slog.Default()
	}
	return cb.intercept
}

func (cb *circuitBreaker) intercept(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	cb.mu.Lock()
	state := cb.state
	switch state {
	case StateOpen:
		if time.Since(cb.lastOpen) >= cb.timeout {
			cb.state = StateHalfOpen
			cb.log.Info("circuit breaker state", "from", StateOpen, "to", StateHalfOpen)
			state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			cb.log.Debug("circuit open, rejecting call")
			return status.Error(codes.Unavailable, "service temporarily unavailable")
		}
	case StateHalfOpen:
		// пропускаем один пробный запрос
	default:
		// Closed
	}
	cb.mu.Unlock()

	err := invoker(ctx, method, req, reply, cc, opts...)
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded || st.Code() == codes.Internal {
			switch cb.state {
			case StateHalfOpen:
				cb.state = StateOpen
				cb.lastOpen = time.Now()
				cb.log.Info("circuit breaker state", "from", StateHalfOpen, "to", StateOpen)
			case StateClosed:
				cb.failures++
				if cb.failures >= cb.threshold {
					cb.state = StateOpen
					cb.lastOpen = time.Now()
					cb.log.Info("circuit breaker state", "from", StateClosed, "to", StateOpen, "failures", cb.failures)
				}
			}
		}
		return err
	}

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
		cb.log.Info("circuit breaker state", "from", StateHalfOpen, "to", StateClosed)
	} else if cb.state == StateClosed {
		cb.failures = 0
	}
	return nil
}
