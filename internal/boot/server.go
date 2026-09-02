// Package boot is the composition root: it wires the platform Kernel plus every domain service under one worker Supervisor.
package boot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"altalune.id/template/internal/password"

	"altalune.id/template/authl"
	"altalune.id/template/internal/api"
	"altalune.id/template/internal/apperror"
	"altalune.id/template/internal/auth"
	i18npkg "altalune.id/template/internal/i18n"
	"altalune.id/template/internal/invite"
	"altalune.id/template/internal/onboard"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform"
	"altalune.id/template/internal/platform/capabilities"
	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/notify"
	"altalune.id/template/internal/platform/session"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/platform/tokens"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/todo"
	"altalune.id/template/internal/user"
	"altalune.id/template/internal/web"
	webhandlers "altalune.id/template/internal/web/handlers"
	webmw "altalune.id/template/internal/web/middleware"
	"altalune.id/template/logger"
	"altalune.id/template/mailer"
	"altalune.id/template/nanoid"
	"altalune.id/template/telemetry"
	"altalune.id/template/worker"
)

// Server is the fully-wired dependency graph produced by BootServer.
type Server struct {
	Cfg      *config.Config
	Caps     capabilities.Capabilities
	Platform *platform.Kernel

	Auth     *auth.Service
	Users    *user.Service
	Orgs     *org.Service
	Projects *project.Service
	Todos    *todo.Service
	Invites  *invite.Service
	Onboards *onboard.Service

	Onboard *user.OnboardWorkflow

	Onboarded bool

	Web        http.Handler
	API        *api.Server
	Supervisor *worker.Supervisor

	shutdownOTel func(context.Context) error
}

// BootServer builds every dependency and returns a wired [Server].
func BootServer(ctx context.Context, cfg *config.Config) (*Server, error) {
	log := logger.New(cfg.Log)

	tp, mp, shutdownOTel, err := telemetry.Setup(ctx, cfg.Telemetry, log)
	if err != nil {
		return nil, fmt.Errorf("boot: telemetry: %w", err)
	}

	mail, err := mailer.New(mailerConfig(cfg.Mail))
	if err != nil {
		_ = shutdownOTel(context.Background())
		return nil, fmt.Errorf("boot: mailer: %w", err)
	}

	sinks := notify.Build(cfg.Observability.Reporter, mail, log)
	reporter := apperror.NewReporter(log, cfg.Mode.IsProduction(),
		apperror.WithContextMeta(tenant.ContextMeta),
		apperror.WithSinks(sinks...),
	)

	pool, pgConn, err := openDBAndMigrate(ctx, cfg, log)
	if err != nil {
		_ = shutdownOTel(context.Background())
		return nil, err
	}

	verifier, err := tokens.NewVerifier(ctx, cfg.Tokens)
	if err != nil {
		_ = pool.Close()
		_ = shutdownOTel(context.Background())
		return nil, fmt.Errorf("boot: tokens: %w", err)
	}

	altAuth, err := buildAltAuth(ctx, cfg, log)
	if err != nil {
		_ = pool.Close()
		_ = shutdownOTel(context.Background())
		return nil, fmt.Errorf("boot: authl: %w", err)
	}

	sessions := session.NewMemoryStore()
	caps := capabilities.From(cfg)

	kernel := &platform.Kernel{
		Pool:     pool,
		PgConn:   pgConn,
		Log:      log,
		Reporter: reporter,
		Sessions: sessions,
		Verifier: verifier,
		Mail:     mail,
		AltAuth:  altAuth,
		Tracer:   tp.Tracer("altalune.id/template"),
		Meter:    mp.Meter("altalune.id/template"),
		Notify:   sinks,
		Nano:     nanoid.New,
		Caps:     caps,
	}
	for _, s := range sinks {
		if c, ok := s.(io.Closer); ok {
			kernel.AddCloser(c)
		}
	}
	kernel.AddCloser(pool)

	userStore := user.NewStore(cfg.DB, pool)
	orgStore := org.NewStore(cfg.DB, pool, pgConn)
	projectStore := project.NewStore(cfg.DB, pool, pgConn)
	todoStore := todo.NewStore(cfg.DB, pool, pgConn)
	inviteStore := invite.NewStore(cfg.DB, pool, pgConn)
	onboardStore := onboard.NewStore(cfg.DB, pool)

	orgs := org.NewService(orgStore, caps, log, reporter.Unexpected)
	projects := project.NewService(projectStore, log, reporter.Unexpected)
	todos := todo.NewService(todoStore, log, reporter.Unexpected)
	onboards := onboard.NewService(onboardStore, log, reporter.Unexpected)

	invitesEnabled := cfg.Mode == config.ModeCloud || cfg.OIDC.Issuer != ""

	sendWorkflow := invite.NewSendWorkflow(
		inviteStore,
		mail,
		strings.TrimRight(cfg.HTTP.BaseURL, "/")+cfg.HTTP.BasePath,
		log,
		reporter.Unexpected,
	)
	acceptWorkflow := invite.NewAcceptWorkflow(
		inviteStore,
		userStoreForInvite{store: userStore},
		orgStoreForInvite{store: orgStore},
		log,
		reporter.Unexpected,
	)
	invites := invite.NewService(inviteStore, sendWorkflow, acceptWorkflow, invitesEnabled, log, reporter.Unexpected)

	users := user.NewService(
		userStore,
		user.GenesisConfig{Email: cfg.Genesis.Email, Name: cfg.Genesis.Email},
		log,
		reporter.Unexpected,
		user.WithInviteFinder(invites),
	)

	onboardWorkflow := user.NewOnboardWorkflow(
		userStore,
		orgStoreForOnboard{store: orgStore},
		projectStoreForOnboard{store: projectStore},
		inviteStoreForOnboard{store: inviteStore},
		onboardPolicyFrom(cfg),
		log,
		reporter.Unexpected,
	)

	genesisHash, err := hashGenesisPassword(cfg.Genesis.Password)
	if err != nil {
		_ = pool.Close()
		_ = shutdownOTel(context.Background())
		return nil, fmt.Errorf("boot: hash genesis password: %w", err)
	}
	local := auth.NewLocalLogin(
		userStoreForAuth{store: userStore},
		auth.Genesis{
			Email:        cfg.Genesis.Email,
			PasswordHash: genesisHash,
			Name:         cfg.Genesis.Email,
		},
		log,
		reporter.Unexpected,
		auth.WithLocalNotFound(user.IsNotFoundError),
	)

	oidcOpts := []auth.OIDCOption{
		auth.WithSignupRequired(user.IsSignupRequiredError),
	}
	if cfg.Mode == config.ModeSelfhosted {
		oidcOpts = append(oidcOpts, auth.WithAllowSignup(func(ctx context.Context, email string) error {
			if req, rerr := onboards.Required(ctx); rerr == nil && req {
				return nil
			}
			return users.CheckOIDCSignupEligibility(ctx, email)
		}))
	}
	oidcLogin := auth.NewOIDCLogin(
		func(ctx context.Context, claims auth.EnsureClaims) (*auth.UserRef, bool, error) {
			u, err := users.EnsureFromOIDC(ctx, user.Claims(claims))
			if err != nil {
				return nil, false, err
			}
			return &auth.UserRef{ID: u.ID, Email: u.Email, Name: u.Name, Source: u.Source, Locale: u.Locale, TermsAcceptedAt: u.TermsAcceptedAt}, false, nil
		},
		func(ctx context.Context, req auth.OnboardRequest) (auth.OnboardResult, error) {
			res, err := onboardWorkflow.Onboard(ctx, req.UserID, req.Email)
			if err != nil {
				return auth.OnboardResult{}, err
			}
			return auth.OnboardResult{OrgID: res.OrgID, ProjectID: res.ProjectID}, nil
		},
		log,
		reporter.Unexpected,
		oidcOpts...,
	)

	auths := auth.NewService(local, oidcLogin, log, reporter.Unexpected)

	onboarded, err := bootstrap(ctx, cfg, users, orgs, projects, onboards, log)
	if err != nil {
		_ = pool.Close()
		_ = shutdownOTel(context.Background())
		return nil, fmt.Errorf("boot: bootstrap: %w", err)
	}
	caps.OnboardingRequired = !onboarded
	if !caps.LocalIdentity {
		has, hErr := users.HasLocalUsers(ctx)
		if hErr != nil {
			log.Warn("boot: HasLocalUsers probe failed", slog.String("err", hErr.Error()))
		} else if has {
			caps.LocalIdentity = true
		}
	}
	kernel.Caps = caps

	sup := worker.New(log)
	if cfg.Telemetry.Metrics.Prometheus.Enabled {
		sup.Register(telemetry.PrometheusWorker(cfg.Telemetry.Metrics.Prometheus, log))
	}

	apiSrv := api.New(cfg, kernel, auths, users, orgs, projects, todos, invites, todoStore)
	var apiHandler http.Handler
	if cfg.API.Enabled {
		apiHandler = apiSrv.Handler(cfg.HTTP.BasePath)
	}

	bundle, defaultLoc, err := buildI18nBundle(cfg)
	if err != nil {
		_ = pool.Close()
		_ = shutdownOTel(context.Background())
		return nil, err
	}

	required := &atomic.Bool{}
	required.Store(!onboarded)
	webHandler := buildWebHandler(cfg, kernel, caps, log, reporter,
		auths, users, orgs, projects, todos, invites, onboards, required, apiHandler, bundle, defaultLoc)

	sup.Register(worker.HTTP("http", cfg.HTTP.Addr, webHandler, log))

	return &Server{
		Cfg:          cfg,
		Caps:         caps,
		Platform:     kernel,
		Auth:         auths,
		Users:        users,
		Orgs:         orgs,
		Projects:     projects,
		Todos:        todos,
		Invites:      invites,
		Onboards:     onboards,
		Onboarded:    onboarded,
		Onboard:      onboardWorkflow,
		Web:          webHandler,
		API:          apiSrv,
		Supervisor:   sup,
		shutdownOTel: shutdownOTel,
	}, nil
}

func buildWebHandler(
	cfg *config.Config,
	kernel *platform.Kernel,
	caps capabilities.Capabilities,
	slogger *slog.Logger,
	reporter *apperror.Reporter,
	auths *auth.Service,
	users *user.Service,
	orgs *org.Service,
	projects *project.Service,
	todos *todo.Service,
	invites *invite.Service,
	onboards *onboard.Service,
	required *atomic.Bool,
	apiHandler http.Handler,
	bundle *i18npkg.Bundle,
	defaultLoc i18npkg.Locale,
) http.Handler {
	deps := newWebDeps(cfg, caps, kernel.Sessions, slogger)
	deps.Orgs = orgs
	deps.Projects = projects
	deps.I18n = bundle

	authHandler := webhandlers.NewAuthHandler(deps, auths, users, orgs, projects, kernel.AltAuth, required)
	onboardingHandler := webhandlers.NewOnboardingHandler(deps, users)
	onboardHandler := webhandlers.NewOnboardHandler(deps, users, orgs, projects, onboards, required)
	homeHandler := webhandlers.NewHomeHandler(deps, orgs, projects)
	orgHandler := webhandlers.NewOrgHandler(deps, orgs)
	projectHandler := webhandlers.NewProjectHandler(deps, projects)
	todoHandler := webhandlers.NewTodoHandler(deps, projects, todos)
	inviteHandler := webhandlers.NewInviteHandler(deps, orgs, invites)
	localeHandler := webhandlers.NewLocaleHandler(deps, users)
	welcomeHandler := webhandlers.NewWelcomeHandler(deps, users)
	signupHandler := webhandlers.NewSignupHandler(deps, users, orgs, projects)
	legalHandler := webhandlers.NewLegalHandler(deps)

	errTmpl := webmw.LogError{Log: slogger}

	return web.NewServer(web.ServerOpts{
		BasePath: cfg.HTTP.BasePath,
		AppHandlers: []web.Register{
			authHandler, onboardingHandler, onboardHandler, homeHandler, orgHandler, projectHandler, todoHandler, inviteHandler, localeHandler, welcomeHandler, signupHandler, legalHandler,
		},
		APIHandler: apiHandler,
		RobotsCfg:  &struct{ RobotsTxt string }{RobotsTxt: cfg.HTTP.RobotsTxt},
		Middlewares: []web.Middleware{
			webmw.RequestID,
			webmw.RequestLog(slogger),
			webmw.OTel,
			webmw.Recover(reporter.Unexpected, errTmpl),
			webmw.Session(webmw.SessionConfig{
				Store:  kernel.Sessions,
				Secret: []byte(cfg.HTTP.StateSecret),
			}),
			i18npkg.Middleware(i18npkg.MiddlewareOpts{
				Bundle:     bundle,
				Default:    defaultLoc,
				UserLookup: sessionLocaleLookup,
			}),
			webhandlers.OnboardingGate(cfg.HTTP.BasePath, required),
			webhandlers.WelcomeGate(cfg.HTTP.BasePath, cfg.Compliance.RequireAcceptance),
		},
	})
}

func buildI18nBundle(cfg *config.Config) (*i18npkg.Bundle, i18npkg.Locale, error) {
	tag := cfg.I18n.DefaultLocale
	if tag == "" {
		tag = string(i18npkg.EnUS)
	}
	tmp := i18npkg.NewEmbeddedBundle(i18npkg.EnUS)
	loc, err := tmp.Parse(tag)
	if err != nil {
		return nil, "", fmt.Errorf("i18n: default locale %q not among embedded locales", tag)
	}
	return i18npkg.NewEmbeddedBundle(loc), loc, nil
}

func sessionLocaleLookup(ctx context.Context) string {
	return session.PrincipalFrom(ctx).Locale
}

func newWebDeps(cfg *config.Config, caps capabilities.Capabilities, sessions session.Store, slogger *slog.Logger) webhandlers.Deps {
	return webhandlers.Deps{
		Cfg:      cfg,
		Caps:     caps,
		Sessions: sessions,
		Logger:   stdlog.New(logSlogWriter{log: slogger}, "", 0),
	}
}

type logSlogWriter struct{ log *slog.Logger }

func (w logSlogWriter) Write(p []byte) (int, error) {
	if w.log != nil {
		w.log.Info(strings.TrimRight(string(p), "\n"))
	}
	return len(p), nil
}

// Run starts every registered worker and blocks until ctx is cancelled or a worker fails.
func (s *Server) Run(ctx context.Context) error {
	return s.Supervisor.Run(ctx)
}

// Close shuts OTel down and then releases every platform resource.
func (s *Server) Close() error {
	var errs []error
	if s.shutdownOTel != nil {
		if err := s.shutdownOTel(context.Background()); err != nil {
			errs = append(errs, err)
		}
	}
	if s.Platform != nil {
		if err := s.Platform.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NOTE: path must match "GET /oauth/callback" in internal/web/handlers/auth.go.
func oidcRedirectURL(cfg *config.Config) string {
	if cfg.HTTP.BaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s%s/oauth/callback",
		strings.TrimRight(cfg.HTTP.BaseURL, "/"), cfg.HTTP.BasePath)
}

func buildAltAuth(ctx context.Context, cfg *config.Config, log *slog.Logger) (*authl.Client, error) {
	if cfg.OIDC.Issuer == "" {
		return nil, nil
	}
	secret, err := resolveStateSecret(cfg, log)
	if err != nil {
		return nil, err
	}
	redirect := oidcRedirectURL(cfg)
	return authl.NewClient(ctx, authl.Config{
		Issuer:           cfg.OIDC.Issuer,
		ClientID:         cfg.OIDC.ClientID,
		ClientSecret:     cfg.OIDC.ClientSecret,
		RedirectURL:      redirect,
		Scopes:           cfg.OIDC.Scopes,
		Resource:         cfg.OIDC.Resource,
		RememberLastUser: true,
		LastUserCookie:   "altempl_last_user",
		StateCookie:      "altempl_oidc_state",
		StateSecret:      secret,
		CookieSecure:     cfg.HTTP.CookieSecure,
	})
}

func resolveStateSecret(cfg *config.Config, log *slog.Logger) ([]byte, error) {
	raw := cfg.HTTP.StateSecret
	if raw != "" {
		buf, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			if b2, err2 := base64.StdEncoding.DecodeString(raw); err2 == nil {
				buf = b2
			} else {
				return nil, fmt.Errorf("config: http.stateSecret must be base64url — %w", err)
			}
		}
		if len(buf) < 32 {
			return nil, errors.New("config: http.stateSecret must decode to >= 32 bytes")
		}
		return buf, nil
	}
	ephemeral, err := authl.GenerateStateSecret()
	if err != nil {
		return nil, fmt.Errorf("mint ephemeral state secret: %w", err)
	}
	log.Warn("http.stateSecret is empty — using an ephemeral secret; set ALT_HTTP_STATE_SECRET to persist")
	return ephemeral, nil
}

func onboardPolicyFrom(cfg *config.Config) user.Policy {
	policyMode := user.PolicyModeCloud
	if cfg.Mode == config.ModeSelfhosted {
		policyMode = user.PolicyModeSelfhosted
	}
	return user.Policy{
		Mode:             policyMode,
		SingletonOrgSlug: cfg.Tenant.SingletonOrg.Slug,
	}
}

func hashGenesisPassword(plain string) (string, error) {
	if strings.TrimSpace(plain) == "" {
		return "", nil
	}
	return password.Hash(plain)
}

func mailerConfig(m config.MailConfig) mailer.Config {
	return mailer.Config{
		Driver: m.Driver,
		From:   m.From,
		SMTP: mailer.SMTPConfig{
			Host: m.SMTP.Host,
			Port: m.SMTP.Port,
			User: m.SMTP.User,
			Pass: m.SMTP.Pass,
			TLS:  m.SMTP.TLS,
		},
	}
}
