package classify

import (
	"reflect"
	"testing"
)

// TestComputeParity is the regression gate for the pack-engine refactor: the
// data-driven Compute must reproduce the EXACT mappings the original hardcoded
// rule table produced, for every baseline event type (no sensitive data classes,
// so premium packs don't fire). If this breaks, the refactor changed behavior.
func TestComputeParity(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		customer  []string
		want      []Mapping
	}{
		{"identity_change", "identity.permission.granted", nil, []Mapping{
			{"SOC2", "CC6.1", "identity_change"},
		}},
		{"role_change", "identity.role.changed", nil, []Mapping{
			{"SOC2", "CC6.1", "role_change"},
			{"SOC2", "CC6.2", "role_change"},
		}},
		{"login_anomaly", "identity.login.failed", nil, []Mapping{
			{"SOC2", "CC7.2", "login_anomaly"},
		}},
		{"privileged_admin_action", "admin.user.created", nil, []Mapping{
			{"SOC2", "CC6.1", "privileged_admin_action"},
			{"SOC2", "CC7.2", "privileged_admin_action"},
		}},
		{"data_access", "data.accessed", nil, []Mapping{
			{"SOC2", "CC6.1", "data_access"},
			{"SOC2", "CC7.2", "data_access"},
		}},
		{"data_export", "data.exported", nil, []Mapping{
			{"SOC2", "CC6.1", "data_export"},
			{"SOC2", "CC7.2", "data_export"},
			{"GDPR", "Art. 30", "data_export"},
		}},
		{"external_data_sharing", "data.shared_external", nil, []Mapping{
			{"SOC2", "CC9.2", "external_data_sharing"},
			{"GDPR", "Art. 30", "external_data_sharing"},
			{"GDPR", "Art. 32", "external_data_sharing"},
		}},
		{"data_deletion", "data.deleted", nil, []Mapping{
			{"SOC2", "CC7.2", "data_deletion"},
			{"GDPR", "Art. 32", "data_deletion"},
		}},
		{"retention_enforcement", "data.retention_applied", nil, []Mapping{
			{"GDPR", "Art. 30", "retention_enforcement"},
		}},
		{"ai_agent_action", "agent.tool_called", nil, []Mapping{
			{"SOC2", "CC6.1", "ai_agent_action"},
			{"SOC2", "CC7.2", "ai_agent_action"},
			{"SOC2", "CC8.1", "ai_agent_action"},
		}},
		{"continuous_control_monitoring", "control.check_failed", nil, []Mapping{
			{"SOC2", "CC7.2", "continuous_control_monitoring"},
		}},
		{"control_exception", "control.exception_opened", nil, []Mapping{
			{"SOC2", "CC7.2", "control_exception"},
			{"SOC2", "CC9.2", "control_exception"},
		}},
		{"vendor_data_transfer", "vendor.data_sent", nil, []Mapping{
			{"SOC2", "CC9.2", "vendor_data_transfer"},
			{"GDPR", "Art. 30", "vendor_data_transfer"},
			{"GDPR", "Art. 32", "vendor_data_transfer"},
		}},
		{"vendor_api_call", "vendor.api_called", nil, []Mapping{
			{"SOC2", "CC9.2", "vendor_api_call"},
		}},
		{"security_signal", "security.alert_triggered", nil, []Mapping{
			{"SOC2", "CC7.2", "security_signal"},
		}},
		{"change_management", "system.config_changed", nil, []Mapping{
			{"SOC2", "CC8.1", "change_management"},
		}},
		{"incident_response", "system.incident_opened", nil, []Mapping{
			{"SOC2", "CC7.2", "incident_response"},
			{"GDPR", "Art. 33", "incident_response"},
		}},
		{"unmatched", "random.unknown", nil, nil},
		{"customer_declared_only", "random.unknown", []string{"HIPAA:164.312(b)"}, []Mapping{
			{"HIPAA", "164.312(b)", "customer_declared"},
		}},
		{"customer_declared_missing_colon_dropped", "random.unknown", []string{"no-framework"}, nil},
		{"customer_declared_appended_after_rules", "agent.tool_called", []string{"ISO27001:A.8.15"}, []Mapping{
			{"SOC2", "CC6.1", "ai_agent_action"},
			{"SOC2", "CC7.2", "ai_agent_action"},
			{"SOC2", "CC8.1", "ai_agent_action"},
			{"ISO27001", "A.8.15", "customer_declared"},
		}},
		{"customer_declared_dup_keeps_rule_reason", "agent.tool_called", []string{"SOC2:CC6.1"}, []Mapping{
			{"SOC2", "CC6.1", "ai_agent_action"},
			{"SOC2", "CC7.2", "ai_agent_action"},
			{"SOC2", "CC8.1", "ai_agent_action"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(MapInput{EventType: tc.eventType, CustomerControlIDs: tc.customer})
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compute(%q) =\n  %+v\nwant\n  %+v", tc.eventType, got, tc.want)
			}
		})
	}
}

// TestBaselineOnlyForSensitiveClasses confirms the open repository ships the
// baseline ruleset only. An agent action on sensitive data still maps to the
// baseline SOC2 controls, and it does NOT pick up any HIPAA, NYDFS, or SEC
// controls, because the maintained premium rulesets that add those are a hosted
// offering and are not embedded here.
func TestBaselineOnlyForSensitiveClasses(t *testing.T) {
	baselineAgent := []Mapping{
		{"SOC2", "CC6.1", "ai_agent_action"},
		{"SOC2", "CC7.2", "ai_agent_action"},
		{"SOC2", "CC8.1", "ai_agent_action"},
	}

	for _, dc := range []string{"phi", "financial", "pci"} {
		got := Compute(MapInput{EventType: "agent.tool_called", DataClasses: []string{dc}})
		if !reflect.DeepEqual(got, baselineAgent) {
			t.Errorf("agent.* with data class %q should map to baseline SOC2 only, got\n  %+v", dc, got)
		}
		for _, m := range got {
			if m.Framework != "SOC2" {
				t.Errorf("unexpected non-baseline framework %q in open repo for data class %q", m.Framework, dc)
			}
		}
	}

	// A non-sensitive agent action gets the same baseline SOC2 mapping.
	plain := Compute(MapInput{EventType: "agent.tool_called"})
	if !reflect.DeepEqual(plain, baselineAgent) {
		t.Errorf("agent.* with no data classes should map to baseline SOC2 only, got\n  %+v", plain)
	}
}
