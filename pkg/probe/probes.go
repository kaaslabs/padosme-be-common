package probe

import (
	"context"
	"database/sql"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RedisPinger is the interface required for Redis health probes.
// Satisfied by *redis.Client from github.com/redis/go-redis/v9 and similar clients.
type RedisPinger interface {
	Ping(ctx context.Context) error
}

// MongoChecker is the interface required for MongoDB health probes.
// Satisfied by wrapper types that expose connection status.
type MongoChecker interface {
	IsConnected(ctx context.Context) bool
}

// PostgresProbe creates a probe that pings a *sql.DB.
func PostgresProbe(name string, db *sql.DB, critical bool) ProbeConfig {
	return ProbeConfig{
		Name:     name,
		Critical: critical,
		Check: func(ctx context.Context) error {
			return db.PingContext(ctx)
		},
	}
}

// RedisProbe creates a probe that pings a Redis client.
// The client must satisfy the RedisPinger interface (e.g. a wrapper around
// *redis.Client whose Ping returns just an error).
func RedisProbe(name string, client RedisPinger, critical bool) ProbeConfig {
	return ProbeConfig{
		Name:     name,
		Critical: critical,
		Check: func(ctx context.Context) error {
			return client.Ping(ctx)
		},
	}
}

// RabbitMQProbe creates a probe that dials and immediately closes an AMQP connection.
// This avoids holding idle connections — each check is a fresh dial.
func RabbitMQProbe(name string, amqpURL string, critical bool) ProbeConfig {
	return ProbeConfig{
		Name:     name,
		Critical: critical,
		Check: func(ctx context.Context) error {
			conn, err := amqp.DialConfig(amqpURL, amqp.Config{
				Properties: amqp.Table{
					"connection_name": name + "-health-probe",
				},
			})
			if err != nil {
				return fmt.Errorf("amqp dial: %w", err)
			}
			return conn.Close()
		},
	}
}

// MongoProbe creates a probe for a MongoDB client.
// The client must satisfy the MongoChecker interface.
func MongoProbe(name string, client MongoChecker, critical bool) ProbeConfig {
	return ProbeConfig{
		Name:     name,
		Critical: critical,
		Check: func(ctx context.Context) error {
			if !client.IsConnected(ctx) {
				return fmt.Errorf("mongo: not connected")
			}
			return nil
		},
	}
}
