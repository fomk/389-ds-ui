# 389-ds-ui

A web administration UI for **389 Directory Server** (389-ds-base) with a convenient LDAP editor,
shipped as a lightweight Docker service next to the DS container.

> Status: **1.0** — see the [changelog](CHANGELOG.md) and the [user guide](docs/user-guide.md). Images: `ghcr.io/fomk/389-ds-ui` (with the 389-ds image `ghcr.io/fomk/389-ds-base-docker`).

## Key features

- **Go backend**: a single static binary with the SPA embedded, 20–150 MB image.
- **389-ds-base in Docker on Debian** (trixie, 389-ds 3.x, LMDB by default). The DS image is maintained in a separate repository, 389-ds-base-docker; ds-ui runs next to it and is not bundled into it.
- **Deployment modes** (one codebase, different providers):
  - **C — sidecar** (default): two containers, shared `/data` volume, LDAPI autobind, restart through a Docker API proxy;
  - **B — remote**: LDAP over the network only (for remote nodes / replicas).
- **What it covers**: an LDAP browser and editor (schema-aware forms, templates, ACIs with effective rights, account state, groups, LDIF and search), monitoring and a health check, databases (suffixes and sub-suffixes, indexes, VLV, attribute encryption, backups, LDIF import/export), logs with live follow, server and plug-in settings, schema, password policies and an account report, roles and Class of Service, replication with a topology report, TLS and the certificate database. The full list per deployment mode is in [docs/features.md](docs/features.md).
- **Sign-in with directory accounts** (`DSUI_AUTH=ldap`, the default): Directory Manager or members of the configured admin/operator groups; directory data is accessed as the signed-in user, so the server's ACIs apply ([docs/deployment.md](docs/deployment.md#sign-in)).
- **Server certificate from files** of certbot, lego or cert-manager: ds-ui imports renewed certificates and can restart the server ([docs/deployment.md](docs/deployment.md#certificate-from-files)).
- **Multilingual UI**: English and Russian; adding a language is a single JSON file ([docs/i18n.md](docs/i18n.md)).

## Documentation

| Document | Contents |
|---|---|
| [docs/user-guide.md](docs/user-guide.md) | User guide: signing in, every section of the interface, troubleshooting |
| [CHANGELOG.md](CHANGELOG.md) | What changed in each release |
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

From a checkout of this repository (the compose files and the secret live here):

```bash
make secrets                 # creates secrets/dm_password
# run the published release …
DSUI_IMAGE=ghcr.io/fomk/389-ds-ui:1.0 DOCKER_GID=$(stat -c %g /var/run/docker.sock) docker compose up -d --no-build
# … or build ds-ui from the checkout
make up

# UI:   http://127.0.0.1:8080 — sign in as "Directory Manager" with the password in secrets/dm_password
# LDAP: ldap://localhost:389, suffix dc=example,dc=com (DS_SUFFIX overrides it)
```

The UI listens on 127.0.0.1 only; for remote access use an SSH tunnel or a reverse proxy with HTTPS. Details: [docs/deployment.md](docs/deployment.md); how to use it: [docs/user-guide.md](docs/user-guide.md); development: [docs/development.md](docs/development.md).

## License

[MIT](LICENSE)
