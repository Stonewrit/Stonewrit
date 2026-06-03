package classify

import (
	"reflect"
	"sort"
	"testing"
)

func TestClassify_DataClasses(t *testing.T) {
	cases := []struct {
		name string
		in   Event
		want []string
	}{
		{"ai_agent_actor", Event{ActorType: "ai_agent"}, []string{"ai_agent_action"}},
		{"data_prefix", Event{EventType: "data.accessed"}, []string{"data_event"}},
		{"admin_prefix", Event{EventType: "admin.user.created"}, []string{"privileged_admin"}},
		{"identity_prefix", Event{EventType: "identity.login.failed"}, []string{"identity_event"}},
		{"agent_prefix", Event{EventType: "agent.tool_called"}, []string{"ai_agent_action"}},
		{"vendor_prefix", Event{EventType: "vendor.data_sent"}, []string{"vendor_data_egress"}},
		{
			"customer_preserved_and_sorted",
			Event{EventType: "data.accessed", DataClasses: []string{"pii", "customer_data"}},
			[]string{"customer_data", "data_event", "pii"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.in).DataClasses
			sort.Strings(tc.want)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DataClasses = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClassify_RiskLevel(t *testing.T) {
	cases := []struct {
		name string
		in   Event
		want RiskLevel
	}{
		{"sensitive_class_high", Event{EventType: "data.accessed", DataClasses: []string{"phi"}}, RiskHigh},
		{"exported_high", Event{EventType: "data.exported"}, RiskHigh},
		{"shared_external_high", Event{EventType: "data.shared_external"}, RiskHigh},
		{"data_prefix_medium", Event{EventType: "data.accessed"}, RiskMedium},
		{"admin_prefix_medium", Event{EventType: "admin.user.created"}, RiskMedium},
		{"denied_medium", Event{EventType: "system.config_changed", ActionResult: "denied"}, RiskMedium},
		{"ai_agent_medium", Event{EventType: "system.run", ActorType: "ai_agent"}, RiskMedium},
		{"default_low", Event{EventType: "system.heartbeat"}, RiskLow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.in).RiskLevel; got != tc.want {
				t.Errorf("RiskLevel = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMatch_AllDimensions checks the predicate semantics: every populated
// dimension must be satisfied, and within a dimension the list is OR'd.
func TestMatch_AllDimensions(t *testing.T) {
	m := Match{
		EventTypePrefix: []string{"agent."},
		DataClassesAny:  []string{"phi", "pci"},
		ActionResultAny: []string{"denied"},
	}
	yes := Subject{EventType: "agent.tool_called", DataClasses: []string{"pci"}, ActionResult: "denied"}
	if !m.Matches(yes) {
		t.Error("expected match when every dimension is satisfied")
	}
	no := Subject{EventType: "agent.tool_called", DataClasses: []string{"public"}, ActionResult: "denied"}
	if m.Matches(no) {
		t.Error("expected no match when the data-class dimension is unsatisfied")
	}
}
