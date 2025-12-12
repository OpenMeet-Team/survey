package consumer

import (
	"context"
	"database/sql"
	"testing"

	"github.com/openmeet-team/survey/internal/db"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) (*sql.DB, *db.Queries) {
	t.Helper()

	// Use test database connection string
	// This assumes a test database is available - in real CI, use testcontainers
	connStr := "postgres://postgres:postgres@localhost:5432/survey_test?sslmode=disable"
	database, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Skipf("Skipping test - cannot connect to test database: %v", err)
	}

	// Verify connection
	if err := database.Ping(); err != nil {
		t.Skipf("Skipping test - cannot ping test database: %v", err)
	}

	// Reset cursor to 0
	_, err = database.Exec("UPDATE jetstream_cursor SET time_us = 0, updated_at = NOW() WHERE id = 1")
	if err != nil {
		t.Skipf("Skipping test - cursor table not initialized: %v", err)
	}

	queries := db.NewQueries(database)
	return database, queries
}

func TestGetCursor(t *testing.T) {
	database, queries := setupTestDB(t)
	defer database.Close()

	t.Run("returns initial cursor value of 0", func(t *testing.T) {
		cursor, err := GetCursor(context.Background(), queries)
		if err != nil {
			t.Fatalf("GetCursor failed: %v", err)
		}

		if cursor != 0 {
			t.Errorf("Expected cursor to be 0, got %d", cursor)
		}
	})
}

func TestUpdateCursor(t *testing.T) {
	database, queries := setupTestDB(t)
	defer database.Close()

	t.Run("updates cursor to new value", func(t *testing.T) {
		newTimeUs := int64(1234567890123456)

		err := UpdateCursor(context.Background(), queries, newTimeUs)
		if err != nil {
			t.Fatalf("UpdateCursor failed: %v", err)
		}

		// Verify the update
		cursor, err := GetCursor(context.Background(), queries)
		if err != nil {
			t.Fatalf("GetCursor failed: %v", err)
		}

		if cursor != newTimeUs {
			t.Errorf("Expected cursor to be %d, got %d", newTimeUs, cursor)
		}
	})

	t.Run("updates cursor multiple times", func(t *testing.T) {
		values := []int64{111, 222, 333, 444, 555}

		for _, val := range values {
			err := UpdateCursor(context.Background(), queries, val)
			if err != nil {
				t.Fatalf("UpdateCursor failed for %d: %v", val, err)
			}

			cursor, err := GetCursor(context.Background(), queries)
			if err != nil {
				t.Fatalf("GetCursor failed: %v", err)
			}

			if cursor != val {
				t.Errorf("Expected cursor to be %d, got %d", val, cursor)
			}
		}
	})
}

func TestGetCursorWithTransaction(t *testing.T) {
	database, queries := setupTestDB(t)
	defer database.Close()

	t.Run("cursor update is atomic within transaction", func(t *testing.T) {
		ctx := context.Background()

		// Start transaction
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to start transaction: %v", err)
		}
		defer tx.Rollback()

		txQueries := db.NewQueries(tx)

		// Update cursor within transaction
		err = UpdateCursor(ctx, txQueries, 999999)
		if err != nil {
			t.Fatalf("UpdateCursor failed: %v", err)
		}

		// Read cursor within transaction - should see new value
		cursor, err := GetCursor(ctx, txQueries)
		if err != nil {
			t.Fatalf("GetCursor failed: %v", err)
		}

		if cursor != 999999 {
			t.Errorf("Expected cursor to be 999999, got %d", cursor)
		}

		// Rollback transaction
		tx.Rollback()

		// Read cursor outside transaction - should still be old value
		cursor, err = GetCursor(ctx, queries)
		if err != nil {
			t.Fatalf("GetCursor failed: %v", err)
		}

		// Should not be 999999 since we rolled back
		if cursor == 999999 {
			t.Error("Cursor should not be 999999 after rollback")
		}
	})
}
