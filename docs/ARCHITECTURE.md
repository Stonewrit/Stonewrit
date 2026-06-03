# Architecture

Stonewrit turns a stream of events into a tamper-evident, independently
verifiable audit log. This document explains the pieces, the boundary that keeps
the trust mechanism pure, and the open-core split.

## Principle

A hash chain is commodity logic. Keeping it closed protects nothing and weakens
the trust claim: a black box asks you to trust the operator, while an open
verifier lets you check the operator. So the mechanism is open, and the value
that is genuinely a service stays hosted.

What is open: canonicalization, hashing, chain linkage, the verifier, the event
spec, the rule engine, and the baseline ruleset. What is hosted: neutral
third-party witnessing, maintained premium rulesets, and the multi-tenant
product surface.

## The layers

```
core    pure mechanism: canonicalize, hash, chain, verify. No DB, HTTP, clock, or log.
spec    the event schema and the frozen conformance vectors every implementation reads.
classify    rule engine plus the open baseline ruleset (a plugin around core, never inside it).
server  ingest service: validate a key, hash via core, classify, seal, write to Postgres.
cmd/verify  a thin CLI built on core alone, for verifying an exported bundle.
```

The server and the public verifier import the exact same `core`. That shared
identity is the integrity of the whole system: a chain produced by any server
verifies under the same public verifier.

## What belongs in core

`core` is pure and deterministic: the same input bytes produce the same output
hash on any machine, in any language, forever. If a function needs a database, a
clock, a network call, configuration, or a log line, it does not belong in
`core`; the caller does that and passes the results in.

In `core`:

- RFC 8785 (JCS) canonicalization.
- Payload hashing and event-hash chain linkage.
- Deterministic time formatting (the caller supplies the time value).
- Single-event and full-chain verification.
- The content-payload builder, shared by the accept path and the verifier so the
  two cannot drift.
- The frozen version constants and conformance vectors.

Out of `core`: Postgres, HTTP, routing, authentication, API keys, tenant
scoping, rate limiting, classification rules, agent scope checks, `time.Now()`,
surrogate id generation, logging, metrics, and configuration. Authentication,
classification, and scope checks are plugins around `core`, never inside it.

## The frozen pre-image

This scheme is fixed. Changing it would invalidate every chain ever produced, so
a change is a new `CanonicalizationVersion`, never a silent edit.

1. `payload_hash = "sha256:" + sha256hex( CanonicalJSON(content_payload) )`.
2. `event_hash = "sha256:" + sha256hex( previous_event_hash + ":" + payload_hash + ":" + position )`.
3. The first event in a chain uses the literal string `"genesis"` as its
   previous hash.
4. `position` is a base-10 decimal with no padding.

The pre-image chains the `payload_hash` string (already prefixed with
`sha256:`), not the raw canonical bytes, because the payload hash already commits
to the content. The content payload is the field set the spec defines; the
computed control mappings and scope check are stored separately and are not part
of the hash, so opening the classifier does not touch verifiability.

## Cross-language parity

The hard part of JCS is that key ordering, number formatting, and UTF-8 handling
must be byte-identical across languages. Do not trust a language's default JSON
serializer to be JCS compliant. Use a real canonicalizer and lock it with the
shared conformance vectors in `core/testdata/golden_vectors.json`. The vectors
already cover the known edge cases: unicode, control characters, multiline
strings, large integers, empty arrays, and nested objects.

## The accept and seal path

Ingest is split into a fast accept and a background seal so the hot path stays
small:

1. Accept validates the key, builds the content payload, computes
   `payload_hash` via `core`, classifies, and writes a row to `pending_events`,
   returning a content-hash receipt.
2. The sealer drains pending rows per shard under a row lock, assigns chain
   positions, computes `event_hash` via `core`, and writes the immutable
   `events` journal plus a chain checkpoint.

Sharding is an internal write-throughput detail. Auditors group chains by
environment and verify each chain's head.

## Migrations

The open server ships its own SQL migrations for the core ingest tables it
writes: events, chains, pending events, projects, environments, an API-key
store, and the supporting catalog and metering tables. They are embedded in the
binary and applied by a separate `migrate` command, never automatically on
startup, so schema changes stay an explicit operational step. A self-hoster
points the server at any Postgres and runs `migrate up` once.

## Open and hosted

The open server is a complete ingest data plane: validate, classify with the
baseline ruleset, seal, and chain. The hosted product wraps it with the
commercial concerns that are a service rather than a mechanism:

- Neutral witnessing and anchoring. A fully self-hosted log is one its operator
  could alter; periodically publishing signed chain heads to an external
  transparency log is the value a neutral witness adds. The sealer exposes an
  anchoring hook with a no-op default; the hosted deployment supplies the real
  implementation.
- Maintained premium rulesets, kept current and attested as regulations move.
- The dashboard, management API, billing and metering, and SSO.
