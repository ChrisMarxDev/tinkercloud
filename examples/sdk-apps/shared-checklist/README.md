# Shared Checklist

A small collaborative checklist showing the complete KV lifecycle:

- current viewer, app, and capability discovery;
- bounded prefix listing with cursors;
- create, versioned update, and versioned delete;
- `AbortSignal` cancellation and typed errors; and
- `tiny.live.onKvChange()` as a refresh hint.

Build all SDK examples from the parent directory with `npm run build`, then run
`tiny deploy .` here. The manifest is owner-only until you add viewer emails or
domains.
