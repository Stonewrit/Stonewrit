-- name: EnsureShardChainFromEnv :exec
-- Idempotent shard creation. Pulls organization_id + project_id from the
-- environments table. ON CONFLICT DO NOTHING so it takes NO row lock when the
-- chain already exists - that lets a busy shard's chain row be claimed via
-- LockShardChain's FOR UPDATE SKIP LOCKED instead of blocking other sealers.
INSERT INTO chains (organization_id, project_id, environment_id, shard_index, name)
SELECT e.organization_id, e.project_id, e.id, $2::int, 'shard-' || $2::text
FROM environments e
WHERE e.id = $1
ON CONFLICT (environment_id, shard_index) DO NOTHING;

-- name: LockShardChain :one
-- SKIP LOCKED: if another sealer already holds this shard's chain row, return
-- no row so the caller skips the shard this tick instead of blocking on it.
-- Carries project_id/organization_id so the sealer doesn't need a second read.
SELECT id, organization_id, project_id, latest_position, latest_event_hash, status
FROM chains
WHERE environment_id = $1 AND shard_index = $2
FOR UPDATE SKIP LOCKED;

-- name: UpdateShardTip :exec
UPDATE chains
SET latest_position = $2,
    latest_event_hash = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: InsertChainCheckpoint :exec
-- Append-only anchor: records the chain head (root_hash = the tip event_hash,
-- which cryptographically commits to every prior event) at each seal batch, so
-- a third party / external anchor has a dangling proof the chain reached this
-- state. anchoring_type/status default to internal/completed.
INSERT INTO chain_checkpoints (
  organization_id, project_id, environment_id, chain_id,
  from_position, to_position, root_hash, event_count
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListShardsForEnvironment :many
SELECT id, shard_index, latest_position, latest_event_hash, status, updated_at
FROM chains
WHERE environment_id = $1
ORDER BY shard_index;

-- name: ListChainsForOrg :many
SELECT
  id, organization_id, project_id, environment_id, shard_index,
  name, status, latest_position, latest_event_hash, created_at, updated_at
FROM chains
WHERE organization_id = $1
  AND (sqlc.narg('project_id')::uuid IS NULL OR project_id = sqlc.narg('project_id')::uuid)
ORDER BY updated_at DESC;

-- name: GetChainForOrg :one
SELECT
  id, organization_id, project_id, environment_id, shard_index,
  name, status, latest_position, latest_event_hash, created_at, updated_at
FROM chains
WHERE id = $1 AND organization_id = $2;
