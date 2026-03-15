package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryUnaryClientInterceptor возвращает interceptor с exponential backoff.
// Retry только для UNAVAILABLE, DEADLINE_EXCEEDED. Максимум maxAttempts попыток.
func RetryUnaryClientInterceptor(maxAttempts int, baseDelay time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		delay := baseDelay
		for attempt := 0; attempt < maxAttempts; attempt++ {
			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			if lastErr == nil {
				return nil
			}
			st, ok := status.FromError(lastErr)
			if !ok {
				return lastErr
			}
			// Без retry для: INVALID_ARGUMENT, NOT_FOUND, RESOURCE_EXHAUSTED, UNAUTHENTICATED
			switch st.Code() {
			case codes.Unavailable, codes.DeadlineExceeded:
				// retry
			default:
				return lastErr
			}
			if attempt == maxAttempts-1 {
				return lastErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2 // exponential backoff
			}
		}
		return lastErr
	}
}
