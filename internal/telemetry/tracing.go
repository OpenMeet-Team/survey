package telemetry

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.28.0"
)

// InitTracing initializes OpenTelemetry tracing with OTLP HTTP exporter
// Returns a shutdown function that should be deferred
func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	// Get configuration from environment
	// Default to HTTP OTLP port (4318) - more compatible than gRPC (4317)
	endpoint := getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")
	serviceName := getEnvOrDefault("OTEL_SERVICE_NAME", "survey-api")

	// Strip http:// or https:// scheme if present (HTTP exporter expects host:port only)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Create OTLP HTTP trace exporter
	// Use insecure connection for local development
	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		// Log warning but don't fail - allow service to run without tracing
		log.Printf("Warning: Failed to create OTLP exporter (endpoint: %s): %v", endpoint, err)
		return func(context.Context) error { return nil }, nil
	}

	// Create resource with service name
	// Use NewSchemaless to avoid schema URL conflicts
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	log.Printf("OpenTelemetry tracing initialized (service: %s, endpoint: %s)", serviceName, endpoint)

	// Return shutdown function
	return func(ctx context.Context) error {
		// Shutdown tracer provider (will flush remaining spans)
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown tracer provider: %w", err)
		}
		return nil
	}, nil
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
