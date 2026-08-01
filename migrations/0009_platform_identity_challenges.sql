-- Platform-only global identity login has no app or role precondition.
CREATE TABLE platform_identity_challenges (
  id TEXT PRIMARY KEY,
  browser_binding_hash BLOB NOT NULL,
  normalized_email TEXT NOT NULL,
  code_hash BLOB NOT NULL,
  expires_at TEXT NOT NULL,
  attempts INTEGER NOT NULL CHECK(attempts >= 0),
  consumed_at TEXT,
  invalidated_at TEXT,
  request_fingerprint_hash BLOB,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_platform_identity_challenges_binding
  ON platform_identity_challenges(browser_binding_hash,normalized_email,expires_at);
