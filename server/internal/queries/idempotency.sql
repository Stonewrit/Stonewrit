-- name: GetIdempotentResponse :one
SELECT response_body, status_code
FROM idempotency_keys
WHERE organization_id = $1 AND key = $2;

-- name: ClaimIdempotencyKey :one
-- Atomic idempotency gate. The FIRST request for (org, key) inserts and gets
-- the row back; concurrent racers hit the unique constraint, get DO NOTHING
-- (zero rows → pgx.ErrNoRows), and must then read the winner's cached
-- response. This is what makes 25 parallel same-key POSTs collapse to one
-- event instead of racing past a non-atomic SELECT.
INSERT INTO idempotency_keys (
  organization_id, project_id, environment_id, api_key_id,
  key, request_hash, response_body, status_code, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (organization_id, key) DO NOTHING
RETURNING id;

-- name: UpsertIdempotentResponse :exec
INSERT INTO idempotency_keys (
  organization_id, project_id, environment_id, api_key_id,
  key, request_hash, response_body, status_code, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (organization_id, key) DO UPDATE SET
  response_body = EXCLUDED.response_body,
  status_code = EXCLUDED.status_code,
  expires_at = EXCLUDED.expires_at;

-- name: DeleteExpiredIdempotencyKeys :exec
DELETE FROM idempotency_keys WHERE expires_at < NOW();
