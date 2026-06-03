-- name: GetOrgBillingState :one
-- Read the org's overage spend-cap settings + the current period anchor (set
-- by the Node usage cron, Stripe-aligned). Returns no rows when the org has
-- never configured a limit / been metered (caller falls back to uncapped +
-- calendar-month period).
SELECT overage_limit_cents, overage_limit_disabled, period_start
FROM org_billing_state
WHERE organization_id = $1;

-- name: BumpUsageCounter :one
-- Atomic per-(org, period) accepted-event tally used for EXACT cross-pod spend
-- cap enforcement (the per-pod cache could overshoot). Returns the running
-- total after adding $3. Append-only except a single negative delta to roll
-- back one rejected over-cap accept.
INSERT INTO org_usage_counters (organization_id, period_start, accepted_count)
VALUES ($1, $2, $3)
ON CONFLICT (organization_id, period_start)
DO UPDATE SET accepted_count = org_usage_counters.accepted_count + $3,
              updated_at = NOW()
RETURNING accepted_count;
