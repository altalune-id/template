package todo_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/db"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/todo"
	"altalune.id/template/schema"
)

func newSQLiteStoreForTest(t *testing.T) (todo.Store, *sql.DB, tenant.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("foreign_keys pragma: %v", err)
	}
	cfg := config.Defaults()
	if err := schema.MigrateUp(context.Background(), sqlDB, cfg); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	uid, oid, pid := seedTenant(t, sqlDB, cfg.DB.TablePrefix)
	tc := tenant.Context{OrgID: oid, ProjectID: pid, UserID: uid}

	store := todo.NewStore(db.DBConfig{Driver: db.DriverSQLite, TablePrefix: cfg.DB.TablePrefix}, db.Pool{W: sqlDB, R: sqlDB}, nil)
	return store, sqlDB, tc
}

func seedTenant(t *testing.T, sqlDB *sql.DB, prefix string) (userID, orgID, projID uuid.UUID) {
	t.Helper()
	userID = uuid.New()
	orgID = uuid.New()
	projID = uuid.New()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := sqlDB.Exec(
		"INSERT INTO "+prefix+"users (id, email, name, avatar_url, is_admin, created_at, updated_at) "+
			"VALUES (?, ?, '', '', 0, ?, ?)",
		userID.String(), userID.String()+"@x.com", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(
		"INSERT INTO "+prefix+"orgs (id, slug, name, created_by, created_at, updated_at) "+
			"VALUES (?, ?, 'Org', ?, ?, ?)",
		orgID.String(), orgID.String()[:8], userID.String(), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(
		"INSERT INTO "+prefix+"projects (id, org_id, slug, name, created_by, created_at, updated_at) "+
			"VALUES (?, ?, 'web', 'Web', ?, ?, ?)",
		projID.String(), orgID.String(), userID.String(), now, now); err != nil {
		t.Fatal(err)
	}
	return
}

func TestSQLiteStore_SaveAndByID(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	ctx := tenant.Into(context.Background(), tc)

	td, err := todo.New(tc.OrgID, tc.ProjectID, "buy milk")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, td); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.ByID(ctx, td.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.Title != "buy milk" {
		t.Errorf("title=%q", got.Title)
	}
	if got.OrgID != tc.OrgID {
		t.Errorf("org mismatch")
	}
}

func TestSQLiteStore_ByID_NotFound(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	ctx := tenant.Into(context.Background(), tc)
	_, err := store.ByID(ctx, uuid.New())
	if !todo.IsNotFoundError(err) {
		t.Errorf("want IsNotFoundError, got %v", err)
	}
}

func TestSQLiteStore_ListDoneFilter(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	ctx := tenant.Into(context.Background(), tc)

	t1, _ := todo.New(tc.OrgID, tc.ProjectID, "one")
	t2, _ := todo.New(tc.OrgID, tc.ProjectID, "two")
	t2.Toggle()
	t3, _ := todo.New(tc.OrgID, tc.ProjectID, "three")
	_ = store.Save(ctx, t1)
	_ = store.Save(ctx, t2)
	_ = store.Save(ctx, t3)

	all, err := store.List(ctx, tc.OrgID, tc.ProjectID, todo.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("all len=%d want 3", len(all))
	}

	yes := true
	done, err := store.List(ctx, tc.OrgID, tc.ProjectID, todo.ListOpts{Done: &yes})
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || done[0].Title != "two" {
		t.Errorf("filter mismatch: %+v", done)
	}
}

func TestSQLiteStore_SaveIsUpsert(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	ctx := tenant.Into(context.Background(), tc)
	td, _ := todo.New(tc.OrgID, tc.ProjectID, "milk")
	if err := store.Save(ctx, td); err != nil {
		t.Fatal(err)
	}
	td.Toggle()
	if err := store.Save(ctx, td); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, err := store.ByID(ctx, td.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Done {
		t.Errorf("Done round-trip failed")
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	ctx := tenant.Into(context.Background(), tc)
	td, _ := todo.New(tc.OrgID, tc.ProjectID, "milk")
	_ = store.Save(ctx, td)

	if err := store.Delete(ctx, td.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ByID(ctx, td.ID); !todo.IsNotFoundError(err) {
		t.Errorf("after Delete want IsNotFoundError, got %v", err)
	}
	if err := store.Delete(ctx, td.ID); !todo.IsNotFoundError(err) {
		t.Errorf("double delete want IsNotFoundError, got %v", err)
	}
}

func TestSQLiteStore_ClearDone(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	ctx := tenant.Into(context.Background(), tc)
	for i, done := range []bool{true, false, true} {
		td, _ := todo.New(tc.OrgID, tc.ProjectID, "t")
		if done {
			td.Toggle()
		}
		td.CreatedAt = td.CreatedAt.Add(time.Duration(i) * time.Millisecond)
		_ = store.Save(ctx, td)
	}
	n, err := store.ClearDone(ctx, tc.OrgID, tc.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("cleared=%d want 2", n)
	}
	rest, _ := store.List(ctx, tc.OrgID, tc.ProjectID, todo.ListOpts{})
	if len(rest) != 1 {
		t.Errorf("rest len=%d want 1", len(rest))
	}
}

func TestSQLiteStore_TenantMissing(t *testing.T) {
	store, _, tc := newSQLiteStoreForTest(t)
	td, _ := todo.New(tc.OrgID, tc.ProjectID, "milk")
	err := store.Save(context.Background(), td)
	if !tenant.IsMissingError(err) {
		t.Errorf("Save without tenant: want MissingError, got %v", err)
	}

	var zero *todo.NotFoundError
	if errors.As(err, &zero) {
		t.Errorf("MissingError should not resolve to NotFoundError")
	}
}
