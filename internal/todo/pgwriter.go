package todo

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"
)

func (s *postgresStore) Save(ctx context.Context, t *Todo) error {
	tx, owned, tc, err := s.txAcquire(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	stmt := s.table.INSERT(s.table.AllColumns).
		VALUES(
			t.ID, t.OrgID, t.ProjectID, tc.UserID, t.Title, t.Done,
			t.CreatedAt.UTC(), now,
		).
		ON_CONFLICT(s.table.ID).
		DO_UPDATE(
			postgres.SET(
				s.table.Title.SET(postgres.String(t.Title)),
				s.table.Done.SET(postgres.Bool(t.Done)),
				s.table.UpdatedAt.SET(postgres.TimestampzT(now)),
			),
		)
	if _, execErr := stmt.ExecContext(ctx, tx); execErr != nil {
		return s.endTx(tx, owned, fmt.Errorf("todo.postgres.Save: %w", execErr))
	}
	return s.endTx(tx, owned, nil)
}

func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	tx, owned, _, err := s.txAcquire(ctx)
	if err != nil {
		return err
	}
	stmt := s.table.DELETE().WHERE(s.table.ID.EQ(postgres.UUID(id)))
	res, execErr := stmt.ExecContext(ctx, tx)
	if execErr != nil {
		return s.endTx(tx, owned, fmt.Errorf("todo.postgres.Delete: %w", execErr))
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return s.endTx(tx, owned, fmt.Errorf("todo.postgres.Delete: rows affected: %w", raErr))
	}
	if n == 0 {
		return s.endTx(tx, owned, &NotFoundError{ID: id.String()})
	}
	return s.endTx(tx, owned, nil)
}

func (s *postgresStore) ClearDone(ctx context.Context, orgID, projectID uuid.UUID) (int, error) {
	tx, owned, _, err := s.txAcquire(ctx)
	if err != nil {
		return 0, err
	}
	stmt := s.table.DELETE().
		WHERE(s.table.OrgID.EQ(postgres.UUID(orgID)).
			AND(s.table.ProjectID.EQ(postgres.UUID(projectID))).
			AND(s.table.Done.EQ(postgres.Bool(true))))
	res, execErr := stmt.ExecContext(ctx, tx)
	if execErr != nil {
		return 0, s.endTx(tx, owned, fmt.Errorf("todo.postgres.ClearDone: %w", execErr))
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return 0, s.endTx(tx, owned, fmt.Errorf("todo.postgres.ClearDone: rows affected: %w", raErr))
	}
	if endErr := s.endTx(tx, owned, nil); endErr != nil {
		return 0, endErr
	}
	return int(n), nil
}
