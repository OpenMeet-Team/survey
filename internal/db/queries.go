package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/openmeet-team/survey/internal/models"
)

// Querier interface represents a database connection or transaction
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// Queries provides database query methods
type Queries struct {
	db Querier
}

// NewQueries creates a new Queries instance
func NewQueries(db Querier) *Queries {
	return &Queries{db: db}
}

// GetDB returns the underlying database connection
func (q *Queries) GetDB() Querier {
	return q.db
}

// Survey Queries

// CreateSurvey inserts a new survey into the database
func (q *Queries) CreateSurvey(ctx context.Context, s *models.Survey) error {
	// Marshal definition to JSON for JSONB storage
	defJSON, err := json.Marshal(s.Definition)
	if err != nil {
		return fmt.Errorf("failed to marshal survey definition: %w", err)
	}

	query := `
		INSERT INTO surveys (id, uri, cid, author_did, slug, title, description, definition, starts_at, ends_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = q.db.ExecContext(
		ctx,
		query,
		s.ID,
		s.URI,
		s.CID,
		s.AuthorDID,
		s.Slug,
		s.Title,
		s.Description,
		defJSON,
		s.StartsAt,
		s.EndsAt,
		s.CreatedAt,
		s.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert survey: %w", err)
	}

	return nil
}

// GetSurveyByURI retrieves a survey by its ATProto URI
func (q *Queries) GetSurveyByURI(ctx context.Context, uri string) (*models.Survey, error) {
	query := `
		SELECT id, uri, cid, author_did, slug, title, description, definition, starts_at, ends_at, results_uri, results_cid, created_at, updated_at
		FROM surveys
		WHERE uri = $1
	`

	survey := &models.Survey{}
	var defJSON []byte

	err := q.db.QueryRowContext(ctx, query, uri).Scan(
		&survey.ID,
		&survey.URI,
		&survey.CID,
		&survey.AuthorDID,
		&survey.Slug,
		&survey.Title,
		&survey.Description,
		&defJSON,
		&survey.StartsAt,
		&survey.EndsAt,
		&survey.ResultsURI,
		&survey.ResultsCID,
		&survey.CreatedAt,
		&survey.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("survey not found: %w", err)
		}
		return nil, fmt.Errorf("failed to query survey: %w", err)
	}

	// Unmarshal JSONB definition
	if err := json.Unmarshal(defJSON, &survey.Definition); err != nil {
		return nil, fmt.Errorf("failed to unmarshal survey definition: %w", err)
	}

	return survey, nil
}

// GetSurveyBySlug retrieves a survey by its slug
func (q *Queries) GetSurveyBySlug(ctx context.Context, slug string) (*models.Survey, error) {
	query := `
		SELECT id, uri, cid, author_did, slug, title, description, definition, starts_at, ends_at, results_uri, results_cid, created_at, updated_at
		FROM surveys
		WHERE slug = $1
	`

	survey := &models.Survey{}
	var defJSON []byte

	err := q.db.QueryRowContext(ctx, query, slug).Scan(
		&survey.ID,
		&survey.URI,
		&survey.CID,
		&survey.AuthorDID,
		&survey.Slug,
		&survey.Title,
		&survey.Description,
		&defJSON,
		&survey.StartsAt,
		&survey.EndsAt,
		&survey.ResultsURI,
		&survey.ResultsCID,
		&survey.CreatedAt,
		&survey.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("survey not found: %w", err)
		}
		return nil, fmt.Errorf("failed to query survey: %w", err)
	}

	// Unmarshal JSONB definition
	if err := json.Unmarshal(defJSON, &survey.Definition); err != nil {
		return nil, fmt.Errorf("failed to unmarshal survey definition: %w", err)
	}

	return survey, nil
}

// GetSurveyByID retrieves a survey by its ID
func (q *Queries) GetSurveyByID(ctx context.Context, id uuid.UUID) (*models.Survey, error) {
	query := `
		SELECT id, uri, cid, author_did, slug, title, description, definition, starts_at, ends_at, results_uri, results_cid, created_at, updated_at
		FROM surveys
		WHERE id = $1
	`

	survey := &models.Survey{}
	var defJSON []byte

	err := q.db.QueryRowContext(ctx, query, id).Scan(
		&survey.ID,
		&survey.URI,
		&survey.CID,
		&survey.AuthorDID,
		&survey.Slug,
		&survey.Title,
		&survey.Description,
		&defJSON,
		&survey.StartsAt,
		&survey.EndsAt,
		&survey.ResultsURI,
		&survey.ResultsCID,
		&survey.CreatedAt,
		&survey.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("survey not found: %w", err)
		}
		return nil, fmt.Errorf("failed to query survey: %w", err)
	}

	// Unmarshal JSONB definition
	if err := json.Unmarshal(defJSON, &survey.Definition); err != nil {
		return nil, fmt.Errorf("failed to unmarshal survey definition: %w", err)
	}

	return survey, nil
}

// ListSurveys retrieves surveys with pagination
func (q *Queries) ListSurveys(ctx context.Context, limit, offset int) ([]*models.Survey, error) {
	query := `
		SELECT id, uri, cid, author_did, slug, title, description, definition, starts_at, ends_at, results_uri, results_cid, created_at, updated_at
		FROM surveys
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := q.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query surveys: %w", err)
	}
	defer rows.Close()

	var surveys []*models.Survey
	for rows.Next() {
		survey := &models.Survey{}
		var defJSON []byte

		err := rows.Scan(
			&survey.ID,
			&survey.URI,
			&survey.CID,
			&survey.AuthorDID,
			&survey.Slug,
			&survey.Title,
			&survey.Description,
			&defJSON,
			&survey.StartsAt,
			&survey.EndsAt,
			&survey.ResultsURI,
			&survey.ResultsCID,
			&survey.CreatedAt,
			&survey.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan survey: %w", err)
		}

		// Unmarshal JSONB definition
		if err := json.Unmarshal(defJSON, &survey.Definition); err != nil {
			return nil, fmt.Errorf("failed to unmarshal survey definition: %w", err)
		}

		surveys = append(surveys, survey)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating surveys: %w", err)
	}

	return surveys, nil
}

// SlugExists checks if a survey slug already exists
func (q *Queries) SlugExists(ctx context.Context, slug string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM surveys WHERE slug = $1)`

	var exists bool
	err := q.db.QueryRowContext(ctx, query, slug).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check slug existence: %w", err)
	}

	return exists, nil
}

// UpdateSurvey updates an existing survey and sets updated_at
func (q *Queries) UpdateSurvey(ctx context.Context, s *models.Survey) error {
	// Marshal definition to JSON for JSONB storage
	defJSON, err := json.Marshal(s.Definition)
	if err != nil {
		return fmt.Errorf("failed to marshal survey definition: %w", err)
	}

	query := `
		UPDATE surveys
		SET uri = $2, cid = $3, author_did = $4, slug = $5, title = $6,
		    description = $7, definition = $8, starts_at = $9, ends_at = $10,
		    updated_at = NOW()
		WHERE id = $1
	`

	result, err := q.db.ExecContext(
		ctx,
		query,
		s.ID,
		s.URI,
		s.CID,
		s.AuthorDID,
		s.Slug,
		s.Title,
		s.Description,
		defJSON,
		s.StartsAt,
		s.EndsAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update survey: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("survey not found")
	}

	return nil
}

// Response Queries

// CreateResponse inserts a new response into the database
func (q *Queries) CreateResponse(ctx context.Context, r *models.Response) error {
	// Marshal answers to JSON for JSONB storage
	answersJSON, err := json.Marshal(r.Answers)
	if err != nil {
		return fmt.Errorf("failed to marshal response answers: %w", err)
	}

	query := `
		INSERT INTO responses (id, survey_id, voter_did, voter_session, record_uri, record_cid, answers, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = q.db.ExecContext(
		ctx,
		query,
		r.ID,
		r.SurveyID,
		r.VoterDID,
		r.VoterSession,
		r.RecordURI,
		r.RecordCID,
		answersJSON,
		r.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert response: %w", err)
	}

	return nil
}

// GetResponseByID retrieves a response by its ID
func (q *Queries) GetResponseByID(ctx context.Context, id uuid.UUID) (*models.Response, error) {
	query := `
		SELECT id, survey_id, voter_did, voter_session, record_uri, record_cid, answers, created_at
		FROM responses
		WHERE id = $1
	`

	response := &models.Response{}
	var answersJSON []byte

	err := q.db.QueryRowContext(ctx, query, id).Scan(
		&response.ID,
		&response.SurveyID,
		&response.VoterDID,
		&response.VoterSession,
		&response.RecordURI,
		&response.RecordCID,
		&answersJSON,
		&response.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("response not found: %w", err)
		}
		return nil, fmt.Errorf("failed to query response: %w", err)
	}

	// Unmarshal JSONB answers
	if err := json.Unmarshal(answersJSON, &response.Answers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response answers: %w", err)
	}

	return response, nil
}

// GetResponseBySurveyAndVoter retrieves an existing response for a voter on a survey
func (q *Queries) GetResponseBySurveyAndVoter(ctx context.Context, surveyID uuid.UUID, voterDID, voterSession string) (*models.Response, error) {
	var query string
	var args []interface{}

	if voterDID != "" {
		query = `
			SELECT id, survey_id, voter_did, voter_session, record_uri, record_cid, answers, created_at
			FROM responses
			WHERE survey_id = $1 AND voter_did = $2
		`
		args = []interface{}{surveyID, voterDID}
	} else if voterSession != "" {
		query = `
			SELECT id, survey_id, voter_did, voter_session, record_uri, record_cid, answers, created_at
			FROM responses
			WHERE survey_id = $1 AND voter_session = $2
		`
		args = []interface{}{surveyID, voterSession}
	} else {
		return nil, fmt.Errorf("either voterDID or voterSession must be provided")
	}

	response := &models.Response{}
	var answersJSON []byte

	err := q.db.QueryRowContext(ctx, query, args...).Scan(
		&response.ID,
		&response.SurveyID,
		&response.VoterDID,
		&response.VoterSession,
		&response.RecordURI,
		&response.RecordCID,
		&answersJSON,
		&response.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No existing response is not an error
		}
		return nil, fmt.Errorf("failed to query response: %w", err)
	}

	// Unmarshal JSONB answers
	if err := json.Unmarshal(answersJSON, &response.Answers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response answers: %w", err)
	}

	return response, nil
}

// ListResponsesBySurvey retrieves all responses for a survey
func (q *Queries) ListResponsesBySurvey(ctx context.Context, surveyID uuid.UUID) ([]*models.Response, error) {
	query := `
		SELECT id, survey_id, voter_did, voter_session, record_uri, record_cid, answers, created_at
		FROM responses
		WHERE survey_id = $1
		ORDER BY created_at ASC
	`

	rows, err := q.db.QueryContext(ctx, query, surveyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query responses: %w", err)
	}
	defer rows.Close()

	var responses []*models.Response
	for rows.Next() {
		response := &models.Response{}
		var answersJSON []byte

		err := rows.Scan(
			&response.ID,
			&response.SurveyID,
			&response.VoterDID,
			&response.VoterSession,
			&response.RecordURI,
			&response.RecordCID,
			&answersJSON,
			&response.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan response: %w", err)
		}

		// Unmarshal JSONB answers
		if err := json.Unmarshal(answersJSON, &response.Answers); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response answers: %w", err)
		}

		responses = append(responses, response)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating responses: %w", err)
	}

	return responses, nil
}

// CountResponsesBySurvey counts the number of responses for a survey
func (q *Queries) CountResponsesBySurvey(ctx context.Context, surveyID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM responses WHERE survey_id = $1`

	var count int
	err := q.db.QueryRowContext(ctx, query, surveyID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count responses: %w", err)
	}

	return count, nil
}

// GetResponseByRecordURI retrieves a response by its ATProto record URI
func (q *Queries) GetResponseByRecordURI(ctx context.Context, recordURI string) (*models.Response, error) {
	query := `
		SELECT id, survey_id, voter_did, voter_session, record_uri, record_cid, answers, created_at
		FROM responses
		WHERE record_uri = $1
	`

	response := &models.Response{}
	var answersJSON []byte

	err := q.db.QueryRowContext(ctx, query, recordURI).Scan(
		&response.ID,
		&response.SurveyID,
		&response.VoterDID,
		&response.VoterSession,
		&response.RecordURI,
		&response.RecordCID,
		&answersJSON,
		&response.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found is not an error for this query
		}
		return nil, fmt.Errorf("failed to query response: %w", err)
	}

	// Unmarshal JSONB answers
	if err := json.Unmarshal(answersJSON, &response.Answers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response answers: %w", err)
	}

	return response, nil
}

// UpdateResponseAnswers updates the answers for an existing response
func (q *Queries) UpdateResponseAnswers(ctx context.Context, id uuid.UUID, answers map[string]models.Answer, cid string) error {
	answersJSON, err := json.Marshal(answers)
	if err != nil {
		return fmt.Errorf("failed to marshal answers: %w", err)
	}

	query := `
		UPDATE responses
		SET answers = $2, record_cid = $3
		WHERE id = $1
	`

	result, err := q.db.ExecContext(ctx, query, id, answersJSON, cid)
	if err != nil {
		return fmt.Errorf("failed to update response: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("response not found")
	}

	return nil
}

// DeleteResponseByRecordURI deletes a response by its ATProto record URI
func (q *Queries) DeleteResponseByRecordURI(ctx context.Context, recordURI string) error {
	query := `DELETE FROM responses WHERE record_uri = $1`

	result, err := q.db.ExecContext(ctx, query, recordURI)
	if err != nil {
		return fmt.Errorf("failed to delete response: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Not an error if response doesn't exist
	if rows == 0 {
		return nil
	}

	return nil
}

// DeleteSurveyByURI deletes a survey by its ATProto URI
func (q *Queries) DeleteSurveyByURI(ctx context.Context, uri string) error {
	query := `DELETE FROM surveys WHERE uri = $1`

	result, err := q.db.ExecContext(ctx, query, uri)
	if err != nil {
		return fmt.Errorf("failed to delete survey: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Not an error if survey doesn't exist
	if rows == 0 {
		return nil
	}

	return nil
}

// Results Aggregation

// GetSurveyResults aggregates all responses for a survey into results
func (q *Queries) GetSurveyResults(ctx context.Context, surveyID uuid.UUID) (*models.SurveyResults, error) {
	// First, get the survey to understand question structure
	survey, err := q.GetSurveyByID(ctx, surveyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get survey: %w", err)
	}

	// Get all responses
	responses, err := q.ListResponsesBySurvey(ctx, surveyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get responses: %w", err)
	}

	// Initialize results structure
	results := &models.SurveyResults{
		SurveyID:        surveyID,
		TotalVotes:      len(responses),
		QuestionResults: make(map[string]*models.QuestionResult),
	}

	// Initialize question results based on survey definition
	for _, question := range survey.Definition.Questions {
		results.QuestionResults[question.ID] = &models.QuestionResult{
			QuestionID:   question.ID,
			OptionCounts: make(map[string]int),
			TextAnswers:  []string{},
		}
	}

	// Aggregate responses
	for _, response := range responses {
		for questionID, answer := range response.Answers {
			qResult, exists := results.QuestionResults[questionID]
			if !exists {
				continue // Skip answers for questions that no longer exist
			}

			// Count selected options
			for _, optionID := range answer.SelectedOptions {
				qResult.OptionCounts[optionID]++
			}

			// Collect text answers
			if answer.Text != "" {
				qResult.TextAnswers = append(qResult.TextAnswers, answer.Text)
			}
		}
	}

	return results, nil
}

// UpdateSurveyResults updates the results URI and CID for a survey
func (q *Queries) UpdateSurveyResults(ctx context.Context, surveyID uuid.UUID, resultsURI, resultsCID string) error {
	query := `
		UPDATE surveys
		SET results_uri = $2, results_cid = $3, updated_at = NOW()
		WHERE id = $1
	`

	result, err := q.db.ExecContext(ctx, query, surveyID, resultsURI, resultsCID)
	if err != nil {
		return fmt.Errorf("failed to update survey results: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("survey not found")
	}

	return nil
}

// GetSurveyByResultsURI retrieves a survey by its results URI
func (q *Queries) GetSurveyByResultsURI(ctx context.Context, resultsURI string) (*models.Survey, error) {
	query := `
		SELECT id, uri, cid, author_did, slug, title, description, definition, starts_at, ends_at, results_uri, results_cid, created_at, updated_at
		FROM surveys
		WHERE results_uri = $1
	`

	survey := &models.Survey{}
	var defJSON []byte

	err := q.db.QueryRowContext(ctx, query, resultsURI).Scan(
		&survey.ID,
		&survey.URI,
		&survey.CID,
		&survey.AuthorDID,
		&survey.Slug,
		&survey.Title,
		&survey.Description,
		&defJSON,
		&survey.StartsAt,
		&survey.EndsAt,
		&survey.ResultsURI,
		&survey.ResultsCID,
		&survey.CreatedAt,
		&survey.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found is not an error for this query
		}
		return nil, fmt.Errorf("failed to query survey: %w", err)
	}

	// Unmarshal JSONB definition
	if err := json.Unmarshal(defJSON, &survey.Definition); err != nil {
		return nil, fmt.Errorf("failed to unmarshal survey definition: %w", err)
	}

	return survey, nil
}
