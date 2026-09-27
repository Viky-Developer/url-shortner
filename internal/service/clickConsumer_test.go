package service_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/external/queue"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/service"
)

type mockClickConsumer struct {
	consumeFn func(ctx context.Context, handler func(ctx context.Context, event queue.ClickEvent) error) error
}

func (m *mockClickConsumer) ConsumeClicks(ctx context.Context, handler func(ctx context.Context, event queue.ClickEvent) error) error {
	if m.consumeFn != nil {
		return m.consumeFn(ctx, handler)
	}
	return nil
}

func (m *mockClickConsumer) Close() error {
	return nil
}

type mockClickRecorder struct {
	recordFn func(ctx context.Context, urlID int64, click payload.ClickInfo) error
	calls    []payload.ClickInfo
	urlIDs   []int64
}

func (m *mockClickRecorder) RecordClickTx(ctx context.Context, urlID int64, click payload.ClickInfo) error {
	m.calls = append(m.calls, click)
	m.urlIDs = append(m.urlIDs, urlID)
	if m.recordFn != nil {
		return m.recordFn(ctx, urlID, click)
	}
	return nil
}

func TestClickConsumerWorkerProcessesEvent(t *testing.T) {
	log, _ := logger.New()
	event := queue.ClickEvent{
		URLID:     42,
		IP:        "198.51.100.1",
		UserAgent: "Mozilla/5.0 Test",
		Referrer:  "https://google.com",
		Timestamp: time.Now().UTC(),
	}

	consumer := &mockClickConsumer{
		consumeFn: func(ctx context.Context, handler func(ctx context.Context, event queue.ClickEvent) error) error {
			return handler(ctx, event)
		},
	}

	recorder := &mockClickRecorder{}
	worker := service.NewClickConsumerWorker(consumer, recorder, log)

	worker.Start(context.Background())

	if len(recorder.calls) != 1 {
		t.Fatalf("expected 1 call to RecordClickTx, got %d", len(recorder.calls))
	}
	if recorder.urlIDs[0] != 42 {
		t.Errorf("urlID = %d, want 42", recorder.urlIDs[0])
	}
	if !recorder.calls[0].IP.Equal(net.ParseIP("198.51.100.1")) {
		t.Errorf("IP = %v, want 198.51.100.1", recorder.calls[0].IP)
	}
	if recorder.calls[0].UserAgent != "Mozilla/5.0 Test" {
		t.Errorf("UserAgent = %q, want 'Mozilla/5.0 Test'", recorder.calls[0].UserAgent)
	}
	if recorder.calls[0].Referrer != "https://google.com" {
		t.Errorf("Referrer = %q, want 'https://google.com'", recorder.calls[0].Referrer)
	}
}

func TestClickConsumerWorkerNilConsumer(t *testing.T) {
	log, _ := logger.New()
	recorder := &mockClickRecorder{}
	worker := service.NewClickConsumerWorker(nil, recorder, log)

	// Should return cleanly without panic
	worker.Start(context.Background())

	if len(recorder.calls) != 0 {
		t.Errorf("expected 0 calls when consumer is nil, got %d", len(recorder.calls))
	}
}

func TestClickConsumerWorkerRecorderError(t *testing.T) {
	log, _ := logger.New()
	event := queue.ClickEvent{
		URLID: 99,
		IP:    "127.0.0.1",
	}

	var handlerErr error
	consumer := &mockClickConsumer{
		consumeFn: func(ctx context.Context, handler func(ctx context.Context, event queue.ClickEvent) error) error {
			handlerErr = handler(ctx, event)
			return handlerErr
		},
	}

	expectedErr := errors.New("db failure")
	recorder := &mockClickRecorder{
		recordFn: func(ctx context.Context, urlID int64, click payload.ClickInfo) error {
			return expectedErr
		},
	}

	worker := service.NewClickConsumerWorker(consumer, recorder, log)
	worker.Start(context.Background())

	if !errors.Is(handlerErr, expectedErr) {
		t.Errorf("handler error = %v, want %v", handlerErr, expectedErr)
	}
}
