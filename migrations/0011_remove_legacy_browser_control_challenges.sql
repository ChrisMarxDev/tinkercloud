-- The unified browser broker no longer uses otp_challenges for dashboard
-- browser login. Keep only app-bound handoff challenges and explicitly marked
-- CLI challenges; discard every legacy/null/browser control challenge.
PRAGMA foreign_keys=OFF;

CREATE TABLE otp_challenges_v11 (
  id TEXT PRIMARY KEY,
  app_id TEXT REFERENCES applications(id),
  purpose TEXT NOT NULL CHECK(purpose IN ('viewer','control')),
  control_channel TEXT CHECK(control_channel IS NULL OR control_channel='cli'),
  normalized_email TEXT NOT NULL,
  code_hash BLOB NOT NULL,
  expires_at TEXT NOT NULL,
  attempts INTEGER NOT NULL CHECK(attempts>=0),
  consumed_at TEXT,
  invalidated_at TEXT,
  request_fingerprint_hash BLOB,
  created_at TEXT NOT NULL,
  CHECK((purpose='viewer' AND app_id IS NOT NULL AND control_channel IS NULL) OR
        (purpose='control' AND app_id IS NULL AND control_channel='cli')),
  UNIQUE(app_id,normalized_email,id)
);

INSERT INTO otp_challenges_v11(id,app_id,purpose,control_channel,normalized_email,code_hash,expires_at,attempts,consumed_at,invalidated_at,request_fingerprint_hash,created_at)
  SELECT id,app_id,purpose,control_channel,normalized_email,code_hash,expires_at,attempts,consumed_at,invalidated_at,request_fingerprint_hash,created_at
  FROM otp_challenges
  WHERE (purpose='viewer' AND app_id IS NOT NULL AND control_channel IS NULL)
     OR (purpose='control' AND app_id IS NULL AND control_channel='cli');

DROP TABLE otp_challenges;
ALTER TABLE otp_challenges_v11 RENAME TO otp_challenges;
CREATE INDEX idx_otp_challenges_app_email ON otp_challenges(app_id,normalized_email,expires_at);

PRAGMA foreign_keys=ON;
