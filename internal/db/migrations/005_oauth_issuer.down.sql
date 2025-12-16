-- Remove issuer column from oauth_sessions

ALTER TABLE oauth_sessions
DROP COLUMN issuer;
