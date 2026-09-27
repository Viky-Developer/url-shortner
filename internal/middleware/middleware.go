// Package middleware provides reusable HTTP middleware for the application:
// request logging, panic recovery, and JSON content-type enforcement.
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/utils"
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

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Logger returns middleware that logs every request with colour-coded status:
//
//	2xx → green, 4xx → yellow, 5xx → red.
func Logger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriter(w)

			next.ServeHTTP(wrapped, r)

			status := wrapped.statusCode
			duration := time.Since(start)

			// Colorize method and status for terminal output
			method := colorMethod(r.Method)
			statusStr := colorStatus(status)

			// Print colored log directly to terminal (bypassing zap's escaping)
			fmt.Printf("%s %s %s %s %s\n", method, r.URL.Path, statusStr, duration, r.RemoteAddr)

			// Log with structured fields (no color, for JSON/structured logging)
			log.Info("request",
				logger.String("method", utils.SanitizeLog(r.Method)),
				logger.String("path", utils.SanitizeLog(r.URL.Path)),
				logger.Int("status", status),
				logger.String("duration", duration.String()),
				logger.String("remote", utils.SanitizeLog(r.RemoteAddr)),
			)
		})
	}
}

func colorMethod(method string) string {
	switch method {
	case http.MethodGet:
		return colorCyan + method + colorReset
	case http.MethodPost:
		return colorGreen + method + colorReset
	case http.MethodPatch, http.MethodPut:
		return colorYellow + method + colorReset
	case http.MethodDelete:
		return colorRed + method + colorReset
	default:
		return colorWhite + method + colorReset
	}
}

func colorStatus(code int) string {
	s := fmt.Sprintf("%d", code)
	switch {
	case code >= 500:
		return colorRed + s + colorReset
	case code >= 400:
		return colorYellow + s + colorReset
	case code >= 300:
		return colorCyan + s + colorReset
	case code >= 200:
		return colorGreen + s + colorReset
	default:
		return colorWhite + s + colorReset
	}
}

// Recovery returns middleware that recovers from panics raised by downstream
// handlers, logs the panic, and responds with 500 Internal Server Error.
func Recovery(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("panic",
						logger.Any("error", err),
						logger.Stack(),
					)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("Internal Server Error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// ContentTypeJSON returns middleware that enforces the Content-Type: application/json
// header on requests with a body.
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > 0 && r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte(`{"error":"Content-Type must be application/json"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Chain applies the given middleware functions to the handler in reverse order,
// so the first middleware in the list wraps the handler last and therefore runs first.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
