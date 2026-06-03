// Package bootstrap runs the idempotent setup the server needs on boot so a
// self-hoster never has to seed anything by hand: it loads the baseline
// compliance catalog from the embedded ruleset, and, when authentication is
// disabled, ensures the default tenant exists.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stonewrit/stonewrit/classify"
	"github.com/stonewrit/stonewrit/server/internal/auth"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

// SeedCatalog upserts the baseline frameworks and controls so events map to
// controls out of the box. Idempotent: existing rows are left untouched.
func SeedCatalog(ctx context.Context, q *queries.Queries) error {
	for _, f := range classify.Frameworks() {
		if err := q.UpsertFramework(ctx, queries.UpsertFrameworkParams{
			ID:          f.ID,
			Name:        f.Name,
			Description: optText(f.Description),
			Version:     optText(f.Version),
		}); err != nil {
			return fmt.Errorf("seed framework %s: %w", f.ID, err)
		}
	}
	for _, c := range classify.Controls() {
		if err := q.UpsertControl(ctx, queries.UpsertControlParams{
			Framework:   c.Framework,
			ControlID:   c.ControlID,
			Title:       c.Title,
			Description: optText(c.Description),
			Category:    optText(c.Category),
		}); err != nil {
			return fmt.Errorf("seed control %s:%s: %w", c.Framework, c.ControlID, err)
		}
	}
	return nil
}

// EnsureDefaultTenant creates the default project and environment that the
// no-auth default scope points at, so ingest works with no seeding.
func EnsureDefaultTenant(ctx context.Context, q *queries.Queries) error {
	projID, err := parseUUID(auth.DefaultProjectID)
	if err != nil {
		return err
	}
	envID, err := parseUUID(auth.DefaultEnvironmentID)
	if err != nil {
		return err
	}
	if err := q.EnsureDefaultProject(ctx, projID); err != nil {
		return fmt.Errorf("ensure default project: %w", err)
	}
	if err := q.EnsureDefaultEnvironment(ctx, queries.EnsureDefaultEnvironmentParams{
		ID:        envID,
		ProjectID: projID,
	}); err != nil {
		return fmt.Errorf("ensure default environment: %w", err)
	}
	return nil
}

func parseUUID(s string) (pgtype.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

// optText returns a pointer to s, or nil when empty, matching the nullable text
// columns the catalog upserts target.
func optText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
