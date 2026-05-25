package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "soa_hw"

// HTTP metrics (booking-service).
var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests.",
	}, []string{"method", "endpoint", "status"})

	HTTPRequestErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_request_errors_total",
		Help:      "Total number of HTTP request errors.",
	}, []string{"method", "endpoint", "error_type"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "endpoint"})
)

// gRPC metrics (flight-service).
var (
	GRPCRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "grpc_requests_total",
		Help:      "Total number of gRPC requests.",
	}, []string{"method", "endpoint", "status"})

	GRPCRequestErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "grpc_request_errors_total",
		Help:      "Total number of gRPC request errors.",
	}, []string{"method", "endpoint", "error_type"})

	GRPCRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "grpc_request_duration_seconds",
		Help:      "gRPC request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "endpoint"})
)
