// Package logger provides a thin wrapper around zap that exposes a small
// Logger interface plus field constructors for structured logging.
package logger

import (
	"fmt"
	"os"
	"runtime/debug"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field is a single key-value pair appended to a log line.
type Field = zap.Field

// String returns a string field for structured logging.
func String(key, value string) Field { return zap.String(key, value) }

// Int returns an int field for structured logging.
func Int(key string, value int) Field { return zap.Int(key, value) }

// Int64 returns an int64 field for structured logging.
func Int64(key string, value int64) Field { return zap.Int64(key, value) }

// Bool returns a bool field for structured logging.
func Bool(key string, value bool) Field { return zap.Bool(key, value) }

// Any returns a loosely-typed field for structured logging.
func Any(key string, value any) Field { return zap.Any(key, value) }

// Error returns an error field for structured logging.
func Error(err error) Field { return zap.Error(err) }

// Stack returns a stack-trace field for structured logging.
func Stack() Field { return zap.Stack("stack") }

// Logger is the minimal logging contract used across the application.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	Sync() error
}

// Option configures a logger during construction.
type Option func(*options)

// options holds the logger configuration values.
type options struct {
	level zapcore.Level
	json  bool
}

// WithLevel returns an Option that sets the minimum log level. Invalid levels
// are ignored and the default info level is kept.
func WithLevel(level string) Option {
	return func(o *options) {
		if lv, err := zapcore.ParseLevel(level); err == nil {
			o.level = lv
		}
	}
}

// WithJSON returns an Option that selects JSON encoding for log output.
func WithJSON(enable bool) Option {
	return func(o *options) {
		o.json = enable
	}
}

// defaultLogger is the package-level logger used by Recover.
var defaultLogger Logger

// New builds a Logger from the given options, redirecting the standard log
// package to the resulting logger. The built logger is also stored as the
// package default used by Recover.
func New(opts ...Option) (Logger, error) {
	o := options{level: zapcore.InfoLevel}
	for _, opt := range opts {
		opt(&o)
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(o.level)
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	if o.json {
		cfg.Encoding = "json"
	} else {
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	z, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	zap.RedirectStdLog(z)
	defaultLogger = &zapLogger{z: z}

	return defaultLogger, nil
}

// Recover logs a recovered panic through the package default logger and exits
// with a non-zero status. If no logger has been created yet, it re-panics.
func Recover() {
	if r := recover(); r != nil {
		l := defaultLogger
		if l == nil {
			panic(r)
		}
		l.Error("panic recovered",
			Any("panic", r),
			String("stack", string(debug.Stack())),
		)
		os.Exit(1)
	}
}

// zapLogger adapts a *zap.Logger to the Logger interface.
type zapLogger struct {
	z *zap.Logger
}

// Debug logs a message at debug level.
func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.z.Debug(msg, fields...)
}

// Info logs a message at info level.
func (l *zapLogger) Info(msg string, fields ...Field) {
	l.z.Info(msg, fields...)
}

// Warn logs a message at warn level.
func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.z.Warn(msg, fields...)
}

// Error logs a message at error level.
func (l *zapLogger) Error(msg string, fields ...Field) {
	l.z.Error(msg, fields...)
}

// Fatal logs a message at fatal level and then calls os.Exit(1).
func (l *zapLogger) Fatal(msg string, fields ...Field) {
	l.z.Fatal(msg, fields...)
}

// Sync flushes any buffered log entries.
func (l *zapLogger) Sync() error {
	return l.z.Sync()
}
