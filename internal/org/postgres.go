package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	pdb "altalune.id/template/internal/platform/db"
	pgent "altalune.id/template/internal/platform/db/entity/postgres"
	"altalune.id/template/internal/platform/tenant"
)

type postgresStore struct {
	pool    pdb.Pool
	pc      *tenant.PgConn
	orgs    *pgent.Orgs
	members *pgent.Memberships
	users   *pgent.Users
}

func newPostgresStore(pool pdb.Pool, pc *tenant.PgConn, schema, tablePrefix string) *postgresStore {
	return &postgresStore{
		pool:    pool,
		pc:      pc,
		orgs:    pgent.NewOrgs(schema, tablePrefix),
		members: pgent.NewMemberships(schema, tablePrefix),
		users:   pgent.NewUsers(schema, tablePrefix),
	}
}

type pgOrgRow struct {
	ID        uuid.UUID `alias:"orgs.id"`
	Slug      string    `alias:"orgs.slug"`
	Name      string    `alias:"orgs.name"`
	CreatedBy uuid.UUID `alias:"orgs.created_by"`
	CreatedAt time.Time `alias:"orgs.created_at"`
	System    bool      `alias:"orgs.system"`
}

func (r *pgOrgRow) toOrg() *Org {
	return &Org{
		ID:        r.ID,
		Slug:      r.Slug,
		Name:      r.Name,
		OwnerID:   r.CreatedBy,
		CreatedAt: r.CreatedAt,
		System:    r.System,
	}
}

type pgMembershipRow struct {
	OrgID     uuid.UUID `alias:"memberships.org_id"`
	UserID    uuid.UUID `alias:"memberships.user_id"`
	Role      string    `alias:"memberships.role"`
	CreatedAt time.Time `alias:"memberships.created_at"`
	System    bool      `alias:"memberships.system"`
}

func (r *pgMembershipRow) toMembership() *Membership {
	return &Membership{
		OrgID:     r.OrgID,
		UserID:    r.UserID,
		Role:      Role(r.Role),
		CreatedAt: r.CreatedAt,
		System:    r.System,
	}
}

type pgMemberProfileRow struct {
	UserID    uuid.UUID `alias:"memberships.user_id"`
	Role      string    `alias:"memberships.role"`
	CreatedAt time.Time `alias:"memberships.created_at"`
	System    bool      `alias:"memberships.system"`
	Email     string    `alias:"users.email"`
	Name      string    `alias:"users.name"`
}

func (r *pgMemberProfileRow) toProfile() *MemberProfile {
	return &MemberProfile{
		UserID:    r.UserID,
		Email:     r.Email,
		Name:      r.Name,
		Role:      Role(r.Role),
		CreatedAt: r.CreatedAt,
		System:    r.System,
	}
}

func (s *postgresStore) txAcquire(ctx context.Context) (*sql.Tx, bool, error) {
	if tx, ok := pdb.CurrentTx(ctx); ok {
		return tx, false, nil
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, false, err
	}
	tx, err := s.pc.BeginTenanted(ctx, tc)
	if err != nil {
		return nil, false, fmt.Errorf("org.postgres: begin: %w", err)
	}
	return tx, true, nil
}

func (s *postgresStore) endTx(tx *sql.Tx, owned bool, err error) error {
	if !owned {
		return err
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if cerr := tx.Commit(); cerr != nil {
		return fmt.Errorf("org.postgres: commit: %w", cerr)
	}
	return nil
}

func isPostgresUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}
