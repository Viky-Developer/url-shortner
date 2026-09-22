package queue

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestClickEventSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	evt := ClickEvent{
		URLID:     42,
		IP:        "192.168.1.1",
		UserAgent: "Mozilla/5.0 TestAgent",
		Referrer:  "https://google.com",
		Timestamp: now,
	}

	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ClickEvent
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.URLID != 42 {
		t.Errorf("url_id = %d, want 42", decoded.URLID)
	}
	if decoded.IP != "192.168.1.1" {
		t.Errorf("ip = %q, want 192.168.1.1", decoded.IP)
	}
	if decoded.UserAgent != "Mozilla/5.0 TestAgent" {
		t.Errorf("user_agent = %q, want Mozilla/5.0 TestAgent", decoded.UserAgent)
	}
	if decoded.Referrer != "https://google.com" {
		t.Errorf("referrer = %q, want https://google.com", decoded.Referrer)
	}

	info := decoded.ToClickInfo()
	if !info.IP.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("info.IP = %v, want 192.168.1.1", info.IP)
	}
	if info.UserAgent != "Mozilla/5.0 TestAgent" {
		t.Errorf("info.UserAgent = %q", info.UserAgent)
	}
	if info.Referrer != "https://google.com" {
		t.Errorf("info.Referrer = %q", info.Referrer)
	}
}

func TestNoopQueue(t *testing.T) {
	noop := NoopQueue{}

	if err := noop.PublishClick(context.Background(), ClickEvent{URLID: 1}); err != nil {
		t.Fatalf("noop publish error: %v", err)
	}
	if err := noop.Close(); err != nil {
		t.Fatalf("noop close error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := noop.ConsumeClicks(ctx, func(context.Context, ClickEvent) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

func TestRabbitMQConfigFields(t *testing.T) {
	cfg := RabbitMQConfig{
		URL:        "amqp://guest:guest@localhost:5672/",
		Exchange:   "custom.exchange",
		RoutingKey: "custom.key",
		QueueName:  "custom.queue",
	}
	if cfg.Exchange != "custom.exchange" {
		t.Errorf("exchange = %q", cfg.Exchange)
	}
	if cfg.RoutingKey != "custom.key" {
		t.Errorf("routingKey = %q", cfg.RoutingKey)
	}
}

func TestNewRabbitMQClientConnectionFailure(t *testing.T) {
	cfg := RabbitMQConfig{
		URL:        "amqp://invalid-host:1234/",
		Exchange:   "url.clicks.direct",
		RoutingKey: "url.clicks.route",
		QueueName:  "url.clicks",
	}
	_, err := NewRabbitMQClient(cfg, nil)
	if err == nil {
		t.Fatal("expected connection error for invalid url")
	}
}
