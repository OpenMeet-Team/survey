package consumer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openmeet-team/survey/internal/db"
	"github.com/openmeet-team/survey/internal/models"
	"github.com/openmeet-team/survey/internal/telemetry"
)

// JetstreamMessage represents a message from the Jetstream firehose
type JetstreamMessage struct {
	Did    string           `json:"did,omitempty"`
	TimeUs int64            `json:"time_us"`
	Kind   string           `json:"kind"`
	Commit *JetstreamCommit `json:"commit,omitempty"`
}

// JetstreamCommit represents the commit portion of a Jetstream message
type JetstreamCommit struct {
	Rev        string                 `json:"rev,omitempty"`
	Operation  string                 `json:"operation"` // create, update, delete
	Collection string                 `json:"collection"`
	RKey       string                 `json:"rkey"`
	Record     map[string]interface{} `json:"record,omitempty"` // Present for create/update
	CID        string                 `json:"cid,omitempty"`    // Present for create/update
	Repo       string                 `json:"repo"`             // DID of the repo owner
}

// Processor handles processing of Jetstream messages
type Processor struct {
	queries *db.Queries
}

// NewProcessor creates a new Processor instance
func NewProcessor(queries *db.Queries) *Processor {
	return &Processor{
		queries: queries,
	}
}

// ProcessMessage processes a single Jetstream message
func (p *Processor) ProcessMessage(ctx context.Context, msg *JetstreamMessage) error {
	// Filter for commit messages only
	if msg.Kind != "commit" || msg.Commit == nil {
		return nil // Skip non-commit messages
	}

	// Route to appropriate handler based on collection
	switch msg.Commit.Collection {
	case "net.openmeet.survey":
		return p.processSurveyCommit(ctx, msg)
	case "net.openmeet.survey.response":
		return p.processResponseCommit(ctx, msg)
	case "net.openmeet.survey.results":
		return p.processResultsCommit(ctx, msg)
	default:
		return nil // Skip other collections
	}
}

// processSurveyCommit handles create/update/delete operations for surveys
func (p *Processor) processSurveyCommit(ctx context.Context, msg *JetstreamMessage) error {
	commit := msg.Commit

	switch commit.Operation {
	case "create":
		return p.createSurvey(ctx, commit)
	case "update":
		return p.updateSurvey(ctx, commit)
	case "delete":
		return p.deleteSurvey(ctx, commit)
	default:
		return nil // Skip unknown operations
	}
}

// createSurvey indexes a new survey from ATProto
func (p *Processor) createSurvey(ctx context.Context, commit *JetstreamCommit) error {
	if commit.Record == nil {
		return fmt.Errorf("create operation missing record")
	}

	// Parse the survey record
	def, name, description, err := ParseSurveyRecord(commit.Record)
	if err != nil {
		return fmt.Errorf("failed to parse survey record: %w", err)
	}

	// Validate the survey definition
	if err := def.ValidateDefinition(); err != nil {
		return fmt.Errorf("invalid survey definition: %w", err)
	}

	// Generate slug from name
	baseSlug := GenerateSlugFromTitle(name)
	slug := baseSlug

	// Handle slug collisions by appending -2, -3, etc.
	suffix := 2
	for {
		exists, err := p.queries.SlugExists(ctx, slug)
		if err != nil {
			return fmt.Errorf("failed to check slug existence: %w", err)
		}
		if !exists {
			break
		}
		slug = fmt.Sprintf("%s-%d", baseSlug, suffix)
		suffix++

		// Safety limit to prevent infinite loop
		if suffix > 100 {
			return fmt.Errorf("too many slug collisions for %s", baseSlug)
		}
	}

	// Construct record URI
	uri := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Create the survey
	survey := &models.Survey{
		ID:          uuid.New(),
		URI:         &uri,
		CID:         &commit.CID,
		AuthorDID:   &commit.Repo,
		Slug:        slug,
		Title:       name,
		Description: &description,
		Definition:  *def,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// TODO: Parse startsAt/endsAt from record if present

	if err := p.queries.CreateSurvey(ctx, survey); err != nil {
		return fmt.Errorf("failed to create survey: %w", err)
	}

	// Record business metrics
	telemetry.SurveysIndexed.Inc()
	telemetry.SurveyQuestionCount.Observe(float64(len(def.Questions)))
	for _, q := range def.Questions {
		telemetry.SurveyQuestionTypes.WithLabelValues(string(q.Type)).Inc()
	}

	return nil
}

// updateSurvey updates an existing indexed survey
func (p *Processor) updateSurvey(ctx context.Context, commit *JetstreamCommit) error {
	if commit.Record == nil {
		return fmt.Errorf("update operation missing record")
	}

	// Construct record URI
	uri := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Look up existing survey
	survey, err := p.queries.GetSurveyByURI(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to get survey by URI: %w", err)
	}
	if survey == nil {
		// Survey doesn't exist in our index - treat as create
		return p.createSurvey(ctx, commit)
	}

	// Authorization check: verify the update comes from the survey author
	if survey.AuthorDID != nil && *survey.AuthorDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot update survey owned by %s", commit.Repo, *survey.AuthorDID)
	}

	// Parse the updated survey record
	def, name, description, err := ParseSurveyRecord(commit.Record)
	if err != nil {
		return fmt.Errorf("failed to parse survey record: %w", err)
	}

	// Validate the survey definition
	if err := def.ValidateDefinition(); err != nil {
		return fmt.Errorf("invalid survey definition: %w", err)
	}

	// Update the survey (keep existing slug, ID, etc.)
	survey.CID = &commit.CID
	survey.Title = name
	survey.Description = &description
	survey.Definition = *def

	if err := p.queries.UpdateSurvey(ctx, survey); err != nil {
		return fmt.Errorf("failed to update survey: %w", err)
	}

	return nil
}

// deleteSurvey removes a survey from the index
func (p *Processor) deleteSurvey(ctx context.Context, commit *JetstreamCommit) error {
	// Construct record URI
	uri := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Look up existing survey for authorization check
	survey, err := p.queries.GetSurveyByURI(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to get survey by URI: %w", err)
	}
	if survey == nil {
		// Survey doesn't exist - nothing to delete (idempotent)
		return nil
	}

	// Authorization check: verify the delete comes from the survey author
	if survey.AuthorDID != nil && *survey.AuthorDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot delete survey owned by %s", commit.Repo, *survey.AuthorDID)
	}

	// Delete the survey (cascades to responses due to ON DELETE CASCADE)
	if err := p.queries.DeleteSurveyByURI(ctx, uri); err != nil {
		return fmt.Errorf("failed to delete survey: %w", err)
	}

	return nil
}

// processResponseCommit handles create/update/delete operations for survey responses
func (p *Processor) processResponseCommit(ctx context.Context, msg *JetstreamMessage) error {
	commit := msg.Commit

	switch commit.Operation {
	case "create":
		return p.createResponse(ctx, commit)
	case "update":
		return p.updateResponse(ctx, commit)
	case "delete":
		return p.deleteResponse(ctx, commit)
	default:
		return nil // Skip unknown operations
	}
}

// createResponse indexes a new survey response from ATProto
func (p *Processor) createResponse(ctx context.Context, commit *JetstreamCommit) error {
	if commit.Record == nil {
		return fmt.Errorf("create operation missing record")
	}

	// Parse the response record
	surveyURI, answers, err := ParseResponseRecord(commit.Record)
	if err != nil {
		return fmt.Errorf("failed to parse response record: %w", err)
	}

	// Look up the survey by URI
	survey, err := p.queries.GetSurveyByURI(ctx, surveyURI)
	if err != nil {
		return fmt.Errorf("failed to get survey by URI %s: %w", surveyURI, err)
	}
	if survey == nil {
		return fmt.Errorf("survey not found: %s", surveyURI)
	}

	// Validate answers against survey definition
	if err := models.ValidateAnswers(&survey.Definition, answers); err != nil {
		return fmt.Errorf("answer validation failed: %w", err)
	}

	// Extract voter DID from commit.repo
	voterDID := commit.Repo

	// Construct record URI
	recordURI := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Check for duplicate response (user already voted on this survey)
	existing, err := p.queries.GetResponseBySurveyAndVoter(ctx, survey.ID, voterDID, "")
	if err != nil {
		return fmt.Errorf("failed to check for existing response: %w", err)
	}
	if existing != nil {
		// User already voted - this is a duplicate, skip it
		return nil
	}

	// Create the response
	response := &models.Response{
		ID:        uuid.New(),
		SurveyID:  survey.ID,
		VoterDID:  &voterDID,
		RecordURI: &recordURI,
		RecordCID: &commit.CID,
		Answers:   answers,
		CreatedAt: time.Now(),
	}

	if err := p.queries.CreateResponse(ctx, response); err != nil {
		return fmt.Errorf("failed to create response: %w", err)
	}

	// Record business metrics
	telemetry.VotesIndexed.Inc()

	// Get current vote count for this survey to track distribution
	voteCount, err := p.queries.CountResponsesBySurvey(ctx, survey.ID)
	if err == nil {
		telemetry.VotesPerSurvey.Observe(float64(voteCount))
	}

	return nil
}

// updateResponse updates an existing indexed response
func (p *Processor) updateResponse(ctx context.Context, commit *JetstreamCommit) error {
	if commit.Record == nil {
		return fmt.Errorf("update operation missing record")
	}

	// Construct record URI
	recordURI := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Look up existing response
	response, err := p.queries.GetResponseByRecordURI(ctx, recordURI)
	if err != nil {
		return fmt.Errorf("failed to get response by URI: %w", err)
	}
	if response == nil {
		// Response doesn't exist in our index - treat as create
		return p.createResponse(ctx, commit)
	}

	// Authorization check: verify the update comes from the original voter
	if response.VoterDID != nil && *response.VoterDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot update response owned by %s", commit.Repo, *response.VoterDID)
	}

	// Parse the updated response record
	surveyURI, answers, err := ParseResponseRecord(commit.Record)
	if err != nil {
		return fmt.Errorf("failed to parse response record: %w", err)
	}

	// Get the survey to validate answers
	survey, err := p.queries.GetSurveyByURI(ctx, surveyURI)
	if err != nil {
		return fmt.Errorf("failed to get survey by URI %s: %w", surveyURI, err)
	}
	if survey == nil {
		return fmt.Errorf("survey not found: %s", surveyURI)
	}

	// Validate answers
	if err := models.ValidateAnswers(&survey.Definition, answers); err != nil {
		return fmt.Errorf("answer validation failed: %w", err)
	}

	// Update the response
	if err := p.queries.UpdateResponseAnswers(ctx, response.ID, answers, commit.CID); err != nil {
		return fmt.Errorf("failed to update response: %w", err)
	}

	return nil
}

// deleteResponse removes a response from the index
func (p *Processor) deleteResponse(ctx context.Context, commit *JetstreamCommit) error {
	// Construct record URI
	recordURI := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Look up existing response for authorization check
	response, err := p.queries.GetResponseByRecordURI(ctx, recordURI)
	if err != nil {
		return fmt.Errorf("failed to get response by URI: %w", err)
	}
	if response == nil {
		// Response doesn't exist - nothing to delete (idempotent)
		return nil
	}

	// Authorization check: verify the delete comes from the original voter
	if response.VoterDID != nil && *response.VoterDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot delete response owned by %s", commit.Repo, *response.VoterDID)
	}

	// Delete the response
	if err := p.queries.DeleteResponseByRecordURI(ctx, recordURI); err != nil {
		return fmt.Errorf("failed to delete response: %w", err)
	}

	return nil
}

// processResultsCommit handles create/update/delete operations for survey results
func (p *Processor) processResultsCommit(ctx context.Context, msg *JetstreamMessage) error {
	commit := msg.Commit

	switch commit.Operation {
	case "create":
		return p.createResults(ctx, commit)
	case "update":
		return p.updateResults(ctx, commit)
	case "delete":
		return p.deleteResults(ctx, commit)
	default:
		return nil // Skip unknown operations
	}
}

// createResults indexes a new results record from ATProto
func (p *Processor) createResults(ctx context.Context, commit *JetstreamCommit) error {
	if commit.Record == nil {
		return fmt.Errorf("create operation missing record")
	}

	// Parse the results record to get the survey URI
	surveyURI, err := ParseResultsRecord(commit.Record)
	if err != nil {
		return fmt.Errorf("failed to parse results record: %w", err)
	}

	// Look up the survey
	survey, err := p.queries.GetSurveyByURI(ctx, surveyURI)
	if err != nil {
		return fmt.Errorf("failed to get survey by URI %s: %w", surveyURI, err)
	}
	if survey == nil {
		return fmt.Errorf("survey not found: %s", surveyURI)
	}

	// Authorization check: verify the results publish comes from the survey author
	if survey.AuthorDID != nil && *survey.AuthorDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot publish results for survey owned by %s", commit.Repo, *survey.AuthorDID)
	}

	// Construct results record URI
	resultsURI := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Update the survey with results URI/CID
	if err := p.queries.UpdateSurveyResults(ctx, survey.ID, resultsURI, commit.CID); err != nil {
		return fmt.Errorf("failed to update survey results: %w", err)
	}

	// Record business metric
	telemetry.ResultsPublished.Inc()

	return nil
}

// updateResults updates an existing results record
func (p *Processor) updateResults(ctx context.Context, commit *JetstreamCommit) error {
	if commit.Record == nil {
		return fmt.Errorf("update operation missing record")
	}

	// Construct results record URI
	resultsURI := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Look up existing survey by results URI
	survey, err := p.queries.GetSurveyByResultsURI(ctx, resultsURI)
	if err != nil {
		return fmt.Errorf("failed to get survey by results URI: %w", err)
	}
	if survey == nil {
		// Results record doesn't exist in our index - treat as create
		return p.createResults(ctx, commit)
	}

	// Authorization check: verify the update comes from the survey author
	if survey.AuthorDID != nil && *survey.AuthorDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot update results for survey owned by %s", commit.Repo, *survey.AuthorDID)
	}

	// Parse the results record to verify it still references the same survey
	surveyURI, err := ParseResultsRecord(commit.Record)
	if err != nil {
		return fmt.Errorf("failed to parse results record: %w", err)
	}

	if survey.URI != nil && *survey.URI != surveyURI {
		return fmt.Errorf("results record cannot change survey reference")
	}

	// Update the CID
	if err := p.queries.UpdateSurveyResults(ctx, survey.ID, resultsURI, commit.CID); err != nil {
		return fmt.Errorf("failed to update survey results: %w", err)
	}

	return nil
}

// deleteResults removes results from a survey
func (p *Processor) deleteResults(ctx context.Context, commit *JetstreamCommit) error {
	// Construct results record URI
	resultsURI := fmt.Sprintf("at://%s/%s/%s", commit.Repo, commit.Collection, commit.RKey)

	// Look up existing survey by results URI
	survey, err := p.queries.GetSurveyByResultsURI(ctx, resultsURI)
	if err != nil {
		return fmt.Errorf("failed to get survey by results URI: %w", err)
	}
	if survey == nil {
		// Results don't exist - nothing to delete (idempotent)
		return nil
	}

	// Authorization check: verify the delete comes from the survey author
	if survey.AuthorDID != nil && *survey.AuthorDID != commit.Repo {
		return fmt.Errorf("unauthorized: DID %s cannot delete results for survey owned by %s", commit.Repo, *survey.AuthorDID)
	}

	// Clear the results URI/CID from the survey (set to NULL)
	query := `UPDATE surveys SET results_uri = NULL, results_cid = NULL, updated_at = NOW() WHERE id = $1`
	_, err = p.queries.GetDB().ExecContext(ctx, query, survey.ID)
	if err != nil {
		return fmt.Errorf("failed to clear survey results: %w", err)
	}

	return nil
}

// ProcessMessageWithCursor processes a message and updates the cursor atomically
func (p *Processor) ProcessMessageWithCursor(ctx context.Context, msg *JetstreamMessage, getDB func() db.Querier) error {
	// Start a transaction
	dbConn, ok := p.queries.GetDB().(*sql.DB)
	if !ok {
		// If we're already in a transaction, just process the message
		if err := p.ProcessMessage(ctx, msg); err != nil {
			return fmt.Errorf("failed to process message: %w", err)
		}
		return UpdateCursor(ctx, p.queries, msg.TimeUs)
	}

	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create transaction-scoped processor
	txQueries := db.NewQueries(tx)
	txProcessor := NewProcessor(txQueries)

	// Process the message
	if err := txProcessor.ProcessMessage(ctx, msg); err != nil {
		return fmt.Errorf("failed to process message: %w", err)
	}

	// Update cursor
	if err := UpdateCursor(ctx, txQueries, msg.TimeUs); err != nil {
		return fmt.Errorf("failed to update cursor: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
