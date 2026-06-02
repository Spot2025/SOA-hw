package metrics

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor records gRPC RED metrics.
// hello
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		endpoint := info.FullMethod
		method := extractGRPCMethod(endpoint)

		resp, err := handler(ctx, req)

		code := codes.OK
		if err != nil {
			if st, ok := status.FromError(err); ok {
				code = st.Code()
			} else {
				code = codes.Unknown
			}
		}

		statusLabel := code.String()
		GRPCRequestsTotal.WithLabelValues(method, endpoint, statusLabel).Inc()
		GRPCRequestDuration.WithLabelValues(method, endpoint).Observe(time.Since(start).Seconds())

		if err != nil && code != codes.OK {
			GRPCRequestErrorsTotal.WithLabelValues(method, endpoint, classifyGRPCError(code)).Inc()
		}

		return resp, err
	}
}

func extractGRPCMethod(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return fullMethod
}

func classifyGRPCError(code codes.Code) string {
	switch code {
	case codes.NotFound:
		return "not_found"
	case codes.InvalidArgument, codes.FailedPrecondition:
		return "validation"
	case codes.ResourceExhausted:
		return "resource_exhausted"
	case codes.Unavailable:
		return "unavailable"
	case codes.Unauthenticated, codes.PermissionDenied:
		return "auth"
	default:
		return "internal"
	}
}
