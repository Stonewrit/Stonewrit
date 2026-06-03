# Scaling

How the server scales as traffic grows: what to turn, in what order, and how it
interacts with rate limiting. Read the mental model first; it changes what
scaling even means here.

## Mental model

The server is stateless. Everything that must be consistent across requests
lives in Postgres, not in process memory:

- Rate-limit counters are an atomic `INSERT ... ON CONFLICT DO UPDATE` in
  Postgres, keyed by `(api_key_metadata_id, bucket, window_start)`. Window
  boundaries are floored so every instance agrees on the same bucket without
  coordination.
- Event counts, tier, and spend cap are read from Postgres.

The only in-process state is a few short-lived caches that exist purely to spare
Postgres reads; they are allowed to be slightly stale and never need to be
shared. So you scale the server by adding instances, and the ceiling you reach as
you grow is Postgres, not the Go process.

## Rate limits and horizontal scaling

A naive rate limiter keeps counters in memory, so running N instances gives each
customer N times their intended limit. This one does not: because the counters
are centralized in Postgres, a key's limit is enforced correctly no matter how
many instances run. Adding instances is safe from a rate-limit perspective, and
you do not need to change anything in the limiter when you scale out.

The cost of that correctness is two database writes per request (a burst and a
sustained increment). That write load grows with traffic, which is why Postgres
is the real ceiling.

## What is tunable

| Knob | Where | Env? | Notes |
|---|---|:---:|---|
| Database pool size per instance | `DATABASE_POOL_MAX` | yes | Default 30. The most important dial. |
| Instance count | your orchestrator | yes | Horizontal scale. Safe (see above). |
| `PORT`, `LOG_LEVEL` | env | yes | Operational. |
| Per-tier rate ceilings | `TierLimits` in `server/internal/ratelimit/limiter.go` | no | A code change today. See below. |
| Included events and overage rates | `STARTER_INCLUDED_EVENTS` and related | yes | Billing, not throughput. |

## Connection math

Total Postgres connections equal `instances x DATABASE_POOL_MAX`, which must
stay under your database's connection ceiling. A connection pooler in front of
Postgres multiplexes many client connections over few backend ones, which is
what makes wide horizontal scaling viable. Keep the pool modest per instance
(20 to 50) and let the pooler absorb the fan-out.

## Raising a tier's request limits

The ceilings live in `server/internal/ratelimit/limiter.go`. Limits are per API
key, per tier (the bucket is keyed by `api_key_metadata_id`), so adding
customers never shrinks anyone else's limit, and a customer can self-scale
throughput by issuing more keys. To raise a tier today, edit `TierLimits` and
redeploy.

When you outgrow redeploys, make `TierLimits` env-driven the same way the billing
tunables are read in `server/internal/config/env.go`, then a tier can be raised
from configuration without a build.

## Postgres is the real ceiling

Each accepted event does roughly: a cached tier resolve, two rate-limit upserts,
a cached month-count read, a cached spend-cap read, and the event write. The
caches keep reads cheap; the writes scale linearly with volume. As you grow,
watch Postgres in this order:

1. Connection pressure. Tune `DATABASE_POOL_MAX` and keep a pooler.
2. Compute. The bucket upserts and event inserts are write-heavy; increase the
   database's CPU and IO.
3. Bucket table growth. Old rate-limit windows are pruned by the rate-limit
   cleanup worker; make sure it is running.
4. Count cost. Keep the events table indexed on the columns the counts filter so
   they stay cheap as the table grows into the millions.

## Staged playbook

| Stage | Traffic | Do this |
|---|---|---|
| Launch | low | One instance, default pool, base database size, pooler on. |
| Growth | steady climb | Increase database compute first. Confirm the cleanup worker runs. Verify the events-table indexes. |
| Scale out | spiky or high RPS | Add instances. Keep `instances x DATABASE_POOL_MAX` sane behind the pooler. Rate limits stay correct automatically. |
| High volume | sustained heavy | Make `TierLimits` env-driven. Consider moving rate-limit counters and month counts to a cache such as Redis to cut write load, add read replicas for cached reads, and partition or retain the events table by month. |
