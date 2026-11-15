package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/session"
	"altalune.id/template/internal/web/handlers"
)

func TestProjectHandler_PostRename_RequiresManager(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		role org.Role
		want int
	}{
		{"member is refused", org.RoleMember, http.StatusForbidden},
		{"admin renames", org.RoleAdmin, http.StatusSeeOther},
		{"owner renames", org.RoleOwner, http.StatusSeeOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ctx := context.Background()
			owner := uuid.New()
			o := f.seedOrg(t, "acme", owner)
			p, err := f.Projects.Create(setTenant(ctx, o.ID, owner), o.ID, "alpha", "Alpha")
			require.NoError(t, err)

			caller := owner
			if tc.role != org.RoleOwner {
				caller = uuid.New()
				_, err = f.Orgs.AddMember(setTenant(ctx, o.ID, owner), o.ID, caller, tc.role)
				require.NoError(t, err)
			}

			h := handlers.NewProjectHandler(f.Deps, f.Projects)
			mux := http.NewServeMux()
			h.Register(mux)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, f.authedRequest(t, http.MethodPost, "/orgs/acme/projects/alpha/rename", "name=Renamed", session.Principal{UserID: caller, ActiveOrgID: o.ID}))
			require.Equal(t, tc.want, rec.Code, "body=%s", rec.Body.String())

			got, err := f.Projects.ByID(setTenantProject(ctx, o.ID, p.ID, owner), p.ID)
			require.NoError(t, err)
			wantName := "Renamed"
			if tc.want == http.StatusForbidden {
				wantName = "Alpha"
			}
			require.Equal(t, wantName, got.Name)
		})
	}
}

type storeDownError struct{}

func (*storeDownError) Error() string { return "store down" }

func TestProjectHandler_PostRename_RefusesWhenManagerLookupFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	owner := uuid.New()
	o := f.seedOrg(t, "acme", owner)
	p, err := f.Projects.Create(setTenant(ctx, o.ID, owner), o.ID, "alpha", "Alpha")
	require.NoError(t, err)

	h := handlers.NewProjectHandler(f.Deps, f.Projects)
	mux := http.NewServeMux()
	h.Register(mux)
	req := f.authedRequest(t, http.MethodPost, "/orgs/acme/projects/alpha/rename", "name=Renamed", session.Principal{UserID: owner, ActiveOrgID: o.ID})

	before := f.OrgStore.MembershipOfCalls()
	f.OrgStore.MembershipOfFailAfter = before + 1
	f.OrgStore.MembershipOfErr = &storeDownError{}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	f.OrgStore.MembershipOfErr = nil

	require.Equal(t, before+2, f.OrgStore.MembershipOfCalls(), "expected the scope gate then IsManager to read membership; a changed call order makes this test meaningless")
	require.Equal(t, http.StatusForbidden, rec.Code, "body=%s", rec.Body.String())
	got, err := f.Projects.ByID(setTenantProject(ctx, o.ID, p.ID, owner), p.ID)
	require.NoError(t, err)
	require.Equal(t, "Alpha", got.Name)
}
