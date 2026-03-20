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
	"go.uber.org/zap"
)

// PublisherConfig holds the AMQP connection and exchange settings for a publisher.
type PublisherConfig struct {
	URL          string // amqp://user:pass@host:port/vhost
	Exchange     string
	ExchangeType string // defaults to "topic"
}

// Publisher wraps an AMQP connection and provides a thread-safe Publish method.
// It performs a single reconnect attempt on transient channel errors.
type Publisher struct {
	cfg    PublisherConfig
	logger *zap.Logger
	mu     sync.RWMutex
	conn   *amqp.Connection
	ch     *amqp.Channel
	closed bool
}

// NewPublisher dials the broker and declares the exchange.
// Returns an error if the initial connection fails.
func NewPublisher(ctx context.Context, cfg PublisherConfig, logger *zap.Logger) (*Publisher, error) {
	if cfg.ExchangeType == "" {
		cfg.ExchangeType = "topic"
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
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return fmt.Errorf("rabbitmq publisher: already closed")
	}

	if err := p.publishOnce(routingKey, body); err != nil {
		p.logger.Warn("rabbitmq: publish failed, reconnecting",
			zap.String("exchange", p.cfg.Exchange),
			zap.String("routing_key", routingKey),
			zap.Error(err),
		)
		if reconnErr := p.reconnect(); reconnErr != nil {
			return fmt.Errorf("rabbitmq publisher: reconnect failed: %w", reconnErr)
		}
		return p.publishOnce(routingKey, body)
	}
	return nil
}

func (p *Publisher) publishOnce(routingKey string, body []byte) error {
	p.mu.RLock()
	ch := p.ch
	exchange := p.cfg.Exchange
	p.mu.RUnlock()

	return ch.Publish(
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Body:         body,
		},
	)
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
	conn, err := amqp.Dial(p.cfg.URL)
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
	p.mu.Lock()
	p.conn = conn
	p.ch = ch
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
	p.mu.Unlock()
	return p.connect()
}
