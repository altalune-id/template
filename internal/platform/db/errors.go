package db

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// InvalidRoleError is returned when a configured role name cannot be used as a Postgres identifier.
type InvalidRoleError struct {
	Role   string
	Reason string
}

func (e *InvalidRoleError) Error() string {
	return fmt.Sprintf("db: invalid role %q: %s", e.Role, e.Reason)
}

// IsInvalidRoleError reports whether err wraps an *InvalidRoleError.
func IsInvalidRoleError(err error) bool {
	_, ok := errors.AsType[*InvalidRoleError](err)
	return ok
}

// IsRowSecurityViolation reports an upsert refused on another org's row; trust it only after the store's same-org check.
func IsRowSecurityViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "42501" {
		return false
	}
	// NOTE: the routine name is never translated; the English text is the fallback when a server sends no routine.
	if pgErr.Routine != "" {
		return pgErr.Routine == "ExecWithCheckOptions"
	}
	return strings.Contains(pgErr.Message, "row-level security policy (USING expression)")
}
