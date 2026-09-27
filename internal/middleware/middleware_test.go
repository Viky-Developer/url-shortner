package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vicky/url-shortner/external/logger"
)

func TestRecovery(t *testing.T) {
	log, _ := logger.New()

	t.Run("recovers from panic", func(t *testing.T) {
		panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		Recovery(log)(panicking).ServeHTTP(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Internal Server Error") {
			t.Fatalf("expected error body, got %s", w.Body.String())
		}
	})

	t.Run("passes through without panic", func(t *testing.T) {
		ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		Recovery(log)(ok).ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

func TestContentTypeJSON(t *testing.T) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("rejects non-json content type with body", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"a":1}`))
		r.Header.Set("Content-Type", "text/plain")
		r.ContentLength = 7

		ContentTypeJSON(noop).ServeHTTP(w, r)

		if w.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", w.Code)
		}
	})

	t.Run("allows application/json", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"a":1}`))
		r.Header.Set("Content-Type", "application/json")
		r.ContentLength = 7

		ContentTypeJSON(noop).ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("allows requests without body", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		ContentTypeJSON(noop).ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

func TestChain(t *testing.T) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Mw1", "applied")
			next.ServeHTTP(w, r)
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Mw2", "applied")
			next.ServeHTTP(w, r)
		})
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	Chain(noop, mw1, mw2).ServeHTTP(w, r)

	if w.Header().Get("X-Mw1") != "applied" {
		t.Fatal("mw1 was not applied")
	}
	if w.Header().Get("X-Mw2") != "applied" {
		t.Fatal("mw2 was not applied")
	}
}

func TestNewResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := newResponseWriter(w)

	if rw.statusCode != http.StatusOK {
		t.Fatalf("expected default status 200, got %d", rw.statusCode)
	}

	rw.WriteHeader(http.StatusCreated)
	if rw.statusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rw.statusCode)
	}
}

func TestColorMethod(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "OPTIONS"}
	for _, m := range methods {
		result := colorMethod(m)
		if !strings.Contains(result, m) {
			t.Fatalf("colorMethod(%q) should contain the method, got %s", m, result)
		}
	}
}

func TestColorStatus(t *testing.T) {
	codes := []int{100, 200, 301, 400, 500}
	for _, c := range codes {
		result := colorStatus(c)
		if result == "" {
			t.Fatalf("colorStatus(%d) returned empty string", c)
		}
	}
}
