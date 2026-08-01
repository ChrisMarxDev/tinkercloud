-- Pre-production clean-slate cutover. The old direct app OTP path is gone:
-- every child app session must have a durable global identity parent. Drop
-- parentless rows rather than preserving a second browser credential shape.
PRAGMA foreign_keys=OFF;

CREATE TABLE sessions_v10 (
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
  identity_session_id TEXT NOT NULL REFERENCES identity_sessions(id),
  CHECK(user_id IS NULL)
);

INSERT INTO sessions_v10(id,scope,app_id,identity_id,user_id,secret_hash,expires_at,revoked_at,last_seen_at,created_at,identity_session_id)
  SELECT s.id,s.scope,s.app_id,s.identity_id,s.user_id,s.secret_hash,s.expires_at,s.revoked_at,s.last_seen_at,s.created_at,s.identity_session_id
  FROM sessions s JOIN identity_sessions parent ON parent.id=s.identity_session_id
  WHERE s.scope='app';

DROP TABLE sessions;
ALTER TABLE sessions_v10 RENAME TO sessions;
CREATE INDEX idx_sessions_app_secret ON sessions(app_id,secret_hash);
CREATE INDEX idx_sessions_identity_session ON sessions(identity_session_id,revoked_at);

PRAGMA foreign_keys=ON;
