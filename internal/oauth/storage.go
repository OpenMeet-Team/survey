package oauth

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// OAuthRequest represents a pending OAuth request
type OAuthRequest struct {
	State          string
	Issuer         string
	PKCEVerifier   string
	DPoPPrivateKey string
	Destination    string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// OAuthSession represents an authenticated user session
type OAuthSession struct {
	ID             string
	DID            string
	AccessToken    string
	RefreshToken   string
	DPoPKey        string     // DPoP private key (JWK format)
	PDSUrl         string     // User's PDS URL for direct writes
	TokenExpiresAt *time.Time // When the access token expires
	Issuer         string     // Auth server URL (needed for token refresh)
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// Storage provides database operations for OAuth
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new Storage instance
func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

// SaveOAuthRequest stores an OAuth request state
func (s *Storage) SaveOAuthRequest(ctx context.Context, req OAuthRequest) error {
	query := `
		INSERT INTO oauth_requests (state, issuer, pkce_verifier, dpop_private_key, destination, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := s.db.ExecContext(
		ctx,
		query,
		req.State,
		req.Issuer,
		req.PKCEVerifier,
		req.DPoPPrivateKey,
		req.Destination,
		req.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save OAuth request: %w", err)
	}

	return nil
}

// GetOAuthRequest retrieves an OAuth request by state
func (s *Storage) GetOAuthRequest(ctx context.Context, state string) (*OAuthRequest, error) {
	query := `
		SELECT state, issuer, pkce_verifier, dpop_private_key, destination, created_at, expires_at
		FROM oauth_requests
		WHERE state = $1
	`

	req := &OAuthRequest{}
	err := s.db.QueryRowContext(ctx, query, state).Scan(
		&req.State,
		&req.Issuer,
		&req.PKCEVerifier,
		&req.DPoPPrivateKey,
		&req.Destination,
		&req.CreatedAt,
		&req.ExpiresAt,
	)

	if err != nil {
		return nil, err
	}

	return req, nil
}

// DeleteOAuthRequest removes an OAuth request by state
func (s *Storage) DeleteOAuthRequest(ctx context.Context, state string) error {
	query := `DELETE FROM oauth_requests WHERE state = $1`

	_, err := s.db.ExecContext(ctx, query, state)
	if err != nil {
		return fmt.Errorf("failed to delete OAuth request: %w", err)
	}

	return nil
}

// CreateSession creates a new OAuth session
func (s *Storage) CreateSession(ctx context.Context, session OAuthSession) error {
	query := `
		INSERT INTO oauth_sessions (id, did, access_token, refresh_token, dpop_key, pds_url, token_expires_at, issuer, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := s.db.ExecContext(
		ctx,
		query,
		session.ID,
		session.DID,
		session.AccessToken,
		session.RefreshToken,
		session.DPoPKey,
		session.PDSUrl,
		session.TokenExpiresAt,
		session.Issuer,
		session.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetSessionByID retrieves a session by its ID
func (s *Storage) GetSessionByID(ctx context.Context, id string) (*OAuthSession, error) {
	query := `
		SELECT id, did, access_token, refresh_token, dpop_key, pds_url, token_expires_at, issuer, created_at, expires_at
		FROM oauth_sessions
		WHERE id = $1
	`

	session := &OAuthSession{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&session.ID,
		&session.DID,
		&session.AccessToken,
		&session.RefreshToken,
		&session.DPoPKey,
		&session.PDSUrl,
		&session.TokenExpiresAt,
		&session.Issuer,
		&session.CreatedAt,
		&session.ExpiresAt,
	)

	if err != nil {
		return nil, err
	}

	return session, nil
}

// UpdateSessionTokens updates the access token, refresh token, and expiration for a session
func (s *Storage) UpdateSessionTokens(ctx context.Context, id, accessToken, refreshToken string, tokenExpiresAt *time.Time) error {
	query := `
		UPDATE oauth_sessions
		SET access_token = $1, refresh_token = $2, token_expires_at = $3
		WHERE id = $4
	`

	result, err := s.db.ExecContext(ctx, query, accessToken, refreshToken, tokenExpiresAt, id)
	if err != nil {
		return fmt.Errorf("failed to update session tokens: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// DeleteSession removes a session by ID
func (s *Storage) DeleteSession(ctx context.Context, id string) error {
	query := `DELETE FROM oauth_sessions WHERE id = $1`

	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// CleanupExpiredRequests removes expired OAuth requests
func (s *Storage) CleanupExpiredRequests(ctx context.Context) (int64, error) {
	query := `DELETE FROM oauth_requests WHERE expires_at < NOW()`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired requests: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return count, nil
}

// CleanupExpiredSessions removes expired sessions
func (s *Storage) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	query := `DELETE FROM oauth_sessions WHERE expires_at < NOW()`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return count, nil
}

// StartCleanupWorker starts a background goroutine that periodically cleans up
// expired OAuth requests and sessions. It runs until the context is cancelled.
func StartCleanupWorker(ctx context.Context, storage *Storage, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("OAuth cleanup worker started (interval: %v)", interval)

	// Run cleanup immediately on start
	runCleanup(ctx, storage)

	for {
		select {
		case <-ctx.Done():
			log.Println("OAuth cleanup worker stopped")
			return
		case <-ticker.C:
			runCleanup(ctx, storage)
		}
	}
}

// runCleanup executes both cleanup operations and logs results
func runCleanup(ctx context.Context, storage *Storage) {
	// Cleanup expired requests
	requestCount, err := storage.CleanupExpiredRequests(ctx)
	if err != nil {
		log.Printf("Error cleaning up expired OAuth requests: %v", err)
	} else if requestCount > 0 {
		log.Printf("Cleaned up %d expired OAuth requests", requestCount)
	}

	// Cleanup expired sessions
	sessionCount, err := storage.CleanupExpiredSessions(ctx)
	if err != nil {
		log.Printf("Error cleaning up expired OAuth sessions: %v", err)
	} else if sessionCount > 0 {
		log.Printf("Cleaned up %d expired OAuth sessions", sessionCount)
	}
}
