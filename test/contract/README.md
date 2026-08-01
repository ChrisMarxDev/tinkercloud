# Contract Test Charters

Future tests in this directory should validate stable boundaries without
depending on private implementation details:

- all routes have an authorization classification;
- protected handlers require an authorization context;
- HTTP JSON/error examples match the spec;
- `tinker.yaml` parsing follows precedence and secure defaults;
- event payloads obey redaction and version schemas;
- state transition tables reject illegal moves;
- migrations work on empty and previous supported databases.

No executable tests are included in the concept scaffold.
