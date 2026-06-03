package core

import "testing"

// buildChain returns n linked records starting at genesis, each with content so
// payload hashes are checked too. ptr is a helper for the previous-hash field.
func buildChain(t *testing.T, n int) []Record {
	t.Helper()
	records := make([]Record, 0, n)
	prev := ""
	for i := 1; i <= n; i++ {
		content := sampleContent()
		content.Event.ExternalEventID = ""
		content.Event.Metadata = map[string]any{"i": i}

		ph, err := PayloadHash(BuildContentPayload(content))
		if err != nil {
			t.Fatalf("payload hash: %v", err)
		}
		eh := EventHash(prev, ph, int64(i))

		rec := Record{
			PayloadHash:   ph,
			EventHash:     eh,
			ChainPosition: int64(i),
			Content:       &content,
		}
		if i > 1 {
			p := prev
			rec.PreviousEventHash = &p
		}
		records = append(records, rec)
		prev = eh
	}
	return records
}

func TestVerifyEvent_GenesisValid(t *testing.T) {
	rec := buildChain(t, 1)[0]
	res := VerifyEvent(rec)
	if !res.Valid {
		t.Fatalf("expected valid, got %+v", res)
	}
	if !res.Checks.EventHashMatches || !res.Checks.LinksToPrevious ||
		!res.Checks.PayloadHashChecked || !res.Checks.PayloadHashMatches {
		t.Errorf("expected all checks true, got %+v", res.Checks)
	}
}

func TestVerifyEvent_TamperedPayload(t *testing.T) {
	rec := buildChain(t, 1)[0]
	// Mutate the content so the recomputed payload hash no longer matches.
	rec.Content.Event.Action.Result = "denied"
	res := VerifyEvent(rec)
	if res.Valid {
		t.Error("expected invalid after content tamper")
	}
	if res.Checks.PayloadHashMatches {
		t.Error("payload hash should not match after tamper")
	}
}

func TestVerifyEvent_TamperedEventHash(t *testing.T) {
	rec := buildChain(t, 1)[0]
	rec.EventHash = "sha256:deadbeef"
	res := VerifyEvent(rec)
	if res.Valid || res.Checks.EventHashMatches {
		t.Errorf("expected event hash mismatch, got %+v", res.Checks)
	}
}

func TestVerifyEvent_BrokenLink(t *testing.T) {
	recs := buildChain(t, 2)
	// Position 2 with a nil previous hash is a broken link.
	rec := recs[1]
	rec.PreviousEventHash = nil
	res := VerifyEvent(rec)
	if res.Checks.LinksToPrevious {
		t.Error("position 2 with nil previous hash must not link")
	}
}

func TestVerifyEvent_NoContentSkipsPayloadCheck(t *testing.T) {
	rec := buildChain(t, 1)[0]
	rec.Content = nil
	res := VerifyEvent(rec)
	if res.Checks.PayloadHashChecked {
		t.Error("payload should not be checked without content")
	}
	if !res.Valid {
		t.Error("event with valid linkage and no content should still be valid")
	}
}

func TestVerifyChain_Valid(t *testing.T) {
	res := VerifyChain(buildChain(t, 5))
	if !res.Valid {
		t.Fatalf("expected valid chain, got %+v", res)
	}
	if res.EventsVerified != 5 || res.Count != 5 {
		t.Errorf("expected 5/5 verified, got %d/%d", res.EventsVerified, res.Count)
	}
	if res.Break != nil {
		t.Errorf("unexpected break: %+v", res.Break)
	}
}

func TestVerifyChain_BreakAtEventHash(t *testing.T) {
	recs := buildChain(t, 4)
	recs[2].EventHash = "sha256:tampered"
	res := VerifyChain(recs)
	if res.Valid {
		t.Fatal("expected invalid chain")
	}
	if res.Break == nil || res.Break.Position != 3 {
		t.Errorf("expected break at position 3, got %+v", res.Break)
	}
	if res.EventsVerified != 2 {
		t.Errorf("expected 2 events verified before break, got %d", res.EventsVerified)
	}
}

func TestVerifyChain_BreakAtLinkage(t *testing.T) {
	recs := buildChain(t, 3)
	bogus := "sha256:not-the-tip"
	recs[1].PreviousEventHash = &bogus
	res := VerifyChain(recs)
	if res.Valid || res.Break == nil || res.Break.Position != 2 {
		t.Errorf("expected linkage break at position 2, got valid=%v break=%+v", res.Valid, res.Break)
	}
}

func TestVerifyChain_BreakAtPayload(t *testing.T) {
	recs := buildChain(t, 3)
	recs[1].Content.Event.Action.Result = "denied"
	res := VerifyChain(recs)
	if res.Valid || res.Break == nil || res.Break.Reason != "payload_hash mismatch" {
		t.Errorf("expected payload break, got valid=%v break=%+v", res.Valid, res.Break)
	}
}

// TestVerifyChain_WithoutStoredPreviousHash proves a bundle that omits the
// denormalized previous_event_hash still verifies via the running tip.
func TestVerifyChain_WithoutStoredPreviousHash(t *testing.T) {
	recs := buildChain(t, 4)
	for i := range recs {
		recs[i].PreviousEventHash = nil
		recs[i].Content = nil // bundle without raw content
	}
	res := VerifyChain(recs)
	if !res.Valid || res.EventsVerified != 4 {
		t.Errorf("expected valid 4/4, got valid=%v verified=%d break=%+v", res.Valid, res.EventsVerified, res.Break)
	}
}
