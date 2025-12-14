package oauth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// Config holds OAuth handler configuration
type Config struct {
	Host      string // Public hostname (e.g., survey.openmeet.net)
	SecretJWK string // Signing key (JWK format)
}

// Handlers provides OAuth HTTP handlers
type Handlers struct {
	storage *Storage
	config  Config
}

// normalizeHost strips protocol prefix from host if present
// e.g., "https://survey.openmeet.net" -> "survey.openmeet.net"
func normalizeHost(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	return host
}

// NewHandlers creates a new Handlers instance
func NewHandlers(db *sql.DB, config Config) *Handlers {
	// Normalize the host to ensure it's just the hostname without protocol
	config.Host = normalizeHost(config.Host)
	return &Handlers{
		storage: NewStorage(db),
		config:  config,
	}
}

// ClientMetadata represents OAuth client metadata
// See: https://atproto.com/specs/oauth#client-metadata
type ClientMetadata struct {
	ClientID                    string   `json:"client_id"`
	ClientName                  string   `json:"client_name,omitempty"`
	ApplicationType             string   `json:"application_type,omitempty"`
	GrantTypes                  []string `json:"grant_types"`
	Scope                       string   `json:"scope"`
	ResponseTypes               []string `json:"response_types"`
	RedirectURIs                []string `json:"redirect_uris"`
	DPopBoundAccessTokens       bool     `json:"dpop_bound_access_tokens"`
	TokenEndpointAuthMethod     string   `json:"token_endpoint_auth_method,omitempty"`
	TokenEndpointAuthSigningAlg string   `json:"token_endpoint_auth_signing_alg,omitempty"`
	JwksUri                     string   `json:"jwks_uri,omitempty"`
}

// LoginPage renders the OAuth login page
func (h *Handlers) LoginPage(c echo.Context) error {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Login - Survey Service</title>
    <style>
        body {
            font-family: system-ui, -apple-system, sans-serif;
            max-width: 400px;
            margin: 100px auto;
            padding: 20px;
        }
        .form-group {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: 500;
        }
        input[type="text"] {
            width: 100%;
            padding: 8px;
            font-size: 14px;
            border: 1px solid #ddd;
            border-radius: 4px;
        }
        button {
            background: #0085ff;
            color: white;
            padding: 10px 20px;
            border: none;
            border-radius: 4px;
            font-size: 14px;
            cursor: pointer;
            width: 100%;
        }
        button:hover {
            background: #0066cc;
        }
        .help-text {
            font-size: 12px;
            color: #666;
            margin-top: 5px;
        }
    </style>
</head>
<body>
    <h1>Login with AT Protocol</h1>
    <form action="/oauth/login" method="post">
        <div class="form-group">
            <label for="handle">ATProto Handle:</label>
            <input type="text" id="handle" name="handle" placeholder="alice.bsky.social" required>
            <div class="help-text">Enter your AT Protocol handle (e.g., alice.bsky.social)</div>
        </div>
        <button type="submit">Continue</button>
    </form>
</body>
</html>`

	return c.HTML(http.StatusOK, html)
}

// Login initiates the OAuth flow
func (h *Handlers) Login(c echo.Context) error {
	// Only accept POST requests
	if c.Request().Method != http.MethodPost {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "method not allowed")
	}

	// Get handle from form
	handle := c.FormValue("handle")
	if handle == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "handle is required")
	}

	// Get destination (where to redirect after auth)
	destination := c.QueryParam("destination")
	if destination == "" {
		destination = "/"
	}

	// Generate state for CSRF protection EARLY (before any network calls that might fail)
	// Note: GenerateState() panics on crypto error (design choice for unrecoverable errors)
	state := GenerateState()

	// Set state in cookie for CSRF protection
	// This prevents attackers from using their own state value with a victim's session
	// Must be set BEFORE any network calls so tests can verify cookie is set even if resolution fails
	// Use SameSiteLax (not Strict) because OAuth callbacks are cross-site navigations
	stateCookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/oauth",
		HttpOnly: true,
		Secure:   true, // HTTPS only
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes (same as OAuth request expiry)
	}
	c.SetCookie(stateCookie)

	// Resolve handle → DID
	did, err := HandleToDID(handle)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to resolve handle: %v", err))
	}

	// Resolve DID → PDS
	pds, err := DIDToPDS(did)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to resolve PDS: %v", err))
	}

	// Resolve PDS → Auth Server
	authServer, err := PDSToAuthServer(pds)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to resolve auth server: %v", err))
	}

	// Resolve Auth Server → PAR endpoint
	parEndpoint, err := AuthServerToPAREndpoint(authServer)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to resolve PAR endpoint: %v", err))
	}

	// Generate PKCE verifier
	pkceVerifier := GenerateCodeVerifier()

	// Generate DPoP key pair (JWK format)
	// Note: GenerateSecretJWK() panics on crypto error
	dpopKeyJWK := GenerateSecretJWK()

	// Build client metadata URL
	clientID := fmt.Sprintf("https://%s/oauth/client-metadata.json", h.config.Host)
	redirectURI := fmt.Sprintf("https://%s/oauth/callback", h.config.Host)

	// Execute PAR request
	parConfig := PARConfig{
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		Scope:         "atproto transition:generic",
		State:         state,
		CodeVerifier:  pkceVerifier,
		DPoPKey:       dpopKeyJWK,
		ClientKey:     h.config.SecretJWK,
		PAREndpoint:   parEndpoint,
		AuthServerURL: authServer,
	}

	requestURI, err := ExecutePAR(parConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("PAR request failed: %v", err))
	}

	// Save OAuth request state
	oauthReq := OAuthRequest{
		State:          state,
		Issuer:         authServer,
		PKCEVerifier:   pkceVerifier,
		DPoPPrivateKey: dpopKeyJWK,
		Destination:    destination,
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}

	if err := h.storage.SaveOAuthRequest(c.Request().Context(), oauthReq); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save OAuth request")
	}

	// Get authorization endpoint
	authEndpoint, err := getAuthorizationEndpoint(authServer)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get authorization endpoint")
	}

	// Redirect to authorization server
	redirectURL := fmt.Sprintf("%s?client_id=%s&request_uri=%s", authEndpoint, clientID, requestURI)
	return c.Redirect(http.StatusFound, redirectURL)
}

// Callback handles the OAuth callback
func (h *Handlers) Callback(c echo.Context) error {
	// Get callback parameters
	iss := c.QueryParam("iss")
	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if iss == "" || code == "" || state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing required parameters")
	}

	// CSRF Protection: Verify state matches the cookie value
	// This prevents attackers from using their own authorization code with a victim's session
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "CSRF detected: missing state cookie")
	}

	if stateCookie.Value != state {
		return echo.NewHTTPError(http.StatusBadRequest, "CSRF detected: state mismatch")
	}

	// Clear the state cookie immediately after validation (single-use)
	clearStateCookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/oauth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete cookie
	}
	c.SetCookie(clearStateCookie)

	// Look up OAuth request by state
	oauthReq, err := h.storage.GetOAuthRequest(c.Request().Context(), state)
	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired state")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to retrieve OAuth request")
	}

	// Verify issuer matches
	if oauthReq.Issuer != iss {
		return echo.NewHTTPError(http.StatusBadRequest, "issuer mismatch")
	}

	// Get token endpoint from the auth server
	tokenEndpoint, err := GetTokenEndpoint(iss)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to get token endpoint: %v", err))
	}

	// Build client ID and redirect URI
	clientID := fmt.Sprintf("https://%s/oauth/client-metadata.json", h.config.Host)
	redirectURI := fmt.Sprintf("https://%s/oauth/callback", h.config.Host)

	// Exchange authorization code for tokens
	tokenConfig := TokenConfig{
		Code:          code,
		CodeVerifier:  oauthReq.PKCEVerifier,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		TokenEndpoint: tokenEndpoint,
		ClientKey:     h.config.SecretJWK,
		DPoPKey:       oauthReq.DPoPPrivateKey,
		AuthServerURL: iss,
	}

	tokenResp, err := ExchangeToken(tokenConfig)
	if err != nil {
		// Clean up OAuth request on error
		if delErr := h.storage.DeleteOAuthRequest(c.Request().Context(), state); delErr != nil {
			c.Logger().Errorf("Failed to delete OAuth request: %v", delErr)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("token exchange failed: %v", err))
	}

	// Resolve PDS URL for the user's DID
	pdsURL, err := DIDToPDS(tokenResp.Sub)
	if err != nil {
		c.Logger().Errorf("Failed to resolve PDS URL for DID %s: %v", tokenResp.Sub, err)
		// Continue without PDS URL - user can still use the service
		pdsURL = ""
	}

	// Calculate token expiration time
	var tokenExpiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		tokenExpiresAt = &expiresAt
	}

	// Create session with tokens for PDS writes
	sessionID := GenerateState() // Reuse state generation for session ID
	session := OAuthSession{
		ID:             sessionID,
		DID:            tokenResp.Sub,
		AccessToken:    tokenResp.AccessToken,
		RefreshToken:   tokenResp.RefreshToken,
		DPoPKey:        oauthReq.DPoPPrivateKey,
		PDSUrl:         pdsURL,
		TokenExpiresAt: tokenExpiresAt,
		ExpiresAt:      time.Now().Add(24 * time.Hour), // Session cookie expiry
	}

	if err := h.storage.CreateSession(c.Request().Context(), session); err != nil {
		// Clean up OAuth request on error
		if delErr := h.storage.DeleteOAuthRequest(c.Request().Context(), state); delErr != nil {
			c.Logger().Errorf("Failed to delete OAuth request: %v", delErr)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create session")
	}

	// Clean up OAuth request (no longer needed)
	if err := h.storage.DeleteOAuthRequest(c.Request().Context(), state); err != nil {
		// Log but don't fail
		c.Logger().Errorf("Failed to delete OAuth request: %v", err)
	}

	// Set session cookie
	cookie := &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // HTTPS only
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	}
	c.SetCookie(cookie)

	// Redirect to destination
	destination := oauthReq.Destination
	if destination == "" {
		destination = "/"
	}

	return c.Redirect(http.StatusFound, destination)
}

// ClientMetadata returns the OAuth client metadata
func (h *Handlers) ClientMetadata(c echo.Context) error {
	metadata := ClientMetadata{
		ClientID:                    fmt.Sprintf("https://%s/oauth/client-metadata.json", h.config.Host),
		ClientName:                  "Survey Service",
		ApplicationType:             "web",
		GrantTypes:                  []string{"authorization_code", "refresh_token"},
		Scope:                       "atproto transition:generic",
		ResponseTypes:               []string{"code"},
		RedirectURIs:                []string{fmt.Sprintf("https://%s/oauth/callback", h.config.Host)},
		DPopBoundAccessTokens:       true,
		TokenEndpointAuthMethod:     "private_key_jwt",
		TokenEndpointAuthSigningAlg: "ES256",
		JwksUri:                     fmt.Sprintf("https://%s/oauth/jwks.json", h.config.Host),
	}

	return c.JSON(http.StatusOK, metadata)
}

// JWKS returns the JSON Web Key Set
func (h *Handlers) JWKS(c echo.Context) error {
	// Convert private JWK to public JWK
	publicJWK, err := PrivateJWKToPublicJWK(h.config.SecretJWK)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate public JWK")
	}

	// Parse the public JWK to ensure it's valid JSON
	var jwkMap map[string]interface{}
	if err := json.Unmarshal([]byte(publicJWK), &jwkMap); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid JWK format")
	}

	// Return JWKS format (keys array)
	jwks := map[string]interface{}{
		"keys": []interface{}{jwkMap},
	}

	return c.JSON(http.StatusOK, jwks)
}

// Logout handles user logout
func (h *Handlers) Logout(c echo.Context) error {
	// Get session cookie
	cookie, err := c.Cookie("session")
	if err == nil && cookie.Value != "" {
		// Delete session from database
		if err := h.storage.DeleteSession(c.Request().Context(), cookie.Value); err != nil {
			c.Logger().Errorf("Failed to delete session: %v", err)
		}
	}

	// Clear session cookie
	c.SetCookie(&http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete cookie
	})

	return c.Redirect(http.StatusFound, "/")
}

// Helper functions

// getAuthorizationEndpoint fetches the authorization endpoint from the auth server
func getAuthorizationEndpoint(authServer string) (string, error) {
	resp, err := http.Get(authServer + "/.well-known/oauth-authorization-server")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var metadata map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", err
	}

	endpoint, ok := metadata["authorization_endpoint"].(string)
	if !ok {
		return "", fmt.Errorf("missing authorization_endpoint")
	}

	return endpoint, nil
}
