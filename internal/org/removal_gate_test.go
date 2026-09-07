package org_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/testutil/fakes"
)

func seedRemovalOrg(t *testing.T, store *fakes.Org, role org.Role) (uuid.UUID, uuid.UUID) {
	t.Helper()
	orgID, userID := uuid.New(), uuid.New()
	m, err := org.NewMembership(orgID, userID, role)
	require.NoError(t, err)
	require.NoError(t, store.SaveMembership(tenant.WithOrg(t.Context(), orgID), m))
	return orgID, userID
}

func TestRemoveMember_RefusesRemovingYourself(t *testing.T) {
	store := fakes.NewOrg()
	svc := newServiceWithStore(t, store)
	orgID, userID := seedRemovalOrg(t, store, org.RoleMember)

	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: orgID, UserID: userID})
	err := svc.RemoveMember(ctx, orgID, userID)
	require.Error(t, err)
	require.True(t, org.IsSelfRemovalError(err), "want *SelfRemovalError, got %T: %v", err, err)

	_, mErr := store.MembershipOf(ctx, orgID, userID)
	require.NoError(t, mErr, "the membership must survive a refused removal")
}

func TestRemoveMember_RefusesRemovingAnOwner(t *testing.T) {
	store := fakes.NewOrg()
	svc := newServiceWithStore(t, store)
	orgID, ownerID := seedRemovalOrg(t, store, org.RoleOwner)

	actor := uuid.New()
	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: orgID, UserID: actor})
	err := svc.RemoveMember(ctx, orgID, ownerID)
	require.Error(t, err)
	require.True(t, org.IsOwnerRemovalError(err), "want *OwnerRemovalError, got %T: %v", err, err)

	_, mErr := store.MembershipOf(ctx, orgID, ownerID)
	require.NoError(t, mErr, "the owner membership must survive a refused removal")
}

func TestRemoveMember_AllowsRemovingAnotherNonOwner(t *testing.T) {
	store := fakes.NewOrg()
	svc := newServiceWithStore(t, store)
	orgID, targetID := seedRemovalOrg(t, store, org.RoleMember)

	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: orgID, UserID: uuid.New()})
	require.NoError(t, svc.RemoveMember(ctx, orgID, targetID))

	_, mErr := store.MembershipOf(ctx, orgID, targetID)
	require.Error(t, mErr, "the membership must be gone")
}
