// Control-pack definitions and the evaluator over them. A pack bundles
// frameworks, controls, and event-to-control mapping rules in one JSON file.
// Adding a pack is dropping a JSON definition under defs and reseeding, with no
// code change.
//
// This file ships the baseline ruleset only. Maintained premium rulesets are a
// hosted offering, loaded by the hosted deployment, and are not bundled here.

package classify

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed defs/baseline/*.json
var baselineFS embed.FS

// Framework is a compliance framework header (for example SOC2 or HIPAA).
type Framework struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// Control is one control within a framework (e.g. SOC2 CC6.1).
type Control struct {
	Framework   string `json:"framework"`
	ControlID   string `json:"control_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// Match is a declarative predicate. A rule matches when EVERY populated
// dimension is satisfied; within a dimension the list is OR'd. The two
// event-type fields are one dimension (exact OR prefix).
type Match struct {
	EventTypeExact  []string `json:"eventTypeExact,omitempty"`
	EventTypePrefix []string `json:"eventTypePrefix,omitempty"`
	DataClassesAny  []string `json:"dataClassesAny,omitempty"`
	ActorTypeAny    []string `json:"actorTypeAny,omitempty"`
	ActionResultAny []string `json:"actionResultAny,omitempty"`
}

// Rule maps a matched event to control references ("FRAMEWORK:CONTROL_ID").
type Rule struct {
	ID       string   `json:"id"`
	Match    Match    `json:"match"`
	Controls []string `json:"controls"`
	Reason   string   `json:"reason"`
}

// Pack is one self-contained definition file.
type Pack struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Frameworks  []Framework `json:"frameworks"`
	Controls    []Control   `json:"controls"`
	Rules       []Rule      `json:"rules"`
}

// Subject is the event facts a rule is evaluated against.
type Subject struct {
	EventType    string
	DataClasses  []string
	ActorType    string
	ActionResult string
}

// Matches reports whether the subject satisfies the predicate.
func (m Match) Matches(s Subject) bool {
	if len(m.EventTypeExact) > 0 || len(m.EventTypePrefix) > 0 {
		ok := false
		for _, e := range m.EventTypeExact {
			if s.EventType == e {
				ok = true
				break
			}
		}
		if !ok {
			for _, p := range m.EventTypePrefix {
				if strings.HasPrefix(s.EventType, p) {
					ok = true
					break
				}
			}
		}
		if !ok {
			return false
		}
	}
	if len(m.DataClassesAny) > 0 && !intersects(s.DataClasses, m.DataClassesAny) {
		return false
	}
	if len(m.ActorTypeAny) > 0 && !contains(m.ActorTypeAny, s.ActorType) {
		return false
	}
	if len(m.ActionResultAny) > 0 && !contains(m.ActionResultAny, s.ActionResult) {
		return false
	}
	return true
}

var (
	once   sync.Once
	loaded []Pack
)

// All returns every embedded pack, sorted by pack id (so baseline sorts before
// premium packs and rule evaluation order is deterministic). Panics if an
// embedded definition is malformed - that is a compile-time-shaped bug caught
// by tests, never a runtime condition.
func All() []Pack {
	once.Do(func() {
		var err error
		loaded, err = load()
		if err != nil {
			panic(fmt.Sprintf("packs: %v", err))
		}
	})
	return loaded
}

func load() ([]Pack, error) {
	var out []Pack
	for _, src := range []struct {
		fsys embed.FS
		dir  string
	}{
		{baselineFS, "defs/baseline"},
	} {
		entries, err := fs.ReadDir(src.fsys, src.dir)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src.dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, err := src.fsys.ReadFile(src.dir + "/" + e.Name())
			if err != nil {
				return nil, fmt.Errorf("read %s/%s: %w", src.dir, e.Name(), err)
			}
			var p Pack
			if err := json.Unmarshal(b, &p); err != nil {
				return nil, fmt.Errorf("parse %s/%s: %w", src.dir, e.Name(), err)
			}
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Rules returns every rule across all packs, in pack-sorted, in-file order.
func Rules() []Rule {
	var rs []Rule
	for _, p := range All() {
		rs = append(rs, p.Rules...)
	}
	return rs
}

// Frameworks returns every framework across all packs.
func Frameworks() []Framework {
	var fws []Framework
	for _, p := range All() {
		fws = append(fws, p.Frameworks...)
	}
	return fws
}

// Controls returns every control across all packs.
func Controls() []Control {
	var cs []Control
	for _, p := range All() {
		cs = append(cs, p.Controls...)
	}
	return cs
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func intersects(a, b []string) bool {
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}
