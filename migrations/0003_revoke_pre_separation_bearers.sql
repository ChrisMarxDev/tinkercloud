-- Before control browser sessions became a distinct credential type, browser
-- and CLI values shared api_tokens and cannot be classified safely on upgrade.
-- Revoke every pre-separation bearer; the next CLI login issues a new token.
UPDATE api_tokens SET revoked_at=datetime('now') WHERE revoked_at IS NULL;
