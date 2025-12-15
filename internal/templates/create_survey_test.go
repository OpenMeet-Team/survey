package templates

import (
	"bytes"
	"context"
	"testing"

	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateSurvey_RendersWithoutError ensures the template compiles and renders
func TestCreateSurvey_RendersWithoutError(t *testing.T) {
	tests := []struct {
		name       string
		user       *oauth.User
		profile    *oauth.Profile
		posthogKey string
	}{
		{
			name:       "anonymous user",
			user:       nil,
			profile:    nil,
			posthogKey: "",
		},
		{
			name: "authenticated user",
			user: &oauth.User{
				DID: "did:plc:test123",
			},
			profile: &oauth.Profile{
				DID:         "did:plc:test123",
				Handle:      "test.bsky.social",
				DisplayName: "Test User",
			},
			posthogKey: "test-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := context.Background()

			err := CreateSurvey(tt.user, tt.profile, tt.posthogKey, "").Render(ctx, &buf)
			require.NoError(t, err, "Template should render without errors")

			html := buf.String()
			assert.NotEmpty(t, html, "Rendered HTML should not be empty")
		})
	}
}

// TestCreateSurvey_ContainsAIGenerationUI ensures the AI generation UI elements are present
func TestCreateSurvey_ContainsAIGenerationUI(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	err := CreateSurvey(nil, nil, "", "").Render(ctx, &buf)
	require.NoError(t, err)

	html := buf.String()

	// Check for AI generation section elements
	assert.Contains(t, html, "id=\"ai-description\"", "Should have AI description textarea")
	assert.Contains(t, html, "id=\"ai-consent\"", "Should have consent checkbox")
	assert.Contains(t, html, "id=\"generate-btn\"", "Should have generate button")
	assert.Contains(t, html, "id=\"ai-error\"", "Should have error display area")
	assert.Contains(t, html, "id=\"char-counter\"", "Should have character counter")

	// Check for character limit
	assert.Contains(t, html, "maxlength=\"2000\"", "Should have 2000 character limit")

	// Check for consent requirement text
	assert.Contains(t, html, "OpenAI", "Should mention OpenAI in consent text")

	// Check for existing editor elements (should still be present)
	assert.Contains(t, html, "id=\"editor-container\"", "Monaco editor container should still be present")
	assert.Contains(t, html, "id=\"survey-form\"", "Survey form should still be present")
}

// TestCreateSurvey_ContainsGenerateHandlerScript ensures the JavaScript for AI generation is present
func TestCreateSurvey_ContainsGenerateHandlerScript(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	err := CreateSurvey(nil, nil, "", "").Render(ctx, &buf)
	require.NoError(t, err)

	html := buf.String()

	// Check for JavaScript handling
	assert.Contains(t, html, "/api/v1/surveys/generate", "Should have API endpoint reference")
	assert.Contains(t, html, "POST", "Should make POST request")

	// Check for character counter logic
	assert.Contains(t, html, "char-counter", "Should update character counter")

	// Check for consent validation
	assert.Contains(t, html, "ai-consent", "Should check consent before generating")
}

// TestCreateSurvey_TemplateMode ensures template mode shows correct UI
func TestCreateSurvey_TemplateMode(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	templateJSON := `{"title":"Test Survey","questions":[{"id":"q1","text":"Test?","type":"single"}]}`
	err := CreateSurvey(nil, nil, "", templateJSON).Render(ctx, &buf)
	require.NoError(t, err)

	html := buf.String()

	// Check for template-specific UI elements
	assert.Contains(t, html, "Build on Existing Survey", "Should have template mode header")
	assert.Contains(t, html, "Modify with AI", "Should have modify section heading")
	assert.Contains(t, html, "Describe what you'd like to change", "Should have template-specific label")
	assert.Contains(t, html, "Modify Survey", "Should have modify button text")
	assert.Contains(t, html, "Skip to Editor", "Should have simplified skip button text")

	// Check that template data is embedded as data attribute
	assert.Contains(t, html, "id=\"template-data\"", "Should have template data element")
	assert.Contains(t, html, "data-template=", "Should have data-template attribute")

	// Verify normal mode elements are NOT present
	assert.NotContains(t, html, "Create New Survey", "Should NOT have create new header")
	assert.NotContains(t, html, "Generate Survey with AI", "Should NOT have generate section heading")
}

// TestCreateSurvey_NormalMode ensures normal mode shows correct UI
func TestCreateSurvey_NormalMode(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()

	err := CreateSurvey(nil, nil, "", "").Render(ctx, &buf)
	require.NoError(t, err)

	html := buf.String()

	// Check for normal mode UI elements
	assert.Contains(t, html, "Create New Survey", "Should have normal mode header")
	assert.Contains(t, html, "Generate Survey with AI", "Should have generate section heading")
	assert.Contains(t, html, "Describe your survey in plain text", "Should have normal label")
	assert.Contains(t, html, "Generate Survey", "Should have generate button text")
	assert.Contains(t, html, "Skip to Advanced Editor", "Should have full skip button text")

	// Verify template mode elements are NOT present
	assert.NotContains(t, html, "Build on Existing Survey", "Should NOT have template header")
	assert.NotContains(t, html, "Modify with AI", "Should NOT have modify heading")
	assert.NotContains(t, html, "id=\"template-data\"", "Should NOT have template data script")
}
