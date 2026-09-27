// Package queue defines the message queue interfaces and event contracts
// used for asynchronous processing across the application.
package queue

import (
	"context"
	"net"
	"time"

	"github.com/vicky/url-shortner/internal/payload"
)

// ClickEvent carries the metadata captured during a short URL redirect.
// It is published asynchronously to a queue so the redirect response
// does not block on database writes.
type ClickEvent struct {
	URLID     int64     `json:"url_id"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Referrer  string    `json:"referrer"`
	Timestamp time.Time `json:"timestamp"`
}

// ToClickInfo converts a ClickEvent into the payload.ClickInfo struct
// consumed by the service database logger.
func (e ClickEvent) ToClickInfo() payload.ClickInfo {
	return payload.ClickInfo{
		IP:        net.ParseIP(e.IP),
		UserAgent: e.UserAgent,
		Referrer:  e.Referrer,
	}
}

// ClickPublisher defines the contract for producing click events onto the queue.
type ClickPublisher interface {
	PublishClick(ctx context.Context, event ClickEvent) error
	Close() error
}

// ClickConsumer defines the contract for consuming click events from the queue.
type ClickConsumer interface {
	ConsumeClicks(ctx context.Context, handler func(ctx context.Context, event ClickEvent) error) error
	Close() error
}
