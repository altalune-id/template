//go:build integration

package tenant_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/db"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/testutil/pgtest"
	"altalune.id/template/schema"
)

// TestPgTenants_Each_RLSVisibility asserts an app role bound by FORCE'd RLS sees zero orgs while a BYPASSRLS maintenance role sees them all.
func TestPgTenants_Each_RLSVisibility(t *testing.T) {
	h := pgtest.New(t)
	owner := h.OpenDB(t)

	cfg := config.Defaults()
	cfg.DB.Driver = "postgres"
	cfg.DB.DSN = h.DSN
	cfg.DB.Schema = "public"
	cfg.Tenant.RLSEnforce = true
	require.NoError(t, schema.MigrateUp(t.Context(), owner, cfg))

	prefix := cfg.DB.TablePrefix
	userID, orgID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	_, err := owner.ExecContext(t.Context(),
		"INSERT INTO "+prefix+"users (id, email, name, avatar_url, is_admin, created_at, updated_at) VALUES ($1,$2,'','',false,$3,$3)",
		userID, userID.String()+"@x.co", now)
	require.NoError(t, err)
	_, err = owner.ExecContext(t.Context(),
		"INSERT INTO "+prefix+"orgs (id, slug, name, created_by, created_at, updated_at) VALUES ($1,'acme','Acme',$2,$3,$3)",
		orgID, userID, now)
	require.NoError(t, err)

	appDSN := createRole(t, owner, h.DSN, "app_role", false, prefix)
	maintDSN := createRole(t, owner, h.DSN, "maint_role", true, prefix)

	count := func(dsn string) int {
		conn, oErr := sql.Open("pgx", dsn)
		require.NoError(t, oErr)
		t.Cleanup(func() { _ = conn.Close() })

		tn := tenant.NewPgTenants(db.Pool{W: conn, R: conn, M: conn},
			db.DriverPostgres, "public", prefix, slog.New(slog.NewTextHandler(io.Discard, nil)))
		var n int
		require.NoError(t, tn.Each(t.Context(), func(context.Context, string) error {
			n++
			return nil
		}))
		return n
	}

	require.Zero(t, count(appDSN), "app role must see no orgs under FORCE'd RLS")
	require.Equal(t, 1, count(maintDSN), "BYPASSRLS maintenance role must see every org")
}

func createRole(t *testing.T, owner *sql.DB, baseDSN, name string, bypassRLS bool, prefix string) string {
	t.Helper()
	attr := "NOBYPASSRLS"
	if bypassRLS {
		attr = "BYPASSRLS"
	}
	_, err := owner.ExecContext(t.Context(),
		"CREATE ROLE "+name+" LOGIN PASSWORD 'pw' "+attr)
	require.NoError(t, err)
	_, err = owner.ExecContext(t.Context(), "GRANT USAGE ON SCHEMA public TO "+name)
	require.NoError(t, err)
	_, err = owner.ExecContext(t.Context(),
		"GRANT SELECT ON "+prefix+"orgs TO "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = owner.ExecContext(context.Background(), "REVOKE ALL ON "+prefix+"orgs FROM "+name)
		_, _ = owner.ExecContext(context.Background(), "REVOKE ALL ON SCHEMA public FROM "+name)
		_, _ = owner.ExecContext(context.Background(), "DROP ROLE IF EXISTS "+name)
	})
	return pgtest.DSNWithUser(t, baseDSN, name, "pw")
}
