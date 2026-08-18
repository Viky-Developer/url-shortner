package routes

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/handler"
)

func TestNew(t *testing.T) {
	log, _ := logger.New()
	h := handler.NewURLHandler(nil, log)
	mux := New(h)

	if mux == nil {
		t.Fatal("expected non-nil handler from New()")
	}

	if _, ok := mux.(*http.ServeMux); !ok {
		t.Fatal("expected *http.ServeMux from New()")
	}
}

func TestRoutesRegistered(t *testing.T) {
	log, _ := logger.New()
	h := handler.NewURLHandler(nil, log)
	mux := New(h)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/123/shorten"},
		{http.MethodGet, "/api/v1/abc"},
		{http.MethodGet, "/api/v1/123/urls"},
		{http.MethodGet, "/api/v1/123/urls/456"},
		{http.MethodPatch, "/api/v1/123/urls/456"},
		{http.MethodDelete, "/api/v1/123/urls/456"},
		{http.MethodDelete, "/api/v1/123/urls/456/approve"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req := &http.Request{
				Method: rt.method,
				URL:    &url.URL{Path: rt.path},
			}
			_, pattern := mux.(*http.ServeMux).Handler(req)
			if pattern == "" {
				t.Fatalf("no route registered for %s %s", rt.method, rt.path)
			}
		})
	}
}
