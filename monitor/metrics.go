package monitor

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 性能指标收集器
type Metrics struct {
	RequestCount        prometheus.Counter
	RequestDuration     prometheus.Histogram
	ErrorCount          prometheus.Counter
	ActiveConnections   prometheus.Gauge
	BackendHealthyCount prometheus.Gauge
}

// NewMetrics 创建新的性能指标收集器
func NewMetrics() *Metrics {
	return &Metrics{
		RequestCount: promauto.NewCounter(prometheus.CounterOpts{
			Name: "go_nginx_requests_total",
			Help: "Total number of requests processed",
		}),
		RequestDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "go_nginx_request_duration_seconds",
			Help:    "Duration of requests in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		ErrorCount: promauto.NewCounter(prometheus.CounterOpts{
			Name: "go_nginx_errors_total",
			Help: "Total number of errors",
		}),
		ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "go_nginx_active_connections",
			Help: "Current number of active connections",
		}),
		BackendHealthyCount: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "go_nginx_backend_healthy_count",
			Help: "Number of healthy backend servers",
		}),
	}
}

// RecordRequest 记录请求指标
func (m *Metrics) RecordRequest(duration time.Duration, isError bool) {
	m.RequestCount.Inc()
	m.RequestDuration.Observe(duration.Seconds())
	m.ActiveConnections.Inc()
	defer m.ActiveConnections.Dec()

	if isError {
		m.ErrorCount.Inc()
	}
}
