package middleware

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func Logging(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			var bodyStr string
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
				bodyBytes, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				bodyStr = string(bodyBytes)
				if len(bodyStr) > 4096 {
					bodyStr = bodyStr[:4096] + "...(truncated)"
				}
			}

			next.ServeHTTP(rec, r)

			duration := time.Since(start)
			evt := logger.Info().
				Str("request_id", GetRequestID(r.Context())).
				Str("method", r.Method).
				Str("endpoint", r.URL.Path).
				Int("status_code", rec.statusCode).
				Int64("duration_ms", duration.Milliseconds()).
				Str("timestamp", start.UTC().Format(time.RFC3339))

			if uid, ok := r.Context().Value(UserIDKey).(int64); ok && uid != 0 {
				evt = evt.Int64("user_id", uid)
			}

			if bodyStr != "" {
				evt = evt.Str("request_body", bodyStr)
			}

			evt.Send()
		})
	}
}
