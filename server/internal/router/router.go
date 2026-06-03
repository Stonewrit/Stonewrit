package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/config"
	"github.com/stonewrit/stonewrit/server/internal/ingest"
	mw "github.com/stonewrit/stonewrit/server/internal/middleware"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/internal/quota"
	"github.com/stonewrit/stonewrit/server/internal/ratelimit"
	"github.com/stonewrit/stonewrit/server/internal/routes"
	"github.com/stonewrit/stonewrit/server/internal/verify"
)

type Deps struct {
	Pool       *pgxpool.Pool
	Queries    *queries.Queries
	Log        zerolog.Logger
	ShardCount int
	Billing    config.BillingEnv
}

func New(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(mw.RequestID)
	r.Use(mw.AccessLog(d.Log))
	r.Use(mw.Recoverer(d.Log))

	// Public health endpoints
	health := &routes.Health{Pool: d.Pool}
	r.Get("/health", health.Health)
	r.Get("/ready", health.Ready)
	r.Get("/dbping", health.DBPing)

	// Wire shared services
	verifier := &auth.Verifier{Q: d.Queries}
	limiter := &ratelimit.Limiter{Q: d.Queries}
	enforcer := &quota.Enforcer{Q: d.Queries, Limiter: limiter, Billing: d.Billing}
	ingestSvc := &ingest.Service{
		Q:          d.Queries,
		Quota:      enforcer,
		ShardCount: d.ShardCount,
	}

	apiKeyMW := &mw.APIKey{Verifier: verifier, Logger: d.Log}
	rateMW := &mw.RateLimit{Limiter: limiter, Logger: d.Log}

	events := routes.NewEvents(ingestSvc, d.Log)
	verifySvc := &verify.Service{Q: d.Queries}
	eventsRead := &routes.EventsRead{Q: d.Queries, Verifier: verifySvc}
	chainsR := &routes.Chains{Q: d.Queries}
	evidenceR := &routes.Evidence{Q: d.Queries}

	r.Route("/api/v1", func(r chi.Router) {
		// Write path: requires events:write
		r.Group(func(r chi.Router) {
			r.Use(apiKeyMW.Require("events:write"))
			r.Use(rateMW.Check())
			events.Mount(r)
		})

		// Read path: events:read suffices. Rate limit still applies.
		r.Group(func(r chi.Router) {
			r.Use(apiKeyMW.Require("events:read"))
			r.Use(rateMW.Check())
			eventsRead.Mount(r)
		})

		// Chain verify: chains:verify scope.
		r.Group(func(r chi.Router) {
			r.Use(apiKeyMW.Require("chains:verify"))
			r.Use(rateMW.Check())
			chainsR.Mount(r)
		})

		// Evidence: evidence:read scope.
		r.Group(func(r chi.Router) {
			r.Use(apiKeyMW.Require("evidence:read"))
			r.Use(rateMW.Check())
			evidenceR.Mount(r)
		})
	})

	return r
}
