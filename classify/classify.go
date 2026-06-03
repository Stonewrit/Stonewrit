// Package classify is the open rule engine: a pure, deterministic, data-driven
// classifier and control mapper over an embedded baseline ruleset. It runs
// inline on the accept path with no database, network, clock, or logging.
//
// Two things ship here. The engine (rule evaluation) is commodity logic with no
// IP. The baseline ruleset (defs/baseline) maps events to SOC 2, ISO 27001,
// HIPAA, and GDPR controls as an open point-in-time snapshot. Maintained,
// attested, and broader rulesets are kept current as a hosted offering and are
// not bundled here.
//
// Classify derives data classes and a risk level from an event:
//
// Rules:
//
//	data_classes:
//	  * customer-declared dataClasses (verbatim)
//	  * actorType == "ai_agent"        → add "ai_agent_action"
//	  * eventType startsWith "data."   → add "data_event"
//	  * eventType startsWith "admin."  → add "privileged_admin"
//	  * eventType startsWith "identity." → add "identity_event"
//	  * eventType startsWith "agent."  → add "ai_agent_action"
//	  * eventType startsWith "vendor." → add "vendor_data_egress"
//
//	risk_level (highest match wins):
//	  * data_classes ∩ {pii, customer_data, phi, pci, financial} → high
//	  * eventType == "data.exported" → high
//	  * eventType == "data.shared_external" → high
//	  * eventType startsWith "data." → at least medium
//	  * eventType startsWith "admin." → at least medium
//	  * actionResult == "denied" → at least medium
//	  * actorType == "ai_agent" → at least medium
//	  * default → low
package classify

import (
	"sort"
	"strings"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type Event struct {
	EventType    string
	ActorType    string
	ActionResult string
	DataClasses  []string
}

type Result struct {
	DataClasses []string  `json:"dataClasses"`
	RiskLevel   RiskLevel `json:"riskLevel"`
}

var highRisk = map[string]struct{}{
	"pii":           {},
	"customer_data": {},
	"phi":           {},
	"pci":           {},
	"financial":     {},
}

func Classify(e Event) Result {
	set := make(map[string]struct{}, len(e.DataClasses)+4)
	for _, c := range e.DataClasses {
		set[c] = struct{}{}
	}

	if e.ActorType == "ai_agent" {
		set["ai_agent_action"] = struct{}{}
	}
	if strings.HasPrefix(e.EventType, "data.") {
		set["data_event"] = struct{}{}
	}
	if strings.HasPrefix(e.EventType, "admin.") {
		set["privileged_admin"] = struct{}{}
	}
	if strings.HasPrefix(e.EventType, "identity.") {
		set["identity_event"] = struct{}{}
	}
	if strings.HasPrefix(e.EventType, "agent.") {
		set["ai_agent_action"] = struct{}{}
	}
	if strings.HasPrefix(e.EventType, "vendor.") {
		set["vendor_data_egress"] = struct{}{}
	}

	classes := make([]string, 0, len(set))
	for c := range set {
		classes = append(classes, c)
	}
	sort.Strings(classes)

	risk := RiskLow
	bumpHigh := func(cond bool) {
		if cond {
			risk = RiskHigh
		}
	}
	bumpAtLeastMedium := func() {
		if risk == RiskLow {
			risk = RiskMedium
		}
	}

	for _, c := range classes {
		if _, ok := highRisk[c]; ok {
			risk = RiskHigh
			break
		}
	}
	bumpHigh(e.EventType == "data.exported")
	bumpHigh(e.EventType == "data.shared_external")

	if risk != RiskHigh {
		if strings.HasPrefix(e.EventType, "data.") {
			bumpAtLeastMedium()
		}
		if strings.HasPrefix(e.EventType, "admin.") {
			bumpAtLeastMedium()
		}
		if e.ActionResult == "denied" {
			bumpAtLeastMedium()
		}
		if e.ActorType == "ai_agent" {
			bumpAtLeastMedium()
		}
	}

	return Result{
		DataClasses: classes,
		RiskLevel:   risk,
	}
}
