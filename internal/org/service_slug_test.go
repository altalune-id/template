package org_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/slug"
)

func TestNewOrg_AcceptsGeneratedSlugs(t *testing.T) {
	for range 500 {
		s := slug.Generate()
		_, err := org.NewOrg(s, "Acme", uuid.New())
		require.NoErrorf(t, err, "generated slug %q must satisfy the org slug invariants", s)
	}
}

func TestService_Create_GeneratesSlugWhenBlank(t *testing.T) {
	for _, supplied := range []string{"", "   "} {
		svc, _ := newTestService(t, true)
		o, err := svc.Create(context.Background(), org.CreateRequest{Slug: supplied, Name: "Acme", OwnerID: uuid.New()})
		require.NoError(t, err)
		assert.NotEmpty(t, o.Slug)
		_, err = org.NewOrg(o.Slug, o.Name, o.OwnerID)
		assert.NoError(t, err, "generated slug %q must be valid", o.Slug)
	}
}

func TestService_Create_RetriesPastTakenGeneratedSlugs(t *testing.T) {
	svc, store := newTestService(t, true)
	store.TakeNextSlugs(org.MaxSlugAttempts - 1)

	o, err := svc.Create(context.Background(), org.CreateRequest{Name: "Acme", OwnerID: uuid.New()})
	require.NoError(t, err)
	assert.NotEmpty(t, o.Slug)
}

func TestService_Create_GivesUpAfterMaxSlugAttempts(t *testing.T) {
	svc, store := newTestService(t, true)
	store.TakeNextSlugs(org.MaxSlugAttempts)

	_, err := svc.Create(context.Background(), org.CreateRequest{Name: "Acme", OwnerID: uuid.New()})
	assert.True(t, org.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)
}

func TestService_Create_KeepsUserEditedSlug(t *testing.T) {
	svc, _ := newTestService(t, true)
	o, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme-hq", Name: "Acme", OwnerID: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, "acme-hq", o.Slug)
}

func TestService_Create_UserEditedSlugTakenIsNotRetried(t *testing.T) {
	svc, _ := newTestService(t, true)
	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme-hq", Name: "Acme", OwnerID: uuid.New()})
	require.NoError(t, err)

	_, err = svc.Create(context.Background(), org.CreateRequest{Slug: "acme-hq", Name: "Acme Two", OwnerID: uuid.New()})
	require.True(t, org.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)
	assert.Contains(t, err.Error(), "acme-hq")
}
