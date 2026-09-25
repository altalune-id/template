package authn

import "errors"

// UnauthorizedError is the single opaque authentication failure.
type UnauthorizedError struct{}

func (*UnauthorizedError) Error() string { return "authn: unauthorized" }

// IsUnauthorizedError reports whether err is an *UnauthorizedError.
func IsUnauthorizedError(err error) bool {
	var target *UnauthorizedError
	return errors.As(err, &target)
}

// InsufficientScopeError names the scope the credential was missing.
type InsufficientScopeError struct{ Scope string }

func (e *InsufficientScopeError) Error() string {
	return "authn: credential lacks scope " + e.Scope
}

// IsInsufficientScopeError reports whether err is an *InsufficientScopeError.
func IsInsufficientScopeError(err error) bool {
	var target *InsufficientScopeError
	return errors.As(err, &target)
}
