package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/apikey"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/platform/session"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/testutil/fakes"
	"altalune.id/template/internal/web/handlers"
)

type projectCatalog struct{ svc *project.Service }

func (c projectCatalog) ProjectIDs(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	list, err := c.svc.List(tenant.WithOrg(ctx, orgID), orgID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, p := range list {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

type apikeyWebFixture struct {
	*handlerFixture
	Keys   *apikey.Service
	Store  *fakes.APIKey
	Mux    *http.ServeMux
	owner  uuid.UUID
	member uuid.UUID
	org    uuid.UUID
	alpha  *project.Project
	beta   *project.Project
}

func newAPIKeyWebFixture(t *testing.T) *apikeyWebFixture {
	t.Helper()
	f := newFixture(t)
	owner, member := uuid.New(), uuid.New()
	o := f.seedOrg(t, "acme", owner)
	m, err := org.NewMembership(o.ID, member, org.RoleMember)
	require.NoError(t, err)
	require.NoError(t, f.OrgStore.SaveMembership(context.Background(), m))
	alpha, err := f.Projects.Create(setTenant(context.Background(), o.ID, owner), o.ID, "alpha", "Alpha")
	require.NoError(t, err)
	beta, err := f.Projects.Create(setTenant(context.Background(), o.ID, owner), o.ID, "beta", "Beta")
	require.NoError(t, err)

	store := fakes.NewAPIKey()
	keys := apikey.NewService(store, apikey.Scheme{}, f.Orgs, projectCatalog{svc: f.Projects}, discardLogger(), passthroughUnexpected())
	mux := http.NewServeMux()
	handlers.NewAPIKeyHandler(f.Deps, f.Projects, keys).Register(mux)
	return &apikeyWebFixture{handlerFixture: f, Keys: keys, Store: store, Mux: mux, owner: owner, member: member, org: o.ID, alpha: alpha, beta: beta}
}

func (x *apikeyWebFixture) as(t *testing.T, userID uuid.UUID, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := x.authedRequest(t, method, target, form.Encode(), session.Principal{UserID: userID, ActiveOrgID: x.org})
	r.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	x.Mux.ServeHTTP(rec, r)
	return rec
}

func (x *apikeyWebFixture) orgKeys(t *testing.T) []*apikey.APIKey {
	t.Helper()
	keys, err := x.Keys.ListOrg(setTenant(context.Background(), x.org, x.owner))
	require.NoError(t, err)
	return keys
}

func TestOrgAPIKeys_OwnerMintsASelectedProjectsKey(t *testing.T) {
	x := newAPIKeyWebFixture(t)
	rec := x.as(t, x.owner, http.MethodPost, "/orgs/acme/apikeys", url.Values{
		"name": {"ci"}, "scopes": {authn.ScopePostsRead}, "grant": {"selected"}, "project_ids": {x.alpha.ID.String()},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	keys := x.orgKeys(t)
	require.Len(t, keys, 1)
	assert.Equal(t, []uuid.UUID{x.alpha.ID}, keys[0].ProjectIDs)
	assert.Contains(t, rec.Body.String(), "…"+keys[0].SecretHint, "the list must show only the secret suffix")
	assert.Contains(t, rec.Body.String(), "Alpha")
}

func TestOrgAPIKeys_OwnerPromotesAKey(t *testing.T) {
	x := newAPIKeyWebFixture(t)
	x.as(t, x.owner, http.MethodPost, "/orgs/acme/apikeys", url.Values{
		"name": {"ci"}, "grant": {"selected"}, "project_ids": {x.alpha.ID.String()},
	})
	id := x.orgKeys(t)[0].ID.String()

	rec := x.as(t, x.owner, http.MethodPost, "/orgs/acme/apikeys/"+id+"/projects", url.Values{"project_ids": {x.beta.ID.String()}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.ElementsMatch(t, []uuid.UUID{x.alpha.ID, x.beta.ID}, x.orgKeys(t)[0].ProjectIDs)

	rec = x.as(t, x.owner, http.MethodPost, "/orgs/acme/apikeys/"+id+"/all-projects", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, x.orgKeys(t)[0].AllProjects)
	assert.NotContains(t, rec.Body.String(), "/all-projects", "an all-projects key offers no further promotion")
}

func TestOrgAPIKeys_EmptySelectionIsRefused(t *testing.T) {
	x := newAPIKeyWebFixture(t)
	rec := x.as(t, x.owner, http.MethodPost, "/orgs/acme/apikeys", url.Values{"name": {"ci"}, "grant": {"selected"}})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "apikey.error_empty_grant")
	assert.Empty(t, x.orgKeys(t))
}

// SECURITY: a member sees the lists read-only and every write is refused by the service, not only hidden by the page.
func TestAPIKeys_MemberIsReadOnly(t *testing.T) {
	x := newAPIKeyWebFixture(t)
	x.as(t, x.owner, http.MethodPost, "/orgs/acme/apikeys", url.Values{"name": {"ci"}, "grant": {"all"}})
	id := x.orgKeys(t)[0].ID.String()

	page := x.as(t, x.member, http.MethodGet, "/orgs/acme/apikeys", nil)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "apikey.read_only")
	assert.NotContains(t, page.Body.String(), `name="name"`, "a member must not get the mint form")
	assert.NotContains(t, page.Body.String(), "/revoke", "a member must not get a revoke button")

	for name, target := range map[string]string{
		"mint org key":     "/orgs/acme/apikeys",
		"revoke org key":   "/orgs/acme/apikeys/" + id + "/revoke",
		"mint project key": "/orgs/acme/projects/alpha/apikeys",
	} {
		t.Run(name, func(t *testing.T) {
			rec := x.as(t, x.member, http.MethodPost, target, url.Values{"name": {"sneaky"}, "grant": {"all"}})
			assert.Contains(t, rec.Body.String(), "apikey.error_not_manager", "body=%s", rec.Body.String())
		})
	}
	keys := x.orgKeys(t)
	require.Len(t, keys, 1)
	assert.Nil(t, keys[0].RevokedAt, "a member's revoke must not land")
	assert.Len(t, x.Store.All(), 1, "a member must mint nothing")
}

func TestProjectAPIKeys_HideTheRetiredScope(t *testing.T) {
	x := newAPIKeyWebFixture(t)
	page := x.as(t, x.owner, http.MethodGet, "/orgs/acme/projects/alpha/apikeys", nil)
	require.Equal(t, http.StatusOK, page.Code)
	assert.NotContains(t, page.Body.String(), `value="`+authn.ScopeAPIKeysWrite+`"`)
	assert.Contains(t, page.Body.String(), `value="`+authn.ScopePostsRead+`"`)
}
