# Stonewrit

Tamper-evident audit evidence for sensitive system actions and AI agent
activity. Send normalized events; Stonewrit stores them in a hash-linked,
verifiable chain and maps them to compliance controls (SOC 2, ISO 27001, HIPAA,
GDPR).

The point of this repository is trust you do not have to take on faith. The
mechanism that makes an audit log tamper-evident is open and runnable: the
canonicalization, the hashing, the chain linkage, the verifier, the event spec,
and the baseline classifier. You can recompute every hash yourself.

[![test](https://github.com/stonewrit/stonewrit/actions/workflows/test.yml/badge.svg)](https://github.com/stonewrit/stonewrit/actions/workflows/test.yml)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

## How it works

Each event is reduced to a canonical form, hashed, and linked to its
predecessor:

1. The event content is canonicalized with RFC 8785 (JCS) so the same logical
   event always produces the same bytes, in any language.
2. `payload_hash = "sha256:" + sha256(canonical_content)`.
3. `event_hash = "sha256:" + sha256(previous_event_hash + ":" + payload_hash + ":" + position)`.
4. The first event in a chain links to the literal `"genesis"`.

Because each event hash commits to the one before it, you cannot alter, drop, or
reorder a past event without breaking every hash after it. The scheme is frozen
and pinned by conformance vectors in
[`core/testdata/golden_vectors.json`](core/testdata/golden_vectors.json), so
independent implementations agree by construction.

## Repository layout

| Path | What it is |
|---|---|
| [`core/`](core) | The pure mechanism: canonicalization, payload and event hashing, chain verification. No database, no HTTP, no clock. |
| [`spec/`](spec) | The event schema and the frozen conformance vectors every implementation checks against. |
| [`classify/`](classify) | The rule engine plus the open baseline ruleset that maps events to controls. |
| [`cmd/verify/`](cmd/verify) | A standalone CLI to verify an evidence bundle without trusting any server. |
| [`server/`](server) | The ingest service: validate a key, hash, classify, seal into the chain, write to Postgres. |
| [`docs/`](docs) | Architecture, deployment, and scaling notes. |

## Quick start

### Verify a bundle

Anyone can check a Stonewrit evidence bundle. From source:

```
go run ./cmd/verify path/to/bundle.json
```

It walks each chain, recomputes every event hash, checks the chain against its
published head, and exits non-zero if anything does not line up. Released
binaries are published as `stonewrit-verify` for Linux, macOS, and Windows.

### Run the server

The fastest path is the local stack, which starts Postgres, applies migrations,
and runs the server:

```
docker compose up --build
```

Or run it against your own Postgres:

```
cp .env.example .env          # set DATABASE_URL
make migrate                  # create the schema
make run                      # start the server on :3002
```

Health endpoints: `/health`, `/ready`, `/dbping`.

## Open core

Everything in this repository is Apache 2.0. The open project is the trust
mechanism and a self-hostable ingest server. The hosted product adds the parts
that are a service rather than a mechanism:

- Neutral, third-party witnessing that anchors chain heads, so a log's operator
  cannot quietly rewrite their own history.
- Maintained and attested premium rulesets kept current as regulations move.
- The multi-tenant dashboard, management API, billing, and SSO.

Opening the verifier and the spec does not weaken any of that. A verifier you
can run is a stronger trust claim than a black box.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md). In short:

```
make test             # unit tests, race detector, coverage
make test-integration # integration tests against a throwaway Postgres
make lint             # golangci-lint
```

## License

[Apache License 2.0](LICENSE).
