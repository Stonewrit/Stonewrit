package testkit

import (
	mrand "math/rand/v2"
	"strings"
)

// Builder produces one event payload.
type Builder func() map[string]any

// CompositeBuilder produces a sequence of events that flow together.
type CompositeBuilder func() []map[string]any

// All occurred_at fields below use NowISO so events land "now" in the
// dashboard - useful for visually verifying ingest, race, chain, and UI.

// InvalidScenarios - handcrafted 422 cases.
var InvalidScenarios = map[string]Builder{
	"missing-event-type": func() map[string]any {
		return map[string]any{
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"bad-occurred-at": func() map[string]any {
		return map[string]any{
			"event_type":  "identity.login.succeeded",
			"occurred_at": "last tuesday",
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"event-type-too-long": func() map[string]any {
		return map[string]any{
			"event_type":  strings.Repeat("a", 200),
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"missing-actor": func() map[string]any {
		return map[string]any{
			"event_type":  "admin.something",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"missing-action": func() map[string]any {
		return map[string]any{
			"event_type":  "identity.login.succeeded",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"missing-resource": func() map[string]any {
		return map[string]any{
			"event_type":  "identity.login.succeeded",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
		}
	},
	"bad-email": func() map[string]any {
		return map[string]any{
			"event_type":  "identity.login.succeeded",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human", "email": "not-an-email"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"event-type-empty": func() map[string]any {
		return map[string]any{
			"event_type":  "",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"missing-source-system": func() map[string]any {
		return map[string]any{
			"event_type":  "identity.login.succeeded",
			"occurred_at": NowISO(),
			"source":      map[string]any{"service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"missing-action-result": func() map[string]any {
		return map[string]any{
			"event_type":  "identity.login.succeeded",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"classification-not-array": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "customer", "classification": "pii"},
		}
	},
	"classification-too-many": func() map[string]any {
		cls := make([]string, 30)
		for i := range cls {
			cls[i] = "cls_" + RandomID()
		}
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "customer", "classification": cls},
		}
	},
	"empty-payload": func() map[string]any { return map[string]any{} },
	"event-type-as-number": func() map[string]any {
		return map[string]any{
			"event_type":  12345,
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"event-type-as-null": func() map[string]any {
		return map[string]any{
			"event_type":  nil,
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
}

// AdversarialScenarios - "try to break it" payloads.
var AdversarialScenarios = map[string]Builder{
	"sql-injection": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "'; DROP TABLE events; --", "service": "x"},
			"actor":       map[string]any{"type": "human", "id": "1' OR '1'='1", "email": "alice@example.com"},
			"action":      map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"xss": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source": map[string]any{
				"system":  "<script>alert(1)</script>",
				"service": "<img src=x onerror=alert(1)>",
			},
			"actor":    map[string]any{"type": "human"},
			"action":   map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource": map[string]any{"type": "r"},
		}
	},
	"max-length-stuffing": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source": map[string]any{
				"system":  strings.Repeat("a", 119),
				"service": strings.Repeat("b", 119),
			},
			"actor": map[string]any{
				"type":         "human",
				"id":           strings.Repeat("a", 199),
				"display_name": strings.Repeat("a", 199),
			},
			"action": map[string]any{
				"name":     strings.Repeat("a", 119),
				"category": strings.Repeat("b", 59),
				"result":   "allowed",
				"reason":   strings.Repeat("r", 499),
			},
			"resource": map[string]any{"type": strings.Repeat("a", 119)},
		}
	},
	"unicode-soup": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "🔥🚀\u202E", "service": "x‍‍‍"},
			"actor":       map[string]any{"type": "human", "display_name": "🦄💀☣️"},
			"action":      map[string]any{"name": "do. .thing", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "🗃️"},
		}
	},
	"deeply-nested-metadata": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
			"metadata":    nestedObject(20),
		}
	},
	"weird-metadata-values": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
			"metadata": map[string]any{
				"empty":    "",
				"ws":       "   ",
				"nullval":  nil,
				"zero":     0,
				"bool":     false,
				"arr":      []any{},
				"obj":      map[string]any{},
				"long_str": strings.Repeat("x", 2000),
			},
		}
	},
	"future-occurred-at": func() map[string]any {
		// Per user request: use now() instead of a far-future date so the event
		// lands in the dashboard timeline like normal traffic.
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"path-traversal-ids": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human", "id": "../../../../etc/passwd"},
			"action":      map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "r", "id": "..\\..\\windows\\system32"},
		}
	},
	"control-chars-in-fields": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s\n\r\t", "service": "x"},
			"actor":       map[string]any{"type": "human", "display_name": "line1\nline2\nline3"},
			"action":      map[string]any{"name": "a", "category": "data_access", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
}

// BoundaryScenarios - exactly-at-limit values, should ALL succeed.
// Per user request: all date scenarios now use NowISO so they're visually
// recognizable in the dashboard as fresh test events.
var BoundaryScenarios = map[string]Builder{
	"event-type-at-max-120-chars": func() map[string]any {
		return map[string]any{
			"event_type":  strings.Repeat("a", 120),
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"action-name-at-max-120-chars": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": strings.Repeat("a", 120), "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"actor-id-at-max-200-chars": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human", "id": strings.Repeat("a", 200)},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"metadata-with-50-fields": func() map[string]any {
		meta := make(map[string]any, 50)
		for i := 0; i < 50; i++ {
			meta["k"+itoa(i)] = "v" + itoa(i)
		}
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
			"metadata":    meta,
		}
	},
	"classification-at-max-20": func() map[string]any {
		cls := make([]string, 20)
		for i := range cls {
			cls[i] = "cls_" + itoa(i)
		}
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r", "classification": cls},
		}
	},
	"fields-accessed-at-max-50": func() map[string]any {
		fa := make([]string, 50)
		for i := range fa {
			fa[i] = "f" + itoa(i)
		}
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r", "fields_accessed": fa},
		}
	},
	"occurred-at-now-1": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"occurred-at-now-2": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
	"occurred-at-now-3": func() map[string]any {
		return map[string]any{
			"event_type":  "data.accessed",
			"occurred_at": NowISO(),
			"source":      map[string]any{"system": "s", "service": "x"},
			"actor":       map[string]any{"type": "human"},
			"action":      map[string]any{"name": "a", "category": "b", "result": "allowed"},
			"resource":    map[string]any{"type": "r"},
		}
	},
}

// CompositeScenarios - multi-event flows that hit several services + actors.
// All timestamps are NowISO so the flow appears in the dashboard timeline.
var CompositeScenarios = map[string]CompositeBuilder{
	"data-export-flow": func() []map[string]any {
		analystEmail := Pick(HumanFirstNames) + "@" + Pick(EmailDomains)
		analystID := "usr_" + RandomID()
		customerHash := "sha256:" + RandomHex(16)
		tenantID := "tnt_" + RandomID()
		human := map[string]any{"type": "human", "id": analystID, "email": analystEmail, "role": "analyst"}
		return []map[string]any{
			GenerateEvent(GenerateOpts{
				EventType: "identity.login.succeeded",
				ActorType: ActorHuman, Result: ResultAllowed,
				OccurredAt: NowISO(),
			}),
			GenerateEvent(GenerateOpts{
				EventType: "identity.permission.check",
				ActorType: ActorHuman, Result: ResultAllowed,
				OccurredAt: NowISO(),
			}),
			{
				"event_type":  "data.accessed",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "production-api", "service": "crm", "environment": "prod"},
				"actor":       human,
				"action":      map[string]any{"name": "customer.read", "category": "data_access", "result": "allowed"},
				"resource": map[string]any{
					"type":            "customer",
					"id_hash":         customerHash,
					"tenant_id":       tenantID,
					"classification":  []string{"pii", "customer_data"},
					"fields_accessed": PickN(FieldsAccessed, 3),
				},
			},
			{
				"event_type":  "data.exported",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "production-api", "service": "export-service", "environment": "prod"},
				"actor":       human,
				"action":      map[string]any{"name": "data.export", "category": "data_modification", "result": "allowed"},
				"resource": map[string]any{
					"type":            "report",
					"id":              "rep_" + RandomID(),
					"tenant_id":       tenantID,
					"classification":  []string{"pii", "financial"},
					"fields_accessed": []string{"email", "last_4_card"},
				},
			},
			{
				"event_type":  "vendor.data_sent",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "billing", "service": "stripe-sync", "environment": "prod"},
				"actor":       map[string]any{"type": "service_account", "id": "sa_" + RandomID(), "role": "sync"},
				"action":      map[string]any{"name": "stripe.customer.sync", "category": "vendor", "result": "allowed"},
				"resource": map[string]any{
					"type":           "customer",
					"id_hash":        customerHash,
					"tenant_id":      tenantID,
					"classification": []string{"pii", "financial"},
				},
			},
		}
	},
	"ai-agent-refund-flow": func() []map[string]any {
		agent := BuildActor(ActorAIAgent)
		reviewer := BuildActor(ActorHuman)
		refundID := "ref_" + RandomID()
		approvalID := "apr_" + RandomID()
		return []map[string]any{
			{
				"event_type":  "agent.tool_called",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "ai-platform", "service": "triage-bot", "environment": "prod"},
				"actor":       agent,
				"action":      map[string]any{"name": "sql.query", "category": "agent", "result": "allowed"},
				"resource":    map[string]any{"type": "db.refund_eligibility", "classification": []string{"customer_data", "financial"}},
			},
			{
				"event_type":  "agent.action_proposed",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "ai-platform", "service": "triage-bot", "environment": "prod"},
				"actor":       agent,
				"action":      map[string]any{"name": "refund.propose", "category": "agent", "result": "pending"},
				"resource":    map[string]any{"type": "refund", "id": refundID},
				"policy":      map[string]any{"approval_required": true},
			},
			{
				"event_type":  "agent.human_review_requested",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "ai-platform", "service": "triage-bot", "environment": "prod"},
				"actor":       agent,
				"action":      map[string]any{"name": "request.escalation", "category": "agent", "result": "pending", "reason": "over_threshold"},
				"resource":    map[string]any{"type": "refund", "id": refundID},
			},
			{
				"event_type":  "admin.refund_approved",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "platform", "service": "support-tools", "environment": "prod"},
				"actor":       reviewer,
				"action":      map[string]any{"name": "refund.approve", "category": "admin", "result": "allowed"},
				"resource":    map[string]any{"type": "refund", "id": refundID},
				"policy":      map[string]any{"approval_id": approvalID},
			},
			{
				"event_type":  "agent.action_executed",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "ai-platform", "service": "triage-bot", "environment": "prod"},
				"actor":       agent,
				"action":      map[string]any{"name": "refund.execute", "category": "agent", "result": "allowed"},
				"resource":    map[string]any{"type": "refund", "id": refundID},
				"policy":      map[string]any{"approval_id": approvalID},
			},
		}
	},
	"brute-force-login-storm": func() []map[string]any {
		attackerIP := RandomIP()
		targetUser := "usr_" + RandomID()
		var events []map[string]any
		for i := 0; i < 8; i++ {
			events = append(events, map[string]any{
				"event_type":  "identity.login.failed",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "auth", "service": "web", "environment": "prod"},
				"actor":       map[string]any{"type": "human", "ip": attackerIP},
				"action":      map[string]any{"name": "login", "category": "identity", "result": "denied", "reason": "bad_password"},
				"resource":    map[string]any{"type": "user", "id": targetUser},
			})
		}
		events = append(events,
			map[string]any{
				"event_type":  "security.suspicious_activity",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "edge", "service": "detector", "environment": "prod"},
				"actor":       map[string]any{"type": "system", "id": "detector"},
				"action":      map[string]any{"name": "threat.flagged", "category": "security", "result": "denied", "reason": "multiple_failed_logins_same_ip"},
				"resource":    map[string]any{"type": "user", "id": targetUser},
			},
			map[string]any{
				"event_type":  "security.rate_limit_breached",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "edge", "service": "rate-limiter", "environment": "prod"},
				"actor":       map[string]any{"type": "system", "id": "edge", "ip": attackerIP},
				"action":      map[string]any{"name": "block", "category": "security", "result": "denied"},
				"resource":    map[string]any{"type": "ip", "id": attackerIP},
			},
		)
		return events
	},
	"admin-prod-access-grant": func() []map[string]any {
		requester := BuildActor(ActorHuman)
		approver := BuildActor(ActorHuman)
		approvalID := "apr_" + RandomID()
		serviceID := "svc_" + RandomID()
		return []map[string]any{
			{
				"event_type":  "admin.production_access_requested",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "platform", "service": "access-control", "environment": "prod"},
				"actor":       requester,
				"action":      map[string]any{"name": "access.request", "category": "admin", "result": "pending", "reason": "incident_response"},
				"resource":    map[string]any{"type": "service", "id": serviceID},
				"policy":      map[string]any{"approval_required": true},
			},
			{
				"event_type":  "admin.approval_granted",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "platform", "service": "access-control", "environment": "prod"},
				"actor":       approver,
				"action":      map[string]any{"name": "access.approve", "category": "admin", "result": "allowed"},
				"resource":    map[string]any{"type": "access_request", "id": approvalID},
			},
			{
				"event_type":  "admin.production_access_granted",
				"occurred_at": NowISO(),
				"source":      map[string]any{"system": "platform", "service": "access-control", "environment": "prod"},
				"actor":       requester,
				"action":      map[string]any{"name": "access.grant", "category": "admin", "result": "allowed"},
				"resource":    map[string]any{"type": "service", "id": serviceID, "policy_id": "pol_prod_access_v3"},
				"policy":      map[string]any{"approval_id": approvalID, "policy_id": "pol_prod_access_v3"},
			},
		}
	},
}

// nestedObject builds a recursively-nested chain n levels deep.
func nestedObject(depth int) map[string]any {
	root := map[string]any{}
	cur := root
	for i := 0; i < depth; i++ {
		next := map[string]any{"level": i}
		cur["nested"] = next
		cur = next
	}
	return root
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// Suppress unused import warning if mrand isn't used yet in some build flag.
var _ = mrand.IntN
