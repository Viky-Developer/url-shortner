// Package graceful provides helpers for graceful server shutdown.
package graceful

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vicky/url-shortner/external/logger"
)

// WaitForSignal blocks until an interrupt (Ctrl+C) or SIGTERM signal is
// received. It should be called before Shutdown so the server can drain
// in-flight requests.
func WaitForSignal() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

// Shutdown gracefully stops the server, waiting up to timeout for active
// connections to drain before forcing a close. Any shutdown error is logged.
func Shutdown(server *http.Server, log logger.Logger, timeout time.Duration) {
	log.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("server shutdown failed", logger.Error(err))
	}
}
