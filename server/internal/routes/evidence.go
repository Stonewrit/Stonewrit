package routes

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/httperror"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

type Evidence struct {
	Q *queries.Queries
}

func (h *Evidence) Mount(r chi.Router) {
	r.Get("/evidence/summary", h.Summary)
	r.Get("/evidence/events/{eventId}", h.EventMappings)
}

func (h *Evidence) Summary(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", "missing auth")
		return
	}

	q := r.URL.Query()
	framework := strings.ToUpper(q.Get("framework"))
	projectIDStr := q.Get("project_id")
	fromStr := q.Get("from")
	toStr := q.Get("to")

	if framework == "" || projectIDStr == "" {
		httperror.Write(w, r, http.StatusBadRequest, "missing_params", "framework and project_id are required")
		return
	}

	projectU, err := uuid.Parse(projectIDStr)
	if err != nil {
		httperror.Write(w, r, http.StatusBadRequest, "invalid_project_id", "project_id is not a uuid")
		return
	}
	projectID := pgtype.UUID{Bytes: projectU, Valid: true}

	now := time.Now().UTC()
	from := now.AddDate(0, -1, 0)
	to := now
	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	fromTs := pgtype.Timestamptz{Time: from, Valid: true}
	toTs := pgtype.Timestamptz{Time: to, Valid: true}

	// Framework existence
	fw, err := h.Q.GetFramework(r.Context(), framework)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httperror.Write(w, r, http.StatusNotFound, "framework_not_found", "unknown framework")
			return
		}
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	type totals struct {
		Total      int64
		Privileged int64
		AIAgent    int64
		OutOfScope int64
		Denied     int64
		ExtShare   int64
		OpenExc    int64
	}
	var t totals
	var perControl []queries.GetEvidenceCountsPerControlRow
	var catalog []queries.ListControlsForFrameworkRow

	g, gctx := errgroup.WithContext(r.Context())

	g.Go(func() error {
		n, err := h.Q.CountEventsInWindow(gctx, queries.CountEventsInWindowParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		t.Total = n
		return err
	})
	g.Go(func() error {
		n, err := h.Q.CountPrivilegedEventsInWindow(gctx, queries.CountPrivilegedEventsInWindowParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		t.Privileged = n
		return err
	})
	g.Go(func() error {
		n, err := h.Q.CountAIAgentEventsInWindow(gctx, queries.CountAIAgentEventsInWindowParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		t.AIAgent = n
		return err
	})
	g.Go(func() error {
		n, err := h.Q.CountOutOfScopeAgentActionsInWindow(gctx, queries.CountOutOfScopeAgentActionsInWindowParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		t.OutOfScope = n
		return err
	})
	g.Go(func() error {
		n, err := h.Q.CountPolicyDenialsInWindow(gctx, queries.CountPolicyDenialsInWindowParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		t.Denied = n
		return err
	})
	g.Go(func() error {
		n, err := h.Q.CountExternalSharesInWindow(gctx, queries.CountExternalSharesInWindowParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		t.ExtShare = n
		return err
	})
	g.Go(func() error {
		n, err := h.Q.CountOpenExceptions(gctx, queries.CountOpenExceptionsParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID,
		})
		t.OpenExc = n
		return err
	})
	g.Go(func() error {
		rows, err := h.Q.GetEvidenceCountsPerControl(gctx, queries.GetEvidenceCountsPerControlParams{
			OrganizationID: ac.OrganizationID, ProjectID: projectID, Framework: framework,
			ReceivedAt: fromTs, ReceivedAt_2: toTs,
		})
		perControl = rows
		return err
	})
	g.Go(func() error {
		rows, err := h.Q.ListControlsForFramework(gctx, framework)
		catalog = rows
		return err
	})

	if err := g.Wait(); err != nil {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	counts := make(map[string]int64, len(perControl))
	for _, c := range perControl {
		counts[c.Framework+":"+c.ControlID] = c.EvidenceCount
	}
	controls := make([]map[string]any, 0, len(catalog))
	for _, c := range catalog {
		controls = append(controls, map[string]any{
			"control_id":     c.ControlID,
			"title":          c.Title,
			"description":    c.Description,
			"category":       c.Category,
			"evidence_count": counts[c.Framework+":"+c.ControlID],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"framework":  fw.ID,
		"name":       fw.Name,
		"project_id": projectIDStr,
		"period":     map[string]string{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)},
		"totals": map[string]int64{
			"total_events":                    t.Total,
			"privileged_access_count":         t.Privileged,
			"ai_agent_action_count":           t.AIAgent,
			"out_of_scope_agent_action_count": t.OutOfScope,
			"policy_denial_count":             t.Denied,
			"external_data_sharing_count":     t.ExtShare,
			"open_exceptions":                 t.OpenExc,
		},
		"controls": controls,
	})
}

func (h *Evidence) EventMappings(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.Q.GetEventControlMappings(r.Context(), queries.GetEventControlMappingsParams{
		OrganizationID: ac.OrganizationID,
		EventID:        eventUUID,
	})
	if err != nil {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	mappings := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		mappings = append(mappings, map[string]any{
			"framework":      m.Framework,
			"control_id":     m.ControlID,
			"mapping_reason": m.MappingReason,
			"confidence":     m.Confidence,
			"status":         m.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event_id": chi.URLParam(r, "eventId"),
		"mappings": mappings,
	})
}
