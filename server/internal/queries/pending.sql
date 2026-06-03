-- name: InsertPendingEvent :exec
-- Single-event accept path. Batch path uses pgx.CopyFrom directly.
INSERT INTO pending_events (
  id, organization_id, project_id, environment_id, shard_index,
  external_event_id, payload_hash, event_data, classification,
  control_mappings, scope_check, api_key_metadata_id, idempotency_key
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: DrainPendingForShard :many
-- Sealer drains in deterministic order so chain positions are reproducible.
-- Column order mirrors the pending_events model (scope_check last, as ALTER
-- appended it) so sqlc returns the PendingEvent struct, not a bespoke Row type.
SELECT
  id, organization_id, project_id, environment_id, shard_index,
  external_event_id, payload_hash, event_data, classification,
  control_mappings, api_key_metadata_id, idempotency_key, accepted_at, scope_check
FROM pending_events
WHERE environment_id = $1 AND shard_index = $2
ORDER BY accepted_at ASC, id ASC
LIMIT $3;

-- name: DeletePendingByIds :exec
DELETE FROM pending_events
WHERE id = ANY($1::uuid[]);

-- name: GetPendingEventForOrg :one
SELECT
  id, organization_id, project_id, environment_id, shard_index,
  external_event_id, payload_hash, event_data, classification,
  control_mappings, api_key_metadata_id, idempotency_key, accepted_at, scope_check
FROM pending_events
WHERE id = $1 AND organization_id = $2;

-- name: CountPendingByShard :one
SELECT COUNT(*)::bigint AS count FROM pending_events
WHERE environment_id = $1 AND shard_index = $2;

-- name: ListShardsWithPending :many
-- Used by the sealer to find shards that need draining. Distinct
-- (environment_id, shard_index) keeps the work-list compact.
SELECT DISTINCT environment_id, shard_index
FROM pending_events
LIMIT $1;
