package oauth

import (
	"context"
	"database/sql"
	"time"

	"github.com/labstack/echo/v4"
)

// User represents an authenticated user
type User struct {
	DID string
}

// SessionStore defines the session operations needed by the middleware.
type SessionStore interface {
	GetSessionByID(ctx context.Context, id string) (*OAuthSession, error)
	DeleteSession(ctx context.Context, id string) error
}

// SessionMiddleware creates middleware that reads the session cookie
// and adds the user to the context if the session is valid
func SessionMiddleware(storage SessionStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try to get session cookie
			cookie, err := c.Cookie("session")
			if err != nil {
				// No session cookie - continue without user
				return next(c)
			}

			// Look up session in database
			session, err := storage.GetSessionByID(c.Request().Context(), cookie.Value)
			if err != nil {
				if err == sql.ErrNoRows {
					// Invalid session - continue without user
					return next(c)
				}
				// Database error - log but continue
				c.Logger().Errorf("Failed to get session: %v", err)
				return next(c)
			}

			// Check if session is expired
			if session.ExpiresAt.Before(time.Now()) {
				// Clean up expired session from database
				if err := storage.DeleteSession(c.Request().Context(), cookie.Value); err != nil {
					c.Logger().Errorf("Failed to delete expired session: %v", err)
				}
				return next(c)
			}

			// Valid session - add user to context
			user := &User{
				DID: session.DID,
			}
			c.Set("user", user)

			return next(c)
		}
	}
}

// GetUser retrieves the authenticated user from the Echo context
// Returns nil if no user is authenticated
func GetUser(c echo.Context) *User {
	val := c.Get("user")
	if val == nil {
		return nil
	}

	user, ok := val.(*User)
	if !ok {
		return nil
	}

	return user
}

// GetSession retrieves the full OAuth session from the Echo context
// Returns nil if no session exists
// This requires the session ID to be stored in context by SessionMiddleware
func GetSession(c echo.Context, storage *Storage) (*OAuthSession, error) {
	// Get session cookie
	cookie, err := c.Cookie("session")
	if err != nil {
		return nil, nil // No session cookie
	}

	// Look up full session in database
	session, err := storage.GetSessionByID(c.Request().Context(), cookie.Value)
	if err != nil {
		return nil, err
	}

	return session, nil
}
