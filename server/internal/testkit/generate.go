package testkit

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand/v2"
	"strings"
	"time"
)

type GenerateOpts struct {
	Category   Category
	EventType  string
	ActorType  ActorType
	Result     ActionResult
	WithPolicy *bool
	WithReq    *bool
	WithMeta   *bool
	OccurredAt string
}

// GenerateEvent composes a fresh valid event from the banks. Mirrors
// the TS generateEvent() - same defaults, same probabilities.
func GenerateEvent(opts GenerateOpts) map[string]any {
	cat := opts.Category
	if cat == "" {
		if inferred := inferCategoryFromEventType(opts.EventType); inferred != "" {
			cat = inferred
		} else {
			cat = Pick(Categories)
		}
	}
	eventType := opts.EventType
	if eventType == "" {
		eventType = Pick(EventTypes[cat])
	}

	actorType := opts.ActorType
	if actorType == "" {
		actorType = defaultActorTypeFor(cat, eventType)
	}
	actor := BuildActor(actorType)

	result := opts.Result
	if result == "" {
		result = Pick(ActionResults)
	}
	action := map[string]any{
		"name":     Pick(ActionNames[cat]),
		"category": string(cat),
		"result":   string(result),
	}
	if result == ResultDenied || result == ResultError {
		action["reason"] = Pick(ActionReasons)
	}

	resourceType := Pick(ResourceTypes)
	resource := map[string]any{"type": resourceType}
	if mrand.Float64() < 0.5 {
		resource["id"] = resourceType[:min(3, len(resourceType))] + "_" + RandomID()
	}
	if mrand.Float64() < 0.4 {
		resource["id_hash"] = "sha256:" + RandomHex(16)
	}
	if mrand.Float64() < 0.5 {
		resource["tenant_id"] = "tnt_" + RandomID()
	}
	if mrand.Float64() < 0.5 {
		resource["classification"] = PickN(Classifications, 1+mrand.IntN(3))
	}
	if mrand.Float64() < 0.3 {
		resource["fields_accessed"] = PickN(FieldsAccessed, 1+mrand.IntN(5))
	}

	occurredAt := opts.OccurredAt
	if occurredAt == "" {
		occurredAt = NowISO()
	}

	event := map[string]any{
		"event_type":  eventType,
		"occurred_at": occurredAt,
		"source":      BuildSource(),
		"actor":       actor,
		"action":      action,
		"resource":    resource,
	}

	if mrand.Float64() < 0.6 {
		event["external_event_id"] = "ext_" + RandomID()
	}

	if shouldInclude(opts.WithPolicy, 0.3) {
		event["policy"] = BuildPolicy(result)
	}
	if shouldInclude(opts.WithReq, 0.25) {
		event["request"] = map[string]any{
			"request_id": "req_" + RandomID(),
			"trace_id":   "trc_" + RandomID(),
		}
	}
	if shouldInclude(opts.WithMeta, 0.4) {
		event["metadata"] = BuildMetadata()
	}

	return event
}

func shouldInclude(opt *bool, defaultProb float64) bool {
	if opt != nil {
		return *opt
	}
	return mrand.Float64() < defaultProb
}

func BuildActor(t ActorType) map[string]any {
	switch t {
	case ActorHuman:
		a := map[string]any{
			"type":  "human",
			"id":    "usr_" + RandomID(),
			"email": Pick(HumanFirstNames) + "@" + Pick(EmailDomains),
			"role":  Pick(Roles),
		}
		if mrand.Float64() < 0.6 {
			a["ip"] = RandomIP()
		}
		return a
	case ActorAIAgent:
		return map[string]any{
			"type":                "ai_agent",
			"id":                  "agt_" + RandomID(),
			"display_name":        Pick(AIAgentNames),
			"human_supervisor_id": "usr_" + RandomID(),
		}
	case ActorSystem:
		return map[string]any{
			"type":         "system",
			"id":           "svc_" + RandomID(),
			"display_name": Pick(SystemNames),
		}
	case ActorServiceAccount:
		return map[string]any{
			"type": "service_account",
			"id":   "sa_" + RandomID(),
			"role": Pick([]string{"integration", "sync", "mirror", "replicator"}),
		}
	case ActorExternalVendor:
		return map[string]any{
			"type":         "external_vendor",
			"id":           "vnd_" + RandomID(),
			"display_name": Pick(VendorNames),
		}
	}
	return map[string]any{"type": string(t), "id": "act_" + RandomID()}
}

func defaultActorTypeFor(cat Category, eventType string) ActorType {
	if strings.HasPrefix(eventType, "agent.") || cat == CatAgent {
		return ActorAIAgent
	}
	if cat == CatSystem || cat == CatCompliance || cat == CatSecurity {
		if mrand.Float64() < 0.6 {
			return ActorSystem
		}
		return ActorServiceAccount
	}
	if cat == CatVendor {
		if mrand.Float64() < 0.5 {
			return ActorExternalVendor
		}
		return ActorServiceAccount
	}
	return ActorHuman
}

func BuildSource() map[string]any {
	s := map[string]any{
		"system":      Pick(SourceSystems),
		"service":     Pick(SourceServices),
		"environment": Pick(Environments),
	}
	if mrand.Float64() < 0.4 {
		s["region"] = Pick(Regions)
	}
	if mrand.Float64() < 0.3 {
		s["version"] = fmt.Sprintf("v%d.%d.%d",
			1+mrand.IntN(5), mrand.IntN(20), mrand.IntN(50))
	}
	return s
}

func BuildPolicy(result ActionResult) map[string]any {
	requiresApproval := mrand.Float64() < 0.4
	decision := "allow"
	if result == ResultDenied {
		decision = "deny"
	}
	p := map[string]any{
		"decision":          decision,
		"policy_id":         Pick(PolicyIDs),
		"approval_required": requiresApproval,
	}
	if requiresApproval && mrand.Float64() < 0.7 {
		p["approval_id"] = "apr_" + RandomID()
	}
	if mrand.Float64() < 0.3 {
		ctl := []string{"CC6.1", "CC7.2", "CC8.1", "Art. 30"}
		p["control_ids"] = PickN(ctl, 1+mrand.IntN(2))
	}
	return p
}

func BuildMetadata() map[string]any {
	type kv struct {
		k string
		v any
	}
	bank := []kv{
		{"client_version", fmt.Sprintf("%d.%d.%d", 1+mrand.IntN(5), mrand.IntN(20), mrand.IntN(50))},
		{"user_agent", Pick([]string{"Mozilla/5.0", "curl/8.5", "PostmanRuntime/7.39", "Python/3.11 requests"})},
		{"duration_ms", mrand.IntN(5000)},
		{"retry_count", mrand.IntN(4)},
		{"risk_score", fmt.Sprintf("%.2f", mrand.Float64())},
		{"locale", Pick([]string{"en-US", "en-GB", "de-DE", "fr-FR", "ja-JP"})},
		{"feature_flags", PickN([]string{"flag-a", "flag-b", "flag-c", "flag-d"}, mrand.IntN(3))},
		{"ab_variant", Pick([]string{"control", "treatment-a", "treatment-b"})},
		{"campaign_id", "cmp_" + RandomID()},
	}
	mrand.Shuffle(len(bank), func(i, j int) { bank[i], bank[j] = bank[j], bank[i] })
	n := 2 + mrand.IntN(4)
	if n > len(bank) {
		n = len(bank)
	}
	out := make(map[string]any, n)
	for _, e := range bank[:n] {
		out[e.k] = e.v
	}
	return out
}

func inferCategoryFromEventType(eventType string) Category {
	if eventType == "" {
		return ""
	}
	for cat, types := range EventTypes {
		for _, t := range types {
			if t == eventType {
				return cat
			}
		}
	}
	prefix := strings.SplitN(eventType, ".", 2)[0]
	switch prefix {
	case "identity":
		return CatIdentity
	case "admin":
		return CatAdmin
	case "data":
		return CatDataAccess
	case "agent":
		return CatAgent
	case "control", "evidence", "exception":
		return CatCompliance
	case "vendor":
		return CatVendor
	case "security":
		return CatSecurity
	case "system":
		return CatSystem
	}
	return ""
}

// Pick returns a uniformly random element from arr.
func Pick[T any](arr []T) T {
	if len(arr) == 0 {
		var zero T
		return zero
	}
	return arr[mrand.IntN(len(arr))]
}

// PickN returns up to n random elements from arr without repetition.
func PickN[T any](arr []T, n int) []T {
	if n <= 0 || len(arr) == 0 {
		return []T{}
	}
	if n > len(arr) {
		n = len(arr)
	}
	cp := make([]T, len(arr))
	copy(cp, arr)
	mrand.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	return cp[:n]
}

// NowISO returns RFC 3339 with millisecond precision (matches JS Date.toISOString()).
func NowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// RandomID returns 8 lowercase alphanumeric chars (matches TS randomId()).
func RandomID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[mrand.IntN(len(chars))]
	}
	return string(b)
}

func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func RandomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", mrand.IntN(256), mrand.IntN(256), mrand.IntN(256), mrand.IntN(256))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
