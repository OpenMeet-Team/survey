package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openmeet-team/survey/internal/models"
)

func TestProcessSurveyResponse(t *testing.T) {
	database, queries := setupTestDB(t)
	defer database.Close()

	processor := NewProcessor(queries)
	ctx := context.Background()

	// Create a test survey
	survey := &models.Survey{
		ID:        uuid.New(),
		URI:       stringPtr("at://did:plc:test123/net.openmeet.survey/abc123"),
		CID:       stringPtr("bafy123"),
		AuthorDID: stringPtr("did:plc:test123"),
		Slug:      "test-survey",
		Title:     "Test Survey",
		Definition: models.SurveyDefinition{
			Questions: []models.Question{
				{
					ID:       "q1",
					Text:     "What's your favorite day?",
					Type:     models.QuestionTypeSingle,
					Required: true,
					Options: []models.Option{
						{ID: "mon", Text: "Monday"},
						{ID: "tue", Text: "Tuesday"},
						{ID: "wed", Text: "Wednesday"},
					},
				},
			},
			Anonymous: false,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := queries.CreateSurvey(ctx, survey)
	if err != nil {
		t.Fatalf("Failed to create test survey: %v", err)
	}

	t.Run("processes valid survey response", func(t *testing.T) {
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation: "create",
				Repo:      "did:plc:voter123",
				Collection: "net.openmeet.survey.response",
				RKey:      "response456",
				CID:       "bafy789",
				Record: map[string]interface{}{
					"$type":  "net.openmeet.survey.response",
					"subject": map[string]interface{}{
						"uri": "at://did:plc:test123/net.openmeet.survey/abc123",
					},
					"answers": []interface{}{
						map[string]interface{}{
							"questionId": "q1",
							"selected":   []interface{}{"mon"},
						},
					},
					"createdAt": time.Now().Format(time.RFC3339),
				},
			},
			TimeUs: 1234567890,
		}

		err := processor.ProcessMessage(ctx, msg)
		if err != nil {
			t.Fatalf("ProcessMessage failed: %v", err)
		}

		// Verify response was created
		responses, err := queries.ListResponsesBySurvey(ctx, survey.ID)
		if err != nil {
			t.Fatalf("Failed to list responses: %v", err)
		}

		if len(responses) != 1 {
			t.Fatalf("Expected 1 response, got %d", len(responses))
		}

		response := responses[0]
		if response.VoterDID == nil || *response.VoterDID != "did:plc:voter123" {
			t.Errorf("Expected VoterDID to be did:plc:voter123, got %v", response.VoterDID)
		}

		if response.RecordURI == nil || *response.RecordURI != "at://did:plc:voter123/net.openmeet.survey.response/response456" {
			t.Errorf("Expected RecordURI to be at://did:plc:voter123/net.openmeet.survey.response/response456, got %v", response.RecordURI)
		}

		if response.RecordCID == nil || *response.RecordCID != "bafy789" {
			t.Errorf("Expected RecordCID to be bafy789, got %v", response.RecordCID)
		}

		if len(response.Answers) != 1 {
			t.Fatalf("Expected 1 answer, got %d", len(response.Answers))
		}

		answer := response.Answers["q1"]
		if len(answer.SelectedOptions) != 1 || answer.SelectedOptions[0] != "mon" {
			t.Errorf("Expected answer to be [mon], got %v", answer.SelectedOptions)
		}
	})

	t.Run("rejects response to unknown survey", func(t *testing.T) {
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation: "create",
				Repo:      "did:plc:voter456",
				Collection: "net.openmeet.survey.response",
				RKey:      "response789",
				CID:       "bafy999",
				Record: map[string]interface{}{
					"$type":  "net.openmeet.survey.response",
					"subject": map[string]interface{}{
						"uri": "at://did:plc:unknown/net.openmeet.survey/unknown",
					},
					"answers": []interface{}{
						map[string]interface{}{
							"questionId": "q1",
							"selected":   []interface{}{"mon"},
						},
					},
					"createdAt": time.Now().Format(time.RFC3339),
				},
			},
			TimeUs: 1234567891,
		}

		err := processor.ProcessMessage(ctx, msg)
		if err == nil {
			t.Fatal("Expected error for unknown survey, got nil")
		}

		if !contains(err.Error(), "survey not found") {
			t.Errorf("Expected 'survey not found' error, got: %v", err)
		}
	})

	t.Run("rejects invalid answers", func(t *testing.T) {
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation: "create",
				Repo:      "did:plc:voter789",
				Collection: "net.openmeet.survey.response",
				RKey:      "response999",
				CID:       "bafy111",
				Record: map[string]interface{}{
					"$type":  "net.openmeet.survey.response",
					"subject": map[string]interface{}{
						"uri": "at://did:plc:test123/net.openmeet.survey/abc123",
					},
					"answers": []interface{}{
						map[string]interface{}{
							"questionId": "q1",
							"selected":   []interface{}{"invalid_option"}, // Invalid option
						},
					},
					"createdAt": time.Now().Format(time.RFC3339),
				},
			},
			TimeUs: 1234567892,
		}

		err := processor.ProcessMessage(ctx, msg)
		if err == nil {
			t.Fatal("Expected error for invalid answer, got nil")
		}

		if !contains(err.Error(), "invalid option") {
			t.Errorf("Expected 'invalid option' error, got: %v", err)
		}
	})

	t.Run("skips non-create operations", func(t *testing.T) {
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "delete",
				Repo:       "did:plc:voter123",
				Collection: "net.openmeet.survey.response",
				RKey:       "response456",
			},
			TimeUs: 1234567893,
		}

		err := processor.ProcessMessage(ctx, msg)
		// Should not return error for delete operations, just skip them
		if err != nil {
			t.Errorf("ProcessMessage should skip delete operations without error, got: %v", err)
		}
	})
}

// TestAuthorizationChecks tests that only the correct DID can update/delete records
func TestAuthorizationChecks(t *testing.T) {
	database, queries := setupTestDB(t)
	defer database.Close()

	processor := NewProcessor(queries)
	ctx := context.Background()

	t.Run("updateSurvey rejects wrong author DID", func(t *testing.T) {
		// Create a survey owned by did:plc:author1
		survey := &models.Survey{
			ID:        uuid.New(),
			URI:       stringPtr("at://did:plc:author1/net.openmeet.survey/survey1"),
			CID:       stringPtr("bafy123"),
			AuthorDID: stringPtr("did:plc:author1"),
			Slug:      "test-survey-auth-1",
			Title:     "Test Survey",
			Definition: models.SurveyDefinition{
				Questions: []models.Question{
					{
						ID:       "q1",
						Text:     "Question 1?",
						Type:     models.QuestionTypeSingle,
						Required: true,
						Options: []models.Option{
							{ID: "a", Text: "Option A"},
							{ID: "b", Text: "Option B"},
						},
					},
				},
				Anonymous: false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := queries.CreateSurvey(ctx, survey)
		if err != nil {
			t.Fatalf("Failed to create test survey: %v", err)
		}

		// Attempt to update from different DID (did:plc:attacker)
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "update",
				Repo:       "did:plc:attacker",
				Collection: "net.openmeet.survey",
				RKey:       "survey1",
				CID:        "bafy456",
				Record: map[string]interface{}{
					"$type": "net.openmeet.survey",
					"name":  "Malicious Update",
					"definition": map[string]interface{}{
						"questions": []interface{}{
							map[string]interface{}{
								"id":       "q1",
								"text":     "Hacked?",
								"type":     "net.openmeet.survey#single",
								"required": true,
								"options": []interface{}{
									map[string]interface{}{"id": "yes", "text": "Yes"},
								},
							},
						},
					},
				},
			},
			TimeUs: 1234567890,
		}

		err = processor.ProcessMessage(ctx, msg)
		if err == nil {
			t.Fatal("Expected error for unauthorized update, got nil")
		}

		if !contains(err.Error(), "unauthorized") {
			t.Errorf("Expected 'unauthorized' error, got: %v", err)
		}
	})

	t.Run("updateSurvey allows correct author DID", func(t *testing.T) {
		// Create a survey owned by did:plc:author2
		survey := &models.Survey{
			ID:        uuid.New(),
			URI:       stringPtr("at://did:plc:author2/net.openmeet.survey/survey2"),
			CID:       stringPtr("bafy200"),
			AuthorDID: stringPtr("did:plc:author2"),
			Slug:      "test-survey-auth-2",
			Title:     "Test Survey 2",
			Definition: models.SurveyDefinition{
				Questions: []models.Question{
					{
						ID:       "q1",
						Text:     "Question 1?",
						Type:     models.QuestionTypeSingle,
						Required: true,
						Options: []models.Option{
							{ID: "a", Text: "Option A"},
							{ID: "b", Text: "Option B"},
						},
					},
				},
				Anonymous: false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := queries.CreateSurvey(ctx, survey)
		if err != nil {
			t.Fatalf("Failed to create test survey: %v", err)
		}

		// Update from correct DID
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "update",
				Repo:       "did:plc:author2",
				Collection: "net.openmeet.survey",
				RKey:       "survey2",
				CID:        "bafy201",
				Record: map[string]interface{}{
					"$type": "net.openmeet.survey",
					"name":  "Updated Title",
					"definition": map[string]interface{}{
						"questions": []interface{}{
							map[string]interface{}{
								"id":       "q1",
								"text":     "Updated Question?",
								"type":     "net.openmeet.survey#single",
								"required": true,
								"options": []interface{}{
									map[string]interface{}{"id": "a", "text": "Option A"},
									map[string]interface{}{"id": "b", "text": "Option B"},
								},
							},
						},
					},
				},
			},
			TimeUs: 1234567891,
		}

		err = processor.ProcessMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Expected no error for authorized update, got: %v", err)
		}

		// Verify the update was applied
		updated, err := queries.GetSurveyByURI(ctx, *survey.URI)
		if err != nil {
			t.Fatalf("Failed to get updated survey: %v", err)
		}
		if updated.Title != "Updated Title" {
			t.Errorf("Expected title 'Updated Title', got: %s", updated.Title)
		}
	})

	t.Run("deleteSurvey rejects wrong author DID", func(t *testing.T) {
		// Create a survey owned by did:plc:author3
		survey := &models.Survey{
			ID:        uuid.New(),
			URI:       stringPtr("at://did:plc:author3/net.openmeet.survey/survey3"),
			CID:       stringPtr("bafy300"),
			AuthorDID: stringPtr("did:plc:author3"),
			Slug:      "test-survey-auth-3",
			Title:     "Test Survey 3",
			Definition: models.SurveyDefinition{
				Questions: []models.Question{
					{
						ID:       "q1",
						Text:     "Question 1?",
						Type:     models.QuestionTypeSingle,
						Required: true,
						Options: []models.Option{
							{ID: "a", Text: "Option A"},
						},
					},
				},
				Anonymous: false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := queries.CreateSurvey(ctx, survey)
		if err != nil {
			t.Fatalf("Failed to create test survey: %v", err)
		}

		// Attempt to delete from different DID
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "delete",
				Repo:       "did:plc:attacker",
				Collection: "net.openmeet.survey",
				RKey:       "survey3",
			},
			TimeUs: 1234567892,
		}

		err = processor.ProcessMessage(ctx, msg)
		if err == nil {
			t.Fatal("Expected error for unauthorized delete, got nil")
		}

		if !contains(err.Error(), "unauthorized") {
			t.Errorf("Expected 'unauthorized' error, got: %v", err)
		}

		// Verify survey still exists
		stillThere, err := queries.GetSurveyByURI(ctx, *survey.URI)
		if err != nil {
			t.Fatalf("Failed to check survey existence: %v", err)
		}
		if stillThere == nil {
			t.Error("Survey should still exist after unauthorized delete")
		}
	})

	t.Run("updateResponse rejects wrong voter DID", func(t *testing.T) {
		// Create a survey
		survey := &models.Survey{
			ID:        uuid.New(),
			URI:       stringPtr("at://did:plc:author4/net.openmeet.survey/survey4"),
			CID:       stringPtr("bafy400"),
			AuthorDID: stringPtr("did:plc:author4"),
			Slug:      "test-survey-auth-4",
			Title:     "Test Survey 4",
			Definition: models.SurveyDefinition{
				Questions: []models.Question{
					{
						ID:       "q1",
						Text:     "Question 1?",
						Type:     models.QuestionTypeSingle,
						Required: true,
						Options: []models.Option{
							{ID: "a", Text: "Option A"},
							{ID: "b", Text: "Option B"},
						},
					},
				},
				Anonymous: false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := queries.CreateSurvey(ctx, survey)
		if err != nil {
			t.Fatalf("Failed to create test survey: %v", err)
		}

		// Create a response by did:plc:voter1
		response := &models.Response{
			ID:        uuid.New(),
			SurveyID:  survey.ID,
			VoterDID:  stringPtr("did:plc:voter1"),
			RecordURI: stringPtr("at://did:plc:voter1/net.openmeet.survey.response/resp1"),
			RecordCID: stringPtr("bafy401"),
			Answers: map[string]models.Answer{
				"q1": {SelectedOptions: []string{"a"}},
			},
			CreatedAt: time.Now(),
		}

		err = queries.CreateResponse(ctx, response)
		if err != nil {
			t.Fatalf("Failed to create test response: %v", err)
		}

		// Attempt to update from different DID
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "update",
				Repo:       "did:plc:attacker",
				Collection: "net.openmeet.survey.response",
				RKey:       "resp1",
				CID:        "bafy402",
				Record: map[string]interface{}{
					"$type": "net.openmeet.survey.response",
					"subject": map[string]interface{}{
						"uri": *survey.URI,
					},
					"answers": []interface{}{
						map[string]interface{}{
							"questionId": "q1",
							"selected":   []interface{}{"b"},
						},
					},
				},
			},
			TimeUs: 1234567893,
		}

		err = processor.ProcessMessage(ctx, msg)
		if err == nil {
			t.Fatal("Expected error for unauthorized response update, got nil")
		}

		if !contains(err.Error(), "unauthorized") {
			t.Errorf("Expected 'unauthorized' error, got: %v", err)
		}
	})

	t.Run("updateResponse allows correct voter DID", func(t *testing.T) {
		// Create a survey
		survey := &models.Survey{
			ID:        uuid.New(),
			URI:       stringPtr("at://did:plc:author5/net.openmeet.survey/survey5"),
			CID:       stringPtr("bafy500"),
			AuthorDID: stringPtr("did:plc:author5"),
			Slug:      "test-survey-auth-5",
			Title:     "Test Survey 5",
			Definition: models.SurveyDefinition{
				Questions: []models.Question{
					{
						ID:       "q1",
						Text:     "Question 1?",
						Type:     models.QuestionTypeSingle,
						Required: true,
						Options: []models.Option{
							{ID: "a", Text: "Option A"},
							{ID: "b", Text: "Option B"},
						},
					},
				},
				Anonymous: false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := queries.CreateSurvey(ctx, survey)
		if err != nil {
			t.Fatalf("Failed to create test survey: %v", err)
		}

		// Create a response by did:plc:voter2
		response := &models.Response{
			ID:        uuid.New(),
			SurveyID:  survey.ID,
			VoterDID:  stringPtr("did:plc:voter2"),
			RecordURI: stringPtr("at://did:plc:voter2/net.openmeet.survey.response/resp2"),
			RecordCID: stringPtr("bafy501"),
			Answers: map[string]models.Answer{
				"q1": {SelectedOptions: []string{"a"}},
			},
			CreatedAt: time.Now(),
		}

		err = queries.CreateResponse(ctx, response)
		if err != nil {
			t.Fatalf("Failed to create test response: %v", err)
		}

		// Update from correct DID
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "update",
				Repo:       "did:plc:voter2",
				Collection: "net.openmeet.survey.response",
				RKey:       "resp2",
				CID:        "bafy502",
				Record: map[string]interface{}{
					"$type": "net.openmeet.survey.response",
					"subject": map[string]interface{}{
						"uri": *survey.URI,
					},
					"answers": []interface{}{
						map[string]interface{}{
							"questionId": "q1",
							"selected":   []interface{}{"b"},
						},
					},
				},
			},
			TimeUs: 1234567894,
		}

		err = processor.ProcessMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Expected no error for authorized response update, got: %v", err)
		}

		// Verify the update was applied
		updated, err := queries.GetResponseByRecordURI(ctx, *response.RecordURI)
		if err != nil {
			t.Fatalf("Failed to get updated response: %v", err)
		}
		if len(updated.Answers["q1"].SelectedOptions) != 1 || updated.Answers["q1"].SelectedOptions[0] != "b" {
			t.Errorf("Expected answer 'b', got: %v", updated.Answers["q1"].SelectedOptions)
		}
	})

	t.Run("deleteResponse rejects wrong voter DID", func(t *testing.T) {
		// Create a survey
		survey := &models.Survey{
			ID:        uuid.New(),
			URI:       stringPtr("at://did:plc:author6/net.openmeet.survey/survey6"),
			CID:       stringPtr("bafy600"),
			AuthorDID: stringPtr("did:plc:author6"),
			Slug:      "test-survey-auth-6",
			Title:     "Test Survey 6",
			Definition: models.SurveyDefinition{
				Questions: []models.Question{
					{
						ID:       "q1",
						Text:     "Question 1?",
						Type:     models.QuestionTypeSingle,
						Required: true,
						Options: []models.Option{
							{ID: "a", Text: "Option A"},
						},
					},
				},
				Anonymous: false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := queries.CreateSurvey(ctx, survey)
		if err != nil {
			t.Fatalf("Failed to create test survey: %v", err)
		}

		// Create a response by did:plc:voter3
		response := &models.Response{
			ID:        uuid.New(),
			SurveyID:  survey.ID,
			VoterDID:  stringPtr("did:plc:voter3"),
			RecordURI: stringPtr("at://did:plc:voter3/net.openmeet.survey.response/resp3"),
			RecordCID: stringPtr("bafy601"),
			Answers: map[string]models.Answer{
				"q1": {SelectedOptions: []string{"a"}},
			},
			CreatedAt: time.Now(),
		}

		err = queries.CreateResponse(ctx, response)
		if err != nil {
			t.Fatalf("Failed to create test response: %v", err)
		}

		// Attempt to delete from different DID
		msg := &JetstreamMessage{
			Kind: "commit",
			Commit: &JetstreamCommit{
				Operation:  "delete",
				Repo:       "did:plc:attacker",
				Collection: "net.openmeet.survey.response",
				RKey:       "resp3",
			},
			TimeUs: 1234567895,
		}

		err = processor.ProcessMessage(ctx, msg)
		if err == nil {
			t.Fatal("Expected error for unauthorized response delete, got nil")
		}

		if !contains(err.Error(), "unauthorized") {
			t.Errorf("Expected 'unauthorized' error, got: %v", err)
		}

		// Verify response still exists
		stillThere, err := queries.GetResponseByRecordURI(ctx, *response.RecordURI)
		if err != nil {
			t.Fatalf("Failed to check response existence: %v", err)
		}
		if stillThere == nil {
			t.Error("Response should still exist after unauthorized delete")
		}
	})
}

func stringPtr(s string) *string {
	return &s
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
