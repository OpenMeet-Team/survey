.PHONY: test test-unit test-e2e test-all migrate migrate-up migrate-down migrate-create migrate-force templ frontend

GO := /usr/local/go/bin/go
TEMPL := $(shell which templ 2>/dev/null || echo "$(HOME)/go/bin/templ")
MIGRATE := $(shell which migrate 2>/dev/null || echo "$(HOME)/go/bin/migrate")
MIGRATIONS_PATH := internal/db/migrations

# Database URL from environment or construct from individual vars
DATABASE_URL ?= postgresql://$(DATABASE_USER):$(DATABASE_PASSWORD)@$(DATABASE_HOST):$(DATABASE_PORT)/$(DATABASE_NAME)?sslmode=$(DATABASE_SSLMODE)

# Run unit tests only (default)
test: test-unit

# Run unit tests (excluding e2e)
test-unit:
	$(GO) test -v ./...

# Run E2E tests (requires Docker)
test-e2e:
	$(GO) test -v -tags=e2e ./internal/api/e2e_test.go ./internal/api/handlers.go ./internal/api/dto.go ./internal/api/middleware.go ./internal/api/router.go -timeout=5m

# Run all tests (unit + e2e)
test-all:
	$(GO) test -v ./...
	$(GO) test -v -tags=e2e ./internal/api/e2e_test.go ./internal/api/handlers.go ./internal/api/dto.go ./internal/api/middleware.go ./internal/api/router.go -timeout=5m

# Generate templ templates
templ:
	$(TEMPL) generate

# Build frontend assets (Monaco editor)
frontend:
	cd web && npm install && npm run build

# Build the API server (includes frontend and templ)
build: frontend templ
	$(GO) build -o bin/survey-api ./cmd/api

# Build API server without frontend (faster for backend-only changes)
build-quick: templ
	$(GO) build -o bin/survey-api ./cmd/api

# Build the consumer
build-consumer:
	$(GO) build -o bin/survey-consumer ./cmd/consumer

# Run the API server locally
run:
	$(GO) run ./cmd/api

# Run the consumer locally
run-consumer:
	$(GO) run ./cmd/consumer

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf web/dist
	rm -rf web/node_modules

# ============================================================================
# Database Migrations (requires golang-migrate)
# Install: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
# ============================================================================

# Run all pending migrations
migrate: migrate-up

# Apply all up migrations
migrate-up:
	$(MIGRATE) -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up

# Rollback the last migration
migrate-down:
	$(MIGRATE) -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down 1

# Rollback all migrations
migrate-down-all:
	$(MIGRATE) -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down -all

# Show current migration version
migrate-version:
	$(MIGRATE) -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" version

# Create a new migration (usage: make migrate-create NAME=add_users_table)
migrate-create:
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_PATH) -seq $(NAME)

# Force set migration version (usage: make migrate-force VERSION=3)
migrate-force:
	$(MIGRATE) -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" force $(VERSION)

# Migrate to a specific version (usage: make migrate-goto VERSION=2)
migrate-goto:
	$(MIGRATE) -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" goto $(VERSION)
