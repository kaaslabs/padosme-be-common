package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// ConfigSource loads a set of key-value config entries from any backend.
// Implement this interface to plug in different backends (HTTP, DB, etc.).
type ConfigSource interface {
	Load(ctx context.Context, keys []string) (map[string]string, error)
}

// Watcher maintains a live, thread-safe copy of config values refreshed from a
// ConfigSource on a fixed interval. Optionally receives real-time push updates
// via RabbitMQ config.events. Use Start(ctx) as a worker.Supervisor Fn.
type Watcher struct {
	source   ConfigSource
	keys     []string
	interval time.Duration
	logger   *zap.Logger
	mu       sync.RWMutex
	values   map[string]string
}

// NewWatcher creates a Watcher. Call Start(ctx) to begin periodic refresh.
// Returns an error if interval is not positive.
func NewWatcher(source ConfigSource, keys []string, interval time.Duration, logger *zap.Logger) (*Watcher, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("config watcher: interval must be positive, got %s", interval)
	}
	return &Watcher{
		source:   source,
		keys:     keys,
		interval: interval,
		logger:   logger,
		values:   make(map[string]string),
	}, nil
}

// Start performs an immediate load, then refreshes on the configured interval.
// Returns nil when ctx is cancelled. Non-nil errors are rare (only on source
// failure with no previous values) but supervisor-compatible.
func (w *Watcher) Start(ctx context.Context) error {
	if err := w.reload(ctx); err != nil {
		w.logger.Warn("config watcher: initial load failed", zap.Error(err))
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.reload(ctx); err != nil {
				w.logger.Warn("config watcher: reload failed", zap.Error(err))
			}
		}
	}
}

// SubscribeRabbitMQ connects to RabbitMQ and listens on the config.events exchange
// for real-time config updates. Messages must be JSON: {"key":"k","value":"v"}.
// Blocks until ctx is cancelled or connection is lost. Supervisor-compatible.
func (w *Watcher) SubscribeRabbitMQ(amqpURL, queue string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		conn, err := amqp.DialConfig(amqpURL, amqp.Config{
			Properties: amqp.Table{
				"connection_name": "config-watcher-" + queue,
			},
		})
		if err != nil {
			return fmt.Errorf("config watcher: rabbitmq dial: %w", err)
		}
		defer conn.Close()

		ch, err := conn.Channel()
		if err != nil {
			return fmt.Errorf("config watcher: rabbitmq channel: %w", err)
		}
		defer ch.Close()

		if err := ch.ExchangeDeclare("config.events", "topic", true, false, false, false, nil); err != nil {
			return fmt.Errorf("config watcher: declare config.events exchange: %w", err)
		}
		if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
			return fmt.Errorf("config watcher: declare queue %q: %w", queue, err)
		}
		if err := ch.QueueBind(queue, "#", "config.events", false, nil); err != nil {
			return fmt.Errorf("config watcher: bind queue: %w", err)
		}

		msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
		if err != nil {
			return fmt.Errorf("config watcher: consume: %w", err)
		}

		connClose := conn.NotifyClose(make(chan *amqp.Error, 1))
		w.logger.Info("config watcher: subscribed to config.events via RabbitMQ")

		for {
			select {
			case <-ctx.Done():
				return nil
			case amqpErr, ok := <-connClose:
				if !ok {
					return fmt.Errorf("config watcher: rabbitmq connection closed")
				}
				return fmt.Errorf("config watcher: rabbitmq connection lost: %s", amqpErr.Reason)
			case msg, ok := <-msgs:
				if !ok {
					return fmt.Errorf("config watcher: delivery channel closed")
				}
				w.applyUpdate(msg.Body)
				msg.Ack(false)
			}
		}
	}
}

type configUpdateMessage struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (w *Watcher) applyUpdate(body []byte) {
	var update configUpdateMessage
	if err := json.Unmarshal(body, &update); err != nil {
		w.logger.Warn("config watcher: malformed update message", zap.Error(err))
		return
	}
	if update.Key == "" {
		return
	}
	w.mu.Lock()
	w.values[update.Key] = update.Value
	w.mu.Unlock()
	w.logger.Info("config watcher: applied real-time update",
		zap.String("key", update.Key),
		zap.String("value", update.Value),
	)
}

func (w *Watcher) reload(ctx context.Context) error {
	vals, err := w.source.Load(ctx, w.keys)
	if err != nil {
		return err
	}
	w.mu.Lock()
	// Remove keys that disappeared from the source.
	for _, k := range w.keys {
		if _, exists := vals[k]; !exists {
			delete(w.values, k)
		}
	}
	for k, v := range vals {
		w.values[k] = v
	}
	w.mu.Unlock()
	return nil
}

// Set directly writes a config value. Useful for overrides or test injection.
func (w *Watcher) Set(key, value string) {
	w.mu.Lock()
	w.values[key] = value
	w.mu.Unlock()
}

// Get returns the raw string value for key, or "" if absent.
func (w *Watcher) Get(key string) string {
	w.mu.RLock()
	v := w.values[key]
	w.mu.RUnlock()
	return v
}

// GetString returns the string value for key, or def if absent or empty.
func (w *Watcher) GetString(key, def string) string {
	w.mu.RLock()
	v, ok := w.values[key]
	w.mu.RUnlock()
	if !ok || v == "" {
		return def
	}
	return v
}

// GetInt parses the value as an integer, returning def on absence or parse error.
func (w *Watcher) GetInt(key string, def int) int {
	v := w.Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// GetBool parses the value as a boolean. Accepts "true", "1", "yes" (case-insensitive).
// Returns def on absence or unrecognised value.
func (w *Watcher) GetBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(w.Get(key)))
	if v == "" {
		return def
	}
	return v == "true" || v == "1" || v == "yes"
}

// --- HTTP Source ---

// HTTPSource polls a JSON HTTP endpoint that returns {"key":"value",...}.
// The endpoint receives requested keys as repeated ?key= query parameters.
type HTTPSource struct {
	baseURL string
	client  *http.Client
}

// NewHTTPSource creates an HTTPSource for the given config service URL.
// Example: NewHTTPSource("http://config-service:8080/config")
func NewHTTPSource(configServiceURL string) *HTTPSource {
	return &HTTPSource{
		baseURL: configServiceURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Load fetches the specified keys from the HTTP config service.
func (s *HTTPSource) Load(ctx context.Context, keys []string) (map[string]string, error) {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, fmt.Errorf("config http source: invalid base URL: %w", err)
	}
	q := u.Query()
	for _, k := range keys {
		q.Add("key", k)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("config http source: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("config http source: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("config http source: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("config http source: read body: %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("config http source: decode response: %w", err)
	}
	return result, nil
}

// --- Static Source (useful for testing) ---

// StaticSource implements ConfigSource with a fixed map. Useful in tests and
// for bootstrapping services before a config service is available.
type StaticSource struct {
	values map[string]string
}

// NewStaticSource creates a ConfigSource backed by the provided map.
func NewStaticSource(values map[string]string) *StaticSource {
	return &StaticSource{values: values}
}

func (s *StaticSource) Load(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := s.values[k]; ok {
			result[k] = v
		}
	}
	return result, nil
}
