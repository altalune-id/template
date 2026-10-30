package controlplane

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/platform/session"
	"altalune.id/template/internal/platform/tokens"
)

// SECURITY: RFC 6750 Section 3.1: a 403 challenge names a scope only when a token could be granted it, never a retired one.
func TestTranslateAuthnErrors_ChallengeNamesOnlyATokenScope(t *testing.T) {
	resource, err := tokens.NewResource("https://app.example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		scope string
		want  string
	}{
		{"a scope a token may hold", authn.ScopePostsAdmin, resource.InsufficientScope(authn.ScopePostsAdmin)},
		{"a retired scope", authn.ScopeAPIKeysWrite, ""},
		{"no declared scope", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refuse := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
				return nil, &authn.InsufficientScopeError{Scope: tc.scope, Source: session.SourceToken}
			}
			_, err := translateAuthnErrors(&resource)(refuse)(t.Context(), connect.NewRequest(&struct{}{}))
			got := ""
			if cerr := (*connect.Error)(nil); errors.As(err, &cerr) {
				got = cerr.Meta().Get("WWW-Authenticate")
			}
			if got != tc.want {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, tc.want)
			}
		})
	}
}
