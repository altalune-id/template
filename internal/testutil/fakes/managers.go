package fakes

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"altalune.id/template/internal/org"
)

// Managers is an in-memory owner/admin gate for tests; only seated users manage.
type Managers struct {
	mu       sync.Mutex
	seated   map[[2]uuid.UUID]bool
	everyone bool
}

// NewManagers returns a gate with nobody seated.
func NewManagers() *Managers { return &Managers{seated: map[[2]uuid.UUID]bool{}} }

// PermissiveManagers returns a gate that admits every person, for tests that are not about the gate; a machine principal is still refused.
func PermissiveManagers() *Managers {
	m := NewManagers()
	m.everyone = true
	return m
}

// Seat makes userID an owner or admin of orgID.
func (m *Managers) Seat(orgID, userID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seated[[2]uuid.UUID{orgID, userID}] = true
}

// RequireManager refuses with *org.NotManagerError unless userID was seated in orgID.
func (m *Managers) RequireManager(_ context.Context, orgID, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if userID == uuid.Nil || (!m.everyone && !m.seated[[2]uuid.UUID{orgID, userID}]) {
		return &org.NotManagerError{OrgID: orgID.String(), UserID: userID.String()}
	}
	return nil
}

// OrgProjects is an in-memory apikey.Projects for tests.
type OrgProjects struct {
	mu    sync.Mutex
	byOrg map[uuid.UUID][]uuid.UUID
}

// NewOrgProjects returns a catalog with no projects.
func NewOrgProjects() *OrgProjects { return &OrgProjects{byOrg: map[uuid.UUID][]uuid.UUID{}} }

// Add records projectID as a project of orgID.
func (p *OrgProjects) Add(orgID, projectID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byOrg[orgID] = append(p.byOrg[orgID], projectID)
}

// ProjectIDs returns the projects recorded for orgID.
func (p *OrgProjects) ProjectIDs(_ context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]uuid.UUID(nil), p.byOrg[orgID]...), nil
}
