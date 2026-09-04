//go:build integration

// Package pgtest spins up ephemeral Postgres via testcontainers for integration tests.
package pgtest

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	// pgx stdlib driver registration for sql.Open("pgx", dsn).
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	envDSN     = "TEST_PG_DSN"
	labelOwner = "id.altalune.pgtest"
	staleAfter = 30 * time.Minute
)

//nolint:gochecknoglobals // the stale-container sweep runs once per test binary, not once per test.
var sweepOnce sync.Once

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

// New returns a fresh Postgres for the test, reusing TEST_PG_DSN when that is set.
// NOTE: Ryuk cannot boot on macOS+podman without a rootful privileged machine
// (https://golang.testcontainers.org/system_requirements/using_podman/), and t.Cleanup does not run
// on timeout or SIGINT — so containers are labelled and stale ones are swept here instead.
func New(t *testing.T) *Handle {
	t.Helper()
	if dsn := os.Getenv(envDSN); dsn != "" {
		return &Handle{DSN: dsn}
	}

	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	sweepOnce.Do(func() { sweepStale(t) })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	c, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("altempl_test"),
		postgres.WithUsername("altempl"),
		postgres.WithPassword("altempl"),
		testcontainers.WithLabels(map[string]string{labelOwner: "true"}),
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

func sweepStale(t *testing.T) {
	t.Helper()
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Logf("pgtest: sweep: docker provider: %v", err)
		return
	}
	defer func() { _ = provider.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cli := provider.Client()
	found, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", labelOwner+"=true"),
	})
	if err != nil {
		t.Logf("pgtest: sweep: container list: %v", err)
		return
	}

	cutoff := time.Now().Add(-staleAfter).Unix()
	for _, ctr := range found.Items {
		if ctr.Created > cutoff {
			continue
		}
		if _, rErr := cli.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); rErr != nil {
			t.Logf("pgtest: sweep: remove %s: %v", ctr.ID, rErr)
			continue
		}
		t.Logf("pgtest: sweep: removed stale container %s", ctr.ID)
	}
}

// DSNWithUser returns baseDSN with its credentials replaced by user and pass.
func DSNWithUser(t *testing.T, baseDSN, user, pass string) string {
	t.Helper()
	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("pgtest: parse DSN %q: %v", baseDSN, err)
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
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
