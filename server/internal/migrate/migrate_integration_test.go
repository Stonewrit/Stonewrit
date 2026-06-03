//go:build integration

package migrate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stonewrit/stonewrit/server/migrations"
)

// requirePool connects to the integration database named by DATABASE_URL,
// skipping the test when it is not set so the suite stays green without a DB.
func requirePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestUpDownIdempotent exercises the full runner lifecycle against a real
// database: a clean apply, an idempotent re-apply, a rollback, and a re-apply.
func TestUpDownIdempotent(t *testing.T) {
	pool := requirePool(t)
	ctx := context.Background()

	// Start from a known-clean state so the test is repeatable.
	if _, err := Down(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("pre-clean down: %v", err)
	}

	applied, err := Up(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("expected at least one migration applied")
	}

	// Re-running applies nothing.
	again, err := Up(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("up (idempotent): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("expected no migrations on second up, got %v", again)
	}

	done, err := Applied(ctx, pool)
	if err != nil {
		t.Fatalf("applied: %v", err)
	}
	if len(done) != len(applied) {
		t.Errorf("applied count = %d, want %d", len(done), len(applied))
	}

	// Roll back the latest, then re-apply it.
	v, err := Down(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if v == 0 {
		t.Fatal("expected a version rolled back")
	}
	reapplied, err := Up(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if len(reapplied) != 1 || reapplied[0] != v {
		t.Errorf("expected to re-apply %d, got %v", v, reapplied)
	}
}
