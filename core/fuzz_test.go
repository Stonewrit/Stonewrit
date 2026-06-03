package core

import (
	"encoding/json"
	"testing"
)

// FuzzCanonicalJSON asserts two invariants over arbitrary JSON: canonicalizing
// never panics on valid input, and the canonical form is a fixed point
// (canonicalizing it again yields identical bytes). Idempotence is the property
// that lets independent implementations agree on a hash.
func FuzzCanonicalJSON(f *testing.F) {
	seeds := []string{
		`{}`,
		`{"a":1,"b":2}`,
		`{"z":[1,2,3],"a":{"nested":true}}`,
		`{"s":"café ☕ 日本語 🔒","ctrl":"a\nb\tc"}`,
		`[1,2,3]`,
		`{"big":9007199254740993,"neg":-5,"f":1.25}`,
		`{"a":null,"b":false}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return // only exercise valid JSON
		}
		first, err := CanonicalJSON(v)
		if err != nil {
			return
		}
		var reparsed any
		if err := json.Unmarshal(first, &reparsed); err != nil {
			t.Fatalf("canonical output is not valid JSON: %v", err)
		}
		second, err := CanonicalJSON(reparsed)
		if err != nil {
			t.Fatalf("re-canonicalize failed: %v", err)
		}
		if string(first) != string(second) {
			t.Fatalf("not idempotent\n first:  %s\n second: %s", first, second)
		}
	})
}
