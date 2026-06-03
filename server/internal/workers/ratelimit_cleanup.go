// Package workers houses the background goroutines that run alongside
// the HTTP server: rate-limit table sweeper, evidence export builder, etc.
package workers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

type RateLimitCleanup struct {
	Q        *queries.Queries
	Log      zerolog.Logger
	Interval time.Duration // default 15m
	StaleAge time.Duration // default 2h
}

func (w *RateLimitCleanup) defaults() {
	if w.Interval == 0 {
		w.Interval = 15 * time.Minute
	}
	if w.StaleAge == 0 {
		w.StaleAge = 2 * time.Hour
	}
}

func (w *RateLimitCleanup) Run(ctx context.Context) error {
	w.defaults()
	w.Log.Info().
		Dur("interval", w.Interval).
		Dur("stale_age", w.StaleAge).
		Msg("rate_limit_cleanup worker running")

	// Run immediately on boot so a fresh process doesn't wait 15 minutes
	// before its first sweep.
	w.sweep(ctx)

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

func (w *RateLimitCleanup) sweep(ctx context.Context) {
	cutoff := time.Now().Add(-w.StaleAge)
	if err := w.Q.DeleteStaleRateLimitBuckets(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true}); err != nil {
		w.Log.Error().Err(err).Msg("rate_limit_cleanup sweep failed")
	}
}
