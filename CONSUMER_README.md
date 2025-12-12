# Survey Jetstream Consumer

ATProto Jetstream consumer that indexes surveys and responses from Bluesky into PostgreSQL.

## What It Does

Consumes real-time ATProto events and indexes them:
- **Surveys** (`net.openmeet.survey`) → `surveys` table
- **Responses** (`net.openmeet.survey.response`) → `responses` table

Handles create/update/delete operations for both collections.

## Key Features

- **Cursor-based resumption** - Resumes from last processed message after restart
- **Atomic processing** - Message + cursor update in single transaction
- **Automatic reconnection** - Exponential backoff (1s → 60s max)
- **Slug auto-generation** - Creates URL-friendly slugs from survey names
- **Duplicate prevention** - Skips duplicate votes (one per DID per survey)

## Architecture

```
Jetstream → Parser → Processor → PostgreSQL
             ↓
        Cursor tracking
```

**Files:**
- `jetstream.go` - WebSocket client + reconnection
- `processor.go` - Message routing (create/update/delete)
- `atproto.go` - Lexicon parsing (ATProto → our models)
- `cursor.go` - Resumption point tracking

## Record Mapping

### Survey (`net.openmeet.survey`)

ATProto lexicon uses `name`, we use `title`:
```json
{"name": "Team Meeting"} → {title: "Team Meeting", slug: "team-meeting"}
```

Question types have token prefix:
```json
{"type": "net.openmeet.survey#single"} → {type: "single"}
```

### Response (`net.openmeet.survey.response`)

Field is `selectedOptions` (not `selected`):
```json
{"selectedOptions": ["opt1"]} → {selectedOptions: ["opt1"]}
```

Voter DID comes from `commit.repo`, not record body.

## Running

### Build
```bash
go build -o bin/consumer ./cmd/consumer
```

### Environment
```bash
export DATABASE_PASSWORD=your_password
./bin/consumer
```

### Graceful Shutdown
`SIGTERM` or Ctrl+C saves cursor and closes WebSocket cleanly.

## Deployment

**Critical:** Must run as **single replica** (WebSocket is stateful).

```yaml
replicas: 1  # Required
```

## Cursor Management

Single-row table tracks progress:
```sql
CREATE TABLE jetstream_cursor (
    id INT PRIMARY KEY DEFAULT 1,
    time_us BIGINT NOT NULL,
    CHECK (id = 1)
);
```

URL changes on resume:
- First run: `wss://jetstream2.../subscribe?wantedCollections=...`
- Resume: `...?wantedCollections=...&cursor=1234567890`

## Error Handling

| Error Type | Behavior |
|-----------|----------|
| Connection error | Reconnect with backoff, resume from cursor |
| Processing error | Log + skip message, cursor NOT updated |
| Validation error | Log + skip, cursor NOT updated |

## Testing

Tests skip when DB unavailable (CI-safe):
```bash
go test ./internal/consumer -v
```

With test DB:
```bash
createdb survey_test
psql survey_test < internal/db/migrations/001_initial.up.sql
go test ./internal/consumer -v
```

## Troubleshooting

**Not receiving messages?**
- Check only 1 replica running
- Verify cursor: `SELECT * FROM jetstream_cursor;`

**Duplicate processing?**
- Multiple replicas running (check Kubernetes/Docker)

**Surveys not indexed?**
- Check logs for validation errors
- Verify lexicon matches schema
