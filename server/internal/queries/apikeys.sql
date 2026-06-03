-- name: LookupApiKeyByHash :one
-- Resolve a presented bearer token (already hashed by the caller) to its scope.
-- Revoked keys never match.
SELECT id, organization_id, project_id, environment_id, scopes
FROM api_keys
WHERE key_hash = $1
  AND revoked_at IS NULL;

-- name: InsertApiKey :one
INSERT INTO api_keys (key_hash, name, organization_id, project_id, environment_id, scopes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at;

-- name: ListApiKeys :many
SELECT id, name, organization_id, project_id, environment_id, scopes, created_at, revoked_at
FROM api_keys
ORDER BY created_at DESC;

-- name: RevokeApiKey :exec
UPDATE api_keys
SET revoked_at = now()
WHERE id = $1
  AND revoked_at IS NULL;
