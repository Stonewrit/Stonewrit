-- name: ClaimPendingExport :one
-- Picks the next processing export and locks it for the worker. Uses
-- SKIP LOCKED so multiple workers can drain in parallel.
SELECT id, organization_id, project_id, environment_id, framework,
       period_from, period_to, controls, include_raw_events, include_hash_proofs
FROM evidence_exports
WHERE status = 'processing'
ORDER BY created_at ASC
FOR UPDATE SKIP LOCKED
LIMIT 1;

-- name: ListEventsForExport :many
SELECT id, event_type, occurred_at, received_at, actor_type, actor_id,
       action_name, action_result, resource_type, resource_id,
       chain_id, chain_position, payload_hash, event_hash,
       data_classes
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4
ORDER BY chain_id, chain_position;

-- name: ListChainSummariesForOrg :many
SELECT id, environment_id, shard_index, name, status, latest_position, latest_event_hash
FROM chains
WHERE organization_id = $1
  AND project_id = $2;

-- name: MarkExportCompleted :exec
UPDATE evidence_exports
SET status = 'completed',
    bundle = $2,
    bundle_size = $3,
    bundle_hash = $4,
    bundle_signed_at = NOW(),
    event_count = $5,
    completed_at = NOW()
WHERE id = $1;

-- name: MarkExportFailed :exec
UPDATE evidence_exports
SET status = 'failed',
    error_message = $2,
    completed_at = NOW()
WHERE id = $1;
