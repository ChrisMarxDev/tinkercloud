# Attachment Shelf

A private, app-scoped attachment shelf that demonstrates TinyHost blob
capability discovery, one-file upload, attachment-only download, cursor
listing, deletion, cancellation, and typed quota/error handling.

Files are utility-grade local VPS data. They have no public URL and TinyHost
V1 provides no backup or durability guarantee.

Build all SDK examples from the parent directory with `npm run build`, then run
`tiny deploy .` here. The manifest is owner-only until you add viewer emails or
domains deliberately.
