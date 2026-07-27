# Team Pulse

A lightweight current-status board showing:

- current viewer and app information;
- capability discovery and graceful manual-refresh fallback;
- one KV record per server-derived viewer identity;
- custom channel subscribe, publish, status, and close; and
- KV rereads after live hints, reconnect, and tab visibility changes.

Pulse records are current shared app state, not reliable online presence.
Build from the parent directory with `npm run build`, then deploy here with
`tiny deploy .`.
