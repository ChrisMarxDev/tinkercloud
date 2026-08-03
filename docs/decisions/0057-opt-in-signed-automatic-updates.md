# 0057 — Opt-in signed automatic server updates

**Status:** Accepted

## Context

Tinkercloud already verifies an exact signed server artifact and compatibility
manifest, preserves a bounded previous-binary snapshot, restarts the service,
and restores that binary when health or gateway-denial evidence fails. The
remaining operator ceremony is discovering every new release and invoking the
same safe update manually.

The server's durable authority is not the binary. Configuration, root-owned
credentials, the control SQLite database, immutable app releases, app-local
databases and blobs, viewer sessions, deployer bearers, and ACME state must
survive every supported update. An automatic updater must not create a second
control plane or weaken the signed-release boundary.

## Decision

Tinkercloud provides an explicitly enabled automatic update channel. The
operator chooses `beta` or `stable` once through the root-local command surface.
A fixed systemd timer then performs bounded periodic checks with randomized
delay. Automatic updates are disabled until that command succeeds and can be
disabled root-locally at any time.

Channel discovery is untrusted input. It may identify candidate versions and
exact official GitHub release directories, but it grants no installation
authority. Tinkercloud accepts only a strictly newer semantic version whose
server artifact and complete release manifest pass the existing pinned
Ed25519, digest, origin, compatibility, restart, health, and anonymous-denial
gates. A beta channel selects prereleases; a stable channel excludes them.
Downgrades, same-version replacements, cross-origin assets, and malformed or
ambiguous discovery state deny before snapshot. The only redirect exception is
the bounded immutable official GitHub release-asset transport hop in ADR 0059;
it does not apply to discovery or custom origins.

Unattended updates initially require the installed persistence schema version
to remain unchanged. A schema-changing release stops with an operator-visible
status and requires a separately reviewed manual migration decision. This
keeps automatic rollback truthful: the prior binary remains compatible with
the same durable database.

The updater replaces only the installed server binary. It never replaces,
deletes, reconstructs, or copies the operator configuration, systemd
credentials, control database, app release tree, app-local databases, blobs,
or ACME cache. Existing valid CLI bearers, global browser identities, app
sessions, ownership, policies, and application data remain authoritative.

An empty instance is a valid update target. With no active app, the updater
must prove that exact database state, normal platform health and listeners, and
a safe denial on a fixed synthetic unknown app hostname. It must not require
the operator to create a probe app.

## Consequences

The normal operator action is one command followed by unattended safe checks,
not recurring release-base input. Network or discovery failure leaves the
installed version untouched. A failed candidate restores the prior binary and
rechecks health while retaining the same durable state.

Release acceptance must seed and retain an app, policy, CLI bearer, global
browser identity, app session, KV/document/blob data, and immutable release
across a real version update. It must repeat those assertions after an injected
failed candidate and automatic rollback. Local unit tests alone are not enough
to claim state-preserving automatic updates complete.

The update timer is an operator convenience, not another security authority.
Root access to the VPS remains the recovery boundary.
