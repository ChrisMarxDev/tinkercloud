# Local app development

`tinker dev` is a development-only loopback server for static Tinkercloud apps. It
uses normal local SQLite state and provides current viewer/app information, KV,
basic document collections, and live KV/collection freshness notifications. It does not implement
login, deployment, blobs, LLM/provider capabilities, or any production access
policy. It is intentionally not a substitute for Tinkercloud verification.

Run it from an app project directory:

```sh
tinker dev
```

When `tinker.yaml` exists, `tinker dev` serves its `build.output`; otherwise it
serves the supplied directory (or the current directory). It prints an obvious
development identity and URL. State is project-local at `.tinker/local/` and is
already ignored by the repository's `.tinker/` rule. The command accepts only
`localhost` or numeric loopback listeners. For a second app use a different
port:

```sh
tinker dev ./my-app --listen 127.0.0.1:8788
```

The existing SDK's `tinker.user`, `tinker.app`, `tinker.capabilities`, `tinker.kv`, and
`tinker.live.onKvChange` work unchanged. The preview document routes are
`tinker.db.collection("tasks")` uses the same `/_tinker/api/v1/db/{collection}`
route shape as Tinkercloud: `POST` creates a server-generated document ID, while
`GET`, `PUT`, `DELETE`, bounded listing, snapshot revisions, and
`subscribe_collection`/`collection.changed` are available for local SDK work.
Every KV/document mutation and its collection revision is one local SQLite
transaction. Collection events are published only after that transaction
commits; failed optimistic-version writes never publish an event. Treat events
as refresh hints and let the SDK reconcile snapshots, exactly as on Tinkercloud.

The emulator accepts one strict JSON value per mutation request and uses the
same opaque `doc_` ID grammar as the hosted capability. It remains a local
developer convenience, not a replacement for production policy, quota,
provider, or gateway testing.

The built SDK is exercised against this loopback surface for KV, collection
CRUD/snapshots, and reconnect recovery. It accepts only loopback browser
origins and the current SDK live protocol; it is not a permissive WebSocket
test server.

Stop with Ctrl-C. State survives restart in the project-local state directory.
Never bind the development server beyond loopback: it deliberately refuses
such configuration.
