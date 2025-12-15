package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/openmeet-team/survey/internal/generator"
	"github.com/openmeet-team/survey/internal/models"
	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/openmeet-team/survey/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSurvey_Metrics_Success(t *testing.T) {
	// Reset metrics before test
	telemetry.AIGenerationsTotal.Reset()
	telemetry.AITokensTotal.Reset()
	telemetry.AIDailyCostUSD.Set(0)

	e := echo.New()
	mockGen := NewMockSurveyGenerator(&generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{ID: "q1", Text: "Do you like coffee?", Type: "single", Options: []models.Option{
					{ID: "yes", Text: "Yes"},
					{ID: "no", Text: "No"},
				}},
			},
		},
		InputTokens:   100,
		OutputTokens:  200,
		EstimatedCost: 0.005,
	}, nil)

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   mockGen,
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a yes/no poll about coffee",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify metrics were recorded
	successCount := testutil.ToFloat64(telemetry.AIGenerationsTotal.WithLabelValues("success"))
	assert.Equal(t, 1.0, successCount, "success counter should be incremented")

	inputTokens := testutil.ToFloat64(telemetry.AITokensTotal.WithLabelValues("input"))
	assert.Equal(t, 100.0, inputTokens, "input tokens should be recorded")

	outputTokens := testutil.ToFloat64(telemetry.AITokensTotal.WithLabelValues("output"))
	assert.Equal(t, 200.0, outputTokens, "output tokens should be recorded")

	dailyCost := testutil.ToFloat64(telemetry.AIDailyCostUSD)
	assert.Equal(t, 0.005, dailyCost, "daily cost should be updated")
}

func TestGenerateSurvey_Metrics_Error(t *testing.T) {
	// Reset metrics before test
	telemetry.AIGenerationsTotal.Reset()

	e := echo.New()
	mockGen := NewMockSurveyGenerator(nil, assert.AnError)

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   mockGen,
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// Verify error metric was recorded
	errorCount := testutil.ToFloat64(telemetry.AIGenerationsTotal.WithLabelValues("error"))
	assert.Equal(t, 1.0, errorCount, "error counter should be incremented")
}

func TestGenerateSurvey_Metrics_RateLimited_Anonymous(t *testing.T) {
	// Reset metrics before test
	telemetry.AIRateLimitHitsTotal.Reset()

	e := echo.New()
	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(nil, nil),
		generatorRL: NewMockRateLimiter(false, true), // Deny anonymous
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	// Verify rate limit metric was recorded
	rateLimitHits := testutil.ToFloat64(telemetry.AIRateLimitHitsTotal.WithLabelValues("anonymous"))
	assert.Equal(t, 1.0, rateLimitHits, "anonymous rate limit hits should be incremented")
}

func TestGenerateSurvey_Metrics_RateLimited_Authenticated(t *testing.T) {
	// Reset metrics before test
	telemetry.AIRateLimitHitsTotal.Reset()

	e := echo.New()
	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(nil, nil),
		generatorRL: NewMockRateLimiter(true, false), // Deny authenticated
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Set authenticated user context
	user := &oauth.User{DID: "did:plc:test123"}
	c.Set("user", user)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	// Verify rate limit metric was recorded
	rateLimitHits := testutil.ToFloat64(telemetry.AIRateLimitHitsTotal.WithLabelValues("authenticated"))
	assert.Equal(t, 1.0, rateLimitHits, "authenticated rate limit hits should be incremented")
}

func TestGenerateSurvey_Metrics_BudgetExceeded(t *testing.T) {
	// Reset metrics before test
	telemetry.AIGenerationsTotal.Reset()

	e := echo.New()
	mockGen := NewMockSurveyGenerator(nil, assert.AnError) // Will simulate budget error
	// Update mock error to simulate budget exceeded
	mockGen.err = generator.ErrCostLimitExceeded

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   mockGen,
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	// Verify budget exceeded metric was recorded
	budgetCount := testutil.ToFloat64(telemetry.AIGenerationsTotal.WithLabelValues("budget_exceeded"))
	assert.Equal(t, 1.0, budgetCount, "budget_exceeded counter should be incremented")
}

func TestGenerateSurvey_Metrics_Duration(t *testing.T) {
	// We can't easily test histogram values, but we can verify it doesn't panic
	e := echo.New()
	mockGen := NewMockSurveyGenerator(&generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{ID: "q1", Text: "Test?", Type: "single", Options: []models.Option{
					{ID: "yes", Text: "Yes"},
				}},
			},
		},
		InputTokens:   50,
		OutputTokens:  100,
		EstimatedCost: 0.002,
	}, nil)

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   mockGen,
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Should not panic when recording duration
	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}
