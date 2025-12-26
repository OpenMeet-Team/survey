# Survey Service

A standalone survey/polling service with ATProto integration.

## Features

- **Multi-question surveys**: Single choice, multiple choice, and free text questions
- **YAML/JSON definitions**: Define surveys in YAML or JSON
- **AI Survey Generation**: Create surveys from natural language prompts using OpenAI (optional)
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
# Install golang-migrate (one time)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Create database
createdb survey

# Run all migrations
make migrate
```

The project uses [golang-migrate](https://github.com/golang-migrate/migrate) for database migrations. See the Makefile for additional targets: `migrate-down`, `migrate-version`, `migrate-create`.

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

# AI Survey Generation (optional - enables OpenAI-powered survey creation)
export OPENAI_API_KEY=sk-...                        # Your OpenAI API key
```

## AI Survey Generation

The survey service includes optional AI-powered survey generation that converts natural language descriptions into structured survey JSON using OpenAI's GPT-4o-mini.

### Configuration

Enable AI generation by setting the OpenAI API key:

```bash
export OPENAI_API_KEY=sk-...
```

If the API key is not set, the `/api/v1/surveys/generate` endpoint will return `503 Service Unavailable`.

### API Endpoint

**POST** `/api/v1/surveys/generate`

**Request:**
```json
{
  "description": "Create a feedback survey for my photography meetup - ask about venue rating, useful topics, and suggestions",
  "existing_json": "",  // Optional: for iterative refinement
  "consent": true       // Required: user must consent to OpenAI processing
}
```

**Response (Success):**
```json
{
  "survey_json": {
    "questions": [
      {
        "id": "q1",
        "text": "How would you rate the venue?",
        "type": "single",
        "options": [
          {"id": "opt1", "text": "1 - Poor"},
          {"id": "opt2", "text": "2"},
          {"id": "opt3", "text": "3"},
          {"id": "opt4", "text": "4"},
          {"id": "opt5", "text": "5 - Excellent"}
        ]
      }
    ],
    "anonymous": false
  },
  "tokens_used": 350,
  "cost": 0.00055
}
```

**Error Responses:**
- `400 Bad Request` - Missing consent, empty description, input too long, or blocked pattern
- `429 Too Many Requests` - Rate limit exceeded
- `503 Service Unavailable` - AI generation not configured or budget exceeded

### Rate Limits

The service implements per-replica in-memory rate limiting (configurable via environment variables):

| User Type | Default | Env Vars |
|-----------|---------|----------|
| Anonymous (by IP) | 5 per hour | `AI_RATE_LIMIT_ANON_LIMIT`, `AI_RATE_LIMIT_ANON_WINDOW_HOURS` |
| Authenticated (by DID) | 20 per day | `AI_RATE_LIMIT_AUTH_LIMIT`, `AI_RATE_LIMIT_AUTH_WINDOW_HOURS` |

**Multi-replica behavior**: With N replicas, effective limits are N× the configured values. This is acceptable for MVP - cost limits are the primary protection.

### Cost Controls

Each replica enforces a daily budget:
- **Daily budget**: $10 per replica
- **Estimated cost per generation**: ~$0.0005-0.0006

This allows ~18,000 generations per replica per day before the budget is exceeded.

### Security Features

1. **Input Validation**
   - Maximum 2,000 characters
   - Character whitelist (alphanumeric + basic punctuation)
   - Blocked patterns detection (e.g., "ignore previous instructions")

2. **Output Sanitization**
   - JSON parsing and validation
   - XSS prevention via HTML sanitization
   - Schema validation against survey definition constraints

3. **Privacy**
   - Explicit consent required before sending data to OpenAI
   - No PII included in prompts (only survey description)
   - Prompts and responses not logged (only metrics)

### Web UI

The `/surveys/new` page includes an AI generation section where users can:
1. Enter a natural language description (up to 2,000 characters)
2. Accept consent checkbox for OpenAI processing
3. Click "Generate Survey" to create the survey JSON
4. Review and edit the generated survey in the Monaco editor
5. Preview and submit

The AI section works alongside the Monaco JSON/YAML editor - users can skip AI and write surveys manually if preferred.

### Monitoring

The following Prometheus metrics track AI generation:

```
survey_ai_generations_total{status="success|error|rate_limited|budget_exceeded"}
survey_ai_generation_duration_seconds
survey_ai_tokens_total{type="input|output"}
survey_ai_daily_cost_usd
survey_ai_rate_limit_hits_total{user_type="anonymous|authenticated"}
```

### Testing

Use the `FakeLLM` provider for testing without making real API calls:

```go
import "github.com/tmc/langchaingo/llms/fake"

fakeLLM := fake.NewFakeLLM(func(ctx context.Context, prompt string) (string, error) {
    return `{"questions":[{"id":"q1","text":"Test?","type":"single","options":[{"id":"yes","text":"Yes"}]}]}`, nil
})
generator := generator.NewSurveyGenerator(fakeLLM, "fake-model")
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

#### HTML Routes (Web UI)

| Endpoint | Description |
|----------|-------------|
| `GET /` | Landing page with stats |
| `GET /surveys/new` | Create survey form |
| `GET /surveys/:slug` | Survey form (vote) |
| `GET /surveys/:slug/results` | Results page |
| `GET /s/:slug` | Short URL redirect |
| `GET /at/:did/:rkey` | ATProto URL redirect |
| `GET /my-data` | PDS browser overview |
| `GET /my-data/:collection` | List collection records |
| `GET /my-data/:collection/:rkey` | Edit single record |
| `GET /health` | Liveness probe |
| `GET /health/ready` | Readiness probe (checks DB) |
| `GET /metrics` | Prometheus metrics |

#### JSON API

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/surveys` | Create survey |
| `POST /api/v1/surveys/generate` | Generate survey using AI (requires consent) |
| `GET /api/v1/surveys/:slug` | Get survey by slug |
| `POST /api/v1/surveys/:slug/responses` | Submit response |
| `GET /api/v1/surveys/:slug/results` | Get results |

**Note:** Public list endpoints (`GET /surveys` and `GET /api/v1/surveys`) were intentionally removed. Surveys are only accessible via direct link to prevent discovery of all surveys.

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
- Survey creation and retrieval by slug (YAML/JSON parsing)
- List endpoint removal (returns 404)
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
│   ├── consumer/         # Jetstream consumer
│   ├── db/               # Database access and migrations
│   ├── models/           # Domain models
│   ├── oauth/            # ATProto OAuth + PDS integration
│   ├── telemetry/        # Metrics setup
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
