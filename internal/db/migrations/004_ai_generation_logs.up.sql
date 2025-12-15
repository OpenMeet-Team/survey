-- AI Generation Logging
-- Tracks all AI survey generation requests/responses for debugging and analysis

CREATE TABLE ai_generation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- DID for authenticated, IP hash for anonymous
    user_type TEXT NOT NULL CHECK (user_type IN ('anonymous', 'authenticated')),
    input_prompt TEXT NOT NULL,
    system_prompt TEXT NOT NULL,
    raw_response TEXT, -- NULL if generation failed
    status TEXT NOT NULL CHECK (status IN ('success', 'error', 'rate_limited', 'validation_failed')),
    error_message TEXT,
    input_tokens INT NOT NULL DEFAULT 0,
    output_tokens INT NOT NULL DEFAULT 0,
    cost_usd DECIMAL(10, 6) NOT NULL DEFAULT 0.0,
    duration_ms INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for debugging specific users
CREATE INDEX idx_ai_generation_logs_user_id ON ai_generation_logs(user_id);

-- Index for querying by status (to find failures)
CREATE INDEX idx_ai_generation_logs_status ON ai_generation_logs(status);

-- Index for time-based queries
CREATE INDEX idx_ai_generation_logs_created_at ON ai_generation_logs(created_at DESC);
