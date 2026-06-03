# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities privately. Do not open a public issue or
pull request for a security problem.

Use GitHub's private vulnerability reporting at
[Security Advisories](https://github.com/stonewrit/stonewrit/security/advisories/new),
or email security@stonewrit.com.

Please include enough detail to reproduce: affected version or commit, a
description of the issue, and a proof of concept if you have one. We aim to
acknowledge reports within three business days.

## Supported versions

Until a 1.0 release, security fixes target the `main` branch and the most recent
tagged release.

## Scope and threat model

Stonewrit's value is tamper-evidence: an event hash chain that anyone can verify
independently. Reports we are especially interested in:

- A way to make `core` produce a different hash for the same logical input
  across runs or platforms, or to break canonicalization parity.
- A way to forge or alter a sealed chain so that the public verifier still
  reports it as valid.
- Authentication or tenant-scoping flaws in the server that let one tenant read
  or write another tenant's events.
- Any path that lets the accept or seal path persist an event whose stored
  `payload_hash` or `event_hash` does not match its content.

The conformance vectors in `core/testdata/golden_vectors.json` are the frozen
reference for hashing behavior. A genuine drift there is a security issue.
