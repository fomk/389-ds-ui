# Deployment

> ds-ui itself connects to the server with Directory Manager privileges; users sign in with their directory accounts (see [Sign-in](#sign-in)). Serve it over HTTPS (a reverse proxy) when it is reachable beyond `127.0.0.1`.

## Images

| Image | Dockerfile | Base | Contents | Size |
|---|---|---|---|---|
| `ds` | 389-ds-base-docker repository | `debian:trixie-slim` | 389-ds-base 3.3.1 built from the release tarball, tini, `dscontainer` + `ds-entrypoint` wrapper; used by `compose.yaml` as `389-ds-base:3.3.1` (`DS_IMAGE`) | ~468 MB |
| `ds` (dev) | `deploy/ds/Dockerfile` | `debian:trixie-slim` | Debian's 389-ds-base 3.1.2, same layout; CI and `DS_IMAGE=389-ds-ui/ds:dev` | ~436 MB |
| `ui` (sidecar) | `deploy/ui/Dockerfile`, target `sidecar` | `debian:trixie-slim` | ds-ui + `libnss3-tools` (certutil), user 389 | ~152 MB |
| `ui` (static) | `deploy/ui/Dockerfile`, target `static` | `distroless/static:nonroot` | ds-ui only (mode B) | ~20 MB |

The `deploy/ds` image is for development and CI. The DS image itself comes from the separate repository 389-ds-base-docker (built from the upstream release tarball); it keeps the same layout — state in `/data`, LDAPI socket `/data/run/slapd-localhost.socket`, uid/gid 389, `DS_DM_PASSWORD_FILE` / `DS_SUFFIX` — so ds-ui works with it unchanged. There is no all-in-one image (ADR-012).

The uid/gid of the `dirsrv` user in the `ds` and `ui` (sidecar) images is fixed: **389:389** (`--build-arg DS_UID/DS_GID`).

## Mode C — sidecar (default)

File: [`compose.yaml`](../compose.yaml). Run:

```bash
make up        # creates secrets/dm_password (if missing), detects DOCKER_GID and runs docker compose up -d --build
# the ds image (389-ds-base:3.3.1) must exist locally: build it in 389-ds-base-docker,
# or run with DS_IMAGE=389-ds-ui/ds:dev after `make ds-image`
make down      # stop; the ds-data volume is preserved
```

Services:

| Service | Purpose |
|---|---|
| `ds` | 389-ds; creates the instance in `/data` on first start, suffix from `DS_SUFFIX`; healthcheck — `dscontainer -H` |
| `ui` | ds-ui running as uid 389; LDAPI autobind → DM via the shared volume; port `127.0.0.1:8080` |
| `docker-proxy` | `wollomatic/socket-proxy`: only `GET …/containers/389-ds-ui-ds-1/json` and `POST …/(restart\|stop\|start)`, only from the `ui` service |

Important:
- The compose project name is fixed (`name: 389-ds-ui`), so the DS container is named `389-ds-ui-ds-1` — this name is set in the proxy allowlist and in `DSUI_DS_CONTAINER`. If you change the project name, update both places.
- `docker-proxy` runs as `65534:${DOCKER_GID}`; `DOCKER_GID` is the gid of `/var/run/docker.sock` on the host (`make up` detects it automatically; defaults to 0, which suits Docker Desktop).
- `secrets/dm_password` is bind-mounted, so the file must be readable by uid 389 inside the container (`make secrets` sets 0644). The password is applied by `dscontainer` on every DS start.

### `ds` variables

| Variable | Description |
|---|---|
| `DS_SUFFIX` | suffix created at startup if it does not exist (backend `DS_BACKEND`, default `userroot`) |
| `DS_DM_PASSWORD_FILE` | file with the Directory Manager password (or `DS_DM_PASSWORD`) |
| `DS_STARTUP_TIMEOUT` | readiness wait, s (default 60) |
| `DS_ERRORLOG_LEVEL`, `DS_MEMORY_PERCENTAGE`, `DS_REINDEX` | passed to `dscontainer` as is |

TLS from PEM (until certificate management arrives in the UI in stage 6): at startup `dscontainer` imports `/data/tls/server.key`, `/data/tls/server.crt` and the CA from `/data/tls/ca/*.crt` into the NSS DB, provided `/data/config/pwdfile.txt` also exists (it is created together with the instance).

## Mode B — remote

File: [`compose.remote.yaml`](../compose.remote.yaml).

```bash
DSUI_LDAP_URL=ldaps://ds1.example.com:636 docker compose -f compose.remote.yaml up -d
```

The bind password is taken from `secrets/dm_password`. For LDAPS with a custom CA, mount the CA and set `DSUI_LDAP_CA_FILE`. For `ldap://` with `DSUI_LDAP_CA_FILE`, StartTLS is used.

Not available: logs, backup/LDIF files, NSS certificates, restart.

## ds-ui environment variables

| Variable | Default | Description |
|---|---|---|
| `DSUI_LISTEN` | `:8080` | HTTP address |
| `DSUI_MODE` | `sidecar` | `sidecar` / `remote` |
| `DSUI_INSTANCE` | `localhost` | 389 instance name |
| `DSUI_LDAP_URL` | sidecar: `ldapi://%2Fdata%2Frun%2Fslapd-<instance>.socket`; remote: required | `ldapi://…`, `ldap://…`, `ldaps://…` |
| `DSUI_LDAP_BIND_DN` | — | simple bind (otherwise: SASL EXTERNAL for ldapi, anonymous for ldap) |
| `DSUI_LDAP_BIND_PASSWORD_FILE` | — | password file; required with `DSUI_LDAP_BIND_DN` |
| `DSUI_LDAP_CA_FILE` | — | CA for LDAPS / StartTLS |
| `DSUI_LDAP_TIMEOUT` | `10s` | connection and operation timeout |
| `DSUI_DATA_DIR` | `/data` | root of the shared volume (sidecar) |
| `DSUI_CERT_FILE`, `DSUI_CERT_KEY_FILE`, `DSUI_CERT_CHECK_INTERVAL`, `DSUI_CERT_AUTO_RESTART` | — | server certificate from files, see [Certificate from files](#certificate-from-files) |
| `DSUI_TLS_HOST` | the host of an `ldap(s)://` URL, else `ds` | where ds-ui reaches the LDAPS port to check the certificate the server presents (the port is `nsslapd-securePort`) |
| `DSUI_LIFECYCLE` | `docker` if `DSUI_DOCKER_HOST` is set (sidecar), otherwise `none` | process management provider |
| `DSUI_DOCKER_HOST` | — | `tcp://docker-proxy:2375` |
| `DSUI_DS_CONTAINER` | — | DS container name for the Docker API |
| `DSUI_ALLOWED_ORIGINS` | `http://127.0.0.1:8080,http://localhost:8080` | origins under which the UI is opened: the `Host` header of every request (except `/healthz`, `/readyz`) must match one of them (DNS-rebinding protection, otherwise 421), and state-changing requests must carry a matching `Origin`. Add your URL when using an SSH tunnel on another port or a hostname; in compose it can be overridden from the environment |
| `DSUI_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

HTTP endpoints for orchestrators: `/healthz` — the process is alive (independent of DS); `/readyz` — there is an LDAP connection to DS. Image healthcheck: `ds-ui healthcheck`.

## `/data` volume (`ds` image)

The layout is defined by `dscontainer` (verified on 389-ds 3.1.2 and 3.3.1):

| Path | Contents |
|---|---|
| `/data/config` | instance configuration (`dse.ldif`, schema, NSS DB `cert9.db`/`key4.db`), symlinked from `/etc/dirsrv/slapd-localhost` |
| `/data/ssca` | self-signed CA, symlinked from `/etc/dirsrv/ssca` |
| `/data/db` | databases (LMDB) |
| `/data/logs` | `access`, `errors`, `audit` (also used by `auditfail`), `security` (JSON lines); rotated files `<name>.<timestamp>` |
| `/data/ldif` | LDIF for import/export (`nsslapd-ldifdir`) |
| `/data/bak` | backups (`nsslapd-bakdir`) |
| `/data/run` | LDAPI socket `slapd-localhost.socket`, pid, `lock/`, `dbhome/` |

## Backup

- Regular backups — via a backup task (UI from stage 2) or `docker compose exec ds dsconf ldapi://%2fdata%2frun%2fslapd-localhost.socket backup create`; copy the `/data/bak` directory off the host.
- A snapshot of the `ds-data` volume is consistent only while DS is stopped.

## Certificate from files

ds-ui can keep the server certificate in step with PEM files maintained by an external tool — certbot, lego, acme.sh, cert-manager, or a deploy script. ds-ui does not obtain certificates itself (no ACME client, ADR-013); it checks the files and imports what they contain. Sidecar mode only (it needs the NSS database).

| Variable | Default | Meaning |
|---|---|---|
| `DSUI_CERT_FILE` | — | PEM file with the server certificate first, then its chain (`fullchain.pem`, `tls.crt`). Turns the feature on |
| `DSUI_CERT_KEY_FILE` | — | PEM private key, unencrypted (`privkey.pem`, `tls.key`); empty: the key is in `DSUI_CERT_FILE` |
| `DSUI_CERT_CHECK_INTERVAL` | `10m` | How often the files are read (at least `10s`) |
| `DSUI_CERT_AUTO_RESTART` | `false` | Restart the server after an import (needs the Docker proxy). Otherwise the "restart required" banner appears and the new certificate is presented after the next restart |

What happens on each check:

- The key must belong to the first certificate, and the certificate must be valid now; otherwise nothing changes and the error is shown on the Security page.
- If the server is already configured with this certificate, nothing happens.
- Otherwise the certificate, its key and chain are imported into the NSS database and made the server certificate (`nsSSLPersonalitySSL`), and the import is recorded in the task history. A certificate with the **same subject** as one already in the database (a renewal) replaces it under its nickname — NSS cannot keep two such certificates apart. A new subject gets the nickname `Server-Cert-auto-<date>-<time>`; earlier automatic imports are deleted once they are no longer the configured certificate.
- The running server keeps presenting the old certificate until it restarts.
- While a backend encrypts attributes, a certificate with **another key** is refused (the server would not start: the encryption key is wrapped with the server key). Renew with the same key — `certbot renew --reuse-key`, cert-manager `privateKey.rotationPolicy: Never` — or turn attribute encryption off first. The same check applies when the server certificate is chosen manually.

The files must be readable by uid 389 inside the `ui` container. Examples:

```yaml
# compose.override.yaml — certbot on the host
services:
  ui:
    environment:
      DSUI_CERT_FILE: /certs/live/ldap.example.org/fullchain.pem
      DSUI_CERT_KEY_FILE: /certs/live/ldap.example.org/privkey.pem
      DSUI_CERT_AUTO_RESTART: "true"
    volumes:
      - /etc/letsencrypt:/certs:ro
```

certbot writes `privkey.pem` readable by root only; give uid 389 access, e.g. with a deploy hook (`/etc/letsencrypt/renewal-hooks/deploy/`) that copies the pair to a directory owned by 389, and mount that directory instead. In Kubernetes, mount the cert-manager secret (`tls.crt`, `tls.key`) into the ds-ui container with `defaultMode` and `fsGroup: 389`; the secret's symlinked files are re-read on every check.

Public CAs: Let's Encrypt certificates no longer carry the client-authentication usage, so they do not suit replication agreements that authenticate with certificates (`SSLCLIENTAUTH`); use an internal CA there.

## Sign-in

`DSUI_AUTH=ldap` (default) asks for a directory account; `DSUI_AUTH=none` turns sign-in off (then publish the port only on `127.0.0.1` or a trusted network).

| Variable | Default | Meaning |
|---|---|---|
| `DSUI_AUTH` | `ldap` | `ldap` or `none` |
| `DSUI_ADMIN_GROUPS` | — | Groups (DNs separated by `;`) whose members are administrators |
| `DSUI_OPERATOR_GROUPS` | — | Groups whose members are operators |
| `DSUI_SESSION_IDLE` | `30m` | A session ends after this long without requests |
| `DSUI_SESSION_MAX` | `12h` | …and at the latest after this long |

- The user name is a `uid` (searched in all suffixes, it must be unique), a full DN, or `Directory Manager`. The password is checked with a bind to the server, so its password policy, lockout and security log apply.
- **Roles.** Directory Manager is always an administrator; otherwise the role comes from membership (`member`, `uniqueMember`, or `memberOf` for nested groups) of the configured groups. Accounts in neither group cannot sign in. Without groups only Directory Manager can sign in.
  - *Administrators* may do everything.
  - *Operators* see the administration pages (settings, monitoring, logs, replication…) without changing them, and work with directory data.
- **Directory data** (LDAP browser, search, LDIF, export) is read and written with Proxied Authorization as the signed-in user: the server applies **their ACIs**, so a group member who needs to edit entries also needs an ACI that allows it. 389-ds records ds-ui's own bind DN in `modifiersName`; ds-ui's request log and task history name the user.
- Server administration (cn=config, tasks, files, certificates) is done by ds-ui with its own privileges and limited by role.
- Sessions are kept in memory: restarting ds-ui signs everyone out. The cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` behind HTTPS (also when a proxy sets `X-Forwarded-Proto: https`). Five failed attempts per user name and client block sign-in for five minutes.

Example: administrators and operators from two groups.

```sh
DSUI_ADMIN_GROUPS="cn=ds-ui admins,ou=Groups,dc=example,dc=com" \
DSUI_OPERATOR_GROUPS="cn=ds-ui operators,ou=Groups,dc=example,dc=com" make up
```
