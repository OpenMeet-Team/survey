package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

// enableTracing sets both gates so InitTracing takes the live path. t.Setenv
// restores the previous values when the test ends, so these no longer leak
// into later tests the way the old os.Setenv/os.Unsetenv pairs did.
func enableTracing(t *testing.T, endpoint string) {
	t.Helper()
	t.Setenv("ENABLE_TRACING", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)
}

// assertTracingDisabled checks the observable signature of "tracing is off":
// InitTracing returned a usable no-op shutdown without installing a global
// TracerProvider.
func assertTracingDisabled(t *testing.T, prev any, shutdown func(context.Context) error, err error) {
	t.Helper()
	require.NoError(t, err)
	require.NotNil(t, shutdown, "shutdown must be non-nil -- callers always defer it")
	assert.True(t, any(otel.GetTracerProvider()) == prev,
		"global TracerProvider should be untouched when tracing is disabled")
	assert.NoError(t, shutdown(context.Background()), "no-op shutdown must be safe to call")
}

func TestInitTracing_DisabledWhenEnableTracingUnset(t *testing.T) {
	t.Setenv("ENABLE_TRACING", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")

	prev := any(otel.GetTracerProvider())
	shutdown, err := InitTracing(context.Background())

	assertTracingDisabled(t, prev, shutdown, err)
}

func TestInitTracing_DisabledWhenEnableTracingNotTrue(t *testing.T) {
	// Only the exact string "true" enables tracing -- "1", "yes", "TRUE" do not.
	for _, v := range []string{"false", "1", "yes", "TRUE"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("ENABLE_TRACING", v)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")

			prev := any(otel.GetTracerProvider())
			shutdown, err := InitTracing(context.Background())

			assertTracingDisabled(t, prev, shutdown, err)
		})
	}
}

// The regression this change exists for: with ENABLE_TRACING=true but no
// endpoint, the old code silently fell back to "localhost:4318" and exported
// into the void forever. Unsetting the variable must actually disable tracing.
func TestInitTracing_DisabledWhenEndpointUnset(t *testing.T) {
	t.Setenv("ENABLE_TRACING", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	prev := any(otel.GetTracerProvider())
	shutdown, err := InitTracing(context.Background())

	assertTracingDisabled(t, prev, shutdown, err)
}

func TestInitTracing_Success(t *testing.T) {
	enableTracing(t, "localhost:4317")
	t.Setenv("OTEL_SERVICE_NAME", "test-service")

	prev := any(otel.GetTracerProvider())
	shutdown, err := InitTracing(context.Background())

	// Should not error even if the endpoint is unavailable -- export failures
	// surface later, in the batch processor, not at init.
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	assert.False(t, any(otel.GetTracerProvider()) == prev,
		"a global TracerProvider should be installed when both gates pass")

	assert.NoError(t, shutdown(context.Background()))
}

func TestInitTracing_WithCustomEndpoint(t *testing.T) {
	enableTracing(t, "custom-endpoint:4318")
	t.Setenv("OTEL_SERVICE_NAME", "custom-service")

	shutdown, err := InitTracing(context.Background())

	require.NoError(t, err)
	require.NotNil(t, shutdown)
	assert.NoError(t, shutdown(context.Background()))
}

// The http:// prefix is stripped before reaching the exporter, which expects
// host:port. This is the form the k8s configmap actually supplied.
func TestInitTracing_StripsSchemeFromEndpoint(t *testing.T) {
	enableTracing(t, "http://jaeger.monitoring.svc.cluster.local:4318")

	shutdown, err := InitTracing(context.Background())

	require.NoError(t, err)
	require.NotNil(t, shutdown)
	assert.NoError(t, shutdown(context.Background()))
}

func TestInitTracing_ShutdownIsIdempotent(t *testing.T) {
	enableTracing(t, "localhost:4318")

	shutdown, err := InitTracing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// Calling shutdown repeatedly should neither panic nor error.
	assert.NoError(t, shutdown(context.Background()))
	assert.NoError(t, shutdown(context.Background()))
}

func TestInitTracing_CreatesSpans(t *testing.T) {
	enableTracing(t, "localhost:4318")

	shutdown, err := InitTracing(context.Background())
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	defer shutdown(context.Background())

	tracer := otel.Tracer("test-tracer")
	require.NotNil(t, tracer)

	ctx, span := tracer.Start(context.Background(), "test-operation")
	require.NotNil(t, span)

	// Recording is independent of whether export succeeds -- there is no
	// collector listening in tests.
	assert.True(t, span.IsRecording(), "span should be recording when tracing is enabled")

	span.End()
	assert.NotNil(t, ctx)
}
