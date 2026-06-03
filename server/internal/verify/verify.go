// Package verify recomputes hashes for a stored event and confirms the chain
// invariants. It is a thin server-side adapter: it reads the row from Postgres,
// hands the pure values to the core verifier, and shapes the HTTP response. The
// recomputation itself lives in core so the server and the public verifier
// cannot disagree.
package verify

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stonewrit/stonewrit/core"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/spec"
)

var ErrNotFound = errors.New("event not found")

type State string

const (
	StatePending State = "pending"
	StateSealed  State = "sealed"
)

type Checks struct {
	PayloadHashMatches bool `json:"payload_hash_matches"`
	EventHashMatches   bool `json:"event_hash_matches"`
	LinksToPrevious    bool `json:"links_to_previous"`
}

type Result struct {
	Valid    bool              `json:"valid"`
	State    State             `json:"state"`
	Checks   Checks            `json:"checks"`
	Computed map[string]string `json:"computed"`
	Stored   map[string]any    `json:"stored"`
}

type Service struct {
	Q *queries.Queries
}

func (s *Service) VerifyEvent(ctx context.Context, orgID, eventIDStr string) (*Result, error) {
	eventUUID, err := parseUUID(eventIDStr)
	if err != nil {
		return nil, ErrNotFound
	}

	ev, err := s.Q.GetEventForOrg(ctx, queries.GetEventForOrgParams{
		ID:             eventUUID,
		OrganizationID: orgID,
	})
	if err == nil {
		return s.verifySealed(ev), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	pending, err := s.Q.GetPendingEventForOrg(ctx, queries.GetPendingEventForOrgParams{
		ID:             eventUUID,
		OrganizationID: orgID,
	})
	if err == nil {
		return s.verifyPending(pending)
	}
	return nil, ErrNotFound
}

func (s *Service) verifySealed(ev queries.Event) *Result {
	// Reconstruct the content envelope the accept path hashed, in the exact
	// string form (canonical UUIDs) so the recomputed payload hash matches.
	var input spec.EventInput
	var content *core.ContentInput
	if json.Unmarshal(ev.RawPayload, &input) == nil {
		c := core.ContentInput{
			OrganizationID: ev.OrganizationID,
			ProjectID:      uuid.UUID(ev.ProjectID.Bytes).String(),
			EnvironmentID:  uuid.UUID(ev.EnvironmentID.Bytes).String(),
			ReceivedAt:     ev.ReceivedAt.Time,
			Event:          input,
		}
		content = &c
	}

	vr := core.VerifyEvent(core.Record{
		PreviousEventHash: ev.PreviousEventHash,
		PayloadHash:       ev.PayloadHash,
		EventHash:         ev.EventHash,
		ChainPosition:     ev.ChainPosition,
		Content:           content,
	})

	return &Result{
		Valid: vr.Valid,
		State: StateSealed,
		Checks: Checks{
			PayloadHashMatches: vr.Checks.PayloadHashMatches,
			EventHashMatches:   vr.Checks.EventHashMatches,
			LinksToPrevious:    vr.Checks.LinksToPrevious,
		},
		Computed: vr.Computed,
		Stored: map[string]any{
			"id":                  uuid.UUID(ev.ID.Bytes).String(),
			"chain_id":            uuid.UUID(ev.ChainID.Bytes).String(),
			"chain_position":      ev.ChainPosition,
			"event_hash":          ev.EventHash,
			"payload_hash":        ev.PayloadHash,
			"previous_event_hash": ev.PreviousEventHash,
			"event_type":          ev.EventType,
			"received_at":         ev.ReceivedAt.Time,
		},
	}
}

func (s *Service) verifyPending(p queries.PendingEvent) (*Result, error) {
	var env struct {
		ReceivedAt time.Time       `json:"received_at"`
		Input      spec.EventInput `json:"input"`
	}
	if err := json.Unmarshal(p.EventData, &env); err != nil {
		return nil, err
	}

	recomputed, err := core.PayloadHash(core.BuildContentPayload(core.ContentInput{
		OrganizationID: p.OrganizationID,
		ProjectID:      uuid.UUID(p.ProjectID.Bytes).String(),
		EnvironmentID:  uuid.UUID(p.EnvironmentID.Bytes).String(),
		ReceivedAt:     env.ReceivedAt,
		Event:          env.Input,
	}))
	if err != nil {
		return nil, err
	}

	checks := Checks{
		PayloadHashMatches: recomputed == p.PayloadHash,
		EventHashMatches:   false, // no chain position yet
		LinksToPrevious:    false, // no chain link yet
	}

	return &Result{
		Valid:    checks.PayloadHashMatches,
		State:    StatePending,
		Checks:   checks,
		Computed: map[string]string{"payload_hash": recomputed},
		Stored: map[string]any{
			"id":             uuid.UUID(p.ID.Bytes).String(),
			"payload_hash":   p.PayloadHash,
			"accepted_at":    p.AcceptedAt.Time,
			"shard_index":    p.ShardIndex,
			"event_type":     env.Input.EventType,
			"environment_id": uuid.UUID(p.EnvironmentID.Bytes).String(),
		},
	}, nil
}

func parseUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, errors.New("empty uuid")
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}
