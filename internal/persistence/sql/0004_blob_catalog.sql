CREATE TABLE app_blobs (
  id TEXT NOT NULL,
  app_id TEXT NOT NULL REFERENCES applications(id),
  state TEXT NOT NULL CHECK(state IN ('staging','ready','deleting')),
  display_name TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK(size_bytes>=0),
  content_hash TEXT NOT NULL,
  created_by_identity_id TEXT NOT NULL REFERENCES identities(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(app_id,id)
);
CREATE INDEX idx_app_blobs_ready_order ON app_blobs(app_id,state,created_at,id);
