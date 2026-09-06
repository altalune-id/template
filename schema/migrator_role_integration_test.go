//go:build integration

package schema_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/db"
	"altalune.id/template/internal/testutil/pgtest"
	"altalune.id/template/nanoid"
	"altalune.id/template/schema"
)

func TestMigrateUp_BookkeepingOwnershipIsUniform(t *testing.T) {
	h := pgtest.New(t)

	// NOTE: pgtest reuses TEST_PG_DSN when set, so role and table names must be unique per run.
	suffix, err := nanoid.New(10)
	require.NoError(t, err)
	suffix = strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(suffix))
	role := "altempl_owner_" + suffix
	prefix := "t" + suffix + "_"

	admin, err := db.Open(t.Context(), db.DBConfig{Driver: db.DriverPostgres, DSN: h.DSN}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })

	_, err = admin.ExecContext(t.Context(), fmt.Sprintf(`CREATE ROLE %q NOLOGIN`, role))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.ExecContext(t.Context(), fmt.Sprintf(`DROP OWNED BY %q`, role))
		_, _ = admin.ExecContext(t.Context(), fmt.Sprintf(`DROP ROLE IF EXISTS %q`, role))
	})
	_, err = admin.ExecContext(t.Context(), fmt.Sprintf(`GRANT %q TO CURRENT_USER`, role))
	require.NoError(t, err)
	// NOTE: USAGE as well as CREATE — without USAGE the schema drops out of search_path and DDL fails with 3F000.
	_, err = admin.ExecContext(t.Context(), fmt.Sprintf(`GRANT USAGE, CREATE ON SCHEMA public TO %q`, role))
	require.NoError(t, err)

	cfg := config.Defaults()
	cfg.DB.Driver = "postgres"
	cfg.DB.Schema = "public"
	cfg.DB.TablePrefix = prefix

	migDB, err := db.Open(t.Context(), db.DBConfig{
		Driver: db.DriverPostgres, DSN: h.DSN, Role: role, MaxOpenConns: 1,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = migDB.Close() })

	require.NoError(t, schema.MigrateUp(t.Context(), migDB, cfg), "first MigrateUp")
	require.NoError(t, schema.MigrateUp(t.Context(), migDB, cfg),
		"second MigrateUp must not hit permission denied on the bookkeeping table")

	var owner string
	require.NoError(t, migDB.QueryRowContext(t.Context(),
		`SELECT tableowner FROM pg_tables WHERE tablename = $1`,
		prefix+"goose_db_version").Scan(&owner))
	require.Equal(t, role, owner, "bookkeeping table must be owned by the migration role")
}
