# State Machines

State transitions should be explicit domain operations. No repository caller may
set state columns arbitrarily.

## Application

```text
creating ──valid policy──▶ active
   │                        │  │
   └────failure────────▶ failed │
                            │   │
                    suspend │   │ delete
                            ▼   ▼
                         suspended ──▶ deleting ──▶ permanently removed
                            │
                         resume
                            ▼
                          active
```

Only `active` apps resolve in the app plane. `creating`, `failed`, `suspended`,
and transient `deleting` states fail closed. Successful deletion removes the
application record and its owned data rather than retaining a `deleted` state.

## Deployment

```text
uploading
   ├── upload failed ──────────────▶ failed
   ▼
uploaded
   ├── invalid archive ────────────▶ rejected
   ▼
validating
   ├── validation failed ─────────▶ rejected
   ▼
staged
   ├── policy/TLS unavailable ────▶ failed
   ▼
verified
   ├── activation transaction fail▶ failed
   ▼
active ───────────────────────────── superseded
   │
   └── post-activation probe fails → failed + previous restored
```

`rejected` means user input did not meet a contract. `failed` means an
operational step could not complete. Both are terminal; retry creates a new
deployment attempt with an idempotency relationship.

## OTP challenge

```text
pending
  ├── correct + currently allowed ─▶ consumed
  ├── expiry ──────────────────────▶ expired
  ├── too many attempts ──────────▶ locked
  └── superseded by new challenge ▶ invalidated
```

Verification locks the challenge record so concurrent correct submissions can
create at most one session.

## Session

```text
active
  ├── expiry ─────────▶ expired
  ├── explicit revoke ▶ revoked
  └── security rotate ▶ rotated ──▶ replacement active
```

Policy denial does not need to mutate the session. A session proves
authentication, not continuing authorization.

## Token

```text
active ──expire──▶ expired
   ├────revoke───▶ revoked
   └────rotate───▶ revoked + new active token
```

Token scope is evaluated on every control-plane command.

## Server update

```text
checking → downloaded → verified → rollback-ready → activating → healthy
    │           │          │            │              │
    └───────────┴──────────┴────────────┴──────────────▶ failed
                                               │
                                               └── health fails → rolled-back
```

Only signed compatible artifacts can reach `verified`. Activation requires a
bounded local recovery snapshot. Failed health checks restore the prior binary
and any update-specific state transition. General backup/restore is deferred.
