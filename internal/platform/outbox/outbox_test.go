package outbox_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/db"
	sqliteent "altalune.id/template/internal/platform/db/entity/sqlite"
	"altalune.id/template/internal/platform/outbox"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/schema"
)

const prefix = "altempl_"

func newSQLiteStore(t *testing.T) (outbox.Store, context.Context, tenant.Context) {
	t.Helper()

	cfg := config.Defaults()
	cfg.DB.Driver = db.DriverSQLite
	cfg.DB.DSN = filepath.Join(t.TempDir(), "outbox.db")

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sqlDB, err := db.Open(t.Context(), cfg.DB, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, schema.MigrateUp(t.Context(), sqlDB, cfg))
	require.Equal(t, prefix, cfg.DB.TablePrefix)

	tc := seedTenant(t, sqlDB)
	store := outbox.NewStore(
		db.DBConfig{Driver: db.DriverSQLite, TablePrefix: prefix},
		db.Pool{W: sqlDB, R: sqlDB},
		nil,
	)
	return store, tenant.Into(t.Context(), tc), tc
}

func seedTenant(t *testing.T, sqlDB *sql.DB) tenant.Context {
	t.Helper()
	userID, orgID, projID := uuid.New(), uuid.New(), uuid.New()
	now := sqliteent.SQLiteTime(time.Now())

	_, err := sqlDB.Exec(
		"INSERT INTO "+prefix+"users (id, email, name, avatar_url, is_admin, created_at, updated_at) VALUES (?, ?, '', '', 0, ?, ?)",
		userID.String(), userID.String()+"@example.com", now, now)
	require.NoError(t, err)
	_, err = sqlDB.Exec(
		"INSERT INTO "+prefix+"orgs (id, slug, name, created_by, created_at, updated_at) VALUES (?, ?, 'Org', ?, ?, ?)",
		orgID.String(), orgID.String()[:8], userID.String(), now, now)
	require.NoError(t, err)
	_, err = sqlDB.Exec(
		"INSERT INTO "+prefix+"projects (id, org_id, slug, name, created_by, created_at, updated_at) VALUES (?, ?, ?, 'Web', ?, ?, ?)",
		projID.String(), orgID.String(), projID.String()[:8], userID.String(), now, now)
	require.NoError(t, err)

	return tenant.Context{OrgID: orgID, ProjectID: projID, UserID: userID}
}

func entry(tc tenant.Context, eventID uuid.UUID) outbox.Entry {
	return outbox.Entry{
		ID:        uuid.New(),
		EventID:   eventID,
		OrgID:     tc.OrgID,
		ProjectID: tc.ProjectID,
		Target:    "delivery-endpoint",
		Payload:   []byte(`{"kind":"example"}`),
	}
}

func TestBackoffGrowsAndIsJittered(t *testing.T) {
	prev := time.Duration(0)
	for attempt := 1; attempt <= 6; attempt++ {
		got := outbox.Backoff(attempt)
		if got <= prev {
			t.Fatalf("Backoff(%d) = %v, not greater than %v", attempt, got, prev)
		}
		prev = got
	}
	first := outbox.Backoff(5)
	same := true
	for range 8 {
		if outbox.Backoff(5) != first {
			same = false
			break
		}
	}
	if same {
		t.Fatal("Backoff is not jittered; every replica will retry in lockstep")
	}
}

func TestBackoffTreatsNonPositiveAttemptAsFirst(t *testing.T) {
	for _, attempt := range []int{-3, 0, 1} {
		got := outbox.Backoff(attempt)
		assert.Positive(t, got, "Backoff(%d)", attempt)
		assert.Less(t, got, 2*time.Second, "Backoff(%d)", attempt)
	}
}

// SECURITY/correctness: two replicas must never claim one entry, or a tenant is delivered the same event twice per tick forever.
func TestClaimDueIsExclusive(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	require.NoError(t, s.Enqueue(ctx, entry(tc, uuid.New())))

	var a, b []outbox.Entry
	var errA, errB error
	var wg sync.WaitGroup
	wg.Go(func() { a, errA = s.ClaimDue(ctx, time.Now(), 10) })
	wg.Go(func() { b, errB = s.ClaimDue(ctx, time.Now(), 10) })
	wg.Wait()

	require.NoError(t, errA)
	require.NoError(t, errB)
	if len(a)+len(b) != 1 {
		t.Fatalf("claimed %d entries across two callers, want exactly 1", len(a)+len(b))
	}
}

func TestClaimDueLeasesTheEntryAgainstTheNextTick(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	require.NoError(t, s.Enqueue(ctx, entry(tc, uuid.New())))

	now := time.Now().UTC()
	first, err := s.ClaimDue(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, 1, first[0].Attempt)
	assert.Equal(t, outbox.StatusPending, first[0].Status)
	assert.WithinDuration(t, now.Add(outbox.ClaimLease), first[0].NextAttemptAt, time.Second)

	second, err := s.ClaimDue(ctx, now.Add(time.Minute), 10)
	require.NoError(t, err)
	assert.Empty(t, second, "the lease did not hold the entry back from the next tick")

	third, err := s.ClaimDue(ctx, now.Add(outbox.ClaimLease+time.Second), 10)
	require.NoError(t, err)
	require.Len(t, third, 1)
	assert.Equal(t, 2, third[0].Attempt)
}

func TestFailSchedulesRetryAndRecordsCause(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	require.NoError(t, s.Enqueue(ctx, e))

	retryAt := time.Now().Add(time.Minute).UTC()
	require.NoError(t, s.Fail(ctx, e, retryAt, "502 from endpoint"))

	due, err := s.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	if len(due) != 0 {
		t.Fatal("a failed entry was claimable before its retry time")
	}

	after, err := s.ClaimDue(ctx, retryAt.Add(time.Second), 10)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, "502 from endpoint", after[0].LastError)
}

func TestFailSettlesTerminallyOnceAttemptsRunOut(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	require.NoError(t, s.Enqueue(ctx, e))

	now := time.Now().UTC()
	for i := 1; i <= outbox.MaxAttempts; i++ {
		claimed, err := s.ClaimDue(ctx, now, 10)
		require.NoError(t, err, "attempt %d", i)
		require.Len(t, claimed, 1, "attempt %d", i)
		require.Equal(t, i, claimed[0].Attempt)
		require.NoError(t, s.Fail(ctx, claimed[0], now, "endpoint keeps refusing"))
	}

	settled, err := s.ClaimDue(ctx, now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, settled, "a permanently failing entry is still spinning")

	err = s.Fail(ctx, e, now, "again")
	require.Error(t, err)
	assert.True(t, outbox.IsTerminalStateError(err), "got %v", err)
}

// TestClaimDueReapsEntriesWhoseAttemptsRanOut proves the claim path settles exhaustion when no Fail ever lands.
func TestClaimDueReapsEntriesWhoseAttemptsRanOut(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	require.NoError(t, s.Enqueue(ctx, e))

	at := time.Now().UTC()
	for i := 1; i <= outbox.MaxAttempts; i++ {
		claimed, err := s.ClaimDue(ctx, at, 10)
		require.NoError(t, err, "attempt %d", i)
		require.Len(t, claimed, 1, "attempt %d", i)
		at = at.Add(outbox.ClaimLease + time.Second)
	}

	none, err := s.ClaimDue(ctx, at.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Empty(t, none)

	err = s.Succeed(ctx, e, time.Now())
	require.Error(t, err)
	assert.True(t, outbox.IsTerminalStateError(err), "got %v", err)
}

func TestSucceedSettlesAndRefusesASecondTransition(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	require.NoError(t, s.Enqueue(ctx, e))

	require.NoError(t, s.Succeed(ctx, e, time.Now()))

	none, err := s.ClaimDue(ctx, time.Now().Add(time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, none)

	err = s.Fail(ctx, e, time.Now(), "late failure")
	require.Error(t, err)
	assert.True(t, outbox.IsTerminalStateError(err), "got %v", err)
}

func TestSettleUnknownEntryIsNotFound(t *testing.T) {
	s, ctx, _ := newSQLiteStore(t)
	err := s.Succeed(ctx, outbox.Entry{ID: uuid.New()}, time.Now())
	require.Error(t, err)
	assert.True(t, outbox.IsNotFoundError(err), "got %v", err)
}

func TestEnqueueIsIdempotentPerEventAndTarget(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	eventID := uuid.New()
	first := entry(tc, eventID)
	second := entry(tc, eventID)

	require.NoError(t, s.Enqueue(ctx, first))
	require.NoError(t, s.Enqueue(ctx, second))

	claimed, err := s.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1, "a redelivered domain event became two deliveries")
	assert.Equal(t, first.ID, claimed[0].ID)

	other := entry(tc, eventID)
	other.Target = "another-endpoint"
	require.NoError(t, s.Enqueue(ctx, other))
	more, err := s.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, more, 1)
	assert.Equal(t, "another-endpoint", more[0].Target)
}

func TestEnqueueRejectsAnEntryOutsideTheTenantScope(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	e.OrgID = uuid.New()

	err := s.Enqueue(ctx, e)
	require.Error(t, err)
	assert.True(t, outbox.IsInvalidEntryError(err), "got %v", err)
}

func TestEnqueueRejectsAnIncompleteEntry(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	for name, mutate := range map[string]func(*outbox.Entry){
		"no id":           func(e *outbox.Entry) { e.ID = uuid.Nil },
		"no event id":     func(e *outbox.Entry) { e.EventID = uuid.Nil },
		"no org":          func(e *outbox.Entry) { e.OrgID = uuid.Nil },
		"no project":      func(e *outbox.Entry) { e.ProjectID = uuid.Nil },
		"no target":       func(e *outbox.Entry) { e.Target = "" },
		"long target":     func(e *outbox.Entry) { e.Target = string(make([]byte, outbox.MaxTargetLen+1)) },
		"already settled": func(e *outbox.Entry) { e.Status = outbox.StatusDelivered },
	} {
		t.Run(name, func(t *testing.T) {
			e := entry(tc, uuid.New())
			mutate(&e)
			err := s.Enqueue(ctx, e)
			require.Error(t, err)
			assert.True(t, outbox.IsInvalidEntryError(err), "got %v", err)
		})
	}
}

func TestStoreOperationsRequireATenantScope(t *testing.T) {
	s, _, tc := newSQLiteStore(t)
	bare := t.Context()

	require.Error(t, s.Enqueue(bare, entry(tc, uuid.New())))
	_, err := s.ClaimDue(bare, time.Now(), 10)
	require.Error(t, err)
	require.Error(t, s.Succeed(bare, outbox.Entry{ID: uuid.New()}, time.Now()))
	require.Error(t, s.Fail(bare, outbox.Entry{ID: uuid.New()}, time.Now(), "x"))
}

func TestClaimDueIgnoresAnotherOrgsEntries(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	require.NoError(t, s.Enqueue(ctx, entry(tc, uuid.New())))

	otherCtx := tenant.Into(t.Context(), tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New()})
	claimed, err := s.ClaimDue(otherCtx, time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, claimed, "an entry leaked across the tenant boundary")

	mine, err := s.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Len(t, mine, 1)
}

func TestStatusValidAndTerminal(t *testing.T) {
	assert.True(t, outbox.StatusPending.Valid())
	assert.True(t, outbox.StatusDelivered.Valid())
	assert.True(t, outbox.StatusFailed.Valid())
	assert.False(t, outbox.Status("queued").Valid())

	assert.False(t, outbox.StatusPending.Terminal())
	assert.True(t, outbox.StatusDelivered.Terminal())
	assert.True(t, outbox.StatusFailed.Terminal())
}

// TestClaimDueLeavesAnInFlightFinalAttemptAlone proves the reap waits for the lease, so a sweep during the last delivery cannot settle an entry another dispatcher is still working.
func TestClaimDueLeavesAnInFlightFinalAttemptAlone(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	require.NoError(t, s.Enqueue(ctx, e))

	at := time.Now().UTC()
	var last outbox.Entry
	for i := 1; i <= outbox.MaxAttempts; i++ {
		claimed, err := s.ClaimDue(ctx, at, 10)
		require.NoError(t, err, "attempt %d", i)
		require.Len(t, claimed, 1, "attempt %d", i)
		last = claimed[0]
		at = at.Add(outbox.ClaimLease + time.Second)
	}
	require.Equal(t, outbox.MaxAttempts, last.Attempt)

	_, err := s.ClaimDue(ctx, last.NextAttemptAt.Add(-time.Minute), 10)
	require.NoError(t, err)

	require.NoError(t, s.Succeed(ctx, last, time.Now()),
		"a sweep inside the lease reaped an entry whose final delivery was still in flight")
}

// TestSettleFromAStaleClaimIsRefused proves a settle is fenced on the attempt it claimed, so a slow dispatcher finishing after its lease expired cannot overwrite the next claimant's lease.
func TestSettleFromAStaleClaimIsRefused(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	require.NoError(t, s.Enqueue(ctx, entry(tc, uuid.New())))

	now := time.Now().UTC()
	first, err := s.ClaimDue(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, first, 1)

	later := now.Add(outbox.ClaimLease + time.Second)
	second, err := s.ClaimDue(ctx, later, 10)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, 2, second[0].Attempt)

	require.Error(t, s.Fail(ctx, first[0], later.Add(2*time.Second), "A finished after its lease"),
		"a settle from an expired claim was accepted")

	none, err := s.ClaimDue(ctx, later.Add(time.Minute), 10)
	require.NoError(t, err)
	assert.Empty(t, none, "a stale settle released the live lease and manufactured a third delivery")
}

// TestFailTruncatesACauseOnARuneBoundary proves an over-long cause is cut without splitting a rune, which Postgres rejects as an invalid UTF-8 byte sequence and which loses the outcome entirely.
func TestFailTruncatesACauseOnARuneBoundary(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	require.NoError(t, s.Enqueue(ctx, entry(tc, uuid.New())))

	now := time.Now().UTC()
	claimed, err := s.ClaimDue(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	cause := strings.Repeat("a", outbox.MaxCauseLen-1) + "é" + strings.Repeat("b", 64)
	require.False(t, utf8.ValidString(cause[:outbox.MaxCauseLen]), "the fixture does not straddle the cut")
	require.NoError(t, s.Fail(ctx, claimed[0], now, cause))

	after, err := s.ClaimDue(ctx, now.Add(outbox.ClaimLease+time.Second), 10)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.True(t, utf8.ValidString(after[0].LastError),
		"a truncated cause was persisted with a rune cut in half")
	assert.LessOrEqual(t, len(after[0].LastError), outbox.MaxCauseLen)
}

// TestEnqueueRejectsAnotherProjectsEntry proves the project scope is checked in Go; the FK runs as the table owner and bypasses RLS, so the database would accept another org's project.
func TestEnqueueRejectsAnotherProjectsEntry(t *testing.T) {
	s, ctx, tc := newSQLiteStore(t)
	e := entry(tc, uuid.New())
	e.ProjectID = uuid.New()

	err := s.Enqueue(ctx, e)
	require.Error(t, err)
	assert.True(t, outbox.IsInvalidEntryError(err), "got %v", err)
}
