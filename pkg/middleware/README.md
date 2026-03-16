# pkg/middleware

## Purpose
Gin HTTP middleware shared across all PadosMe backend services.

## Files
| File | Description |
|------|-------------|
| `logger.go` | Structured request logging + OpenTelemetry HTTP metrics |
| `recovery.go` | Panic recovery — returns 500 instead of crashing |
| `requestid.go` | X-Request-ID propagation — generates UUID if missing |
| `serviceauth.go` | X-Service-Token validation for internal service-to-service calls |
| `middleware_test.go` | Unit tests for requestid and serviceauth middleware |

## Usage
```go
import "github.com/kaaslabs/padosme-be-common/pkg/middleware"

r := gin.New()
r.Use(
    middleware.RequestID(),
    middleware.NewLoggerMiddleware(logger).Logger(),
    middleware.Recovery(logger),
)

// For internal services only (query-intelligence, search-engine, ai-service):
r.Use(middleware.ServiceAuth(cfg.ServiceToken))

// In a handler, read the request ID:
requestID := middleware.GetRequestID(c)
```

## Dependencies
- `github.com/gin-gonic/gin`
- `github.com/google/uuid`
- `go.uber.org/zap`
- `go.opentelemetry.io/otel`

## Running Tests
```bash
go test -v ./pkg/middleware/...
```
