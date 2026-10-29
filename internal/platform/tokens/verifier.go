// Package tokens verifies the Othra access tokens of one resource server through oauthrs.
package tokens

import (
	"context"
	"errors"
	"net/http"

	"github.com/altalune-id/oauthrs"
	"github.com/altalune-id/oidcrp"

	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/platform/session"
)

// PersonalTokenPrefix starts every Othra personal token, the only opaque token sent to introspection.
const PersonalTokenPrefix = "othra_pt_" //nolint:gosec // G101: a public token prefix, not a credential.

// Verifier verifies a bearer token and returns the authenticated Principal.
type Verifier interface {
	Verify(ctx context.Context, bearer string) (session.Principal, error)
}

// NewKeySource returns the issuer's discovery and key cache, shared by the app's resource servers; nil without an issuer.
func NewKeySource(cfg Config, client *http.Client) *oidcrp.KeySource {
	if cfg.Issuer == "" {
		return nil
	}
	return oidcrp.NewKeySource(cfg.Issuer, client)
}

// NewVerifier builds the Verifier for cfg over keys, which nil builds; an empty Issuer refuses every token.
func NewVerifier(cfg Config, keys *oidcrp.KeySource) (Verifier, error) {
	if cfg.Issuer == "" {
		return disabledVerifier{}, nil
	}
	if cfg.Audience == "" {
		return nil, errors.New("tokens: audience required when issuer set")
	}
	rsCfg := oauthrs.Config{Issuer: cfg.Issuer, Audience: cfg.Audience, Keys: keys, ClockSkew: cfg.ClockSkew}
	scheme := authn.Scheme{}
	if cfg.Introspection.Enabled() {
		// SECURITY: a negative cache TTL asks Othra on every call, so a revoked personal token stops at once.
		rsCfg.Introspection = &oauthrs.Introspection{
			ClientID: cfg.Introspection.ClientID, ClientSecret: cfg.Introspection.ClientSecret, TokenPrefix: PersonalTokenPrefix, CacheTTL: -1,
		}
		scheme.TokenPrefix = PersonalTokenPrefix
	}
	rs, err := oauthrs.New(rsCfg)
	if err != nil {
		return nil, err
	}
	return &verifier{rs: rs, scheme: scheme}, nil
}

type verifier struct {
	rs     *oauthrs.Verifier
	scheme authn.Scheme
}

// SECURITY: only a JWT or a personal token reaches oauthrs, so an app API key never leaves the process.
func (v *verifier) Verify(ctx context.Context, raw string) (session.Principal, error) {
	if shape := v.scheme.Looks(raw); shape != authn.ShapeJWT && shape != authn.ShapePersonalToken {
		return session.Principal{}, &InvalidTokenError{Reason: "neither an access token nor a personal token"}
	}
	tok, err := v.rs.Verify(ctx, raw)
	switch {
	case oauthrs.IsExpiredTokenError(err):
		return session.Principal{}, &ExpiredTokenError{Cause: err}
	case err != nil:
		return session.Principal{}, &InvalidTokenError{Reason: "refused by the resource server", Cause: err}
	}
	return session.Principal{
		Source:     session.SourceToken,
		IDPIssuer:  tok.Issuer,
		IDPSubject: tok.Subject,
		ClientID:   tok.ClientID,
		Scopes:     tok.Scopes,
	}, nil
}

type disabledVerifier struct{}

func (disabledVerifier) Verify(_ context.Context, _ string) (session.Principal, error) {
	return session.Principal{}, &InvalidTokenError{Reason: "verifier disabled (tokens.issuer empty)"}
}
