package api

import (
	"database/sql"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// SetupRoutes configures all API routes
func SetupRoutes(e *echo.Echo, h *Handlers, hh *HealthHandlers, oh *oauth.Handlers, db *sql.DB) {
	// Health check and metrics endpoints (no middleware)
	e.GET("/health", hh.Health)
	e.GET("/health/ready", hh.Readiness)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Static files (Monaco editor assets)
	e.Static("/static", "web/dist")

	// Apply middleware to all other routes
	e.Use(RequestIDMiddleware())
	e.Use(MetricsMiddleware())
	e.Use(SecurityHeadersMiddleware())
	e.Use(otelecho.Middleware("survey-api"))

	// Create session middleware
	storage := oauth.NewStorage(db)
	sessionMiddleware := oauth.SessionMiddleware(storage)

	// Create rate limiters
	rateLimiters := NewRateLimiterConfig()

	// Create body limit config
	bodyLimits := DefaultBodyLimitConfig()

	// JSON API routes - v1
	api := e.Group("/api/v1")

	// CORS configuration for API routes
	api.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"}, // Allow all origins for MVP
		AllowMethods: []string{
			echo.GET,
			echo.POST,
			echo.PUT,
			echo.DELETE,
			echo.OPTIONS,
		},
		AllowHeaders: []string{
			echo.HeaderContentType,
			echo.HeaderAuthorization,
			echo.HeaderAccept,
		},
	}))

	// Survey management with rate limiting and body limits
	api.POST("/surveys", h.CreateSurvey, rateLimiters.SurveyCreation.Middleware(), NewBodyLimitMiddleware(bodyLimits.SurveyCreation))
	api.GET("/surveys/:slug", h.GetSurvey, rateLimiters.GeneralAPI.Middleware())

	// Response submission and results with rate limiting and body limits
	api.POST("/surveys/:slug/responses", h.SubmitResponse, rateLimiters.VoteSubmission.Middleware(), NewBodyLimitMiddleware(bodyLimits.ResponseSubmission))
	api.GET("/surveys/:slug/results", h.GetResults, rateLimiters.GeneralAPI.Middleware())

	// HTML routes (Templ handlers) - with session middleware
	web := e.Group("", sessionMiddleware)

	// Short URL routes with rate limiting
	web.GET("/s/:slug", h.ShortSlugURL, rateLimiters.GeneralAPI.Middleware())
	web.GET("/at/:did/:rkey", h.ATProtoURL, rateLimiters.GeneralAPI.Middleware())

	// Survey creation with rate limiting and body limits
	web.GET("/surveys/new", h.CreateSurveyPageHTML, rateLimiters.GeneralAPI.Middleware())
	web.POST("/surveys", h.CreateSurveyHTML, rateLimiters.SurveyCreation.Middleware(), NewBodyLimitMiddleware(bodyLimits.SurveyCreation))

	// Survey viewing and voting with rate limiting and body limits
	web.GET("/surveys/:slug", h.GetSurveyHTML, rateLimiters.GeneralAPI.Middleware())
	web.POST("/surveys/:slug/responses", h.SubmitResponseHTML, rateLimiters.VoteSubmission.Middleware(), NewBodyLimitMiddleware(bodyLimits.ResponseSubmission))

	// Results with rate limiting
	web.GET("/surveys/:slug/results", h.GetResultsHTML, rateLimiters.GeneralAPI.Middleware())
	web.GET("/surveys/:slug/results-partial", h.GetResultsPartialHTML, rateLimiters.GeneralAPI.Middleware())
	web.POST("/surveys/:slug/publish-results", h.PublishResultsHTML, rateLimiters.GeneralAPI.Middleware())

	// My Data routes (requires login) with rate limiting
	web.GET("/my-data", h.MyDataHTML, rateLimiters.GeneralAPI.Middleware())
	web.GET("/my-data/:collection", h.MyDataCollectionHTML, rateLimiters.GeneralAPI.Middleware())
	web.GET("/my-data/:collection/:rkey", h.MyDataRecordHTML, rateLimiters.GeneralAPI.Middleware())
	web.POST("/my-data/:collection/:rkey", h.UpdateRecordHTML, rateLimiters.GeneralAPI.Middleware())
	web.POST("/my-data/delete", h.DeleteRecordsHTML, rateLimiters.GeneralAPI.Middleware())

	// OAuth routes with rate limiting
	if oh != nil {
		oauthGroup := e.Group("/oauth")
		oauthGroup.GET("/login", oh.LoginPage, rateLimiters.OAuth.Middleware())
		oauthGroup.POST("/login", oh.Login, rateLimiters.OAuth.Middleware())
		oauthGroup.GET("/callback", oh.Callback, rateLimiters.OAuth.Middleware())
		oauthGroup.GET("/client-metadata.json", oh.ClientMetadata, rateLimiters.OAuth.Middleware())
		oauthGroup.GET("/jwks.json", oh.JWKS, rateLimiters.OAuth.Middleware())
		oauthGroup.POST("/logout", oh.Logout, rateLimiters.OAuth.Middleware())
	}

	// Landing page with statistics
	web.GET("/", h.LandingPage, rateLimiters.GeneralAPI.Middleware())
}
