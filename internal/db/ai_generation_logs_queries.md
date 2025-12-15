# AI Generation Logs - Query Examples

## Debugging Failed Generations

Find recent failed generations to debug issues:

```sql
-- Get all failed generations from today
SELECT
    id,
    user_id,
    user_type,
    input_prompt,
    error_message,
    duration_ms,
    created_at
FROM ai_generation_logs
WHERE status IN ('error', 'validation_failed')
    AND created_at >= CURRENT_DATE
ORDER BY created_at DESC;
```

## Inspecting a Specific Generation

View full details of a generation (including system prompt and raw LLM response):

```sql
-- Get complete details for a specific generation
SELECT
    id,
    user_id,
    user_type,
    input_prompt,
    system_prompt,
    raw_response,
    status,
    error_message,
    input_tokens,
    output_tokens,
    cost_usd,
    duration_ms,
    created_at
FROM ai_generation_logs
WHERE id = '<uuid-here>'
    OR user_id = 'did:plc:xxx'  -- or specific user
ORDER BY created_at DESC
LIMIT 10;
```

## User Activity Analysis

Find heavy users or potential abuse:

```sql
-- Top users by generation count (last 24 hours)
SELECT
    user_id,
    user_type,
    COUNT(*) as generation_count,
    SUM(cost_usd) as total_cost,
    COUNT(*) FILTER (WHERE status = 'success') as successful,
    COUNT(*) FILTER (WHERE status = 'rate_limited') as rate_limited
FROM ai_generation_logs
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY user_id, user_type
ORDER BY generation_count DESC
LIMIT 20;
```

## Cost Analysis

Track AI generation costs:

```sql
-- Daily cost breakdown
SELECT
    DATE(created_at) as date,
    COUNT(*) as total_requests,
    COUNT(*) FILTER (WHERE status = 'success') as successful,
    SUM(input_tokens) as total_input_tokens,
    SUM(output_tokens) as total_output_tokens,
    SUM(cost_usd) as total_cost
FROM ai_generation_logs
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

## Performance Monitoring

Track generation performance:

```sql
-- Average generation time by status
SELECT
    status,
    COUNT(*) as count,
    AVG(duration_ms) as avg_duration_ms,
    MIN(duration_ms) as min_duration_ms,
    MAX(duration_ms) as max_duration_ms,
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms) as median_duration_ms,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms) as p95_duration_ms
FROM ai_generation_logs
WHERE created_at >= NOW() - INTERVAL '7 days'
GROUP BY status;
```

## Finding Problematic Prompts

Identify patterns in failed generations:

```sql
-- Most common error messages
SELECT
    error_message,
    COUNT(*) as occurrences,
    AVG(duration_ms) as avg_duration_ms
FROM ai_generation_logs
WHERE status IN ('error', 'validation_failed')
    AND created_at >= NOW() - INTERVAL '7 days'
GROUP BY error_message
ORDER BY occurrences DESC
LIMIT 10;
```

## Rate Limiting Analysis

Check rate limit effectiveness:

```sql
-- Rate limit hits by user type
SELECT
    user_type,
    DATE(created_at) as date,
    COUNT(*) as rate_limit_hits,
    COUNT(DISTINCT user_id) as unique_users
FROM ai_generation_logs
WHERE status = 'rate_limited'
    AND created_at >= NOW() - INTERVAL '7 days'
GROUP BY user_type, DATE(created_at)
ORDER BY date DESC, user_type;
```

## Cleanup Old Logs

Remove logs older than 90 days (run periodically):

```sql
-- WARNING: This deletes data permanently
DELETE FROM ai_generation_logs
WHERE created_at < NOW() - INTERVAL '90 days';
```
