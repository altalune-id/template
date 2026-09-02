//go:build integration

// Package pgtest spins up ephemeral Postgres via testcontainers for integration tests.
package pgtest

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	// pgx stdlib driver registration for sql.Open("pgx", dsn).
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const envDSN = "TEST_PG_DSN"

// Handle wraps an ephemeral or shared Postgres instance for a test.
type Handle struct {
	DSN       string
	container testcontainers.Container
}

// Close terminates the underlying container (no-op when reusing an external Postgres via TEST_PG_DSN).
func (h *Handle) Close() error {
	if h == nil || h.container == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.container.Terminate(ctx)
}

// New returns a fresh Postgres for the test. If TEST_PG_DSN is set, it reuses that instance.
// Otherwise it starts a testcontainers-managed Postgres and terminates it when the test finishes.
// NOTE: disables the testcontainers Ryuk reaper so podman (which lacks a default `bridge` network) works;
// t.Cleanup already terminates containers, so Ryuk is redundant here.
func New(t *testing.T) *Handle {
	t.Helper()
	if dsn := os.Getenv(envDSN); dsn != "" {
		return &Handle{DSN: dsn}
	}

	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	c, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("altempl_test"),
		postgres.WithUsername("altempl"),
		postgres.WithPassword("altempl"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skipf("pgtest: cannot start Postgres container (need docker or podman socket): %v", err)
		return nil
	}

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("pgtest: connection string: %v", err)
	}

	h := &Handle{DSN: dsn, container: c}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Logf("pgtest: terminate: %v", err)
		}
	})
	return h
}

// OpenDB returns a *sql.DB against the handle's DSN with a fresh public schema, so each test starts from a clean slate on a shared instance.
func (h *Handle) OpenDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", h.DSN)
	if err != nil {
		t.Fatalf("pgtest: sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("pgtest: ping: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"); err != nil {
		_ = db.Close()
		t.Fatalf("pgtest: reset public schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
