-- Durable operator-owned global gate for the public-static pilot.
PRAGMA foreign_keys=OFF;
CREATE TABLE access_policies_l3 (app_id TEXT NOT NULL REFERENCES applications(id), revision INTEGER NOT NULL, mode TEXT NOT NULL CHECK(mode IN ('private','public')), created_by TEXT REFERENCES users(id), created_at TEXT NOT NULL, PRIMARY KEY(app_id,revision));
INSERT INTO access_policies_l3(app_id,revision,mode,created_by,created_at) SELECT app_id,revision,mode,created_by,created_at FROM access_policies;
CREATE TABLE access_rules_l3 (id TEXT PRIMARY KEY, app_id TEXT NOT NULL, policy_revision INTEGER NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('owner','email','domain')), normalized_value TEXT NOT NULL, created_by TEXT REFERENCES users(id), created_at TEXT NOT NULL, UNIQUE(app_id,policy_revision,kind,normalized_value), FOREIGN KEY(app_id,policy_revision) REFERENCES access_policies_l3(app_id,revision));
INSERT INTO access_rules_l3(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) SELECT id,app_id,policy_revision,kind,normalized_value,created_by,created_at FROM access_rules;
DROP TABLE access_rules;
DROP TABLE access_policies;
ALTER TABLE access_policies_l3 RENAME TO access_policies;
ALTER TABLE access_rules_l3 RENAME TO access_rules;
CREATE TABLE public_static_settings (singleton INTEGER PRIMARY KEY CHECK(singleton=1), enabled INTEGER NOT NULL CHECK(enabled IN (0,1)), revision INTEGER NOT NULL CHECK(revision>0), updated_at TEXT NOT NULL);
INSERT INTO public_static_settings(singleton,enabled,revision,updated_at) VALUES(1,0,1,datetime('now'));
ALTER TABLE deployments ADD COLUMN public_acknowledged INTEGER NOT NULL DEFAULT 0 CHECK(public_acknowledged IN (0,1));
PRAGMA foreign_keys=ON;
