# Architecture

## Context

```
          administrator's browser
                   │ HTTP (127.0.0.1:8080 / internal network)
                   ▼
┌──────────── ui container ─────────────┐        ┌──────────── ds container ────────────┐
│ ds-ui (Go, single binary)             │        │ debian:trixie-slim + 389-ds-base      │
│  ├─ HTTP API /api/v1 (chi + huma)     │ LDAPI  │ ns-slapd (foreground, PID 1/entry)    │
│  ├─ SSE /api/v1/events                ├───────►│  :3389 LDAP  :3636 LDAPS              │
│  ├─ SPA (Vue 3, go:embed)             │ socket │  /data/run/slapd-<inst>.socket        │
│  └─ providers                         │        │                                       │
│      LDAPConn / Lifecycle / Files /   │        └──────────────┬────────────────────────┘
│      Certs / Offline                  │                       │
└───────┬───────────────┬───────────────┘                       │
        │ volume /data  │ HTTP                                  │ volume /data
        │ (shared)      ▼                                       │
        │      ┌──────────────────────┐                         │
        │      │ docker-socket-proxy  │── restart ds ──► Docker │
        │      └──────────────────────┘                         │
        └──────────────────── ds-data ─────────────────────────┘
```

This is mode **C (sidecar)** — the primary one. Modes A and B differ only in the set of active providers.

## Deployment modes

| | A. All-in-one | B. Remote | C. Sidecar |
|---|---|---|---|
| Containers | 1 (s6-overlay: ns-slapd + ds-ui) | ui (+proxy) separate from DS | ds + ui + docker-socket-proxy |
| LDAPConn | ldapi + SASL EXTERNAL | ldap(s) + simple bind (DM/service account) | ldapi + SASL EXTERNAL |
| Lifecycle | s6 (`s6-svc`) | docker-socket-proxy / none | docker-socket-proxy |
| Files | local `/data` | none | shared volume `/data` |
| Certs | `dsconf security ... -j` | none | `certutil`/`pk12util` |
| Offline | `dsctl` (stopping ns-slapd via s6) | none | none (`docker exec ds dsctl …`) |
| Feature coverage | ~100% | ~85% | ~95% |

Details — in [decisions.md](decisions.md) (ADR-002) and [features.md](features.md).

## Backend components

```
cmd/ds-ui/              entry point, config, dependency wiring, healthcheck subcommand
internal/config/        env → Config, validation
internal/ldapx/         LDAP layer, unaware of 389:
  ├─ pool               connections, reconnect, timeouts, bind strategies
  ├─ schema             RFC 4512 parser, must/may resolution, cache
  ├─ controls           VLV, SSS (ASN.1), GER
  ├─ ldif               LDIF read/write (on top of go-ldap/ldif)
  └─ dn                 DN normalization, escaping, comparison
internal/ds/            389-ds knowledge ("mini-lib389"):
  ├─ rootdse, version   version, DB implementation, capabilities
  ├─ backends, mapping  suffixes, backends, mapping tree
  ├─ indexes, vlv
  ├─ tasks              cn=tasks task engine
  ├─ monitor
  ├─ plugins            plugin registry + complex plugins
  ├─ pwpolicy, accounts
  ├─ schema             custom schema modification
  ├─ replication        replica, changelog, agreements, RUV
  ├─ security           cn=encryption, TLS flags
  └─ restartreg         registry of attributes requiring a restart
internal/pages/         declarative page engine (YAML → GET/PATCH)
internal/providers/     interfaces and implementations: lifecycle (s6, docker, none),
                        files (local, none), certs (certutil, dsconf, none), offline
internal/logs/          log reading, tail, filtering
internal/events/        event bus → SSE (tasks, logs, server state)
internal/api/           huma HTTP handlers, DTOs, error mapping
pages/                  YAML page descriptions (go:embed)
web/                    SPA
```

Dependency rule: `api → ds/pages/logs/providers → ldapx`. `ldapx` imports nothing from `ds`. Handlers do not access LDAP directly — only via `ds`/`pages`.

## Providers

Below are the target interfaces. Implemented in stage 0: `ldapx.Client.Do(ctx, fn)` (a shared connection with reconnect), `providers.Lifecycle` (`docker`, `none`), `Files`/`Certs` detection → `Capabilities`.

```go
type LDAPConn interface {
    // Connection for a request. Currently a shared pool bound as DM.
    // Once authentication is introduced — a connection as the user from the context.
    Conn(ctx context.Context) (*ldap.Conn, func(), error)
}

type Lifecycle interface {
    Status(ctx context.Context) (ServerState, error) // running / stopped / restarting / unknown
    Restart(ctx context.Context) error
    Stop(ctx context.Context) error
    Start(ctx context.Context) error
}

type Files interface {
    Logs() LogStore       // paths are taken from cn=config (nsslapd-*log)
    LDIF() FileStore      // nsslapd-ldifdir
    Backups() FileStore   // nsslapd-bakdir
}

type Certs interface { /* list, get, importPEM, importP12, csr, delete, setTrust */ }
type Offline interface { /* dbverify, db2ldif, ldif2db — mode A only */ }
```

On startup ds-ui checks the availability of each provider (socket, volume, proxy, binaries) and publishes the result at `GET /api/v1/system/capabilities`. The SPA hides/disables unavailable functions.

## Frontend

- Multilingual: vue-i18n, a language = one JSON file in `web/src/i18n/locales/`, en + ru are required, the backend returns machine-readable codes, and the UI translates them ([i18n.md](i18n.md), ADR-009).
- Vue 3 + TypeScript + Vite, Naive UI (NTree with lazy loading and virtual scrolling, NDataTable, NForm), @lucide/vue icons, vue-router, Pinia.
- The TS API client is generated from OpenAPI (`openapi-typescript` + `openapi-fetch`).
- Dynamic form rendering for YAML pages and for schema-driven entries.
- SSE client for tasks, logs, server state.
- The build is embedded in the Go binary via `go:embed`; SPA routing falls back to `index.html`.

## Key flows

**Changing a setting:**
`PATCH /pages/{id}` → `pages` reads the entry, computes the diff → a single `modify` → `restartreg` checks the attributes → response with `restartRequired`; the flag accumulates in the server state and is shown as a banner until restart.

**Task (e.g. backup):**
`POST /backups` → `ds/tasks` creates `cn=backup_<ts>,cn=backup,cn=tasks,cn=config` → `202 {taskId}` → a background poller reads the status every 1 s → events to SSE → on completion, an entry in the task history.

**Restart (mode C):**
`POST /system/restart` → Lifecycle(docker) → `POST /containers/<ds>/restart` via the proxy → state `restarting` → polling LDAPI until ready → `running`, restart flag cleared.

**Log tail:**
`GET /logs/{name}/stream` (SSE) → `logs` opens the file on the shared volume, tracks growth and rotation (inode change) → lines into the stream.

## Security

- There is no authentication. ds-ui operates with Directory Manager rights (autobind via LDAPI as uid 389 `dirsrv` — without root, or the DM password in mode B).
- The UI port is published only on `127.0.0.1` or to the internal network; every page shows a banner about the no-authentication mode.
- Docker API — only via `wollomatic/socket-proxy` with an allowlist: `GET …/containers/<ds>/json` and `POST …/containers/<ds>/(restart|stop|start)`, source — the `ui` container only (ADR-002). Even if ds-ui is compromised, it cannot create a container or run `exec`.
- File operations — only within paths obtained from `cn=config`, with path traversal checks.
- Secrets (the DM password in mode B, remote node credentials) — only from files (`*_FILE`), not from env values.
- CSRF: in the absence of authentication — `Origin`/`Host` checks and a mandatory custom header for mutations.
- Preparing for authentication: `LDAPConn.Conn(ctx)` receives the request context — later the connection will take the bind account from the session, and permissions will be determined by the DS's own ACIs.
