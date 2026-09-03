# Configuration

## Precedence

`defaults <- config.yaml <- ALT_* env`. Env always wins. Nested keys map
by dot notation with dots → underscores:

| YAML | Env var |
|---|---|
| `mode: cloud` | `ALT_MODE=cloud` |
| `db.driver: postgres` | `ALT_DB_DRIVER=postgres` |
| `http.basePath: /altempl` | `ALT_HTTP_BASE_PATH=/altempl` |
| `tokens.audience: urn:altempl:api` | `ALT_TOKENS_AUDIENCE=urn:altempl:api` |
| `tenant.singletonOrg.slug: main` | `ALT_TENANT_SINGLETON_ORG_SLUG=main` |

`config.example.yaml` and `.env.example` are generated from struct tags
in `internal/platform/config`. After editing those, run `make config-examples`.

## Awareness markers

Every `.env.example` field carries a marker in `[brackets]`:

| Marker | Meaning |
|---|---|
| `required` | boot fails if unset (in the applicable mode) |
| `bootstrap` | locks in at first boot; changing later is a no-op on persisted data |
| `secret` | never commit; `secret` fields never emit defaults |
| `mode:cloud` / `mode:selfhosted` | only meaningful in the named mode |

## Modes

| Property | `selfhosted` | `cloud` |
|---|---|---|
| DB driver | `sqlite` or `postgres` | `postgres` only |
| OIDC identity | optional | required (`issuer` + `clientID` + `clientSecret`) |
| Local password login | on by default | off; set `ALT_GENESIS_BREAK_GLASS=true` to re-enable |
| Org creation from UI | disabled | enabled |
| Public signup | disabled | enabled |

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

## Validation

`config.Validate()` runs on every boot with specific messages
(e.g. `cloud requires db.driver=postgres, got "sqlite"`).
