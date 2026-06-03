# Deploying the server

The server is a single static Go binary that talks to Postgres. It is stateless,
so you can run as many instances as you like behind a load balancer; Postgres is
the shared state.

## Build

A multi-stage Dockerfile is included and produces a small, non-root image with
the `server` and the `stonewrit` management binary:

```
docker build -t stonewrit/server .
```

## Configuration

The server reads configuration from the environment.

| Variable | Required | Default | Purpose |
|---|:---:|---|---|
| `DATABASE_URL` | yes | | Postgres connection string. |
| `PORT` | no | `3002` | Port the server binds to. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. |
| `DATABASE_POOL_MAX` | no | `30` | Max pooled database connections. |
| `APIKEY_AUTH` | no | `false` | When true, require an API key on every request. |

## Authentication

By default the server runs open: it provisions a built-in default tenant on boot
and accepts events with no key. That is convenient for local use and for running
behind your own gateway, but an open instance accepts events from anyone, so for
anything internet-facing set `APIKEY_AUTH=true` and mint keys with the CLI:

```
stonewrit key create --name ci      # prints a token once
stonewrit key list
stonewrit key revoke <id>
```

The server logs a prominent warning on boot whenever auth is disabled.

## Migrations

Schema changes are an explicit step, never run automatically on startup. Apply
them with the `stonewrit` CLI before or during a deploy:

```
stonewrit migrate up      # apply all pending migrations
stonewrit migrate status  # show applied and pending migrations
stonewrit migrate down    # roll back the most recent migration
```

In Docker, run `stonewrit migrate up` as a one-shot before the server starts.
The included `docker-compose.yml` shows this pattern: a `migrate` service runs to
completion, and the `server` waits for it.

## Health checks

| Path | Meaning |
|---|---|
| `/health` | The process is up. |
| `/ready` | The process is ready to serve. |
| `/dbping` | The database is reachable. |

Point your liveness probe at `/health` and your readiness probe at `/ready` or
`/dbping`.

## Running multiple instances

The server is safe to scale horizontally. The sealer that builds the chain takes
a per-shard row lock in Postgres, so instances coordinate through the database
with no external lock service. Rate limits and idempotency are enforced in
Postgres, so they hold across instances. See [SCALING.md](SCALING.md) for the
capacity model.
