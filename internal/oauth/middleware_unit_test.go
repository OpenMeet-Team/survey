package oauth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSessionStore struct {
	sessions    map[string]*OAuthSession
	deleteErr   error
	deleteCalls []string
}

func (s *stubSessionStore) GetSessionByID(ctx context.Context, id string) (*OAuthSession, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return session, nil
}

func (s *stubSessionStore) DeleteSession(ctx context.Context, id string) error {
	s.deleteCalls = append(s.deleteCalls, id)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.sessions, id)
	return nil
}

func TestSessionMiddlewareExpiredSessionDeletes(t *testing.T) {
	store := &stubSessionStore{
		sessions: map[string]*OAuthSession{
			"expired-session": {
				ID:        "expired-session",
				DID:       "did:plc:expired",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
			},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "expired-session"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedUser *User
	nextCalled := false
	handler := SessionMiddleware(store)(func(c echo.Context) error {
		nextCalled = true
		capturedUser = GetUser(c)
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.True(t, nextCalled)
	assert.Nil(t, capturedUser)
	assert.Equal(t, []string{"expired-session"}, store.deleteCalls)
	_, exists := store.sessions["expired-session"]
	assert.False(t, exists)
}

func TestSessionMiddlewareDeleteErrorDoesNotBlock(t *testing.T) {
	store := &stubSessionStore{
		sessions: map[string]*OAuthSession{
			"expired-session": {
				ID:        "expired-session",
				DID:       "did:plc:expired",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
			},
		},
		deleteErr: errors.New("delete failed"),
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "expired-session"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedUser *User
	nextCalled := false
	handler := SessionMiddleware(store)(func(c echo.Context) error {
		nextCalled = true
		capturedUser = GetUser(c)
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.True(t, nextCalled)
	assert.Nil(t, capturedUser)
	assert.Equal(t, []string{"expired-session"}, store.deleteCalls)
	_, exists := store.sessions["expired-session"]
	assert.True(t, exists)
}

func TestSessionMiddlewareValidSessionDoesNotDelete(t *testing.T) {
	store := &stubSessionStore{
		sessions: map[string]*OAuthSession{
			"valid-session": {
				ID:        "valid-session",
				DID:       "did:plc:valid",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "valid-session"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedUser *User
	handler := SessionMiddleware(store)(func(c echo.Context) error {
		capturedUser = GetUser(c)
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	require.NotNil(t, capturedUser)
	assert.Equal(t, "did:plc:valid", capturedUser.DID)
	assert.Empty(t, store.deleteCalls)
}
