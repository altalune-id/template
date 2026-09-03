package org

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"

	apperrorv1 "altalune.id/template/gen/go/apperror/v1"
	"altalune.id/template/internal/apperror"
)

// NotFoundError signals a missing org row.
type NotFoundError struct {
	ID   string
	Slug string
}

func (e *NotFoundError) Error() string {
	switch {
	case e == nil:
		return "org: not found"
	case e.Slug != "":
		return fmt.Sprintf("org: not found: slug=%q", e.Slug)
	case e.ID != "":
		return fmt.Sprintf("org: not found: id=%s", e.ID)
	default:
		return "org: not found"
	}
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgNotFound,
		"Organisation not found",
		codes.NotFound,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgNotFound},
	)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError signals a slug collision on create.
type AlreadyExistsError struct {
	Slug string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("org: slug already exists: %q", e.Slug)
}

// ToAppError maps AlreadyExistsError to the canonical AlreadyExists envelope.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgAlreadyExists,
		"Organisation slug already taken",
		codes.AlreadyExists,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgAlreadyExists},
	)
}

// IsAlreadyExistsError reports whether err's tree contains an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// InvalidSlugError signals a slug that fails the aggregate invariants.
type InvalidSlugError struct {
	Slug   string
	Reason string
}

func (e *InvalidSlugError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("org: invalid slug: %q", e.Slug)
	}
	return fmt.Sprintf("org: invalid slug: %s", e.Reason)
}

// ToAppError maps InvalidSlugError to the canonical Validation envelope.
func (e *InvalidSlugError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgInvalidSlug,
		"Invalid organisation slug",
		codes.InvalidArgument,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgInvalidSlug},
	)
}

// IsInvalidSlugError reports whether err's tree contains an *InvalidSlugError.
func IsInvalidSlugError(err error) bool {
	_, ok := errors.AsType[*InvalidSlugError](err)
	return ok
}

// InvalidNameError signals a name that fails the aggregate invariants.
type InvalidNameError struct {
	Reason string
}

func (e *InvalidNameError) Error() string {
	if e.Reason == "" {
		return "org: invalid name"
	}
	return "org: invalid name: " + e.Reason
}

// ToAppError maps InvalidNameError to the canonical Validation envelope.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgInvalidName,
		"Invalid organisation name",
		codes.InvalidArgument,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgInvalidName},
	)
}

// IsInvalidNameError reports whether err's tree contains an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// InvalidRoleError signals a role outside the enumerated Role set.
type InvalidRoleError struct {
	Role string
}

func (e *InvalidRoleError) Error() string {
	return fmt.Sprintf("org: invalid role: %q", e.Role)
}

// ToAppError maps InvalidRoleError onto the canonical Validation envelope.
func (e *InvalidRoleError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeValidation,
		"Invalid membership role",
		codes.InvalidArgument,
		&apperrorv1.ErrorDetail{Code: apperror.CodeValidation},
	)
}

// IsInvalidRoleError reports whether err's tree contains an *InvalidRoleError.
func IsInvalidRoleError(err error) bool {
	_, ok := errors.AsType[*InvalidRoleError](err)
	return ok
}

// MembershipExistsError signals an idempotent AddMember on an existing row.
type MembershipExistsError struct {
	OrgID  string
	UserID string
}

func (e *MembershipExistsError) Error() string {
	return fmt.Sprintf("org: membership exists: org=%s user=%s", e.OrgID, e.UserID)
}

// ToAppError maps MembershipExistsError to the canonical AlreadyExists envelope.
func (e *MembershipExistsError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgMembershipExists,
		"Membership already exists",
		codes.AlreadyExists,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgMembershipExists},
	)
}

// IsMembershipExistsError reports whether err's tree contains a *MembershipExistsError.
func IsMembershipExistsError(err error) bool {
	_, ok := errors.AsType[*MembershipExistsError](err)
	return ok
}

// MembershipMissingError signals a missing membership row.
type MembershipMissingError struct {
	OrgID  string
	UserID string
}

func (e *MembershipMissingError) Error() string {
	return fmt.Sprintf("org: membership missing: org=%s user=%s", e.OrgID, e.UserID)
}

// ToAppError maps MembershipMissingError to the canonical NotFound envelope.
func (e *MembershipMissingError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgMembershipMissing,
		"Membership not found",
		codes.NotFound,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgMembershipMissing},
	)
}

// IsMembershipMissingError reports whether err's tree contains a *MembershipMissingError.
func IsMembershipMissingError(err error) bool {
	_, ok := errors.AsType[*MembershipMissingError](err)
	return ok
}

// SystemProtectedError signals a destructive op on a bootstrap-owned row.
type SystemProtectedError struct {
	Op       string
	OrgID    string
	UserID   string
	Resource string
}

func (e *SystemProtectedError) Error() string {
	if e == nil {
		return "org: system-protected"
	}
	res := e.Resource
	if res == "" {
		res = "org"
	}
	if e.Op == "" {
		return fmt.Sprintf("org: system-protected %s", res)
	}
	return fmt.Sprintf("org: system-protected %s: %s not allowed", res, e.Op)
}

// ToAppError maps SystemProtectedError to a FailedPrecondition envelope.
func (e *SystemProtectedError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgSystemProtected,
		"System-protected object cannot be modified",
		codes.FailedPrecondition,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgSystemProtected},
	)
}

// IsSystemProtectedError reports whether err's tree contains a *SystemProtectedError.
func IsSystemProtectedError(err error) bool {
	_, ok := errors.AsType[*SystemProtectedError](err)
	return ok
}

// CreationDisabledError signals that capabilities.OrgCreation is false.
type CreationDisabledError struct{}

func (*CreationDisabledError) Error() string { return "org: creation disabled" }

// ToAppError maps CreationDisabledError to a FailedPrecondition envelope.
func (*CreationDisabledError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeOrgCreationDisabled,
		"Organisation creation is disabled",
		codes.FailedPrecondition,
		&apperrorv1.ErrorDetail{Code: apperror.CodeOrgCreationDisabled},
	)
}

// IsCreationDisabledError reports whether err's tree contains a *CreationDisabledError.
func IsCreationDisabledError(err error) bool {
	_, ok := errors.AsType[*CreationDisabledError](err)
	return ok
}
