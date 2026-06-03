// Package migrations holds the embedded SQL schema for the open ingest server
// and exposes it as a filesystem the migrate runner applies. Migrations are
// applied by the migrate command, never automatically on server startup.
package migrations

import "embed"

// FS holds the numbered .up.sql and .down.sql files.
//
//go:embed *.sql
var FS embed.FS
