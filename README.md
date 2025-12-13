# Survey Service

A standalone survey/polling service with ATProto integration.

## Features

- **Multi-question surveys**: Single choice, multiple choice, and free text questions
- **YAML/JSON definitions**: Define surveys in YAML or JSON
- **Web UI**: Clean, responsive HTML interface with HTMX
- **JSON API**: RESTful API for programmatic access
- **Live results**: Real-time result aggregation with polling
- **Privacy-preserving**: Per-survey salted guest identity (can't track across surveys)
- **ATProto login**: OAuth authentication via any ATProto PDS
- **PDS writes**: Surveys and responses stored in user's Personal Data Server
- **Federated indexing**: Jetstream consumer indexes surveys from any PDS on the network

## Architecture

- **survey-api**: Web server with HTML (Templ) and JSON API endpoints
- **survey-consumer**: Jetstream consumer that indexes ATProto surveys, responses, and results

## Tech Stack

- **Language**: Go 1.24+
- **HTTP Framework**: Echo v4
- **Templates**: Templ + HTMX
- **Database**: PostgreSQL (via pgx/v5)
- **Observability**: OpenTelemetry (otelsql)
- **Metrics**: Prometheus

## Quick Start

### Prerequisites

- Go 1.24+
- PostgreSQL 14+

### Database Setup

```bash
# Create database
createdb survey

# Run migrations
psql survey < internal/db/migrations/001_initial.up.sql
```

### Configuration

```bash
# Database
export DATABASE_HOST=localhost
export DATABASE_PORT=5432
export DATABASE_USER=postgres
export DATABASE_PASSWORD=yourpassword
export DATABASE_NAME=survey

# API Server
export PORT=8080

# OpenTelemetry Tracing (optional)
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318  # Jaeger OTLP HTTP endpoint
export OTEL_SERVICE_NAME=survey-api                 # Service name in traces

# ATProto OAuth (optional - enables "Login with ATProto")
export OAUTH_SECRET_JWK_B64=<base64-encoded-JWK>   # Generate with: go run ./cmd/keygen
export SERVER_HOST=https://survey.example.com       # Public URL of your service
```

**Tracing**: The service exports traces to Jaeger via OTLP HTTP. HTTP requests (via otelecho) and database queries (via otelsql) are automatically traced. If the OTLP endpoint is unavailable, the service logs a warning and continues running. To run Jaeger locally:

```bash
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
# UI: http://localhost:16686
```

### Running the API Server

```bash
go run ./cmd/api
# Server starts on http://localhost:8080
```

### Running the Jetstream Consumer

The consumer indexes ATProto records from the ATProto network:

```bash
go run ./cmd/consumer
# Connects to wss://jetstream2.us-east.bsky.network
```

**Collections indexed:**
- `net.openmeet.survey` - Survey definitions from any PDS
- `net.openmeet.survey.response` - User votes
- `net.openmeet.survey.results` - Finalized results (anonymized aggregates)

**Features:**
- Cursor-based resumption (survives restarts)
- Exponential backoff reconnection (1s → 60s)
- Authorization checks (only owners can update/delete)
- Atomic message + cursor updates (no duplicates)

### Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Survey list (HTML) |
| `GET /surveys/new` | Create survey form (HTML) |
| `GET /surveys/:slug` | Survey form (HTML) |
| `GET /surveys/:slug/results` | Results page (HTML) |
| `GET /health` | Liveness probe |
| `GET /health/ready` | Readiness probe (checks DB) |
| `GET /metrics` | Prometheus metrics |

#### JSON API

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/surveys` | Create survey |
| `GET /api/v1/surveys` | List surveys |
| `GET /api/v1/surveys/:slug` | Get survey |
| `POST /api/v1/surveys/:slug/responses` | Submit response |
| `GET /api/v1/surveys/:slug/results` | Get results |

## Survey Definition Format

```yaml
name: "Weekly Sync Preference"
description: "Help us pick a meeting time"
anonymous: false
startsAt: "2025-12-11T00:00:00Z"
endsAt: "2025-12-31T23:59:00Z"

questions:
  - id: q1
    text: "Preferred day?"
    type: single
    required: true
    options:
      - id: mon
        text: "Monday"
      - id: tue
        text: "Tuesday"

  - id: q2
    text: "What topics should we cover?"
    type: multi
    required: false
    options:
      - id: planning
        text: "Sprint planning"
      - id: demos
        text: "Demos"

  - id: q3
    text: "Any other feedback?"
    type: text
    required: false
```

## Testing

### Unit Tests

Run unit tests using mocks:
```bash
make test-unit
# or
go test -v ./...
```

### End-to-End Tests

E2E tests use [testcontainers-go](https://golang.testcontainers.org/) to spin up a real PostgreSQL database and test the full HTTP flow.

**Requirements:**
- Docker must be running
- Network access to pull `postgres:16-alpine` image

**Run E2E tests:**
```bash
make test-e2e
```

**What's tested:**
- Survey creation and listing (YAML/JSON parsing)
- Response submission with validation
- Duplicate vote prevention (voter session hashing)
- Invalid answer rejection
- Slug validation and auto-generation
- Health check endpoints
- Results aggregation

E2E tests are tagged with `//go:build e2e` so they don't run with regular unit tests.

## Project Structure

```
survey/
├── cmd/
│   ├── api/              # survey-api entrypoint
│   └── consumer/         # survey-consumer entrypoint
├── internal/
│   ├── api/              # HTTP handlers, router, middleware
│   │   ├── handlers_test.go     # Unit tests (mocks)
│   │   └── e2e_test.go          # E2E tests (real DB)
│   ├── consumer/         # Jetstream consumer
│   │   ├── jetstream.go         # WebSocket client
│   │   ├── processor.go         # Message routing & CRUD
│   │   ├── cursor.go            # Cursor persistence
│   │   └── atproto.go           # Record parsing
│   ├── db/               # Database access and migrations
│   ├── telemetry/        # Metrics setup
│   ├── models/           # Domain models
│   └── templates/        # Templ templates
├── lexicon/              # ATProto lexicon schemas
├── k8s/                  # Kubernetes manifests
├── Makefile              # Build and test targets
└── Dockerfile
```

## Deployment

### Docker

```bash
docker build -t survey .
docker run -p 8080:8080 -e DATABASE_PASSWORD=secret survey
```

### Kubernetes

```bash
kubectl apply -k k8s/base/
```

The deployment includes:
- **survey-api**: 2 replicas (stateless, scalable)
- **survey-consumer**: 1 replica (single Jetstream cursor)

## ATProto Lexicons

- `net.openmeet.survey` - Survey/poll definition record
- `net.openmeet.survey.response` - User response (vote) record
- `net.openmeet.survey.results` - Finalized, anonymized results (published by survey author after voting ends)

See `lexicon/` directory for full schemas.

### Privacy Design

After a survey's `endsAt` time passes:
1. Survey author aggregates and publishes `net.openmeet.survey.results` to their PDS
2. Voters can then delete their individual `response` records from their own PDS
3. Anonymized vote counts persist on the author's PDS

## License

Apache License 2.0 - See LICENSE file.
