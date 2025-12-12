package consumer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/openmeet-team/survey/internal/models"
)

// ParseSurveyRecord parses an ATProto survey record into our Survey model
// Handles lexicon field mapping: name -> title, questions array with token types
func ParseSurveyRecord(record map[string]interface{}) (*models.SurveyDefinition, string, string, error) {
	// Extract name (maps to our "title")
	name, ok := record["name"].(string)
	if !ok || name == "" {
		return nil, "", "", fmt.Errorf("survey name is required")
	}

	// Extract description (optional)
	var description string
	if desc, hasDesc := record["description"].(string); hasDesc {
		description = desc
	}

	// Extract anonymous flag (optional, default false)
	anonymous := false
	if anonVal, hasAnon := record["anonymous"].(bool); hasAnon {
		anonymous = anonVal
	}

	// Parse questions array
	questionsRaw, ok := record["questions"].([]interface{})
	if !ok || len(questionsRaw) == 0 {
		return nil, "", "", fmt.Errorf("survey must have at least one question")
	}

	questions := make([]models.Question, 0, len(questionsRaw))
	for i, qRaw := range questionsRaw {
		qObj, ok := qRaw.(map[string]interface{})
		if !ok {
			return nil, "", "", fmt.Errorf("question %d is not an object", i)
		}

		question, err := parseQuestion(qObj, i)
		if err != nil {
			return nil, "", "", err
		}

		questions = append(questions, *question)
	}

	def := &models.SurveyDefinition{
		Questions: questions,
		Anonymous: anonymous,
	}

	return def, name, description, nil
}

// parseQuestion parses a single question from ATProto format
func parseQuestion(qObj map[string]interface{}, index int) (*models.Question, error) {
	// Extract question ID
	id, ok := qObj["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("question %d: id is required", index)
	}

	// Extract question text
	text, ok := qObj["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("question %d: text is required", index)
	}

	// Extract question type (with token prefix like "net.openmeet.survey#single")
	typeRaw, ok := qObj["type"].(string)
	if !ok || typeRaw == "" {
		return nil, fmt.Errorf("question %d: type is required", index)
	}

	// Strip token prefix to get simple type
	questionType := stripTokenPrefix(typeRaw)

	// Extract required flag (optional, default false)
	required := false
	if reqVal, hasReq := qObj["required"].(bool); hasReq {
		required = reqVal
	}

	// Parse options array (for choice questions)
	var options []models.Option
	if optionsRaw, hasOptions := qObj["options"].([]interface{}); hasOptions {
		for j, optRaw := range optionsRaw {
			optObj, ok := optRaw.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("question %d, option %d: not an object", index, j)
			}

			option, err := parseOption(optObj, index, j)
			if err != nil {
				return nil, err
			}

			options = append(options, *option)
		}
	}

	return &models.Question{
		ID:       id,
		Text:     text,
		Type:     models.QuestionType(questionType),
		Required: required,
		Options:  options,
	}, nil
}

// parseOption parses a single option from ATProto format
func parseOption(optObj map[string]interface{}, qIndex, optIndex int) (*models.Option, error) {
	id, ok := optObj["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("question %d, option %d: id is required", qIndex, optIndex)
	}

	text, ok := optObj["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("question %d, option %d: text is required", qIndex, optIndex)
	}

	return &models.Option{
		ID:   id,
		Text: text,
	}, nil
}

// stripTokenPrefix converts "net.openmeet.survey#single" -> "single"
func stripTokenPrefix(tokenType string) string {
	// Split on '#' and take the last part
	parts := strings.Split(tokenType, "#")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return tokenType
}

// ParseResponseRecord parses an ATProto response record into our Answer model
// Handles lexicon field mapping: selectedOptions (not "selected")
func ParseResponseRecord(record map[string]interface{}) (string, map[string]models.Answer, error) {
	// Extract subject (survey reference)
	subject, ok := record["subject"].(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("subject is required")
	}

	surveyURI, ok := subject["uri"].(string)
	if !ok || surveyURI == "" {
		return "", nil, fmt.Errorf("subject.uri is required")
	}

	// Parse answers array
	answersRaw, ok := record["answers"].([]interface{})
	if !ok || len(answersRaw) == 0 {
		return "", nil, fmt.Errorf("answers array is required")
	}

	answers := make(map[string]models.Answer)
	for i, ansRaw := range answersRaw {
		ansObj, ok := ansRaw.(map[string]interface{})
		if !ok {
			return "", nil, fmt.Errorf("answer %d is not an object", i)
		}

		questionID, ok := ansObj["questionId"].(string)
		if !ok || questionID == "" {
			return "", nil, fmt.Errorf("answer %d: questionId is required", i)
		}

		answer := models.Answer{}

		// Parse selectedOptions array (for choice questions)
		if selectedRaw, hasSelected := ansObj["selectedOptions"]; hasSelected {
			selectedArr, ok := selectedRaw.([]interface{})
			if !ok {
				return "", nil, fmt.Errorf("answer %d: selectedOptions must be an array", i)
			}

			for j, optRaw := range selectedArr {
				optID, ok := optRaw.(string)
				if !ok {
					return "", nil, fmt.Errorf("answer %d, option %d: not a string", i, j)
				}
				answer.SelectedOptions = append(answer.SelectedOptions, optID)
			}
		}

		// Parse text field (for text questions)
		if textRaw, hasText := ansObj["text"]; hasText {
			textStr, ok := textRaw.(string)
			if !ok {
				return "", nil, fmt.Errorf("answer %d: text must be a string", i)
			}
			answer.Text = textStr
		}

		answers[questionID] = answer
	}

	return surveyURI, answers, nil
}

// GenerateSlugFromTitle creates a URL-friendly slug from a survey title
// Handles collisions by appending -2, -3, etc.
var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func GenerateSlugFromTitle(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace non-alphanumeric characters with hyphens
	slug = slugRegex.ReplaceAllString(slug, "-")

	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Ensure minimum length
	if len(slug) < 3 {
		slug = "survey-" + slug
	}

	// Truncate to max length (50 chars)
	if len(slug) > 50 {
		slug = slug[:50]
		// Trim trailing hyphen if we cut in the middle
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}

// ParseResultsRecord parses an ATProto survey results record
// Returns: surveyURI, resultsCID
func ParseResultsRecord(record map[string]interface{}) (string, error) {
	// Extract subject (survey reference)
	subject, ok := record["subject"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("subject is required")
	}

	surveyURI, ok := subject["uri"].(string)
	if !ok || surveyURI == "" {
		return "", fmt.Errorf("subject.uri is required")
	}

	// We don't need to parse the actual results data - just track that results were published
	return surveyURI, nil
}
