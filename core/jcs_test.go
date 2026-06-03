package core

import (
	"encoding/json"
	"testing"
)

// TestCanonicalJSON_KeyOrdering pins the most important JCS property: object
// keys are emitted in lexicographic order regardless of input order.
func TestCanonicalJSON_KeyOrdering(t *testing.T) {
	got, err := CanonicalJSON(map[string]any{"b": 1, "a": 2, "c": 3})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if want := `{"a":2,"b":1,"c":3}`; string(got) != want {
		t.Errorf("key ordering\n got:  %s\n want: %s", got, want)
	}
}

// TestCanonicalJSON_Stable checks the canonical byte form for a range of inputs
// that exercise the documented hard cases: unicode, control characters, empty
// containers, nesting, and booleans/null.
func TestCanonicalJSON_Stable(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"empty_object", map[string]any{}, `{}`},
		{"empty_array", []any{}, `[]`},
		{"nested", map[string]any{"obj": map[string]any{"x": []any{1, 2}}}, `{"obj":{"x":[1,2]}}`},
		{"null_and_bool", map[string]any{"a": nil, "b": true, "c": false}, `{"a":null,"b":true,"c":false}`},
		{"unicode_preserved", map[string]any{"s": "café 日本語"}, `{"s":"café 日本語"}`},
		{"control_chars_escaped", map[string]any{"s": "a\nb\tc"}, `{"s":"a\nb\tc"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanonicalJSON(tc.in)
			if err != nil {
				t.Fatalf("CanonicalJSON: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("\n got:  %s\n want: %s", got, tc.want)
			}
		})
	}
}

// TestCanonicalJSON_Idempotent verifies that re-canonicalizing canonical bytes
// yields identical bytes. If this ever fails, the canonicalizer is not a fixed
// point and cross-language parity cannot hold.
func TestCanonicalJSON_Idempotent(t *testing.T) {
	inputs := []any{
		map[string]any{},
		map[string]any{"z": 1, "a": map[string]any{"y": 2, "x": 3}},
		map[string]any{"arr": []any{3, 2, 1}, "s": "mixed café ☕"},
		map[string]any{"big": 9007199254740993.0, "neg": -1, "f": 1.5},
	}
	for i, in := range inputs {
		first, err := CanonicalJSON(in)
		if err != nil {
			t.Fatalf("case %d: CanonicalJSON: %v", i, err)
		}
		var reparsed any
		if err := json.Unmarshal(first, &reparsed); err != nil {
			t.Fatalf("case %d: reparse: %v", i, err)
		}
		second, err := CanonicalJSON(reparsed)
		if err != nil {
			t.Fatalf("case %d: re-canonicalize: %v", i, err)
		}
		if string(first) != string(second) {
			t.Errorf("case %d not idempotent\n first:  %s\n second: %s", i, first, second)
		}
	}
}

// TestCanonicalJSON_Error confirms that an unserializable value surfaces an
// error rather than producing garbage bytes.
func TestCanonicalJSON_Error(t *testing.T) {
	if _, err := CanonicalJSON(make(chan int)); err == nil {
		t.Error("expected an error canonicalizing a channel value")
	}
}
