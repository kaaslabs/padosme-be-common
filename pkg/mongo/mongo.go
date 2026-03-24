// Package mongo provides a shared MongoDB client for PadosMe microservices.
// It wraps the official mongo-driver with retry/backoff and health-check support,
// following the same patterns as pkg/rabbitmq.
package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// Config holds MongoDB connection settings.
type Config struct {
	URL            string        // mongodb://user:pass@host:port
	DBName         string        // database name to use
	ConnectTimeout time.Duration // per-attempt timeout (default: 10s)
	MaxRetries     int           // max connection attempts (default: 10)
}

func (c *Config) applyDefaults() {
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 10
	}
}

// Client wraps a *mongo.Client and exposes the configured database.
type Client struct {
	cfg    Config
	client *mongo.Client
	db     *mongo.Database
}

// Connect dials MongoDB with exponential backoff up to cfg.MaxRetries attempts.
// Returns an error only after all retries are exhausted or ctx is cancelled.
func Connect(ctx context.Context, cfg Config, logger *zap.Logger) (*Client, error) {
	cfg.applyDefaults()

	wait := time.Second
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
		client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(cfg.URL))
		if err == nil {
			err = client.Ping(connectCtx, nil)
		}
		cancel()

		if err == nil {
			logger.Sugar().Infow("mongodb connected", "db", cfg.DBName)
			return &Client{
				cfg:    cfg,
				client: client,
				db:     client.Database(cfg.DBName),
			}, nil
		}

		lastErr = err
		if attempt == cfg.MaxRetries {
			break
		}

		logger.Sugar().Warnw("mongodb not ready, retrying",
			"attempt", attempt,
			"max", cfg.MaxRetries,
			"retry_in", wait,
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("mongo connect: context cancelled: %w", ctx.Err())
		case <-time.After(wait):
		}

		if wait < 30*time.Second {
			wait *= 2
		}
	}

	return nil, fmt.Errorf("mongo connect: exhausted %d attempts: %w", cfg.MaxRetries, lastErr)
}

// Database returns the configured *mongo.Database for use by repositories.
func (c *Client) Database() *mongo.Database {
	return c.db
}

// MongoClient returns the underlying *mongo.Client (e.g. for health-check pings).
func (c *Client) MongoClient() *mongo.Client {
	return c.client
}

// IsConnected reports whether the MongoDB connection is currently live.
func (c *Client) IsConnected(ctx context.Context) bool {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.Ping(pingCtx, nil) == nil
}

// Disconnect closes the connection gracefully.
func (c *Client) Disconnect(ctx context.Context) error {
	if err := c.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("mongo disconnect: %w", err)
	}
	return nil
}
