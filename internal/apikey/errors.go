package apikey

import "errors"

// UnknownScopeError names a scope outside the authn catalog.
type UnknownScopeError struct{ Scope string }

func (e *UnknownScopeError) Error() string { return "apikey: unknown scope " + e.Scope }

// IsUnknownScopeError reports whether err is an *UnknownScopeError.
func IsUnknownScopeError(err error) bool {
	var target *UnknownScopeError
	return errors.As(err, &target)
}

// NotFoundError reports a key that does not exist, is revoked, or has expired.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "apikey: not found" }

// IsNotFoundError reports whether err is a *NotFoundError.
func IsNotFoundError(err error) bool {
	var target *NotFoundError
	return errors.As(err, &target)
}

// RetiredScopeError names a scope that still validates on existing keys but may not be granted to a new one.
type RetiredScopeError struct{ Scope string }

func (e *RetiredScopeError) Error() string { return "apikey: retired scope " + e.Scope }

// IsRetiredScopeError reports whether err is a *RetiredScopeError.
func IsRetiredScopeError(err error) bool {
	var target *RetiredScopeError
	return errors.As(err, &target)
}

// ScopeLevelError names an org-level scope requested on a project key.
type ScopeLevelError struct{ Scope string }

func (e *ScopeLevelError) Error() string { return "apikey: scope " + e.Scope + " is org-level" }

// IsScopeLevelError reports whether err is a *ScopeLevelError.
func IsScopeLevelError(err error) bool {
	var target *ScopeLevelError
	return errors.As(err, &target)
}

// EmptyGrantError reports an org key grant that names no project and is not all projects.
type EmptyGrantError struct{}

func (*EmptyGrantError) Error() string { return "apikey: grant names no project" }

// IsEmptyGrantError reports whether err is an *EmptyGrantError.
func IsEmptyGrantError(err error) bool {
	var target *EmptyGrantError
	return errors.As(err, &target)
}

// GrantConflictError reports a grant that is all projects and also names projects.
type GrantConflictError struct{}

func (*GrantConflictError) Error() string { return "apikey: grant is all projects and names projects" }

// IsGrantConflictError reports whether err is a *GrantConflictError.
func IsGrantConflictError(err error) bool {
	var target *GrantConflictError
	return errors.As(err, &target)
}

// NotOrgKeyError reports a grant change on a project key, which is bound to its project for life.
type NotOrgKeyError struct{}

func (*NotOrgKeyError) Error() string { return "apikey: only an org key has a project grant" }

// IsNotOrgKeyError reports whether err is a *NotOrgKeyError.
func IsNotOrgKeyError(err error) bool {
	var target *NotOrgKeyError
	return errors.As(err, &target)
}

// AlreadyAllProjectsError reports a grant change on an org key that already reaches every project.
type AlreadyAllProjectsError struct{}

func (*AlreadyAllProjectsError) Error() string { return "apikey: key already reaches all projects" }

// IsAlreadyAllProjectsError reports whether err is an *AlreadyAllProjectsError.
func IsAlreadyAllProjectsError(err error) bool {
	var target *AlreadyAllProjectsError
	return errors.As(err, &target)
}

// RevokedError reports a grant change on a revoked key.
type RevokedError struct{}

func (*RevokedError) Error() string { return "apikey: key is revoked" }

// IsRevokedError reports whether err is a *RevokedError.
func IsRevokedError(err error) bool {
	var target *RevokedError
	return errors.As(err, &target)
}

// ProjectNotInOrgError names a granted project that is not a project of the key's org.
type ProjectNotInOrgError struct{ ProjectID string }

func (e *ProjectNotInOrgError) Error() string { return "apikey: project not in org: " + e.ProjectID }

// IsProjectNotInOrgError reports whether err is a *ProjectNotInOrgError.
func IsProjectNotInOrgError(err error) bool {
	var target *ProjectNotInOrgError
	return errors.As(err, &target)
}
