package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const metadataKey = "authorization" // или "x-api-key" — выбран API Key в metadata

func RequireAPIKey(apiKey string) func(context.Context) error {
	return func(ctx context.Context) error {
		if apiKey == "" {
			return nil
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return status.Error(codes.Unauthenticated, "missing credentials")
		}
		vals := md.Get(metadataKey)
		if len(vals) == 0 {
			vals = md.Get("x-api-key")
		}
		var token string
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if strings.HasPrefix(strings.ToLower(v), "bearer ") {
				token = strings.TrimPrefix(v[7:], " ")
				break
			}
			token = v
			break
		}
		if token == "" {
			return status.Error(codes.Unauthenticated, "missing credentials")
		}
		if token != apiKey {
			return status.Error(codes.Unauthenticated, "invalid credentials")
		}
		return nil
	}
}
