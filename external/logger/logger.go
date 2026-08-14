package logger

import (
	"fmt"
	"os"
	"runtime/debug"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Field = zap.Field

func String(key, value string) Field { return zap.String(key, value) }

func Int(key string, value int) Field { return zap.Int(key, value) }

func Bool(key string, value bool) Field { return zap.Bool(key, value) }

func Any(key string, value any) Field { return zap.Any(key, value) }

func Error(err error) Field { return zap.Error(err) }

func Stack() Field { return zap.Stack("stack") }

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	Sync() error
}

type Option func(*options)

type options struct {
	level zapcore.Level
	json  bool
}

func WithLevel(level string) Option {
	return func(o *options) {
		if lv, err := zapcore.ParseLevel(level); err == nil {
			o.level = lv
		}
	}
}

func WithJSON(enable bool) Option {
	return func(o *options) {
		o.json = enable
	}
}

var defaultLogger Logger

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

type zapLogger struct {
	z *zap.Logger
}

func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.z.Debug(msg, fields...)
}

func (l *zapLogger) Info(msg string, fields ...Field) {
	l.z.Info(msg, fields...)
}

func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.z.Warn(msg, fields...)
}

func (l *zapLogger) Error(msg string, fields ...Field) {
	l.z.Error(msg, fields...)
}

func (l *zapLogger) Fatal(msg string, fields ...Field) {
	l.z.Fatal(msg, fields...)
}

func (l *zapLogger) Sync() error {
	return l.z.Sync()
}
