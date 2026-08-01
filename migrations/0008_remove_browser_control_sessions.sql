-- Pre-production credential consolidation. Browser dashboard authority now
-- derives from the platform-host global identity family and the current users
-- role lookup; it is no longer a separate sessions.scope='control' credential.
-- CLI bearer credentials remain in api_tokens and are intentionally untouched.
PRAGMA foreign_keys=OFF;

CREATE TABLE sessions_v8 (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL CHECK(scope='app'),
  app_id TEXT NOT NULL REFERENCES applications(id),
  identity_id TEXT NOT NULL REFERENCES identities(id),
  user_id TEXT,
  secret_hash BLOB NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  last_seen_at TEXT,
  created_at TEXT NOT NULL,
  identity_session_id TEXT REFERENCES identity_sessions(id),
  CHECK(user_id IS NULL)
);

INSERT INTO sessions_v8(id,scope,app_id,identity_id,user_id,secret_hash,expires_at,revoked_at,last_seen_at,created_at,identity_session_id)
  SELECT id,scope,app_id,identity_id,user_id,secret_hash,expires_at,revoked_at,last_seen_at,created_at,identity_session_id
  FROM sessions WHERE scope='app';

DROP TABLE sessions;
ALTER TABLE sessions_v8 RENAME TO sessions;
CREATE INDEX idx_sessions_app_secret ON sessions(app_id,secret_hash);
CREATE INDEX idx_sessions_identity_session ON sessions(identity_session_id,revoked_at);

PRAGMA foreign_keys=ON;
