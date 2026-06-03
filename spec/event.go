// Package spec defines the public event schema: the wire types a client sends
// to the ingest API and the shape the verifier reconstructs to recompute a
// payload hash. The field set, length caps, and array limits are part of the
// frozen contract; every SDK and server validates against the same shape.
package spec

import "time"

type EventInput struct {
	EventType       string    `json:"event_type" validate:"required,min=1,max=120"`
	OccurredAt      time.Time `json:"occurred_at" validate:"required"`
	ExternalEventID string    `json:"external_event_id,omitempty" validate:"omitempty,max=200"`

	Source   Source         `json:"source" validate:"required"`
	Actor    Actor          `json:"actor" validate:"required"`
	Action   Action         `json:"action" validate:"required"`
	Resource Resource       `json:"resource" validate:"required"`
	Policy   *Policy        `json:"policy,omitempty"`
	Request  *RequestCtx    `json:"request,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Source struct {
	System      string `json:"system" validate:"required,min=1,max=120"`
	Service     string `json:"service" validate:"required,min=1,max=120"`
	Environment string `json:"environment,omitempty" validate:"omitempty,max=60"`
	Region      string `json:"region,omitempty" validate:"omitempty,max=60"`
	Version     string `json:"version,omitempty" validate:"omitempty,max=60"`
}

type Actor struct {
	Type              string `json:"type" validate:"required,min=1,max=60"`
	ID                string `json:"id,omitempty" validate:"omitempty,max=200"`
	DisplayName       string `json:"display_name,omitempty" validate:"omitempty,max=200"`
	Email             string `json:"email,omitempty" validate:"omitempty,email,max=320"`
	Role              string `json:"role,omitempty" validate:"omitempty,max=120"`
	HumanSupervisorID string `json:"human_supervisor_id,omitempty" validate:"omitempty,max=200"`
	IP                string `json:"ip,omitempty" validate:"omitempty,max=64"`
	UserAgent         string `json:"user_agent,omitempty" validate:"omitempty,max=500"`
	IDHash            string `json:"id_hash,omitempty" validate:"omitempty,max=128"`
}

type Action struct {
	Name     string `json:"name" validate:"required,min=1,max=120"`
	Category string `json:"category" validate:"required,min=1,max=60"`
	Result   string `json:"result" validate:"required,min=1,max=60"`
	Reason   string `json:"reason,omitempty" validate:"omitempty,max=500"`
}

type Resource struct {
	Type           string   `json:"type" validate:"required,min=1,max=120"`
	ID             string   `json:"id,omitempty" validate:"omitempty,max=200"`
	IDHash         string   `json:"id_hash,omitempty" validate:"omitempty,max=128"`
	TenantID       string   `json:"tenant_id,omitempty" validate:"omitempty,max=200"`
	TenantIDHash   string   `json:"tenant_id_hash,omitempty" validate:"omitempty,max=128"`
	Classification []string `json:"classification,omitempty" validate:"omitempty,max=20,dive,max=60"`
	FieldsAccessed []string `json:"fields_accessed,omitempty" validate:"omitempty,max=50,dive,max=120"`
}

type Policy struct {
	Decision         string   `json:"decision,omitempty" validate:"omitempty,max=60"`
	PolicyID         string   `json:"policy_id,omitempty" validate:"omitempty,max=200"`
	ApprovalRequired *bool    `json:"approval_required,omitempty"`
	ApprovalID       string   `json:"approval_id,omitempty" validate:"omitempty,max=200"`
	ControlIDs       []string `json:"control_ids,omitempty" validate:"omitempty,max=20,dive,max=120"`
}

type RequestCtx struct {
	RequestID     string `json:"request_id,omitempty" validate:"omitempty,max=200"`
	TraceID       string `json:"trace_id,omitempty" validate:"omitempty,max=200"`
	SessionID     string `json:"session_id,omitempty" validate:"omitempty,max=200"`
	SessionIDHash string `json:"session_id_hash,omitempty" validate:"omitempty,max=128"`
}

// BatchRequest is the body of POST /api/v1/events/batch.
type BatchRequest struct {
	Events []EventInput `json:"events" validate:"required,min=1,max=1000,dive"`
}

// AcceptedResponse is what 202 returns for both /events and /events/batch.
//
// Notably we return `payload_hash` and `accepted_at` immediately so clients
// hold a valid content-hash receipt even before the sealer has assigned
// a chain position. Once the sealer commits, the same `id` is queryable
// via GET /api/v1/events/:id with status=sealed and full chain hashes.
type AcceptedResponse struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	PayloadHash   string    `json:"payload_hash"`
	AcceptedAt    time.Time `json:"accepted_at"`
	EnvironmentID string    `json:"environment_id"`
	ShardIndex    int       `json:"shard_index"`
}

type BatchAcceptedResponse struct {
	Accepted int                `json:"accepted"`
	Rejected int                `json:"rejected"`
	Results  []BatchEventResult `json:"results"`
}

type BatchEventResult struct {
	Status   string            `json:"status"` // "accepted" | "rejected"
	Index    int               `json:"index"`
	Response *AcceptedResponse `json:"response,omitempty"`
	Error    *BatchEventError  `json:"error,omitempty"`
}

type BatchEventError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
