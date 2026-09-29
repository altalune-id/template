package apikey_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/template/internal/apikey"
	"altalune.id/template/internal/platform/authn"
	sqliteent "altalune.id/template/internal/platform/db/entity/sqlite"
	"altalune.id/template/internal/platform/tenant"
)

func TestSQLiteOrgKeyRoundTrip(t *testing.T) {
	store, sqlDB, tc := newAPIKeyStoreForTest(t)
	second := seedProject(t, sqlDB, tc)
	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: tc.OrgID, UserID: tc.UserID})

	k, plaintext, err := apikey.Scheme{}.MintOrg(tc.OrgID, "ci", []string{authn.ScopePostsRead}, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{tc.ProjectID}}, nil, time.Now().UTC())
	require.NoError(t, err)
	k.CreatedBy = tc.UserID
	require.NoError(t, store.Save(ctx, k))

	got, err := store.ByID(ctx, k.ID)
	require.NoError(t, err)
	assert.Equal(t, apikey.KindOrg, got.Kind)
	assert.Equal(t, uuid.Nil, got.ProjectID)
	assert.Equal(t, []uuid.UUID{tc.ProjectID}, got.ProjectIDs)
	assert.Equal(t, k.SecretHint, got.SecretHint)
	assert.Equal(t, tc.UserID, got.CreatedBy)

	require.NoError(t, got.GrantProjects([]uuid.UUID{second}))
	require.NoError(t, store.Save(ctx, got))
	resolved, err := store.BySecretHash(t.Context(), sha(plaintext))
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{tc.ProjectID, second}, resolved.ProjectIDs, "the hot path must carry the grant")

	require.NoError(t, resolved.GrantAllProjects())
	require.NoError(t, store.Save(ctx, resolved))
	all, err := store.ByID(ctx, k.ID)
	require.NoError(t, err)
	assert.True(t, all.AllProjects)
	assert.Empty(t, all.ProjectIDs, "promoting to all projects must drop the named rows")

	orgKeys, err := store.ListOrg(ctx)
	require.NoError(t, err)
	require.Len(t, orgKeys, 1)
	projectKeys, err := store.List(ctx, tc.ProjectID)
	require.NoError(t, err)
	assert.Empty(t, projectKeys)
}

// SECURITY: SQLite has no RLS, so the composite foreign key is what refuses a grant naming another org's project.
func TestSQLiteGrantCannotNameAnotherOrgsProject(t *testing.T) {
	store, sqlDB, tc := newAPIKeyStoreForTest(t)
	foreign := seedTenant(t, sqlDB)
	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: tc.OrgID, UserID: tc.UserID})

	k, _, err := apikey.Scheme{}.MintOrg(tc.OrgID, "ci", nil, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{foreign.ProjectID}}, nil, time.Now().UTC())
	require.NoError(t, err)
	require.Error(t, store.Save(ctx, k), "a grant row naming another org's project must be refused")

	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT count(*) FROM "+prefix+"api_key_projects").Scan(&n))
	assert.Zero(t, n)
}

func TestSQLiteKindShapeIsEnforced(t *testing.T) {
	_, sqlDB, tc := newAPIKeyStoreForTest(t)
	now := sqliteent.SQLiteTime(time.Now())
	_, err := sqlDB.Exec(
		"INSERT INTO "+prefix+"api_keys (id, org_id, project_id, kind, all_projects, name, secret_hash, created_at) VALUES (?, ?, NULL, 'project', 0, 'bad', ?, ?)",
		uuid.NewString(), tc.OrgID.String(), []byte(uuid.NewString()), now)
	require.Error(t, err, "a project key without a project must violate the kind check")
}

func sha(plaintext string) [32]byte { return sha256.Sum256([]byte(plaintext)) }
