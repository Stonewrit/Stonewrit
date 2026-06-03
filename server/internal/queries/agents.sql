-- name: GetAgentByExternalID :one
-- Resolve an incoming actor.id to a registered agent WITHIN (org, project).
-- ALWAYS filtered by all three columns - the (org, project, external_id) unique
-- index is also the cross-org lookup guard. Used by the Go ingest path to
-- compute an observe-only scope_check; this never mutates the registry.
--
-- Open-core note: agent management (CRUD) lives in the proprietary dashboard;
-- this read is the only registry touch in the Go service, mirroring how it
-- reads api_key_metadata. At the open-core split it sits behind the
-- scope_check resolve seam, so the open server can omit it entirely.
SELECT id, name, status, authorized_scopes
FROM agents
WHERE organization_id = $1
  AND project_id = $2
  AND external_id = $3;
