package tenant_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"altalune.id/template/internal/platform/tenant"
)

func TestFrom_Missing_ReturnsMissingError(t *testing.T) {
	_, err := tenant.From(context.Background())
	if err == nil {
		t.Fatal("expected error on bare ctx")
	}
	if !tenant.IsMissingError(err) {
		t.Fatalf("want *MissingError, got %T: %v", err, err)
	}
}

func TestInto_And_From_Roundtrip(t *testing.T) {
	want := tenant.Context{
		OrgID:     uuid.New(),
		ProjectID: uuid.New(),
		UserID:    uuid.New(),
	}
	ctx := tenant.Into(context.Background(), want)
	got, err := tenant.From(ctx)
	if err != nil {
		t.Fatalf("From returned error: %v", err)
	}
	if got != want {
		t.Errorf("got=%+v want=%+v", got, want)
	}
}
