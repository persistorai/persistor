// Package metrics defines Prometheus metrics for the persistor.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "persistor_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "persistor_http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "persistor_errors_total",
			Help: "Total errors by type",
		},
		[]string{"type"},
	)

	EmbedQueueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "persistor_embed_queue_depth",
			Help: "Current embedding queue depth",
		},
	)

	WSConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "persistor_websocket_connections",
			Help: "Active WebSocket connections",
		},
	)

	NodeCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "persistor_nodes_total",
			Help: "Total node count",
		},
	)

	EdgeCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "persistor_edges_total",
			Help: "Total edge count",
		},
	)
)

func init() {
	prometheus.MustRegister(
		RequestDuration, RequestsTotal, ErrorsTotal,
		EmbedQueueDepth, WSConnections,
		NodeCount, EdgeCount,
	)
}
