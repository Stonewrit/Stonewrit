package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	// Per-connection connect timeout. With the default (unset), a slow TLS+SCRAM
	// handshake or a Neon serverless cold-start is bounded only by the caller's
	// ctx. Give each connect room so high-latency links and waking computes
	// don't fail instantly.
	cfg.ConnConfig.ConnectTimeout = 15 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Boot ping. 30s (not 5s): the first real connection does TCP + TLS + SCRAM
	// auth plus a possible Neon compute wake, which is several round-trips. On a
	// cold Neon compute or a high-latency link the full authenticated handshake
	// can exceed 5s even though a bare TCP connect is instant, and the server
	// would otherwise fail to start despite the DB being reachable.
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
