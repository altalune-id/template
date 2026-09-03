package handlers

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"altalune.id/template/internal/platform/session"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/todo"
	"altalune.id/template/internal/web/templates"
)

// TodoHandler wraps the projects/todos services for the /projects/{slug}/todos routes.
type TodoHandler struct {
	Deps
	Todos *todo.Service
}

// NewTodoHandler wires the handler.
func NewTodoHandler(d Deps, projects *project.Service, todos *todo.Service) *TodoHandler {
	d.Projects = projects
	return &TodoHandler{Deps: d, Todos: todos}
}

func (h *TodoHandler) requireTenant(w http.ResponseWriter, r *http.Request) (session.Principal, string, *project.Project, bool) {
	p, sid, ok := h.LoadSession(r)
	if !ok {
		http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return session.Principal{}, "", nil, false
	}
	slug := r.PathValue("slug")
	if slug == "" {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad request", "Missing project slug.")
		return session.Principal{}, "", nil, false
	}
	ctx := tenant.Into(r.Context(), tenant.Context{
		OrgID:  p.ActiveOrgID,
		UserID: p.UserID,
	})
	proj, err := h.Projects.BySlug(ctx, p.ActiveOrgID, slug)
	if err != nil {
		h.ErrorPage(w, r, http.StatusNotFound, "Project not found", "No project with that slug in the active org.")
		return session.Principal{}, "", nil, false
	}
	return p, sid, proj, true
}

func withTenantCtx(r *http.Request, p session.Principal, projID uuid.UUID) *http.Request {
	ctx := tenant.Into(r.Context(), tenant.Context{
		OrgID:     p.ActiveOrgID,
		ProjectID: projID,
		UserID:    p.UserID,
	})
	return r.WithContext(ctx)
}

// GetOverview renders the project overview page.
func (h *TodoHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	p, sid, proj, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if p.ActiveProjectID != proj.ID {
		updated := p
		updated.ActiveProjectID = proj.ID
		if err := h.UpdateSession(r, sid, updated); err != nil {
			h.LogErr("web overview: update session", err)
		}
		p = updated
	}
	r = withTenantCtx(r, p, proj.ID)
	items, err := h.Todos.List(r.Context(), todo.ListOpts{})
	if err != nil {
		h.LogErr("web overview: list", err)
		h.ErrorPage(w, r, http.StatusInternalServerError, "Load failed", "Could not load project overview.")
		return
	}
	var open, done int
	for _, t := range items {
		if t.Done {
			done++
		} else {
			open++
		}
	}
	Render(w, r, templates.OverviewLayout(h.LayoutForProject(r, "Overview · "+proj.Name, proj.Slug, proj.Name, proj.ID.String(), "overview"), templates.OverviewView{
		ProjectID:   proj.ID.String(),
		ProjectSlug: proj.Slug,
		ProjectName: proj.Name,
		TotalTodos:  len(items),
		OpenTodos:   open,
		DoneTodos:   done,
	}))
}

// GetTodos renders the full page.
func (h *TodoHandler) GetTodos(w http.ResponseWriter, r *http.Request) {
	p, sid, proj, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if p.ActiveProjectID != proj.ID {
		updated := p
		updated.ActiveProjectID = proj.ID
		if err := h.UpdateSession(r, sid, updated); err != nil {
			h.LogErr("web todo: update session", err)
		}
		p = updated
	}
	r = withTenantCtx(r, p, proj.ID)
	items, err := h.Todos.List(r.Context(), todo.ListOpts{})
	if err != nil {
		h.LogErr("web todo: list", err)
		h.ErrorPage(w, r, http.StatusInternalServerError, "List failed", "Could not load todos.")
		return
	}
	Render(w, r, templates.TodosLayout(h.LayoutForProject(r, "Todos · "+proj.Name, proj.Slug, proj.Name, proj.ID.String(), "todos"), templates.TodosView{
		ProjectID:   proj.ID.String(),
		ProjectSlug: proj.Slug,
		ProjectName: proj.Name,
		Items:       renderRows(items),
	}))
}

// PostCreate creates a todo and returns the refreshed list fragment.
func (h *TodoHandler) PostCreate(w http.ResponseWriter, r *http.Request) {
	p, _, proj, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad request", "Could not parse form body.")
		return
	}
	title := strings.TrimSpace(r.PostForm.Get("title"))
	r = withTenantCtx(r, p, proj.ID)
	if _, err := h.Todos.Create(r.Context(), title); err != nil {
		h.LogErr("web todo: create", err)
	}
	h.writeListFragment(w, r)
}

func (h *TodoHandler) requireTodoTenant(w http.ResponseWriter, r *http.Request) (session.Principal, *todo.Todo, bool) {
	p, _, ok := h.LoadSession(r)
	if !ok {
		http.Redirect(w, r, ResolveReturnTo(h.Cfg.HTTP.BasePath, "/login"), http.StatusSeeOther)
		return session.Principal{}, nil, false
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad id", "Malformed todo id.")
		return session.Principal{}, nil, false
	}
	ctx := tenant.Into(r.Context(), tenant.Context{
		OrgID:  p.ActiveOrgID,
		UserID: p.UserID,
	})
	t, err := h.Todos.ByID(ctx, id)
	if err != nil {
		if todo.IsNotFoundError(err) {
			h.ErrorPage(w, r, http.StatusNotFound, "Not found", "That todo no longer exists.")
			return session.Principal{}, nil, false
		}
		h.LogErr("web todo: byID", err)
		h.ErrorPage(w, r, http.StatusInternalServerError, "Lookup failed", "Could not load that todo.")
		return session.Principal{}, nil, false
	}
	return p, t, true
}

// PostToggle flips done and returns the single-row partial.
func (h *TodoHandler) PostToggle(w http.ResponseWriter, r *http.Request) {
	p, t, ok := h.requireTodoTenant(w, r)
	if !ok {
		return
	}
	r = withTenantCtx(r, p, t.ProjectID)
	updated, err := h.Todos.Toggle(r.Context(), t.ID)
	if err != nil {
		if todo.IsNotFoundError(err) {
			h.ErrorPage(w, r, http.StatusNotFound, "Not found", "That todo no longer exists.")
			return
		}
		h.LogErr("web todo: toggle", err)
		h.ErrorPage(w, r, http.StatusInternalServerError, "Toggle failed", err.Error())
		return
	}
	Render(w, r, templates.TodoRowFragment(h.Base(r, ""), templates.TodoRow{
		ID: updated.ID.String(), Title: updated.Title, Done: updated.Done,
	}))
}

// Delete removes the row.
func (h *TodoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	p, t, ok := h.requireTodoTenant(w, r)
	if !ok {
		return
	}
	r = withTenantCtx(r, p, t.ProjectID)
	if err := h.Todos.Delete(r.Context(), t.ID); err != nil && !todo.IsNotFoundError(err) {
		h.LogErr("web todo: delete", err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// PostClear removes all done todos and returns the refreshed list fragment.
func (h *TodoHandler) PostClear(w http.ResponseWriter, r *http.Request) {
	p, _, proj, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	r = withTenantCtx(r, p, proj.ID)
	if _, err := h.Todos.ClearDone(r.Context()); err != nil {
		h.LogErr("web todo: clear", err)
	}
	h.writeListFragment(w, r)
}

func (h *TodoHandler) writeListFragment(w http.ResponseWriter, r *http.Request) {
	items, err := h.Todos.List(r.Context(), todo.ListOpts{})
	if err != nil {
		h.LogErr("web todo: list", err)
		h.ErrorPage(w, r, http.StatusInternalServerError, "List failed", "Could not load todos.")
		return
	}
	Render(w, r, templates.TodoList(h.Base(r, ""), renderRows(items)))
}

func renderRows(items []*todo.Todo) []templates.TodoRow {
	rows := make([]templates.TodoRow, 0, len(items))
	for _, t := range items {
		rows = append(rows, templates.TodoRow{ID: t.ID.String(), Title: t.Title, Done: t.Done})
	}
	return rows
}

// Register wires the routes onto mux.
func (h *TodoHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects/{slug}/overview", h.GetOverview)
	mux.HandleFunc("GET /projects/{slug}/todos", h.GetTodos)
	mux.HandleFunc("POST /projects/{slug}/todos", h.PostCreate)
	mux.HandleFunc("POST /projects/{slug}/todos/clear", h.PostClear)
	mux.HandleFunc("POST /todos/{id}/toggle", h.PostToggle)
	mux.HandleFunc("DELETE /todos/{id}", h.Delete)
	mux.HandleFunc("POST /todos/{id}/delete", h.Delete)
}
