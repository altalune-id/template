# Configuration

## Precedence

`defaults <- config.yaml <- ALT_* env`. Env always wins. Nested keys map
by dot notation with dots → underscores:

| YAML                               | Env var                               |
| ---------------------------------- | ------------------------------------- |
| `mode: cloud`                      | `ALT_MODE=cloud`                      |
| `db.driver: postgres`              | `ALT_DB_DRIVER=postgres`              |
| `http.basePath: /altempl`          | `ALT_HTTP_BASE_PATH=/altempl`         |
| `tokens.audience: urn:altempl:api` | `ALT_TOKENS_AUDIENCE=urn:altempl:api` |
| `tenant.singletonOrg.slug: main`   | `ALT_TENANT_SINGLETON_ORG_SLUG=main`  |

`config.example.yaml` and `.env.example` are generated from struct tags
in `internal/platform/config`. After editing those, run `make config-examples`.

## Awareness markers

Every `.env.example` field carries a marker in `[brackets]`:

| Marker                           | Meaning                                                             |
| -------------------------------- | ------------------------------------------------------------------- |
| `required`                       | boot fails if unset (in the applicable mode)                        |
| `bootstrap`                      | locks in at first boot; changing later is a no-op on persisted data |
| `secret`                         | never commit; `secret` fields never emit defaults                   |
| `mode:cloud` / `mode:selfhosted` | only meaningful in the named mode                                   |

## Modes

| Property             | `selfhosted`           | `cloud`                                              |
| -------------------- | ---------------------- | ---------------------------------------------------- |
| DB driver            | `sqlite` or `postgres` | `postgres` only                                      |
| OIDC identity        | optional               | required (`issuer` + `clientID` + `clientSecret`)    |
| Local password login | on by default          | off; set `ALT_GENESIS_BREAK_GLASS=true` to re-enable |
| Org creation from UI | disabled               | enabled                                              |
| Public signup        | disabled               | enabled                                              |

Onboarding, first-org seeding, and admin bootstrap are uniform across modes.

## First-boot

```
onboarded row exists?    → skip; boot into dashboard
GENESIS_EMAIL empty?     → all web routes → /onboard
GENESIS_EMAIL set?       → create admin + first org + first project; mark onboarded
```

Bootstrap seeds use `ALT_TENANT_SINGLETON_ORG_SLUG` (default `default`),
`ALT_TENANT_SINGLETON_ORG_NAME` (default `Default Organization`), and
`ALT_TENANT_PERSONAL_PROJECT_SLUG` (default `default`) — both modes.

**The `/onboard` form** — shown when no genesis env vars are set. Local
path (email + password + org + project → dashboard) is available when
`caps.LocalIdentity` is on; OIDC path (provider roundtrip → app creates
first org + project) when `caps.ExternalIdentity` is on. Cloud shows only
OIDC by default; selfhosted shows both.

**Cloud + genesis + break-glass** — setting `ALT_GENESIS_EMAIL` +
`ALT_GENESIS_PASSWORD` in cloud requires `ALT_GENESIS_BREAK_GLASS=true`.
Without it, boot fails loud: the local login form is hidden in cloud, so
the genesis user would be unreachable via the UI.

## Scheduler

| Key                              | Default | Awareness   | Meaning                                                                                                                |
| -------------------------------- | ------- | ----------- | ---------------------------------------------------------------------------------------------------------------------- |
| `scheduler.enabled`              | `true`  | `bootstrap` | Master switch for the periodic-job runner. `false` boots the app with no jobs; `altempl scheduler run` then exits `7`. |
| `scheduler.timezone`             | `UTC`   | `bootstrap` | IANA zone for every wall-clock schedule. Rejected at boot if `time.LoadLocation` cannot resolve it.                    |
| `scheduler.shutdownGrace`        | `30s`   | `-`         | How long the runner waits for in-flight jobs on shutdown before giving up.                                             |
| `scheduler.jobs.<name>.timezone` | —       | `-`         | Per-job override of `scheduler.timezone`, keyed by the job name from `altempl scheduler list`.                         |

Timezone resolves in three steps: `scheduler.jobs.<name>.timezone`, then
`scheduler.timezone`, then UTC.

Overrides only affect **wall-clock** schedules (cron, daily-at). Boot logs a
warning when an override names an unknown job, or a job on an interval
schedule — an interval has no wall-clock anchor to shift.

**Job cadences are deliberately not configurable.** A cadence is a package
constant in the owning module's `scheduler.go` (e.g. `sweepCron` in
`internal/todo/scheduler.go`), because changing one changes the domain's
behaviour and belongs in review, not in a deploy-time env var. Only the
timezone is an operator knob.

## Database

| Key                              | Default | Awareness          | Meaning                                                                                                                                      |
| -------------------------------- | ------- | ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `db.maintenance.dsn`             | —       | `secret,bootstrap` | Credential for cross-tenant maintenance reads (tenant enumeration). Expected `BYPASSRLS` and read-only. Empty falls back to the writer pool. |
| `db.maintenance.role`            | —       | `bootstrap`        | `SET ROLE` applied on each maintenance connection.                                                                                           |
| `db.maintenance.maxOpenConns`    | `2`     | `-`                | Cap on the maintenance pool. Small on purpose — it only enumerates tenants.                                                                  |
| `db.maintenance.maxIdleConns`    | `0`     | `-`                | Idle cap on the maintenance pool.                                                                                                            |
| `db.maintenance.connMaxLifetime` | `0`     | `-`                | Max lifetime of a maintenance connection.                                                                                                    |
| `db.maintenance.connMaxIdleTime` | `0`     | `-`                | Max idle time of a maintenance connection.                                                                                                   |
| `db.connectTimeout`              | `30s`   | `-`                | Total budget for the initial connect-and-ping at boot, retries included. `0` disables retrying.                                              |
| `db.connectBackoff`              | `250ms` | `-`                | Starting backoff between connect attempts; doubles per attempt, capped at 5s, and never overruns `connectTimeout`.                           |
| `db.health.interval`             | `30s`   | `-`                | Tick cadence of the `db-health` worker.                                                                                                      |
| `db.health.timeout`              | `2s`    | `-`                | Per-handle ping timeout within one probe round.                                                                                              |

The separate reader and maintenance handles are Postgres-only; under
`driver: sqlite` their DSNs are ignored and every handle aliases the writer.

`/readyz` reports the snapshot the `db-health` worker writes, so
`db.health.interval` sets how stale a readiness answer can be. Boot takes one
synchronous probe, so `/readyz` is accurate before the first tick. The worker
is independent of the scheduler and runs in every replica, so `/readyz` stays
DB-aware under `scheduler.enabled=false` and `serve --no-scheduler` alike. See
[`DEPLOYMENT.md`](DEPLOYMENT.md) for the maintenance role's grants and the
RLS interaction.

## Validation

`config.Validate()` runs on every boot with specific messages
(e.g. `cloud requires db.driver=postgres, got "sqlite"`).
