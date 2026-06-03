// Package ingest carries the synchronous accept path for events.
//
// The accept path is intentionally minimal: validate, hash the content,
// classify, compute control mappings, write the row to pending_events,
// return 202 with a content-hash receipt. The sealer worker later drains
// pending into events and computes the chain hash.
//
// Target: under 2ms p99 for the accept call, a single Postgres round-trip in
// the steady state (one INSERT into pending_events, plus a cached idempotency
// lookup).
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stonewrit/stonewrit/classify"
	"github.com/stonewrit/stonewrit/core"
	"github.com/stonewrit/stonewrit/server/internal/auth"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/internal/resolvecontrols"
	"github.com/stonewrit/stonewrit/server/internal/shard"
	"github.com/stonewrit/stonewrit/spec"
)

type Service struct {
	Q          *queries.Queries
	ShardCount int
}

type AcceptInput struct {
	Auth           *auth.Context
	Event          spec.EventInput
	IdempotencyKey string
	RequestID      string
}

type AcceptResult struct {
	// Status + Body are what the handler writes verbatim - identical bytes
	// whether this is a fresh accept or an idempotent replay, so the two
	// paths can't diverge.
	Status int
	Body   []byte
	Cached bool

	// Structured fields for the fresh-accept path (batch handler + tests).
	ID          uuid.UUID
	PayloadHash string
	AcceptedAt  time.Time
	ShardIndex  int
}

// Accept is the hot path. Returns a Cached result if the idempotency key
// is recognized; otherwise classifies, hashes, persists to pending_events.
func (s *Service) Accept(ctx context.Context, in AcceptInput) (*AcceptResult, error) {
	// Route by external_event_id when present so two events sharing one (a
	// client retry) ALWAYS land on the same shard - the sealer's per-shard
	// dedup then catches the duplicate before the COPY. Routing duplicates to
	// different shards is what could make a shard wedge forever on the
	// (external_event_id, environment_id) unique constraint. Events with no
	// external id fall back to actor.id (intra-actor ordering) then random.
	shardKey := in.Event.ExternalEventID
	if shardKey == "" {
		shardKey = in.Event.Actor.ID
	}
	shardIdx := shard.IndexFor(shardKey, s.ShardCount)
	// Millisecond precision so the value stored in the journal (and later
	// re-hashed during verify, including by JS clients) is byte-identical to
	// what we hash here. See core.HashTime for the rationale.
	receivedAt := time.Now().UTC().Truncate(time.Millisecond)

	payloadHash, err := core.PayloadHash(core.BuildContentPayload(core.ContentInput{
		OrganizationID: in.Auth.OrganizationID,
		ProjectID:      in.Auth.ProjectID,
		EnvironmentID:  in.Auth.EnvironmentID,
		ReceivedAt:     receivedAt,
		Event:          in.Event,
	}))
	if err != nil {
		return nil, fmt.Errorf("payload hash: %w", err)
	}

	cl := classify.Classify(classify.Event{
		EventType:    in.Event.EventType,
		ActorType:    in.Event.Actor.Type,
		ActionResult: in.Event.Action.Result,
		DataClasses:  in.Event.Resource.Classification,
	})

	var customerCtl []string
	if in.Event.Policy != nil {
		customerCtl = in.Event.Policy.ControlIDs
	}
	proposed := classify.Compute(classify.MapInput{
		EventType: in.Event.EventType,
		// Classifier output (resource classification plus derived classes). This
		// is what data-class-conditional rules see.
		DataClasses:        cl.DataClasses,
		ActorType:          in.Event.Actor.Type,
		ActionResult:       in.Event.Action.Result,
		CustomerControlIDs: customerCtl,
	})
	resolved, err := resolvecontrols.ResolveAgainstCatalog(ctx, s.Q, proposed)
	if err != nil {
		return nil, fmt.Errorf("resolve controls: %w", err)
	}

	eventDataJSON, err := json.Marshal(eventDataEnvelope{
		ReceivedAt: receivedAt,
		Input:      in.Event,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal event data: %w", err)
	}
	classificationJSON, err := json.Marshal(cl)
	if err != nil {
		return nil, err
	}
	mappingsJSON, err := json.Marshal(resolved)
	if err != nil {
		return nil, err
	}

	eventID := uuid.New()

	projectUUID, err := parsePgUUID(in.Auth.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("parse project uuid: %w", err)
	}
	envUUID, err := parsePgUUID(in.Auth.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("parse env uuid: %w", err)
	}
	apiKeyUUID, err := parsePgUUID(in.Auth.APIKeyID)
	if err != nil {
		return nil, fmt.Errorf("parse api key uuid: %w", err)
	}

	// Observe-only agent scope check. Computed here (after the content hash at
	// L86, so it CANNOT affect payload_hash), stored in pending_events.scope_check,
	// and copied forward to events.scope_check_result by the sealer. nil for
	// non-agent events. A registry-lookup failure is swallowed - scope_check is
	// best-effort and must never block ingest (also keeps an agents-table-less
	// open-core server ingesting normally).
	var scopeCheckJSON []byte
	if scopeCheck, scErr := ComputeScopeCheck(
		ctx, s.Q, in.Auth.OrganizationID, projectUUID, in.Event.Actor, in.Event.Action,
	); scErr == nil && scopeCheck != nil {
		scopeCheckJSON, err = json.Marshal(scopeCheck)
		if err != nil {
			return nil, fmt.Errorf("marshal scope_check: %w", err)
		}
	}

	// Build the exact 202 body now so the same bytes are returned on a fresh
	// accept AND cached for idempotent replays.
	body, err := json.Marshal(spec.AcceptedResponse{
		ID:            eventID.String(),
		Status:        "pending",
		PayloadHash:   payloadHash,
		AcceptedAt:    receivedAt,
		EnvironmentID: in.Auth.EnvironmentID,
		ShardIndex:    shardIdx,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal accepted response: %w", err)
	}

	// Atomic idempotency gate. The first writer for (org, key) claims the
	// row; concurrent racers hit the unique constraint, get no row back, and
	// return the winner's cached response - so N parallel same-key POSTs
	// collapse to a single event instead of racing past a non-atomic SELECT.
	if in.IdempotencyKey != "" {
		requestHash, err := core.PayloadHash(in.Event)
		if err != nil {
			return nil, fmt.Errorf("request hash: %w", err)
		}
		statusInt32 := int32(http.StatusAccepted)
		expires := time.Now().Add(24 * time.Hour)

		_, claimErr := s.Q.ClaimIdempotencyKey(ctx, queries.ClaimIdempotencyKeyParams{
			OrganizationID: in.Auth.OrganizationID,
			ProjectID:      projectUUID,
			EnvironmentID:  envUUID,
			ApiKeyID:       apiKeyUUID,
			Key:            in.IdempotencyKey,
			RequestHash:    requestHash,
			ResponseBody:   body,
			StatusCode:     &statusInt32,
			ExpiresAt:      pgtype.Timestamptz{Time: expires, Valid: true},
		})
		if errors.Is(claimErr, pgx.ErrNoRows) {
			// Lost the race (or a genuine replay): the winner already wrote
			// the row. Return its cached response verbatim.
			cached, getErr := s.Q.GetIdempotentResponse(ctx, queries.GetIdempotentResponseParams{
				OrganizationID: in.Auth.OrganizationID,
				Key:            in.IdempotencyKey,
			})
			if getErr != nil {
				return nil, fmt.Errorf("idempotency replay lookup: %w", getErr)
			}
			status := http.StatusAccepted
			if cached.StatusCode != nil {
				status = int(*cached.StatusCode)
			}
			return &AcceptResult{Cached: true, Status: status, Body: cached.ResponseBody}, nil
		}
		if claimErr != nil {
			return nil, fmt.Errorf("idempotency claim: %w", claimErr)
		}
		// Won the claim - fall through and create the event.
	}

	if err := s.Q.InsertPendingEvent(ctx, queries.InsertPendingEventParams{
		ID:              pgtype.UUID{Bytes: eventID, Valid: true},
		OrganizationID:  in.Auth.OrganizationID,
		ProjectID:       projectUUID,
		EnvironmentID:   envUUID,
		ShardIndex:      int32(shardIdx),
		ExternalEventID: nullableText(in.Event.ExternalEventID),
		PayloadHash:     payloadHash,
		EventData:       eventDataJSON,
		Classification:  classificationJSON,
		ControlMappings: mappingsJSON,
		ScopeCheck:      scopeCheckJSON,
		ApiKeyID:        apiKeyUUID,
		IdempotencyKey:  nullableText(in.IdempotencyKey),
	}); err != nil {
		return nil, fmt.Errorf("insert pending: %w", err)
	}

	return &AcceptResult{
		Status:      http.StatusAccepted,
		Body:        body,
		ID:          eventID,
		PayloadHash: payloadHash,
		AcceptedAt:  receivedAt,
		ShardIndex:  shardIdx,
	}, nil
}

type eventDataEnvelope struct {
	ReceivedAt time.Time       `json:"received_at"`
	Input      spec.EventInput `json:"input"`
}
