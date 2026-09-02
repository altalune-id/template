package apperror

const (
	CodeUnexpectedError = "altempl.unexpected"
	CodeTenantMissing   = "altempl.tenant_missing"
	CodeUnauthenticated = "altempl.unauthenticated"
	CodeForbidden       = "altempl.forbidden"
	CodeValidation      = "altempl.validation"
	CodeNotFound        = "altempl.not_found"
	CodeAlreadyExists   = "altempl.already_exists"

	CodeTodoNotFound       = "todo.not_found"
	CodeTodoInvalidTitle   = "todo.invalid_title"
	CodeTodoAlreadyDeleted = "todo.already_deleted"

	CodeUserNotFound      = "user.not_found"
	CodeUserAlreadyExists = "user.already_exists"
	CodeUserNotInvited    = "user.not_invited"
	CodeUserInvalidEmail  = "user.invalid_email"
	CodeUserInvalidName   = "user.invalid_name"

	CodeOrgNotFound          = "org.not_found"
	CodeOrgAlreadyExists     = "org.already_exists"
	CodeOrgInvalidSlug       = "org.invalid_slug"
	CodeOrgInvalidName       = "org.invalid_name"
	CodeOrgMembershipExists  = "org.membership_exists"
	CodeOrgMembershipMissing = "org.membership_missing"
	CodeOrgCreationDisabled  = "org.creation_disabled"
	CodeOrgSystemProtected   = "org.system_protected"

	CodeProjectNotFound        = "project.not_found"
	CodeProjectAlreadyExists   = "project.already_exists"
	CodeProjectInvalidSlug     = "project.invalid_slug"
	CodeProjectSystemProtected = "project.system_protected"

	CodeInviteNotFound    = "invite.not_found"
	CodeInviteExpired     = "invite.expired"
	CodeInviteAlreadyUsed = "invite.already_used"
	CodeInviteInvalidRole = "invite.invalid_role"
	CodeInviteDisabled    = "invite.disabled"

	CodeSignupRequired = "signup.required"

	CodeAuthInvalidCredentials = "auth.invalid_credentials" //nolint:gosec // error code, not a credential
	CodeAuthOIDCUnavailable    = "auth.oidc_unavailable"
	CodeAuthOIDCClaimMissing   = "auth.oidc_claim_missing"

	CodeTokenExpired = "token.expired"

	// Onboarding.
	CodeOnboardingRequired    = "onboarding.required"
	CodeOnboardingAlreadyDone = "onboarding.already_done"
)
