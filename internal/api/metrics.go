package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const metricsNamespace = "cliproxyapi"

type serverMetrics struct {
	registry        *prometheus.Registry
	inFlight        prometheus.Gauge
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestSize     *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
	handler         http.Handler
}

func newServerMetrics() *serverMetrics {
	registry := prometheus.NewRegistry()
	metrics := &serverMetrics{
		registry: registry,
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Current number of HTTP requests being served.",
		}),
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests served.",
		}, []string{"method", "route", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latencies in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		requestSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "http",
			Name:      "request_size_bytes",
			Help:      "Approximate HTTP request sizes in bytes.",
			Buckets:   prometheus.ExponentialBuckets(128, 2, 10),
		}, []string{"method", "route"}),
		responseSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP response sizes in bytes.",
			Buckets:   prometheus.ExponentialBuckets(128, 2, 10),
		}, []string{"method", "route", "status"}),
	}

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metrics.inFlight,
		metrics.requestsTotal,
		metrics.requestDuration,
		metrics.requestSize,
		metrics.responseSize,
	)
	metrics.handler = promhttp.InstrumentMetricHandler(registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	return metrics
}

func (m *serverMetrics) Middleware() gin.HandlerFunc {
	if m == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if c.Request != nil && c.Request.URL != nil && c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		startedAt := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		requestSize := estimateRequestSize(c.Request)
		c.Next()

		method := normalizeHTTPMethod(c.Request)
		route := normalizeHTTPRoute(c)
		status := strconv.Itoa(c.Writer.Status())

		m.requestsTotal.WithLabelValues(method, route, status).Inc()
		m.requestDuration.WithLabelValues(method, route, status).Observe(time.Since(startedAt).Seconds())
		m.requestSize.WithLabelValues(method, route).Observe(float64(requestSize))

		responseSize := c.Writer.Size()
		if responseSize < 0 {
			responseSize = 0
		}
		m.responseSize.WithLabelValues(method, route, status).Observe(float64(responseSize))
	}
}

func (m *serverMetrics) Handler() http.Handler {
	if m == nil {
		return promhttp.Handler()
	}
	return m.handler
}

func normalizeHTTPMethod(req *http.Request) string {
	if req == nil {
		return "UNKNOWN"
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		return "UNKNOWN"
	}
	return method
}

func normalizeHTTPRoute(c *gin.Context) string {
	if c == nil {
		return "unknown"
	}
	if route := strings.TrimSpace(c.FullPath()); route != "" {
		return route
	}
	if c.Request != nil && c.Request.URL != nil && strings.TrimSpace(c.Request.URL.Path) != "" {
		return "unmatched"
	}
	return "unknown"
}

func estimateRequestSize(req *http.Request) int {
	if req == nil {
		return 0
	}

	size := 0
	size += len(req.Method)
	size += len(req.Proto)
	size += len(req.Host)

	if req.URL != nil {
		size += len(req.URL.Path)
		size += len(req.URL.RawQuery)
	}

	for name, values := range req.Header {
		size += len(name)
		for _, value := range values {
			size += len(value)
		}
	}

	if req.ContentLength > 0 {
		size += int(req.ContentLength)
	}

	return size
}
