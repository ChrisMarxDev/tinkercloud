-- Global viewer identity stays on the platform host. App sessions remain
-- host-only child credentials and must never be accepted as platform identity.
CREATE TABLE identity_sessions (
  id TEXT PRIMARY KEY,
  identity_id TEXT NOT NULL REFERENCES identities(id),
  family_id TEXT NOT NULL,
  secret_hash BLOB NOT NULL UNIQUE,
  -- Hash of the platform host-only browser binding. It groups competing OTP
  -- completions without becoming an identity credential or policy input.
  browser_binding_hash BLOB NOT NULL,
  previous_secret_hash BLOB UNIQUE,
  previous_valid_until TEXT,
  expires_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  rotated_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  CHECK((previous_secret_hash IS NULL AND previous_valid_until IS NULL) OR
        (previous_secret_hash IS NOT NULL AND previous_valid_until IS NOT NULL))
);
CREATE INDEX idx_identity_sessions_identity ON identity_sessions(identity_id,revoked_at,expires_at);
CREATE UNIQUE INDEX idx_identity_sessions_browser_binding_active
  ON identity_sessions(browser_binding_hash) WHERE revoked_at IS NULL;

-- The app host creates the state and keeps it in a host-only cookie. Only the
-- opaque handoff id crosses to the platform identity host.
CREATE TABLE identity_handoffs (
  id TEXT PRIMARY KEY,
  app_id TEXT NOT NULL REFERENCES applications(id),
  state_hash BLOB NOT NULL UNIQUE,
  return_path TEXT NOT NULL,
  force_login INTEGER NOT NULL CHECK(force_login IN (0,1)),
  -- Bound when an OTP is requested. The raw cookie never reaches SQLite.
  browser_binding_hash BLOB,
  identity_session_id TEXT REFERENCES identity_sessions(id),
  expires_at TEXT NOT NULL,
  authorized_at TEXT,
  consumed_at TEXT,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  CHECK((identity_session_id IS NULL AND authorized_at IS NULL) OR
        (identity_session_id IS NOT NULL AND authorized_at IS NOT NULL)),
  CHECK(revoked_at IS NULL OR
        (identity_session_id IS NOT NULL AND authorized_at IS NOT NULL AND consumed_at IS NULL))
);
CREATE INDEX idx_identity_handoffs_expiry ON identity_handoffs(expires_at,consumed_at,revoked_at);
CREATE INDEX idx_identity_handoffs_browser_binding ON identity_handoffs(browser_binding_hash,expires_at);

ALTER TABLE sessions ADD COLUMN identity_session_id TEXT REFERENCES identity_sessions(id);
CREATE INDEX idx_sessions_identity_session ON sessions(identity_session_id,revoked_at);
