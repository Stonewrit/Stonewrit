package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stonewrit/stonewrit/server/internal/bootstrap"
	"github.com/stonewrit/stonewrit/server/internal/config"
	"github.com/stonewrit/stonewrit/server/internal/db"
	"github.com/stonewrit/stonewrit/server/internal/logger"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/internal/router"
	"github.com/stonewrit/stonewrit/server/internal/sealer"
	"github.com/stonewrit/stonewrit/server/internal/shard"
	"github.com/stonewrit/stonewrit/server/internal/workers"
)

func main() {
	env, err := config.Load()
	if err != nil {
		println("config load failed:", err.Error())
		os.Exit(1)
	}

	log := logger.New(env.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, env.DatabaseURL, env.PoolMax)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create db pool")
	}
	defer pool.Close()

	log.Info().
		Int32("pool_max", env.PoolMax).
		Str("port", env.Port).
		Bool("auth_enabled", env.AuthEnabled).
		Msg("stonewrit server starting")

	q := queries.New(pool)

	// Idempotent boot setup: seed the baseline catalog, and (when auth is off)
	// ensure the default tenant exists so ingest needs no seeding.
	if err := bootstrap.SeedCatalog(ctx, q); err != nil {
		log.Fatal().Err(err).Msg("failed to seed compliance catalog")
	}
	if !env.AuthEnabled {
		if err := bootstrap.EnsureDefaultTenant(ctx, q); err != nil {
			log.Fatal().Err(err).Msg("failed to ensure default tenant")
		}
		log.Warn().Msg("AUTH DISABLED: every request runs under the default tenant. Set APIKEY_AUTH=true to require API keys.")
	}

	srv := &http.Server{
		Addr: ":" + env.Port,
		Handler: router.New(router.Deps{
			Pool:        pool,
			Queries:     q,
			Log:         log,
			ShardCount:  shard.DefaultCount,
			AuthEnabled: env.AuthEnabled,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Sealer drains pending_events into the chained events table.
	// Runs in-process; horizontally scales with HTTP replicas because the
	// per-shard chain lock serialises competing sealers at Postgres.
	sealerSvc := &sealer.Sealer{
		Pool: pool,
		Q:    q,
		Log:  log,
	}
	go func() {
		if err := sealerSvc.Run(ctx); err != nil {
			log.Error().Err(err).Msg("sealer exited with error")
		}
	}()

	// Background export builder.
	exportWorker := &workers.GenerateExport{Pool: pool, Q: q, Log: log}
	go func() { _ = exportWorker.Run(ctx) }()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Fatal().Err(err).Msg("server crashed")
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
	log.Info().Msg("stonewrit-api-go stopped")
}
