package schema

import (
	"strings"
	"testing"
)

// Every postgres migration that assumes the owner role must hand the connection back before
// goose writes its bookkeeping row as the connection role, or the INSERT hits permission denied.
func TestPostgresMigrations_ResetRoleAfterEverySetRole(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations/postgres")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	seen := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		seen++
		t.Run(e.Name(), func(t *testing.T) {
			body, err := migrationsFS.ReadFile("migrations/postgres/" + e.Name())
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			sets := strings.Count(string(body), "SET ROLE {{.Role}}")
			resets := strings.Count(string(body), "RESET ROLE")
			if sets != resets {
				t.Errorf("%d SET ROLE vs %d RESET ROLE; every goose block that sets the owner role must reset it", sets, resets)
			}
		})
	}
	if seen == 0 {
		t.Fatal("no postgres migrations found; the guard would pass vacuously")
	}
}
