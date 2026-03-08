package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// RecoveryMiddleware handles panic recovery
type RecoveryMiddleware struct {
	logger      *zap.Logger
	panicsTotal metric.Int64Counter
}

// NewRecoveryMiddleware creates a new recovery middleware
func NewRecoveryMiddleware(logger *zap.Logger) *RecoveryMiddleware {
	meter := otel.Meter(middlewareMeter)

	panicsTotal, _ := meter.Int64Counter(
		"http.server.panics.total",
		metric.WithDescription("Total HTTP handler panics recovered"),
		metric.WithUnit("{panic}"),
	)

	return &RecoveryMiddleware{
		logger:      logger,
		panicsTotal: panicsTotal,
	}
}

// Recovery returns a middleware that recovers from panics
func (m *RecoveryMiddleware) Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				ctx := c.Request.Context()

				// Mark the active span as failed so the panic shows up in traces.
				span := trace.SpanFromContext(ctx)
				span.SetStatus(codes.Error, "panic recovered")
				if err, ok := r.(error); ok {
					span.RecordError(err)
				}

				m.panicsTotal.Add(ctx, 1)

				m.logger.Error("panic recovered",
					zap.Any("error", r),
					zap.String("stack", string(debug.Stack())),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "INTERNAL_ERROR",
						"message": "internal server error",
					},
				})
			}
		}()
		c.Next()
	}
}
