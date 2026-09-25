# Reviewing a module

The rules behind each line are the contracts in [`docs/`](../../../../docs/) — the module shape is
[`modules`](../../../../docs/modules/README.md), whose
[Section 11](../../../../docs/modules/README.md#11-review-checklist) is the doc-side checklist.
This one is the pre-merge pass, and the section below it is what that checklist cannot express.

## Checklist

- File set matches the canonical shape. No `postgres_repo.go`, no `domain/` subpackage, no
  splitting by layer.
- `Store` lives in `store.go` beside the aggregate. Verbs only, `context.Context` first.
- Every typed error has `ToAppError()` and an `Is<FullTypeName>` helper, and every code has a
  `../../../../docs/errors/README.md` row.
- Every service method opens a span; every DB method carries an explicit `org_id` predicate.
- Every upsert conflict clause carries the tenant predicate and a `RowsAffected() == 0` branch.
- Every service method taking a bare id checks org **and** project.
- Every `List` orders by a total order; every SQLite timestamp goes through `SQLiteTime`, bind
  sites included; every SQL NULL comes from a `Null*` helper in `entity/{postgres,sqlite}`,
  never a wrapped `postgres.NULL` / `sqlite.NULL`.
- A conditional write is **one** `Save` — no guarded body write followed by an unguarded
  satellite write. The version guard rejects anything outside `[1, math.MaxInt32]`.
- Migration has `ENABLE` + `FORCE` + a policy, inside `{{if .RLSEnforce}}`, with the
  `{{.TablePrefix}}` literal; `make tenant-tables` regenerated.
- Every route belongs to exactly one surface, and is reachable only from that surface's chain.
  New machine-surface procedures appear in their scope table and their verb registry.
- Every `<script>` carries `nonce={ d.Nonce }`, every htmx attribute `hx-nonce={ d.Nonce }`.
- Cross-module references by UUID only.
- No `panic` in the request path. No `log.Println`. No `fmt.Print*` in domain code.
- Comments obey the discipline: godoc one-liners plus `SECURITY:`/`NOTE:`/`TODO:` markers, no
  rationale prose.

## Gates

| Gate                                 | Catches                                             |
| ------------------------------------ | --------------------------------------------------- |
| `make check`                         | fmt, vet, templ drift, **race** — not optional      |
| `make lint`                          | depguard purity rules, forbidigo                    |
| `make comment-check`                 | comment discipline (CI job `comment-check`)         |
| `make i18n-check`                    | a `d.Tr` key missing from a locale, or read as dead |
| `make config-examples`               | `.env.example` / `config.example.yaml` drift        |
| `make tenant-tables`                 | a new tenant table missing from the generated list  |
| `make test-integration`              | anything touching `postgres.go` or a migration      |
| `bash scripts/verify-serve-smoke.sh` | boot, `/healthz`, clean SIGTERM shutdown            |
| `bash scripts/verify-mcp-smoke.sh`   | the MCP surface, an `(mcp.v1.tool)` annotation      |

`make check` runs the race detector because the stores build jet expressions concurrently; a
non-race run cannot see the class of bug that was found downstream.

## Things that pass review while being wrong

These are the failures that survived a plan, a spec review, and a green test suite in this
codebase. Look for them specifically.

**A test that cannot fail.** The most common shape: a tenant-isolation test on an RLS-enforcing
Postgres fixture. RLS refuses the write either way, so it passes with or without the guard it
claims to cover. Ask of every security test: _in what posture is this guard the only thing
standing?_ Run it in that posture — SQLite, or a superuser connection — then revert the guard and
watch it fail.

**A fixture that cannot prove what it claims.** The default integration fixture connects as the
container superuser, which bypasses RLS outright, so `..._OtherOrgIsInvisible` on it proves
nothing. A real one migrates as a BYPASSRLS owner with `RLSEnforce=true`, binds the store to a
separate `NOBYPASSRLS` login role, and asserts the policies exist.

**A fake that enforces the thing under test.** An in-memory store that filters by project makes a
project-scope test pass regardless of the production code. Assert the fake does not filter.

**A guard with no coverage.** A `WHERE` clause or a depguard glob that matches nothing looks
exactly like protection. Break it on purpose and confirm the failure.

**A conditional write split in two.** Both halves guard correctly, both tests pass, and two
writers still interleave between them. Ask what a second writer does _between_ the statements.

**A `Delete` that never loads the row.** It has no scope check at all — the store's org filter is
the only thing between a sibling project and a destructive write.

**An edited migration on a live schema.** Goose does not checksum, so `migrate up` silently does
nothing and the change appears to have been applied.

**A success message for work that did not happen.** Check that a reported side effect has a
corresponding write. A command printing `project=default` while creating no project is a real
example from this codebase.

**A comment describing a mechanism that cannot occur.** Worse than no comment, because it will be
trusted. Verify the claim before copying a rationale from a neighbouring file.

**A test that is green on macOS and red on Linux CI.** Postgres `timestamptz` is microsecond
precision; `time.Now()` on macOS is already microsecond-granular, so a nanosecond value survives
a round trip locally but not on Linux. `got.CreatedAt.Equal(want.CreatedAt)` therefore passes on
every developer machine and fails in CI. Compare against
`want.CreatedAt.Truncate(time.Microsecond)`, as `internal/org/definer_integration_test.go` does.
SQLite keeps the full nanosecond value through `SQLiteTime`, so the same aggregate round-trips at
different precision per driver.

## Verifying a claim

Prefer evidence over reasoning when both are available. `EXPLAIN` the query, run the mutation,
boot the binary, open the page. Several conclusions here were confidently wrong until someone ran
the thing — "the lint rule is inert" (it was not; the test used an allowed stdlib import) and
"the ORDER BY is load-bearing" (it was not; `SECURITY DEFINER` functions are never inlined).
