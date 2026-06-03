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
- Embedded SQL migrations for the core ingest schema, applied by a separate
  `migrate` command.
- Frozen conformance vectors and a cross-check that the verifier and server
  agree on every hash.
