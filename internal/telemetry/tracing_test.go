package telemetry

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestInitTracing_Success(t *testing.T) {
	// Set up environment
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	os.Setenv("OTEL_SERVICE_NAME", "test-service")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	// Call InitTracing
	shutdown, err := InitTracing(context.Background())

	// Should not error even if endpoint is unavailable (warning only)
	assert.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Should set global tracer provider
	tp := otel.GetTracerProvider()
	assert.NotNil(t, tp)

	// Cleanup
	if shutdown != nil {
		err := shutdown(context.Background())
		assert.NoError(t, err)
	}
}

func TestInitTracing_DefaultValues(t *testing.T) {
	// Clear environment variables to test defaults
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	os.Unsetenv("OTEL_SERVICE_NAME")

	// Call InitTracing
	shutdown, err := InitTracing(context.Background())

	// Should use defaults
	assert.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Cleanup
	if shutdown != nil {
		err := shutdown(context.Background())
		assert.NoError(t, err)
	}
}

func TestInitTracing_WithCustomEndpoint(t *testing.T) {
	// Set custom endpoint
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "custom-endpoint:4318")
	os.Setenv("OTEL_SERVICE_NAME", "custom-service")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	// Call InitTracing
	shutdown, err := InitTracing(context.Background())

	// Should not error (endpoint may not exist, but that's OK)
	assert.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Cleanup
	if shutdown != nil {
		err := shutdown(context.Background())
		assert.NoError(t, err)
	}
}

func TestInitTracing_ShutdownIsIdempotent(t *testing.T) {
	// Initialize tracing
	shutdown, err := InitTracing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// Call shutdown multiple times - should not panic or error
	err = shutdown(context.Background())
	assert.NoError(t, err)

	err = shutdown(context.Background())
	assert.NoError(t, err)
}

func TestInitTracing_CreatesSpans(t *testing.T) {
	// Initialize tracing
	shutdown, err := InitTracing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	defer shutdown(context.Background())

	// Get tracer from global provider
	tracer := otel.Tracer("test-tracer")
	assert.NotNil(t, tracer)

	// Create a test span
	ctx, span := tracer.Start(context.Background(), "test-operation")
	assert.NotNil(t, span)

	// Span should be recording (even if export fails due to no Jaeger)
	assert.True(t, span.IsRecording(), "Span should be recording")

	span.End()

	// Verify context contains trace info
	assert.NotNil(t, ctx)
}
