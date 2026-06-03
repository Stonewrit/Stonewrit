package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stonewrit/stonewrit/spec"
)

func sampleEvent() spec.EventInput {
	approval := true
	return spec.EventInput{
		EventType:       "data.accessed",
		OccurredAt:      time.Date(2026, 5, 29, 15, 11, 58, 655_000_000, time.UTC),
		ExternalEventID: "ext-123",
		Source:          spec.Source{System: "billing", Service: "api"},
		Actor:           spec.Actor{Type: "human", ID: "u1", Email: "u1@example.com"},
		Action:          spec.Action{Name: "read", Category: "data", Result: "allowed"},
		Resource:        spec.Resource{Type: "invoice", ID: "inv_9", Classification: []string{"pii"}},
		Policy:          &spec.Policy{Decision: "allow", ApprovalRequired: &approval},
		Request:         &spec.RequestCtx{RequestID: "req_1", TraceID: "trace_1"},
		Metadata:        map[string]any{"region": "us-east-1", "n": 42},
	}
}

func sampleContent() ContentInput {
	return ContentInput{
		OrganizationID: "org_123",
		ProjectID:      "11111111-1111-1111-1111-111111111111",
		EnvironmentID:  "22222222-2222-2222-2222-222222222222",
		ReceivedAt:     time.Date(2026, 5, 29, 15, 11, 58, 655_000_000, time.UTC),
		Event:          sampleEvent(),
	}
}

// TestPayloadHash_RoundTripIdentity is the load-bearing invariant: the hash the
// accept path computes from the in-memory event must equal the hash the
// verifier computes after the event has been serialized to storage and read
// back. If serialization changed any field, this would diverge.
func TestPayloadHash_RoundTripIdentity(t *testing.T) {
	in := sampleContent()

	// Accept path: hash the in-memory event directly.
	acceptHash, err := PayloadHash(BuildContentPayload(in))
	if err != nil {
		t.Fatalf("accept hash: %v", err)
	}

	// Verify path: the event was stored as raw_payload JSON and read back.
	raw, err := json.Marshal(in.Event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var roundTripped spec.EventInput
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	verifyIn := in
	verifyIn.Event = roundTripped
	verifyHash, err := PayloadHash(BuildContentPayload(verifyIn))
	if err != nil {
		t.Fatalf("verify hash: %v", err)
	}

	if acceptHash != verifyHash {
		t.Errorf("payload hash diverged across serialization\n accept: %s\n verify: %s", acceptHash, verifyHash)
	}

	// And the event hash chained from it must also agree.
	if a, b := EventHash("", acceptHash, 1), EventHash("", verifyHash, 1); a != b {
		t.Errorf("event hash diverged\n accept: %s\n verify: %s", a, b)
	}
}

// TestBuildContentPayload_OptionalFields confirms optional fields appear only
// when present, since their presence changes the hash and must match how the
// event was hashed at ingest.
func TestBuildContentPayload_OptionalFields(t *testing.T) {
	always := []string{
		"organization_id", "project_id", "environment_id", "received_at",
		"event_type", "occurred_at", "source", "actor", "action", "resource",
	}

	full := BuildContentPayload(sampleContent())
	for _, k := range always {
		if _, ok := full[k]; !ok {
			t.Errorf("expected key %q always present", k)
		}
	}
	for _, k := range []string{"external_event_id", "policy", "request", "metadata"} {
		if _, ok := full[k]; !ok {
			t.Errorf("expected optional key %q present when set", k)
		}
	}

	bare := sampleContent()
	bare.Event = spec.EventInput{
		EventType:  "system.ping",
		OccurredAt: time.Unix(0, 0).UTC(),
		Source:     spec.Source{System: "s", Service: "svc"},
		Actor:      spec.Actor{Type: "system"},
		Action:     spec.Action{Name: "ping", Category: "ops", Result: "allowed"},
		Resource:   spec.Resource{Type: "node"},
	}
	min := BuildContentPayload(bare)
	for _, k := range []string{"external_event_id", "policy", "request", "metadata"} {
		if _, ok := min[k]; ok {
			t.Errorf("optional key %q must be absent when unset", k)
		}
	}
}

// TestPayloadHash_ReceivedAtMillisecond confirms received_at is hashed at
// millisecond precision: sub-millisecond differences collapse to the same hash,
// while a one-millisecond difference does not.
func TestPayloadHash_ReceivedAtMillisecond(t *testing.T) {
	base := sampleContent()

	a := base
	a.ReceivedAt = time.Date(2026, 5, 29, 15, 11, 58, 655_000_000, time.UTC)
	b := base
	b.ReceivedAt = time.Date(2026, 5, 29, 15, 11, 58, 655_999_000, time.UTC) // +999us

	ha, _ := PayloadHash(BuildContentPayload(a))
	hb, _ := PayloadHash(BuildContentPayload(b))
	if ha != hb {
		t.Errorf("sub-millisecond difference changed the hash:\n %s\n %s", ha, hb)
	}

	c := base
	c.ReceivedAt = time.Date(2026, 5, 29, 15, 11, 58, 656_000_000, time.UTC) // +1ms
	hc, _ := PayloadHash(BuildContentPayload(c))
	if ha == hc {
		t.Error("a one-millisecond difference should change the hash")
	}
}
