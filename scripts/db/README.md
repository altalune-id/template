# DB provisioning

Provider-agnostic scripts to provision a per-app role graph in Postgres. Works
on Neon, RDS, CloudSQL, Supabase, and self-hosted.

## Files

- `provision.sh` — entry point. Creates the DB, renders templates for a given
  app prefix, runs bootstrap + ops.
- `bootstrap.template.sql` — seven-role graph, ownership, default privileges.
- `ops.template.sql` — SECURITY DEFINER procedures for day-2 role/user ops.
- `verify.template.sql` — post-migration invariant check.

Templates use `@@APP@@` as the role/schema prefix; `provision.sh` renders
them via `sed` at run time.

## Role graph (per app)

```
provider admin  (neondb_owner / postgres / rds_superuser)
    │  only used during provisioning
    ▼
<app>_owner        NOLOGIN CREATEROLE — owns schema, tables, sequences, ops
    │
    ├── <app>_migrator  LOGIN — runs migrations under SET ROLE <app>_owner
    ├── <app>_service   LOGIN — DML on tables (default privileges)
    ├── <app>_editor    NOLOGIN group — SELECT + UPDATE for humans
    ├── <app>_reader    NOLOGIN group — SELECT via pg_read_all_data
    └── <app>_ops       NOLOGIN group — EXECUTE on ops procedures

<app>_maintenance  LOGIN BYPASSRLS — read-only cross-tenant reads
```

Only `<app>_migrator` does DDL. `<app>_service` does DML. Humans in `_editor`
edit rows, `_reader` reads, `_ops` manages other humans.

`<app>_maintenance` stands outside the `_owner` tree — it owns nothing and
only reads. Tenant-scoped scheduler jobs enumerate every org, which the
`FORCE ROW LEVEL SECURITY` policy on `orgs` hides from `<app>_service`;
`BYPASSRLS` is the only attribute that lifts it (owning the table does not,
and `pg_read_all_data` alone does not either). Export it as
`ALT_DB_MAINTENANCE_DSN`. Creating it requires an admin that itself has
`BYPASSRLS` or `SUPERUSER`.

## Quick start

```bash
scripts/db/provision.sh
```

Interactive by default; prompts for `APP`, `ADMIN_URL`, `DB_NAME`, and
passwords (type `gen` to auto-generate). Set the env vars up front to run
non-interactively.

## Provider notes

- **Neon**: `ADMIN_URL` uses the _direct_ endpoint (not `-pooler` — pooling
  drops the DDL and `SET ROLE`). Runtime services should use the pooled
  endpoint.
- **RDS / CloudSQL / self-hosted**: single endpoint; connect as your admin.

## Multi-app usage

Each app gets its own role graph. Roles exist cluster-wide; grants are
per-database:

```bash
APP=auth    DB_NAME=authdb    scripts/db/provision.sh
APP=billing DB_NAME=billingdb scripts/db/provision.sh
```

A human onboarded to `auth` can be granted access to `billing` without
creating a new login — see below.

## Day-2 ops

Call from any session whose role is a member of `<app>_ops`.

```sql
-- Onboard a new human (creates the LOGIN role + first app membership)
CALL auth_ops.onboard('hirzi2026', '<pw ≥16>', 'auth_editor', DATE '2026-12-31');

-- Add the same human to another app (no new login)
CALL billing_ops.grant_membership('hirzi2026', 'billing_reader');

-- Remove from one app
CALL billing_ops.revoke_membership('hirzi2026', 'billing_reader');

-- Rotate password (cluster-wide, one call is enough)
CALL auth_ops.rotate_password('hirzi2026', '<new_pw>', DATE '2027-12-31');

-- Lock out entirely (NOLOGIN cluster-wide + this app's memberships gone)
CALL auth_ops.offboard('hirzi2026');

-- Roster (per app)
SELECT * FROM auth_ops.list_operators();
```

Grant a human day-2 ops rights for an app:

```sql
GRANT auth_ops TO hirzi2026;
```

## Running bootstrap / ops standalone

The templates are transaction fragments (no built-in `BEGIN`/`COMMIT`) so
provision.sh can wrap both in one atomic transaction. To run one alone
atomically, use psql's `--single-transaction`:

```bash
APP=auth
sed "s/@@APP@@/$APP/g" scripts/db/bootstrap.template.sql | \
  psql --single-transaction \
       -v migrator_password="$MIG_PW" \
       -v service_password="$SVC_PW" \
       -v maintenance_password="$MNT_PW" \
       "$ADMIN_URL/authdb"

sed "s/@@APP@@/$APP/g" scripts/db/ops.template.sql | \
  psql --single-transaction "$ADMIN_URL/authdb"
```

## Verify

After running migrations, check that ownership and default privileges are
intact:

```bash
APP=auth sed "s/@@APP@@/$APP/g" scripts/db/verify.template.sql \
  | psql "$ADMIN_URL/authdb"
```

Fails loudly if any `public.*` object isn't owned by `<app>_owner`, if
default privileges are missing, or if `<app>_maintenance` is absent or has
lost `BYPASSRLS`.
