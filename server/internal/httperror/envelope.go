// Package httperror standardises the JSON error envelope every endpoint
// returns on failure. Format matches the Hono predecessor so clients don't
// need to switch shapes during cutover:
//
//	{"error": {"code": "...", "message": "...", "request_id": "...", "details": {...}}}
package httperror

import (
	"encoding/json"
	"net/http"

	"github.com/stonewrit/stonewrit/server/internal/middleware"
)

type Envelope struct {
	Error Body `json:"error"`
}

type Body struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}

func Write(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	WriteWithDetails(w, r, status, code, message, nil)
}

func WriteWithDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Error: Body{
			Code:      code,
			Message:   message,
			RequestID: middleware.RequestIDFrom(r.Context()),
			Details:   details,
		},
	})
}
