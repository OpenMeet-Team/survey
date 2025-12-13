package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/openmeet-team/survey/internal/api"
	"github.com/openmeet-team/survey/internal/db"
	"github.com/openmeet-team/survey/internal/oauth"
	"github.com/openmeet-team/survey/internal/telemetry"
)

func main() {
	// Register Prometheus metrics
	telemetry.RegisterMetrics()

	// Initialize OpenTelemetry tracing
	ctx := context.Background()
	shutdownTracing, err := telemetry.InitTracing(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize tracing: %v", err)
	}
	defer func() {
		// Shutdown tracing on exit
		if err := shutdownTracing(context.Background()); err != nil {
			log.Printf("Error shutting down tracing: %v", err)
		}
	}()

	// Get database configuration from environment
	dbConfig, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load database config: %v", err)
	}

	// Connect to database
	database, err := db.Connect(ctx, dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close(database)

	log.Println("Connected to database successfully")

	// Create database queries instance
	queries := db.NewQueries(database)

	// Create Echo instance
	e := echo.New()

	// Basic middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Create OAuth storage for session management
	oauthStorage := oauth.NewStorage(database)

	// Create handlers with OAuth storage for PDS writes
	handlers := api.NewHandlersWithOAuth(queries, oauthStorage)
	healthHandlers := api.NewHealthHandlers(database)

	// Create OAuth handlers (optional - requires OAUTH_SECRET_JWK_B64 and SERVER_HOST env vars)
	var oauthHandlers *oauth.Handlers
	secretJWKB64 := os.Getenv("OAUTH_SECRET_JWK_B64")
	host := os.Getenv("SERVER_HOST")
	if secretJWKB64 != "" && host != "" {
		// Decode base64 JWK
		secretJWKBytes, err := base64.StdEncoding.DecodeString(secretJWKB64)
		if err != nil {
			log.Fatalf("Failed to decode OAUTH_SECRET_JWK_B64: %v", err)
		}
		oauthConfig := oauth.Config{
			Host:      host,
			SecretJWK: string(secretJWKBytes),
		}
		oauthHandlers = oauth.NewHandlers(database, oauthConfig)
		log.Println("OAuth handlers initialized")
	} else {
		log.Println("OAuth disabled (OAUTH_SECRET_JWK_B64 and SERVER_HOST not configured)")
	}

	// Setup routes (includes metrics and request ID middleware)
	api.SetupRoutes(e, handlers, healthHandlers, oauthHandlers, database)

	// Start server with graceful shutdown
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server in a goroutine
	go func() {
		addr := fmt.Sprintf(":%s", port)
		log.Printf("Starting server on %s", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatalf("shutting down the server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}

	log.Println("Server shutdown complete")
}
