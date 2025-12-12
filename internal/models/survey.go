package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// QuestionType represents the type of question
type QuestionType string

const (
	QuestionTypeSingle QuestionType = "single"
	QuestionTypeMulti  QuestionType = "multi"
	QuestionTypeText   QuestionType = "text"
)

// Survey represents a survey definition stored in the database
type Survey struct {
	ID          uuid.UUID         `db:"id" json:"id"`
	URI         *string           `db:"uri" json:"uri,omitempty"`
	CID         *string           `db:"cid" json:"cid,omitempty"`
	AuthorDID   *string           `db:"author_did" json:"authorDid,omitempty"`
	Slug        string            `db:"slug" json:"slug"`
	Title       string            `db:"title" json:"title"`
	Description *string           `db:"description" json:"description,omitempty"`
	Definition  SurveyDefinition  `db:"definition" json:"definition"`
	StartsAt    *time.Time        `db:"starts_at" json:"startsAt,omitempty"`
	EndsAt      *time.Time        `db:"ends_at" json:"endsAt,omitempty"`
	ResultsURI  *string           `db:"results_uri" json:"resultsUri,omitempty"`
	ResultsCID  *string           `db:"results_cid" json:"resultsCid,omitempty"`
	CreatedAt   time.Time         `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time         `db:"updated_at" json:"updatedAt"`
}

// SurveyDefinition represents the survey structure stored as JSONB
type SurveyDefinition struct {
	Questions []Question `json:"questions"`
	Anonymous bool       `json:"anonymous"`
}

// Question represents a survey question
type Question struct {
	ID       string       `json:"id"`
	Text     string       `json:"text"`
	Type     QuestionType `json:"type"`
	Required bool         `json:"required"`
	Options  []Option     `json:"options,omitempty"`
}

// Option represents a choice option for a question
type Option struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// ParseSurveyDefinition parses a survey definition from JSON or YAML
func ParseSurveyDefinition(data []byte) (*SurveyDefinition, error) {
	var def SurveyDefinition

	// Try JSON first
	if err := json.Unmarshal(data, &def); err == nil {
		return &def, nil
	}

	// Try YAML
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse as JSON or YAML: %w", err)
	}

	return &def, nil
}

// ValidateDefinition validates the survey definition
func (d *SurveyDefinition) ValidateDefinition() error {
	if len(d.Questions) == 0 {
		return errors.New("survey must have at least one question")
	}

	questionIDs := make(map[string]bool)

	for i, q := range d.Questions {
		// Validate question ID
		if q.ID == "" {
			return fmt.Errorf("question %d: question ID is required", i)
		}

		// Check for duplicate question IDs
		if questionIDs[q.ID] {
			return fmt.Errorf("question %d: duplicate question ID '%s'", i, q.ID)
		}
		questionIDs[q.ID] = true

		// Validate question text
		if q.Text == "" {
			return fmt.Errorf("question %d: question text is required", i)
		}

		// Validate question type
		if q.Type != QuestionTypeSingle && q.Type != QuestionTypeMulti && q.Type != QuestionTypeText {
			return fmt.Errorf("question %d: invalid question type '%s'", i, q.Type)
		}

		// Validate options for choice questions
		if q.Type == QuestionTypeSingle || q.Type == QuestionTypeMulti {
			if len(q.Options) < 2 {
				return fmt.Errorf("question %d: choice questions must have at least 2 options", i)
			}

			optionIDs := make(map[string]bool)
			for j, opt := range q.Options {
				if opt.ID == "" {
					return fmt.Errorf("question %d, option %d: option ID is required", i, j)
				}
				if opt.Text == "" {
					return fmt.Errorf("question %d, option %d: option text is required", i, j)
				}
				if optionIDs[opt.ID] {
					return fmt.Errorf("question %d: duplicate option ID '%s'", i, opt.ID)
				}
				optionIDs[opt.ID] = true
			}
		}
	}

	return nil
}

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]{3}$`)

// ValidateSlug validates a survey slug
func ValidateSlug(slug string) error {
	if len(slug) < 3 || len(slug) > 50 {
		return errors.New("slug must be between 3 and 50 characters")
	}

	if !slugRegex.MatchString(slug) {
		return errors.New("slug must contain only lowercase letters, numbers, and hyphens (cannot start or end with hyphen)")
	}

	return nil
}

// SurveyResults represents aggregated results for a survey
type SurveyResults struct {
	SurveyID        uuid.UUID                  `json:"surveyId"`
	TotalVotes      int                        `json:"totalVotes"`
	QuestionResults map[string]*QuestionResult `json:"questionResults"` // keyed by question ID
}

// QuestionResult represents aggregated results for a single question
type QuestionResult struct {
	QuestionID   string         `json:"questionId"`
	OptionCounts map[string]int `json:"optionCounts"` // keyed by option ID, value is count
	TextAnswers  []string       `json:"textAnswers"`  // for text questions
}
