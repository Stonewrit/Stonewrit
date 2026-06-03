package migrate

import (
	"testing"

	"github.com/stonewrit/stonewrit/server/migrations"
)

// TestLoad_EmbeddedMigrations parses the real embedded migration set: every up
// file pairs with a down file, versions are unique and ascending, and the first
// migration is the core ingest schema. This runs with no database.
func TestLoad_EmbeddedMigrations(t *testing.T) {
	ms, err := Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("expected at least one embedded migration")
	}

	var last int64
	for i, m := range ms {
		if m.UpSQL == "" {
			t.Errorf("migration %04d has empty up SQL", m.Version)
		}
		if m.DownSQL == "" {
			t.Errorf("migration %04d has empty down SQL", m.Version)
		}
		if i > 0 && m.Version <= last {
			t.Errorf("versions not strictly ascending at %04d", m.Version)
		}
		last = m.Version
	}

	if ms[0].Version != 1 || ms[0].Name != "core_ingest" {
		t.Errorf("first migration = %04d_%s, want 0001_core_ingest", ms[0].Version, ms[0].Name)
	}
}

func TestParseName(t *testing.T) {
	cases := []struct {
		in      string
		version int64
		label   string
		wantErr bool
	}{
		{"0001_core_ingest.up.sql", 1, "core_ingest", false},
		{"0042_add_index.down.sql", 42, "add_index", false},
		{"nope.up.sql", 0, "", true},
	}
	for _, tc := range cases {
		v, label, err := parseName(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", tc.in)
			}
			continue
		}
		if err != nil || v != tc.version || label != tc.label {
			t.Errorf("%s: got (%d, %q, %v), want (%d, %q, nil)", tc.in, v, label, err, tc.version, tc.label)
		}
	}
}
