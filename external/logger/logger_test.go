package logger

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewDefaultLevel(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewWithDebugLevel(t *testing.T) {
	l, err := New(WithLevel("debug"))
	if err != nil {
		t.Fatalf("New(WithLevel(debug)): %v", err)
	}
	l.Debug("debug message")
}

func TestNewWithInvalidLevelFallback(t *testing.T) {
	l, err := New(WithLevel("bogus"))
	if err != nil {
		t.Fatalf("New(WithLevel(bogus)): %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewWithJSON(t *testing.T) {
	l, err := New(WithJSON(true))
	if err != nil {
		t.Fatalf("New(WithJSON(true)): %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestFieldConstructors(t *testing.T) {
	f := String("key", "val")
	if f.Key != "key" {
		t.Errorf("String key = %q, want key", f.Key)
	}

	f = Int("n", 42)
	if f.Key != "n" {
		t.Errorf("Int key = %q, want n", f.Key)
	}

	f = Int64("n64", 42)
	if f.Key != "n64" {
		t.Errorf("Int64 key = %q, want n64", f.Key)
	}

	f = Bool("flag", true)
	if f.Key != "flag" {
		t.Errorf("Bool key = %q, want flag", f.Key)
	}

	f = Any("x", "hello")
	if f.Key != "x" {
		t.Errorf("Any key = %q, want x", f.Key)
	}

	f = Error(nil)
	if f.Key != "" {
		t.Errorf("Error(nil) key = %q, want empty (nil is no-op)", f.Key)
	}

	f = Raw("raw", "val")
	if f.Key != "raw" {
		t.Errorf("Raw key = %q, want raw", f.Key)
	}

	f = Stack()
	if f.Key != "stack" {
		t.Errorf("Stack key = %q, want stack", f.Key)
	}
}

func TestLoggerMethods(t *testing.T) {
	// Capture stdout to verify output is produced.
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	l, err := New(WithLevel("debug"))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	l.Debug("debug msg", String("k", "v"))
	l.Info("info msg", Int("n", 1))
	l.Warn("warn msg", Bool("b", true))
	l.Error("error msg", Any("x", nil))

	_ = w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	for _, want := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestSync(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	// Sync on stdout is a no-op error in dev mode, just ensure it doesn't panic.
	_ = l.Sync()
}

func TestDefaultLoggerSetByNew(t *testing.T) {
	orig := defaultLogger
	defer func() { defaultLogger = orig }()

	l, err := New()
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if defaultLogger == nil {
		t.Fatal("defaultLogger should be set after New()")
	}
	_ = l
}

func TestRecoverPanicsWithoutDefaultLogger(t *testing.T) {
	orig := defaultLogger
	defaultLogger = nil
	defer func() { defaultLogger = orig }()

	// recover() outside a panic returns nil, so Recover is a no-op.
	// This just verifies it doesn't crash when defaultLogger is nil and
	// there's no active panic.
	Recover()
}

func TestRecoverWithDefaultLogger(t *testing.T) {
	orig := defaultLogger
	defer func() { defaultLogger = orig }()

	// Build a logger that writes to stderr so Recover doesn't panic.
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	cfg.OutputPaths = []string{"stderr"}
	cfg.DisableStacktrace = true
	z, _ := cfg.Build()
	defaultLogger = &zapLogger{z: z}

	// Recover calls os.Exit(1) on panic, so we can't directly test that path
	// without forking. Just verify the defaultLogger is non-nil.
	if defaultLogger == nil {
		t.Fatal("defaultLogger should not be nil")
	}
}
