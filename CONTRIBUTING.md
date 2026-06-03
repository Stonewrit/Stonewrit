# Contributing to Stonewrit

Thanks for your interest in contributing. This guide covers how to build, test,
and submit changes.

## Ground rules

- Be respectful. See the [Code of Conduct](CODE_OF_CONDUCT.md).
- Open an issue before large changes so we can agree on direction.
- Security issues go through private disclosure, not public issues or pull
  requests. See [SECURITY.md](SECURITY.md).

## Developer Certificate of Origin (DCO)

Every commit must be signed off. By signing off you certify that you wrote the
change or otherwise have the right to submit it under the project license (see
[developercertificate.org](https://developercertificate.org)). Add the sign-off
with the `-s` flag:

```
git commit -s -m "core: add nested-array conformance vector"
```

This appends a `Signed-off-by` line to your commit message. Stonewrit is an
open-core project; the DCO keeps contribution provenance clear.

## Prerequisites

- Go 1.25 or newer
- Docker (for integration tests and the local stack)
- `sqlc` if you change SQL queries

## Build and test

```
make build            # build the server, migrate, and verify binaries
make test             # unit tests with the race detector and coverage
make test-integration # integration tests against a throwaway Postgres
make lint             # golangci-lint
make fmt              # gofmt
make vet              # go vet
make fuzz             # fuzz the canonicalizer
```

Run the whole local stack with Postgres and migrations:

```
make up
```

## The frozen contract

`core/` and `spec/` define the tamper-evidence mechanism. The canonicalization
rules, the hash pre-image, and the conformance vectors in
`core/testdata/golden_vectors.json` are frozen. Changing any of them invalidates
every chain ever produced, so it is never a silent edit:

- If a change alters a computed hash, it is a new `CanonicalizationVersion`
  (for example `jcs-v2`), with a clear migration story, not an in-place change.
- The golden vectors are regenerated only when intentionally rotating the
  version, with `make golden`.

## Contributing rulesets

Baseline ruleset changes go in `classify/defs/baseline`. Mappings are suggested
evidence, never a compliance claim. Cite the control text your mapping refers
to in the pull request so reviewers can confirm it.

## Pull requests

- Keep changes focused. One concern per pull request.
- Add or update tests. New behavior without a test will be asked to add one.
- Make sure `make test`, `make vet`, and `make lint` pass.
- Fill out the pull request checklist.

## Commit messages

Use a short, imperative subject prefixed with the area, for example:

```
core: add control-character conformance vector
server: bound the export worker poll interval
```
