package core

import "testing"

// Known-good vectors for the chain hash. These outputs must remain stable for
// the lifetime of the codebase, or every event hash ever produced becomes
// invalid. A divergence here is a canonicalization regression, not a value to
// update.

func TestPayloadHash_Stable(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		want    string
	}{
		{
			name:    "empty object",
			payload: map[string]any{},
			want:    "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
		},
		{
			name: "ordered keys",
			payload: map[string]any{
				"b": 1,
				"a": "hello",
			},
			// JCS sorts keys lexically: {"a":"hello","b":1}
			want: "sha256:6a3598197725135098bf9a34cac41f76010823521079ba0678b887f9b6786436",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PayloadHash(tc.payload)
			if err != nil {
				t.Fatalf("PayloadHash: %v", err)
			}
			if got != tc.want {
				t.Errorf("hash mismatch\n got:  %s\n want: %s", got, tc.want)
			}
		})
	}
}

func TestEventHash_Genesis(t *testing.T) {
	got := EventHash("", "sha256:abc", 1)
	want := EventHash("genesis", "sha256:abc", 1)
	if got != want {
		t.Errorf("empty previousHash must equal 'genesis'\n got:  %s\n want: %s", got, want)
	}
}

func TestEventHash_Stable(t *testing.T) {
	// Whatever this resolves to, it must never change. It is part of the
	// frozen jcs-v1 contract that every event hash on disk depends on.
	got := EventHash("genesis", "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", 1)
	want := "sha256:a25a3647c71aae8585b6e6609c9eee5ae3d79d16bf0bc94944aba43ba4f7a724"
	if got != want {
		t.Errorf("event hash drift\n got:  %s\n want: %s", got, want)
	}
}
