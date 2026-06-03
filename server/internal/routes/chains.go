package routes

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stonewrit/stonewrit/core"
	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/httperror"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

// ChainVerifySyncLimit caps in-line chain walks. Larger chains require
// the (future) async verify worker. Matches Hono's CHAIN_VERIFY_SYNC_LIMIT.
const ChainVerifySyncLimit = 10_000

type Chains struct {
	Q *queries.Queries
}

func (h *Chains) Mount(r chi.Router) {
	r.Get("/chains", h.List)
	r.Get("/chains/{chainId}", h.Get)
	r.Get("/chains/{chainId}/verify", h.Verify)
}

func (h *Chains) List(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}

	var projectFilter pgtype.UUID
	if q := r.URL.Query().Get("project_id"); q != "" {
		u, err := uuid.Parse(q)
		if err != nil {
			httperror.Write(w, r, http.StatusBadRequest, "invalid_project_id", "project_id is not a uuid")
			return
		}
		projectFilter = pgtype.UUID{Bytes: u, Valid: true}
	}

	rows, err := h.Q.ListChainsForOrg(r.Context(), queries.ListChainsForOrgParams{
		OrganizationID: ac.OrganizationID,
		ProjectID:      projectFilter,
	})
	if err != nil {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		out = append(out, chainJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"chains": out})
}

func (h *Chains) Get(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}
	chainUUID, err := parsePathUUID(r, "chainId")
	if err != nil {
		httperror.Write(w, r, http.StatusBadRequest, "invalid_chain_id", "chain id is not a uuid")
		return
	}

	c, err := h.Q.GetChainForOrg(r.Context(), queries.GetChainForOrgParams{
		ID:             chainUUID,
		OrganizationID: ac.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httperror.Write(w, r, http.StatusNotFound, "chain_not_found", "chain not found")
			return
		}
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"chain": chainGetJSON(c)})
}

func (h *Chains) Verify(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}
	chainUUID, err := parsePathUUID(r, "chainId")
	if err != nil {
		httperror.Write(w, r, http.StatusBadRequest, "invalid_chain_id", "chain id is not a uuid")
		return
	}

	c, err := h.Q.GetChainForOrg(r.Context(), queries.GetChainForOrgParams{
		ID:             chainUUID,
		OrganizationID: ac.OrganizationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httperror.Write(w, r, http.StatusNotFound, "chain_not_found", "chain not found")
			return
		}
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if c.LatestPosition > ChainVerifySyncLimit {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"valid":           false,
			"too_large":       true,
			"chain_id":        uuid.UUID(c.ID.Bytes).String(),
			"latest_position": c.LatestPosition,
			"reason":          "chain exceeds synchronous verify limit; use async verify worker",
		})
		return
	}

	events, err := h.Q.WalkChainForVerify(r.Context(), queries.WalkChainForVerifyParams{
		ChainID: c.ID,
		Limit:   ChainVerifySyncLimit,
	})
	if err != nil {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	prev := ""
	verified := 0
	var brokenAt *map[string]any
	for _, ev := range events {
		expectPrev := ""
		if ev.PreviousEventHash != nil {
			expectPrev = *ev.PreviousEventHash
		}
		if expectPrev != prev {
			breakInfo := map[string]any{
				"event_id":      uuid.UUID(ev.ID.Bytes).String(),
				"position":      ev.ChainPosition,
				"reason":        "previous_event_hash does not match running tip",
				"expected_prev": prev,
				"stored_prev":   expectPrev,
			}
			brokenAt = &breakInfo
			break
		}
		recomputed := core.EventHash(prev, ev.PayloadHash, ev.ChainPosition)
		if recomputed != ev.EventHash {
			breakInfo := map[string]any{
				"event_id":   uuid.UUID(ev.ID.Bytes).String(),
				"position":   ev.ChainPosition,
				"reason":     "event_hash mismatch",
				"recomputed": recomputed,
				"stored":     ev.EventHash,
			}
			brokenAt = &breakInfo
			break
		}
		prev = ev.EventHash
		verified++
	}

	// Tip sanity
	var tipOK bool
	if brokenAt == nil && len(events) > 0 {
		last := events[len(events)-1]
		if c.LatestEventHash != nil {
			tipOK = *c.LatestEventHash == last.EventHash
		} else {
			tipOK = false
		}
	}

	// A chain with NO events is valid ONLY if it's genuinely empty (position 0,
	// no tip hash). A non-zero tip with zero events means the events were
	// deleted or the tip was forged - that's tampering, not "valid".
	emptyChainConsistent := len(events) == 0 &&
		c.LatestPosition == 0 && c.LatestEventHash == nil

	resp := map[string]any{
		"valid": brokenAt == nil &&
			(emptyChainConsistent || (len(events) > 0 && tipOK)),
		"chain_id":        uuid.UUID(c.ID.Bytes).String(),
		"events_verified": verified,
		"latest_position": c.LatestPosition,
		"tip_matches":     tipOK,
	}
	if brokenAt != nil {
		resp["break"] = *brokenAt
	}
	writeJSON(w, http.StatusOK, resp)
}

func chainJSON(c queries.ListChainsForOrgRow) map[string]any {
	return map[string]any{
		"id":              uuid.UUID(c.ID.Bytes).String(),
		"organization_id": c.OrganizationID,
		"project_id":      uuid.UUID(c.ProjectID.Bytes).String(),
		"environment_id":  uuid.UUID(c.EnvironmentID.Bytes).String(),
		// shard_index is an internal write-throughput detail - never exposed.
		"name":              c.Name,
		"status":            c.Status,
		"latest_position":   c.LatestPosition,
		"latest_event_hash": c.LatestEventHash,
		"created_at":        c.CreatedAt.Time,
		"updated_at":        c.UpdatedAt.Time,
	}
}

func chainGetJSON(c queries.GetChainForOrgRow) map[string]any {
	return map[string]any{
		"id":              uuid.UUID(c.ID.Bytes).String(),
		"organization_id": c.OrganizationID,
		"project_id":      uuid.UUID(c.ProjectID.Bytes).String(),
		"environment_id":  uuid.UUID(c.EnvironmentID.Bytes).String(),
		// shard_index is an internal write-throughput detail - never exposed.
		"name":              c.Name,
		"status":            c.Status,
		"latest_position":   c.LatestPosition,
		"latest_event_hash": c.LatestEventHash,
		"created_at":        c.CreatedAt.Time,
		"updated_at":        c.UpdatedAt.Time,
	}
}
