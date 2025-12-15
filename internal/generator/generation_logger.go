package generator

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrDatabaseError is returned when database operation fails
	ErrDatabaseError = errors.New("database error while logging AI generation")
)

// AIGenerationLog represents a single AI generation request/response log entry
type AIGenerationLog struct {
	ID           uuid.UUID
	UserID       string // DID for authenticated, IP hash for anonymous
	UserType     string // "anonymous" or "authenticated"
	InputPrompt  string
	SystemPrompt string
	RawResponse  string // Empty if generation failed
	Status       string // "success", "error", "rate_limited", "validation_failed"
	ErrorMessage string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	DurationMS   int
	CreatedAt    time.Time
}

// Validate checks if the log entry is valid
func (l *AIGenerationLog) Validate() error {
	validStatuses := map[string]bool{
		"success":            true,
		"error":              true,
		"rate_limited":       true,
		"validation_failed":  true,
	}
	if !validStatuses[l.Status] {
		return errors.New("invalid status: must be success, error, rate_limited, or validation_failed")
	}

	validUserTypes := map[string]bool{
		"anonymous":     true,
		"authenticated": true,
	}
	if !validUserTypes[l.UserType] {
		return errors.New("invalid user_type: must be anonymous or authenticated")
	}

	return nil
}

// GenerationLogDB defines the interface for logging to database
type GenerationLogDB interface {
	LogGeneration(ctx context.Context, log *AIGenerationLog) error
}

// GenerationLogger logs AI generation requests and responses
type GenerationLogger struct {
	db GenerationLogDB
}

// NewGenerationLogger creates a new generation logger
func NewGenerationLogger(db GenerationLogDB) *GenerationLogger {
	return &GenerationLogger{db: db}
}

// LogSuccess logs a successful AI generation
func (l *GenerationLogger) LogSuccess(
	ctx context.Context,
	userID string,
	userType string,
	inputPrompt string,
	systemPrompt string,
	rawResponse string,
	result *GenerateResult,
	durationMS int,
) error {
	// Allow nil logger (no-op)
	if l == nil {
		return nil
	}

	log := &AIGenerationLog{
		ID:           uuid.New(),
		UserID:       userID,
		UserType:     userType,
		InputPrompt:  inputPrompt,
		SystemPrompt: systemPrompt,
		RawResponse:  rawResponse,
		Status:       "success",
		ErrorMessage: "",
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      result.EstimatedCost,
		DurationMS:   durationMS,
		CreatedAt:    time.Now(),
	}

	if err := log.Validate(); err != nil {
		return err
	}

	return l.db.LogGeneration(ctx, log)
}

// LogError logs a failed AI generation
func (l *GenerationLogger) LogError(
	ctx context.Context,
	userID string,
	userType string,
	inputPrompt string,
	systemPrompt string,
	status string, // "error", "rate_limited", "validation_failed"
	errorMessage string,
	inputTokens int,
	outputTokens int,
	costUSD float64,
	durationMS int,
) error {
	// Allow nil logger (no-op)
	if l == nil {
		return nil
	}

	log := &AIGenerationLog{
		ID:           uuid.New(),
		UserID:       userID,
		UserType:     userType,
		InputPrompt:  inputPrompt,
		SystemPrompt: systemPrompt,
		RawResponse:  "",
		Status:       status,
		ErrorMessage: errorMessage,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
		DurationMS:   durationMS,
		CreatedAt:    time.Now(),
	}

	if err := log.Validate(); err != nil {
		return err
	}

	return l.db.LogGeneration(ctx, log)
}
