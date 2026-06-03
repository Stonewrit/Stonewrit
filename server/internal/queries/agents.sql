-- name: GetAgentByExternalID :one
-- Resolve an incoming actor.id to a registered agent within (org, project),
-- filtered by all three columns. Used at ingest to compute an observe-only
-- scope_check; it never mutates the registry. The agents table is empty by
-- default and optional, so a missing row is normal and never an error.
SELECT id, name, status, authorized_scopes
FROM agents
WHERE organization_id = $1
  AND project_id = $2
  AND external_id = $3;
