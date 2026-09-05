package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const unmatchedRoute = "unmatched"

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests handled, labelled by method, route, and response status.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Latency of HTTP requests in seconds, labelled by method and route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	urlsCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "shortener_urls_created_total",
			Help: "Total number of short URLs created.",
		},
	)

	redirectsServedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "shortener_redirects_served_total",
			Help: "Total number of short-URL redirects served.",
		},
	)
)

// Prometheus implements Metrics on the default Prometheus registry.
type Prometheus struct{}

// New returns a Metrics backend backed by the default Prometheus registry.
func New() Metrics { return &Prometheus{} }

// Handler returns the standard promhttp handler exposing all collected metrics.
func (p *Prometheus) Handler() http.Handler { return promhttp.Handler() }

// ObserveRequest records the request rate, latency, and status distribution.
// Routes without a matched mux pattern are grouped under the "unmatched" label
// to keep label cardinality bounded.
func (p *Prometheus) ObserveRequest(method, route string, status int, duration time.Duration) {
	if route == "" {
		route = unmatchedRoute
	}
	statusLabel := strconv.Itoa(status)

	httpRequestsTotal.WithLabelValues(method, route, statusLabel).Inc()
	httpRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}

// IncURLsCreated increments the URLs-created counter.
func (p *Prometheus) IncURLsCreated() { urlsCreatedTotal.Inc() }

// IncRedirectsServed increments the redirects-served counter.
func (p *Prometheus) IncRedirectsServed() { redirectsServedTotal.Inc() }
