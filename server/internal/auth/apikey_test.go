package auth

import "testing"

func TestHashKey(t *testing.T) {
	// Deterministic and a 64-char hex SHA-256 digest, which is the contract the
	// CLI relies on when it stores key_hash.
	a := HashKey("sk_demo_token")
	b := HashKey("sk_demo_token")
	if a != b {
		t.Fatal("HashKey is not deterministic")
	}
	if len(a) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(a))
	}
	if HashKey("sk_demo_token") == HashKey("sk_other_token") {
		t.Error("different tokens must hash differently")
	}
}

func TestDefaultContextHasAllScopes(t *testing.T) {
	ctx := DefaultContext()
	for _, s := range AllScopes {
		if !ctx.HasScope(s) {
			t.Errorf("default context missing scope %q", s)
		}
	}
}
