-- name: GetEventForOrg :one
SELECT * FROM events
WHERE id = $1 AND organization_id = $2;

-- name: GetEventByExternalId :one
SELECT * FROM events
WHERE environment_id = $1 AND external_event_id = $2;

-- name: ListEventsByProject :many
SELECT * FROM events
WHERE project_id = $1 AND organization_id = $2
ORDER BY received_at DESC, id DESC
LIMIT $3;

-- name: GetEventNeighbors :many
SELECT
  id, chain_position, event_hash, previous_event_hash, payload_hash,
  event_type, received_at
FROM events
WHERE chain_id = $1 AND chain_position = ANY($2::bigint[])
ORDER BY chain_position;

-- name: WalkChainForVerify :many
SELECT * FROM events
WHERE chain_id = $1
ORDER BY chain_position ASC
LIMIT $2;

-- name: GetLastEventInChain :one
SELECT id, chain_position, event_hash, payload_hash
FROM events
WHERE chain_id = $1
ORDER BY chain_position DESC
LIMIT 1;

-- name: CountEventsByEnvironment :one
SELECT COUNT(*)::bigint AS count FROM events
WHERE environment_id = $1 AND organization_id = $2;

-- name: CountEventsThisMonth :one
SELECT COUNT(*)::bigint AS count FROM events
WHERE organization_id = $1
  AND received_at >= date_trunc('month', NOW());

-- name: ExistingExternalEventIDs :many
-- Used by the sealer to skip pending rows whose external_event_id already
-- exists in the journal for this environment. Without this filter, a
-- duplicate row would fail the entire batch INSERT and the shard would
-- be stuck retrying forever.
SELECT external_event_id
FROM events
WHERE environment_id = $1
  AND external_event_id = ANY($2::text[]);

-- name: InsertEventClassifications :exec
INSERT INTO event_classifications (
  organization_id, event_id, data_classes, risk_level, confidence, method
)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertEventControlMapping :exec
INSERT INTO event_control_mappings (
  organization_id, event_id, framework, control_id, mapping_reason, confidence, mapped_by, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
