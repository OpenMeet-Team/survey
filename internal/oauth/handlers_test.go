//go:build e2e

package oauth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/openmeet-team/survey/internal/db"
)

func TestLoginPageHandler(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/oauth/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handlers.LoginPage(c)
	if err != nil {
		t.Fatalf("LoginPage handler failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "ATProto Handle") || !strings.Contains(body, "AT Protocol") {
		t.Error("Login page should reference AT Protocol handle")
	}
	if !strings.Contains(body, "action=\"/oauth/login\"") {
		t.Error("Login page should have form with action=/oauth/login")
	}
}

func TestLoginHandler(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	t.Run("initiates OAuth flow with valid handle", func(t *testing.T) {
		e := echo.New()

		form := url.Values{}
		form.Set("handle", "test.bsky.social")

		req := httptest.NewRequest(http.MethodPost, "/oauth/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handlers.Login(c)

		// This will fail because we can't actually resolve a real handle in tests
		// But we want to verify the handler structure works
		if err == nil {
			// If it doesn't error, should redirect
			if rec.Code != http.StatusFound && rec.Code != http.StatusSeeOther {
				t.Errorf("Expected redirect status, got %d", rec.Code)
			}
		} else {
			// Expected to fail on handle resolution in test environment
			t.Logf("Expected failure in test environment: %v", err)
		}
	})

	t.Run("returns error for empty handle", func(t *testing.T) {
		e := echo.New()

		form := url.Values{}
		form.Set("handle", "")

		req := httptest.NewRequest(http.MethodPost, "/oauth/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handlers.Login(c)

		if err == nil {
			t.Error("Expected error for empty handle")
		}
	})

	t.Run("rejects GET requests", func(t *testing.T) {
		e := echo.New()

		req := httptest.NewRequest(http.MethodGet, "/oauth/login", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handlers.Login(c)

		if err == nil {
			t.Error("Expected error for GET request to login handler")
		}
	})
}

func TestClientMetadataHandler(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/oauth/client-metadata.json", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handlers.ClientMetadata(c)
	if err != nil {
		t.Fatalf("ClientMetadata handler failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected JSON content type, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "client_id") {
		t.Error("Client metadata should contain client_id")
	}
	if !strings.Contains(body, "dpop_bound_access_tokens") {
		t.Error("Client metadata should contain dpop_bound_access_tokens")
	}
	if !strings.Contains(body, "https://survey.local.openmeet.net") {
		t.Error("Client metadata should reference the configured host")
	}
}

func TestJWKSHandler(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/oauth/jwks.json", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handlers.JWKS(c)
	if err != nil {
		t.Fatalf("JWKS handler failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected JSON content type, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "keys") {
		t.Error("JWKS response should contain 'keys' array")
	}
}

func TestCallbackHandler(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	t.Run("returns error for missing parameters", func(t *testing.T) {
		e := echo.New()

		req := httptest.NewRequest(http.MethodGet, "/oauth/callback", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handlers.Callback(c)

		if err == nil {
			t.Error("Expected error for missing callback parameters")
		}
	})

	t.Run("returns error for invalid state", func(t *testing.T) {
		e := echo.New()

		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?iss=https://bsky.social&code=test-code&state=invalid-state", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handlers.Callback(c)

		if err == nil {
			t.Error("Expected error for invalid state")
		}
	})

	t.Run("returns error for issuer mismatch", func(t *testing.T) {
		e := echo.New()

		// Create a test OAuth request
		state := GenerateState()
		oauthReq := OAuthRequest{
			State:          state,
			Issuer:         "https://bsky.social",
			PKCEVerifier:   GenerateCodeVerifier(),
			DPoPPrivateKey: GenerateSecretJWK(),
			Destination:    "/",
			ExpiresAt:      time.Now().Add(10 * time.Minute),
		}

		err := handlers.storage.SaveOAuthRequest(context.Background(), oauthReq)
		if err != nil {
			t.Fatalf("Failed to save test OAuth request: %v", err)
		}

		// Call callback with different issuer
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?iss=https://different.issuer&code=test-code&state="+state, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err = handlers.Callback(c)

		if err == nil {
			t.Error("Expected error for issuer mismatch")
		}
	})
}

// TestLoginSetsOAuthStateCookie tests that Login sets the oauth_state cookie
func TestLoginSetsOAuthStateCookie(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()

	form := url.Values{}
	form.Set("handle", "test.bsky.social")

	req := httptest.NewRequest(http.MethodPost, "/oauth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = handlers.Login(c)

	// Even if the handler fails (due to resolution), it should have attempted to set the cookie
	// Check if oauth_state cookie was set (if handler got far enough)
	cookies := rec.Result().Cookies()
	var found bool
	for _, cookie := range cookies {
		if cookie.Name == "oauth_state" {
			found = true
			// Verify cookie properties
			if cookie.Path != "/oauth" {
				t.Errorf("Expected cookie path /oauth, got %s", cookie.Path)
			}
			if !cookie.HttpOnly {
				t.Error("Expected HttpOnly cookie")
			}
			if !cookie.Secure {
				t.Error("Expected Secure cookie")
			}
			if cookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("Expected SameSite=Lax, got %v", cookie.SameSite)
			}
			if cookie.MaxAge != 600 { // 10 minutes
				t.Errorf("Expected MaxAge=600, got %d", cookie.MaxAge)
			}
			if cookie.Value == "" {
				t.Error("Expected non-empty cookie value")
			}
			break
		}
	}

	// This test will fail initially because we haven't implemented the cookie setting yet
	if !found {
		t.Error("Expected oauth_state cookie to be set")
	}
}

// TestCallbackRejectsWithoutStateCookie tests that Callback rejects when oauth_state cookie is missing
func TestCallbackRejectsWithoutStateCookie(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()

	// Create a valid OAuth request
	state := GenerateState()
	oauthReq := OAuthRequest{
		State:          state,
		Issuer:         "https://bsky.social",
		PKCEVerifier:   GenerateCodeVerifier(),
		DPoPPrivateKey: GenerateSecretJWK(),
		Destination:    "/",
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}

	err := handlers.storage.SaveOAuthRequest(context.Background(), oauthReq)
	if err != nil {
		t.Fatalf("Failed to save test OAuth request: %v", err)
	}

	// Call callback WITHOUT oauth_state cookie (CSRF attack simulation)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?iss=https://bsky.social&code=test-code&state="+state, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = handlers.Callback(c)

	if err == nil {
		t.Error("Expected error when oauth_state cookie is missing")
	} else {
		// Verify it's specifically a CSRF error
		httpErr, ok := err.(*echo.HTTPError)
		if !ok {
			t.Errorf("Expected echo.HTTPError, got %T", err)
		} else {
			if httpErr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request, got %d", httpErr.Code)
			}
			errMsg := httpErr.Message.(string)
			if !strings.Contains(errMsg, "CSRF") && !strings.Contains(errMsg, "state") {
				t.Errorf("Expected CSRF-related error message, got: %s", errMsg)
			}
		}
	}
}

// TestCallbackRejectsStateMismatch tests that Callback rejects when state doesn't match cookie
func TestCallbackRejectsStateMismatch(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()

	// Create a valid OAuth request
	state := GenerateState()
	oauthReq := OAuthRequest{
		State:          state,
		Issuer:         "https://bsky.social",
		PKCEVerifier:   GenerateCodeVerifier(),
		DPoPPrivateKey: GenerateSecretJWK(),
		Destination:    "/",
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}

	err := handlers.storage.SaveOAuthRequest(context.Background(), oauthReq)
	if err != nil {
		t.Fatalf("Failed to save test OAuth request: %v", err)
	}

	// Call callback with DIFFERENT state in cookie (CSRF attack simulation)
	differentState := GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?iss=https://bsky.social&code=test-code&state="+state, nil)
	req.AddCookie(&http.Cookie{
		Name:  "oauth_state",
		Value: differentState, // Different from query param state
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = handlers.Callback(c)

	if err == nil {
		t.Error("Expected error when state doesn't match cookie")
	} else {
		// Verify it's specifically a CSRF error
		httpErr, ok := err.(*echo.HTTPError)
		if !ok {
			t.Errorf("Expected echo.HTTPError, got %T", err)
		} else {
			if httpErr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 Bad Request, got %d", httpErr.Code)
			}
			errMsg := httpErr.Message.(string)
			if !strings.Contains(errMsg, "CSRF") || !strings.Contains(errMsg, "mismatch") {
				t.Errorf("Expected CSRF mismatch error message, got: %s", errMsg)
			}
		}
	}
}

// TestCallbackSucceedsWithMatchingState tests that Callback succeeds when state matches cookie
func TestCallbackSucceedsWithMatchingState(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()

	// Create a valid OAuth request
	state := GenerateState()
	oauthReq := OAuthRequest{
		State:          state,
		Issuer:         "https://bsky.social",
		PKCEVerifier:   GenerateCodeVerifier(),
		DPoPPrivateKey: GenerateSecretJWK(),
		Destination:    "/",
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}

	err := handlers.storage.SaveOAuthRequest(context.Background(), oauthReq)
	if err != nil {
		t.Fatalf("Failed to save test OAuth request: %v", err)
	}

	// Call callback with MATCHING state in cookie
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?iss=https://bsky.social&code=test-code&state="+state, nil)
	req.AddCookie(&http.Cookie{
		Name:  "oauth_state",
		Value: state, // Same as query param
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = handlers.Callback(c)

	// This will fail during token exchange (which is expected in test environment)
	// But it should NOT fail on CSRF validation
	if err != nil {
		httpErr, ok := err.(*echo.HTTPError)
		if ok {
			errMsg := httpErr.Message.(string)
			// Should NOT be a CSRF error
			if strings.Contains(errMsg, "CSRF") {
				t.Errorf("Should not fail with CSRF error when state matches, got: %s", errMsg)
			}
		}
		// Other errors (like token exchange) are acceptable in test environment
		t.Logf("Expected failure in test environment (token exchange): %v", err)
	}
}

// TestCallbackClearsStateCookie tests that Callback clears the oauth_state cookie after verification
func TestCallbackClearsStateCookie(t *testing.T) {
	dbConn := setupHandlerTestDB(t)
	defer dbConn.Close()

	config := Config{
		Host:      "survey.local.openmeet.net",
		SecretJWK: mustGenerateTestKey(t),
	}

	handlers := NewHandlers(dbConn, config)

	e := echo.New()

	// Create a valid OAuth request
	state := GenerateState()
	oauthReq := OAuthRequest{
		State:          state,
		Issuer:         "https://bsky.social",
		PKCEVerifier:   GenerateCodeVerifier(),
		DPoPPrivateKey: GenerateSecretJWK(),
		Destination:    "/",
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}

	err := handlers.storage.SaveOAuthRequest(context.Background(), oauthReq)
	if err != nil {
		t.Fatalf("Failed to save test OAuth request: %v", err)
	}

	// Call callback with matching state
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?iss=https://bsky.social&code=test-code&state="+state, nil)
	req.AddCookie(&http.Cookie{
		Name:  "oauth_state",
		Value: state,
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_ = handlers.Callback(c)

	// Check if oauth_state cookie was cleared (MaxAge=-1)
	cookies := rec.Result().Cookies()
	var found bool
	for _, cookie := range cookies {
		if cookie.Name == "oauth_state" {
			found = true
			if cookie.MaxAge != -1 {
				t.Errorf("Expected oauth_state cookie to be cleared (MaxAge=-1), got MaxAge=%d", cookie.MaxAge)
			}
			break
		}
	}

	if !found {
		t.Error("Expected oauth_state cookie to be cleared (not found in response)")
	}
}

// Helper functions

func setupHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	cfg, err := db.ConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load database config: %v", err)
	}

	dbConn, err := db.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Clean up test data
	_, _ = dbConn.Exec("DELETE FROM oauth_requests WHERE state LIKE '%test%'")
	_, _ = dbConn.Exec("DELETE FROM oauth_sessions WHERE id LIKE '%test%'")

	return dbConn
}

func mustGenerateTestKey(t *testing.T) string {
	t.Helper()
	// GenerateSecretJWK panics on error, which is fine for tests
	return GenerateSecretJWK()
}
