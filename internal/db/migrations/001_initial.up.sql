-- Initial schema for survey service
-- Based on Survey Service Implementation Plan

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Cursor tracking for Jetstream consumer (future use)
CREATE TABLE jetstream_cursor (
    id INT PRIMARY KEY DEFAULT 1,
    time_us BIGINT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CHECK (id = 1)  -- Single row table
);

-- Initialize cursor with 0 (will start from beginning)
INSERT INTO jetstream_cursor (id, time_us) VALUES (1, 0);

-- Surveys table
CREATE TABLE surveys (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,

    -- ATProto location (nullable until published to PDS)
    uri TEXT UNIQUE,           -- at://did:plc:xxx/net.openmeet.survey/rkey
    cid TEXT,

    -- Core fields
    author_did TEXT,           -- Creator's DID (nullable for web-only surveys)
    slug TEXT UNIQUE NOT NULL, -- URL-friendly identifier
    title TEXT NOT NULL,
    description TEXT,
    definition JSONB NOT NULL, -- Parsed survey structure (questions, options)

    -- Lifecycle
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Finalized results (set when author publishes aggregated results)
    results_uri TEXT,
    results_cid TEXT
);

-- Index for slug lookups (primary access pattern)
CREATE INDEX idx_surveys_slug ON surveys(slug);

-- Index for listing surveys by creation time
CREATE INDEX idx_surveys_created_at ON surveys(created_at DESC);

-- Responses table
CREATE TABLE responses (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    survey_id UUID NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,

    -- Identity (one of these will be set)
    -- ATProto user: did:plc:xxx
    voter_did TEXT,
    -- Web guest: sha256(survey_id + IP + UserAgent) - per-survey salted for privacy
    voter_session TEXT,

    -- ATProto location (nullable for web votes)
    record_uri TEXT,
    record_cid TEXT,

    -- Response data
    -- Format: {"q1": ["mon"], "q2": ["planning", "demos"], "q3": "Great idea!"}
    answers JSONB NOT NULL,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints: one response per voter per survey
    -- Only one of voter_did or voter_session should be set
    CONSTRAINT chk_voter_identity CHECK (
        (voter_did IS NOT NULL AND voter_session IS NULL) OR
        (voter_did IS NULL AND voter_session IS NOT NULL)
    )
);

-- One response per ATProto user per survey
CREATE UNIQUE INDEX idx_responses_survey_voter_did
    ON responses(survey_id, voter_did)
    WHERE voter_did IS NOT NULL;

-- One response per web guest per survey
CREATE UNIQUE INDEX idx_responses_survey_voter_session
    ON responses(survey_id, voter_session)
    WHERE voter_session IS NOT NULL;

-- Index for aggregating responses by survey
CREATE INDEX idx_responses_survey_id ON responses(survey_id);

-- Index for looking up ATProto records (for Jetstream consumer)
CREATE INDEX idx_responses_record_uri ON responses(record_uri) WHERE record_uri IS NOT NULL;

-- Index for looking up surveys by results record
CREATE INDEX idx_surveys_results_uri ON surveys(results_uri) WHERE results_uri IS NOT NULL;
