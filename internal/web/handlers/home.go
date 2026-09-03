package handlers

import (
	"net/http"

	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/web"
	"altalune.id/template/internal/web/templates"
)

// HomeHandler serves the signed-in dashboard at `/`.
type HomeHandler struct{ Deps }

// NewHomeHandler wires the handler.
func NewHomeHandler(d Deps, orgs *org.Service, projects *project.Service) *HomeHandler {
	d.Orgs = orgs
	d.Projects = projects
	return &HomeHandler{Deps: d}
}

// Register wires GET / onto mux.
func (h *HomeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.GetHome)
}

// GetHome redirects unauthenticated visitors to /login and renders the dashboard otherwise.
func (h *HomeHandler) GetHome(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.LoadSession(r)
	if !ok || p.UserID == [16]byte{} {
		http.Redirect(w, r, web.Path(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	view := templates.DashboardView{
		ActiveProjID: p.ActiveProjectID.String(),
	}
	if h.Orgs != nil {
		if orgs, err := h.Orgs.List(ctx, p.UserID); err == nil {
			for _, o := range orgs {
				view.Orgs = append(view.Orgs, templates.OrgSummary{ID: o.ID.String(), Slug: o.Slug, Name: o.Name, System: o.System})
				if o.ID == p.ActiveOrgID {
					view.OrgName = o.Name
					view.OrgSlug = o.Slug
				}
			}
		}
	}
	if p.ActiveOrgID != [16]byte{} && h.Projects != nil {
		tctx := tenant.Into(ctx, tenant.Context{OrgID: p.ActiveOrgID, UserID: p.UserID, ProjectID: p.ActiveProjectID})
		projects, err := h.Projects.List(tctx, p.ActiveOrgID)
		if err != nil {
			h.LogErr("home: list projects", err)
		}
		for _, pr := range projects {
			view.Projects = append(view.Projects, templates.ProjectSummary{ID: pr.ID.String(), Slug: pr.Slug, Name: pr.Name, System: pr.System})
		}
	}
	Render(w, r, templates.DashboardLayout(h.Layout(r, "Dashboard", web.ActiveNav{Scope: web.NavScopeOrg, OrgKey: "overview"}), view))
}
