package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/openmeet-team/survey/internal/generator"
)

// LogGeneration implements the GenerationLogDB interface
// Inserts an AI generation log into the database
func (q *Queries) LogGeneration(ctx context.Context, log *generator.AIGenerationLog) error {
	query := `
		INSERT INTO ai_generation_logs (
			id, user_id, user_type, input_prompt, system_prompt, raw_response,
			status, error_message, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := q.db.ExecContext(
		ctx,
		query,
		log.ID,
		log.UserID,
		log.UserType,
		log.InputPrompt,
		log.SystemPrompt,
		log.RawResponse,
		log.Status,
		log.ErrorMessage,
		log.InputTokens,
		log.OutputTokens,
		log.CostUSD,
		log.DurationMS,
		log.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert AI generation log: %w", err)
	}

	return nil
}

// GetGenerationLog retrieves a single AI generation log by ID
func (q *Queries) GetGenerationLog(ctx context.Context, id uuid.UUID) (*generator.AIGenerationLog, error) {
	query := `
		SELECT id, user_id, user_type, input_prompt, system_prompt, raw_response,
			status, error_message, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		FROM ai_generation_logs
		WHERE id = $1
	`

	log := &generator.AIGenerationLog{}
	err := q.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID,
		&log.UserID,
		&log.UserType,
		&log.InputPrompt,
		&log.SystemPrompt,
		&log.RawResponse,
		&log.Status,
		&log.ErrorMessage,
		&log.InputTokens,
		&log.OutputTokens,
		&log.CostUSD,
		&log.DurationMS,
		&log.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get AI generation log: %w", err)
	}

	return log, nil
}

// GetGenerationLogsByUser retrieves AI generation logs for a specific user
func (q *Queries) GetGenerationLogsByUser(ctx context.Context, userID string, limit, offset int) ([]*generator.AIGenerationLog, error) {
	query := `
		SELECT id, user_id, user_type, input_prompt, system_prompt, raw_response,
			status, error_message, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		FROM ai_generation_logs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := q.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query AI generation logs by user: %w", err)
	}
	defer rows.Close()

	var logs []*generator.AIGenerationLog
	for rows.Next() {
		log := &generator.AIGenerationLog{}
		err := rows.Scan(
			&log.ID,
			&log.UserID,
			&log.UserType,
			&log.InputPrompt,
			&log.SystemPrompt,
			&log.RawResponse,
			&log.Status,
			&log.ErrorMessage,
			&log.InputTokens,
			&log.OutputTokens,
			&log.CostUSD,
			&log.DurationMS,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan AI generation log: %w", err)
		}
		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating AI generation logs: %w", err)
	}

	return logs, nil
}

// GetGenerationLogsByStatus retrieves AI generation logs by status
func (q *Queries) GetGenerationLogsByStatus(ctx context.Context, status string, limit, offset int) ([]*generator.AIGenerationLog, error) {
	query := `
		SELECT id, user_id, user_type, input_prompt, system_prompt, raw_response,
			status, error_message, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		FROM ai_generation_logs
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := q.db.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query AI generation logs by status: %w", err)
	}
	defer rows.Close()

	var logs []*generator.AIGenerationLog
	for rows.Next() {
		log := &generator.AIGenerationLog{}
		err := rows.Scan(
			&log.ID,
			&log.UserID,
			&log.UserType,
			&log.InputPrompt,
			&log.SystemPrompt,
			&log.RawResponse,
			&log.Status,
			&log.ErrorMessage,
			&log.InputTokens,
			&log.OutputTokens,
			&log.CostUSD,
			&log.DurationMS,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan AI generation log: %w", err)
		}
		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating AI generation logs: %w", err)
	}

	return logs, nil
}

// GetRecentGenerationLogs retrieves recent AI generation logs
func (q *Queries) GetRecentGenerationLogs(ctx context.Context, limit, offset int) ([]*generator.AIGenerationLog, error) {
	query := `
		SELECT id, user_id, user_type, input_prompt, system_prompt, raw_response,
			status, error_message, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		FROM ai_generation_logs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := q.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent AI generation logs: %w", err)
	}
	defer rows.Close()

	var logs []*generator.AIGenerationLog
	for rows.Next() {
		log := &generator.AIGenerationLog{}
		err := rows.Scan(
			&log.ID,
			&log.UserID,
			&log.UserType,
			&log.InputPrompt,
			&log.SystemPrompt,
			&log.RawResponse,
			&log.Status,
			&log.ErrorMessage,
			&log.InputTokens,
			&log.OutputTokens,
			&log.CostUSD,
			&log.DurationMS,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan AI generation log: %w", err)
		}
		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating AI generation logs: %w", err)
	}

	return logs, nil
}
