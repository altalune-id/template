package boot_test

import (
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	mcpinternal "altalune.id/template/internal/mcp"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/user"
)

func (i *tokenIssuer) mintWith(t *testing.T, audience string, scopes []string, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{"aud": audience, "scope": strings.Join(scopes, " ")}
	maps.Copy(claims, extra)
	return i.idp.AccessToken(claims)
}

type foreignTenant struct {
	orgID     string
	projectID string
}

func seedForeignTenant(t *testing.T, f *mcpFixture) foreignTenant {
	t.Helper()

	ctx := t.Context()
	outsider, err := f.srv.Users.Create(ctx, user.CreateRequest{
		Email: "outsider@example.com", Name: "Outsider", Source: user.SourceOIDC,
	})
	require.NoError(t, err)
	o, err := f.srv.Orgs.Create(ctx, org.CreateRequest{Slug: "foreign-org", Name: "Foreign Org", OwnerID: outsider.ID})
	require.NoError(t, err)

	orgCtx := tenant.Into(ctx, tenant.Context{OrgID: o.ID, UserID: outsider.ID})
	p, err := f.srv.Projects.Create(orgCtx, o.ID, "foreign-project", "Foreign Project")
	require.NoError(t, err)

	projCtx := tenant.WithProject(orgCtx, p.ID)
	cat, err := f.srv.Categories.Create(projCtx, "Secrets", "secrets")
	require.NoError(t, err)
	post, err := f.srv.Posts.Create(projCtx, cat.ID, "Foreign", "foreign-secret", "body")
	require.NoError(t, err)
	_, err = f.srv.Posts.Publish(projCtx, post.ID, 0)
	require.NoError(t, err)

	return foreignTenant{orgID: o.ID.String(), projectID: p.ID.String()}
}

// TestMCP_JWTResolvesItsTenantFromMembership drives the S7 mount with a Bearer JWT end to end. SECURITY: a token never names its org.
func TestMCP_JWTResolvesItsTenantFromMembership(t *testing.T) {
	f := newMCPFixture(t, mcpOpts{enabled: true})
	foreign := seedForeignTenant(t, f)

	read := []string{authn.ScopePostsRead}

	t.Run("a token with no org_id resolves the subject's only membership", func(t *testing.T) {
		token := f.issuer.mintWith(t, mcpAudience, read, nil)
		rec := f.call(t, token, callToolBody(mcpinternal.ToolBlogList, map[string]any{"projectId": f.projectID}))

		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
		require.NotContains(t, rec.Body.String(), "Tenant context names no org",
			"a verified JWT reached a tenant-scoped tool with no org: membership never resolved; body=%s", rec.Body.String())
		require.Contains(t, rec.Body.String(), `\"slug\":\"hello\"`,
			"the JWT call must read the org's own posts; body=%s", rec.Body.String())
	})

	t.Run("an org_id claim naming another org is ignored and membership decides", func(t *testing.T) {
		token := f.issuer.mintWith(t, mcpAudience, read, map[string]any{"org_id": foreign.orgID})

		own := f.call(t, token, callToolBody(mcpinternal.ToolBlogList, map[string]any{"projectId": f.projectID}))
		require.Equal(t, http.StatusOK, own.Code, "body=%s", own.Body.String())
		require.Contains(t, own.Body.String(), `\"slug\":\"hello\"`, "membership must still resolve the subject's own org; body=%s", own.Body.String())

		rec := f.call(t, token, callToolBody(mcpinternal.ToolBlogList, map[string]any{"projectId": foreign.projectID}))
		require.NotContains(t, rec.Body.String(), "foreign-secret",
			"a token naming org_id=%s read that org's data; an issuer must never name a tenant; body=%s", foreign.orgID, rec.Body.String())
	})
}
