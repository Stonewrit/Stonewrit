# Scaling

The server is stateless, so scaling is mostly about Postgres. This is the model.

## Mental model

Nothing that must be consistent across requests lives in process memory. The
accept path validates, hashes, classifies, and writes one row to
`pending_events`. The sealer drains pending rows into the chained `events`
journal under a per-shard row lock in Postgres, so multiple server instances
coordinate through the database with no external lock service. Idempotency is a
unique constraint in Postgres, so it holds across instances too.

That means you scale the server by adding instances behind a load balancer, and
the ceiling you reach as you grow is Postgres, not the Go process.

## What to turn

| Knob | Where | Notes |
|---|---|---|
| Instance count | your orchestrator | Horizontal scale. Stateless; no sticky sessions. |
| `DATABASE_POOL_MAX` | env | Pooled connections per instance. Default 30. |
| Database size | your Postgres | The real ceiling. The writes are the load. |

## Connection math

Total Postgres connections equal `instances x DATABASE_POOL_MAX`, which must
stay under your database's connection ceiling. A connection pooler in front of
Postgres multiplexes many client connections over few backend ones, which is
what makes wide horizontal scaling viable. Keep the pool modest per instance (20
to 50) and let the pooler absorb the fan-out.

## Postgres is the ceiling

Each accepted event does roughly one insert into `pending_events`, and the
sealer later does a batched `COPY` into `events` plus the classification and
control-mapping rows. The sealer batches per shard, so its cost amortizes well.
As you grow, watch Postgres in this order:

1. Connection pressure. Tune `DATABASE_POOL_MAX` and keep a pooler.
2. Write throughput. The event and mapping inserts are the dominant cost;
   increase the database's CPU and IO.
3. Table growth. Keep the events table indexed on the columns the evidence
   queries filter so reads stay cheap as the table grows into the millions, and
   partition or retain by time at high volume.

## Sharding

A chain is sealed per `(environment, shard_index)`. Sharding spreads sealing
work so multiple sealers run in parallel without contending on one chain row.
It is an internal write-throughput detail; auditors group chains by environment
and verify each chain's head, and the verifier never needs to know about shards.

## Rate limiting

The open server does not rate limit; that is a concern for the gateway or load
balancer you front it with (or the hosted product, which adds per-tenant
limits). If you expose the server directly, put request limits in your proxy.
