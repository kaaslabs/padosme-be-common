package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const middlewareMeter = "padosme-be-common/middleware"

// LoggerMiddleware handles request logging
type LoggerMiddleware struct {
	logger          *zap.Logger
	requestsTotal   metric.Int64Counter
	requestDuration metric.Float64Histogram
}

// NewLoggerMiddleware creates a new logger middleware
func NewLoggerMiddleware(logger *zap.Logger) *LoggerMiddleware {
	meter := otel.Meter(middlewareMeter)

	// Ignore errors — a no-op instrument is returned on failure, which is safe.
	requestsTotal, _ := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Total HTTP requests received"),
		metric.WithUnit("{request}"),
	)

	requestDuration, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)

	return &LoggerMiddleware{
		logger:          logger,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
	}
}

// Logger returns a middleware that logs requests and records HTTP metrics
func (m *LoggerMiddleware) Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip logging for health checks to reduce noise.
		if path == "/health" {
			c.Next()
			return
		}

		start := time.Now()
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		ctx := c.Request.Context()

		// Record HTTP metrics with method/route/status labels.
		attrs := []attribute.KeyValue{
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.status_code", strconv.Itoa(status)),
		}
		m.requestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		m.requestDuration.Record(ctx, latency.Seconds(), metric.WithAttributes(attrs...))

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("body_size", c.Writer.Size()),
		}

		// Attach trace_id so logs can be correlated with traces in Grafana/Loki.
		if spanCtx := trace.SpanFromContext(ctx).SpanContext(); spanCtx.HasTraceID() {
			fields = append(fields, zap.String("trace_id", spanCtx.TraceID().String()))
		}

		if userID, exists := c.Get("user_id"); exists {
			fields = append(fields, zap.Any("user_id", userID))
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		switch {
		case status >= 500:
			m.logger.Error("server error", fields...)
		case status >= 400:
			m.logger.Warn("client error", fields...)
		default:
			m.logger.Info("request", fields...)
		}
	}
}
