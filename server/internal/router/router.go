package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/ingest"
	mw "github.com/stonewrit/stonewrit/server/internal/middleware"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/internal/routes"
	"github.com/stonewrit/stonewrit/server/internal/verify"
)

type Deps struct {
	Pool        *pgxpool.Pool
	Queries     *queries.Queries
	Log         zerolog.Logger
	ShardCount  int
	AuthEnabled bool
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

	// Auth gate. When enabled, each route group requires a valid key with the
	// listed scope. When disabled, every request runs under the default tenant.
	apiKeyMW := &mw.APIKey{Verifier: &auth.Verifier{Q: d.Queries}, Logger: d.Log}
	guard := func(scope string) func(http.Handler) http.Handler {
		if d.AuthEnabled {
			return apiKeyMW.Require(scope)
		}
		return mw.InjectScope(auth.DefaultContext())
	}

	ingestSvc := &ingest.Service{Q: d.Queries, ShardCount: d.ShardCount}
	events := routes.NewEvents(ingestSvc, d.Log)
	eventsRead := &routes.EventsRead{Q: d.Queries, Verifier: &verify.Service{Q: d.Queries}}
	chainsR := &routes.Chains{Q: d.Queries}
	evidenceR := &routes.Evidence{Q: d.Queries}

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(guard("events:write"))
			events.Mount(r)
		})
		r.Group(func(r chi.Router) {
			r.Use(guard("events:read"))
			eventsRead.Mount(r)
		})
		r.Group(func(r chi.Router) {
			r.Use(guard("chains:verify"))
			chainsR.Mount(r)
		})
		r.Group(func(r chi.Router) {
			r.Use(guard("evidence:read"))
			evidenceR.Mount(r)
		})
	})

	return r
}
