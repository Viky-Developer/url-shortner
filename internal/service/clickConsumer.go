package service

import (
	"context"
	"errors"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/external/queue"
	"github.com/vicky/url-shortner/internal/payload"
)

// ClickRecorder defines the interface for recording click events into the database.
type ClickRecorder interface {
	RecordClickTx(ctx context.Context, urlID int64, click payload.ClickInfo) error
}

// ClickConsumerWorker consumes ClickEvents from RabbitMQ and records them
// into the database in the background.
type ClickConsumerWorker struct {
	consumer queue.ClickConsumer
	recorder ClickRecorder
	log      logger.Logger
}

// NewClickConsumerWorker constructs a ClickConsumerWorker.
func NewClickConsumerWorker(consumer queue.ClickConsumer, recorder ClickRecorder, log logger.Logger) *ClickConsumerWorker {
	if log == nil {
		log, _ = logger.New()
	}
	return &ClickConsumerWorker{
		consumer: consumer,
		recorder: recorder,
		log:      log,
	}
}

// Start begins consuming messages from RabbitMQ until ctx is cancelled.
func (w *ClickConsumerWorker) Start(ctx context.Context) {
	if w.consumer == nil {
		w.log.Info("click consumer worker disabled: no consumer configured")
		return
	}

	w.log.Info("click consumer worker starting")
	err := w.consumer.ConsumeClicks(ctx, func(ctx context.Context, event queue.ClickEvent) error {
		if err := w.recorder.RecordClickTx(ctx, event.URLID, event.ToClickInfo()); err != nil {
			w.log.Error("failed to record click in worker", logger.Error(err), logger.Int64("urlID", event.URLID))
			return err
		}
		w.log.Info("click event consumed and recorded successfully",
			logger.Int64("urlID", event.URLID),
			logger.String("ip", event.IP),
			logger.String("referrer", event.Referrer),
		)
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		w.log.Error("click consumer worker exited with error", logger.Error(err))
	} else {
		w.log.Info("click consumer worker stopped")
	}
}
