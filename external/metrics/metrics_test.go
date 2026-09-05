package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveRequestAndBusinessCounters(t *testing.T) {
	m := New()

	m.ObserveRequest("GET", "/health", http.StatusOK, 10*time.Millisecond)
	m.IncURLsCreated()
	m.IncRedirectsServed()

	if got := testutil.ToFloat64(urlsCreatedTotal); got != 1 {
		t.Fatalf("urls created counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(redirectsServedTotal); got != 1 {
		t.Fatalf("redirects served counter = %v, want 1", got)
	}
}

func TestHandlerExposesMetrics(t *testing.T) {
	m := New()
	m.ObserveRequest("POST", "", http.StatusCreated, time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	m.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics handler status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"shortener_urls_created_total",
		"shortener_redirects_served_total",
		`route="unmatched"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}
