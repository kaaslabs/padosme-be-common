package rabbitmq

import (
	"context"
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// ConsumerConfig configures a RabbitMQ consumer.
type ConsumerConfig struct {
	URL           string // amqp://user:pass@host:port/vhost
	Exchange      string
	ExchangeType  string // defaults to "topic"
	Queue         string
	RoutingKey    string
	DLXExchange   string // dead-letter exchange; leave empty to skip DLX
	PrefetchCount int    // QoS prefetch count; defaults to 1
}

// MessageHandler processes a single AMQP delivery body.
// Return nil to ack, ErrRequeue to nack+requeue, any other error to nack+discard (DLX).
type MessageHandler func(ctx context.Context, body []byte) error

// ErrRequeue signals that a message should be nacked and requeued for retry.
// Use this for transient errors (e.g. temporary DB unavailability).
var ErrRequeue = fmt.Errorf("requeue message")

// Consumer establishes a single AMQP consumer. Run() is compatible with the
// worker.Supervisor interface — it returns a non-nil error on connection loss
// so the Supervisor can restart it automatically.
type Consumer struct {
	cfg     ConsumerConfig
	handler MessageHandler
	logger  *zap.Logger
}

// NewConsumer creates a Consumer with the message handler bound at construction.
// Call Run(ctx) to begin consuming; pass it directly to worker.Supervisor.
func NewConsumer(cfg ConsumerConfig, handler MessageHandler, logger *zap.Logger) *Consumer {
	if cfg.ExchangeType == "" {
		cfg.ExchangeType = "topic"
	}
	if cfg.PrefetchCount == 0 {
		cfg.PrefetchCount = 1
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Consumer{cfg: cfg, handler: handler, logger: logger}
}

// Run connects, declares topology, and processes messages until ctx is cancelled
// or the connection is lost. Returns nil on clean shutdown, non-nil on failure
// (which triggers a Supervisor restart).
func (c *Consumer) Run(ctx context.Context) error {
	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("rabbitmq consumer: dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq consumer: open channel: %w", err)
	}
	defer ch.Close()

	if err := c.declareTopology(ch); err != nil {
		return err
	}

	if err := ch.Qos(c.cfg.PrefetchCount, 0, false); err != nil {
		return fmt.Errorf("rabbitmq consumer: set QoS: %w", err)
	}

	msgs, err := ch.Consume(c.cfg.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consumer: start consume on %q: %w", c.cfg.Queue, err)
	}

	connClose := conn.NotifyClose(make(chan *amqp.Error, 1))

	c.logger.Info("rabbitmq consumer started",
		zap.String("queue", c.cfg.Queue),
		zap.String("exchange", c.cfg.Exchange),
		zap.String("routing_key", c.cfg.RoutingKey),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case amqpErr, ok := <-connClose:
			if !ok {
				return fmt.Errorf("rabbitmq: connection closed unexpectedly")
			}
			return fmt.Errorf("rabbitmq: connection lost: %s", amqpErr.Reason)
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("rabbitmq: delivery channel closed")
			}
			c.handleDelivery(ctx, msg)
		}
	}
}

func (c *Consumer) declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		c.cfg.Exchange, c.cfg.ExchangeType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("rabbitmq consumer: declare exchange %q: %w", c.cfg.Exchange, err)
	}

	queueArgs := amqp.Table{}
	if c.cfg.DLXExchange != "" {
		if err := ch.ExchangeDeclare(
			c.cfg.DLXExchange, "topic",
			true, false, false, false, nil,
		); err != nil {
			return fmt.Errorf("rabbitmq consumer: declare DLX %q: %w", c.cfg.DLXExchange, err)
		}
		queueArgs["x-dead-letter-exchange"] = c.cfg.DLXExchange
	}

	if _, err := ch.QueueDeclare(
		c.cfg.Queue, true, false, false, false, queueArgs,
	); err != nil {
		return fmt.Errorf("rabbitmq consumer: declare queue %q: %w", c.cfg.Queue, err)
	}

	if err := ch.QueueBind(
		c.cfg.Queue, c.cfg.RoutingKey, c.cfg.Exchange, false, nil,
	); err != nil {
		return fmt.Errorf("rabbitmq consumer: bind queue %q → %q: %w",
			c.cfg.Queue, c.cfg.Exchange, err)
	}
	return nil
}

func (c *Consumer) handleDelivery(ctx context.Context, msg amqp.Delivery) {
	err := c.handler(ctx, msg.Body)
	switch {
	case err == nil:
		msg.Ack(false)
	case errors.Is(err, ErrRequeue):
		c.logger.Warn("rabbitmq: handler requested requeue",
			zap.String("queue", c.cfg.Queue),
		)
		msg.Nack(false, true)
	default:
		c.logger.Error("rabbitmq: handler failed, discarding to DLX",
			zap.String("queue", c.cfg.Queue),
			zap.Error(err),
		)
		msg.Nack(false, false)
	}
}
