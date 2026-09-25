// Package outbox is the durable at-least-once delivery queue that outbound dispatch claims from, with Postgres as the multi-node adapter and SQLite as the single-node development one.
package outbox

import (
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of an outbox entry.
type Status string

// Entry states, of which StatusPending is the only non-terminal one.
const (
	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
)

// Valid reports whether s is one of the three known states.
func (s Status) Valid() bool {
	return s == StatusPending || s == StatusDelivered || s == StatusFailed
}

// Terminal reports whether s admits no further transition.
func (s Status) Terminal() bool { return s == StatusDelivered || s == StatusFailed }

const (
	// MaxAttempts bounds delivery tries; an entry that reaches it moves to StatusFailed.
	MaxAttempts = 10
	// ClaimLease is how far ClaimDue pushes next_attempt_at, releasing an entry whose dispatcher died.
	ClaimLease = 5 * time.Minute
	// MaxTargetLen bounds the target identifier a caller may enqueue.
	MaxTargetLen = 255
	// MaxClaimLimit caps one ClaimDue batch.
	MaxClaimLimit = 1000
)

const (
	backoffBase     = 2 * time.Second
	backoffShiftCap = 12
)

// Entry is one durable delivery record.
type Entry struct {
	ID            uuid.UUID
	EventID       uuid.UUID
	OrgID         uuid.UUID
	ProjectID     uuid.UUID
	Target        string
	Payload       []byte
	Attempt       int
	NextAttemptAt time.Time
	Status        Status
	LastError     string
}

// Backoff returns the jittered wait before the given attempt.
func Backoff(attempt int) time.Duration {
	shift := min(max(attempt, 1)-1, backoffShiftCap)
	half := (backoffBase << shift) / 2
	return half + rand.N(half)
}

func (e Entry) validate() error {
	switch {
	case e.ID == uuid.Nil:
		return &InvalidEntryError{Field: "ID", Reason: "must not be the zero uuid"}
	case e.EventID == uuid.Nil:
		return &InvalidEntryError{Field: "EventID", Reason: "must not be the zero uuid"}
	case e.OrgID == uuid.Nil:
		return &InvalidEntryError{Field: "OrgID", Reason: "must not be the zero uuid"}
	case e.ProjectID == uuid.Nil:
		return &InvalidEntryError{Field: "ProjectID", Reason: "must not be the zero uuid"}
	case e.Target == "":
		return &InvalidEntryError{Field: "Target", Reason: "must not be empty"}
	case len(e.Target) > MaxTargetLen:
		return &InvalidEntryError{Field: "Target", Reason: "exceeds the length limit"}
	case e.Status != "" && e.Status != StatusPending:
		return &InvalidEntryError{Field: "Status", Reason: "a new entry is always pending"}
	}
	return nil
}

func claimBatch(limit int) int64 {
	return int64(min(max(limit, 1), MaxClaimLimit))
}
