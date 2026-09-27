package metrics

import (
	"net/http"
	"time"
)

// Noop is a Metrics implementation that discards all observations. It is the
// default backend used by code that does not opt into metrics collection.
type Noop struct{}

// Handler returns a handler that serves an empty response.
func (Noop) Handler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

// ObserveRequest records nothing.
func (Noop) ObserveRequest(string, string, int, time.Duration) {}

// IncURLsCreated records nothing.
func (Noop) IncURLsCreated() {}

// IncRedirectsServed records nothing.
func (Noop) IncRedirectsServed() {}
