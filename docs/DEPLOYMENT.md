# Deploying the server

The server is a single static Go binary that talks to Postgres. It is stateless,
so you can run as many instances as you like behind a load balancer; Postgres is
the shared state.

## Build

A multi-stage Dockerfile is included and produces a small, non-root image with
the server and the migrate binary:

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

Metered-overage tunables (`STARTER_INCLUDED_EVENTS`, `OVERAGE_UNIT_EVENTS`, and
related) are optional and documented in `.env.example`.

## Migrations

Schema changes are an explicit step, never run automatically on startup. Apply
them with the migrate binary before or during a deploy:

```
migrate up      # apply all pending migrations
migrate status  # show applied and pending migrations
migrate down    # roll back the most recent migration
```

In Docker, run the `migrate` binary as a one-shot before the server starts. The
included `docker-compose.yml` shows this pattern: a `migrate` service runs to
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
