package core

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGolden rewrites testdata/golden_vectors.json with freshly computed
// payload hashes. Run once to bootstrap, then never again unless the
// canonicalization version changes:
//
//	go test ./core/ -run TestGoldenVectors -update
//
// Every SDK and verifier reads the SAME frozen file and asserts it computes
// identical hashes. That cross-language assertion is what guards against
// canonicalization or hash drift between implementations.
var updateGolden = flag.Bool("update", false, "rewrite golden hash vectors")

const goldenPath = "testdata/golden_vectors.json"

type goldenVector struct {
	Name        string `json:"name"`
	Input       any    `json:"input"`
	PayloadHash string `json:"payload_hash"`
}

func TestGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var vectors []goldenVector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("parse golden: %v", err)
	}

	changed := false
	for i := range vectors {
		got, err := PayloadHash(vectors[i].Input)
		if err != nil {
			t.Fatalf("%s: PayloadHash: %v", vectors[i].Name, err)
		}
		if *updateGolden {
			if vectors[i].PayloadHash != got {
				vectors[i].PayloadHash = got
				changed = true
			}
			continue
		}
		if vectors[i].PayloadHash == "" {
			t.Fatalf("%s: golden empty. Run `go test ./core/ -run TestGoldenVectors -update` first", vectors[i].Name)
		}
		if got != vectors[i].PayloadHash {
			t.Errorf("%s: payload hash drift\n  golden: %s\n  got:    %s", vectors[i].Name, vectors[i].PayloadHash, got)
		}
	}

	if *updateGolden && changed {
		out, err := json.MarshalIndent(vectors, "", "  ")
		if err != nil {
			t.Fatalf("marshal golden: %v", err)
		}
		if err := os.WriteFile(filepath.Clean(goldenPath), append(out, '\n'), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %d golden vectors", len(vectors))
	}
}
