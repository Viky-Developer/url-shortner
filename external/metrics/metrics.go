// Package metrics exposes the Prometheus-backed application metrics consumed
// by middleware and services. Implementations live in this package; consumers
// depend only on the interface.
package metrics

import (
	"net/http"
	"time"
)

// Metrics is the contract used to record HTTP request observability and the
// business counters tracked by the URL shortener.
type Metrics interface {
	// Handler returns the HTTP handler that exposes the metrics in Prometheus
	// text exposition format.
	Handler() http.Handler

	// ObserveRequest records an HTTP request with its method, matched route
	// pattern, response status, and total duration.
	ObserveRequest(method, route string, status int, duration time.Duration)

	// IncURLsCreated records a successfully created short URL.
	IncURLsCreated()

	// IncRedirectsServed records a served short-URL redirect.
	IncRedirectsServed()
}
