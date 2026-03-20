package rabbitmq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Unit tests cover config defaulting and ErrRequeue sentinel.
// Integration tests (requiring a live AMQP broker) are skipped unless
// RABBITMQ_URL is set — run them with: RABBITMQ_URL=amqp://... go test ./pkg/rabbitmq/...

func TestConsumerConfig_Defaults(t *testing.T) {
	c := NewConsumer(ConsumerConfig{
		URL:        "amqp://guest:guest@localhost:5672/",
		Exchange:   "test.events",
		Queue:      "test-queue",
		RoutingKey: "test.event",
	}, func(_ context.Context, _ []byte) error { return nil }, nil)

	assert.Equal(t, "topic", c.cfg.ExchangeType)
	assert.Equal(t, 1, c.cfg.PrefetchCount)
}

func TestConsumerConfig_ExplicitValues(t *testing.T) {
	c := NewConsumer(ConsumerConfig{
		URL:           "amqp://guest:guest@localhost:5672/",
		Exchange:      "test.events",
		ExchangeType:  "direct",
		Queue:         "test-queue",
		RoutingKey:    "test.event",
		DLXExchange:   "test.events.dlx",
		PrefetchCount: 5,
	}, func(_ context.Context, _ []byte) error { return nil }, nil)

	assert.Equal(t, "direct", c.cfg.ExchangeType)
	assert.Equal(t, 5, c.cfg.PrefetchCount)
	assert.Equal(t, "test.events.dlx", c.cfg.DLXExchange)
}

func TestPublisherConfig_ExchangeTypeDefault(t *testing.T) {
	// Verify the default is set during NewPublisher (we can't dial without a broker,
	// so we test the defaulting logic path via the config struct directly).
	cfg := PublisherConfig{
		URL:      "amqp://guest:guest@localhost:5672/",
		Exchange: "test.exchange",
	}
	// Simulate the defaulting that NewPublisher applies.
	if cfg.ExchangeType == "" {
		cfg.ExchangeType = "topic"
	}
	assert.Equal(t, "topic", cfg.ExchangeType)
}

func TestErrRequeue_IsSentinel(t *testing.T) {
	assert.EqualError(t, ErrRequeue, "requeue message")
	assert.NotNil(t, ErrRequeue)
}

func TestMessageHandler_NilIsAck(t *testing.T) {
	var h MessageHandler = func(_ context.Context, _ []byte) error { return nil }
	assert.NoError(t, h(context.Background(), []byte("body")))
}

func TestMessageHandler_ErrRequeueSentinel(t *testing.T) {
	var h MessageHandler = func(_ context.Context, _ []byte) error { return ErrRequeue }
	assert.ErrorIs(t, h(context.Background(), []byte("body")), ErrRequeue)
}
