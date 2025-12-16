-- Add issuer column to oauth_sessions for token refresh
-- The issuer (auth server URL) is needed to refresh access tokens

ALTER TABLE oauth_sessions
ADD COLUMN issuer TEXT;

-- Note: Column is nullable to allow existing sessions to continue working
-- New sessions created after this migration will populate the issuer
