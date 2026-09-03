// Package handlers hosts the HTTP handlers behind the templ-rendered pages.
package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"altalune.id/template/internal/i18n"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/capabilities"
	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/session"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/web"
	"altalune.id/template/internal/web/templates"
	"altalune.id/template/version"
)

// Deps is the common bag of dependencies every handler in this package needs.
type Deps struct {
	Cfg      *config.Config
	Caps     capabilities.Capabilities
	Sessions session.Store
	Logger   *log.Logger
	Orgs     *org.Service
	Projects *project.Service
	I18n     *i18n.Bundle
}

// Base builds a minimal LayoutData for chromeless pages (login, onboarding, error).
func (d Deps) Base(r *http.Request, title string) web.LayoutData {
	p := session.PrincipalFrom(r.Context())
	var pp *session.Principal
	if p.UserID != uuid.Nil {
		pp = &p
	}
	loc := i18n.From(r.Context())
	dir := loc.Dir()
	var supported []i18n.Locale
	if d.I18n != nil {
		dir = d.I18n.Dir(loc)
		supported = d.I18n.All()
	}
	return web.LayoutData{
		Title:         title,
		BasePath:      d.Cfg.HTTP.BasePath,
		BaseURL:       d.Cfg.HTTP.BaseURL,
		Version:       version.Default(),
		UIMode:        web.ResolveUIMode(),
		Caps:          d.Caps,
		Principal:     pp,
		Locale:        loc,
		Dir:           dir,
		Translator:    i18n.TranslatorFrom(r.Context()),
		CurrentPath:   r.URL.RequestURI(),
		SupportedLocs: supported,
		Themes:        web.Themes(),
		ColorModes:    web.ColorModes(),
	}
}

// Layout builds a LayoutData for a signed-in page and populates org/project switchers.
func (d Deps) Layout(r *http.Request, title string, nav web.ActiveNav) web.LayoutData {
	base := d.Base(r, title)
	if base.Principal == nil {
		return base
	}
	base.ActiveNav = nav
	ctx := r.Context()
	p := *base.Principal
	if d.Orgs != nil {
		orgs, err := d.Orgs.List(ctx, p.UserID)
		if err != nil {
			d.LogErr("layout: list orgs", err)
		}
		base.ActiveOrg, base.OtherOrgs = splitOrgs(orgs, p.ActiveOrgID)
	}
	if base.ActiveOrg != nil && d.Projects != nil && p.ActiveOrgID != uuid.Nil {
		tctx := tenant.Into(ctx, tenant.Context{OrgID: p.ActiveOrgID, UserID: p.UserID, ProjectID: p.ActiveProjectID})
		projects, err := d.Projects.List(tctx, p.ActiveOrgID)
		if err != nil {
			d.LogErr("layout: list projects", err)
		}
		base.ActiveProject, base.OtherProjects = splitProjects(projects, p.ActiveProjectID)
	}
	if base.ActiveOrg != nil && d.Orgs != nil {
		if m, err := d.Orgs.MembershipOf(ctx, uuidFromString(base.ActiveOrg.ID), p.UserID); err == nil && m != nil {
			base.ActiveOrg.Role = capitaliseRole(string(m.Role))
		}
	}
	return base
}

// LayoutForOrg tags a page as org-scoped and pins the pill to the requested slug.
func (d Deps) LayoutForOrg(r *http.Request, title, slug, orgKey string) web.LayoutData {
	l := d.Layout(r, title, web.ActiveNav{Scope: web.NavScopeOrg, OrgKey: orgKey})
	if slug == "" || d.Orgs == nil {
		return l
	}
	if l.ActiveOrg != nil && l.ActiveOrg.Slug == slug {
		return l
	}
	o, err := d.Orgs.BySlug(r.Context(), slug)
	if err != nil || o == nil {
		return l
	}
	l.ActiveOrg = &web.ActiveOrg{ID: o.ID.String(), Slug: o.Slug, Name: o.Name}
	return l
}

// LayoutForProject tags a page as project-scoped and pins both pills to the given project.
func (d Deps) LayoutForProject(r *http.Request, title, projectSlug, projectName, projectID, projectKey string) web.LayoutData {
	l := d.Layout(r, title, web.ActiveNav{Scope: web.NavScopeProject, ProjectKey: projectKey})
	if l.ActiveProject == nil || l.ActiveProject.Slug != projectSlug {
		l.ActiveProject = &web.ActiveProject{ID: projectID, Slug: projectSlug, Name: projectName}
	}
	return l
}

func splitOrgs(items []*org.Org, activeID uuid.UUID) (active *web.ActiveOrg, others []web.ActiveOrg) { //nolint:nonamedreturns // two return values differ in role
	others = make([]web.ActiveOrg, 0, len(items))
	for _, o := range items {
		summary := web.ActiveOrg{ID: o.ID.String(), Slug: o.Slug, Name: o.Name}
		if o.ID == activeID {
			a := summary
			active = &a
			continue
		}
		others = append(others, summary)
	}
	if active == nil && len(items) > 0 {
		a := web.ActiveOrg{ID: items[0].ID.String(), Slug: items[0].Slug, Name: items[0].Name}
		active = &a
		others = others[:0]
		for _, o := range items[1:] {
			others = append(others, web.ActiveOrg{ID: o.ID.String(), Slug: o.Slug, Name: o.Name})
		}
	}
	return active, others
}

func splitProjects(items []*project.Project, activeID uuid.UUID) (active *web.ActiveProject, others []web.ActiveProject) { //nolint:nonamedreturns // two return values differ in role
	others = make([]web.ActiveProject, 0, len(items))
	for _, p := range items {
		summary := web.ActiveProject{ID: p.ID.String(), Slug: p.Slug, Name: p.Name}
		if p.ID == activeID {
			a := summary
			active = &a
			continue
		}
		others = append(others, summary)
	}
	return active, others
}

func uuidFromString(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func capitaliseRole(role string) string {
	if role == "" {
		return ""
	}
	return strings.ToUpper(role[:1]) + strings.ToLower(role[1:])
}

// SecretBytes returns cfg.HTTP.StateSecret as bytes.
func (d Deps) SecretBytes() []byte { return []byte(d.Cfg.HTTP.StateSecret) }

// LogErr prints via the injected logger (or the stdlib default when nil).
func (d Deps) LogErr(msg string, err error) {
	if err == nil {
		return
	}
	if d.Logger != nil {
		d.Logger.Printf("%s: %v", msg, err)
		return
	}
	log.Printf("%s: %v", msg, err)
}

// Render writes a templ.Component with a 200 status.
func Render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}

// RenderStatus writes a templ.Component with an explicit status.
func RenderStatus(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}

// ErrorPage renders the error.templ full page with the given status/title/message.
func (d Deps) ErrorPage(w http.ResponseWriter, r *http.Request, status int, title, msg string) {
	base := d.Base(r, title)
	RenderStatus(w, r, status, templates.ErrorLayout(base, templates.ErrorView{
		Status: status, Title: title, Message: msg,
	}))
}

// LoadSession reads the sid cookie, verifies its HMAC, and loads the Principal from the store.
func (d Deps) LoadSession(r *http.Request) (session.Principal, string, bool) {
	c, err := r.Cookie(web.SessionCookieName)
	if err != nil {
		return session.Principal{}, "", false
	}
	sid, err := web.VerifyCookie(d.SecretBytes(), c.Value)
	if err != nil {
		return session.Principal{}, "", false
	}
	p, ok, err := d.Sessions.Load(r.Context(), sid)
	if err != nil || !ok {
		return session.Principal{}, "", false
	}
	return p, sid, true
}

// WriteSession saves the Principal under a fresh sid and writes the signed cookie.
func (d Deps) WriteSession(w http.ResponseWriter, r *http.Request, p session.Principal) error {
	sid, err := web.NewSID()
	if err != nil {
		return err
	}
	if err := d.Sessions.Save(r.Context(), sid, p, time.Now().Add(web.SessionTTL)); err != nil {
		return err
	}
	web.SetCookie(w, web.CookieOpts{
		Name:         web.SessionCookieName,
		Value:        web.SignCookie(d.SecretBytes(), sid),
		BasePath:     d.Cfg.HTTP.BasePath,
		CookieSecure: d.Cfg.HTTP.CookieSecure,
		MaxAge:       int(web.SessionTTL.Seconds()),
	})
	return nil
}

// UpdateSession re-saves the Principal under the existing sid, preserving the cookie.
func (d Deps) UpdateSession(r *http.Request, sid string, p session.Principal) error {
	return d.Sessions.Save(r.Context(), sid, p, time.Now().Add(web.SessionTTL))
}

// ClearSession deletes the store row + expires the cookie.
func (d Deps) ClearSession(w http.ResponseWriter, r *http.Request) {
	if _, sid, ok := d.LoadSession(r); ok {
		_ = d.Sessions.Delete(r.Context(), sid)
	}
	web.ClearCookie(w, web.SessionCookieName, d.Cfg.HTTP.BasePath, d.Cfg.HTTP.CookieSecure)
}

// SanitizeReturnTo rejects any return_to that leaves the same-origin.
func SanitizeReturnTo(raw string) string {
	if raw == "" {
		return ""
	}
	if raw[0] != '/' {
		return ""
	}
	if len(raw) >= 2 && (raw[1] == '/' || raw[1] == '\\') {
		return ""
	}
	if strings.ContainsRune(raw, '\\') {
		return ""
	}
	return raw
}

// ResolveReturnTo returns a safe destination, defaulting to basePath+"/" when raw is unsafe.
func ResolveReturnTo(basePath, raw string) string {
	if s := SanitizeReturnTo(raw); s != "" {
		return web.Path(basePath, s)
	}
	return web.Path(basePath, "/")
}
