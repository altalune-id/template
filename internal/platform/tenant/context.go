// Package tenant carries the OrgID/ProjectID/UserID triple through the request lifecycle.
package tenant

import (
	"context"

	"github.com/google/uuid"
)

// Context is the tenant scope threaded through every request.
type Context struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	UserID    uuid.UUID
}

type ctxKey struct{}

// Into returns a copy of ctx carrying tc.
func Into(ctx context.Context, tc Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, tc)
}

// From returns the tenant Context carried by ctx or a *MissingError when absent.
func From(ctx context.Context) (Context, error) {
	tc, ok := ctx.Value(ctxKey{}).(Context)
	if !ok {
		return Context{}, &MissingError{}
	}
	return tc, nil
}
