# Survey Service

ATProto-native survey application that stores surveys, responses, and results on users' Personal Data Servers (PDS).

## Quick Reference

```bash
# Build
make build              # Runs templ generate + go build

# Test
go test ./...           # All tests
go test -tags=e2e ./... # All e2e tests
go test ./internal/api/... ./internal/oauth/...  # Specific packages

# Run locally
./bin/survey-api        # Requires .env with DB connection
```

## Project Structure

```
survey/
├── cmd/api/            # Main entrypoint
├── internal/
│   ├── api/            # HTTP handlers, routes, middleware
│   │   ├── handlers.go       # All HTTP handlers
│   │   ├── handlers_test.go  # Handler tests with MockQueries
│   │   ├── router.go         # Route setup
│   │   └── middleware.go     # Auth, logging middleware
│   ├── oauth/          # ATProto OAuth + PDS integration
│   │   ├── pds.go            # PDS CRUD: CreateRecord, ListRecords, DeleteRecord, UpdateRecord
│   │   ├── storage.go        # Session storage (OAuthSession)
│   │   ├── resolve.go        # Handle→DID→PDS resolution
│   │   ├── jwt.go            # DPoP proof generation
│   │   └── middleware.go     # Session middleware, GetUser, GetSession
│   ├── db/             # Database queries and migrations
│   │   ├── queries.go        # QueriesInterface implementation
│   │   └── migrations/       # SQL migrations
│   ├── models/         # Domain models
│   │   ├── survey.go         # Survey, SurveyDefinition, Question, Option
│   │   └── response.go       # Response, Answer, Stats
│   ├── templates/      # Templ templates (HTML)
│   │   ├── layout.templ      # Base layout with nav
│   │   ├── landing.templ     # Landing page with stats
│   │   ├── my_data.templ     # PDS record browser
│   │   ├── survey_*.templ    # Survey-related pages
│   │   └── create_survey.templ
│   └── consumer/       # Firehose event processor
└── Makefile
```

## ATProto Integration

### Lexicons (Collections)

| Collection | Purpose |
|------------|---------|
| `net.openmeet.survey` | Survey definitions created by users |
| `net.openmeet.survey.response` | Responses submitted by voters |
| `net.openmeet.survey.results` | Published aggregated results |

### PDS Operations

**Creating records** (requires DPoP auth):
```go
oauth.CreateRecord(session, "net.openmeet.survey", rkey, record)
```

**Listing records** (public, no auth):
```go
oauth.ListRecords(pdsURL, did, "net.openmeet.survey", cursor, limit)
```

**Deleting records** (requires DPoP auth):
```go
oauth.DeleteRecord(session, "net.openmeet.survey", rkey)
```

### Session Data

`OAuthSession` contains:
- `DID` - User's decentralized identifier
- `PDSUrl` - User's Personal Data Server URL
- `AccessToken` - Bearer token for PDS requests
- `DPoPKey` - Private key for DPoP proof signing
- `RefreshToken` - For token refresh

### DPoP Authentication

All PDS write operations require DPoP (Demonstration of Proof-of-Possession):
1. Generate DPoP proof with `CreateDPoPProof(key, method, url, nonce, accessToken)`
2. Add headers: `Authorization: DPoP {token}`, `DPoP: {proof}`
3. Handle nonce challenges if server responds with `use_dpop_nonce`

## Handler Patterns

### HTML Handlers

All HTML handlers follow this pattern:
```go
func (h *Handlers) SomePageHTML(c echo.Context) error {
    // 1. Get user (optional for public pages)
    user, profile := getUserAndProfile(c)

    // 2. Fetch data
    data, err := h.queries.GetSomething(c.Request().Context())

    // 3. Render template
    c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
    component := templates.SomePage(data, user, profile)
    return component.Render(c.Request().Context(), c.Response().Writer)
}
```

### Auth-Required Handlers

For pages requiring login:
```go
user := oauth.GetUser(c)
if user == nil {
    return c.String(http.StatusUnauthorized, "Authentication required")
}

session, err := oauth.GetSession(c, h.oauthStorage)
if err != nil || session == nil {
    return c.String(http.StatusUnauthorized, "Session not found")
}
```

## Testing

### MockQueries

Tests use `MockQueries` that implements `QueriesInterface`:
```go
type MockQueries struct {
    surveys   map[uuid.UUID]*models.Survey
    responses map[uuid.UUID]*models.Response
    // ...
}
```

### Test Setup Pattern

```go
func setupTest() (*echo.Echo, *MockQueries, *Handlers) {
    e := echo.New()
    mq := NewMockQueries()
    h := &Handlers{queries: mq}
    return e, mq, h
}
```

### Running Tests

```bash
# All tests
go test ./...

# Specific package with verbose output
go test -v ./internal/api/...

# Single test
go test -v ./internal/api/... -run TestMyDataHTML
```

## Templates (Templ)

Templates use [templ](https://templ.guide/) - Go's type-safe templating.

### Generate Templates

```bash
make templ
# or
templ generate
```

### Template Pattern

```go
templ SomePage(data *SomeData, user *oauth.User, profile *oauth.Profile) {
    @Layout("Page Title", user, profile) {
        <div class="card">
            // Page content
        </div>
    }
}
```

### Layout

`Layout` provides:
- Navigation bar with auth state
- User avatar/name when logged in
- "Login with ATProto" when not logged in

## Key Routes

| Route | Handler | Description |
|-------|---------|-------------|
| `GET /` | `LandingPage` | Landing with live stats |
| `GET /surveys` | `ListSurveysHTML` | Survey list |
| `GET /surveys/new` | `CreateSurveyPageHTML` | Create form |
| `POST /surveys` | `CreateSurveyHTML` | Submit new survey |
| `GET /surveys/:slug` | `SurveyFormHTML` | Answer survey |
| `GET /surveys/:slug/results` | `SurveyResultsHTML` | View results |
| `GET /my-data` | `MyDataHTML` | PDS browser overview |
| `GET /my-data/:collection` | `MyDataCollectionHTML` | List collection records |
| `GET /my-data/:collection/:rkey` | `MyDataRecordHTML` | Edit single record |
| `POST /my-data/delete` | `DeleteRecordsHTML` | Batch delete records |

## Anonymous vs Authenticated Voting

**Authenticated users (DID)**:
- Response stored with `voter_did`
- Written to user's PDS
- Can view in My Data

**Anonymous users**:
- Response stored with `voter_session` (SHA256 of surveyID + IP + userAgent)
- Local DB only, no PDS
- One vote per survey per session

## Database

PostgreSQL with these main tables:
- `surveys` - Survey definitions with `author_did`
- `responses` - Votes with `voter_did` OR `voter_session`
- `oauth_sessions` - OAuth session storage

## Environment Variables

```bash
DATABASE_URL=postgres://user:pass@host:5432/survey
OAUTH_CLIENT_ID=...
OAUTH_REDIRECT_URI=...
```

## Gotchas

### Template Changes Not Showing

Run `make templ` or `templ generate` after editing `.templ` files.

### PDS Write Failures

Common causes:
- Token expired (check `session.TokenExpiresAt`)
- Missing DPoP key
- Invalid collection name (must be `a.b.c` format)
- Need `validate: false` for custom lexicons

### JSON Display in Templates

Use `record.ValueJSON` (pre-formatted) instead of `fmt.Sprintf("%v", record.Value)` which outputs Go syntax.
