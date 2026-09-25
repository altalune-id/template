package fakes

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/template/internal/platform/outbox"
	"altalune.id/template/internal/platform/tenant"
)

// Outbox is an in-memory outbox.Store for tests, holding the same claim and ctx contract as the real adapters.
type Outbox struct {
	mu   sync.Mutex
	byID map[uuid.UUID]outbox.Entry
	keys map[string]uuid.UUID

	// EnqueueErr, when set, is returned by the next Enqueue instead of recording the entry.
	EnqueueErr error
	// ClaimErr, when set, is returned by the next ClaimDue instead of claiming.
	ClaimErr error
}

// NewOutbox returns an empty in-memory outbox.Store.
func NewOutbox() *Outbox {
	return &Outbox{byID: map[uuid.UUID]outbox.Entry{}, keys: map[string]uuid.UUID{}}
}

var _ outbox.Store = (*Outbox)(nil)

// Entries returns every recorded entry ordered by id.
func (f *Outbox) Entries() []outbox.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]outbox.Entry, 0, len(f.byID))
	for _, e := range f.byID {
		out = append(out, cloneOutboxEntry(e))
	}
	slices.SortFunc(out, func(a, b outbox.Entry) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	return out
}

// ByID returns the recorded entry and whether it exists.
func (f *Outbox) ByID(id uuid.UUID) (outbox.Entry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.byID[id]
	return cloneOutboxEntry(e), ok
}

func (f *Outbox) Enqueue(ctx context.Context, e outbox.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.EnqueueErr != nil {
		return f.EnqueueErr
	}
	key := tc.OrgID.String() + "\x00" + e.EventID.String() + "\x00" + e.Target
	if _, dup := f.keys[key]; dup {
		return nil
	}
	e.OrgID = tc.OrgID
	e.Attempt = 0
	e.Status = outbox.StatusPending
	e.LastError = ""
	if e.NextAttemptAt.IsZero() {
		e.NextAttemptAt = time.Now().UTC()
	}
	f.keys[key] = e.ID
	f.byID[e.ID] = cloneOutboxEntry(e)
	return nil
}

func (f *Outbox) ClaimDue(ctx context.Context, now time.Time, limit int) ([]outbox.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ClaimErr != nil {
		return nil, f.ClaimErr
	}
	f.reapLocked(tc.OrgID, now)
	claimed := make([]outbox.Entry, 0, max(limit, 0))
	for _, id := range f.sortedIDsLocked() {
		if len(claimed) >= limit {
			break
		}
		e := f.byID[id]
		if e.OrgID != tc.OrgID || e.Status != outbox.StatusPending || e.Attempt >= outbox.MaxAttempts {
			continue
		}
		if e.NextAttemptAt.After(now) {
			continue
		}
		e.Attempt++
		e.NextAttemptAt = now.Add(outbox.ClaimLease).UTC()
		f.byID[id] = e
		claimed = append(claimed, cloneOutboxEntry(e))
	}
	return claimed, nil
}

func (f *Outbox) Succeed(ctx context.Context, claimed outbox.Entry, _ time.Time) error {
	return f.transition(ctx, claimed, func(e *outbox.Entry) {
		e.Status = outbox.StatusDelivered
		e.LastError = ""
	})
}

func (f *Outbox) Fail(ctx context.Context, claimed outbox.Entry, retryAt time.Time, cause string) error {
	return f.transition(ctx, claimed, func(e *outbox.Entry) {
		e.NextAttemptAt = retryAt.UTC()
		e.LastError = cause
		if e.Attempt >= outbox.MaxAttempts {
			e.Status = outbox.StatusFailed
		}
	})
}

func (f *Outbox) transition(ctx context.Context, claimed outbox.Entry, apply func(*outbox.Entry)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := claimed.ID
	e, ok := f.byID[id]
	if !ok || e.OrgID != tc.OrgID {
		return &outbox.NotFoundError{ID: id.String()}
	}
	if e.Status.Terminal() {
		return &outbox.TerminalStateError{ID: id.String(), Status: e.Status}
	}
	if e.Attempt != claimed.Attempt {
		return &outbox.StaleClaimError{ID: id.String(), Claimed: claimed.Attempt, Current: e.Attempt}
	}
	apply(&e)
	f.byID[id] = e
	return nil
}

func (f *Outbox) reapLocked(orgID uuid.UUID, now time.Time) {
	for id, e := range f.byID {
		if e.OrgID != orgID || e.Status != outbox.StatusPending || e.Attempt < outbox.MaxAttempts {
			continue
		}
		if e.NextAttemptAt.After(now) {
			continue
		}
		e.Status = outbox.StatusFailed
		f.byID[id] = e
	}
}

func (f *Outbox) sortedIDsLocked() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(f.byID))
	for id := range f.byID {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int {
		x, y := f.byID[a], f.byID[b]
		if c := x.NextAttemptAt.Compare(y.NextAttemptAt); c != 0 {
			return c
		}
		return bytes.Compare(a[:], b[:])
	})
	return ids
}

func cloneOutboxEntry(e outbox.Entry) outbox.Entry {
	e.Payload = slices.Clone(e.Payload)
	return e
}
