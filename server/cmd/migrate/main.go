// Command migrate applies the embedded SQL migrations to the database named by
// DATABASE_URL. It is separate from the server on purpose: schema changes are
// an explicit operational step, not something that runs on every boot.
//
// Usage:
//
//	migrate up     # apply all pending migrations (default)
//	migrate down   # roll back the most recently applied migration
//	migrate status # list applied and pending migrations
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stonewrit/stonewrit/server/internal/config"
	"github.com/stonewrit/stonewrit/server/internal/db"
	"github.com/stonewrit/stonewrit/server/internal/migrate"
	"github.com/stonewrit/stonewrit/server/migrations"
)

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL, 4)
	if err != nil {
		fatal(err)
	}
	defer pool.Close()

	switch cmd {
	case "up":
		applied, err := migrate.Up(ctx, pool, migrations.FS)
		if err != nil {
			fatal(err)
		}
		if len(applied) == 0 {
			fmt.Println("migrate: already up to date")
			return
		}
		for _, v := range applied {
			fmt.Printf("migrate: applied %04d\n", v)
		}
	case "down":
		v, err := migrate.Down(ctx, pool, migrations.FS)
		if err != nil {
			fatal(err)
		}
		if v == 0 {
			fmt.Println("migrate: nothing to roll back")
			return
		}
		fmt.Printf("migrate: rolled back %04d\n", v)
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
		fatal(fmt.Errorf("unknown command %q (want up, down, or status)", cmd))
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
	os.Exit(1)
}
