# M3 deployment restart denial charter

Startup recovery is database-led. It enumerates durable deployment records in
stable `(app_id, id)` order and derives at most one immutable release location
from each record; it never scans or deletes filesystem trees.

The deny paths are mandatory:

- `uploading`, `uploaded`, `validating`, and `staged` records become `failed`
  on restart and are never resumed from an old staging directory;
- `verified`, `active`, and `superseded` records are retained only when the
  derived content-addressed release exactly matches the persisted hash and
  complete file evidence; missing, substituted, malformed, or unreadable
  evidence becomes `failed`;
- a failed active record clears the current deployment pointer and marks the
  application unavailable before the gateway can serve release bytes;
- rejected and already-failed rows remain unchanged; repeated recovery is
  idempotent;
- database read/write errors abort startup recovery rather than guessing a
  state or treating disk paths as authority.
