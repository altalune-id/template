package handlers

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/web"
	"altalune.id/template/internal/web/templates"
)

// ProjectHandler owns the /projects routes.
type ProjectHandler struct{ Deps }

// NewProjectHandler wires the handler.
func NewProjectHandler(d Deps, projects *project.Service) *ProjectHandler {
	d.Projects = projects
	return &ProjectHandler{Deps: d}
}

// GetList renders /projects.
func (h *ProjectHandler) GetList(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.LoadSession(r)
	if !ok {
		http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return
	}
	ctx := tenant.Into(r.Context(), tenant.Context{OrgID: p.ActiveOrgID, UserID: p.UserID})
	items, err := h.Projects.List(ctx, p.ActiveOrgID)
	if err != nil {
		h.LogErr("web project: list", err)
		h.ErrorPage(w, r, http.StatusInternalServerError, "List failed", "Could not load projects.")
		return
	}
	Render(w, r, templates.ProjectsLayout(h.Layout(r, "Projects", web.ActiveNav{Scope: web.NavScopeOrg, OrgKey: "projects"}), templates.ProjectsView{Projects: projectSummaries(items)}))
}

// GetNew renders /projects/new.
func (h *ProjectHandler) GetNew(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.LoadSession(r)
	if !ok {
		http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return
	}
	if p.ActiveOrgID == uuid.Nil {
		h.ErrorPage(w, r, http.StatusPreconditionRequired, "No active organisation", "Pick or create an organisation first.")
		return
	}
	Render(w, r, templates.ProjectNewLayout(h.Layout(r, "Create project", web.ActiveNav{Scope: web.NavScopeOrg, OrgKey: "projects"}), templates.ProjectNewView{}))
}

// PostCreate handles POST /projects.
func (h *ProjectHandler) PostCreate(w http.ResponseWriter, r *http.Request) {
	p, sid, ok := h.LoadSession(r)
	if !ok {
		http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return
	}
	if p.ActiveOrgID == uuid.Nil {
		h.ErrorPage(w, r, http.StatusPreconditionRequired, "No active organisation", "Pick or create an organisation first.")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad request", "Could not parse form body.")
		return
	}
	slug := strings.TrimSpace(r.PostForm.Get("slug"))
	name := strings.TrimSpace(r.PostForm.Get("name"))
	ctx := tenant.Into(r.Context(), tenant.Context{OrgID: p.ActiveOrgID, UserID: p.UserID})
	created, err := h.Projects.Create(ctx, p.ActiveOrgID, slug, name)
	if err != nil {
		h.LogErr("web project: create", err)
		msg := err.Error()
		if project.IsAlreadyExistsError(err) {
			msg = "Slug is already taken."
		}
		Render(w, r, templates.ProjectNewLayout(
			h.Layout(r, "Create project", web.ActiveNav{Scope: web.NavScopeOrg, OrgKey: "projects"}),
			templates.ProjectNewView{Slug: slug, Name: name, Error: msg},
		))
		return
	}
	updated := p
	updated.ActiveProjectID = created.ID
	if err := h.UpdateSession(r, sid, updated); err != nil {
		h.LogErr("web project: update session", err)
	}
	http.Redirect(w, r, web.Path(h.Cfg.HTTP.BasePath, "/projects/"+created.Slug+"/overview"), http.StatusSeeOther) //nolint:gosec // G710: slug is validated by project.Create's slug pattern
}

// PostRename handles POST /projects/{slug}/rename.
func (h *ProjectHandler) PostRename(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.LoadSession(r)
	if !ok {
		http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return
	}
	slug := r.PathValue("slug")
	if err := r.ParseForm(); err != nil {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad request", "Could not parse form body.")
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	ctx := tenant.Into(r.Context(), tenant.Context{OrgID: p.ActiveOrgID, UserID: p.UserID})
	proj, err := h.Projects.BySlug(ctx, p.ActiveOrgID, slug)
	if err != nil {
		h.ErrorPage(w, r, http.StatusNotFound, "Project not found", "")
		return
	}
	if _, err := h.Projects.Rename(ctx, proj.ID, name); err != nil {
		h.LogErr("web project: rename", err)
		if project.IsSystemProtectedError(err) {
			h.ErrorPage(w, r, http.StatusConflict, "Rename not allowed", "This project is system-protected.")
			return
		}
		h.ErrorPage(w, r, http.StatusBadRequest, "Rename failed", err.Error())
		return
	}
	http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/projects"), http.StatusSeeOther)
}

func projectSummaries(items []*project.Project) []templates.ProjectSummary {
	out := make([]templates.ProjectSummary, 0, len(items))
	for _, p := range items {
		out = append(out, templates.ProjectSummary{ID: p.ID.String(), Slug: p.Slug, Name: p.Name, System: p.System})
	}
	return out
}

// Register wires the project routes onto mux.
func (h *ProjectHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", h.GetList)
	mux.HandleFunc("GET /projects/new", h.GetNew)
	mux.HandleFunc("POST /projects", h.PostCreate)
	mux.HandleFunc("POST /projects/{slug}/rename", h.PostRename)
}
