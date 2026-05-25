package metrics

import (
	"net/http"
	"strconv"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware wraps an HTTP handler and records RED metrics.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		endpoint := normalizeEndpoint(r.Pattern, r.URL.Path)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		status := strconv.Itoa(rec.status)
		method := r.Method
		HTTPRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, endpoint).Observe(time.Since(start).Seconds())

		if rec.status >= 400 {
			errorType := classifyHTTPError(rec.status)
			HTTPRequestErrorsTotal.WithLabelValues(method, endpoint, errorType).Inc()
		}
	})
}

func normalizeEndpoint(pattern, path string) string {
	if pattern != "" {
		return pattern
	}
	return path
}

func classifyHTTPError(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status == 404:
		return "not_found"
	case status == 409:
		return "conflict"
	case status == 503:
		return "unavailable"
	default:
		return "client_error"
	}
}
