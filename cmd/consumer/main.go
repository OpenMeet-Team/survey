package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/openmeet-team/survey/internal/consumer"
	"github.com/openmeet-team/survey/internal/db"
)

func main() {
	log.Println("survey-consumer: Starting ATProto Jetstream consumer...")

	// Load database configuration from environment
	cfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load database config: %v", err)
	}

	// Connect to database
	ctx := context.Background()
	database, err := db.Connect(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close(database)

	log.Println("Connected to database")

	// Create queries instance
	queries := db.NewQueries(database)

	// Build Jetstream URL
	// Subscribe to survey, response, and results collections
	jetstreamURL := "wss://jetstream2.us-east.bsky.network/subscribe?wantedCollections=net.openmeet.survey,net.openmeet.survey.response,net.openmeet.survey.results"

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
