package consumer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/openmeet-team/survey/internal/db"
)

// GetCursor retrieves the current Jetstream cursor value
func GetCursor(ctx context.Context, q *db.Queries) (int64, error) {
	query := `SELECT time_us FROM jetstream_cursor WHERE id = 1`

	var timeUs int64
	err := q.GetDB().QueryRowContext(ctx, query).Scan(&timeUs)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("cursor row not found (id=1 should exist)")
		}
		return 0, fmt.Errorf("failed to get cursor: %w", err)
	}

	return timeUs, nil
}

// UpdateCursor updates the Jetstream cursor to the given value
func UpdateCursor(ctx context.Context, q *db.Queries, timeUs int64) error {
	query := `UPDATE jetstream_cursor SET time_us = $1, updated_at = NOW() WHERE id = 1`

	result, err := q.GetDB().ExecContext(ctx, query, timeUs)
	if err != nil {
		return fmt.Errorf("failed to update cursor: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("cursor row not found (expected id=1)")
	}

	return nil
}
