package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/openmeet-team/survey/internal/generator"
	"github.com/openmeet-team/survey/internal/models"
	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/openmeet-team/survey/internal/telemetry"
	"github.com/openmeet-team/survey/internal/templates"
)

// QueriesInterface defines the interface for database queries
// This allows for mocking in tests
type QueriesInterface interface {
	CreateSurvey(ctx context.Context, s *models.Survey) error
	GetSurveyBySlug(ctx context.Context, slug string) (*models.Survey, error)
	GetSurveyByURI(ctx context.Context, uri string) (*models.Survey, error)
	ListSurveys(ctx context.Context, limit, offset int) ([]*models.Survey, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
	CreateResponse(ctx context.Context, r *models.Response) error
	GetResponseBySurveyAndVoter(ctx context.Context, surveyID uuid.UUID, voterDID, voterSession string) (*models.Response, error)
	GetSurveyResults(ctx context.Context, surveyID uuid.UUID) (*models.SurveyResults, error)
	UpdateSurveyResults(ctx context.Context, surveyID uuid.UUID, resultsURI, resultsCID string) error
	GetStats(ctx context.Context) (*models.Stats, error)
}

// GeneratorInterface defines the interface for AI survey generation
type GeneratorInterface interface {
	Generate(ctx context.Context, prompt string) (*generator.GenerateResult, error)
	GenerateRaw(ctx context.Context, prompt string) (*generator.GenerateResult, error)
	ValidateInput(input string) error
}

// RateLimiterInterface defines the interface for rate limiting
type RateLimiterInterface interface {
	AllowAnonymous(ip string) bool
	AllowAuthenticated(did string) bool
}

// GenerationLoggerInterface defines the interface for logging AI generation attempts
type GenerationLoggerInterface interface {
	LogSuccess(ctx context.Context, userID, userType, inputPrompt, systemPrompt, rawResponse string, result *generator.GenerateResult, durationMS int) error
	LogError(ctx context.Context, userID, userType, inputPrompt, systemPrompt, status, errorMessage string, inputTokens, outputTokens int, costUSD float64, durationMS int) error
}

// Handlers holds the HTTP handlers and dependencies
type Handlers struct {
	queries        QueriesInterface
	oauthStorage   *oauth.Storage
	supportURL     string
	posthogKey     string
	generator      GeneratorInterface
	generatorRL    RateLimiterInterface
	generationLog  GenerationLoggerInterface
}

// NewHandlers creates a new Handlers instance
func NewHandlers(q QueriesInterface) *Handlers {
	return &Handlers{
		queries:      q,
		oauthStorage: nil, // Optional: can be nil if OAuth not configured
		supportURL:   "",
	}
}

// NewHandlersWithOAuth creates a new Handlers instance with OAuth support
func NewHandlersWithOAuth(q QueriesInterface, oauthStorage *oauth.Storage) *Handlers {
	return &Handlers{
		queries:      q,
		oauthStorage: oauthStorage,
		supportURL:   "",
	}
}

// SetSupportURL sets the support URL for the handlers
func (h *Handlers) SetSupportURL(url string) {
	h.supportURL = url
}

// SetPostHogKey sets the PostHog API key for analytics
func (h *Handlers) SetPostHogKey(key string) {
	h.posthogKey = key
}

// SetGenerator sets the AI generator and rate limiter for survey generation
func (h *Handlers) SetGenerator(gen GeneratorInterface, rl RateLimiterInterface) {
	h.generator = gen
	h.generatorRL = rl
}

// SetLogger sets the generation logger for AI survey generation
func (h *Handlers) SetLogger(logger GenerationLoggerInterface) {
	h.generationLog = logger
}

// CreateSurvey creates a new survey
// POST /api/v1/surveys
func (h *Handlers) CreateSurvey(c echo.Context) error {
	var req CreateSurveyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request body",
			Details: err.Error(),
		})
	}

	// Parse the definition (JSON or YAML)
	def, err := models.ParseSurveyDefinition([]byte(req.Definition))
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid survey definition",
			Details: err.Error(),
		})
	}

	// Validate the definition
	if err := def.ValidateDefinition(); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid survey definition",
			Details: err.Error(),
		})
	}

	// Generate or validate slug
	slug := req.Slug
	if slug == "" {
		// Auto-generate from title (first question text as fallback)
		if len(def.Questions) > 0 {
			slug = generateSlug(def.Questions[0].Text)
		} else {
			slug = generateSlug("survey-" + uuid.New().String()[:8])
		}
	} else {
		// Validate provided slug
		if err := models.ValidateSlug(slug); err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "Invalid slug",
				Details: err.Error(),
			})
		}
	}

	// Check if slug already exists
	exists, err := h.queries.SlugExists(c.Request().Context(), slug)
	if err != nil {
		return InternalServerError(c, "Failed to check slug availability", err)
	}
	if exists {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "Survey slug already exists",
			Details: fmt.Sprintf("A survey with slug '%s' already exists", slug),
		})
	}

	// Extract title from definition (use first question text if no explicit title)
	title := ""
	if len(def.Questions) > 0 {
		title = def.Questions[0].Text
	}

	// Create survey model
	now := time.Now()
	survey := &models.Survey{
		ID:         uuid.New(),
		Slug:       slug,
		Title:      title,
		Definition: *def,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Save to database
	if err := h.queries.CreateSurvey(c.Request().Context(), survey); err != nil {
		return InternalServerError(c, "Failed to create survey", err)
	}

	// Return response
	return c.JSON(http.StatusCreated, ToSurveyResponse(survey, true))
}

// GetSurvey retrieves a survey by slug
// GET /api/v1/surveys/:slug
func (h *Handlers) GetSurvey(c echo.Context) error {
	slug := c.Param("slug")

	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Survey not found",
				Details: fmt.Sprintf("No survey found with slug '%s'", slug),
			})
		}
		return InternalServerError(c, "Failed to retrieve survey", err)
	}

	return c.JSON(http.StatusOK, ToSurveyResponse(survey, true))
}

// ListSurveys retrieves a list of surveys with pagination
// GET /api/v1/surveys?limit=20&offset=0
func (h *Handlers) ListSurveys(c echo.Context) error {
	// Parse pagination parameters
	limit := 20 // default
	offset := 0

	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	surveys, err := h.queries.ListSurveys(c.Request().Context(), limit, offset)
	if err != nil {
		return InternalServerError(c, "Failed to retrieve surveys", err)
	}

	// Convert to list response (without definitions)
	result := make([]SurveyListResponse, len(surveys))
	for i, s := range surveys {
		result[i] = *ToSurveyListResponse(s)
	}

	return c.JSON(http.StatusOK, result)
}

// SubmitResponse submits a response to a survey
// POST /api/v1/surveys/:slug/responses
func (h *Handlers) SubmitResponse(c echo.Context) error {
	slug := c.Param("slug")

	// Get the survey
	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Survey not found",
				Details: fmt.Sprintf("No survey found with slug '%s'", slug),
			})
		}
		return InternalServerError(c, "Failed to retrieve survey", err)
	}

	// Parse request body
	var req SubmitResponseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request body",
			Details: err.Error(),
		})
	}

	// Validate answers
	if err := models.ValidateAnswers(&survey.Definition, req.Answers); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid answers",
			Details: err.Error(),
		})
	}

	// Generate voter session (guest identity)
	ip := getClientIP(c)
	userAgent := c.Request().UserAgent()
	voterSession := models.GenerateVoterSession(survey.ID, ip, userAgent)

	// Check if already voted
	existingResponse, err := h.queries.GetResponseBySurveyAndVoter(
		c.Request().Context(),
		survey.ID,
		"", // voterDID (empty for anonymous)
		voterSession,
	)
	if err != nil {
		return InternalServerError(c, "Failed to check for existing response", err)
	}

	if existingResponse != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "Already voted",
			Details: "You have already submitted a response to this survey",
		})
	}

	// Create response
	now := time.Now()
	response := &models.Response{
		ID:           uuid.New(),
		SurveyID:     survey.ID,
		VoterSession: &voterSession,
		Answers:      req.Answers,
		CreatedAt:    now,
	}

	// Save response
	if err := h.queries.CreateResponse(c.Request().Context(), response); err != nil {
		return InternalServerError(c, "Failed to submit response", err)
	}

	// Record metrics (no slug label to avoid cardinality explosion)
	telemetry.SurveyResponsesTotal.WithLabelValues("web").Inc()

	// Return success
	return c.JSON(http.StatusCreated, ResponseSubmittedResponse{
		ID:        response.ID,
		SurveyID:  survey.ID,
		CreatedAt: response.CreatedAt,
	})
}

// GetResults retrieves aggregated results for a survey
// GET /api/v1/surveys/:slug/results
func (h *Handlers) GetResults(c echo.Context) error {
	slug := c.Param("slug")

	// Get the survey
	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Survey not found",
				Details: fmt.Sprintf("No survey found with slug '%s'", slug),
			})
		}
		return InternalServerError(c, "Failed to retrieve survey", err)
	}

	// Get results
	results, err := h.queries.GetSurveyResults(c.Request().Context(), survey.ID)
	if err != nil {
		return InternalServerError(c, "Failed to retrieve results", err)
	}

	return c.JSON(http.StatusOK, results)
}

// Helper Functions

var slugifyRegex = regexp.MustCompile(`[^a-z0-9]+`)

// generateSlug creates a URL-friendly slug from a title
func generateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace non-alphanumeric characters with hyphens
	slug = slugifyRegex.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Limit length to 50 characters
	if len(slug) > 50 {
		slug = slug[:50]
		// Trim trailing hyphen if truncation created one
		slug = strings.TrimRight(slug, "-")
	}

	// Ensure minimum length
	if len(slug) < 3 {
		slug = "survey-" + uuid.New().String()[:8]
	}

	return slug
}

// getClientIP is now implemented in ip_extraction.go with proper security
// This function was vulnerable to IP spoofing attacks - see ip_extraction.go for secure implementation

// HTML Handlers

// GetSurveyHTML renders the survey form page
// GET /surveys/:slug
func (h *Handlers) GetSurveyHTML(c echo.Context) error {
	slug := c.Param("slug")

	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Survey not found")
		}
		return c.String(http.StatusInternalServerError, "Failed to load survey")
	}

	// Get user and profile from context
	user, profile := getUserAndProfile(c)

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.SurveyForm(survey, user, profile, h.posthogKey)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// CreateSurveyPageHTML renders the create survey form
// GET /surveys/new
// Optional query param: template=<slug> to pre-populate from existing survey
func (h *Handlers) CreateSurveyPageHTML(c echo.Context) error {
	// Get user and profile from context
	user, profile := getUserAndProfile(c)

	// Check for template query param
	var templateJSON string
	if templateSlug := c.QueryParam("template"); templateSlug != "" {
		survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), templateSlug)
		if err == nil && survey != nil {
			// Serialize the definition to JSON
			defBytes, err := json.Marshal(survey.Definition)
			if err == nil {
				templateJSON = string(defBytes)
			}
		}
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.CreateSurvey(user, profile, h.posthogKey, templateJSON)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// CreateSurveyHTML handles survey creation from HTML form
// POST /surveys
func (h *Handlers) CreateSurveyHTML(c echo.Context) error {
	slug := c.FormValue("slug")
	definition := c.FormValue("definition")

	// Parse the definition
	def, err := models.ParseSurveyDefinition([]byte(definition))
	if err != nil {
		component := templates.Error("Invalid survey definition: " + err.Error())
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Validate the definition
	if err := def.ValidateDefinition(); err != nil {
		component := templates.Error("Invalid survey definition: " + err.Error())
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Generate or validate slug
	if slug == "" {
		if len(def.Questions) > 0 {
			slug = generateSlug(def.Questions[0].Text)
		} else {
			slug = generateSlug("survey-" + uuid.New().String()[:8])
		}
	} else {
		if err := models.ValidateSlug(slug); err != nil {
			component := templates.Error("Invalid slug: " + err.Error())
			return component.Render(c.Request().Context(), c.Response().Writer)
		}
	}

	// Check if slug exists
	exists, err := h.queries.SlugExists(c.Request().Context(), slug)
	if err != nil {
		component := templates.Error("Failed to check slug availability")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}
	if exists {
		component := templates.Error(fmt.Sprintf("A survey with slug '%s' already exists", slug))
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Extract title from definition
	title := ""
	if len(def.Questions) > 0 {
		title = def.Questions[0].Text
	}

	// Check if user is logged in with OAuth
	var uri *string
	var cid *string
	var authorDID *string

	if h.oauthStorage != nil {
		session, err := oauth.GetSession(c, h.oauthStorage)
		if err == nil && session != nil && session.AccessToken != "" && session.PDSUrl != "" {
			// User is logged in - write to PDS first
			rkey := oauth.GenerateTID()

			// Build AT URI before PDS write (so we can store it locally)
			atURI := fmt.Sprintf("at://%s/net.openmeet.survey/%s", session.DID, rkey)
			uri = &atURI
			authorDID = &session.DID

			// Build ATProto record matching lexicon format
			record := map[string]interface{}{
				"$type":     "net.openmeet.survey",
				"name":      title,
				"questions": def.Questions,
				"createdAt": time.Now().Format(time.RFC3339),
			}

			// Add optional fields if present
			if def.Anonymous {
				record["anonymous"] = def.Anonymous
			}

			// Write to PDS
			pdsURI, pdsCID, err := oauth.CreateRecord(session, "net.openmeet.survey", rkey, record)
			if err != nil {
				// PDS write failed - log but continue with local-only survey
				c.Logger().Errorf("Failed to write survey to PDS: %v", err)
				uri = nil
				authorDID = nil
			} else {
				// PDS write succeeded - update with actual CID
				uri = &pdsURI
				cid = &pdsCID
			}
		}
	}

	// Create survey locally (either after PDS write or as local-only)
	now := time.Now()
	survey := &models.Survey{
		ID:         uuid.New(),
		URI:        uri,
		CID:        cid,
		AuthorDID:  authorDID,
		Slug:       slug,
		Title:      title,
		Definition: *def,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := h.queries.CreateSurvey(c.Request().Context(), survey); err != nil {
		component := templates.Error("Failed to create survey")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Redirect to the new survey
	return c.Redirect(http.StatusSeeOther, "/surveys/"+slug)
}

// SubmitResponseHTML handles survey response submission from HTML form
// POST /surveys/:slug/responses
func (h *Handlers) SubmitResponseHTML(c echo.Context) error {
	slug := c.Param("slug")

	// Get the survey
	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			component := templates.Error("Survey not found")
			return component.Render(c.Request().Context(), c.Response().Writer)
		}
		component := templates.Error("Failed to load survey")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Parse form data into answers
	answers := make(map[string]models.Answer)
	formValues, err := c.FormParams()
	if err != nil {
		component := templates.Error("Invalid form data")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	for _, question := range survey.Definition.Questions {
		if question.Type == models.QuestionTypeSingle {
			if value := formValues.Get(question.ID); value != "" {
				answers[question.ID] = models.Answer{
					SelectedOptions: []string{value},
				}
			}
		} else if question.Type == models.QuestionTypeMulti {
			if values, ok := formValues[question.ID]; ok && len(values) > 0 {
				answers[question.ID] = models.Answer{
					SelectedOptions: values,
				}
			}
		} else if question.Type == models.QuestionTypeText {
			if value := formValues.Get(question.ID); value != "" {
				answers[question.ID] = models.Answer{
					Text: value,
				}
			}
		}
	}

	// Validate answers
	if err := models.ValidateAnswers(&survey.Definition, answers); err != nil {
		component := templates.Error("Invalid answers: " + err.Error())
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Initialize response fields
	var uri *string
	var cid *string
	var voterDID *string
	var voterSession *string

	// Check if user is logged in and survey has a URI (ATProto record)
	// If both conditions are met, write response to user's PDS
	c.Logger().Infof("PDS write check: oauthStorage=%v, surveyURI=%v", h.oauthStorage != nil, survey.URI != nil)
	if h.oauthStorage != nil && survey.URI != nil {
		session, err := oauth.GetSession(c, h.oauthStorage)
		c.Logger().Infof("OAuth session lookup: session=%v, err=%v", session != nil, err)
		if err == nil && session != nil {
			c.Logger().Infof("Attempting PDS write for user %s to survey %s", session.DID, *survey.URI)
			// Generate TID for response rkey
			rkey := oauth.GenerateTID()
			atURI := fmt.Sprintf("at://%s/net.openmeet.survey.response/%s", session.DID, rkey)
			uri = &atURI
			voterDID = &session.DID

			// Convert answers map to lexicon format
			// The lexicon expects an array of {questionId, selectedOptions?, text?}
			lexiconAnswers := make([]map[string]interface{}, 0, len(answers))
			for qid, answer := range answers {
				lexAnswer := map[string]interface{}{
					"questionId": qid,
				}
				if len(answer.SelectedOptions) > 0 {
					lexAnswer["selectedOptions"] = answer.SelectedOptions
				}
				if answer.Text != "" {
					lexAnswer["text"] = answer.Text
				}
				lexiconAnswers = append(lexiconAnswers, lexAnswer)
			}

			// Build ATProto record matching lexicon format
			record := map[string]interface{}{
				"$type": "net.openmeet.survey.response",
				"subject": map[string]string{
					"uri": *survey.URI,
					"cid": *survey.CID,
				},
				"answers":   lexiconAnswers,
				"createdAt": time.Now().Format(time.RFC3339),
			}

			// Write to PDS
			pdsURI, pdsCID, err := oauth.CreateRecord(session, "net.openmeet.survey.response", rkey, record)
			if err != nil {
				// PDS write failed - log but continue with local-only response
				c.Logger().Errorf("Failed to write response to PDS: %v", err)
				uri = nil
				voterDID = nil
			} else {
				// PDS write succeeded - update with actual CID
				c.Logger().Infof("PDS write succeeded: uri=%s, cid=%s", pdsURI, pdsCID)
				uri = &pdsURI
				cid = &pdsCID
			}
		}
	}

	// If not logged in or PDS write failed, fall back to guest voting
	if voterDID == nil {
		ip := getClientIP(c)
		userAgent := c.Request().UserAgent()
		session := models.GenerateVoterSession(survey.ID, ip, userAgent)
		voterSession = &session

		// Check if already voted using session
		existingResponse, err := h.queries.GetResponseBySurveyAndVoter(
			c.Request().Context(),
			survey.ID,
			"",
			*voterSession,
		)
		if err != nil {
			component := templates.Error("Failed to check for existing response")
			return component.Render(c.Request().Context(), c.Response().Writer)
		}

		if existingResponse != nil {
			component := templates.Error("You have already submitted a response to this survey")
			return component.Render(c.Request().Context(), c.Response().Writer)
		}
	} else {
		// Check if already voted using DID
		existingResponse, err := h.queries.GetResponseBySurveyAndVoter(
			c.Request().Context(),
			survey.ID,
			*voterDID,
			"",
		)
		if err != nil {
			component := templates.Error("Failed to check for existing response")
			return component.Render(c.Request().Context(), c.Response().Writer)
		}

		if existingResponse != nil {
			component := templates.Error("You have already submitted a response to this survey")
			return component.Render(c.Request().Context(), c.Response().Writer)
		}
	}

	// Create response locally
	now := time.Now()
	response := &models.Response{
		ID:           uuid.New(),
		SurveyID:     survey.ID,
		VoterDID:     voterDID,
		VoterSession: voterSession,
		RecordURI:    uri,
		RecordCID:    cid,
		Answers:      answers,
		CreatedAt:    now,
	}

	if err := h.queries.CreateResponse(c.Request().Context(), response); err != nil {
		component := templates.Error("Failed to submit response")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Record metrics (no slug label to avoid cardinality explosion)
	telemetry.SurveyResponsesTotal.WithLabelValues("web").Inc()

	// Return thank you message
	component := templates.ThankYou(slug)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// GetResultsHTML renders the survey results page
// GET /surveys/:slug/results
func (h *Handlers) GetResultsHTML(c echo.Context) error {
	slug := c.Param("slug")

	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Survey not found")
		}
		return c.String(http.StatusInternalServerError, "Failed to load survey")
	}

	results, err := h.queries.GetSurveyResults(c.Request().Context(), survey.ID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to load results")
	}

	// Get user and profile from context
	user, profile := getUserAndProfile(c)

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.SurveyResults(survey, results, user, profile, h.posthogKey)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// GetResultsPartialHTML renders just the results partial (for HTMX polling)
// GET /surveys/:slug/results-partial
func (h *Handlers) GetResultsPartialHTML(c echo.Context) error {
	slug := c.Param("slug")

	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Survey not found")
		}
		return c.String(http.StatusInternalServerError, "Failed to load survey")
	}

	results, err := h.queries.GetSurveyResults(c.Request().Context(), survey.ID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to load results")
	}

	component := templates.ResultsPartial(survey, results)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// PublishResultsHTML publishes survey results to the author's PDS
// POST /surveys/:slug/publish-results
func (h *Handlers) PublishResultsHTML(c echo.Context) error {
	slug := c.Param("slug")

	// Get the survey
	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Survey not found")
		}
		return c.String(http.StatusInternalServerError, "Failed to load survey")
	}

	// Check if user is logged in
	if h.oauthStorage == nil {
		component := templates.Error("You must be logged in to publish results")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	session, err := oauth.GetSession(c, h.oauthStorage)
	if err != nil || session == nil {
		component := templates.Error("You must log in to publish results")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Verify survey has a URI (is an ATProto record)
	if survey.URI == nil {
		component := templates.Error("Cannot publish results for local-only surveys. Survey must be an ATProto record.")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Verify user is the survey author
	if survey.AuthorDID == nil || *survey.AuthorDID != session.DID {
		component := templates.Error("Only the survey author can publish results")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Get aggregated results from database
	results, err := h.queries.GetSurveyResults(c.Request().Context(), survey.ID)
	if err != nil {
		component := templates.Error("Failed to aggregate results")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Build question results for the lexicon format
	lexiconQuestionResults := make([]map[string]interface{}, 0, len(results.QuestionResults))
	for _, qResult := range results.QuestionResults {
		// Build option counts array
		optionCounts := make([]map[string]interface{}, 0, len(qResult.OptionCounts))
		for optionID, count := range qResult.OptionCounts {
			optionCounts = append(optionCounts, map[string]interface{}{
				"optionId": optionID,
				"count":    count,
			})
		}

		lexiconQuestionResults = append(lexiconQuestionResults, map[string]interface{}{
			"questionId":        qResult.QuestionID,
			"optionCounts":      optionCounts,
			"textResponseCount": len(qResult.TextAnswers),
		})
	}

	// Build ATProto results record matching lexicon format
	record := map[string]interface{}{
		"$type": "net.openmeet.survey.results",
		"subject": map[string]string{
			"uri": *survey.URI,
			"cid": *survey.CID,
		},
		"totalVotes":      results.TotalVotes,
		"questionResults": lexiconQuestionResults,
		"finalizedAt":     time.Now().Format(time.RFC3339),
	}

	// Generate TID for results rkey
	rkey := oauth.GenerateTID()

	// Write to PDS
	resultsURI, resultsCID, err := oauth.CreateRecord(session, "net.openmeet.survey.results", rkey, record)
	if err != nil {
		c.Logger().Errorf("Failed to write results to PDS: %v", err)
		component := templates.Error("Failed to publish results to your PDS")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Update survey with results URI and CID
	if err := h.queries.UpdateSurveyResults(c.Request().Context(), survey.ID, resultsURI, resultsCID); err != nil {
		c.Logger().Errorf("Failed to update survey with results: %v", err)
		component := templates.Error("Failed to save results reference")
		return component.Render(c.Request().Context(), c.Response().Writer)
	}

	// Redirect to results page
	return c.Redirect(http.StatusSeeOther, "/surveys/"+slug+"/results")
}

// Health Check Handlers

// DBChecker is an interface for checking database connectivity
// This allows for mocking in tests
type DBChecker interface {
	PingContext(ctx context.Context) error
}

// HealthHandlers holds health check dependencies
type HealthHandlers struct {
	db DBChecker
}

// NewHealthHandlers creates a new HealthHandlers instance
func NewHealthHandlers(db DBChecker) *HealthHandlers {
	return &HealthHandlers{
		db: db,
	}
}

// HealthResponse represents the liveness probe response
type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

// ReadinessResponse represents the readiness probe response
type ReadinessResponse struct {
	Status string            `json:"status"`
	Service string           `json:"service"`
	Checks map[string]string `json:"checks"`
}

// Health returns a basic liveness check
// GET /health
func (hh *HealthHandlers) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status:    "healthy",
		Service:   "survey-api",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Readiness returns a readiness check with DB connectivity
// GET /health/ready
func (hh *HealthHandlers) Readiness(c echo.Context) error {
	checks := make(map[string]string)
	status := "ready"

	// Check database connection
	if err := hh.db.PingContext(c.Request().Context()); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		status = "not_ready"
	} else {
		checks["database"] = "healthy"
	}

	httpStatus := http.StatusOK
	if status == "not_ready" {
		httpStatus = http.StatusServiceUnavailable
	}

	return c.JSON(httpStatus, ReadinessResponse{
		Status:  status,
		Service: "survey-api",
		Checks:  checks,
	})
}

// getUserAndProfile retrieves the authenticated user from context and fetches their profile
// Returns nil for both if user is not authenticated
func getUserAndProfile(c echo.Context) (*oauth.User, *oauth.Profile) {
	user := oauth.GetUser(c)
	if user == nil {
		return nil, nil
	}

	// Fetch profile from Bluesky
	profile, err := oauth.GetProfile(user.DID)
	if err != nil {
		// Log error but don't fail - just show user without profile
		c.Logger().Errorf("Failed to fetch profile for %s: %v", user.DID, err)
		return user, nil
	}

	return user, profile
}

// LandingPage renders the landing page with live statistics
// GET /
func (h *Handlers) LandingPage(c echo.Context) error {
	// Get statistics
	stats, err := h.queries.GetStats(c.Request().Context())
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to load statistics")
	}

	// Get user and profile from context
	user, profile := getUserAndProfile(c)

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.LandingPage(stats, user, profile, h.supportURL, h.posthogKey)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// MyDataHTML displays the overview of user's PDS data
// GET /my-data
func (h *Handlers) MyDataHTML(c echo.Context) error {
	// Check authentication
	user := oauth.GetUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Authentication required")
	}

	// Get profile
	_, profile := getUserAndProfile(c)

	// Render overview page
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.MyDataPage(user, profile, h.posthogKey)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// MyDataCollectionHTML displays records from a specific collection
// GET /my-data/:collection
func (h *Handlers) MyDataCollectionHTML(c echo.Context) error {
	// Check authentication
	user := oauth.GetUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Authentication required")
	}

	// Get session to access PDS URL
	if h.oauthStorage == nil {
		return c.String(http.StatusInternalServerError, "OAuth not configured")
	}

	session, err := oauth.GetSession(c, h.oauthStorage)
	if err != nil {
		c.Logger().Errorf("Failed to get session: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to get session")
	}
	if session == nil {
		return c.String(http.StatusUnauthorized, "Session not found")
	}

	// Get parameters
	collection := c.Param("collection")
	cursor := c.QueryParam("cursor")
	limit := 50

	// Fetch records from PDS
	records, err := oauth.ListRecords(session.PDSUrl, session.DID, collection, cursor, limit)
	if err != nil {
		c.Logger().Errorf("Failed to list records from %s: %v", collection, err)
		return c.String(http.StatusInternalServerError, "Failed to fetch records: "+err.Error())
	}

	// Get profile
	_, profile := getUserAndProfile(c)

	// Render collection page
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.MyDataCollectionPage(user, profile, collection, records.Records, records.Cursor, h.posthogKey)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// MyDataRecordHTML displays a single record for editing
// GET /my-data/:collection/:rkey
func (h *Handlers) MyDataRecordHTML(c echo.Context) error {
	// Check authentication
	user := oauth.GetUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Authentication required")
	}

	// Get session to access PDS URL
	if h.oauthStorage == nil {
		return c.String(http.StatusInternalServerError, "OAuth not configured")
	}

	session, err := oauth.GetSession(c, h.oauthStorage)
	if err != nil {
		c.Logger().Errorf("Failed to get session: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to get session")
	}
	if session == nil {
		return c.String(http.StatusUnauthorized, "Session not found")
	}

	// Get parameters
	collection := c.Param("collection")
	rkey := c.Param("rkey")

	// Fetch all records to find the specific one
	// (ATProto doesn't have a getRecord endpoint for public repos)
	records, err := oauth.ListRecords(session.PDSUrl, session.DID, collection, "", 100)
	if err != nil {
		c.Logger().Errorf("Failed to list records from %s: %v", collection, err)
		return c.String(http.StatusInternalServerError, "Failed to fetch records: "+err.Error())
	}

	// Find the specific record
	var record *oauth.PDSRecord
	for i := range records.Records {
		if records.Records[i].RKey == rkey {
			record = &records.Records[i]
			break
		}
	}

	if record == nil {
		return c.String(http.StatusNotFound, "Record not found")
	}

	// Get profile
	_, profile := getUserAndProfile(c)

	// Render record edit page
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	component := templates.MyDataRecordPage(user, profile, collection, record, h.posthogKey)
	return component.Render(c.Request().Context(), c.Response().Writer)
}

// UpdateRecordHTML updates a record via form submission
// POST /my-data/:collection/:rkey
func (h *Handlers) UpdateRecordHTML(c echo.Context) error {
	// Check authentication
	user := oauth.GetUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Authentication required")
	}

	// Get session
	if h.oauthStorage == nil {
		return c.String(http.StatusInternalServerError, "OAuth not configured")
	}

	session, err := oauth.GetSession(c, h.oauthStorage)
	if err != nil {
		c.Logger().Errorf("Failed to get session: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to get session")
	}
	if session == nil {
		return c.String(http.StatusUnauthorized, "Session not found")
	}

	// Get parameters
	collection := c.Param("collection")
	rkey := c.Param("rkey")
	recordJSON := c.FormValue("record")

	// Parse JSON
	var recordData map[string]interface{}
	if err := json.Unmarshal([]byte(recordJSON), &recordData); err != nil {
		return c.String(http.StatusBadRequest, "Invalid JSON: "+err.Error())
	}

	// Update record on PDS
	_, _, err = oauth.UpdateRecord(session, collection, rkey, recordData)
	if err != nil {
		c.Logger().Errorf("Failed to update record %s/%s: %v", collection, rkey, err)
		return c.String(http.StatusInternalServerError, "Failed to update record: "+err.Error())
	}

	// Redirect back to collection view
	return c.Redirect(http.StatusSeeOther, "/my-data/"+collection)
}

// ShortSlugURL provides a short URL redirect to survey by slug
// GET /s/:slug
func (h *Handlers) ShortSlugURL(c echo.Context) error {
	slug := c.Param("slug")

	// Verify survey exists
	survey, err := h.queries.GetSurveyBySlug(c.Request().Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Survey not found")
		}
		return InternalServerError(c, "Failed to retrieve survey", err)
	}

	// Redirect to full survey URL
	return c.Redirect(http.StatusSeeOther, "/surveys/"+survey.Slug)
}

// ATProtoURL provides canonical AT Protocol URL redirect
// GET /at/:did/:rkey
func (h *Handlers) ATProtoURL(c echo.Context) error {
	did := c.Param("did")
	rkey := c.Param("rkey")

	// Construct AT URI
	uri := fmt.Sprintf("at://%s/net.openmeet.survey/%s", did, rkey)

	// Look up survey by URI
	survey, err := h.queries.GetSurveyByURI(c.Request().Context(), uri)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Survey not found")
		}
		return InternalServerError(c, "Failed to retrieve survey", err)
	}

	// Redirect to survey by slug
	return c.Redirect(http.StatusSeeOther, "/surveys/"+survey.Slug)
}

// DeleteRecordsHTML deletes multiple records via form submission
// POST /my-data/delete
func (h *Handlers) DeleteRecordsHTML(c echo.Context) error {
	// Check authentication
	user := oauth.GetUser(c)
	if user == nil {
		return c.String(http.StatusUnauthorized, "Authentication required")
	}

	// Get session
	if h.oauthStorage == nil {
		return c.String(http.StatusInternalServerError, "OAuth not configured")
	}

	session, err := oauth.GetSession(c, h.oauthStorage)
	if err != nil {
		c.Logger().Errorf("Failed to get session: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to get session")
	}
	if session == nil {
		return c.String(http.StatusUnauthorized, "Session not found")
	}

	// Get parameters
	collection := c.FormValue("collection")
	rkeys := c.Request().Form["rkeys"]

	if len(rkeys) == 0 {
		return c.String(http.StatusBadRequest, "No records selected")
	}

	// Delete each record
	for _, rkey := range rkeys {
		err := oauth.DeleteRecord(session, collection, rkey)
		if err != nil {
			// Continue with other deletions even if one fails
			c.Logger().Errorf("Failed to delete record %s/%s: %v", collection, rkey, err)
		}
	}

	// Redirect back to collection view
	return c.Redirect(http.StatusSeeOther, "/my-data/"+collection)
}

// GenerateSurvey handles AI survey generation requests
// POST /api/v1/surveys/generate
func (h *Handlers) GenerateSurvey(c echo.Context) error {
	// Parse request
	var req GenerateSurveyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request body",
			Details: err.Error(),
		})
	}

	// Check consent
	if !req.Consent {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "AI generation requires explicit consent for OpenAI processing",
		})
	}

	// Validate description
	if strings.TrimSpace(req.Description) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Description cannot be empty",
		})
	}

	// Check if generator is configured
	if h.generator == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: "AI survey generation is not available",
		})
	}

	// Check if rate limiter is configured
	if h.generatorRL == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: "AI survey generation is not available",
		})
	}

	// Get user context (authenticated vs anonymous)
	user := oauth.GetUser(c)
	var allowed bool
	var userID string
	var userType string

	if user != nil {
		// Authenticated user - check DID-based rate limit
		allowed = h.generatorRL.AllowAuthenticated(user.DID)
		userID = user.DID
		userType = "authenticated"
	} else {
		// Anonymous user - check IP-based rate limit
		ip := getClientIP(c)
		allowed = h.generatorRL.AllowAnonymous(ip)
		userID = ip
		userType = "anonymous"
	}

	if !allowed {
		// Record rate limit hit metric
		if user != nil {
			telemetry.AIRateLimitHitsTotal.WithLabelValues("authenticated").Inc()
		} else {
			telemetry.AIRateLimitHitsTotal.WithLabelValues("anonymous").Inc()
		}
		telemetry.AIGenerationsTotal.WithLabelValues("rate_limited").Inc()

		// Log rate limit error
		if h.generationLog != nil {
			// We don't have system prompt yet, so we'll use empty string
			_ = h.generationLog.LogError(
				c.Request().Context(),
				userID,
				userType,
				req.Description,
				"", // System prompt not available yet
				"rate_limited",
				"Rate limit exceeded",
				0, 0, 0.0, 0,
			)
		}

		return c.JSON(http.StatusTooManyRequests, ErrorResponse{
			Error: "Rate limit exceeded for AI generation. Please try again later.",
		})
	}

	// Validate user input first (before building combined prompt)
	if err := h.generator.ValidateInput(req.Description); err != nil {
		telemetry.AIGenerationsTotal.WithLabelValues("error").Inc()

		if h.generationLog != nil {
			_ = h.generationLog.LogError(
				c.Request().Context(),
				userID,
				userType,
				req.Description,
				"",
				"validation_failed",
				err.Error(),
				0, 0, 0.0, 0,
			)
		}

		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Build prompt
	prompt := req.Description
	isRefinement := req.ExistingJSON != ""
	if isRefinement {
		prompt = fmt.Sprintf("Existing survey JSON: %s\n\nModification request: %s", req.ExistingJSON, req.Description)
	}

	// Record duration metric
	start := time.Now()

	// Call generator - use GenerateRaw for refinement (already validated user input)
	var result *generator.GenerateResult
	var err error
	if isRefinement {
		result, err = h.generator.GenerateRaw(c.Request().Context(), prompt)
	} else {
		result, err = h.generator.Generate(c.Request().Context(), prompt)
	}

	// Record duration
	duration := time.Since(start).Seconds()
	durationMS := int(duration * 1000)
	telemetry.AIGenerationDuration.Observe(duration)

	if err != nil {
		// Determine error status and message for logging
		var status string
		var errorMessage string

		// Check error type for specific responses
		if errors.Is(err, generator.ErrInputTooLong) || errors.Is(err, generator.ErrEmptyInput) || errors.Is(err, generator.ErrBlockedPattern) {
			status = "validation_failed"
			errorMessage = err.Error()
			telemetry.AIGenerationsTotal.WithLabelValues("error").Inc()

			// Log validation error
			if h.generationLog != nil {
				_ = h.generationLog.LogError(
					c.Request().Context(),
					userID,
					userType,
					req.Description,
					"", // System prompt not available on validation failure
					status,
					errorMessage,
					0, 0, 0.0,
					durationMS,
				)
			}

			// Return specific error response
			if errors.Is(err, generator.ErrInputTooLong) {
				return c.JSON(http.StatusBadRequest, ErrorResponse{
					Error:   "Input too long",
					Details: err.Error(),
				})
			}
			if errors.Is(err, generator.ErrEmptyInput) {
				return c.JSON(http.StatusBadRequest, ErrorResponse{
					Error:   "Input cannot be empty",
					Details: err.Error(),
				})
			}
			if errors.Is(err, generator.ErrBlockedPattern) {
				return c.JSON(http.StatusBadRequest, ErrorResponse{
					Error:   "Input contains blocked pattern",
					Details: "Your input was flagged for potentially unsafe content",
				})
			}
		}

		if errors.Is(err, generator.ErrCostLimitExceeded) {
			status = "error"
			errorMessage = "Cost limit exceeded"
			telemetry.AIGenerationsTotal.WithLabelValues("budget_exceeded").Inc()

			// Log cost limit error
			if h.generationLog != nil {
				_ = h.generationLog.LogError(
					c.Request().Context(),
					userID,
					userType,
					req.Description,
					"", // System prompt not available
					status,
					errorMessage,
					0, 0, 0.0,
					durationMS,
				)
			}

			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{
				Error: "AI generation budget exceeded. Please try again later.",
			})
		}

		// Generic error
		status = "error"
		errorMessage = err.Error()
		telemetry.AIGenerationsTotal.WithLabelValues("error").Inc()

		// Log generic error
		if h.generationLog != nil {
			_ = h.generationLog.LogError(
				c.Request().Context(),
				userID,
				userType,
				req.Description,
				"", // System prompt not available
				status,
				errorMessage,
				0, 0, 0.0,
				durationMS,
			)
		}

		c.Logger().Errorf("AI generation failed: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "AI generation failed",
			Details: err.Error(),
		})
	}

	// Record success metrics
	telemetry.AIGenerationsTotal.WithLabelValues("success").Inc()
	telemetry.AITokensTotal.WithLabelValues("input").Add(float64(result.InputTokens))
	telemetry.AITokensTotal.WithLabelValues("output").Add(float64(result.OutputTokens))

	// Update daily cost (additive - gauge tracks cumulative cost for the day)
	telemetry.AIDailyCostUSD.Add(result.EstimatedCost)

	// Log successful generation
	if h.generationLog != nil {
		_ = h.generationLog.LogSuccess(
			c.Request().Context(),
			userID,
			userType,
			req.Description,
			result.SystemPrompt,
			result.RawResponse,
			result,
			durationMS,
		)
	}

	// Return success response
	return c.JSON(http.StatusOK, GenerateSurveyResponse{
		Definition:   result.Definition,
		TokensUsed:   result.InputTokens + result.OutputTokens,
		Cost:         result.EstimatedCost,
		NeedsCaptcha: false, // MVP: no captcha implementation yet
	})
}
