package telemetry

import "time"

type RequestMetric struct {
	Method    string
	Path      string
	Status    string
	StartTime time.Time
}

type MetricExporter interface {
	SendRequestMetric(m RequestMetric)
}
