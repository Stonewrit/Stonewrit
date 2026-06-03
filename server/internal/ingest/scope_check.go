package ingest

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/spec"
)

// ScopeCheckResult is the observe-only authorization annotation written onto an
// agent event. It records, AS OF accept time, whether the action a registered
// agent performed was within the scopes the agent was authorized for.
//
// It is a SNAPSHOT, not a live reference: agent name/status/scopes are copied in
// so the historical event stays meaningful even after the agent is mutated or
// deleted. It is computed metadata - NEVER part of payload_hash - and the sealer
// copies it verbatim to events.scope_check_result.
type ScopeCheckResult struct {
	AgentID          string   `json:"agent_id,omitempty"`
	AgentExternalID  string   `json:"agent_external_id"`
	AgentName        string   `json:"agent_name,omitempty"`
	AgentStatus      string   `json:"agent_status,omitempty"`
	AuthorizedScopes []string `json:"authorized_scopes,omitempty"`
	RequiredScope    string   `json:"required_scope"`
	InScope          bool     `json:"in_scope"`
	OutOfScope       []string `json:"out_of_scope,omitempty"`
	// ok | agent_not_registered | scope_mismatch | agent_suspended | agent_archived
	Reason string `json:"reason"`
}

// scopeSatisfied is the PURE authorization decision: is `required` covered by the
// agent's `authorized` scope set? Side-effect free and dependency-free - this is
// the seam that, at the open-core split, can sit behind a pluggable interface
// (the DB resolution in ComputeScopeCheck is the only impure part).
//
// v1 convention: an action's required scope is its `action.category`; an agent is
// authorized when that exact category is in its authorized_scopes.
func scopeSatisfied(required string, authorized []string) (in bool, missing []string) {
	for _, s := range authorized {
		if s == required {
			return true, nil
		}
	}
	return false, []string{required}
}

// ComputeScopeCheck resolves an agent event's actor.id to a registered agent
// within (org, project) and computes the observe-only scope_check. Returns nil
// for non-agent events. NEVER rejects: a not-registered actor yields a result
// with reason=agent_not_registered, not an error.
//
// A genuine DB failure returns a non-nil error; the caller treats it as
// non-fatal (ingest proceeds with no scope_check) - observe-only must never
// block the hot path, and this also keeps the open-core server (which may not
// have an agents table) working: the lookup errors, we skip, ingest continues.
func ComputeScopeCheck(
	ctx context.Context,
	q *queries.Queries,
	orgID string,
	projectID pgtype.UUID,
	actor spec.Actor,
	action spec.Action,
) (*ScopeCheckResult, error) {
	if actor.Type != "ai_agent" {
		return nil, nil
	}

	required := action.Category

	agent, err := q.GetAgentByExternalID(ctx, queries.GetAgentByExternalIDParams{
		OrganizationID: orgID,
		ProjectID:      projectID,
		ExternalID:     actor.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return &ScopeCheckResult{
			AgentExternalID: actor.ID,
			RequiredScope:   required,
			InScope:         false,
			OutOfScope:      []string{required},
			Reason:          "agent_not_registered",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	satisfied, missing := scopeSatisfied(required, agent.AuthorizedScopes)
	active := agent.Status == "active"

	res := &ScopeCheckResult{
		AgentID:          uuid.UUID(agent.ID.Bytes).String(),
		AgentExternalID:  actor.ID,
		AgentName:        agent.Name,
		AgentStatus:      agent.Status,
		AuthorizedScopes: agent.AuthorizedScopes,
		RequiredScope:    required,
		InScope:          satisfied && active,
	}
	switch {
	case !active:
		res.Reason = "agent_" + agent.Status // agent_suspended | agent_archived
		if !satisfied {
			res.OutOfScope = missing
		}
	case !satisfied:
		res.Reason = "scope_mismatch"
		res.OutOfScope = missing
	default:
		res.Reason = "ok"
	}
	return res, nil
}
