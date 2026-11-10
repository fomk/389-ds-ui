# Development

## Repository layout

```
389-ds-ui/
├─ cmd/ds-ui/                 main: serve | healthcheck | openapi | version
├─ internal/
│  ├─ config/                 env → Config
│  ├─ ldapx/                  LDAP client (ldapi/ldap/ldaps, EXTERNAL/simple, reconnect)
│  ├─ ds/                     389-ds knowledge: root DSE, monitor, backends, indexes, tasks, restart registry
│  ├─ providers/              Lifecycle (docker, none), Files/Certs detection, Capabilities
│  ├─ entries/                LDAP editor operations: read, children, modify, create, delete, rename, password
│  ├─ templates/              entry templates for the create form (defs/*.yaml, go:embed)
│  ├─ monitor/                in-memory history of monitor counters
│  ├─ events/                 in-process event bus behind /api/v1/events (SSE)
│  ├─ files/                  backup and LDIF directories on the shared volume
│  ├─ logs/                   log tail, filter and follow (with rotation)
│  ├─ api/                    huma handlers, error mapping, CSRF, SPA serving
│  └─ webui/                  go:embed of the built SPA (dist/ is generated, only .gitkeep in git)
├─ api/openapi.json           OpenAPI contract (generated: make gen, checked in CI)
├─ web/                       SPA: Vue 3 + Vite + Naive UI
│  ├─ src/api/                schema.d.ts — generated (do not edit), client.ts — wrapper
│  ├─ src/i18n/               vue-i18n, locales/<language>.json (see i18n.md)
│  ├─ src/components/         shared components (LocaleSwitcher, …)
│  ├─ src/layout/, views/     shell and pages
│  └─ src/stores/             Pinia
├─ deploy/
│  ├─ ds/                     389-ds image on Debian: Dockerfile + entrypoint.sh
│  └─ ui/                     ds-ui image: targets sidecar / static
├─ test/integration/          Go tests against a real DS (testcontainers-go, tag integration)
├─ compose.yaml               mode C (sidecar)
├─ compose.remote.yaml        mode B (remote)
├─ secrets/                   dm_password (not in git)
├─ .github/workflows/ci.yml   lint, unit, generation check, SPA build, integration, images
├─ Makefile
└─ docs/
```

Planned by stage: `deploy/aio/` (stage 7), `test/e2e/` (Playwright, stage 7).

## Tools

| Tool | Version | Note |
|---|---|---|
| Go | 1.26 (see `go.mod`) | |
| Node.js | 22 LTS | |
| Docker / Compose | 27+ / v2 | |
| TypeScript | ~5.9 (pinned) | TS 7 is incompatible with `vue-tsc` and `openapi-typescript` |
| Naive UI | ^2.45 | instead of PrimeVue (commercial license since 5.x), ADR-004 |
| golangci-lint | latest | optional locally: `docker run golangci/golangci-lint …` |

## Make targets

| Target | Action |
|---|---|
| `make up` / `make down` | sidecar stack (ds + ui + docker-proxy) on `127.0.0.1:8080` |
| `make dev` | DS in Docker + `go run ./cmd/ds-ui` in remote mode (DM simple bind over `ldap://127.0.0.1:389`); frontend separately with `cd web && npm run dev` (Vite on :5173 proxies `/api` to :8080) |
| `make gen` | `api/openapi.json` from Go types → `web/src/api/schema.d.ts` |
| `make check-gen` | `gen` + check that the generated files are committed |
| `make lint` | go vet, gofmt, golangci-lint (if installed), vue-tsc, translation check (`check:locales`), npm license check |
| `make test` | Go unit tests |
| `make itest` | build the DS image and run the integration tests |
| `make build` | SPA + `bin/ds-ui` |
| `make images` | ds, ui (sidecar and static) images |
| `make secrets` | create `secrets/dm_password` |

After changing API types in Go: run `make gen` and commit `api/openapi.json` and `web/src/api/schema.d.ts`.

## Testing

- **Unit:** schema parser, DN utilities, entry diff, ASN.1 controls, YAML page engine, error mapping, log parsing — Go `go test`, no network. Frontend helpers (DN, LDAP time parsing, formatting) — vitest (`npm test` in `web/`, `*.test.ts` next to the code).
- **Integration (primary level):** `testcontainers-go` starts the built DS image (one per test package, `DS_IMAGE`, default `389-ds-ui/ds:dev`), connecting via DM simple bind over TCP (LDAPI is not reachable from the host: autobind maps only uid 0/389). Each test works in its own suffix/subtree. Every function in `internal/ds` must have an integration test — this is our substitute for the "reference" lib389. Files are tagged `//go:build integration`.
- **Replication:** a separate suite with 2 DS instances on a shared network; runs in CI nightly and on PRs touching `internal/ds/replication`.
- **Version matrix:** CI runs the integration tests on the target Debian version and (optionally) on the upstream 389ds image to catch `cn=config` changes early.
- **E2E:** Playwright — key scenarios (create a user, change an index, backup/restore, log tail).

## Conventions

**Language:** everything in the repository — README, `docs/`, ADRs, code comments, commit messages — is written in English. The UI itself is multilingual ([i18n.md](i18n.md)).

**Go**
- Errors are wrapped with context (`fmt.Errorf("create backend %q: %w", name, err)`); LDAP errors are mapped to HTTP only in `internal/api`.
- `context.Context` is the first argument in everything that touches the network/disk.
- DNs only via `internal/ldapx/dn` (parsing, RDN value escaping, case-insensitive comparison). No string concatenation for DNs and filters: filters go through `ldap.EscapeFilter`.
- 389 attribute names are constants in `internal/ds`, not string literals in handlers.
- Logging via `slog`, without passwords or values of `userPassword`/`nsslapd-rootpw`/`*Credentials`.

**Frontend**
- No UI text in code: only `t('…')` with keys in `en.json` and `ru.json` ([i18n.md](i18n.md)).
- Only the generated API client.
- Entry forms and YAML pages go through shared dynamic rendering components.
- All destructive actions require confirmation stating the DN/scope.

**Versioning:** SemVer; CHANGELOG in Keep a Changelog format; Conventional Commits.

## How to add a settings page

1. Describe the page in `internal/pages/defs/*.yaml`: DN, fields, types, `restart`/`danger` flags and, if needed, a `when: {dbImpl: …}` condition (format: [internal/pages/README.md](../internal/pages/README.md)).
2. Add its texts (`pages.<id>.*`) to `web/src/i18n/locales/en.json` and `ru.json`.
3. Run `go test ./internal/pages` (definitions and texts) and `make itest` (every attribute exists on a real server).
4. The page appears under "Server settings" automatically.
