-- Local-only, bounded app insights. Marker digests are keyed by the gateway;
-- no raw cookie, identity, request metadata, or URL is stored.
CREATE TABLE local_insights_settings (singleton INTEGER PRIMARY KEY CHECK(singleton=1), enabled INTEGER NOT NULL CHECK(enabled IN (0,1)), updated_at TEXT NOT NULL);
INSERT INTO local_insights_settings(singleton,enabled,updated_at) VALUES(1,1,datetime('now')) ON CONFLICT(singleton) DO NOTHING;
CREATE TABLE app_insight_days (app_id TEXT NOT NULL REFERENCES applications(id), day TEXT NOT NULL, page_views INTEGER NOT NULL CHECK(page_views>=0), last_activity_at TEXT NOT NULL, PRIMARY KEY(app_id,day));
CREATE TABLE app_insight_visitors (app_id TEXT NOT NULL REFERENCES applications(id), day TEXT NOT NULL, marker_digest BLOB NOT NULL CHECK(length(marker_digest)=32), expires_at TEXT NOT NULL, PRIMARY KEY(app_id,day,marker_digest));
CREATE INDEX idx_app_insight_visitors_day ON app_insight_visitors(day);
