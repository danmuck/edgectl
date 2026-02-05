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
	seedProxyRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "edgectl",
			Subsystem: "seed_proxy",
			Name:      "requests_total",
			Help:      "Seed proxy requests from Ghost.",
		},
		[]string{"node", "seed", "method", "path", "status", "success"},
	)
	seedProxyDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "edgectl",
			Subsystem: "seed_proxy",
			Name:      "request_duration_seconds",
			Help:      "Seed proxy request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"node", "seed", "method", "path", "status", "success"},
	)
)

func RegisterMetrics() {
	registerOnce.Do(func() {
		prometheus.MustRegister(httpRequests, httpDuration, seedProxyRequests, seedProxyDuration)
	})
}

func RecordHTTPRequest(node, method, path string, status int, duration time.Duration) {
	RegisterMetrics()
	statusLabel := strconv.Itoa(status)
	httpRequests.WithLabelValues(node, method, path, statusLabel).Inc()
	httpDuration.WithLabelValues(node, method, path, statusLabel).Observe(duration.Seconds())
}

func RecordSeedProxy(node, seed, method, path string, status int, duration time.Duration, success bool) {
	RegisterMetrics()
	statusLabel := strconv.Itoa(status)
	successLabel := strconv.FormatBool(success)
	seedProxyRequests.WithLabelValues(node, seed, method, path, statusLabel, successLabel).Inc()
	seedProxyDuration.WithLabelValues(node, seed, method, path, statusLabel, successLabel).
		Observe(duration.Seconds())
}
