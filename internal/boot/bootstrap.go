package boot

import (
	"context"
	"fmt"
	"log/slog"

	"altalune.id/template/internal/onboard"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/user"
)

const (
	defaultBootstrapOrgSlug    = "default"
	defaultBootstrapOrgName    = "Default Organization"
	defaultBootstrapProjectID  = "default"
	defaultBootstrapProjectStr = "Default Project"
)

func bootstrap(
	ctx context.Context,
	cfg *config.Config,
	users *user.Service,
	orgs *org.Service,
	projects *project.Service,
	onboards *onboard.Service,
	log *slog.Logger,
) (bool, error) {
	required, err := onboards.Required(ctx)
	if err != nil {
		return false, fmt.Errorf("onboard: required: %w", err)
	}
	if !required {
		return true, nil
	}
	if cfg.Genesis.Email == "" {
		return false, nil
	}

	u, err := users.EnsureGenesis(ctx)
	if err != nil {
		return false, fmt.Errorf("genesis: %w", err)
	}
	if u == nil {
		return false, nil
	}

	orgSlug, orgName := bootstrapOrgIdentity(cfg)
	o, oErr := orgs.BootstrapSingleton(ctx, orgSlug, orgName, u.ID)
	if oErr != nil {
		return false, fmt.Errorf("bootstrap first org: %w", oErr)
	}
	if log != nil {
		log.Info("bootstrap: first org ready",
			slog.String("slug", o.Slug),
			slog.String("owner", u.Email),
		)
	}

	projSlug, projName := bootstrapProjectIdentity(cfg)
	tctx := tenant.Into(ctx, tenant.Context{OrgID: o.ID, UserID: u.ID})
	p, pErr := projects.BootstrapSystem(tctx, o.ID, projSlug, projName)
	if pErr != nil {
		return false, fmt.Errorf("bootstrap first project: %w", pErr)
	}
	if log != nil {
		log.Info("bootstrap: first project ready",
			slog.String("slug", p.Slug),
		)
	}

	if _, err := onboards.Complete(ctx, u.ID, onboard.MethodEnvGenesis); err != nil {
		if onboard.IsAlreadyOnboardedError(err) {
			return true, nil
		}
		return false, fmt.Errorf("mark onboarded: %w", err)
	}
	if log != nil {
		log.Info("bootstrap: onboarded via env-genesis",
			slog.String("email", u.Email),
		)
	}
	return true, nil
}

func bootstrapOrgIdentity(cfg *config.Config) (slug, name string) { //nolint:nonamedreturns // pair
	slug = cfg.Tenant.SingletonOrg.Slug
	if slug == "" {
		slug = defaultBootstrapOrgSlug
	}
	name = cfg.Tenant.SingletonOrg.Name
	if name == "" {
		name = defaultBootstrapOrgName
	}
	return slug, name
}

func bootstrapProjectIdentity(cfg *config.Config) (slug, name string) { //nolint:nonamedreturns // pair
	slug = cfg.Tenant.PersonalProjectSlug
	if slug == "" {
		slug = defaultBootstrapProjectID
	}
	name = defaultBootstrapProjectStr
	return slug, name
}
