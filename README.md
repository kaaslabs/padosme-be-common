# padosme-be-common

Common backend infrastructure, shared Go packages, and operational tooling for all Padosme microservices.

## Contents

1. **Platform Architecture** -- Full microservice ecosystem overview
2. **Search Architecture** -- Real-time search flow and indexing pipeline
3. **Infrastructure Services** -- Docker Compose setup for shared services
4. **Database Setup** -- PostgreSQL and MongoDB databases for all services
5. **RabbitMQ Topology** -- Exchange, queue, and binding declarations
6. **Network Configuration** -- Docker networking for inter-service communication
7. **Shared Go Packages** -- Reusable code packages in `pkg/`
8. **Supervisor Pattern** -- Background worker management
9. **Shared Models** -- Common data structures and constants
10. **Cloud Deployment** -- CI/CD and production deployment

---

## Platform Architecture

The Padosme backend is composed of 18 microservices organized by domain. All services are written in Go (except the API gateway, which is Erlang), communicate via RabbitMQ events, and share PostgreSQL, Redis, and the observability stack managed by this repository.

### Identity and Auth

| Service | Description |
|---------|-------------|
| **padosme-auth-service** | JWT authentication, OTP generation, device management |
| **padosme-user-profile-service** | User profiles, KYC verification |
| **padosme-seller-service** | Seller onboarding, verification, business cards |

### Commerce

| Service | Description |
|---------|-------------|
| **padosme-catalogue-service** | Product/service catalog management (MongoDB) |
| **padosme-subscription-service** | Credit wallet, subscription packages |
| **padosme-payment-service** | Stripe/Razorpay payment processing |
| **padosme-wallet-service** | Wallet credits, debit/credit ledger |
| **padosme-coupon-service** | Sales campaigns, coupon lifecycle, salesman tracking |

### Search and Discovery

| Service | Port | Description |
|---------|------|-------------|
| **padosme-search-api** | 8090 | Public search gateway, REST + WebSocket |
| **padosme-query-intelligence** | 8091 | Intent resolution via ontology |
| **padosme-search-engine** | 8092 | RedisSearch geo-spatial lookup + H3 ranking |
| **padosme-indexing-service** | -- | Real-time + nightly batch index builder |
| **padosme-discovery-service** | -- | Location management |

### Communication

| Service | Description |
|---------|-------------|
| **padosme-notification-service** | SMS (Twilio) + Push (APNs/FCM) |
| **padosme-channel-service** | Seller channels, posts, subscriptions |
| **padosme-config-service** | Centralized config + feature flags |

### Analytics and Integration

| Service | Description |
|---------|-------------|
| **padosme-analytics-service** | Event aggregation, dashboards |
| **ledgers-cloud-connect-service** | Zoho Books integration |

---

## Search Architecture

### Real-Time Search Flow

```
Mobile App (Search Tab)
    |
    | WebSocket (JWT auth via ?token=)
    v
padosme-search-api (:8090)
    |
    +-- POST /resolve --> padosme-query-intelligence (:8091)
    |       Cache (Redis) --> Ontology match --> Keyword fallback
    |       Returns: IntentResult {intent, category, product, filters, confidence}
    |
    +-- POST /search --> padosme-search-engine (:8092)
            H3 cell computation --> FT.SEARCH idx:sellers --> Rank
            Returns: Scored seller list
```

### Real-Time Index Pipeline

```
seller-service -------> padosme.events -------> padosme-indexing-service --> Redis (idx:sellers)
catalogue-service ----> catalog.events ------>          ^
location-service -----> location.events ----->          ^
rating-service -------> rating.events ------->          ^
                                                        |
Nightly reconciliation (21:30-04:30): full rebuild from upstream APIs
```

The indexing service maintains four independent consumer queues (one per upstream domain) so that a failure in one domain does not block indexing from others. Each queue has its own DLX with a dedicated dead-letter routing key.

---

## Infrastructure Services

This repository contains the shared Docker Compose configuration for all backend infrastructure:

- **PostgreSQL** (port 5432) -- Primary database with multiple databases for each service
- **pgAdmin4** (port 8000) -- PostgreSQL database management UI
- **Redis** (port 6379) -- Caching, rate limiting, and RedisSearch indexes
- **RedisInsight** (port 5540) -- Redis management and monitoring UI
- **RabbitMQ** (ports 5672, 15672) -- Message broker for inter-service communication
- **Jaeger** (port 16686) -- Distributed tracing
- **OpenTelemetry Collector** (ports 4317, 4318) -- Telemetry data collection and routing
- **Prometheus** (port 9090) -- Metrics storage and querying
- **Grafana** (port 3000) -- Metrics visualization and dashboards

### Quick Start

```bash
# 1. Start infrastructure
cd padosme-be-common
docker-compose up -d

# 2. Verify services
docker-compose ps
docker exec -it padosme-postgres psql -U postgres -c "\l"

# 3. Set up RabbitMQ topology
./scripts/setup-rabbitmq.sh

# 4. Set up databases and run migrations
./scripts/setup-databases.sh
```

### Management UIs

**Database and Cache:**
- **pgAdmin4**: http://localhost:8000 (admin@padosme.local/admin)
- **RedisInsight**: http://localhost:5540

**Observability:**
- **Grafana**: http://localhost:3000 (admin/Kaas-Labs)
- **Prometheus**: http://localhost:9090
- **Jaeger**: http://localhost:16686

**Messaging:**
- **RabbitMQ Management**: http://localhost:15672 (deploy/Kaas-Labs)

See [Observability guide](./docs/OBSERVABILITY.md) for detailed documentation on instrumenting services and creating dashboards.

### Run Individual Microservices

Each microservice has its own docker-compose that connects to the shared network:

```bash
cd ../padosme-auth-service && docker-compose up -d
cd ../padosme-seller-service && docker-compose up -d
cd ../padosme-search-api && docker-compose up -d
```

### Stopping Infrastructure

```bash
docker-compose down

# To also remove volumes (data):
docker-compose down -v
```

---

## Database Setup

The PostgreSQL container hosts 13 databases (11 service databases + postgres + template). One service (catalogue) uses MongoDB, and one (mobile-sms-service) has no database.

### Service-to-Database Mapping

| Service | Database | Engine |
|---------|----------|--------|
| padosme-auth-service | `padosme_auth` | PostgreSQL |
| padosme-user-profile-service | `padosme_profiles` | PostgreSQL |
| padosme-seller-service | `padosme_sellers` | PostgreSQL |
| padosme-notification-service | `padosme_notifications` | PostgreSQL |
| padosme-rating-service | `rating_db` | PostgreSQL |
| padosme-channel-service | `channel_service` | PostgreSQL |
| padosme-coupon-service | `padosme_coupon` | PostgreSQL |
| padosme-wallet-service | `wallet_service` | PostgreSQL |
| padosme-subscription-service | `subscription_service` | PostgreSQL |
| padosme-payment-service | `payment_service` | PostgreSQL |
| padosme-analytics-service | `analytics_db` | PostgreSQL |
| padosme-discovery-service | `discovery` | PostgreSQL |
| ledgers-cloud-connect-service | `padosme_ledgers` | PostgreSQL |
| padosme-catalogue-service | `catalog_db` | MongoDB |
| mobile-sms-service | -- | None |

### Running Migrations

The `scripts/setup-databases.sh` script is the single source of truth for database creation and migration. It creates all databases idempotently and runs migrations in dependency order:

- **Layer 0**: auth-service (no upstream dependency)
- **Layer 1**: user-profile-service (depends on auth events)
- **Layer 2**: seller-service (depends on profile events)
- **Layer 3**: all remaining services (no inter-dependencies)

```bash
# Full setup (create databases + run all migrations)
./scripts/setup-databases.sh

# Dry run (preview what would happen)
./scripts/setup-databases.sh --dry-run

# Create databases only (skip migrations)
./scripts/setup-databases.sh --skip-migrations

# Force re-run all migrations
./scripts/setup-databases.sh --force-migrations
```

Applied migrations are tracked in a `_applied_migrations` table within each database to ensure idempotency.

---

## RabbitMQ Topology

The file `scripts/setup-rabbitmq.sh` is the **single source of truth** for the entire Padosme RabbitMQ topology. Run it after every RabbitMQ restart, fresh deploy, or cluster rebuild.

```bash
# Apply full topology
./scripts/setup-rabbitmq.sh

# Preview without making changes
./scripts/setup-rabbitmq.sh --dry-run
```

### Exchange-to-Service Flow Map

All business-domain exchanges are `topic` type and durable.

```
Producer                    Exchange               Consumers
---------                   --------               ---------
auth-service             -> otp.events          -> mobile-sms-service
auth-service             -> user.events         -> user-profile-service, rating-service
user-profile-service     -> profile.events      -> seller-service
seller-service           -> padosme.events      -> notification-service, indexing-service,
                                                   auth-service, user-profile-service,
                                                   coupon-service
seller-service           -> seller.events       -> rating-service, analytics-service,
                                                   channel-service
seller-service           -> location.events     -> indexing-service
catalogue-service        -> catalog.events      -> rating-service, analytics-service,
                                                   indexing-service
channel-service          -> padosme.channel     -> channel.events (bridge) -> analytics-service
rating-service           -> rating.events       -> analytics-service, indexing-service
wallet-service           -> wallet.events       -> analytics-service
coupon-service           -> coupon.events       -> notification-service, wallet-service,
                                                   subscription-service, analytics-service
subscription-service     -> subscription.events -> coupon-service, wallet-service,
                                                   analytics-service
discovery-service        -> discovery.events    -> analytics-service
subscription-service     -> zoho.exchange       -> ledgers-cloud-connect-service
payment-service          -> payment.events      -> wallet-service
```

### Dead Letter and Retry Strategy

Each service has a dedicated DLX exchange and DLQ. The script handles three DLX patterns:

- **Fanout DLX** (most services): all nacked messages go to a single DLQ per service.
- **Direct DLX** (indexing-service, analytics-service): routing-key-based selection sends failures to the correct per-queue DLQ.
- **Retry with TTL** (notification-service): nacked messages go to a retry queue with per-message TTL, then dead-letter back to the main exchange for redelivery.

### Auto-Migration on PRECONDITION_FAILED

When an exchange or queue declaration returns HTTP 400 (e.g., type changed or queue args changed), the script:
1. Checks if the queue has messages (queues only).
2. If empty (or an exchange), deletes and recreates the resource.
3. If the queue has messages, it refuses to delete and reports an error to prevent data loss.

---

## Network Configuration

All services connect to the `padosme-network` bridge network. Individual microservices should declare:

```yaml
networks:
  padosme-network:
    external: true
```

### Environment Variables

| Service | Host | Port | Default Credentials |
|---------|------|------|---------------------|
| PostgreSQL | padosme-postgres | 5432 | deploy/Kaas-Labs |
| pgAdmin4 | localhost | 8000 | cto@kaaslabs.com/Kaas-Labs |
| Redis | padosme-redis | 6379 | -- |
| RedisInsight | localhost | 5540 | -- |
| RabbitMQ | padosme-rabbitmq | 5672 | deploy/Kaas-Labs |
| RabbitMQ Management | padosme-rabbitmq | 15672 | deploy/Kaas-Labs |
| Jaeger | padosme-jaeger | 16686 | -- |
| OTLP Collector gRPC | padosme-otel-collector | 4317 | -- |
| OTLP Collector HTTP | padosme-otel-collector | 4318 | -- |
| Prometheus | padosme-prometheus | 9090 | -- |
| Grafana | padosme-grafana | 3000 | admin/Kaas-Labs |

---

## Shared Go Packages

All packages live under `pkg/` and are imported as:

```go
import "github.com/kaaslabs/padosme-be-common/v3/pkg/<package>"
```

For local development, add to each service's `go.mod`:

```go
replace github.com/kaaslabs/padosme-be-common/v3 => ../padosme-be-common
```

### `pkg/telemetry` -- OpenTelemetry Integration

Unified telemetry setup providing traces, metrics, and structured logging via Zap.

```go
tel, err := telemetry.New(ctx, telemetry.DefaultConfig("my-service"))
defer tel.Shutdown(ctx)
logger := tel.Logger
tracer := tel.Tracer
```

### `pkg/worker` -- Supervisor Pattern

Production-ready supervisor for background workers with panic recovery, configurable restart limits, and interval-based execution. See the [Supervisor Pattern](#supervisor-pattern-usage) section below.

### `pkg/middleware` -- HTTP Middleware

Gin middleware for cross-cutting concerns:

- **`RequestID()`** -- Ensures every request has a unique `X-Request-ID` header (reuses client-provided ID or generates a UUID).
- **`ServiceAuth(token)`** -- Validates a static `X-Service-Token` header for service-to-service calls.
- **`ServiceAuthJWT(secret)`** -- Validates a Bearer JWT and populates `user_id`, `user_type`, `lang`, and `claims` into the Gin context.
- **`RequireRole(roles...)`** -- Restricts access to specific user types (chain after JWT auth).
- **`NewLoggerMiddleware(logger)`** -- Request/response logging with Zap.
- **`NewRecoveryMiddleware(logger)`** -- Panic recovery with stack traces.

```go
router.Use(middleware.RequestID())
router.Use(middleware.ServiceAuthJWT(jwtSecret))
router.Use(loggerMW.Logger())
router.Use(recoveryMW.Recovery())
```

### `pkg/config` -- Configuration

Two components:

- **`LoadEnvFile(path)`** -- Reads a `.env` file and sets environment variables (dotenv semantics; existing env vars take precedence).
- **`Watcher`** -- Maintains a live, thread-safe copy of config values refreshed from a `ConfigSource` on a fixed interval. Optionally receives real-time push updates via RabbitMQ `config.events`. Compatible with `worker.Supervisor`.

```go
watcher := config.NewWatcher(source, keys, logger, config.WatcherOptions{...})
supervisor.AddWorker(worker.Worker{Name: "config-watcher", Fn: watcher.Start})
value := watcher.Get("feature_flag_x")
```

### `pkg/models` -- Shared Data Models

Common request/response types and the `IntentResult` struct used across the search pipeline. See the [Shared Models](#shared-models) section below for details.

### `pkg/rabbitmq` -- RabbitMQ Publisher and Consumer

Thread-safe AMQP primitives designed for use with the worker Supervisor:

- **`Publisher`** -- Wraps an AMQP connection with auto-reconnect on transient channel errors. Thread-safe `Publish(ctx, routingKey, body)` method.
- **`Consumer`** -- Establishes a single AMQP consumer. `Run(ctx)` returns a non-nil error on connection loss so the Supervisor can restart it. Supports DLX declaration, QoS prefetch, and three ack modes:
  - Return `nil` to ack.
  - Return `ErrRequeue` to nack + requeue (transient errors).
  - Return any other error to nack + discard to DLX.

```go
pub, _ := rabbitmq.NewPublisher(ctx, rabbitmq.PublisherConfig{
    URL: "amqp://deploy:Kaas-Labs@padosme-rabbitmq:5672/",
    Exchange: "coupon.events",
}, logger)
pub.Publish(ctx, "coupon.created", payload)

consumer := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
    URL:         "amqp://deploy:Kaas-Labs@padosme-rabbitmq:5672/",
    Exchange:    "padosme.events",
    Queue:       "coupon.seller-events",
    RoutingKey:  "seller.verified",
    DLXExchange: "coupon-service.dlx",
}, handler, logger)
supervisor.AddWorker(worker.Worker{Name: "seller-consumer", Fn: consumer.Run})
```

### `pkg/errors` -- Structured Error Types

Standard `AppError` type with HTTP status codes, error codes, and request ID propagation. Predefined constructors: `InvalidRequest`, `Unauthorized`, `RateLimited`, `IntentFailed`, `SearchFailed`, `TranslateFailed`, `Timeout`, `Internal`.

```go
return errors.InvalidRequest("missing seller_id", requestID)
```

### `pkg/auth` -- JWT Authentication

JWT parsing and Gin middleware for Bearer token validation. Sets `user_id`, `user_type`, `lang`, and `claims` into the Gin context.

### `pkg/health` -- Health Check Handler

Standard health check endpoint handler for liveness/readiness probes.

### `pkg/mongo` -- MongoDB Client

MongoDB connection helper for services that use MongoDB (e.g., catalogue-service).

---

## Supervisor Pattern Usage

Use `pkg/worker` to run background jobs with:

- Panic recovery
- Configurable restarts (`MaxRestarts`, `RestartWait`)
- Interval workers (`Interval > 0`)
- One-time/long-running workers (`Interval == 0`)
- Graceful shutdown using context cancellation

### Quick Example

```go
logger, _ := zap.NewProduction()
defer logger.Sync()

supervisor := worker.NewSupervisor(logger, worker.SupervisorConfig{
    MaxRestarts: 5,
    RestartWait: 3 * time.Second,
})

supervisor.AddWorker(worker.Worker{
    Name: "session-cleanup",
    Interval: 1 * time.Hour,
    Fn: func(ctx context.Context) error {
        // cleanup logic
        return nil
    },
})

supervisor.Start(ctx)
defer supervisor.Stop()
```

### Full Client Example

A complete client-style example with signal handling, a long-running worker, and an interval worker is available at:

- [`examples/supervisor-client/main.go`](./examples/supervisor-client/main.go)

Run it with:

```bash
go run ./examples/supervisor-client
```

---

## Shared Models

### IntentResult

The core data structure produced by `padosme-query-intelligence` and consumed by `padosme-search-api` and `padosme-search-engine`:

```go
type IntentResult struct {
    Intent     string   `json:"intent"`
    Category   string   `json:"category"`
    Product    string   `json:"product"`
    Filters    []string `json:"filters"`
    Confidence float64  `json:"confidence"`
    Source     string   `json:"source"`
    Query      string   `json:"query"`
}
```

**Valid intent values**: `product_search`, `service_search`, `location_search`

**Valid source values**: `cache` (Redis hit), `ontology` (ontology match), `keyword` (fallback)

**Valid category values**: `restaurant`, `street_food`, `grocery`, `electronics`, `home_services`, `beauty_wellness`, `medical`, `automotive`, `clothing`, `education`, `finance`, `transport`, `petrol_station`, `atm`, `other`

### Response Types

```go
// Standard error response for all services.
type ErrorResponse struct {
    Error     string `json:"error"`
    Code      string `json:"code"`
    RequestID string `json:"request_id"`
}

// Standard success response wrapper.
type SuccessResponse struct {
    Data      interface{} `json:"data"`
    RequestID string      `json:"request_id"`
}
```

### Error Codes

Standard codes defined in `pkg/errors`: `INVALID_REQUEST`, `UNAUTHORIZED`, `RATE_LIMITED`, `INTENT_FAILED`, `SEARCH_FAILED`, `TRANSLATE_FAILED`, `TIMEOUT`, `INTERNAL_ERROR`.

---

## Cloud Deployment (CI/CD)

This repo includes GitHub Actions-based CI/CD for deploying the shared stack to a cloud VM over SSH.

- CI validation workflow: `.github/workflows/ci.yml`
- Deployment workflow: `.github/workflows/deploy-observability.yml`
- Cloud deployment runbook: [`docs/CICD_CLOUD_DEPLOYMENT.md`](./docs/CICD_CLOUD_DEPLOYMENT.md)
- Production compose override: `docker-compose.prod.yml`
- Cloud verification script: `scripts/cloud-verify-observability.sh`

For any service, set `OTEL_EXPORTER_OTLP_ENDPOINT` to your cloud collector (`<vm-ip>:4317`) and `OTEL_SERVICE_NAME` to the service name.
