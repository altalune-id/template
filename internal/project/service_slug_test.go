package project_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"altalune.id/template/internal/platform/slug"
	"altalune.id/template/internal/project"
)

func TestNew_AcceptsGeneratedSlugs(t *testing.T) {
	orgID := uuid.New()
	for range 500 {
		s := slug.Generate()
		if _, err := project.New(orgID, s, "Web"); err != nil {
			t.Fatalf("generated slug %q must satisfy the project slug invariants: %v", s, err)
		}
	}
}

func TestService_Create_GeneratesSlugWhenBlank(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	for _, supplied := range []string{"", "   "} {
		svc, _ := newTestService(t)
		p, err := svc.Create(tenantCtx(orgID, userID), orgID, supplied, "Web")
		if err != nil {
			t.Fatalf("Create(%q): %v", supplied, err)
		}
		if p.Slug == "" {
			t.Fatal("Create left the slug empty")
		}
		if _, err := project.New(orgID, p.Slug, p.Name); err != nil {
			t.Fatalf("generated slug %q is not valid: %v", p.Slug, err)
		}
	}
}

func TestService_Create_RetriesPastTakenGeneratedSlugs(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	svc, store := newTestService(t)
	store.TakeNextSlugs(project.MaxSlugAttempts - 1)

	p, err := svc.Create(tenantCtx(orgID, userID), orgID, "", "Web")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Slug == "" {
		t.Fatal("Create left the slug empty")
	}
}

func TestService_Create_GivesUpAfterMaxSlugAttempts(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	svc, store := newTestService(t)
	store.TakeNextSlugs(project.MaxSlugAttempts)

	_, err := svc.Create(tenantCtx(orgID, userID), orgID, "", "Web")
	if !project.IsAlreadyExistsError(err) {
		t.Fatalf("want AlreadyExistsError, got %T: %v", err, err)
	}
}

func TestService_Create_KeepsUserEditedSlug(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	svc, _ := newTestService(t)

	p, err := svc.Create(tenantCtx(orgID, userID), orgID, "web-app", "Web")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Slug != "web-app" {
		t.Fatalf("Slug = %q, want %q", p.Slug, "web-app")
	}
}

func TestService_Create_UserEditedSlugTakenIsNotRetried(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	svc, _ := newTestService(t)
	ctx := tenantCtx(orgID, userID)

	if _, err := svc.Create(ctx, orgID, "web-app", "Web"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := svc.Create(ctx, orgID, "web-app", "Web Two")
	if !project.IsAlreadyExistsError(err) {
		t.Fatalf("want AlreadyExistsError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "web-app") {
		t.Fatalf("error must name the taken slug: %v", err)
	}
}
