-- Development-stage destructive transition. Before TinyHost is released,
-- discard the old shared control-database KV table rather than attempting a
-- partial copy into per-app databases. All production app KV access now
-- requires apps/{immutable-app-id}/data.db.
DROP TABLE app_kv;
