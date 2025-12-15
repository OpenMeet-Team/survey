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
)

// MockGenerationLogger mocks the generation logger for testing
type MockGenerationLogger struct {
	successCalls []LogSuccessParams
	errorCalls   []LogErrorParams
}

type LogSuccessParams struct {
	UserID       string
	UserType     string
	InputPrompt  string
	SystemPrompt string
	RawResponse  string
	Result       *generator.GenerateResult
	DurationMS   int
}

type LogErrorParams struct {
	UserID       string
	UserType     string
	InputPrompt  string
	SystemPrompt string
	Status       string
	ErrorMessage string
	DurationMS   int
}

func (m *MockGenerationLogger) LogSuccess(
	ctx context.Context,
	userID string,
	userType string,
	inputPrompt string,
	systemPrompt string,
	rawResponse string,
	result *generator.GenerateResult,
	durationMS int,
) error {
	m.successCalls = append(m.successCalls, LogSuccessParams{
		UserID:       userID,
		UserType:     userType,
		InputPrompt:  inputPrompt,
		SystemPrompt: systemPrompt,
		RawResponse:  rawResponse,
		Result:       result,
		DurationMS:   durationMS,
	})
	return nil
}

func (m *MockGenerationLogger) LogError(
	ctx context.Context,
	userID string,
	userType string,
	inputPrompt string,
	systemPrompt string,
	status string,
	errorMessage string,
	inputTokens int,
	outputTokens int,
	costUSD float64,
	durationMS int,
) error {
	m.errorCalls = append(m.errorCalls, LogErrorParams{
		UserID:       userID,
		UserType:     userType,
		InputPrompt:  inputPrompt,
		SystemPrompt: systemPrompt,
		Status:       status,
		ErrorMessage: errorMessage,
		DurationMS:   durationMS,
	})
	return nil
}

// TestGenerateSurvey_Logging_Success verifies successful generation is logged
func TestGenerateSurvey_Logging_Success(t *testing.T) {
	e := echo.New()

	mockGen := NewMockSurveyGenerator(&generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{ID: "q1", Text: "Test?", Type: "single", Options: []models.Option{{ID: "opt1", Text: "Yes"}}},
			},
		},
		InputTokens:   100,
		OutputTokens:  50,
		EstimatedCost: 0.002,
		SystemPrompt:  "You are a helpful survey generator...",
		RawResponse:   `{"questions":[{"id":"q1","text":"Test?","type":"single","required":false,"options":[{"id":"opt1","text":"Yes"}]}],"anonymous":false}`,
	}, nil)

	mockRL := NewMockRateLimiter(true, true)
	mockLogger := &MockGenerationLogger{}

	h := NewHandlers(nil)
	h.SetGenerator(mockGen, mockRL)
	h.SetLogger(mockLogger)

	reqBody := GenerateSurveyRequest{
		Description: "Create a yes/no poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify logger was called
	if len(mockLogger.successCalls) != 1 {
		t.Fatalf("Expected 1 success log call, got %d", len(mockLogger.successCalls))
	}

	logCall := mockLogger.successCalls[0]

	// Verify user type
	if logCall.UserType != "anonymous" {
		t.Errorf("Expected user_type=anonymous, got %s", logCall.UserType)
	}

	// Verify prompt logged
	if logCall.InputPrompt != "Create a yes/no poll" {
		t.Errorf("Expected input_prompt to be logged, got %s", logCall.InputPrompt)
	}

	// Verify system prompt logged
	if logCall.SystemPrompt == "" {
		t.Error("Expected system prompt to be logged")
	}

	// Verify raw response logged
	if logCall.RawResponse == "" {
		t.Error("Expected raw response to be logged")
	}

	// Verify duration tracked (can be 0 for very fast mock responses)
	if logCall.DurationMS < 0 {
		t.Errorf("Expected non-negative duration, got %d", logCall.DurationMS)
	}
}

// TestGenerateSurvey_Logging_RateLimited verifies rate limit errors are logged
func TestGenerateSurvey_Logging_RateLimited(t *testing.T) {
	e := echo.New()

	mockGen := NewMockSurveyGenerator(nil, nil)
	mockRL := NewMockRateLimiter(false, false) // Rate limited
	mockLogger := &MockGenerationLogger{}

	h := NewHandlers(nil)
	h.SetGenerator(mockGen, mockRL)
	h.SetLogger(mockLogger)

	reqBody := GenerateSurveyRequest{
		Description: "Create a survey",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify logger was called with rate_limited status
	if len(mockLogger.errorCalls) != 1 {
		t.Fatalf("Expected 1 error log call, got %d", len(mockLogger.errorCalls))
	}

	logCall := mockLogger.errorCalls[0]

	if logCall.Status != "rate_limited" {
		t.Errorf("Expected status=rate_limited, got %s", logCall.Status)
	}

	if logCall.InputPrompt != "Create a survey" {
		t.Errorf("Expected prompt to be logged, got %s", logCall.InputPrompt)
	}
}

// TestGenerateSurvey_Logging_ValidationFailed verifies validation errors are logged
func TestGenerateSurvey_Logging_ValidationFailed(t *testing.T) {
	e := echo.New()

	mockGen := NewMockSurveyGenerator(nil, generator.ErrInputTooLong)
	mockRL := NewMockRateLimiter(true, true)
	mockLogger := &MockGenerationLogger{}

	h := NewHandlers(nil)
	h.SetGenerator(mockGen, mockRL)
	h.SetLogger(mockLogger)

	reqBody := GenerateSurveyRequest{
		Description: "Very long prompt...",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify logger was called with validation_failed status
	if len(mockLogger.errorCalls) != 1 {
		t.Fatalf("Expected 1 error log call, got %d", len(mockLogger.errorCalls))
	}

	logCall := mockLogger.errorCalls[0]

	if logCall.Status != "validation_failed" {
		t.Errorf("Expected status=validation_failed, got %s", logCall.Status)
	}
}

// TestGenerateSurvey_Logging_Error verifies LLM errors are logged
func TestGenerateSurvey_Logging_Error(t *testing.T) {
	e := echo.New()

	mockGen := NewMockSurveyGenerator(nil, generator.ErrEmptyResponse)
	mockRL := NewMockRateLimiter(true, true)
	mockLogger := &MockGenerationLogger{}

	h := NewHandlers(nil)
	h.SetGenerator(mockGen, mockRL)
	h.SetLogger(mockLogger)

	reqBody := GenerateSurveyRequest{
		Description: "Create a survey",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify logger was called with error status
	if len(mockLogger.errorCalls) != 1 {
		t.Fatalf("Expected 1 error log call, got %d", len(mockLogger.errorCalls))
	}

	logCall := mockLogger.errorCalls[0]

	if logCall.Status != "error" {
		t.Errorf("Expected status=error, got %s", logCall.Status)
	}

	if logCall.ErrorMessage == "" {
		t.Error("Expected error message to be logged")
	}
}

// TestGenerateSurvey_Logging_NilLogger verifies handler works without logger
func TestGenerateSurvey_Logging_NilLogger(t *testing.T) {
	e := echo.New()

	mockGen := NewMockSurveyGenerator(&generator.GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{ID: "q1", Text: "Test?", Type: "single", Options: []models.Option{{ID: "opt1", Text: "Yes"}}},
			},
		},
		InputTokens:   100,
		OutputTokens:  50,
		EstimatedCost: 0.002,
		SystemPrompt:  "You are a helpful survey generator...",
		RawResponse:   `{"questions":[{"id":"q1","text":"Test?","type":"single","required":false,"options":[{"id":"opt1","text":"Yes"}]}],"anonymous":false}`,
	}, nil)

	mockRL := NewMockRateLimiter(true, true)

	h := NewHandlers(nil)
	h.SetGenerator(mockGen, mockRL)
	// Don't set logger - should work without it

	reqBody := GenerateSurveyRequest{
		Description: "Create a yes/no poll",
		Consent:     true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/generate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.GenerateSurvey(c)
	if err != nil {
		t.Fatalf("Handler should work without logger, got error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}
