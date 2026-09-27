package middleware

import (
	"net/http"
	"time"

	"github.com/vicky/url-shortner/external/metrics"
)

// Metrics returns middleware that records HTTP request rate, latency, and
// status distribution into the given metrics backend. Requests that match no
// registered route pattern are grouped under the "unmatched" label to keep
// label cardinality bounded.
func Metrics(m metrics.Metrics) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriter(w)

			next.ServeHTTP(wrapped, r)

			m.ObserveRequest(r.Method, r.Pattern, wrapped.statusCode, time.Since(start))
		})
	}
}
