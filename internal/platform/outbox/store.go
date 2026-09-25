package outbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/qrm"
)

// Store is the driven port for durable outbox persistence.
type Store interface {
	// Enqueue records a pending entry, ignoring a repeat of an (EventID, Target) already queued.
	Enqueue(ctx context.Context, e Entry) error
	// ClaimDue takes up to limit due entries exclusively, bumping Attempt and leasing each for ClaimLease.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]Entry, error)
	// Succeed settles the claimed entry e as delivered, refusing a settle from a superseded claim.
	Succeed(ctx context.Context, e Entry, at time.Time) error
	// Fail reschedules the claimed entry e for retryAt, or settles it as failed once MaxAttempts is reached.
	Fail(ctx context.Context, e Entry, retryAt time.Time, cause string) error
}

// MaxCauseLen bounds the failure cause persisted on an entry.
const MaxCauseLen = 1024

func truncateCause(cause string) string {
	if len(cause) > MaxCauseLen {
		cause = cause[:MaxCauseLen]
	}
	// NOTE: Postgres validates encoding on text input, so a rune split by the cut fails the statement.
	return strings.ToValidUTF8(cause, "")
}

func errorIsNoRows(err error) bool { return errors.Is(err, qrm.ErrNoRows) }
