# 0041 — Minimum-necessary guided human flows

**Status:** Accepted for V1

## Context

Tinkercloud accumulated technically valid commands that required people to prepare
configuration files, repeat server flags, and understand internal setup order.
Those requirements are useful for deterministic automation but are unnecessary
ceremony for a human whose actual goal is “set up this VPS” or “deploy this
app.”

## Decision

Human commands are outcome-first, inference-first, and resumable. They inspect
current state, reuse verified platform and credential state, derive secure
defaults, and ask one bounded question only when a required value cannot be
discovered or safely defaulted. Optional values appear behind one review/edit
step rather than as mandatory questions.

`tinkercloud setup` owns generation of non-secret server configuration. It may ask
for a base domain, operator email, and a Resend credential source, then derive
 conventional platform/app hostnames, sending defaults, and the internal ACME
 contact from the normalized operator email. It never asks for or accepts a
 separate ACME-contact value. Missing external DNS
or email state produces one exact external action and a resumable continuation.
Resend records are collected before the DNS checkpoint so Tinkercloud and provider
records can be added in one DNS-provider session.
Secrets never appear in argv or ordinary config: setup consumes a root-readable
file or writes one from a no-echo prompt directly into the root-owned
credential boundary.

`tinker deploy [DIR]` owns first-run local onboarding. It reuses a verified default
platform and bearer, inspects the project, derives safe slug/output defaults,
defaults access to owner-only, and asks only about missing or ambiguous required
state. One review/edit screen ends in one final deploy action whose label names
any access broadening. It writes `tinker.yaml` as an atomic, reviewable receipt
without a separate confirmation; the deployer need not author YAML first.

JSON and other non-interactive modes never prompt, guess, or mutate missing
prerequisites. Authorization broadening always requires explicit confirmation.

## Consequences

The natural human path becomes shorter without weakening deterministic
automation or secret boundaries. Config files remain valuable records, but
their schemas no longer dictate the order or number of human questions.
Every new prompt must name the decision it unlocks and demonstrate that the
value is necessary, not already known, and unsafe to default.
