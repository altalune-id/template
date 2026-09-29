package org_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/org"
)

// SECURITY: RequireManager is the one owner/admin gate every surface asks, so each denial must be the same typed error.
func TestRequireManager(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t, false)
	ctx := context.Background()
	orgID := uuid.New()

	seat := func(role org.Role) uuid.UUID {
		t.Helper()
		userID := uuid.New()
		m, err := org.NewMembership(orgID, userID, role)
		require.NoError(t, err)
		require.NoError(t, store.SaveMembership(ctx, m))
		return userID
	}

	tests := []struct {
		name   string
		userID uuid.UUID
		allow  bool
	}{
		{"an owner manages", seat(org.RoleOwner), true},
		{"an admin manages", seat(org.RoleAdmin), true},
		{"a member does not", seat(org.RoleMember), false},
		{"a non-member does not", uuid.New(), false},
		{"a machine principal with no user does not", uuid.Nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.RequireManager(ctx, orgID, tt.userID)
			if tt.allow {
				require.NoError(t, err)
				return
			}
			require.True(t, org.IsNotManagerError(err), "got %T: %v", err, err)
		})
	}
}

func TestRoleCanManage(t *testing.T) {
	t.Parallel()
	require.True(t, org.RoleOwner.CanManage())
	require.True(t, org.RoleAdmin.CanManage())
	require.False(t, org.RoleMember.CanManage())
	require.False(t, org.Role("").CanManage())
}
