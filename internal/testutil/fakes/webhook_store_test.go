package fakes_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/platform/events"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/testutil/fakes"
	"altalune.id/template/internal/webhook"
)

// TestWebhookStoreScopesByOrgNotProject pins the fake to the real stores' predicate: org from ctx, never project.
func TestWebhookStoreScopesByOrgNotProject(t *testing.T) {
	a := tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New()}
	sibling := tenant.Context{OrgID: a.OrgID, ProjectID: uuid.New()}
	b := tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New()}

	store := fakes.NewWebhookStore()
	e, err := webhook.New(a.OrgID, a.ProjectID, "https://example.com/hook", "", []events.Type{events.PostPublished})
	require.NoError(t, err)
	require.NoError(t, store.Save(tenant.Into(t.Context(), a), e))

	_, err = store.ByID(tenant.Into(t.Context(), sibling), e.ID)
	require.NoError(t, err, "the fake must not filter by project; the service owns that check")

	_, err = store.ByID(tenant.Into(t.Context(), b), e.ID)
	assert.True(t, webhook.IsNotFoundError(err))

	hijack := *e
	hijack.OrgID = b.OrgID
	assert.True(t, webhook.IsNotFoundError(store.Save(tenant.Into(t.Context(), b), &hijack)))
	assert.True(t, webhook.IsNotFoundError(store.Delete(tenant.Into(t.Context(), b), e.ID)))
}

// TestWebhookStoreSaveAttemptHoldsTheProjectGuard pins the fake to the real stores' SaveAttempt guard and error hook.
func TestWebhookStoreSaveAttemptHoldsTheProjectGuard(t *testing.T) {
	tc := tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New()}
	ctx := tenant.Into(t.Context(), tc)
	store := fakes.NewWebhookStore()
	deliveryID := uuid.New()

	a := webhook.Attempt{OrgID: tc.OrgID, ProjectID: uuid.New(), EndpointID: uuid.New(), DeliveryID: deliveryID}
	assert.True(t, webhook.IsInvalidAttemptError(store.SaveAttempt(ctx, a)))

	a.ProjectID = tc.ProjectID
	assert.True(t, webhook.IsNotFoundError(store.SaveAttempt(ctx, a)), "an attempt for a missing endpoint mirrors the FK")

	e, err := webhook.New(tc.OrgID, tc.ProjectID, "https://example.com/hook", "", []events.Type{events.PostPublished})
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, e))
	endpointID := e.ID
	a.EndpointID = endpointID
	a.CreatedAt = time.Now().Add(-time.Minute)
	require.NoError(t, store.SaveAttempt(ctx, a))
	a.CreatedAt = time.Now()
	require.NoError(t, store.SaveAttempt(ctx, a))

	got, err := store.ListAttempts(ctx, endpointID, deliveryID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.True(t, got[0].CreatedAt.After(got[1].CreatedAt), "newest first")
	assert.NotEqual(t, uuid.Nil, got[0].ID)

	boom := errors.New("boom")
	store.SaveAttemptErr = boom
	assert.ErrorIs(t, store.SaveAttempt(ctx, a), boom)
	assert.Len(t, store.Attempts(), 2)
}
