package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	pdb "altalune.id/template/internal/platform/db"
)

func (s *postgresStore) BySlug(ctx context.Context, slug string) (*Org, error) {
	tx, owned, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	return s.queryOrg(ctx, tx, s.orgs.Slug.EQ(postgres.String(slug)), &NotFoundError{Slug: slug})
}

func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Org, error) {
	tx, owned, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	return s.queryOrg(ctx, tx, s.orgs.ID.EQ(postgres.UUID(id)), &NotFoundError{ID: id.String()})
}

// SECURITY: bypasses tenant scope; caller has no active tenant yet and is choosing one.
func (s *postgresStore) List(ctx context.Context, userID uuid.UUID) ([]*Org, error) {
	if tx, ok := pdb.CurrentTx(ctx); ok {
		return s.listOn(ctx, tx, userID)
	}
	return s.listOn(ctx, s.pc.DB, userID)
}

func (s *postgresStore) listOn(ctx context.Context, execer qrm.DB, userID uuid.UUID) ([]*Org, error) {
	stmt := postgres.SELECT(s.orgs.ID, s.orgs.Slug, s.orgs.Name, s.orgs.CreatedBy, s.orgs.CreatedAt, s.orgs.System).
		FROM(s.orgs.INNER_JOIN(s.members, s.members.OrgID.EQ(s.orgs.ID))).
		WHERE(s.members.UserID.EQ(postgres.UUID(userID))).
		ORDER_BY(s.orgs.CreatedAt.ASC())
	var rows []pgOrgRow
	if err := stmt.QueryContext(ctx, execer, &rows); err != nil {
		return nil, fmt.Errorf("org.postgres: List: %w", err)
	}
	out := make([]*Org, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toOrg())
	}
	return out, nil
}

func (s *postgresStore) MembershipOf(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error) {
	tx, owned, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	stmt := postgres.SELECT(s.members.OrgID, s.members.UserID, s.members.Role, s.members.CreatedAt, s.members.System).
		FROM(s.members).
		WHERE(s.members.OrgID.EQ(postgres.UUID(orgID)).
			AND(s.members.UserID.EQ(postgres.UUID(userID)))).
		LIMIT(1)
	var row pgMembershipRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, &MembershipMissingError{OrgID: orgID.String(), UserID: userID.String()}
		}
		return nil, fmt.Errorf("org.postgres: MembershipOf: %w", err)
	}
	return row.toMembership(), nil
}

func (s *postgresStore) ListMembers(ctx context.Context, orgID uuid.UUID) ([]*Membership, error) {
	tx, owned, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	stmt := postgres.SELECT(s.members.OrgID, s.members.UserID, s.members.Role, s.members.CreatedAt, s.members.System).
		FROM(s.members).
		WHERE(s.members.OrgID.EQ(postgres.UUID(orgID))).
		ORDER_BY(s.members.CreatedAt.ASC())
	var rows []pgMembershipRow
	if err := stmt.QueryContext(ctx, tx, &rows); err != nil {
		return nil, fmt.Errorf("org.postgres: ListMembers: %w", err)
	}
	out := make([]*Membership, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toMembership())
	}
	return out, nil
}

func (s *postgresStore) ListMemberProfiles(ctx context.Context, orgID uuid.UUID) ([]*MemberProfile, error) {
	tx, owned, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	stmt := postgres.SELECT(
		s.members.UserID,
		s.members.Role,
		s.members.CreatedAt,
		s.members.System,
		s.users.Email,
		s.users.Name,
	).
		FROM(s.members.INNER_JOIN(s.users, s.users.ID.EQ(s.members.UserID))).
		WHERE(s.members.OrgID.EQ(postgres.UUID(orgID))).
		ORDER_BY(s.members.CreatedAt.ASC())
	var rows []pgMemberProfileRow
	if err := stmt.QueryContext(ctx, tx, &rows); err != nil {
		return nil, fmt.Errorf("org.postgres: ListMemberProfiles: %w", err)
	}
	out := make([]*MemberProfile, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toProfile())
	}
	return out, nil
}

func (s *postgresStore) queryOrg(ctx context.Context, execer qrm.DB, cond postgres.BoolExpression, notFound *NotFoundError) (*Org, error) {
	stmt := postgres.SELECT(s.orgs.ID, s.orgs.Slug, s.orgs.Name, s.orgs.CreatedBy, s.orgs.CreatedAt, s.orgs.System).
		FROM(s.orgs).
		WHERE(cond).
		LIMIT(1)
	var row pgOrgRow
	if err := stmt.QueryContext(ctx, execer, &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, notFound
		}
		return nil, fmt.Errorf("org.postgres: query: %w", err)
	}
	return row.toOrg(), nil
}
