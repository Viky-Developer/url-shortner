package queue

import (
	"context"
)

// NoopQueue implements both ClickPublisher and ClickConsumer by discarding
// publishes and immediately returning on consume.
type NoopQueue struct{}

// PublishClick discards the event and returns nil.
func (NoopQueue) PublishClick(context.Context, ClickEvent) error {
	return nil
}

// ConsumeClicks blocks until ctx is cancelled and returns ctx.Err().
func (NoopQueue) ConsumeClicks(ctx context.Context, _ func(context.Context, ClickEvent) error) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close is a no-op.
func (NoopQueue) Close() error {
	return nil
}
