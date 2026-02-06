package observability

import (
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "edgectl",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests.",
		},
		[]string{"node", "method", "path", "status"},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "edgectl",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"node", "method", "path", "status"},
	)
	ghostProxyRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "edgectl",
			Subsystem: "ghost_proxy",
			Name:      "requests_total",
			Help:      "Ghost proxy requests from Mirage.",
		},
		[]string{"node", "ghost", "method", "path", "status", "success"},
	)
	ghostProxyDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "edgectl",
			Subsystem: "ghost_proxy",
			Name:      "request_duration_seconds",
			Help:      "Ghost proxy request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"node", "ghost", "method", "path", "status", "success"},
	)
)

func RegisterMetrics() {
	registerOnce.Do(func() {
		prometheus.MustRegister(httpRequests, httpDuration, ghostProxyRequests, ghostProxyDuration)
	})
}

func RecordHTTPRequest(node, method, path string, status int, duration time.Duration) {
	RegisterMetrics()
	statusLabel := strconv.Itoa(status)
	httpRequests.WithLabelValues(node, method, path, statusLabel).Inc()
	httpDuration.WithLabelValues(node, method, path, statusLabel).Observe(duration.Seconds())
}

func RecordGhostProxy(node, ghost, method, path string, status int, duration time.Duration, success bool) {
	RegisterMetrics()
	statusLabel := strconv.Itoa(status)
	successLabel := strconv.FormatBool(success)
	ghostProxyRequests.WithLabelValues(node, ghost, method, path, statusLabel, successLabel).Inc()
	ghostProxyDuration.WithLabelValues(node, ghost, method, path, statusLabel, successLabel).
		Observe(duration.Seconds())
}
