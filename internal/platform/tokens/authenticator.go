package tokens

import (
	"context"

	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/platform/session"
)

// Authenticator adapts a Verifier to authn.Authenticator, so JWT verification is one link in an authn.Chain.
type Authenticator struct {
	v      Verifier
	scheme *authn.Scheme
}

// NewAuthenticator returns an authn.Authenticator backed by v.
func NewAuthenticator(v Verifier) *Authenticator {
	return &Authenticator{v: v}
}

var _ authn.Authenticator = (*Authenticator)(nil)

// WithScheme dispatches by shape: only a JWT or a personal token reaches the Verifier, never an API key.
func (a *Authenticator) WithScheme(s authn.Scheme) *Authenticator {
	a.scheme = &s
	return a
}

// Authenticate implements authn.Authenticator by verifying raw as an access token or personal token.
func (a *Authenticator) Authenticate(ctx context.Context, raw string) (session.Principal, error) {
	if a.scheme != nil {
		if shape := a.scheme.Looks(raw); shape != authn.ShapeJWT && shape != authn.ShapePersonalToken {
			return session.Principal{}, &InvalidTokenError{Reason: "not a token shape"}
		}
	}
	return a.v.Verify(ctx, raw)
}
