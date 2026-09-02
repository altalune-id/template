// Package todo is the todos bounded context.
package todo

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Todo is the aggregate root. Invariants live here.
type Todo struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	Title     string
	Done      bool
	CreatedAt time.Time
}

// New enforces creation invariants: title trimmed, non-empty, <= 200 runes.
func New(orgID, projectID uuid.UUID, title string) (*Todo, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, &InvalidTitleError{Reason: "empty"}
	}
	if utf8.RuneCountInString(title) > 200 {
		return nil, &InvalidTitleError{Reason: "over 200 characters"}
	}
	return &Todo{
		ID:        uuid.Must(uuid.NewV7()),
		OrgID:     orgID,
		ProjectID: projectID,
		Title:     title,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Toggle flips the done flag.
func (t *Todo) Toggle() { t.Done = !t.Done }

// ListOpts filters Store.List. Zero value returns every todo in scope.
type ListOpts struct {
	Done *bool
}
