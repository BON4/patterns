package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusMetrics struct {
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
}

func RegisterPrometheusMetricsExporter() (*PrometheusMetrics, error) {
	var pm PrometheusMetrics

	pm.httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	if err := prometheus.Register(pm.httpDuration); err != nil {
		return nil, err
	}

	pm.httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	if err := prometheus.Register(pm.httpRequests); err != nil {
		return nil, err
	}

	return &pm, nil
}

func (pm *PrometheusMetrics) SendRequestMetric(m RequestMetric) {
	pm.httpRequests.WithLabelValues(
		m.Method, m.Path, m.Status,
	).Inc()

	pm.httpDuration.WithLabelValues(
		m.Method, m.Path, m.Status,
	).Observe(time.Since(m.StartTime).Seconds())
}
