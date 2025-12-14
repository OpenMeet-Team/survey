package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openmeet-team/survey/internal/consumer"
	"github.com/openmeet-team/survey/internal/db"
	"github.com/openmeet-team/survey/internal/telemetry"
)

func main() {
	log.Println("survey-consumer: Starting ATProto Jetstream consumer...")

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

	// Load database configuration from environment
	cfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load database config: %v", err)
	}

	// Connect to database
	database, err := db.Connect(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close(database)

	log.Println("Connected to database")

	// Create queries instance
	queries := db.NewQueries(database)

	// Start metrics server for Prometheus scraping
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "2112"
	}
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		log.Printf("Metrics server listening on :%s", metricsPort)
		if err := http.ListenAndServe(":"+metricsPort, nil); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Build Jetstream URL
	// Subscribe to survey, response, and results collections
	// Note: Jetstream requires repeated query params, not comma-separated values
	jetstreamURL := "wss://jetstream2.us-east.bsky.network/subscribe?wantedCollections=net.openmeet.survey&wantedCollections=net.openmeet.survey.response&wantedCollections=net.openmeet.survey.results"

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run consumer in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- consumer.RunWithReconnect(ctx, jetstreamURL, queries)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	case err := <-errChan:
		if err != nil {
			log.Printf("Consumer error: %v", err)
		}
	}

	log.Println("survey-consumer: Shutdown complete")
}
