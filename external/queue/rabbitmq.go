package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vicky/url-shortner/external/logger"
)

// RabbitMQConfig holds connection details for RabbitMQ.
type RabbitMQConfig struct {
	URL        string
	Exchange   string
	RoutingKey string
	QueueName  string
}

// RabbitMQClient provides publishing and consuming capabilities over RabbitMQ.
type RabbitMQClient struct {
	cfg    RabbitMQConfig
	conn   *amqp.Connection
	ch     *amqp.Channel
	log    logger.Logger
	mu     sync.Mutex
	closed bool
}

// NewRabbitMQClient connects to RabbitMQ, declares the exchange, declares the queue,
// binds them using the routing key, and returns a RabbitMQClient instance.
func NewRabbitMQClient(cfg RabbitMQConfig, log logger.Logger) (*RabbitMQClient, error) {
	if log == nil {
		log, _ = logger.New()
	}

	if cfg.Exchange == "" {
		cfg.Exchange = "url.clicks.direct"
	}
	if cfg.RoutingKey == "" {
		cfg.RoutingKey = "url.clicks.route"
	}

	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to open rabbitmq channel: %w", err)
	}

	// Declare durable direct exchange for routing click events.
	err = ch.ExchangeDeclare(
		cfg.Exchange, // exchange name
		"direct",     // exchange type
		true,         // durable
		false,        // auto-delete
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("failed to declare exchange %q: %w", cfg.Exchange, err)
	}

	// Declare durable queue for click events.
	_, err = ch.QueueDeclare(
		cfg.QueueName, // queue name
		true,          // durable
		false,         // auto-delete
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("failed to declare queue %q: %w", cfg.QueueName, err)
	}

	// Bind queue to exchange using the routing key.
	err = ch.QueueBind(
		cfg.QueueName,  // queue name
		cfg.RoutingKey, // routing key
		cfg.Exchange,   // exchange name
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("failed to bind queue %q to exchange %q with routing key %q: %w", cfg.QueueName, cfg.Exchange, cfg.RoutingKey, err)
	}

	return &RabbitMQClient{
		cfg:  cfg,
		conn: conn,
		ch:   ch,
		log:  log,
	}, nil
}

// PublishClick serializes the ClickEvent to JSON and publishes it to the configured exchange with routing key.
func (c *RabbitMQClient) PublishClick(ctx context.Context, event ClickEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("rabbitmq client is closed")
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal click event: %w", err)
	}

	return c.ch.PublishWithContext(
		ctx,
		c.cfg.Exchange,   // exchange
		c.cfg.RoutingKey, // routing key
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Timestamp:    event.Timestamp,
			Body:         body,
		},
	)
}

// ConsumeClicks starts consuming messages from the queue and calls handler for each event.
// It sets prefetch QoS, runs until ctx is cancelled, and manually ACKs/NACKs messages.
func (c *RabbitMQClient) ConsumeClicks(ctx context.Context, handler func(ctx context.Context, event ClickEvent) error) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("rabbitmq client is closed")
	}

	// Fair dispatch: prefetch up to 10 messages before expecting ACKs.
	if err := c.ch.Qos(10, 0, false); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("failed to set rabbitmq qos: %w", err)
	}

	msgs, err := c.ch.Consume(
		c.cfg.QueueName,
		"",    // consumer tag (auto-generated)
		false, // auto-ack = false (manual ack after DB write)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to start rabbitmq consumer: %w", err)
	}

	c.log.Info("rabbitmq click consumer started",
		logger.String("exchange", c.cfg.Exchange),
		logger.String("routingKey", c.cfg.RoutingKey),
		logger.String("queue", c.cfg.QueueName),
	)

	for {
		select {
		case <-ctx.Done():
			c.log.Info("rabbitmq click consumer shutting down", logger.String("queue", c.cfg.QueueName))
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				c.log.Warn("rabbitmq deliveries channel closed")
				return nil
			}

			var event ClickEvent
			if err := json.Unmarshal(d.Body, &event); err != nil {
				c.log.Error("failed to unmarshal click event from queue", logger.Error(err))
				// Reject malformed message without requeue to avoid poison message loop.
				_ = d.Nack(false, false)
				continue
			}

			// Process event in worker handler.
			handleCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := handler(handleCtx, event); err != nil {
				cancel()
				c.log.Error("failed to process click event, requeuing", logger.Error(err), logger.Int64("urlID", event.URLID))
				// Requeue for retry on transient database errors.
				_ = d.Nack(false, true)
				continue
			}
			cancel()

			_ = d.Ack(false)
		}
	}
}

// Close closes the RabbitMQ channel and connection.
func (c *RabbitMQClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	var chErr, connErr error
	if c.ch != nil {
		chErr = c.ch.Close()
	}
	if c.conn != nil {
		connErr = c.conn.Close()
	}

	if chErr != nil {
		return chErr
	}
	return connErr
}
