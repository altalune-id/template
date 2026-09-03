package invite

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	pdb "altalune.id/template/internal/platform/db"
)

func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Invite, error) {
	tx, owned, _, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	return s.queryOne(ctx, tx, s.table.ID.EQ(postgres.UUID(id)), &NotFoundError{ID: id.String()})
}

// SECURITY: bypasses tenant scope; the token hash is unguessable so cross-tenant lookup is safe.
func (s *postgresStore) ByTokenHash(ctx context.Context, hash string) (*Invite, error) {
	if tx, ok := pdb.CurrentTx(ctx); ok {
		return s.queryOne(ctx, tx, s.table.TokenHash.EQ(postgres.String(hash)), &NotFoundError{})
	}
	return s.queryOne(ctx, s.pc.DB, s.table.TokenHash.EQ(postgres.String(hash)), &NotFoundError{})
}

func (s *postgresStore) ListPending(ctx context.Context, orgID uuid.UUID) ([]*Invite, error) {
	tx, owned, _, err := s.txAcquire(ctx)
	if err != nil {
		return nil, err
	}
	if owned {
		defer func() { _ = tx.Rollback() }()
	}
	stmt := postgres.SELECT(s.table.AllColumns).
		FROM(s.table).
		WHERE(s.table.OrgID.EQ(postgres.UUID(orgID)).
			AND(s.table.AcceptedAt.IS_NULL())).
		ORDER_BY(s.table.CreatedAt.ASC())
	var rows []pgInviteRow
	if qErr := stmt.QueryContext(ctx, tx, &rows); qErr != nil {
		return nil, fmt.Errorf("invite.postgres.ListPending: %w", qErr)
	}
	out := make([]*Invite, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toInvite())
	}
	return out, nil
}

// SECURITY: cross-tenant lookup by email is required to route invited OIDC signups; the email is scoped to the caller's own identity.
func (s *postgresStore) FindPendingForEmail(ctx context.Context, email string) ([]*Invite, error) {
	var execer qrm.DB = s.pc.DB
	if tx, ok := pdb.CurrentTx(ctx); ok {
		execer = tx
	}
	stmt := postgres.SELECT(s.table.AllColumns).
		FROM(s.table).
		WHERE(s.table.Email.EQ(postgres.String(email)).
			AND(s.table.AcceptedAt.IS_NULL())).
		ORDER_BY(s.table.CreatedAt.ASC())
	var rows []pgInviteRow
	if err := stmt.QueryContext(ctx, execer, &rows); err != nil {
		return nil, fmt.Errorf("invite.postgres.FindPendingForEmail: %w", err)
	}
	out := make([]*Invite, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toInvite())
	}
	return out, nil
}
