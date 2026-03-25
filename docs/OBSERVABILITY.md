# Observability Stack

This document describes the observability stack setup for Padosme microservices.

## Components

### 1. OpenTelemetry Collector
- **Port**: 4317 (gRPC), 4318 (HTTP)
- **Purpose**: Central collector for traces and metrics from all services
- **Configuration**: `config/otel-collector-config.yaml`

The OpenTelemetry Collector receives telemetry data from your microservices and routes it to the appropriate backends:
- **Traces** → Jaeger
- **Metrics** → Prometheus

### 2. Prometheus
- **Port**: 9090
- **Purpose**: Time-series database for metrics storage
- **Configuration**: `config/prometheus.yml`
- **UI**: http://localhost:9090

Prometheus scrapes metrics from:
- OpenTelemetry Collector (port 8889) - your application metrics
- OpenTelemetry Collector's own metrics (port 8888)
- Itself (port 9090)

### 3. Grafana
- **Port**: 3000
- **Purpose**: Visualization and dashboards
- **Default credentials**: admin/Kaas-Labs
- **UI**: http://localhost:3000

Grafana is pre-configured with Prometheus as a data source.

### 4. Jaeger
- **Port**: 16686 (UI), 14250 (gRPC)
- **Purpose**: Distributed tracing backend
- **UI**: http://localhost:16686

## Quick Start

1. **Start the stack**:
   ```bash
   docker-compose up -d
   ```

2. **Verify all services are running**:
   ```bash
   docker-compose ps
   ```

3. **Access the UIs**:
   - Grafana: http://localhost:3000 (admin/Kaas-Labs)
   - Prometheus: http://localhost:9090
   - Jaeger: http://localhost:16686

## Instrumenting Your Services

### Go Services (using OpenTelemetry)

1. **Install dependencies**:
   ```bash
   go get go.opentelemetry.io/otel
   go get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc
   go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
   go get go.opentelemetry.io/otel/sdk/metric
   go get go.opentelemetry.io/otel/sdk/trace
   ```

2. **Configure OTLP exporter** (example):
   ```go
   import (
       "context"
       "go.opentelemetry.io/otel"
       "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
       "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
       "go.opentelemetry.io/otel/sdk/metric"
       "go.opentelemetry.io/otel/sdk/resource"
       "go.opentelemetry.io/otel/sdk/trace"
       semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
   )

   func initTelemetry(ctx context.Context, serviceName string) (func(), error) {
       res, err := resource.New(ctx,
           resource.WithAttributes(
               semconv.ServiceName(serviceName),
           ),
       )
       if err != nil {
           return nil, err
       }

       // Traces
       traceExporter, err := otlptracegrpc.New(ctx,
           otlptracegrpc.WithEndpoint("localhost:4317"),
           otlptracegrpc.WithInsecure(),
       )
       if err != nil {
           return nil, err
       }

       tracerProvider := trace.NewTracerProvider(
           trace.WithBatcher(traceExporter),
           trace.WithResource(res),
       )
       otel.SetTracerProvider(tracerProvider)

       // Metrics
       metricExporter, err := otlpmetricgrpc.New(ctx,
           otlpmetricgrpc.WithEndpoint("localhost:4317"),
           otlpmetricgrpc.WithInsecure(),
       )
       if err != nil {
           return nil, err
       }

       meterProvider := metric.NewMeterProvider(
           metric.WithReader(metric.NewPeriodicReader(metricExporter)),
           metric.WithResource(res),
       )
       otel.SetMeterProvider(meterProvider)

       return func() {
           tracerProvider.Shutdown(ctx)
           meterProvider.Shutdown(ctx)
       }, nil
   }
   ```

3. **Environment variables** (for containerized services):
   ```yaml
   environment:
     - OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317
     - OTEL_SERVICE_NAME=your-service-name
     - OTEL_TRACES_EXPORTER=otlp
     - OTEL_METRICS_EXPORTER=otlp
   ```

## Creating Custom Metrics

### Counter Example
```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

meter := otel.Meter("my-service")
counter, _ := meter.Int64Counter(
    "http_requests_total",
    metric.WithDescription("Total number of HTTP requests"),
)

// Increment counter
counter.Add(ctx, 1, metric.WithAttributes(
    attribute.String("method", "GET"),
    attribute.String("endpoint", "/api/users"),
))
```

### Histogram Example
```go
histogram, _ := meter.Float64Histogram(
    "http_request_duration_seconds",
    metric.WithDescription("HTTP request latency in seconds"),
)

// Record value
histogram.Record(ctx, duration.Seconds(), metric.WithAttributes(
    attribute.String("method", "POST"),
    attribute.String("endpoint", "/api/orders"),
))
```

## Grafana Dashboards

### Importing Pre-built Dashboards

1. Go to http://localhost:3000
2. Navigate to **Dashboards** → **Import**
3. Use these dashboard IDs:
   - **3662**: Prometheus 2.0 Stats
   - **11074**: Node Exporter for Prometheus
   - **13639**: OpenTelemetry Collector
   - **14282**: Go Processes

### Creating Custom Dashboards

Example PromQL queries:

```promql
# Request rate
rate(http_requests_total[5m])

# Request duration (p95)
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# Error rate
rate(http_requests_total{status=~"5.."}[5m])

# Service availability
up{job="padosme-services"}
```

## Troubleshooting

### Check OpenTelemetry Collector logs
```bash
docker logs padosme-otel-collector
```

### Check Prometheus targets
Visit http://localhost:9090/targets to see if all targets are being scraped successfully.

### Verify metrics are being received
```bash
# Check if metrics are being exported to Prometheus
curl http://localhost:8889/metrics
```

### Test OTLP endpoint
```bash
# Using grpcurl (install with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest)
grpcurl -plaintext localhost:4317 list
```

## Architecture Diagram

```
┌─────────────┐
│   Service   │
│  (Go/Java)  │
└──────┬──────┘
       │ OTLP (4317/4318)
       ▼
┌─────────────────────┐
│ OpenTelemetry       │
│ Collector           │
└─────┬──────────┬────┘
      │          │
      │ Traces   │ Metrics
      ▼          ▼
┌─────────┐  ┌──────────┐
│ Jaeger  │  │Prometheus│
└─────────┘  └────┬─────┘
                  │
                  ▼
             ┌─────────┐
             │ Grafana │
             └─────────┘
```

## Best Practices

1. **Use semantic naming**: Follow OpenTelemetry semantic conventions
2. **Add context**: Include relevant attributes (service, environment, version)
3. **Set appropriate cardinality**: Avoid high-cardinality labels
4. **Use histogram buckets wisely**: Define buckets that match your SLOs
5. **Monitor the monitors**: Keep an eye on collector and Prometheus resource usage
6. **Set up alerts**: Create Prometheus alerting rules for critical metrics

## Additional Exporters (Optional)

To monitor infrastructure components, you can add:

### PostgreSQL Exporter
```yaml
postgres-exporter:
  image: prometheuscommunity/postgres-exporter
  environment:
    - DATA_SOURCE_NAME=postgresql://postgres:postgres@postgres:5432/postgres?sslmode=disable
  ports:
    - "9187:9187"
```

### Redis Exporter
```yaml
redis-exporter:
  image: oliver006/redis_exporter
  environment:
    - REDIS_ADDR=redis:6379
  ports:
    - "9121:9121"
```

## References

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [OpenTelemetry Go SDK](https://github.com/open-telemetry/opentelemetry-go)
