# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims
to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Open-core monorepo layout: `core` (canonicalization, hashing, chain linkage,
  verification), `spec` (event schema and frozen conformance vectors),
  `classify` (rule engine and baseline ruleset), and the `server` ingest service.
- `stonewrit-verify`, a standalone CLI to independently verify an evidence
  bundle.
- `stonewrit` management CLI: `migrate`, `project`, `environment`, and `key`
  subcommands for self-hosting without a dashboard.
- Auth-optional server. `APIKEY_AUTH` is off by default; the server provisions a
  built-in default tenant on boot so ingest works with zero seeding. Set it true
  to require CLI-minted API keys.
- Boot-time seeding of the baseline compliance catalog, so events map to
  controls with no manual setup.
- Frozen conformance vectors and a cross-check that the verifier and server
  agree on every hash.

### Removed

- All multi-tenant SaaS machinery from the open server: subscriptions, billing
  and metering, per-tenant rate limiting, and the Better-Auth key tables. The
  open server has no users, organizations, or subscriptions.
