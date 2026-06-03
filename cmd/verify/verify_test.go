package main

import (
	"testing"

	"github.com/stonewrit/stonewrit/core"
)

// makeBundle builds a valid n-event bundle for one chain, returning the bundle
// and its true tip hash.
func makeBundle(t *testing.T, chainID string, n int) bundle {
	t.Helper()
	prev := ""
	var events []bundleEvent
	for i := 1; i <= n; i++ {
		ph, err := core.PayloadHash(map[string]any{"i": i})
		if err != nil {
			t.Fatalf("payload hash: %v", err)
		}
		eh := core.EventHash(prev, ph, int64(i))
		events = append(events, bundleEvent{
			ID:            chainID,
			ChainID:       chainID,
			ChainPosition: int64(i),
			PayloadHash:   ph,
			EventHash:     eh,
		})
		prev = eh
	}
	tip := prev
	return bundle{
		SchemaVersion: "stonewrit-evidence-bundle/1",
		Chains:        []bundleChain{{ID: chainID, LatestPosition: int64(n), LatestEventHash: &tip}},
		Events:        events,
	}
}

func TestVerifyBundle_Valid(t *testing.T) {
	rep := verifyBundle(makeBundle(t, "chain-a", 5))
	if !rep.Valid {
		t.Fatalf("expected valid, got %+v", rep)
	}
	if rep.ChainCount != 1 || rep.EventCount != 5 {
		t.Errorf("expected 1 chain / 5 events, got %d / %d", rep.ChainCount, rep.EventCount)
	}
	if rep.Chains[0].EventsVerified != 5 || !rep.Chains[0].TipMatches {
		t.Errorf("chain report wrong: %+v", rep.Chains[0])
	}
}

func TestVerifyBundle_Tampered(t *testing.T) {
	b := makeBundle(t, "chain-a", 4)
	b.Events[2].EventHash = "sha256:tampered"
	rep := verifyBundle(b)
	if rep.Valid {
		t.Fatal("expected invalid after tamper")
	}
	if rep.Chains[0].Break == nil || rep.Chains[0].Break.Position != 3 {
		t.Errorf("expected break at position 3, got %+v", rep.Chains[0].Break)
	}
}

func TestVerifyBundle_TipMismatch(t *testing.T) {
	b := makeBundle(t, "chain-a", 3)
	wrong := "sha256:not-the-real-tip"
	b.Chains[0].LatestEventHash = &wrong
	rep := verifyBundle(b)
	if rep.Valid {
		t.Fatal("expected invalid when the published tip does not match")
	}
	if rep.Chains[0].TipMatches {
		t.Error("tip should not match")
	}
}

func TestVerifyBundle_MultiChain(t *testing.T) {
	a := makeBundle(t, "chain-a", 2)
	c := makeBundle(t, "chain-c", 3)
	merged := bundle{
		SchemaVersion: a.SchemaVersion,
		Chains:        append(a.Chains, c.Chains...),
		Events:        append(a.Events, c.Events...),
	}
	rep := verifyBundle(merged)
	if !rep.Valid || rep.ChainCount != 2 || rep.EventCount != 5 {
		t.Errorf("expected valid 2 chains / 5 events, got %+v", rep)
	}

	// Break one chain; the whole report must fail but still verify the other.
	merged.Events[len(a.Events)+1].EventHash = "sha256:broken"
	rep = verifyBundle(merged)
	if rep.Valid {
		t.Error("expected invalid when one of several chains breaks")
	}
}
