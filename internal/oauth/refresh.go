package oauth

import (
	"context"
	"fmt"
	"time"
)

// EnsureValidToken checks if the access token is valid and refreshes it if necessary.
// Returns nil if token is valid or was successfully refreshed.
// Returns error if refresh is needed but fails (caller should invalidate session).
//
// Token is considered valid if:
// - TokenExpiresAt is nil (no expiration set)
// - TokenExpiresAt is more than 5 minutes in the future
//
// Token refresh is attempted if:
// - TokenExpiresAt is in the past or within 5 minutes
func EnsureValidToken(ctx context.Context, session *OAuthSession, storage *Storage, config Config) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}

	// If no expiration time set, treat as valid
	if session.TokenExpiresAt == nil {
		return nil
	}

	// Check if token is still valid (expires more than 5 minutes from now)
	threshold := time.Now().Add(5 * time.Minute)
	if session.TokenExpiresAt.After(threshold) {
		// Token is still valid, no refresh needed
		return nil
	}

	// Token is expired or expiring soon, need to refresh
	// Verify we have the required fields for refresh
	if session.Issuer == "" {
		return fmt.Errorf("cannot refresh token: session missing issuer")
	}

	if session.RefreshToken == "" {
		return fmt.Errorf("cannot refresh token: session missing refresh token")
	}

	if session.DPoPKey == "" {
		return fmt.Errorf("cannot refresh token: session missing DPoP key")
	}

	if storage == nil {
		return fmt.Errorf("cannot refresh token: storage is nil")
	}

	// Build client ID from config
	clientID := fmt.Sprintf("https://%s/oauth/client-metadata.json", config.Host)

	// Attempt to refresh the token
	newAccessToken, newRefreshToken, expiresIn, err := RefreshAccessToken(
		session,
		session.Issuer,
		clientID,
		config.SecretJWK,
	)

	if err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}

	// Calculate new expiration time
	var newExpiresAt *time.Time
	if expiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
		newExpiresAt = &expiresAt
	}

	// Update session in database
	err = storage.UpdateSessionTokens(ctx, session.ID, newAccessToken, newRefreshToken, newExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to update session tokens: %w", err)
	}

	// Update the session object in memory
	session.AccessToken = newAccessToken
	session.RefreshToken = newRefreshToken
	session.TokenExpiresAt = newExpiresAt

	return nil
}
