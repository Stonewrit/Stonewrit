package classify

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("no packs loaded")
	}
	// The open repository ships the baseline ruleset only. Premium rulesets are
	// a hosted offering and are not embedded here.
	if all[0].ID != "base" {
		t.Errorf("first pack should be 'base', got %q", all[0].ID)
	}
	for _, p := range all {
		if p.ID != "base" {
			t.Errorf("unexpected non-baseline pack embedded in the open repo: %q", p.ID)
		}
	}
}

// TestControlIDsUnique guards against two packs defining the same
// (framework, control_id) - that would collide on the controls catalog's
// unique index at seed time.
func TestControlIDsUnique(t *testing.T) {
	seen := map[string]string{} // "FRAMEWORK:CONTROL" -> pack id
	for _, p := range All() {
		for _, c := range p.Controls {
			key := c.Framework + ":" + c.ControlID
			if prev, dup := seen[key]; dup {
				t.Errorf("control %q defined in both %q and %q", key, prev, p.ID)
			}
			seen[key] = p.ID
		}
	}
}

// TestRulesReferenceSeededControls ensures every rule maps to a control that
// some pack actually defines. A dangling ref would be silently dropped by
// ResolveAgainstCatalog - a pack-authoring bug that produces no evidence.
func TestRulesReferenceSeededControls(t *testing.T) {
	defined := map[string]bool{}
	for _, c := range Controls() {
		defined[c.Framework+":"+c.ControlID] = true
	}
	for _, r := range Rules() {
		for _, ref := range r.Controls {
			if !strings.Contains(ref, ":") {
				t.Errorf("rule %q control ref %q is not FRAMEWORK:CONTROL_ID", r.ID, ref)
				continue
			}
			if !defined[ref] {
				t.Errorf("rule %q references undefined control %q", r.ID, ref)
			}
		}
	}
}

// TestFrameworksReferenced ensures every control's framework has a header
// somewhere across the loaded packs (frameworks are upsert-shared, e.g. HIPAA
// is declared in the baseline pack and reused by the premium pack).
func TestFrameworksReferenced(t *testing.T) {
	frameworks := map[string]bool{}
	for _, f := range Frameworks() {
		frameworks[f.ID] = true
	}
	for _, c := range Controls() {
		if !frameworks[c.Framework] {
			t.Errorf("control %s:%s references framework %q with no header in any pack", c.Framework, c.ControlID, c.Framework)
		}
	}
}
