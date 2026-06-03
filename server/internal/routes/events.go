package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/httperror"
	"github.com/stonewrit/stonewrit/server/internal/ingest"
	"github.com/stonewrit/stonewrit/spec"
)

type Events struct {
	Ingest *ingest.Service
	Log    zerolog.Logger
	Valid  *validator.Validate
}

// NewEvents builds the events handler with a singleton validator.
func NewEvents(ing *ingest.Service, log zerolog.Logger) *Events {
	return &Events{
		Ingest: ing,
		Log:    log,
		Valid:  validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *Events) Mount(r chi.Router) {
	r.Post("/events", h.PostOne)
	r.Post("/events/batch", h.PostBatch)
}

func (h *Events) PostOne(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error",
			"missing auth context")
		return
	}

	var input spec.EventInput
	if !decodeBody(w, r, &input) {
		return
	}
	if err := h.Valid.Struct(&input); err != nil {
		httperror.WriteWithDetails(w, r, http.StatusUnprocessableEntity,
			"validation_failed", "Event input failed validation.", validatorDetails(err))
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	in := ingest.AcceptInput{
		Auth:           ac,
		Event:          input,
		IdempotencyKey: idempotencyKey,
	}
	result, err := h.Ingest.Accept(r.Context(), in)
	if err != nil {
		h.writeAcceptError(w, r, err)
		return
	}

	// Accept builds the exact bytes (fresh or idempotent-cached) so both
	// paths return identical responses.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
}

func (h *Events) PostBatch(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok {
		httperror.Write(w, r, http.StatusInternalServerError, "internal_error",
			"missing auth context")
		return
	}

	var req spec.BatchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.Valid.Struct(&req); err != nil {
		httperror.WriteWithDetails(w, r, http.StatusUnprocessableEntity,
			"validation_failed", "Batch input failed validation.", validatorDetails(err))
		return
	}

	// Per-event idempotency keys aren't supported in batch (same Hono behavior).
	results := make([]spec.BatchEventResult, 0, len(req.Events))
	accepted := 0
	rejected := 0

	for i, ev := range req.Events {
		in := ingest.AcceptInput{
			Auth:  ac,
			Event: ev,
		}
		res, err := h.Ingest.Accept(r.Context(), in)
		if err != nil {
			rejected++
			results = append(results, spec.BatchEventResult{
				Status: "rejected",
				Index:  i,
				Error: &spec.BatchEventError{
					Code:    "ingest_failed",
					Message: err.Error(),
				},
			})
			continue
		}
		accepted++
		results = append(results, spec.BatchEventResult{
			Status: "accepted",
			Index:  i,
			Response: &spec.AcceptedResponse{
				ID:            res.ID.String(),
				Status:        "pending",
				PayloadHash:   res.PayloadHash,
				AcceptedAt:    res.AcceptedAt,
				EnvironmentID: ac.EnvironmentID,
				ShardIndex:    res.ShardIndex,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMultiStatus)
	_ = json.NewEncoder(w).Encode(spec.BatchAcceptedResponse{
		Accepted: accepted,
		Rejected: rejected,
		Results:  results,
	})
}

func (h *Events) writeAcceptError(w http.ResponseWriter, r *http.Request, err error) {
	httperror.Write(w, r, http.StatusInternalServerError, "ingest_failed", err.Error())
}

func validatorDetails(err error) any {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return err.Error()
	}
	details := make([]map[string]string, 0, len(verrs))
	for _, fe := range verrs {
		details = append(details, map[string]string{
			"field": fe.Namespace(),
			"rule":  fe.Tag(),
			"got":   fe.Param(),
		})
	}
	return details
}
