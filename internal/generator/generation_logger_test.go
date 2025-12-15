package generator

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openmeet-team/survey/internal/models"
)

// MockLogDB is a mock database for testing the logger
type MockLogDB struct {
	lastLog *AIGenerationLog
	callCount int
	shouldError bool
}

func (m *MockLogDB) LogGeneration(ctx context.Context, log *AIGenerationLog) error {
	m.callCount++
	m.lastLog = log
	if m.shouldError {
		return ErrDatabaseError
	}
	return nil
}

func TestGenerationLogger_LogSuccess(t *testing.T) {
	mockDB := &MockLogDB{}
	logger := NewGenerationLogger(mockDB)

	ctx := context.Background()
	userID := "did:plc:test123"
	userType := "authenticated"
	inputPrompt := "Create a simple yes/no survey"
	systemPrompt := "You are a survey generator..."
	rawResponse := `{"questions":[{"id":"q1","text":"Do you agree?","type":"single","required":false,"options":[{"id":"opt1","text":"Yes"},{"id":"opt2","text":"No"}]}],"anonymous":false}`

	result := &GenerateResult{
		Definition: &models.SurveyDefinition{
			Questions: []models.Question{
				{
					ID:       "q1",
					Text:     "Do you agree?",
					Type:     "single",
					Required: false,
					Options: []models.Option{
						{ID: "opt1", Text: "Yes"},
						{ID: "opt2", Text: "No"},
					},
				},
			},
			Anonymous: false,
		},
		InputTokens:   100,
		OutputTokens:  50,
		EstimatedCost: 0.0025,
	}

	err := logger.LogSuccess(ctx, userID, userType, inputPrompt, systemPrompt, rawResponse, result, 1500)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if mockDB.callCount != 1 {
		t.Errorf("Expected 1 database call, got %d", mockDB.callCount)
	}

	log := mockDB.lastLog
	if log == nil {
		t.Fatal("Expected log to be created")
	}

	if log.UserID != userID {
		t.Errorf("Expected user_id=%s, got %s", userID, log.UserID)
	}
	if log.UserType != userType {
		t.Errorf("Expected user_type=%s, got %s", userType, log.UserType)
	}
	if log.InputPrompt != inputPrompt {
		t.Errorf("Expected input_prompt=%s, got %s", inputPrompt, log.InputPrompt)
	}
	if log.SystemPrompt != systemPrompt {
		t.Errorf("Expected system_prompt=%s, got %s", systemPrompt, log.SystemPrompt)
	}
	if log.RawResponse != rawResponse {
		t.Errorf("Expected raw_response=%s, got %s", rawResponse, log.RawResponse)
	}
	if log.Status != "success" {
		t.Errorf("Expected status=success, got %s", log.Status)
	}
	if log.ErrorMessage != "" {
		t.Errorf("Expected empty error_message, got %s", log.ErrorMessage)
	}
	if log.InputTokens != 100 {
		t.Errorf("Expected input_tokens=100, got %d", log.InputTokens)
	}
	if log.OutputTokens != 50 {
		t.Errorf("Expected output_tokens=50, got %d", log.OutputTokens)
	}
	if log.CostUSD != 0.0025 {
		t.Errorf("Expected cost_usd=0.0025, got %f", log.CostUSD)
	}
	if log.DurationMS != 1500 {
		t.Errorf("Expected duration_ms=1500, got %d", log.DurationMS)
	}
}

func TestGenerationLogger_LogError(t *testing.T) {
	mockDB := &MockLogDB{}
	logger := NewGenerationLogger(mockDB)

	ctx := context.Background()
	userID := "192.168.1.1"
	userType := "anonymous"
	inputPrompt := "Create a survey with 1000 questions"
	systemPrompt := "You are a survey generator..."
	errorMsg := "input too long: exceeds 2000 characters"

	err := logger.LogError(ctx, userID, userType, inputPrompt, systemPrompt, "validation_failed", errorMsg, 0, 0, 0.0, 50)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if mockDB.callCount != 1 {
		t.Errorf("Expected 1 database call, got %d", mockDB.callCount)
	}

	log := mockDB.lastLog
	if log == nil {
		t.Fatal("Expected log to be created")
	}

	if log.UserID != userID {
		t.Errorf("Expected user_id=%s, got %s", userID, log.UserID)
	}
	if log.UserType != userType {
		t.Errorf("Expected user_type=%s, got %s", userType, log.UserType)
	}
	if log.Status != "validation_failed" {
		t.Errorf("Expected status=validation_failed, got %s", log.Status)
	}
	if log.ErrorMessage != errorMsg {
		t.Errorf("Expected error_message=%s, got %s", errorMsg, log.ErrorMessage)
	}
	if log.RawResponse != "" {
		t.Errorf("Expected empty raw_response for error, got %s", log.RawResponse)
	}
	if log.DurationMS != 50 {
		t.Errorf("Expected duration_ms=50, got %d", log.DurationMS)
	}
}

func TestGenerationLogger_LogRateLimited(t *testing.T) {
	mockDB := &MockLogDB{}
	logger := NewGenerationLogger(mockDB)

	ctx := context.Background()
	userID := "did:plc:ratelimited"
	userType := "authenticated"
	inputPrompt := "Another survey"
	systemPrompt := "You are a survey generator..."

	err := logger.LogError(ctx, userID, userType, inputPrompt, systemPrompt, "rate_limited", "Rate limit exceeded", 0, 0, 0.0, 0)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	log := mockDB.lastLog
	if log.Status != "rate_limited" {
		t.Errorf("Expected status=rate_limited, got %s", log.Status)
	}
}

func TestGenerationLogger_NilLogger(t *testing.T) {
	// Nil logger should be safe to call (no-op)
	var logger *GenerationLogger

	ctx := context.Background()
	err := logger.LogSuccess(ctx, "did:test", "authenticated", "prompt", "system", "response", &GenerateResult{}, 100)

	if err != nil {
		t.Errorf("Nil logger should not error, got %v", err)
	}

	err = logger.LogError(ctx, "did:test", "authenticated", "prompt", "system", "error", "message", 0, 0, 0.0, 100)

	if err != nil {
		t.Errorf("Nil logger should not error, got %v", err)
	}
}

func TestGenerationLogger_DatabaseError(t *testing.T) {
	mockDB := &MockLogDB{shouldError: true}
	logger := NewGenerationLogger(mockDB)

	ctx := context.Background()
	err := logger.LogSuccess(ctx, "did:test", "authenticated", "prompt", "system", "response", &GenerateResult{}, 100)

	if err != ErrDatabaseError {
		t.Errorf("Expected ErrDatabaseError, got %v", err)
	}
}

func TestGenerationLogger_ContextCanceled(t *testing.T) {
	mockDB := &MockLogDB{}
	logger := NewGenerationLogger(mockDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := logger.LogSuccess(ctx, "did:test", "authenticated", "prompt", "system", "response", &GenerateResult{}, 100)

	// Should still attempt to log even if context is canceled (fire-and-forget logging)
	// But check if implementation respects context
	if err != nil && err != context.Canceled {
		t.Logf("Logger returned error on canceled context: %v", err)
	}
}

func TestAIGenerationLog_Validation(t *testing.T) {
	tests := []struct {
		name      string
		log       *AIGenerationLog
		shouldErr bool
	}{
		{
			name: "valid success log",
			log: &AIGenerationLog{
				ID:           uuid.New(),
				UserID:       "did:plc:test",
				UserType:     "authenticated",
				InputPrompt:  "test prompt",
				SystemPrompt: "system prompt",
				RawResponse:  "{}",
				Status:       "success",
				InputTokens:  100,
				OutputTokens: 50,
				CostUSD:      0.01,
				DurationMS:   1000,
				CreatedAt:    time.Now(),
			},
			shouldErr: false,
		},
		{
			name: "valid error log",
			log: &AIGenerationLog{
				ID:           uuid.New(),
				UserID:       "192.168.1.1",
				UserType:     "anonymous",
				InputPrompt:  "test prompt",
				SystemPrompt: "system prompt",
				RawResponse:  "",
				Status:       "error",
				ErrorMessage: "something went wrong",
				InputTokens:  0,
				OutputTokens: 0,
				CostUSD:      0.0,
				DurationMS:   50,
				CreatedAt:    time.Now(),
			},
			shouldErr: false,
		},
		{
			name: "invalid status",
			log: &AIGenerationLog{
				ID:           uuid.New(),
				UserID:       "did:plc:test",
				UserType:     "authenticated",
				InputPrompt:  "test",
				SystemPrompt: "system",
				Status:       "invalid_status",
				CreatedAt:    time.Now(),
			},
			shouldErr: true,
		},
		{
			name: "invalid user_type",
			log: &AIGenerationLog{
				ID:           uuid.New(),
				UserID:       "did:plc:test",
				UserType:     "invalid_type",
				InputPrompt:  "test",
				SystemPrompt: "system",
				Status:       "success",
				CreatedAt:    time.Now(),
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.log.Validate()
			if tt.shouldErr && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no validation error, got %v", err)
			}
		})
	}
}
