package main

// This is an example of how to instrument your Go microservice
// to send telemetry data to the OpenTelemetry Collector

import (
	"context"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Initialize OpenTelemetry with OTLP exporters
func initTelemetry(ctx context.Context, serviceName, version string) (func(), error) {
	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("environment", "development"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Setup Trace Provider
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("localhost:4317"),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tracerProvider)

	// Setup Meter Provider
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("localhost:4317"),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(10*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	// Return cleanup function
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := tracerProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}, nil
}

// Example HTTP handler with metrics and tracing
type InstrumentedHandler struct {
	tracer            trace.Tracer
	requestCounter    metric.Int64Counter
	requestDuration   metric.Float64Histogram
	activeConnections metric.Int64UpDownCounter
}

func NewInstrumentedHandler(serviceName string) (*InstrumentedHandler, error) {
	meter := otel.Meter(serviceName)
	tracer := otel.Tracer(serviceName)

	// Create metrics
	requestCounter, err := meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	activeConnections, err := meter.Int64UpDownCounter(
		"http_active_connections",
		metric.WithDescription("Number of active HTTP connections"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	return &InstrumentedHandler{
		tracer:            tracer,
		requestCounter:    requestCounter,
		requestDuration:   requestDuration,
		activeConnections: activeConnections,
	}, nil
}

func (h *InstrumentedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	// Start a span
	ctx, span := h.tracer.Start(ctx, "HTTP "+r.Method+" "+r.URL.Path,
		trace.WithAttributes(
			semconv.HTTPMethod(r.Method),
			semconv.HTTPRoute(r.URL.Path),
			semconv.HTTPScheme(r.URL.Scheme),
		),
	)
	defer span.End()

	// Track active connections
	h.activeConnections.Add(ctx, 1)
	defer h.activeConnections.Add(ctx, -1)

	// Process request
	statusCode := http.StatusOK
	w.WriteHeader(statusCode)
	w.Write([]byte("Hello, World!"))

	// Record metrics
	duration := time.Since(start).Seconds()

	attrs := []attribute.KeyValue{
		attribute.String("method", r.Method),
		attribute.String("route", r.URL.Path),
		attribute.Int("status_code", statusCode),
	}

	h.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	h.requestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))

	// Add span attributes
	span.SetAttributes(
		semconv.HTTPStatusCode(statusCode),
		attribute.Float64("http.duration", duration),
	)
}

func main() {
	ctx := context.Background()

	// Initialize telemetry
	cleanup, err := initTelemetry(ctx, "example-service", "1.0.0")
	if err != nil {
		log.Fatalf("Failed to initialize telemetry: %v", err)
	}
	defer cleanup()

	// Create instrumented handler
	handler, err := NewInstrumentedHandler("example-service")
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Start server
	log.Println("Starting server on :8080")
	log.Println("Sending telemetry to localhost:4317")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
