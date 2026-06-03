package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/httperror"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/internal/verify"
)

// EventsRead bundles the read endpoints (GET /events/:id, /verify, /chain).
// Lives alongside Events so the same router group can mount both write
// and read paths under the same auth + rate-limit chain.
type EventsRead struct {
	Q        *queries.Queries
	Verifier *verify.Service
}

func (h *EventsRead) Mount(r chi.Router) {
	r.Get("/events/{eventId}", h.Get)
	r.Get("/events/{eventId}/verify", h.Verify)
	r.Get("/events/{eventId}/chain", h.ChainNeighbors)
}

func (h *EventsRead) Get(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}
	eventUUID, err := parsePathUUID(r, "eventId")
	if err != nil {
		httperror.Write(w, r, http.StatusBadRequest, "invalid_event_id", "event id is not a uuid")
		return
	}

	// Sealed first
	ev, err := h.Q.GetEventForOrg(r.Context(), queries.GetEventForOrgParams{
		ID:             eventUUID,
		OrganizationID: ac.OrganizationID,
	})
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"event":  sealedEventJSON(ev),
			"status": "sealed",
		})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Pending fallback
	pe, err := h.Q.GetPendingEventForOrg(r.Context(), queries.GetPendingEventForOrgParams{
		ID:             eventUUID,
		OrganizationID: ac.OrganizationID,
	})
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"event":  pendingEventJSON(pe),
			"status": "pending",
		})
		return
	}

	httperror.Write(w, r, http.StatusNotFound, "event_not_found", "event not found in this organization")
}

func (h *EventsRead) Verify(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}
	eventID := chi.URLParam(r, "eventId")

	result, err := h.Verifier.VerifyEvent(r.Context(), ac.OrganizationID, eventID)
	if err != nil {
		if errors.Is(err, verify.ErrNotFound) {
			httperror.Write(w, r, http.StatusNotFound, "event_not_found", "event not found")
			return
		}
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *EventsRead) ChainNeighbors(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}
	eventUUID, err := parsePathUUID(r, "eventId")
	if err != nil {
		httperror.Write(w, r, http.StatusBadRequest, "invalid_event_id", "event id is not a uuid")
		return
	}

	ev, err := h.Q.GetEventForOrg(r.Context(), queries.GetEventForOrgParams{
		ID:             eventUUID,
		OrganizationID: ac.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httperror.Write(w, r, http.StatusNotFound, "event_not_found", "event not found")
			return
		}
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	prevPos := ev.ChainPosition - 1
	nextPos := ev.ChainPosition + 1
	neighbors, err := h.Q.GetEventNeighbors(r.Context(), queries.GetEventNeighborsParams{
		ChainID: ev.ChainID,
		Column2: []int64{prevPos, nextPos},
	})
	if err != nil {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	var prev, next any
	for _, n := range neighbors {
		jn := neighborJSON(n)
		if n.ChainPosition == prevPos {
			prev = jn
		} else if n.ChainPosition == nextPos {
			next = jn
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"chain_id":       uuid.UUID(ev.ChainID.Bytes).String(),
		"chain_position": ev.ChainPosition,
		"previous":       prev,
		"next":           next,
	})
}

func sealedEventJSON(ev queries.Event) map[string]any {
	var rawPayload any
	_ = json.Unmarshal(ev.RawPayload, &rawPayload)
	out := map[string]any{
		"id":                       uuid.UUID(ev.ID.Bytes).String(),
		"organization_id":          ev.OrganizationID,
		"project_id":               uuid.UUID(ev.ProjectID.Bytes).String(),
		"environment_id":           uuid.UUID(ev.EnvironmentID.Bytes).String(),
		"event_type":               ev.EventType,
		"occurred_at":              ev.OccurredAt.Time,
		"received_at":              ev.ReceivedAt.Time,
		"actor_type":               ev.ActorType,
		"action_name":              ev.ActionName,
		"action_result":            ev.ActionResult,
		"resource_type":            ev.ResourceType,
		"data_classes":             ev.DataClasses,
		"chain_id":                 uuid.UUID(ev.ChainID.Bytes).String(),
		"chain_position":           ev.ChainPosition,
		"payload_hash":             ev.PayloadHash,
		"previous_event_hash":      ev.PreviousEventHash,
		"event_hash":               ev.EventHash,
		"hash_algorithm":           ev.HashAlgorithm,
		"canonicalization_version": ev.CanonicalizationVersion,
		"sealing_status":           ev.SealingStatus,
		"classification_status":    ev.ClassificationStatus,
		"raw_payload":              rawPayload,
	}
	// Observe-only agent scope check, present only for ai_agent events.
	if len(ev.ScopeCheckResult) > 0 {
		var sc any
		if json.Unmarshal(ev.ScopeCheckResult, &sc) == nil {
			out["scope_check"] = sc
		}
	}
	return out
}

func pendingEventJSON(p queries.PendingEvent) map[string]any {
	var eventData any
	_ = json.Unmarshal(p.EventData, &eventData)
	return map[string]any{
		"id":             uuid.UUID(p.ID.Bytes).String(),
		"environment_id": uuid.UUID(p.EnvironmentID.Bytes).String(),
		"shard_index":    p.ShardIndex,
		"payload_hash":   p.PayloadHash,
		"accepted_at":    p.AcceptedAt.Time,
		"event_data":     eventData,
	}
}

func neighborJSON(n queries.GetEventNeighborsRow) map[string]any {
	return map[string]any{
		"id":                  uuid.UUID(n.ID.Bytes).String(),
		"chain_position":      n.ChainPosition,
		"event_hash":          n.EventHash,
		"previous_event_hash": n.PreviousEventHash,
		"payload_hash":        n.PayloadHash,
		"event_type":          n.EventType,
		"received_at":         n.ReceivedAt.Time,
	}
}

func parsePathUUID(r *http.Request, key string) (pgtype.UUID, error) {
	s := chi.URLParam(r, key)
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}
