//go:build e2e

package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionMiddleware(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewStorage(db)

	t.Run("no session cookie - sets nil user in context", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Create test handler that checks context
		var capturedUser *User
		handler := SessionMiddleware(storage)(func(c echo.Context) error {
			capturedUser = GetUser(c)
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		require.NoError(t, err)
		assert.Nil(t, capturedUser)
	})

	t.Run("invalid session cookie - sets nil user in context", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "session",
			Value: "invalid-session-id",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var capturedUser *User
		handler := SessionMiddleware(storage)(func(c echo.Context) error {
			capturedUser = GetUser(c)
			return c.String(http.StatusOK, "ok")
		})

		err := handler(c)
		require.NoError(t, err)
		assert.Nil(t, capturedUser)
	})

	t.Run("valid session cookie - sets user in context", func(t *testing.T) {
		// Create a session in the database first
		e := echo.New()
		setupReq := httptest.NewRequest(http.MethodGet, "/", nil)
		setupRec := httptest.NewRecorder()
		setupCtx := e.NewContext(setupReq, setupRec)

		session := OAuthSession{
			ID:        "test-session-123",
			DID:       "did:plc:test123",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		err := storage.CreateSession(setupCtx.Request().Context(), session)
		require.NoError(t, err)

		// Make request with session cookie
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "session",
			Value: session.ID,
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var capturedUser *User
		handler := SessionMiddleware(storage)(func(c echo.Context) error {
			capturedUser = GetUser(c)
			return c.String(http.StatusOK, "ok")
		})

		err = handler(c)
		require.NoError(t, err)
		require.NotNil(t, capturedUser)
		assert.Equal(t, "did:plc:test123", capturedUser.DID)
	})

	t.Run("expired session - sets nil user in context and deletes session", func(t *testing.T) {
		// Create an expired session
		e := echo.New()
		setupReq := httptest.NewRequest(http.MethodGet, "/", nil)
		setupRec := httptest.NewRecorder()
		setupCtx := e.NewContext(setupReq, setupRec)

		session := OAuthSession{
			ID:        "expired-session-cleanup",
			DID:       "did:plc:expired",
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		err := storage.CreateSession(setupCtx.Request().Context(), session)
		require.NoError(t, err)

		// Verify session exists before middleware call
		_, err = storage.GetSessionByID(setupCtx.Request().Context(), session.ID)
		require.NoError(t, err, "Session should exist before middleware call")

		// Make request with expired session cookie
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "session",
			Value: session.ID,
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var capturedUser *User
		handler := SessionMiddleware(storage)(func(c echo.Context) error {
			capturedUser = GetUser(c)
			return c.String(http.StatusOK, "ok")
		})

		err = handler(c)
		require.NoError(t, err)
		assert.Nil(t, capturedUser)

		// Verify session was deleted from database
		_, err = storage.GetSessionByID(setupCtx.Request().Context(), session.ID)
		assert.Error(t, err, "Expired session should be deleted from database")
	})
}

func TestGetUser(t *testing.T) {
	t.Run("returns nil when no user in context", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		user := GetUser(c)
		assert.Nil(t, user)
	})

	t.Run("returns user when set in context", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		expectedUser := &User{DID: "did:plc:test"}
		c.Set("user", expectedUser)

		user := GetUser(c)
		require.NotNil(t, user)
		assert.Equal(t, "did:plc:test", user.DID)
	})
}
