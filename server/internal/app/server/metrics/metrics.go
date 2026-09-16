package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stashito_http_requests_total",
		Help: "HTTP requests served, by request type, method and status code.",
	}, []string{"type", "method", "code"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stashito_http_request_duration_seconds",
		Help:    "HTTP request duration, by request type and method.",
		Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 15, 60, 300},
	}, []string{"type", "method"})

	CacheEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stashito_cache_events_total",
		Help: "Cache lookups on GET requests, by content type and result (hit or miss).",
	}, []string{"type", "result"})

	BlobBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stashito_blob_bytes_served_total",
		Help: "Blob bytes served to clients, by source (cache or upstream).",
	}, []string{"source"})

	UpstreamRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stashito_upstream_requests_total",
		Help: "Requests sent to upstream registries, by registry host, method and status code.",
	}, []string{"registry", "method", "code"})

	UpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stashito_upstream_errors_total",
		Help: "Upstream registry requests that failed before receiving a response, by registry host.",
	}, []string{"registry"})
)

func CacheHit(contentType string) {
	CacheEvents.WithLabelValues(contentType, "hit").Inc()
}

func CacheMiss(contentType string) {
	CacheEvents.WithLabelValues(contentType, "miss").Inc()
}
