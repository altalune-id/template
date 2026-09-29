package handlers

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/template/internal/apikey"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform/authn"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/web"
	"altalune.id/template/internal/web/templates"
)

// APIKeyHandler owns the project and org API key console pages.
type APIKeyHandler struct {
	Deps
	Keys *apikey.Service
}

// NewAPIKeyHandler wires the handler.
func NewAPIKeyHandler(d Deps, projects *project.Service, keys *apikey.Service) *APIKeyHandler {
	d.Projects = projects
	return &APIKeyHandler{Deps: d, Keys: keys}
}

// GetKeys renders the project API key list page.
func (h *APIKeyHandler) GetKeys(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.RequireProject(w, r)
	if !ok {
		return
	}
	v, err := h.projectView(sc, nil, "", "")
	if err != nil {
		h.LogErr("web apikey: list", err)
		h.ErrorPage(w, sc.req, http.StatusInternalServerError, "List failed", "Could not load API keys.", err)
		return
	}
	Render(w, sc.req, templates.APIKeysLayout(
		h.LayoutForProject(sc.req, "API keys · "+sc.project.Name, sc.org.Slug, sc.project, "apikeys"),
		v,
	))
}

// PostKeyCreate mints a project key and renders the refreshed list with the one-time plaintext reveal.
func (h *APIKeyHandler) PostKeyCreate(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.RequireProject(w, r)
	if !ok {
		return
	}
	in, ok := h.parseMint(w, sc.req)
	if !ok {
		return
	}
	if in.badExpiry {
		h.writeProjectList(w, sc, nil, templates.APIKeyErrorInvalidExpiry, "")
		return
	}
	k, plaintext, err := h.Keys.Mint(sc.req.Context(), in.name, in.scopes, nil, in.expiresAt)
	if err != nil {
		h.LogErr("web apikey: mint", err)
		h.writeProjectList(w, sc, nil, apiKeyErrorKind(err), ErrorRef(err))
		return
	}
	// SECURITY: the plaintext is rendered once and discarded — never logged, stored on a row, or put in the session.
	h.writeProjectList(w, sc, &templates.APIKeyMinted{Name: k.Name, Plaintext: plaintext}, "", "")
}

// PostKeyRevoke revokes a project key and returns the refreshed list fragment.
func (h *APIKeyHandler) PostKeyRevoke(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.RequireProject(w, r)
	if !ok {
		return
	}
	id, ok := h.keyID(w, sc.req)
	if !ok {
		return
	}
	if err := h.Keys.Revoke(sc.req.Context(), id); err != nil && !apikey.IsNotFoundError(err) {
		h.LogErr("web apikey: revoke", err)
		h.writeProjectList(w, sc, nil, apiKeyErrorKind(err), ErrorRef(err))
		return
	}
	h.writeProjectList(w, sc, nil, "", "")
}

// GetOrgKeys renders the org API key list page.
func (h *APIKeyHandler) GetOrgKeys(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.RequireOrg(w, r)
	if !ok {
		return
	}
	v, err := h.orgView(sc, nil, "", "")
	if err != nil {
		h.LogErr("web apikey: list org", err)
		h.ErrorPage(w, sc.req, http.StatusInternalServerError, "List failed", "Could not load API keys.", err)
		return
	}
	Render(w, sc.req, templates.APIKeysLayout(h.LayoutForOrg(sc.req, "API keys · "+sc.org.Name, sc.org.Slug, "apikeys"), v))
}

// PostOrgKeyCreate mints an org key and renders the refreshed list with the one-time plaintext reveal.
func (h *APIKeyHandler) PostOrgKeyCreate(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.RequireOrg(w, r)
	if !ok {
		return
	}
	in, ok := h.parseMint(w, sc.req)
	if !ok {
		return
	}
	if in.badExpiry {
		h.writeOrgList(w, sc, nil, templates.APIKeyErrorInvalidExpiry, "")
		return
	}
	grant := apikey.ProjectGrant{All: sc.req.PostForm.Get("grant") == "all"}
	if !grant.All {
		grant.ProjectIDs = formUUIDs(sc.req.PostForm["project_ids"])
	}
	k, plaintext, err := h.Keys.MintOrg(sc.req.Context(), in.name, in.scopes, grant, in.expiresAt)
	if err != nil {
		h.LogErr("web apikey: mint org", err)
		h.writeOrgList(w, sc, nil, apiKeyErrorKind(err), ErrorRef(err))
		return
	}
	// SECURITY: the plaintext is rendered once and discarded — never logged, stored on a row, or put in the session.
	h.writeOrgList(w, sc, &templates.APIKeyMinted{Name: k.Name, Plaintext: plaintext}, "", "")
}

// PostOrgKeyProjects widens an org key by the submitted projects.
func (h *APIKeyHandler) PostOrgKeyProjects(w http.ResponseWriter, r *http.Request) {
	h.changeOrgKey(w, r, func(sc OrgScope, id uuid.UUID) error {
		_, err := h.Keys.GrantProjects(sc.req.Context(), id, formUUIDs(sc.req.PostForm["project_ids"]))
		return err
	})
}

// PostOrgKeyAllProjects promotes an org key to every project of the org.
func (h *APIKeyHandler) PostOrgKeyAllProjects(w http.ResponseWriter, r *http.Request) {
	h.changeOrgKey(w, r, func(sc OrgScope, id uuid.UUID) error {
		_, err := h.Keys.GrantAllProjects(sc.req.Context(), id)
		return err
	})
}

// PostOrgKeyRevoke revokes an org key and returns the refreshed list fragment.
func (h *APIKeyHandler) PostOrgKeyRevoke(w http.ResponseWriter, r *http.Request) {
	h.changeOrgKey(w, r, func(sc OrgScope, id uuid.UUID) error {
		err := h.Keys.Revoke(sc.req.Context(), id)
		if apikey.IsNotFoundError(err) {
			return nil
		}
		return err
	})
}

func (h *APIKeyHandler) changeOrgKey(w http.ResponseWriter, r *http.Request, change func(OrgScope, uuid.UUID) error) {
	sc, ok := h.RequireOrg(w, r)
	if !ok {
		return
	}
	if err := sc.req.ParseForm(); err != nil {
		h.ErrorPage(w, sc.req, http.StatusBadRequest, "Bad request", "Could not parse form body.")
		return
	}
	id, ok := h.keyID(w, sc.req)
	if !ok {
		return
	}
	if err := change(sc, id); err != nil {
		h.LogErr("web apikey: change org key", err)
		h.writeOrgList(w, sc, nil, apiKeyErrorKind(err), ErrorRef(err))
		return
	}
	h.writeOrgList(w, sc, nil, "", "")
}

type mintInput struct {
	name      string
	scopes    []string
	expiresAt *time.Time
	badExpiry bool
}

func (h *APIKeyHandler) parseMint(w http.ResponseWriter, r *http.Request) (mintInput, bool) {
	if err := r.ParseForm(); err != nil {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad request", "Could not parse form body.")
		return mintInput{}, false
	}
	in := mintInput{name: strings.TrimSpace(r.PostForm.Get("name")), scopes: selectedScopes(r.PostForm["scopes"])}
	expiresAt, err := parseExpiry(r.PostForm.Get("expires_at"))
	in.expiresAt, in.badExpiry = expiresAt, err != nil
	return in, true
}

func (h *APIKeyHandler) keyID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.ErrorPage(w, r, http.StatusBadRequest, "Bad id", "Malformed API key id.")
		return uuid.Nil, false
	}
	return id, true
}

// NOTE: CanManage only hides the buttons; apikey.Service refuses a non-manager on every write whatever the page shows.
func (h *APIKeyHandler) canManage(r *http.Request, orgID, userID uuid.UUID) bool {
	ok, err := h.Orgs.IsManager(r.Context(), orgID, userID)
	if err != nil {
		h.LogErr("web apikey: role", err)
	}
	return ok
}

func (h *APIKeyHandler) projectView(sc ProjectScope, minted *templates.APIKeyMinted, errKind, code string) (templates.APIKeysView, error) {
	items, err := h.Keys.List(sc.req.Context(), sc.project.ID)
	if err != nil {
		return templates.APIKeysView{}, err
	}
	rows := make([]templates.APIKeyRow, 0, len(items))
	for _, k := range items {
		rows = append(rows, apiKeyRow(k, nil))
	}
	return templates.APIKeysView{
		Base:      h.ProjectURL(sc, "/apikeys"),
		CanManage: h.canManage(sc.req, sc.org.ID, sc.principal.UserID),
		Items:     rows,
		Scopes:    scopeOptions(),
		Minted:    minted,
		ErrorKind: errKind,
		ErrorCode: code,
	}, nil
}

func (h *APIKeyHandler) orgView(sc OrgScope, minted *templates.APIKeyMinted, errKind, code string) (templates.APIKeysView, error) {
	items, err := h.Keys.ListOrg(sc.req.Context())
	if err != nil {
		return templates.APIKeysView{}, err
	}
	projects, err := h.Projects.List(sc.req.Context(), sc.org.ID)
	if err != nil {
		return templates.APIKeysView{}, err
	}
	options := make([]templates.APIKeyProjectOption, 0, len(projects))
	for _, p := range projects {
		options = append(options, templates.APIKeyProjectOption{ID: p.ID.String(), Name: p.Name})
	}
	rows := make([]templates.APIKeyRow, 0, len(items))
	for _, k := range items {
		rows = append(rows, apiKeyRow(k, options))
	}
	return templates.APIKeysView{
		Base:      web.Path(h.Cfg.HTTP.BasePath, "/orgs/"+sc.org.Slug+"/apikeys"),
		OrgLevel:  true,
		CanManage: h.canManage(sc.req, sc.org.ID, sc.principal.UserID),
		Items:     rows,
		Scopes:    scopeOptions(),
		Projects:  options,
		Minted:    minted,
		ErrorKind: errKind,
		ErrorCode: code,
	}, nil
}

func (h *APIKeyHandler) writeProjectList(w http.ResponseWriter, sc ProjectScope, minted *templates.APIKeyMinted, errKind, code string) {
	v, err := h.projectView(sc, minted, errKind, code)
	if err != nil {
		h.LogErr("web apikey: list", err)
		h.ErrorPage(w, sc.req, http.StatusInternalServerError, "List failed", "Could not load API keys.", err)
		return
	}
	Render(w, sc.req, templates.APIKeyList(h.ProjectFragmentBase(sc), v))
}

func (h *APIKeyHandler) writeOrgList(w http.ResponseWriter, sc OrgScope, minted *templates.APIKeyMinted, errKind, code string) {
	v, err := h.orgView(sc, minted, errKind, code)
	if err != nil {
		h.LogErr("web apikey: list org", err)
		h.ErrorPage(w, sc.req, http.StatusInternalServerError, "List failed", "Could not load API keys.", err)
		return
	}
	Render(w, sc.req, templates.APIKeyList(h.OrgFragmentBase(sc), v))
}

// Register wires the API key routes onto mux.
func (h *APIKeyHandler) Register(mux web.Mux) {
	mux.HandleFunc("GET /orgs/{org}/projects/{project}/apikeys", h.GetKeys)
	mux.HandleFunc("POST /orgs/{org}/projects/{project}/apikeys", h.PostKeyCreate)
	mux.HandleFunc("POST /orgs/{org}/projects/{project}/apikeys/{id}/revoke", h.PostKeyRevoke)
	mux.HandleFunc("GET /orgs/{org}/apikeys", h.GetOrgKeys)
	mux.HandleFunc("POST /orgs/{org}/apikeys", h.PostOrgKeyCreate)
	mux.HandleFunc("POST /orgs/{org}/apikeys/{id}/projects", h.PostOrgKeyProjects)
	mux.HandleFunc("POST /orgs/{org}/apikeys/{id}/all-projects", h.PostOrgKeyAllProjects)
	mux.HandleFunc("POST /orgs/{org}/apikeys/{id}/revoke", h.PostOrgKeyRevoke)
}

func formUUIDs(raw []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		if id, err := uuid.Parse(strings.TrimSpace(s)); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// SECURITY: an unrecognized or duplicated scope value is dropped rather than reaching Mint.
func selectedScopes(raw []string) []string {
	set := make(map[string]bool, len(raw))
	for _, s := range raw {
		set[strings.TrimSpace(s)] = true
	}
	out := make([]string, 0, len(raw))
	for _, s := range authn.MintableScopes() {
		if set[s] {
			out = append(out, s)
		}
	}
	return out
}

func parseExpiry(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}

func apiKeyErrorKind(err error) string {
	switch {
	case org.IsNotManagerError(err):
		return templates.APIKeyErrorNotManager
	case apikey.IsUnknownScopeError(err), apikey.IsRetiredScopeError(err), apikey.IsScopeLevelError(err):
		return templates.APIKeyErrorScope
	case apikey.IsNotFoundError(err):
		return templates.APIKeyErrorNotFound
	case apikey.IsEmptyGrantError(err):
		return templates.APIKeyErrorEmptyGrant
	case apikey.IsProjectNotInOrgError(err), apikey.IsAlreadyAllProjectsError(err), apikey.IsNotOrgKeyError(err),
		apikey.IsGrantConflictError(err), apikey.IsRevokedError(err):
		return templates.APIKeyErrorGrant
	default:
		return templates.APIKeyErrorFailed
	}
}

func apiKeyRow(k *apikey.APIKey, projects []templates.APIKeyProjectOption) templates.APIKeyRow {
	labels := make([]string, 0, len(k.Scopes))
	for _, s := range k.Scopes {
		labels = append(labels, scopeLabelKey(s))
	}
	row := templates.APIKeyRow{
		ID:             k.ID.String(),
		Name:           k.Name,
		SecretHint:     k.SecretHint,
		ScopeLabelKeys: labels,
		OrgLevel:       k.Kind == apikey.KindOrg,
		AllProjects:    k.AllProjects,
		CreatedAt:      k.CreatedAt.Format(time.RFC3339),
		Revoked:        k.RevokedAt != nil,
	}
	granted := make([]string, 0, len(k.ProjectIDs))
	for _, id := range k.ProjectIDs {
		granted = append(granted, id.String())
	}
	for _, p := range projects {
		if slices.Contains(granted, p.ID) {
			row.ProjectNames = append(row.ProjectNames, p.Name)
			continue
		}
		row.Grantable = append(row.Grantable, p)
	}
	if k.ExpiresAt != nil {
		row.HasExpiry = true
		row.ExpiresAt = k.ExpiresAt.Format(time.RFC3339)
	}
	if k.LastUsedAt != nil {
		row.HasLastUsed = true
		row.LastUsedAt = k.LastUsedAt.Format(time.RFC3339)
	}
	if k.RevokedAt != nil {
		row.RevokedAt = k.RevokedAt.Format(time.RFC3339)
	}
	return row
}

func scopeOptions() []templates.APIKeyScopeOption {
	all := authn.MintableScopes()
	out := make([]templates.APIKeyScopeOption, 0, len(all))
	for _, s := range all {
		out = append(out, templates.APIKeyScopeOption{Value: s, LabelKey: scopeLabelKey(s)})
	}
	return out
}

//i18n:use apikey.scope.*
func scopeLabelKey(scope string) string {
	return "apikey.scope." + strings.ReplaceAll(scope, ":", "_")
}
