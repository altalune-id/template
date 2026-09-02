package tenant

import (
	"errors"

	"google.golang.org/grpc/codes"

	apperrorv1 "altalune.id/template/gen/go/apperror/v1"
	"altalune.id/template/internal/apperror"
)

// MissingError signals that the tenant Context is absent from ctx.
type MissingError struct{}

func (*MissingError) Error() string { return "tenant: missing context" }

// ToAppError maps MissingError to the canonical Unauthenticated envelope.
func (*MissingError) ToAppError() *apperror.AppError {
	return apperror.New(
		apperror.CodeTenantMissing,
		"Tenant context missing",
		codes.Unauthenticated,
		&apperrorv1.ErrorDetail{Code: apperror.CodeTenantMissing},
	)
}

// IsMissingError reports whether err's tree contains a *MissingError.
func IsMissingError(err error) bool {
	_, ok := errors.AsType[*MissingError](err)
	return ok
}
