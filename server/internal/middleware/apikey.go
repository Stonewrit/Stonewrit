package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/server/internal/auth"
)

type APIKey struct {
	Verifier *auth.Verifier
	Logger   zerolog.Logger
}

// Require returns middleware that enforces a valid bearer token and the
// given scope set. Required scopes must be a subset of the granted scopes
// (AND logic), matching require-api-key.ts.
func (m *APIKey) Require(requiredScopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			plaintext, ok := bearerFrom(r)
			if !ok {
				writeError(w, r, http.StatusUnauthorized, "invalid_api_key",
					"Missing or malformed Authorization header.")
				return
			}

			authCtx, err := m.Verifier.Verify(r.Context(), plaintext)
			if err != nil {
				status, code, msg := classifyAuthError(err)
				m.Logger.Debug().
					Str("request_id", RequestIDFrom(r.Context())).
					Err(err).
					Msg("api key verification failed")
				writeError(w, r, status, code, msg)
				return
			}

			for _, scope := range requiredScopes {
				if !authCtx.HasScope(scope) {
					writeError(w, r, http.StatusForbidden, "insufficient_scope",
						"Required scope '"+scope+"' not granted to this API key.")
					return
				}
			}

			ctx := auth.WithContext(r.Context(), authCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerFrom(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

func classifyAuthError(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, auth.ErrInvalidKey):
		return http.StatusUnauthorized, "invalid_api_key", "API key invalid or revoked."
	case errors.Is(err, auth.ErrKeyExpired):
		return http.StatusUnauthorized, "invalid_api_key", "API key has expired."
	case errors.Is(err, auth.ErrKeyDisabled):
		return http.StatusUnauthorized, "invalid_api_key", "API key is disabled."
	case errors.Is(err, auth.ErrKeyNoEnvironment):
		return http.StatusForbidden, "invalid_api_key", "API key is not bound to an environment."
	default:
		return http.StatusInternalServerError, "internal_error", "An internal error occurred."
	}
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{
		Error: errorBody{
			Code:      code,
			Message:   message,
			RequestID: RequestIDFrom(r.Context()),
		},
	})
}
