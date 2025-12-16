package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/openmeet-team/survey/internal/db"
	"github.com/openmeet-team/survey/internal/models"
	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a test database connection
// This function creates a connection using the db package
func setupTestDB(t *testing.T) *sql.DB {
	cfg, err := db.ConfigFromEnv()
	if err != nil {
		t.Skipf("Skipping test: database config error: %v", err)
	}

	dbConn, err := db.Connect(context.Background(), cfg)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
	}

	return dbConn
}

// mockPDSServer creates a test HTTP server that mimics PDS createRecord endpoint
type mockPDSServer struct {
	server       *httptest.Server
	lastRequest  *http.Request
	lastBody     map[string]interface{}
	response     map[string]interface{}
	statusCode   int
	callCount    int
	requireDPoP  bool
}

func newMockPDSServer() *mockPDSServer {
	m := &mockPDSServer{
		statusCode:  http.StatusOK,
		requireDPoP: true,
		response: map[string]interface{}{
			"uri": "at://did:plc:test123/net.openmeet.survey/test123",
			"cid": "bafytest123",
		},
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.callCount++
		m.lastRequest = r

		// Parse request body
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			m.lastBody = body
		}

		if m.requireDPoP {
			// Check Authorization header
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "DPoP ") {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing DPoP authorization"})
				return
			}

			// Check DPoP header
			dpopHeader := r.Header.Get("DPoP")
			if dpopHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing DPoP proof"})
				return
			}
		}

		// Return mock response
		w.WriteHeader(m.statusCode)
		json.NewEncoder(w).Encode(m.response)
	}))

	return m
}

func (m *mockPDSServer) Close() {
	m.server.Close()
}

func (m *mockPDSServer) URL() string {
	return m.server.URL
}

// TestCreateSurveyHTML_PDSWrite_Integration tests the full PDS write flow
// This test requires a database connection
func TestCreateSurveyHTML_PDSWrite_Integration(t *testing.T) {
	// Set up test database
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Set up Echo and handlers
	e := echo.New()
	queries := db.NewQueries(dbConn)
	oauthStorage := oauth.NewStorage(dbConn)

	// Create handlers with OAuth support (nil config for test)
	h := NewHandlersWithOAuth(queries, oauthStorage, nil)

	// Create mock PDS server
	mockPDS := newMockPDSServer()
	defer mockPDS.Close()

	// Create a valid OAuth session with all required fields
	sessionID := uuid.New().String()
	tokenExpiresAt := time.Now().Add(1 * time.Hour)
	session := oauth.OAuthSession{
		ID:             sessionID,
		DID:            "did:plc:test123",
		AccessToken:    "test-access-token",
		RefreshToken:   "test-refresh-token",
		DPoPKey:        oauth.GenerateSecretJWK(), // Generate a real JWK for testing
		PDSUrl:         mockPDS.URL(),
		TokenExpiresAt: &tokenExpiresAt,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}

	// Save session to database
	err := oauthStorage.CreateSession(context.Background(), session)
	require.NoError(t, err)

	// Clean up session after test
	defer oauthStorage.DeleteSession(context.Background(), sessionID)

	// Prepare survey creation form
	definition := `{
		"questions": [
			{
				"id": "q1",
				"text": "Integration test question",
				"type": "single",
				"required": true,
				"options": [
					{"id": "yes", "text": "Yes"},
					{"id": "no", "text": "No"}
				]
			}
		],
		"anonymous": false
	}`

	form := url.Values{}
	form.Set("slug", fmt.Sprintf("integration-test-%d", time.Now().Unix()))
	form.Set("definition", definition)

	req := httptest.NewRequest(http.MethodPost, "/surveys", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)

	// Add session cookie
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: sessionID,
	})

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Execute handler
	err = h.CreateSurveyHTML(c)
	require.NoError(t, err)

	// ====== ASSERTIONS ======

	t.Logf("Response status: %d", rec.Code)
	t.Logf("Response body: %s", rec.Body.String())
	t.Logf("PDS call count: %d", mockPDS.callCount)

	// CRITICAL TEST: Was the PDS called?
	if mockPDS.callCount == 0 {
		t.Error("FAILED: PDS was not called! This indicates the PDS write path is not being executed.")
		t.Log("Possible reasons:")
		t.Log("  1. oauthStorage is nil")
		t.Log("  2. oauth.GetSession() failed to retrieve the session")
		t.Log("  3. Session is missing AccessToken or PDSUrl")
		t.Log("  4. Code path is not reaching the PDS write block")
	} else {
		t.Log("SUCCESS: PDS was called")

		// Verify PDS request
		assert.NotNil(t, mockPDS.lastRequest)
		assert.Equal(t, "POST", mockPDS.lastRequest.Method)
		assert.Contains(t, mockPDS.lastRequest.URL.Path, "com.atproto.repo.createRecord")

		// Verify request body
		assert.NotNil(t, mockPDS.lastBody)
		assert.Equal(t, session.DID, mockPDS.lastBody["repo"])
		assert.Equal(t, "net.openmeet.survey", mockPDS.lastBody["collection"])

		record := mockPDS.lastBody["record"].(map[string]interface{})
		assert.Equal(t, "net.openmeet.survey", record["$type"])
		assert.NotNil(t, record["createdAt"])
	}

	// Verify survey was created (response should redirect or show success)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusSeeOther || rec.Code == http.StatusCreated)
}

// TestSubmitResponseHTML_PDSWrite_Integration tests response PDS write flow
func TestSubmitResponseHTML_PDSWrite_Integration(t *testing.T) {
	// Set up test database
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Set up Echo and handlers
	e := echo.New()
	queries := db.NewQueries(dbConn)
	oauthStorage := oauth.NewStorage(dbConn)

	h := NewHandlersWithOAuth(queries, oauthStorage, nil)

	// Create mock PDS server
	mockPDS := newMockPDSServer()
	defer mockPDS.Close()

	mockPDS.response = map[string]interface{}{
		"uri": "at://did:plc:voter456/net.openmeet.survey.response/resp123",
		"cid": "bafyresp123",
	}

	// Create survey WITH URI (ATProto record)
	surveyID := uuid.New()
	rkey := fmt.Sprintf("survey%d", time.Now().UnixNano())
	surveyURI := fmt.Sprintf("at://did:plc:author123/net.openmeet.survey/%s", rkey)
	surveyCID := "bafysurvey123"
	authorDID := "did:plc:author123"
	slug := fmt.Sprintf("pds-test-survey-%d", time.Now().UnixNano())

	survey := &models.Survey{
		ID:        surveyID,
		URI:       &surveyURI,
		CID:       &surveyCID,
		AuthorDID: &authorDID,
		Slug:      slug,
		Title:     "PDS Test Survey",
		Definition: models.SurveyDefinition{
			Questions: []models.Question{
				{
					ID:       "q1",
					Text:     "Test Question",
					Type:     models.QuestionTypeSingle,
					Required: true,
					Options: []models.Option{
						{ID: "a", Text: "A"},
						{ID: "b", Text: "B"},
					},
				},
			},
			Anonymous: false,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := queries.CreateSurvey(context.Background(), survey)
	require.NoError(t, err)

	// Clean up survey after test
	defer func() {
		// Note: You'd need a DeleteSurvey method or manual cleanup
	}()

	// Create OAuth session for voter
	sessionID := uuid.New().String()
	tokenExpiresAt := time.Now().Add(1 * time.Hour)
	voterSession := oauth.OAuthSession{
		ID:             sessionID,
		DID:            "did:plc:voter456",
		AccessToken:    "voter-access-token",
		RefreshToken:   "voter-refresh-token",
		DPoPKey:        oauth.GenerateSecretJWK(),
		PDSUrl:         mockPDS.URL(),
		TokenExpiresAt: &tokenExpiresAt,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}

	err = oauthStorage.CreateSession(context.Background(), voterSession)
	require.NoError(t, err)

	defer oauthStorage.DeleteSession(context.Background(), sessionID)

	// Submit response form
	form := url.Values{}
	form.Set("q1", "a")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/surveys/%s/responses", slug), strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("User-Agent", "TestAgent/1.0")

	// Add session cookie
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: sessionID,
	})

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues(slug)

	// Execute handler
	err = h.SubmitResponseHTML(c)
	require.NoError(t, err)

	// ====== ASSERTIONS ======

	t.Logf("Response status: %d", rec.Code)
	t.Logf("Response body: %s", rec.Body.String())
	t.Logf("PDS call count: %d", mockPDS.callCount)

	// CRITICAL TEST: Was the PDS called?
	if mockPDS.callCount == 0 {
		t.Error("FAILED: PDS was not called for response submission!")
		t.Log("Possible reasons:")
		t.Log("  1. oauthStorage is nil")
		t.Log("  2. Survey URI is nil")
		t.Log("  3. oauth.GetSession() failed")
		t.Log("  4. Session is missing AccessToken or PDSUrl")
		t.Log("  5. Code path is not reaching the PDS write block at line 618-672")
	} else {
		t.Log("SUCCESS: PDS was called for response")

		// Verify PDS request
		assert.NotNil(t, mockPDS.lastRequest)
		assert.Equal(t, "POST", mockPDS.lastRequest.Method)
		assert.Contains(t, mockPDS.lastRequest.URL.Path, "com.atproto.repo.createRecord")

		// Verify request body
		assert.NotNil(t, mockPDS.lastBody)
		assert.Equal(t, voterSession.DID, mockPDS.lastBody["repo"])
		assert.Equal(t, "net.openmeet.survey.response", mockPDS.lastBody["collection"])

		record := mockPDS.lastBody["record"].(map[string]interface{})
		assert.Equal(t, "net.openmeet.survey.response", record["$type"])

		// Verify subject (strongRef to survey)
		subject := record["subject"].(map[string]interface{})
		assert.Equal(t, surveyURI, subject["uri"])
		assert.Equal(t, surveyCID, subject["cid"])
	}

	// Verify response was created
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusSeeOther)
}

// TestPDSWriteConditions_Debug helps debug why PDS writes aren't happening
func TestPDSWriteConditions_Debug(t *testing.T) {
	t.Run("check condition: oauthStorage is nil", func(t *testing.T) {
		e, mq, h := setupTest() // Uses NewHandlers() which sets oauthStorage to nil

		assert.Nil(t, h.oauthStorage, "oauthStorage should be nil when using NewHandlers()")

		// This simulates what happens in production if oauthStorage is not initialized
		if h.oauthStorage == nil {
			t.Log("INFO: When oauthStorage is nil, PDS writes will NOT happen")
			t.Log("INFO: Handler was created with NewHandlers() instead of NewHandlersWithOAuth()")
		}

		_ = e
		_ = mq
	})

	t.Run("check condition: session cookie missing", func(t *testing.T) {
		e := echo.New()

		req := httptest.NewRequest(http.MethodPost, "/surveys", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Try to get session cookie
		cookie, err := c.Cookie("session")

		assert.Error(t, err, "Should error when session cookie is missing")
		assert.Nil(t, cookie)

		t.Log("INFO: When session cookie is missing, oauth.GetSession() will return nil")
		t.Log("INFO: This causes PDS write to be skipped")
	})

	t.Run("check condition: session has empty AccessToken", func(t *testing.T) {
		session := &oauth.OAuthSession{
			ID:      "test",
			DID:     "did:plc:test",
			PDSUrl:  "https://pds.example.com",
			DPoPKey: oauth.GenerateSecretJWK(),
			// AccessToken is empty!
		}

		hasValidToken := session.AccessToken != "" && session.PDSUrl != ""

		assert.False(t, hasValidToken, "Session should be invalid without AccessToken")

		t.Log("INFO: When AccessToken is empty, condition at line 500 will be false")
		t.Log("INFO: This causes PDS write to be skipped")
	})

	t.Run("check condition: session has empty PDSUrl", func(t *testing.T) {
		session := &oauth.OAuthSession{
			ID:          "test",
			DID:         "did:plc:test",
			AccessToken: "test-token",
			DPoPKey:     oauth.GenerateSecretJWK(),
			// PDSUrl is empty!
		}

		hasValidToken := session.AccessToken != "" && session.PDSUrl != ""

		assert.False(t, hasValidToken, "Session should be invalid without PDSUrl")

		t.Log("INFO: When PDSUrl is empty, condition at line 500 will be false")
		t.Log("INFO: This causes PDS write to be skipped")
	})
}
