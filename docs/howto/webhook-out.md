# Send an outbound event

Surface **S5 dispatch** ([`surfaces`](../surfaces/README.md)) — the only egress surface, where we call a URL the tenant controls. Receiving a third party's push is the opposite direction, [`webhook-in.md`](webhook-in.md) (S4). Operator alerts to Slack or Discord are **not** this surface: `internal/platform/notify` is a separate, fire-and-forget code path, and wiring a tenant endpoint into it leaks operational data cross-tenant.

This surface is a **primitive**: `internal/platform/outbox/` ships the table, the store, the worker and the backoff, and no `Deliverer`. Until a fork registers one, nothing is delivered.

## Steps

1. Enqueue from the domain service, never inline from a request handler:

   ```go
   err := kernel.Outbox.Enqueue(ctx, outbox.Entry{ID: id, EventID: ev, OrgID: org, ProjectID: proj, Target: t, Payload: body})
   ```

   A zero uuid on any of those four ids, an empty or over-long `Target`, or a `Status` other than empty or `StatusPending` is an `InvalidEntryError`.

2. Implement `outbox.Deliverer` — `Deliver(ctx context.Context, e Entry) error`.
3. Build its client with `httpclient.New(...)`, never a bare `http.Client`. The package installs `rejectPrivate` as the dialer's `ControlContext`, refusing loopback, link-local, private and CGNAT addresses at connect time, and refuses redirects unless `AllowRedirects` is set.
4. Sign per R10: `HMAC-SHA256` over `timestamp + "." + raw_body`, sent as `X-Altempl-Timestamp` and `X-Altempl-Signature: v1=<hex>`, primary first when two secrets are active. Do not invent a header.
5. Hand it to boot: `boot.BootServer(ctx, cfg, boot.WithDispatch(d, outbox.WorkerOpts{}))`. `outbox.NewWorker` is registered on the supervisor only when the deliverer is non-nil; otherwise `Run` logs `dispatch idle` and returns.
6. Run `make test-integration` if you touched `internal/platform/outbox/postgres.go` or the outbox migration.

## Tenancy

- **Fan-out, not inheritance (the default).** Dispatch has no request to take a scope from. `Worker.sweep` walks `tenant.Enumerator` — the same `boot.orgEnumerator` the scheduler uses — and drains one org's batch per bound context. Each `Entry` also carries its own `OrgID`.
- The enumeration is itself a cross-tenant read, so it cannot go through RLS: `tenant.NewOrgReader` calls the `<prefix>list_org_ids()` `SECURITY DEFINER` wrapper.
- **The scope carries no `UserID`.** A dispatch acts as the system, so anything attributing the event to a person must carry the actor in the payload.
- **Unscoped egress does not exist here.** `Entry.validate` rejects a zero `OrgID` and `ProjectID`, so there is no way to enqueue a system-wide delivery. Genuinely system-wide outbound work belongs on a `scheduler.Job` with `ScopeSystem`, which fires once per tick unscoped — and which must not touch a tenant-scoped store, because nothing narrows it.

## Gotchas

- Deduplication is the tenant's job too: `UNIQUE (org_id, event_id, target)` means `Enqueue` ignores a repeat, and tenants dedupe on `event_id`.
- `ClaimLease` (5 minutes) releases an entry whose dispatcher died, so a `Deliverer` must tolerate a duplicate delivery. This is at-least-once.
- `MaxAttempts` is 10 with jittered exponential `Backoff`; a row that exhausts it becomes `StatusFailed` and is retained as the delivery log.
- `httpclient.WithAllowPrivateHosts(true)` exists but has **no config key** here. Opening it is an operator decision; a tenant able to widen it by supplying a URL is exactly how SSRF works.

## Contracts

[`surfaces`](../surfaces/README.md) (R10) · [`request scope`](../multitenancy/request-scope.md) · [`architecture`](../architecture/README.md) · [`platform`](../platform/README.md)
