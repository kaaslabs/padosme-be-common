# padosme-be-common

Common backend infrastructure and shared resources for all Padosme microservices.

## Contents

1. **Infrastructure Services** - Docker Compose setup for shared services (PostgreSQL, Redis, RabbitMQ, Jaeger)
2. **Shared Go Packages** - Reusable code packages (telemetry, middleware) in `pkg/`

## Infrastructure Services

This repository contains the shared Docker Compose configuration for all backend infrastructure:

- **PostgreSQL** (port 5432) - Primary database with multiple databases for each service
- **Redis** (port 6379) - Caching and rate limiting
- **RabbitMQ** (ports 5672, 15672) - Message broker for inter-service communication
- **Jaeger** (ports 16686, 4317, 4318) - Distributed tracing and observability

## Quick Start

### 1. Start Infrastructure

```bash
cd padosme-be-common
docker-compose up -d
```

### 2. Verify Services

```bash
# Check all services are running
docker-compose ps

# Check PostgreSQL
docker exec -it padosme-postgres psql -U postgres -c "\l"

# Check RabbitMQ Management UI
open http://localhost:15672  # guest/guest

# Check Jaeger UI
open http://localhost:16686
```

### 3. Run Individual Microservices

Each microservice has its own docker-compose that connects to the shared network:

```bash
# Auth Service
cd ../padosme-auth-service
docker-compose up -d

# User Profile Service
cd ../padosme-user-profile-service
docker-compose up -d

# Seller Service
cd ../padosme-seller-service
docker-compose up -d
```

## Database Setup

The PostgreSQL container automatically creates these databases:
- `padosme_auth` - Auth service database
- `padosme_profiles` - User profile service database
- `padosme_sellers` - Seller service database

To run migrations for each service:

```bash
# Auth Service
psql -h localhost -U postgres -d padosme_auth -f ../padosme-auth-service/migrations/schema.sql

# User Profile Service
psql -h localhost -U postgres -d padosme_profiles -f ../padosme-user-profile-service/migrations/schema.sql

# Seller Service
psql -h localhost -U postgres -d padosme_sellers -f ../padosme-seller-service/migrations/schema.sql
```

## Network Configuration

All services connect to the `padosme-network` bridge network. Individual microservices should use:

```yaml
networks:
  padosme-network:
    external: true
```

## Stopping Infrastructure

```bash
docker-compose down

# To also remove volumes (data):
docker-compose down -v
```

## Environment Variables

Services should use these connection strings:

| Service    | Host             | Port  | Default Credentials |
|------------|------------------|-------|---------------------|
| PostgreSQL | padosme-postgres | 5432  | postgres/postgres   |
| Redis      | padosme-redis    | 6379  | -                   |
| RabbitMQ   | padosme-rabbitmq | 5672  | guest/guest         |
| Jaeger     | padosme-jaeger   | 4317  | -                   |

## Shared Go Packages

See [`pkg/README.md`](./pkg/README.md) for details on available shared packages:

- **`pkg/telemetry`** - OpenTelemetry setup (traces, metrics, logs)
- **`pkg/worker`** - Supervisor pattern for background workers with panic recovery
- **`pkg/middleware`** - HTTP middleware (logger, recovery)

These packages eliminate ~750 lines of duplicated code per service and provide consistent telemetry, error handling, and worker management.

### Usage

Add to your service's `go.mod`:

```go
replace github.com/kaaslabs/padosme-be-common => ../padosme-be-common
```

Then import:

```go
import "github.com/kaaslabs/padosme-be-common/pkg/telemetry"
import "github.com/kaaslabs/padosme-be-common/pkg/worker"
import "github.com/kaaslabs/padosme-be-common/pkg/middleware"
```
