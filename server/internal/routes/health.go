package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Health struct {
	Pool *pgxpool.Pool
}

func (h *Health) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "stonewrit-api-go",
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (h *Health) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (h *Health) DBPing(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	start := time.Now()
	var n int
	err := h.Pool.QueryRow(ctx, "select 1").Scan(&n)
	latency := time.Since(start)

	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"db":         "down",
			"error":      err.Error(),
			"latency_ms": latency.Milliseconds(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"db":         "ok",
		"latency_ms": latency.Milliseconds(),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
