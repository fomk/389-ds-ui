# 389-ds-ui

A web administration UI for **389 Directory Server** (389-ds-base) with a convenient LDAP editor,
shipped as a lightweight Docker service next to the DS container.

> Status: **stage 0 complete** — skeleton, images, sidecar and remote modes, CI. Next up: stage 1 (LDAP editor), see the [roadmap](docs/roadmap.md).

## Key features

- **Go backend**: a single static binary with the SPA embedded, 20–150 MB image.
- **389-ds-base in Docker on Debian** (trixie, 389-ds 3.x, LMDB by default). The DS image is maintained in a separate repository, 389-ds-base-docker; ds-ui runs next to it and is not bundled into it.
- **Deployment modes** (one codebase, different providers):
  - **C — sidecar** (default): two containers, shared `/data` volume, LDAPI autobind, restart through a Docker API proxy;
  - **B — remote**: LDAP over the network only (for remote nodes / replicas).
- **Sign-in with directory accounts** (`DSUI_AUTH=ldap`, the default): Directory Manager or members of the configured admin/operator groups; directory data is accessed as the signed-in user, so the server's ACIs apply ([docs/deployment.md](docs/deployment.md#sign-in)).
- **Multilingual UI**: English and Russian; adding a language is a single JSON file ([docs/i18n.md](docs/i18n.md)).

## Documentation

| Document | Contents |
|---|---|
| [docs/roadmap.md](docs/roadmap.md) | Stages, scope, definitions of done, estimates, risks |
| [docs/architecture.md](docs/architecture.md) | Components, providers, modes, data flows, security |
| [docs/features.md](docs/features.md) | Feature matrix: implementation mechanism, availability per mode, stage |
| [docs/decisions.md](docs/decisions.md) | Architecture decision records (ADRs) and the alternatives considered |
| [docs/api.md](docs/api.md) | REST API conventions, tasks, SSE, errors |
| [docs/development.md](docs/development.md) | Repository layout, tooling, testing, conventions |
| [docs/deployment.md](docs/deployment.md) | Images, compose, environment variables, volumes, security |
| [docs/ds-reference.md](docs/ds-reference.md) | 389-ds internals reference: DNs, attributes, tasks, monitoring |
| [docs/i18n.md](docs/i18n.md) | Localisation: adding languages and strings, plurals |

## Quick start

```bash
make up
# UI:   http://127.0.0.1:8080
# LDAP: ldap://localhost:389, cn=Directory Manager / password in secrets/dm_password
# suffix: dc=example,dc=com (override with DS_SUFFIX)
```

Details: [docs/deployment.md](docs/deployment.md); development: [docs/development.md](docs/development.md).

## License

[MIT](LICENSE)
