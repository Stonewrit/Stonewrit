// Control mapping: turns an event into proposed (framework, control_id)
// mappings by evaluating the declarative ruleset. The server follows up by
// resolving these against its seeded controls catalog.
//
// These are suggested evidence mappings. Stonewrit never claims compliance; it
// surfaces mappings to operators and auditors and lets the human decide.

package classify

import "strings"

type Mapping struct {
	Framework string `json:"framework"`
	ControlID string `json:"control_id"`
	Reason    string `json:"reason"`
}

// MapInput is the event facts the ruleset is evaluated against. DataClasses is
// the classifier output (resource classification plus derived classes), which
// is what data-class-conditional rules match on.
type MapInput struct {
	EventType          string
	DataClasses        []string
	ActorType          string
	ActionResult       string
	CustomerControlIDs []string // policy.control_ids, shaped "FRAMEWORK:CONTROL_ID"
}

// Compute returns the proposed mappings for an event by evaluating every rule
// against it, plus any customer-declared policy.control_ids shaped as
// "FRAMEWORK:CONTROL_ID". Rules are evaluated in pack-sorted, in-file order; the
// first rule to contribute a control reference owns its reason. Customer
// references without a colon are dropped, since the framework cannot be
// inferred otherwise.
func Compute(in MapInput) []Mapping {
	subject := Subject{
		EventType:    in.EventType,
		DataClasses:  in.DataClasses,
		ActorType:    in.ActorType,
		ActionResult: in.ActionResult,
	}

	type hit struct {
		ref    string
		reason string
	}
	order := make([]hit, 0, 4)
	seen := make(map[string]struct{}, 4)

	for _, r := range Rules() {
		if !r.Match.Matches(subject) {
			continue
		}
		for _, ref := range r.Controls {
			if _, dup := seen[ref]; dup {
				continue
			}
			seen[ref] = struct{}{}
			order = append(order, hit{ref: ref, reason: r.Reason})
		}
	}
	for _, ref := range in.CustomerControlIDs {
		if !strings.Contains(ref, ":") {
			continue
		}
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		order = append(order, hit{ref: ref, reason: "customer_declared"})
	}

	out := make([]Mapping, 0, len(order))
	for _, h := range order {
		idx := strings.Index(h.ref, ":")
		out = append(out, Mapping{
			Framework: h.ref[:idx],
			ControlID: h.ref[idx+1:],
			Reason:    h.reason,
		})
	}
	return out
}
