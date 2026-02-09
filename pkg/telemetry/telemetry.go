package telemetry

import (
	"context"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config holds OpenTelemetry configuration
type Config struct {
	Enabled      bool
	OTLPEndpoint string
	Insecure     bool
	ServiceName  string
	Environment  string
	Version      string
}

// DefaultConfig returns default telemetry configuration
// serviceName parameter is required to identify the service
func DefaultConfig(serviceName string) Config {
	return Config{
		Enabled:      getEnvBool("OTEL_ENABLED", true),
		OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Insecure:     getEnvBool("OTEL_INSECURE", true),
		ServiceName:  getEnv("OTEL_SERVICE_NAME", serviceName),
		Environment:  getEnv("OTEL_ENVIRONMENT", "development"),
		Version:      getEnv("SERVICE_VERSION", "1.0.0"),
	}
}

// Telemetry holds all OpenTelemetry providers
type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
	Logger         *zap.Logger
	Tracer         trace.Tracer
	config         Config
}

// New initializes OpenTelemetry with traces, metrics, and logs
func New(ctx context.Context, cfg Config) (*Telemetry, error) {
	if !cfg.Enabled {
		// Return a no-op telemetry setup
		logger, _ := zap.NewProduction()
		return &Telemetry{
			Logger: logger,
			Tracer: otel.Tracer(cfg.ServiceName),
			config: cfg,
		}, nil
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// Initialize trace provider
	tracerProvider, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	// Initialize meter provider
	meterProvider, err := initMeterProvider(ctx, cfg, res)
	if err != nil {
		tracerProvider.Shutdown(ctx)
		return nil, err
	}

	// Initialize logger provider
	loggerProvider, err := initLoggerProvider(ctx, cfg, res)
	if err != nil {
		tracerProvider.Shutdown(ctx)
		meterProvider.Shutdown(ctx)
		return nil, err
	}

	// Set global providers
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create zap logger with OpenTelemetry bridge
	logger := createLogger(cfg, loggerProvider)

	return &Telemetry{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		LoggerProvider: loggerProvider,
		Logger:         logger,
		Tracer:         otel.Tracer(cfg.ServiceName),
		config:         cfg,
	}, nil
}

// Shutdown gracefully shuts down all telemetry providers
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t.Logger != nil {
		t.Logger.Sync()
	}

	if t.TracerProvider != nil {
		if err := t.TracerProvider.Shutdown(ctx); err != nil {
			return err
		}
	}

	if t.MeterProvider != nil {
		if err := t.MeterProvider.Shutdown(ctx); err != nil {
			return err
		}
	}

	if t.LoggerProvider != nil {
		if err := t.LoggerProvider.Shutdown(ctx); err != nil {
			return err
		}
	}

	return nil
}

// initTracerProvider creates and configures the trace provider
func initTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	return tp, nil
}

// initMeterProvider creates and configures the meter provider
func initMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)

	return mp, nil
}

// initLoggerProvider creates and configures the logger provider
func initLoggerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}

	exporter, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)

	return lp, nil
}

// createLogger creates a zap logger that bridges to OpenTelemetry
func createLogger(cfg Config, lp *sdklog.LoggerProvider) *zap.Logger {
	// Create console encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Console core for stdout
	consoleCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)

	// OpenTelemetry bridge core
	otelCore := otelzap.NewCore(cfg.ServiceName, otelzap.WithLoggerProvider(lp))

	// Combine both cores
	combinedCore := zapcore.NewTee(consoleCore, otelCore)

	return zap.New(combinedCore,
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	)
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
