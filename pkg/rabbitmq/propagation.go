package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
)

// amqpHeaderCarrier adapts amqp.Table to propagation.TextMapCarrier so W3C
// traceparent / tracestate headers travel with every message. Consumers that
// start their own span inside the handler will see the publisher's span as
// parent and the Jaeger UI will stitch the two services into one trace.
type amqpHeaderCarrier amqp.Table

var _ propagation.TextMapCarrier = amqpHeaderCarrier(nil)

func (c amqpHeaderCarrier) Get(key string) string {
	v, ok := c[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func (c amqpHeaderCarrier) Set(key, value string) {
	c[key] = value
}

func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
