package core

import (
	"time"

	"github.com/stonewrit/stonewrit/spec"
)

// ContentInput is everything the payload hash commits to for one event. The
// caller supplies the chain-context fields (organization, project, environment,
// and the server-stamped receive time) and the validated event body. core does
// not read a database or a clock; it only formats and hashes what it is given.
//
// ProjectID and EnvironmentID must be in their canonical string form (a
// lowercase UUID, for example), because that exact string is hashed. The ingest
// path and the verifier must produce the same string for the same identifier or
// the recomputed payload hash will not match.
type ContentInput struct {
	OrganizationID string
	ProjectID      string
	EnvironmentID  string
	ReceivedAt     time.Time
	Event          spec.EventInput
}

// BuildContentPayload produces the exact content map that PayloadHash digests.
//
// This is the single source of truth for the payload shape. Both the ingest
// accept path and the verifier call it, so the value they hash cannot drift.
// Optional fields are included only when present, matching how the value was
// hashed at ingest time. The server-assigned surrogate event id, the computed
// scope check, and the control mappings are deliberately NOT part of it.
func BuildContentPayload(in ContentInput) map[string]any {
	e := in.Event
	p := map[string]any{
		"organization_id": in.OrganizationID,
		"project_id":      in.ProjectID,
		"environment_id":  in.EnvironmentID,
		"received_at":     HashTime(in.ReceivedAt),
		"event_type":      e.EventType,
		"occurred_at":     HashTime(e.OccurredAt),
		"source":          e.Source,
		"actor":           e.Actor,
		"action":          e.Action,
		"resource":        e.Resource,
	}
	if e.ExternalEventID != "" {
		p["external_event_id"] = e.ExternalEventID
	}
	if e.Policy != nil {
		p["policy"] = e.Policy
	}
	if e.Request != nil {
		p["request"] = e.Request
	}
	if e.Metadata != nil {
		p["metadata"] = e.Metadata
	}
	return p
}
