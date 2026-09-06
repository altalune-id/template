//go:build integration

package db_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/platform/db"
	"altalune.id/template/internal/testutil/pgtest"
)

func TestOpen_AppliesRoleOnEveryConnection(t *testing.T) {
	h := pgtest.New(t)

	admin, err := db.Open(t.Context(), db.DBConfig{Driver: db.DriverPostgres, DSN: h.DSN}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	_, err = admin.ExecContext(t.Context(), `CREATE ROLE altempl_test_owner NOLOGIN`)
	require.NoError(t, err)
	_, err = admin.ExecContext(t.Context(), `GRANT altempl_test_owner TO CURRENT_USER`)
	require.NoError(t, err)

	roled, err := db.Open(t.Context(), db.DBConfig{
		Driver: db.DriverPostgres, DSN: h.DSN, Role: "altempl_test_owner", MaxOpenConns: 3,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = roled.Close() })

	for i := range 5 {
		var current string
		require.NoError(t, roled.QueryRowContext(t.Context(), `SELECT current_user`).Scan(&current))
		require.Equal(t, "altempl_test_owner", current, "connection %d did not carry the role", i)
	}
}

func TestOpen_EmptyRoleIsNoOp(t *testing.T) {
	h := pgtest.New(t)
	conn, err := db.Open(t.Context(), db.DBConfig{Driver: db.DriverPostgres, DSN: h.DSN}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var current string
	require.NoError(t, conn.QueryRowContext(t.Context(), `SELECT current_user`).Scan(&current))
	require.NotEmpty(t, current)
}

func TestOpen_UnknownRoleFailsLoudly(t *testing.T) {
	h := pgtest.New(t)

	start := time.Now()
	_, err := db.Open(t.Context(), db.DBConfig{
		Driver: db.DriverPostgres, DSN: h.DSN, Role: "altempl_role_does_not_exist",
		ConnectTimeout: 30 * time.Second, ConnectBackoff: 250 * time.Millisecond,
	}, nil)
	elapsed := time.Since(start)

	require.Error(t, err, "an unknown role must not open silently")
	require.Contains(t, err.Error(), "altempl_role_does_not_exist")
	require.Less(t, elapsed, 5*time.Second,
		"a server-rejected SET ROLE is permanent and must not burn the whole connect budget (%s)", elapsed)
}

func TestOpen_InvalidRoleIdentRejected(t *testing.T) {
	h := pgtest.New(t)
	_, err := db.Open(t.Context(), db.DBConfig{
		Driver: db.DriverPostgres, DSN: h.DSN, Role: "bad\nrole",
	}, nil)
	require.Error(t, err)
	require.True(t, db.IsInvalidRoleError(err), "want InvalidRoleError, got %T", err)
}
