// Package migrate is a small, dependency-light migration runner over an
// embedded set of SQL files. It tracks applied versions in a schema_migrations
// table, applies each pending up migration in its own transaction, and is
// idempotent: running it again when nothing is pending does nothing.
//
// It is deliberately minimal. The migration set for the open ingest server is
// small, and keeping the runner in-tree avoids adding a migration-framework
// dependency. If a project later wants community-standard tooling, the SQL
// files are plain and portable.
package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration is one numbered migration with its up and down SQL.
type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}

// Load reads and pairs the *.up.sql and *.down.sql files from fsys, sorted by
// version ascending.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	byVersion := map[int64]*Migration{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		var direction string
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			direction = "up"
		case strings.HasSuffix(name, ".down.sql"):
			direction = "down"
		default:
			continue
		}

		version, label, err := parseName(name)
		if err != nil {
			return nil, err
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		m := byVersion[version]
		if m == nil {
			m = &Migration{Version: version, Name: label}
			byVersion[version] = m
		}
		if direction == "up" {
			m.UpSQL = string(body)
		} else {
			m.DownSQL = string(body)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.UpSQL == "" {
			return nil, fmt.Errorf("migration %d (%s) has no up file", m.Version, m.Name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// parseName splits "0001_core_ingest.up.sql" into version 1 and "core_ingest".
func parseName(name string) (int64, string, error) {
	base := name
	base = strings.TrimSuffix(base, ".up.sql")
	base = strings.TrimSuffix(base, ".down.sql")
	idx := strings.Index(base, "_")
	num := base
	label := ""
	if idx >= 0 {
		num = base[:idx]
		label = base[idx+1:]
	}
	version, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("migration %q: name must start with a numeric version", name)
	}
	return version, label, nil
}

// Up applies every pending migration in order and returns the versions applied.
func Up(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]int64, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	if err := ensureTable(ctx, pool); err != nil {
		return nil, err
	}
	done, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}

	var applied []int64
	for _, m := range migrations {
		if done[m.Version] {
			continue
		}
		if err := applyOne(ctx, pool, m); err != nil {
			return applied, fmt.Errorf("apply %04d_%s: %w", m.Version, m.Name, err)
		}
		applied = append(applied, m.Version)
	}
	return applied, nil
}

// Down rolls back the most recently applied migration and returns its version.
// It returns 0 when there is nothing to roll back.
func Down(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (int64, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return 0, err
	}
	if err := ensureTable(ctx, pool); err != nil {
		return 0, err
	}
	done, err := appliedVersions(ctx, pool)
	if err != nil {
		return 0, err
	}

	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if !done[m.Version] {
			continue
		}
		if m.DownSQL == "" {
			return 0, fmt.Errorf("migration %04d_%s has no down file", m.Version, m.Name)
		}
		if err := rollbackOne(ctx, pool, m); err != nil {
			return 0, fmt.Errorf("rollback %04d_%s: %w", m.Version, m.Name, err)
		}
		return m.Version, nil
	}
	return 0, nil
}

// Applied returns the set of versions recorded as applied, sorted ascending.
func Applied(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	if err := ensureTable(ctx, pool); err != nil {
		return nil, err
	}
	done, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(done))
	for v := range done {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func ensureTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    bigint PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int64]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	done := map[int64]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	return inTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.UpSQL); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, m.Version)
		return err
	})
}

func rollbackOne(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	return inTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.DownSQL); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.Version)
		return err
	})
}

func inTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
