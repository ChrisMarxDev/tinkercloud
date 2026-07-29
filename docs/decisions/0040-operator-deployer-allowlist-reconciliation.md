# 0040 — Reconcile one active deployer allowlist

**Status:** Accepted for V1

The operator dashboard uses one revision-protected, server-rendered active-deployer allowlist instead of per-deployer browser actions. This makes the intended authority set reviewable as one atomic snapshot. Adding entries requires explicit broadening confirmation; removing an active entry revokes its API/control credentials and prevents deployment immediately. Existing deployer IDs and all owned app data remain stable. Audit failure rolls the entire reconciliation back.
