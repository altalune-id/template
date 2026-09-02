package boot_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"altalune.id/template/internal/boot"
)

func TestBoot_Bootstrap_SeedsGenesisAndSingletonOrg(t *testing.T) {
	cfg := newSmokeCfg(t)
	srv, err := boot.BootServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BootServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	orgs, err := srv.Orgs.List(context.Background(), mustGenesisID(t, srv))
	if err != nil {
		t.Fatalf("orgs.List: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("want 1 org for genesis, got %d", len(orgs))
	}
	if orgs[0].Slug != "default" {
		t.Errorf("org slug=%q, want %q", orgs[0].Slug, "default")
	}
}

func TestBoot_Bootstrap_Idempotent(t *testing.T) {
	cfg := newSmokeCfg(t)

	srv1, err := boot.BootServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	genesisID := mustGenesisID(t, srv1)
	_ = srv1.Close()

	srv2, err := boot.BootServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second boot must be idempotent: %v", err)
	}
	t.Cleanup(func() { _ = srv2.Close() })

	orgs, err := srv2.Orgs.List(context.Background(), genesisID)
	if err != nil {
		t.Fatalf("orgs.List: %v", err)
	}
	if len(orgs) != 1 {
		t.Errorf("expected exactly 1 singleton org after second boot, got %d", len(orgs))
	}
}

func mustGenesisID(t *testing.T, srv *boot.Server) uuid.UUID {
	t.Helper()
	u, err := srv.Users.EnsureGenesis(context.Background())
	if err != nil {
		t.Fatalf("EnsureGenesis: %v", err)
	}
	if u == nil {
		t.Fatalf("genesis user unexpectedly nil")
	}
	return u.ID
}
