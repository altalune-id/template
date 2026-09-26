package mcp_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpinternal "altalune.id/template/internal/mcp"
	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/platform/session"
)

type rejectingAuthenticator struct{ err error }

func (r rejectingAuthenticator) Authenticate(context.Context, string) (session.Principal, error) {
	return session.Principal{}, r.err
}

func TestAuthenticate_LogsADistinctReasonPerFailure(t *testing.T) {
	tests := []struct {
		name       string
		auth       authn.Authenticator
		header     string
		wantReason string
	}{
		{"no authenticator", nil, "Bearer a.b.c", "no authenticator configured"},
		{"absent credential", rejectingAuthenticator{err: &authn.UnauthorizedError{}}, "", "no bearer credential"},
		{"unrecognized shape", rejectingAuthenticator{err: &authn.UnauthorizedError{}}, "Bearer opaque-token", "neither the api key prefix nor the JWT shape"},
		{"chain rejected", rejectingAuthenticator{err: &authn.UnauthorizedError{}}, "Bearer a.b.c", "every authenticator in the chain rejected"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

			guard := mcpinternal.Authenticate(tc.auth, authn.Scheme{Prefix: "key_"}, "https://x/meta", log)
			h := guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Fatal("the guard admitted a request it must have rejected")
			}))

			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := buf.String(); !strings.Contains(got, tc.wantReason) {
				t.Fatalf("log did not carry the reason %q:\n%s", tc.wantReason, got)
			}
			if body := rec.Body.String(); strings.Contains(body, tc.wantReason) {
				t.Fatalf("SECURITY: the reason leaked into the response body:\n%s", body)
			}
		})
	}
}
