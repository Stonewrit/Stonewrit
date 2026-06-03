-- name: IncrementRateLimitBucket :one
-- Atomic counter increment via INSERT ... ON CONFLICT.
-- Returns the post-increment count so the caller can decide 429 vs allow.
INSERT INTO rate_limit_buckets (api_key_metadata_id, bucket, window_start, count)
VALUES ($1, $2, $3, 1)
ON CONFLICT (api_key_metadata_id, bucket, window_start)
DO UPDATE SET
  count = rate_limit_buckets.count + 1,
  updated_at = NOW()
RETURNING count;

-- name: ResolveOrgTier :many
-- Entitled subscriptions for an org. Caller picks the highest-tier plan.
-- past_due is intentionally EXCLUDED: a failed payment cuts off ingest
-- immediately (deny → 402), matching the read-only state the web app shows.
-- Only active/trialing are entitled, everywhere (Go, cron, TS gate).
SELECT plan, status
FROM subscription
WHERE reference_id = $1
  AND status IN ('active', 'trialing');

-- name: DeleteStaleRateLimitBuckets :exec
DELETE FROM rate_limit_buckets
WHERE window_start < $1;
