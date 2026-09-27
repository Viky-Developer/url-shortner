package graceful

import (
	"net/http"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
)

func TestShutdownSuccess(t *testing.T) {
	log, err := logger.New(logger.WithLevel("error"))
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}

	srv := &http.Server{Addr: ":0"}
	// Start and immediately shut down (no active connections).
	Shutdown(srv, log, 5*time.Second)
}

func TestShutdownLogsError(t *testing.T) {
	log, err := logger.New(logger.WithLevel("error"))
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}

	// Shutdown on a nil server should not panic — it will log the error.
	srv := &http.Server{Addr: ":0"}
	_ = srv.Close()
	// After Close, Shutdown returns http.ErrServerClosed which is logged.
	Shutdown(srv, log, 1*time.Second)
}
