//go:build integration

package org_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/db"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/testutil/pgtest"
	"altalune.id/template/nanoid"
	"altalune.id/template/schema"
)

// NOTE: pgtest reuses TEST_PG_DSN when set, so role and table names must be unique per run.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	s, err := nanoid.New(10)
	require.NoError(t, err)
	return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s))
}

func createDefinerRole(t *testing.T, admin *sql.DB, name, attrs string) {
	t.Helper()
	_, err := admin.ExecContext(t.Context(), fmt.Sprintf(`CREATE ROLE %q %s`, name, attrs))
	require.NoError(t, err)
	// NOTE: t.Context() is already canceled by the time cleanups run, so teardown needs its own context.
	t.Cleanup(func() {
		_, dropErr := admin.ExecContext(context.Background(), fmt.Sprintf(`DROP OWNED BY %q`, name))
		require.NoError(t, dropErr, "leaked objects owned by %s", name)
		_, dropErr = admin.ExecContext(context.Background(), fmt.Sprintf(`DROP ROLE IF EXISTS %q`, name))
		require.NoError(t, dropErr, "leaked role %s", name)
	})
	_, err = admin.ExecContext(t.Context(), fmt.Sprintf(`GRANT %q TO CURRENT_USER`, name))
	require.NoError(t, err)
}

type definerFixture struct {
	store   org.Store
	appConn *sql.DB
	prefix  string
	userID  uuid.UUID
	orgID   uuid.UUID
	slug    string
	now     time.Time
}

func newDefinerFixture(t *testing.T) *definerFixture {
	t.Helper()
	h := pgtest.New(t)
	suffix := uniqueSuffix(t)
	ownerRole := "altempl_orgowner_" + suffix
	appRole := "altempl_orgapp_" + suffix
	prefix := "t" + suffix + "_"

	admin, err := db.Open(t.Context(), db.DBConfig{Driver: db.DriverPostgres, DSN: h.DSN}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })

	createDefinerRole(t, admin, ownerRole, "NOLOGIN BYPASSRLS")
	// NOTE: USAGE as well as CREATE — without USAGE the schema drops out of search_path and DDL fails with 3F000.
	_, err = admin.ExecContext(t.Context(), fmt.Sprintf(`GRANT USAGE, CREATE ON SCHEMA public TO %q`, ownerRole))
	require.NoError(t, err)

	createDefinerRole(t, admin, appRole, "LOGIN PASSWORD 'pw' NOBYPASSRLS")
	_, err = admin.ExecContext(t.Context(), fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %q`, appRole))
	require.NoError(t, err)

	// NOTE: mirrors scripts/db/bootstrap.template.sql — the service role's EXECUTE comes from default
	// privileges granted at creation time, which the migration's REVOKE ... FROM PUBLIC does not touch.
	for _, stmt := range []string{
		`ALTER DEFAULT PRIVILEGES FOR ROLE %[1]q IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %[2]q`,
		`ALTER DEFAULT PRIVILEGES FOR ROLE %[1]q IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO %[2]q`,
	} {
		_, err = admin.ExecContext(t.Context(), fmt.Sprintf(stmt, ownerRole, appRole))
		require.NoError(t, err)
	}

	migDB, err := db.Open(t.Context(), db.DBConfig{
		Driver: db.DriverPostgres, DSN: h.DSN, Role: ownerRole, MaxOpenConns: 1,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = migDB.Close() })

	cfg := config.Defaults()
	cfg.DB.Driver = "postgres"
	cfg.DB.Schema = "public"
	cfg.DB.TablePrefix = prefix
	cfg.DB.AllowBypassRLS = false
	cfg.Tenant.RLSEnforce = true
	require.NoError(t, schema.MigrateUp(t.Context(), migDB, cfg))

	f := &definerFixture{
		prefix: prefix,
		userID: uuid.New(),
		orgID:  uuid.New(),
		slug:   "acme-" + suffix,
		now:    time.Now().UTC().Truncate(time.Microsecond),
	}

	_, err = migDB.ExecContext(t.Context(),
		"INSERT INTO public."+prefix+"users (id, email, name, avatar_url, is_admin, created_at, updated_at) VALUES ($1,$2,'','',false,$3,$3)",
		f.userID, f.userID.String()+"@x.co", f.now)
	require.NoError(t, err)
	_, err = migDB.ExecContext(t.Context(),
		"INSERT INTO public."+prefix+"orgs (id, slug, name, system, created_by, created_at, updated_at) VALUES ($1,$2,'Acme',true,$3,$4,$4)",
		f.orgID, f.slug, f.userID, f.now)
	require.NoError(t, err)
	_, err = migDB.ExecContext(t.Context(),
		"INSERT INTO public."+prefix+"memberships (id, org_id, user_id, role, system, created_at) VALUES ($1,$2,$3,'owner',true,$4)",
		uuid.New(), f.orgID, f.userID, f.now)
	require.NoError(t, err)

	f.appConn, err = db.Open(t.Context(), db.DBConfig{
		Driver: db.DriverPostgres, DSN: pgtest.DSNWithUser(t, h.DSN, appRole, "pw"), MaxOpenConns: 2,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.appConn.Close() })

	f.store = org.NewStore(
		db.DBConfig{Driver: db.DriverPostgres, Schema: "public", TablePrefix: prefix},
		db.Pool{W: f.appConn, R: f.appConn},
		tenant.NewPgConn(f.appConn),
	)
	return f
}

func TestPostgres_DefinerWrappers_ReadWithoutTenantScope(t *testing.T) {
	f := newDefinerFixture(t)

	var bypass bool
	require.NoError(t, f.appConn.QueryRowContext(t.Context(),
		`SELECT rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass))
	require.False(t, bypass, "the app role must not hold BYPASSRLS or the reads below prove nothing")

	var direct int
	require.NoError(t, f.appConn.QueryRowContext(t.Context(),
		"SELECT count(*) FROM public."+f.prefix+"orgs").Scan(&direct))
	require.Zero(t, direct, "app role must read no orgs from the table under FORCE row level security")

	orgs, err := f.store.List(t.Context(), f.userID)
	require.NoError(t, err, "List must not need a tenant scope the caller cannot have yet")
	require.Len(t, orgs, 1, "List must see the org through the definer wrapper")
	require.Equal(t, f.orgID, orgs[0].ID)
	require.Equal(t, f.slug, orgs[0].Slug)
	require.Equal(t, "Acme", orgs[0].Name)
	require.Equal(t, f.userID, orgs[0].OwnerID)
	require.True(t, orgs[0].System)
	require.WithinDuration(t, f.now, orgs[0].CreatedAt, time.Second)

	got, err := f.store.BySlug(t.Context(), f.slug)
	require.NoError(t, err, "BySlug must not need a tenant scope the caller cannot have yet")
	require.Equal(t, f.orgID, got.ID)
	require.Equal(t, "Acme", got.Name)
	require.Equal(t, f.userID, got.OwnerID)
	require.True(t, got.System)
	require.WithinDuration(t, f.now, got.CreatedAt, time.Second)
}

func TestPostgres_DefinerWrappers_BySlugMissingIsNotFound(t *testing.T) {
	f := newDefinerFixture(t)

	_, err := f.store.BySlug(t.Context(), "no-such-org-"+f.prefix)
	require.True(t, org.IsNotFoundError(err), "want NotFoundError, got %T: %v", err, err)
}

func TestPostgres_DefinerWrappers_ListInsideCallerTransaction(t *testing.T) {
	f := newDefinerFixture(t)

	tc := tenant.Context{OrgID: f.orgID, UserID: f.userID}
	require.NoError(t, tenant.RunInTx(t.Context(), tenant.NewPgConn(f.appConn), tc, func(ctx context.Context) error {
		orgs, err := f.store.List(ctx, f.userID)
		require.NoError(t, err)
		require.Len(t, orgs, 1, "the wrapper must still lift RLS inside a tenant-scoped transaction")

		got, sErr := f.store.BySlug(ctx, f.slug)
		require.NoError(t, sErr)
		require.Equal(t, f.orgID, got.ID)
		return nil
	}))
}
