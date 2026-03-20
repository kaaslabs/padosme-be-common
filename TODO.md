# TODO - padosme-be-common

All items completed on 2026-03-20 — released as v2.0.0.

## Critical

### 1. ✅ Config Client Library
- `pkg/config/client.go`
- `ConfigSource` interface for pluggable backends
- `Watcher` struct with `Start(ctx)` (supervisor-compatible), `Get/GetInt/GetBool/GetString`
- `HTTPSource` polls a JSON config service endpoint (`?key=k1&key=k2`)
- `StaticSource` for tests and bootstrap
- `Watcher.SubscribeRabbitMQ(amqpURL, queue)` for real-time push updates via `config.events`
- Thread-safe with `sync.RWMutex`

### 2. ✅ Service Auth Middleware Enhancement
- `pkg/middleware/serviceauth.go`
- `ServiceAuth(expectedToken)` — existing static X-Service-Token check (backward-compatible)
- `ServiceAuthJWT(secret)` — JWT Bearer validation for service-to-service calls
- `RequireRole(roles...)` — user_type enforcement, delegates to `pkg/auth.RequireUserType`

### 3. ✅ JWT Utilities
- `pkg/auth/jwt.go`
- `Claims` struct: `user_id`, `public_user_id`, `device_id`, `phone`, `user_type`, `lang`
- `ParseToken(tokenString, secret) (*Claims, error)` with `ErrTokenExpired` / `ErrTokenInvalid`
- `RequireAuth(secret)` Gin middleware — validates Bearer token, sets context keys
- `RequireUserType(types...)` Gin middleware — enforces allowed user types (403 on failure)
- `GetClaims(c)`, `GetUserID(c)` context helpers
- Constants: `UserTypeUser`, `UserTypeSeller`, `UserTypeAdmin`

---

## Important

### 4. ✅ RabbitMQ Client Library
- `pkg/rabbitmq/publisher.go` — `Publisher` with auto-reconnect on transient channel errors
- `pkg/rabbitmq/consumer.go` — `Consumer` with DLX support, manual ACK, supervisor-compatible `Run(ctx)`
- `ErrRequeue` sentinel for nack+requeue vs nack+discard (DLX routing)
- `ConsumerConfig.DLXExchange` for dead-letter routing on permanent handler failures

### 5. ✅ Health Check Framework
- `pkg/health/handler.go`
- `NewHandler(version)` with `Register(name, ProbeFunc, critical)`
- `HealthHandler()` — always HTTP 200, body contains full probe status (healthy/degraded/unhealthy)
- `ReadyHandler()` — HTTP 200 if no critical probe fails, HTTP 503 otherwise (k8s readiness)
- `LiveHandler()` — always HTTP 200 (k8s liveness)
- Standardized `HealthResponse{status, version, timestamp, probes}` JSON
