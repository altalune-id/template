package tenant

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"altalune.id/template/internal/platform/db"
	"altalune.id/template/scheduler"
)

var _ scheduler.Tenants = (*PgTenants)(nil)

// PgTenants enumerates orgs on the maintenance handle and binds each to ctx.
type PgTenants struct {
	pool  db.Pool
	query string
	log   *slog.Logger
}

// NewPgTenants builds the tenant enumerator; reads run on pool.M.
func NewPgTenants(pool db.Pool, driver db.Driver, schema, tablePrefix string, log *slog.Logger) *PgTenants {
	table := tablePrefix + "orgs"
	if driver == db.DriverPostgres {
		if schema == "" {
			schema = "public"
		}
		table = schema + "." + table
	}
	return &PgTenants{
		pool:  pool,
		query: "SELECT id FROM " + table + " ORDER BY created_at",
		log:   log,
	}
}

// Each invokes fn once per tenant with a tenant-bound ctx, aborting on the first non-nil fn error.
func (t *PgTenants) Each(ctx context.Context, fn func(ctx context.Context, tenantID string) error) error {
	ids, err := t.list(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		tctx := Into(ctx, Context{OrgID: id})
		if err := fn(tctx, id.String()); err != nil {
			return err
		}
	}
	return nil
}

// NOTE: drains the cursor before any callback runs so a long sweep holds no open connection.
func (t *PgTenants) list(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := t.pool.M.QueryContext(ctx, t.query) //nolint:rowserrcheck // checked via rows.Err below
	if err != nil {
		return nil, fmt.Errorf("tenant: enumerate orgs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []uuid.UUID
	for rows.Next() {
		var raw string
		if scanErr := rows.Scan(&raw); scanErr != nil {
			return nil, fmt.Errorf("tenant: scan org id: %w", scanErr)
		}
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("tenant: parse org id %q: %w", raw, parseErr)
		}
		out = append(out, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("tenant: enumerate orgs: %w", rowsErr)
	}
	if len(out) == 0 && t.log != nil {
		t.log.WarnContext(ctx, "tenant: enumeration returned no orgs — check db.maintenance.dsn if RLS is enforced")
	}
	return out, nil
}
