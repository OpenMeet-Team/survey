package templates

import (
	"strings"
	"testing"

	"github.com/openmeet-team/survey/internal/models"
	"github.com/stretchr/testify/assert"
)

// TestSurveyOGMeta_TitleWithSuffix tests that og:title always includes " - OpenMeet Survey"
func TestSurveyOGMeta_TitleWithSuffix(t *testing.T) {
	tests := []struct {
		name          string
		surveyTitle   string
		expectedTitle string
	}{
		{
			name:          "short title gets suffix",
			surveyTitle:   "Test Survey",
			expectedTitle: "Test Survey - Share Your Opinion on OpenMeet Survey",
		},
		{
			name:          "medium title gets suffix",
			surveyTitle:   "Community Feedback Survey",
			expectedTitle: "Community Feedback Survey - Share Your Opinion on OpenMeet Survey",
		},
		{
			name:          "long title gets suffix",
			surveyTitle:   "Annual Customer Satisfaction and Product Feature Survey",
			expectedTitle: "Annual Customer Satisfaction and Product Feature Survey - Share Your Opinion on OpenMeet Survey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			survey := &models.Survey{
				Title: tt.surveyTitle,
			}

			og := surveyOGMeta(survey)

			assert.Equal(t, tt.expectedTitle, og.Title, "OG title should include suffix")
			// Verify title length is optimal (50-60+ chars)
			assert.GreaterOrEqual(t, len(og.Title), 50, "OG title should be at least 50 characters for optimal social sharing")
		})
	}
}

// TestSurveyOGMeta_DescriptionFallback tests that description has a default when nil
func TestSurveyOGMeta_DescriptionFallback(t *testing.T) {
	tests := []struct {
		name                string
		surveyDescription   *string
		expectedDescription string
	}{
		{
			name:                "nil description gets default",
			surveyDescription:   nil,
			expectedDescription: "Participate in this survey and share your thoughts. Your feedback helps shape better decisions and outcomes for the community.",
		},
		{
			name:                "empty description gets default",
			surveyDescription:   stringPtr(""),
			expectedDescription: "Participate in this survey and share your thoughts. Your feedback helps shape better decisions and outcomes for the community.",
		},
		{
			name:                "whitespace description gets default",
			surveyDescription:   stringPtr("   "),
			expectedDescription: "Participate in this survey and share your thoughts. Your feedback helps shape better decisions and outcomes for the community.",
		},
		{
			name:                "valid description is preserved",
			surveyDescription:   stringPtr("This is a test survey about product feedback"),
			expectedDescription: "This is a test survey about product feedback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			survey := &models.Survey{
				Title:       "Test Survey",
				Description: tt.surveyDescription,
			}

			og := surveyOGMeta(survey)

			assert.Equal(t, tt.expectedDescription, og.Description, "OG description should have default or preserve provided value")
			assert.NotEmpty(t, og.Description, "OG description should never be empty")

			// Verify default description length is optimal (110-160 chars)
			// Only check this for default descriptions, not custom ones
			if tt.surveyDescription == nil || strings.TrimSpace(*tt.surveyDescription) == "" {
				assert.GreaterOrEqual(t, len(og.Description), 110, "Default OG description should be at least 110 characters for optimal social sharing")
			}
		})
	}
}

// TestSurveyOGMeta_TypeIsWebsite tests that og:type is always "website"
func TestSurveyOGMeta_TypeIsWebsite(t *testing.T) {
	survey := &models.Survey{
		Title: "Test Survey",
	}

	og := surveyOGMeta(survey)

	assert.Equal(t, "website", og.Type, "OG type should be website")
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
