package api

import (
	"database/sql"

	"github.com/labstack/echo/v4"
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

	// Apply middleware to all other routes
	e.Use(RequestIDMiddleware())
	e.Use(MetricsMiddleware())
	e.Use(otelecho.Middleware("survey-api"))

	// Create session middleware
	storage := oauth.NewStorage(db)
	sessionMiddleware := oauth.SessionMiddleware(storage)

	// JSON API routes - v1
	api := e.Group("/api/v1")

	// Survey management
	api.POST("/surveys", h.CreateSurvey)
	api.GET("/surveys", h.ListSurveys)
	api.GET("/surveys/:slug", h.GetSurvey)

	// Response submission and results
	api.POST("/surveys/:slug/responses", h.SubmitResponse)
	api.GET("/surveys/:slug/results", h.GetResults)

	// HTML routes (Templ handlers) - with session middleware
	web := e.Group("", sessionMiddleware)

	// Survey list and creation
	web.GET("/surveys", h.ListSurveysHTML)
	web.GET("/surveys/new", h.CreateSurveyPageHTML)
	web.POST("/surveys", h.CreateSurveyHTML)

	// Survey viewing and voting
	web.GET("/surveys/:slug", h.GetSurveyHTML)
	web.POST("/surveys/:slug/responses", h.SubmitResponseHTML)

	// Results
	web.GET("/surveys/:slug/results", h.GetResultsHTML)
	web.GET("/surveys/:slug/results-partial", h.GetResultsPartialHTML)
	web.POST("/surveys/:slug/publish-results", h.PublishResultsHTML)

	// OAuth routes
	if oh != nil {
		oauthGroup := e.Group("/oauth")
		oauthGroup.GET("/login", oh.LoginPage)
		oauthGroup.POST("/login", oh.Login)
		oauthGroup.GET("/callback", oh.Callback)
		oauthGroup.GET("/client-metadata.json", oh.ClientMetadata)
		oauthGroup.GET("/jwks.json", oh.JWKS)
		oauthGroup.POST("/logout", oh.Logout)
	}

	// Redirect root to surveys list
	e.GET("/", func(c echo.Context) error {
		return c.Redirect(302, "/surveys")
	})
}
