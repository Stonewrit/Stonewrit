-- name: LookupApiKeyByHash :one
-- One round-trip: Better Auth key + Stonewrit metadata + env slug.
-- Caller still validates enabled/expires/revoked at the Go layer.
SELECT
  ak.id              AS better_auth_key_id,
  ak.enabled         AS bak_enabled,
  ak.expires_at      AS bak_expires_at,
  m.id               AS api_key_metadata_id,
  m.organization_id  AS organization_id,
  m.project_id       AS project_id,
  m.environment_id   AS environment_id,
  m.scopes           AS scopes,
  m.revoked_at       AS metadata_revoked_at,
  m.expires_at       AS metadata_expires_at,
  e.slug             AS environment_slug
FROM apikey ak
JOIN api_key_metadata m ON m.better_auth_key_id = ak.id
LEFT JOIN environments e ON e.id = m.environment_id
WHERE ak.key = $1
  -- Reject keys bound to a soft-deleted environment (the project/env was
  -- deleted): no row → caller treats it as an invalid key (401). Ingest into a
  -- deleted environment must not be accepted.
  AND EXISTS (
    SELECT 1 FROM environments ed
    WHERE ed.id = m.environment_id AND ed.deleted_at IS NULL
  );

-- name: TouchApiKeyLastUsed :exec
UPDATE api_key_metadata
SET last_used_at = NOW(), updated_at = NOW()
WHERE id = $1;
