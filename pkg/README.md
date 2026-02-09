# Padosme Common Packages

This directory contains shared Go packages used across all Padosme microservices.

## Available Packages

### 1. `telemetry` - OpenTelemetry Integration

Provides unified telemetry setup (traces, metrics, logs) for all services.

**Features:**
- OpenTelemetry traces, metrics, and logs
- Zap logger integration
- Configurable OTLP endpoint
- Service name parameterization

**Usage:**

```go
import "github.com/kaaslabs/padosme-be-common/pkg/telemetry"

// In main.go
telemetryCfg := telemetry.DefaultConfig("my-service-name")
telemetryCfg.Version = "1.0.0"

tel, err := telemetry.New(ctx, telemetryCfg)
if err != nil {
    log.Fatal(err)
}
defer tel.Shutdown(ctx)

logger := tel.Logger
tracer := tel.Tracer
```

**Key Changes from Service-Specific Versions:**
- `DefaultConfig()` now requires a `serviceName` parameter
- No hardcoded service name constant

### 2. `worker` - Supervisor Pattern

Provides a production-ready supervisor for managing background workers with panic recovery and automatic restart.

**Features:**
- Automatic panic recovery
- Configurable restart logic
- Interval-based worker execution
- Graceful shutdown
- Context-aware cancellation
- Per-worker restart limits

**Usage:**

```go
import "github.com/kaaslabs/padosme-be-common/pkg/worker"

// Create supervisor
config := worker.SupervisorConfig{
    MaxRestarts: 5,
    RestartWait: 5 * time.Second,
}
supervisor := worker.NewSupervisor(logger, config)

// Add workers
supervisor.AddWorker(worker.Worker{
    Name: "cleanup-worker",
    Fn: cleanupExpiredSessions,
    Interval: 1 * time.Hour,
})

// Start and manage lifecycle
supervisor.Start(ctx)
defer supervisor.Stop()
```

**Key Features:**
- Workers with `Interval > 0` run periodically
- Workers with `Interval = 0` run once
- Automatic restart on panic/error (up to MaxRestarts)
- Comprehensive test coverage included

### 3. `middleware` - HTTP Middleware

Provides common Gin middleware for logging and panic recovery.

**Available Middleware:**
- **LoggerMiddleware**: Request/response logging with zap
- **RecoveryMiddleware**: Panic recovery with stack traces

**Usage:**

```go
import "github.com/kaaslabs/padosme-be-common/pkg/middleware"

// Create middleware instances
loggerMW := middleware.NewLoggerMiddleware(logger)
recoveryMW := middleware.NewRecoveryMiddleware(logger)

// Use in router
router.Use(recoveryMW.Recovery())
router.Use(loggerMW.Logger())
```

**Key Changes from Service-Specific Versions:**
- Generic error response format (no service-specific models)
- Uses `user_id` context key (standardized across services)

## Integration Guide

### Prerequisites

1. Ensure `padosme-be-common` is accessible as a local module or published to a Git repository

### For Local Development

Add to each service's `go.mod`:

```go
replace github.com/kaaslabs/padosme-be-common => ../padosme-be-common
```

Then:

```bash
go mod tidy
```

### Migration Steps

For each service (auth, user-profile, seller):

1. **Update imports:**
   ```diff
   - import "github.com/kaaslabs/padosme-auth-service/pkg/telemetry"
   + import "github.com/kaaslabs/padosme-be-common/pkg/telemetry"

   - import "github.com/kaaslabs/padosme-auth-service/internal/worker"
   + import "github.com/kaaslabs/padosme-be-common/pkg/worker"
   ```

2. **Update DefaultConfig call:**
   ```diff
   - telemetryCfg := telemetry.DefaultConfig()
   + telemetryCfg := telemetry.DefaultConfig("padosme-auth-service")
   ```

3. **Update middleware imports:**
   ```diff
   - import "github.com/kaaslabs/padosme-auth-service/internal/middleware"
   + import commonmw "github.com/kaaslabs/padosme-be-common/pkg/middleware"
   + import "github.com/kaaslabs/padosme-auth-service/internal/middleware" // for auth-specific middleware
   ```

4. **Supervisor migration (auth & seller services):**
   - The supervisor is 100% compatible, no code changes needed!
   - Just update the import path

   **User-profile service:**
   - Has a specialized supervisor tied to EventConsumer
   - Can keep service-specific version OR refactor to use generic supervisor + custom worker

5. **Run tests:**
   ```bash
   go test ./...
   ```

6. **Remove old packages:**
   ```bash
   rm -rf pkg/telemetry
   rm internal/middleware/logger.go
   rm internal/middleware/recovery.go
   rm internal/worker/supervisor.go  # for auth & seller
   # Keep service-specific middleware (auth.go, tracing.go, etc.)
   ```

## Benefits

✅ **Code Reuse**: Eliminates ~750 lines of duplicated code per service
  - ~250 lines: telemetry
  - ~150 lines: middleware
  - ~200 lines: supervisor
  - ~150 lines: tests

✅ **Consistency**: Ensures all services log, trace, and manage workers uniformly

✅ **Maintenance**: Update telemetry/middleware/worker logic once, all services benefit

✅ **Testing**: Shared packages can be tested independently with comprehensive test coverage

## Current Status

- ✅ Telemetry package created
- ✅ Logger middleware created
- ✅ Recovery middleware created
- ✅ Supervisor/worker package created
- ✅ Comprehensive test coverage added
- ⏳ Service integration pending (optional for MVP)

## Next Steps

1. Test shared packages with one service (recommend starting with auth or seller for supervisor)
2. If successful, migrate remaining services
3. Consider adding more shared packages:
   - RabbitMQ publisher/consumer helpers
   - Common error types
   - Validation utilities
   - Database connection helpers

## Service Compatibility Matrix

| Package | Auth Service | User Profile | Seller Service | Notes |
|---------|-------------|--------------|----------------|-------|
| telemetry | ✅ 100% | ✅ 100% | ✅ 100% | Just change DefaultConfig() call |
| middleware/logger | ✅ 100% | ✅ 100% | ✅ 100% | Drop-in replacement |
| middleware/recovery | ✅ 100% | ✅ 100% | ✅ 100% | Drop-in replacement |
| worker/supervisor | ✅ 100% | ⚠️ Custom | ✅ 100% | User-profile has specialized supervisor |
