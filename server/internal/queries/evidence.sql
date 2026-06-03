-- name: GetFramework :one
SELECT id, name, description, version
FROM frameworks
WHERE id = $1;

-- name: CountEventsInWindow :one
SELECT COUNT(*)::bigint AS count
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4;

-- name: CountPrivilegedEventsInWindow :one
SELECT COUNT(*)::bigint AS count
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4
  AND (
    event_type LIKE 'admin.%' OR
    event_type IN (
      'identity.permission.granted', 'identity.permission.revoked',
      'identity.role.changed', 'identity.mfa.enabled',
      'identity.mfa.disabled', 'identity.session.revoked'
    )
  );

-- name: CountAIAgentEventsInWindow :one
SELECT COUNT(*)::bigint AS count
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4
  AND (actor_type = 'ai_agent' OR event_type LIKE 'agent.%');

-- name: CountOutOfScopeAgentActionsInWindow :one
-- Agent actions whose observe-only scope_check resolved to out-of-scope
-- (in_scope = false), including not-registered agents. Surfaces unauthorized
-- agent behavior for the evidence summary.
SELECT COUNT(*)::bigint AS count
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4
  AND scope_check_result IS NOT NULL
  AND scope_check_result->>'in_scope' = 'false';

-- name: CountPolicyDenialsInWindow :one
SELECT COUNT(*)::bigint AS count
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4
  AND (action_result = 'denied' OR policy_decision = 'denied');

-- name: CountExternalSharesInWindow :one
SELECT COUNT(*)::bigint AS count
FROM events
WHERE organization_id = $1
  AND project_id = $2
  AND received_at >= $3
  AND received_at < $4
  AND event_type IN ('data.exported', 'data.shared_external', 'vendor.data_sent');

-- name: CountOpenExceptions :one
SELECT COUNT(*)::bigint AS count
FROM exceptions
WHERE organization_id = $1
  AND project_id = $2
  AND status = 'open';

-- name: GetEvidenceCountsPerControl :many
SELECT m.framework, m.control_id, COUNT(*)::bigint AS evidence_count
FROM event_control_mappings m
JOIN events e ON e.id = m.event_id
WHERE m.organization_id = $1
  AND e.project_id = $2
  AND m.framework = $3
  AND e.received_at >= $4
  AND e.received_at < $5
GROUP BY m.framework, m.control_id;

-- name: GetEventControlMappings :many
SELECT m.framework, m.control_id, m.mapping_reason, m.confidence, m.status
FROM event_control_mappings m
WHERE m.organization_id = $1
  AND m.event_id = $2;
