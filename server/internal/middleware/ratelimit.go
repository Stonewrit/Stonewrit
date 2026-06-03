package middleware

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/ratelimit"
)

type RateLimit struct {
	Limiter *ratelimit.Limiter
	Logger  zerolog.Logger
}

func (m *RateLimit) Check() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, ok := auth.FromContext(r.Context())
			if !ok {
				writeError(w, r, http.StatusInternalServerError, "internal_error",
					"rate-limit middleware requires upstream auth context")
				return
			}

			var apiKeyUUID pgtype.UUID
			if err := apiKeyUUID.Scan(ac.APIKeyMetadataID); err != nil {
				writeError(w, r, http.StatusInternalServerError, "internal_error",
					"could not parse api key id")
				return
			}

			decision, err := m.Limiter.Check(r.Context(), ac.OrganizationID, apiKeyUUID)
			if err != nil {
				m.Logger.Error().
					Str("request_id", RequestIDFrom(r.Context())).
					Err(err).
					Msg("rate limit check failed")
				// Fail open on transient errors - refusing all traffic on a
				// Postgres blip is worse than briefly running uncapped.
				next.ServeHTTP(w, r)
				return
			}

			setLimitHeaders(w, decision)

			if !decision.Allowed {
				retryAfterSec := (decision.RetryAfterMs + 999) / 1000
				if retryAfterSec < 1 {
					retryAfterSec = 1
				}
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSec, 10))
				writeError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded",
					"Rate limit exceeded ("+decision.DeniedReason+" window).")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setLimitHeaders(w http.ResponseWriter, d *ratelimit.Decision) {
	w.Header().Set("X-RateLimit-Limit-Burst", strconv.Itoa(int(d.Limits.Burst.Max)))
	w.Header().Set("X-RateLimit-Remaining-Burst", strconv.Itoa(int(d.BurstRemaining)))
	w.Header().Set("X-RateLimit-Limit-Sustained", strconv.Itoa(int(d.Limits.Sustained.Max)))
	w.Header().Set("X-RateLimit-Remaining-Sustained", strconv.Itoa(int(d.SustainedRemaining)))
	w.Header().Set("X-RateLimit-Tier", string(d.Tier))
}
