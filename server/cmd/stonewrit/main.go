// Command stonewrit is the management CLI for a self-hosted server: it applies
// migrations and creates the projects, environments, and API keys you need when
// authentication is turned on. It talks straight to Postgres (DATABASE_URL), so
// there is no dashboard and nothing to seed by hand.
//
// Usage:
//
//	stonewrit migrate up|down|status
//	stonewrit project create --name "My App"   |   stonewrit project list
//	stonewrit environment create --project <id> --name prod   |   stonewrit environment list
//	stonewrit key create --name ci   |   stonewrit key list   |   stonewrit key revoke <id>
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stonewrit/stonewrit/server/internal/auth"
	"github.com/stonewrit/stonewrit/server/internal/bootstrap"
	"github.com/stonewrit/stonewrit/server/internal/config"
	"github.com/stonewrit/stonewrit/server/internal/db"
	"github.com/stonewrit/stonewrit/server/internal/migrate"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "migrate":
		migrateCmd(args)
	case "project", "projects":
		projectCmd(args)
	case "environment", "environments", "env":
		environmentCmd(args)
	case "key", "keys":
		keyCmd(args)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `stonewrit - self-hosted management CLI

  stonewrit migrate up|down|status
  stonewrit project create --name NAME [--org default]
  stonewrit project list
  stonewrit environment create --project ID --name NAME [--type production]
  stonewrit environment list
  stonewrit key create [--name NAME] [--project ID] [--environment ID] [--scopes a,b]
  stonewrit key list
  stonewrit key revoke ID
`)
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "stonewrit: %v\n", err)
	os.Exit(1)
}

func openPool(ctx context.Context) *pgxpool.Pool {
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, 4)
	if err != nil {
		fatal(err)
	}
	return pool
}

func migrateCmd(args []string) {
	action := "up"
	if len(args) > 0 {
		action = args[0]
	}
	ctx := context.Background()
	pool := openPool(ctx)
	defer pool.Close()

	switch action {
	case "up":
		applied, err := migrate.Up(ctx, pool, migrations.FS)
		if err != nil {
			fatal(err)
		}
		if len(applied) == 0 {
			fmt.Println("already up to date")
			return
		}
		for _, v := range applied {
			fmt.Printf("applied %04d\n", v)
		}
	case "down":
		v, err := migrate.Down(ctx, pool, migrations.FS)
		if err != nil {
			fatal(err)
		}
		if v == 0 {
			fmt.Println("nothing to roll back")
			return
		}
		fmt.Printf("rolled back %04d\n", v)
	case "status":
		all, err := migrate.Load(migrations.FS)
		if err != nil {
			fatal(err)
		}
		applied, err := migrate.Applied(ctx, pool)
		if err != nil {
			fatal(err)
		}
		done := map[int64]bool{}
		for _, v := range applied {
			done[v] = true
		}
		for _, m := range all {
			state := "pending"
			if done[m.Version] {
				state = "applied"
			}
			fmt.Printf("%04d_%-20s %s\n", m.Version, m.Name, state)
		}
	default:
		fatal(fmt.Errorf("unknown migrate action %q (want up, down, or status)", action))
	}
}

func projectCmd(args []string) {
	if len(args) == 0 {
		usage()
	}
	ctx := context.Background()
	pool := openPool(ctx)
	defer pool.Close()
	q := queries.New(pool)

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("project create", flag.ExitOnError)
		name := fs.String("name", "", "project name (required)")
		org := fs.String("org", auth.DefaultOrganizationID, "organization namespace")
		_ = fs.Parse(args[1:])
		if *name == "" {
			fatal(fmt.Errorf("--name is required"))
		}
		p, err := q.CreateProject(ctx, queries.CreateProjectParams{
			OrganizationID: *org,
			Name:           *name,
			Slug:           slugify(*name),
		})
		if err != nil {
			fatal(err)
		}
		fmt.Printf("created project %s (%s)\n", uuid.UUID(p.ID.Bytes), p.Slug)
	case "list":
		ps, err := q.ListProjects(ctx)
		if err != nil {
			fatal(err)
		}
		for _, p := range ps {
			fmt.Printf("%s  %-20s  %s\n", uuid.UUID(p.ID.Bytes), p.Slug, p.Name)
		}
	default:
		usage()
	}
}

func environmentCmd(args []string) {
	if len(args) == 0 {
		usage()
	}
	ctx := context.Background()
	pool := openPool(ctx)
	defer pool.Close()
	q := queries.New(pool)

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("environment create", flag.ExitOnError)
		project := fs.String("project", "", "project id (required)")
		name := fs.String("name", "", "environment name (required)")
		typ := fs.String("type", "production", "environment type")
		org := fs.String("org", auth.DefaultOrganizationID, "organization namespace")
		_ = fs.Parse(args[1:])
		if *name == "" || *project == "" {
			fatal(fmt.Errorf("--project and --name are required"))
		}
		projID, err := parseUUID(*project)
		if err != nil {
			fatal(fmt.Errorf("invalid --project: %w", err))
		}
		e, err := q.CreateEnvironment(ctx, queries.CreateEnvironmentParams{
			OrganizationID: *org,
			ProjectID:      projID,
			Name:           *name,
			Slug:           slugify(*name),
			Type:           *typ,
		})
		if err != nil {
			fatal(err)
		}
		fmt.Printf("created environment %s (%s)\n", uuid.UUID(e.ID.Bytes), e.Slug)
	case "list":
		es, err := q.ListEnvironments(ctx)
		if err != nil {
			fatal(err)
		}
		for _, e := range es {
			fmt.Printf("%s  project %s  %-16s  %s\n",
				uuid.UUID(e.ID.Bytes), uuid.UUID(e.ProjectID.Bytes), e.Slug, e.Name)
		}
	default:
		usage()
	}
}

func keyCmd(args []string) {
	if len(args) == 0 {
		usage()
	}
	ctx := context.Background()
	pool := openPool(ctx)
	defer pool.Close()
	q := queries.New(pool)

	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("key create", flag.ExitOnError)
		name := fs.String("name", "default", "key name")
		project := fs.String("project", "", "project id (defaults to the built-in default project)")
		environment := fs.String("environment", "", "environment id (defaults to the built-in default environment)")
		scopesCSV := fs.String("scopes", strings.Join(auth.AllScopes, ","), "comma-separated scopes")
		org := fs.String("org", auth.DefaultOrganizationID, "organization namespace")
		_ = fs.Parse(args[1:])

		projID, envID, err := resolveTarget(ctx, q, *project, *environment)
		if err != nil {
			fatal(err)
		}

		token, err := mintToken()
		if err != nil {
			fatal(err)
		}
		row, err := q.InsertApiKey(ctx, queries.InsertApiKeyParams{
			KeyHash:        auth.HashKey(token),
			Name:           *name,
			OrganizationID: *org,
			ProjectID:      projID,
			EnvironmentID:  envID,
			Scopes:         splitCSV(*scopesCSV),
		})
		if err != nil {
			fatal(err)
		}
		fmt.Printf("created key %s\n\n", uuid.UUID(row.ID.Bytes))
		fmt.Printf("  %s\n\n", token)
		fmt.Println("This token is shown once. Store it now; it cannot be recovered.")
	case "list":
		ks, err := q.ListApiKeys(ctx)
		if err != nil {
			fatal(err)
		}
		for _, k := range ks {
			state := "active"
			if k.RevokedAt.Valid {
				state = "revoked"
			}
			fmt.Printf("%s  %-16s  %-8s  %v\n", uuid.UUID(k.ID.Bytes), k.Name, state, k.Scopes)
		}
	case "revoke":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: stonewrit key revoke ID"))
		}
		id, err := parseUUID(args[1])
		if err != nil {
			fatal(fmt.Errorf("invalid key id: %w", err))
		}
		if err := q.RevokeApiKey(ctx, id); err != nil {
			fatal(err)
		}
		fmt.Println("revoked")
	default:
		usage()
	}
}

// resolveTarget returns the project and environment to scope a key to. When
// both are empty it ensures the built-in default tenant exists and uses it.
func resolveTarget(ctx context.Context, q *queries.Queries, project, environment string) (pgtype.UUID, pgtype.UUID, error) {
	if project == "" && environment == "" {
		if err := bootstrap.EnsureDefaultTenant(ctx, q); err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, err
		}
		projID, _ := parseUUID(auth.DefaultProjectID)
		envID, _ := parseUUID(auth.DefaultEnvironmentID)
		return projID, envID, nil
	}
	if project == "" || environment == "" {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("pass both --project and --environment, or neither to use the default")
	}
	projID, err := parseUUID(project)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("invalid --project: %w", err)
	}
	envID, err := parseUUID(environment)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("invalid --environment: %w", err)
	}
	return projID, envID, nil
}

func mintToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sk_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func parseUUID(s string) (pgtype.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "item"
	}
	return s
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
