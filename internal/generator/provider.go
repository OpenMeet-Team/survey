package generator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openmeet-team/survey/internal/models"
	"github.com/tmc/langchaingo/llms"
)

var (
	// ErrEmptyResponse is returned when LLM returns empty response
	ErrEmptyResponse = errors.New("LLM returned empty response")

	// ErrContextCanceled is returned when context is canceled
	ErrContextCanceled = errors.New("context canceled")

	// ErrCostLimitExceeded is returned when daily cost limit is exceeded
	ErrCostLimitExceeded = errors.New("daily cost limit exceeded")
)

// GenerateResult contains the result of an AI generation
type GenerateResult struct {
	Definition    *models.SurveyDefinition
	InputTokens   int
	OutputTokens  int
	EstimatedCost float64
	SystemPrompt  string // The system prompt sent to the LLM
	RawResponse   string // The raw LLM response before sanitization
}

// SurveyGenerator generates surveys using an LLM
type SurveyGenerator struct {
	llm          llms.Model
	model        string
	validator    *InputValidator
	sanitizer    *OutputSanitizer
	costLimiter  *CostLimiter
}

// NewSurveyGenerator creates a new survey generator
func NewSurveyGenerator(llm llms.Model, model string) *SurveyGenerator {
	return &SurveyGenerator{
		llm:         llm,
		model:       model,
		validator:   NewInputValidator(),
		sanitizer:   NewOutputSanitizer(),
		costLimiter: NewCostLimiter(10.0), // $10/day default
	}
}

// ValidateInput validates user input before generation
// Use this to pre-validate input when building refinement prompts
func (g *SurveyGenerator) ValidateInput(input string) error {
	return g.validator.Validate(input)
}

// Generate creates a survey from a natural language prompt
func (g *SurveyGenerator) Generate(ctx context.Context, prompt string) (*GenerateResult, error) {
	// Validate input first
	if err := g.validator.Validate(prompt); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	return g.generateInternal(ctx, prompt)
}

// GenerateRaw creates a survey without validating the prompt
// Use this when the prompt has already been validated or is a refinement prompt
// containing pre-validated user input combined with trusted existing JSON
func (g *SurveyGenerator) GenerateRaw(ctx context.Context, prompt string) (*GenerateResult, error) {
	return g.generateInternal(ctx, prompt)
}

// generateInternal is the shared implementation for Generate and GenerateRaw
func (g *SurveyGenerator) generateInternal(ctx context.Context, prompt string) (*GenerateResult, error) {
	// Check context first
	if ctx.Err() != nil {
		return nil, ErrContextCanceled
	}

	// Estimate cost before making the call
	systemPrompt := g.buildSystemPrompt()
	inputTokens := g.estimateTokens(systemPrompt + prompt)
	outputTokens := 500 // Conservative estimate for survey JSON
	estimatedCost := g.costLimiter.EstimateTokenCost(inputTokens, outputTokens)

	// Check cost limit
	if !g.costLimiter.AllowRequest(estimatedCost) {
		return nil, ErrCostLimitExceeded
	}

	// Build messages
	messages := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: systemPrompt},
			},
		},
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: prompt},
			},
		},
	}

	// Call LLM
	resp, err := g.llm.GenerateContent(ctx, messages, llms.WithModel(g.model))
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, ErrEmptyResponse
	}

	responseText := resp.Choices[0].Content
	if strings.TrimSpace(responseText) == "" {
		return nil, ErrEmptyResponse
	}

	// Sanitize and validate output
	definition, err := g.sanitizer.Sanitize(responseText)
	if err != nil {
		return nil, fmt.Errorf("invalid LLM output: %w", err)
	}

	// Count actual tokens from response metadata if available
	actualInputTokens := inputTokens
	actualOutputTokens := g.estimateTokens(responseText)

	return &GenerateResult{
		Definition:    definition,
		InputTokens:   actualInputTokens,
		OutputTokens:  actualOutputTokens,
		EstimatedCost: estimatedCost,
		SystemPrompt:  systemPrompt,
		RawResponse:   responseText,
	}, nil
}

// buildSystemPrompt creates the system prompt for the LLM
// This matches the lexicon schema in lexicon/net.openmeet.survey.json
func (g *SurveyGenerator) buildSystemPrompt() string {
	return `You are a helpful assistant that creates survey definitions in JSON format.

Given a natural language description of a survey, generate a valid JSON object that matches this structure:

{
  "questions": [
    {
      "id": "q1",
      "text": "Question text here",
      "type": "single" | "multi" | "text",
      "required": false,
      "options": [
        {"id": "opt1", "text": "Option 1"},
        {"id": "opt2", "text": "Option 2"}
      ]
    }
  ],
  "anonymous": false
}

Question Types:
- "single": Single-choice question (radio buttons) - user picks ONE option
- "multi": Multiple-choice question (checkboxes) - user picks MULTIPLE options
- "text": Free-text response - no options needed

Rules:
1. Always return ONLY valid JSON, no markdown, no additional text
2. Generate unique IDs for questions (q1, q2, q3...) and options (opt1, opt2, opt3...)
3. Keep questions clear and concise (max 300 characters)
4. For choice questions (single/multi), provide 2-20 options
5. Options should be distinct and clear (max 150 characters each)
6. Use "single" for yes/no, rating scales, or pick-one questions
7. Use "multi" for check-all-that-apply or select-multiple questions
8. Use "text" for open-ended questions (options array should be empty)
9. Maximum 50 questions per survey (typically 1-5 for polls)
10. Keep all text safe and appropriate - no offensive, dangerous, or inappropriate content
11. Set "required" to false by default unless specified
12. Set "anonymous" to false by default

Generate ONLY the JSON, nothing else. No markdown formatting.`
}

// estimateTokens provides a rough token count estimate
// This is approximate - actual tokenization depends on the model
func (g *SurveyGenerator) estimateTokens(text string) int {
	// Rough heuristic: ~1 token per 4 characters for English text
	// This is conservative and works reasonably well for GPT models
	return len(text) / 4
}
