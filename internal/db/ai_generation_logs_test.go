//go:build e2e

package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/google/uuid"
	"github.com/openmeet-team/survey/internal/generator"
)

// TestLogGeneration_Success tests successful insertion of a generation log
func TestLogGeneration_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	queries := NewQueries(db)

	log := &generator.AIGenerationLog{
		ID:           uuid.New(),
		UserID:       "did:plc:test123",
		UserType:     "authenticated",
		InputPrompt:  "Create a simple survey about coffee preferences",
		SystemPrompt: "You are a helpful survey generator...",
		RawResponse:  `{"questions":[{"id":"q1","text":"Do you like coffee?","type":"single","required":false,"options":[{"id":"opt1","text":"Yes"},{"id":"opt2","text":"No"}]}],"anonymous":false}`,
		Status:       "success",
		ErrorMessage: "",
		InputTokens:  150,
		OutputTokens: 75,
		CostUSD:      0.0035,
		DurationMS:   1234,
		CreatedAt:    time.Now(),
	}

	err := queries.LogGeneration(context.Background(), log)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the log was inserted by retrieving it
	retrieved, err := queries.GetGenerationLog(context.Background(), log.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve log: %v", err)
	}

	if retrieved.UserID != log.UserID {
		t.Errorf("Expected user_id=%s, got %s", log.UserID, retrieved.UserID)
	}
	if retrieved.Status != log.Status {
		t.Errorf("Expected status=%s, got %s", log.Status, retrieved.Status)
	}
	if retrieved.InputTokens != log.InputTokens {
		t.Errorf("Expected input_tokens=%d, got %d", log.InputTokens, retrieved.InputTokens)
	}
	if retrieved.OutputTokens != log.OutputTokens {
		t.Errorf("Expected output_tokens=%d, got %d", log.OutputTokens, retrieved.OutputTokens)
	}
}

// TestLogGeneration_Error tests logging an error case
func TestLogGeneration_Error(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	queries := NewQueries(db)

	log := &generator.AIGenerationLog{
		ID:           uuid.New(),
		UserID:       "192.168.1.1",
		UserType:     "anonymous",
		InputPrompt:  "Create a survey with 1000 questions",
		SystemPrompt: "You are a helpful survey generator...",
		RawResponse:  "",
		Status:       "validation_failed",
		ErrorMessage: "Input too long: exceeds maximum length of 2000 characters",
		InputTokens:  0,
		OutputTokens: 0,
		CostUSD:      0.0,
		DurationMS:   50,
		CreatedAt:    time.Now(),
	}

	err := queries.LogGeneration(context.Background(), log)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the log was inserted
	retrieved, err := queries.GetGenerationLog(context.Background(), log.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve log: %v", err)
	}

	if retrieved.Status != "validation_failed" {
		t.Errorf("Expected status=validation_failed, got %s", retrieved.Status)
	}
	if retrieved.ErrorMessage != log.ErrorMessage {
		t.Errorf("Expected error_message=%s, got %s", log.ErrorMessage, retrieved.ErrorMessage)
	}
	if retrieved.RawResponse != "" {
		t.Errorf("Expected empty raw_response, got %s", retrieved.RawResponse)
	}
}

// TestGetGenerationLogsByUser tests retrieving logs for a specific user
func TestGetGenerationLogsByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	queries := NewQueries(db)

	userID := "did:plc:usertest"

	// Insert multiple logs for this user
	for i := 0; i < 3; i++ {
		log := &generator.AIGenerationLog{
			ID:           uuid.New(),
			UserID:       userID,
			UserType:     "authenticated",
			InputPrompt:  "Test prompt",
			SystemPrompt: "System prompt",
			RawResponse:  "{}",
			Status:       "success",
			InputTokens:  100,
			OutputTokens: 50,
			CostUSD:      0.001,
			DurationMS:   1000,
			CreatedAt:    time.Now().Add(-time.Duration(i) * time.Hour),
		}
		if err := queries.LogGeneration(context.Background(), log); err != nil {
			t.Fatalf("Failed to insert log: %v", err)
		}
	}

	// Retrieve logs for this user
	logs, err := queries.GetGenerationLogsByUser(context.Background(), userID, 10, 0)
	if err != nil {
		t.Fatalf("Failed to retrieve logs: %v", err)
	}

	if len(logs) != 3 {
		t.Errorf("Expected 3 logs, got %d", len(logs))
	}

	// Verify logs are ordered by created_at DESC (most recent first)
	for i := 0; i < len(logs)-1; i++ {
		if logs[i].CreatedAt.Before(logs[i+1].CreatedAt) {
			t.Errorf("Logs are not ordered by created_at DESC")
		}
	}
}

// TestGetGenerationLogsByStatus tests retrieving logs by status
func TestGetGenerationLogsByStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	queries := NewQueries(db)

	// Insert logs with different statuses
	statuses := []string{"success", "error", "rate_limited", "validation_failed"}
	for _, status := range statuses {
		log := &generator.AIGenerationLog{
			ID:           uuid.New(),
			UserID:       "did:plc:test",
			UserType:     "authenticated",
			InputPrompt:  "Test",
			SystemPrompt: "System",
			Status:       status,
			CreatedAt:    time.Now(),
		}
		if err := queries.LogGeneration(context.Background(), log); err != nil {
			t.Fatalf("Failed to insert log: %v", err)
		}
	}

	// Retrieve only error logs
	errorLogs, err := queries.GetGenerationLogsByStatus(context.Background(), "error", 10, 0)
	if err != nil {
		t.Fatalf("Failed to retrieve error logs: %v", err)
	}

	if len(errorLogs) < 1 {
		t.Errorf("Expected at least 1 error log, got %d", len(errorLogs))
	}

	for _, log := range errorLogs {
		if log.Status != "error" {
			t.Errorf("Expected status=error, got %s", log.Status)
		}
	}
}

// TestGetRecentGenerationLogs tests retrieving recent logs
func TestGetRecentGenerationLogs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	queries := NewQueries(db)

	// Insert 5 logs
	for i := 0; i < 5; i++ {
		log := &generator.AIGenerationLog{
			ID:           uuid.New(),
			UserID:       "did:plc:test",
			UserType:     "authenticated",
			InputPrompt:  "Test",
			SystemPrompt: "System",
			Status:       "success",
			CreatedAt:    time.Now().Add(-time.Duration(i) * time.Minute),
		}
		if err := queries.LogGeneration(context.Background(), log); err != nil {
			t.Fatalf("Failed to insert log: %v", err)
		}
	}

	// Retrieve only 3 most recent
	logs, err := queries.GetRecentGenerationLogs(context.Background(), 3, 0)
	if err != nil {
		t.Fatalf("Failed to retrieve logs: %v", err)
	}

	if len(logs) != 3 {
		t.Errorf("Expected 3 logs, got %d", len(logs))
	}

	// Verify ordering
	for i := 0; i < len(logs)-1; i++ {
		if logs[i].CreatedAt.Before(logs[i+1].CreatedAt) {
			t.Errorf("Logs are not ordered by created_at DESC")
		}
	}
}

// setupTestDB sets up a test database connection
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Get DB config from environment
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load database config: %v", err)
	}

	// Connect to test database
	ctx := context.Background()
	dbConn, err := Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Clean up test data before each test
	_, err = dbConn.Exec("DELETE FROM ai_generation_logs WHERE user_id LIKE '%test%' OR user_id LIKE '192.168%'")
	if err != nil {
		t.Logf("Warning: failed to clean ai_generation_logs: %v", err)
	}

	return dbConn
}
