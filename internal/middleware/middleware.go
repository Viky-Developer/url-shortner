// Package middleware provides reusable HTTP middleware for the application:
// request logging, panic recovery, and JSON content-type enforcement.
package middleware

import (
	"net/http"
	"time"

	"github.com/vicky/url-shortner/external/logger"
)

// responseWriter wraps http.ResponseWriter to capture the response status code
// so that the logging middleware can report it after the handler runs.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// newResponseWriter returns a responseWriter initialised with the default
// 200 OK status so that handlers which never call WriteHeader are still
// reported correctly.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

// WriteHeader records the status code before delegating to the wrapped writer.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logger returns middleware that logs every request with its method, path,
// response status, duration, and remote address.
func Logger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriter(w)

			next.ServeHTTP(wrapped, r)

			log.Info("request",
				logger.String("method", r.Method),
				logger.String("path", r.URL.Path),
				logger.Int("status", wrapped.statusCode),
				logger.String("duration", time.Since(start).String()),
				logger.String("remote", r.RemoteAddr),
			)
		})
	}
}

// Recovery returns middleware that recovers from panics raised by downstream
// handlers, logs the panic, and responds with 500 Internal Server Error.
func Recovery(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("panic recovered",
						logger.Any("error", err),
						logger.String("path", r.URL.Path),
					)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// ContentTypeJSON returns middleware that sets the Content-Type header to
// application/json on every response.
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// Chain composes multiple middlewares into a single handler. Middlewares are
// applied right to left, so the first middleware in the list wraps the handler
// last and therefore runs first.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
