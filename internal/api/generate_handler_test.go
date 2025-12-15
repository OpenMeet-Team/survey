package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/openmeet-team/survey/internal/generator"
	"github.com/openmeet-team/survey/internal/models"
	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSurveyGenerator implements a mock version of generator.SurveyGenerator for testing
type MockSurveyGenerator struct {
	result        *generator.GenerateResult
	err           error
	validateError error
}

func (m *MockSurveyGenerator) Generate(ctx context.Context, prompt string) (*generator.GenerateResult, error) {
	return m.result, m.err
}

func (m *MockSurveyGenerator) GenerateRaw(ctx context.Context, prompt string) (*generator.GenerateResult, error) {
	return m.result, m.err
}

func (m *MockSurveyGenerator) ValidateInput(input string) error {
	return m.validateError
}

func NewMockSurveyGenerator(result *generator.GenerateResult, err error) *MockSurveyGenerator {
	return &MockSurveyGenerator{result: result, err: err}
}

// MockRateLimiter implements a mock version of generator.RateLimiter for testing
type MockRateLimiter struct {
	allowAnon bool
	allowAuth bool
}

func (m *MockRateLimiter) AllowAnonymous(ip string) bool {
	return m.allowAnon
}

func (m *MockRateLimiter) AllowAuthenticated(did string) bool {
	return m.allowAuth
}

func NewMockRateLimiter(allowAnon, allowAuth bool) *MockRateLimiter {
	return &MockRateLimiter{allowAnon: allowAnon, allowAuth: allowAuth}
}

// RED PHASE: Write failing tests

func TestGenerateSurvey_MissingConsent(t *testing.T) {
	e := echo.New()
	h := &Handlers{
		queries:        NewMockQueries(),
		generator:      NewMockSurveyGenerator(nil, nil),
		generatorRL:    NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a simple yes/no poll about coffee preference",
		Consent:     false, // No consent given
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp ErrorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "consent")
}

func TestGenerateSurvey_EmptyDescription(t *testing.T) {
	e := echo.New()
	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(nil, nil),
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "", // Empty description
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp ErrorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "empty")
}

func TestGenerateSurvey_Success_Anonymous(t *testing.T) {
	e := echo.New()

	// Mock successful generation
	mockResult := &generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{
					ID:       "q1",
					Text:     "Do you like coffee?",
					Type:     models.QuestionTypeSingle,
					Required: false,
					Options: []models.Option{
						{ID: "yes", Text: "Yes"},
						{ID: "no", Text: "No"},
					},
				},
			},
			Anonymous: false,
		},
		InputTokens:   50,
		OutputTokens:  100,
		EstimatedCost: 0.005,
	}

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(mockResult, nil),
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Create a simple yes/no poll about coffee preference",
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

	var resp GenerateSurveyResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Definition)
	assert.Equal(t, "Do you like coffee?", resp.Definition.Questions[0].Text)
	assert.Equal(t, 150, resp.TokensUsed) // 50 + 100
	assert.Equal(t, 0.005, resp.Cost)
	assert.False(t, resp.NeedsCaptcha)
}

func TestGenerateSurvey_Success_Authenticated(t *testing.T) {
	e := echo.New()

	mockResult := &generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{
					ID:       "q1",
					Text:     "What's your favorite programming language?",
					Type:     models.QuestionTypeSingle,
					Required: false,
					Options: []models.Option{
						{ID: "go", Text: "Go"},
						{ID: "rust", Text: "Rust"},
						{ID: "python", Text: "Python"},
					},
				},
			},
			Anonymous: false,
		},
		InputTokens:   60,
		OutputTokens:  120,
		EstimatedCost: 0.006,
	}

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(mockResult, nil),
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Poll about favorite programming languages",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Set authenticated user in context
	user := &oauth.User{
		DID: "did:plc:test123",
	}
	c.Set("user", user)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GenerateSurveyResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Definition)
	assert.Equal(t, "What's your favorite programming language?", resp.Definition.Questions[0].Text)
	assert.Equal(t, 180, resp.TokensUsed) // 60 + 120
	assert.Equal(t, 0.006, resp.Cost)
	assert.False(t, resp.NeedsCaptcha)
}

func TestGenerateSurvey_RateLimitExceeded_Anonymous(t *testing.T) {
	e := echo.New()

	// Mock rate limit - set to false so it's exceeded
	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(nil, nil),
		generatorRL: NewMockRateLimiter(false, true), // Anonymous blocked
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

	var resp ErrorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "Rate limit")
}

func TestGenerateSurvey_CostLimitExceeded(t *testing.T) {
	e := echo.New()

	// Mock cost limit error - this comes from the generator
	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(nil, generator.ErrCostLimitExceeded),
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

	var resp ErrorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "budget")
}

func TestGenerateSurvey_InvalidInput(t *testing.T) {
	e := echo.New()

	// Mock validation error
	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(nil, generator.ErrInputTooLong),
		generatorRL: NewMockRateLimiter(true, true),
	}

	reqBody := GenerateSurveyRequest{
		Description: "Very long description that exceeds the maximum allowed length...",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp ErrorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "too long")
}

func TestGenerateSurvey_WithExistingJSON(t *testing.T) {
	e := echo.New()

	mockResult := &generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{
					ID:       "q1",
					Text:     "Updated question text",
					Type:     models.QuestionTypeSingle,
					Required: false,
					Options: []models.Option{
						{ID: "yes", Text: "Yes"},
						{ID: "no", Text: "No"},
					},
				},
			},
			Anonymous: false,
		},
		InputTokens:   70,
		OutputTokens:  130,
		EstimatedCost: 0.007,
	}

	h := &Handlers{
		queries:     NewMockQueries(),
		generator:   NewMockSurveyGenerator(mockResult, nil),
		generatorRL: NewMockRateLimiter(true, true),
	}

	existingJSON := `{"questions":[{"id":"q1","text":"Original question","type":"single","options":[{"id":"yes","text":"Yes"}]}]}`

	reqBody := GenerateSurveyRequest{
		Description:  "Make this question better",
		ExistingJSON: existingJSON,
		Consent:      true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GenerateSurveyResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Definition)
	assert.Equal(t, "Updated question text", resp.Definition.Questions[0].Text)
}
