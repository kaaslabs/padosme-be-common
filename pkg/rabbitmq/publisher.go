// Package rabbitmq provides shared RabbitMQ publisher and consumer primitives
// for PadosMe microservices. Both types are designed to work with the worker
// Supervisor: Publisher reconnects on failure, Consumer.Run() returns a non-nil
// error on connection loss so the Supervisor can restart it.
package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// PublisherConfig holds the AMQP connection and exchange settings for a publisher.
type PublisherConfig struct {
	URL          string // amqp://user:pass@host:port/vhost
	Exchange     string
	ExchangeType   string // defaults to "topic"
	ConnectionName string // shown in RabbitMQ Management UI; defaults to exchange name
}

// Publisher wraps an AMQP connection and provides a thread-safe Publish method.
// It performs a single reconnect attempt on transient channel errors.
type Publisher struct {
	cfg    PublisherConfig
	logger *zap.Logger
	mu     sync.RWMutex
	pubMu  sync.Mutex
	conn   *amqp.Connection
	ch     *amqp.Channel
	retCh  chan amqp.Return
	ackCh  chan amqp.Confirmation
	closed bool
}

// NewPublisher dials the broker and declares the exchange.
// Returns an error if the initial connection fails.
func NewPublisher(ctx context.Context, cfg PublisherConfig, logger *zap.Logger) (*Publisher, error) {
	if cfg.ExchangeType == "" {
		cfg.ExchangeType = "topic"
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	p := &Publisher{cfg: cfg, logger: logger}
	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("rabbitmq publisher: initial connect to %q: %w", cfg.Exchange, err)
	}
	return p, nil
}

// Publish sends body to the exchange with routingKey.
// Attempts one reconnect if the underlying channel is closed.
func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	p.pubMu.Lock()
	defer p.pubMu.Unlock()

	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return fmt.Errorf("rabbitmq publisher: already closed")
	}

	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, amqpHeaderCarrier(headers))

	if err := p.publishOnce(routingKey, body, headers); err != nil {
		p.logger.Warn("rabbitmq: publish failed, reconnecting",
			zap.String("exchange", p.cfg.Exchange),
			zap.String("routing_key", routingKey),
			zap.Error(err),
		)
		if reconnErr := p.reconnect(); reconnErr != nil {
			return fmt.Errorf("rabbitmq publisher: reconnect failed: %w", reconnErr)
		}
		return p.publishOnce(routingKey, body, headers)
	}
	return nil
}

func (p *Publisher) publishOnce(routingKey string, body []byte, headers amqp.Table) error {
	p.mu.RLock()
	ch := p.ch
	exchange := p.cfg.Exchange
	retCh := p.retCh
	ackCh := p.ackCh
	p.mu.RUnlock()

	if err := ch.Publish(
		exchange,
		routingKey,
		true,  // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Headers:      headers,
			Body:         body,
		},
	); err != nil {
		return err
	}

	// Publisher confirms ensure broker acceptance; NotifyReturn catches unroutable
	// mandatory messages.
	confirmTimeout := time.NewTimer(5 * time.Second)
	defer confirmTimeout.Stop()

	for {
		select {
		case ret, ok := <-retCh:
			if !ok {
				return fmt.Errorf("rabbitmq publisher: return channel closed")
			}
			return fmt.Errorf("rabbitmq publisher: unroutable message (exchange=%s, routing_key=%s, reply_code=%d, reply_text=%s)",
				ret.Exchange, ret.RoutingKey, ret.ReplyCode, ret.ReplyText)
		case conf, ok := <-ackCh:
			if !ok {
				return fmt.Errorf("rabbitmq publisher: confirm channel closed")
			}
			if !conf.Ack {
				return fmt.Errorf("rabbitmq publisher: broker nacked publish")
			}
			return nil
		case <-confirmTimeout.C:
			return fmt.Errorf("rabbitmq publisher: confirm timeout")
		}
	}
}

// IsConnected reports whether the AMQP connection is currently open.
func (p *Publisher) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conn != nil && !p.conn.IsClosed()
}

// Close shuts down the channel and connection gracefully.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *Publisher) connect() error {
	connName := p.cfg.ConnectionName
	if connName == "" {
		connName = p.cfg.Exchange + "-publisher"
	}
	conn, err := amqp.DialConfig(p.cfg.URL, amqp.Config{
		Properties: amqp.Table{
			"connection_name": connName,
		},
	})
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}
	if err := ch.ExchangeDeclare(
		p.cfg.Exchange,
		p.cfg.ExchangeType,
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return err
	}
	if err := ch.Confirm(false); err != nil {
		ch.Close()
		conn.Close()
		return err
	}
	retCh := ch.NotifyReturn(make(chan amqp.Return, 1))
	ackCh := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	p.mu.Lock()
	p.conn = conn
	p.ch = ch
	p.retCh = retCh
	p.ackCh = ackCh
	p.mu.Unlock()
	return nil
}

func (p *Publisher) reconnect() error {
	p.mu.Lock()
	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.ch = nil
	p.retCh = nil
	p.ackCh = nil
	p.mu.Unlock()
	return p.connect()
}
