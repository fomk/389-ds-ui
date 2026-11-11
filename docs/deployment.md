# Deployment

> ds-ui itself connects to the server with Directory Manager privileges; users sign in with their directory accounts (see [Sign-in](#sign-in)). Serve it over HTTPS (a reverse proxy) when it is reachable beyond `127.0.0.1`.

## Images

| Image | Dockerfile | Base | Contents | Size |
|---|---|---|---|---|
| `ds` | `deploy/ds/Dockerfile` | `debian:trixie-slim` | 389-ds-base 3.1.2, tini, `dscontainer` + `ds-entrypoint` wrapper | ~436 MB |
| `ui` (sidecar) | `deploy/ui/Dockerfile`, target `sidecar` | `debian:trixie-slim` | ds-ui + `libnss3-tools` (certutil), user 389 | ~152 MB |
| `ui` (static) | `deploy/ui/Dockerfile`, target `static` | `distroless/static:nonroot` | ds-ui only (mode B) | ~20 MB |
| `aio` | — | — | mode A, stage 7 | — |

The uid/gid of the `dirsrv` user in the `ds` and `ui` (sidecar) images is fixed: **389:389** (`--build-arg DS_UID/DS_GID`).

## Mode C — sidecar (default)

File: [`compose.yaml`](../compose.yaml). Run:

```bash
make up        # creates secrets/dm_password (if missing), detects DOCKER_GID and runs docker compose up -d --build
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

## Mode A — all-in-one

Planned for stage 7 (DS image + s6-overlay + ds-ui, Lifecycle provider `s6`). Currently `DSUI_LIFECYCLE=s6` makes ds-ui exit with a "not implemented" error.

## ds-ui environment variables

| Variable | Default | Description |
|---|---|---|
| `DSUI_LISTEN` | `:8080` | HTTP address |
| `DSUI_MODE` | `sidecar` | `sidecar` / `remote` / `aio` |
| `DSUI_INSTANCE` | `localhost` | 389 instance name |
| `DSUI_LDAP_URL` | sidecar/aio: `ldapi://%2Fdata%2Frun%2Fslapd-<instance>.socket`; remote: required | `ldapi://…`, `ldap://…`, `ldaps://…` |
| `DSUI_LDAP_BIND_DN` | — | simple bind (otherwise: SASL EXTERNAL for ldapi, anonymous for ldap) |
| `DSUI_LDAP_BIND_PASSWORD_FILE` | — | password file; required with `DSUI_LDAP_BIND_DN` |
| `DSUI_LDAP_CA_FILE` | — | CA for LDAPS / StartTLS |
| `DSUI_LDAP_TIMEOUT` | `10s` | connection and operation timeout |
| `DSUI_DATA_DIR` | `/data` | root of the shared volume (sidecar/aio) |
| `DSUI_TLS_HOST` | the host of an `ldap(s)://` URL, else `ds` | where ds-ui reaches the LDAPS port to check the certificate the server presents (the port is `nsslapd-securePort`) |
| `DSUI_LIFECYCLE` | `docker` if `DSUI_DOCKER_HOST` is set (sidecar); `s6` (aio); otherwise `none` | process management provider |
| `DSUI_DOCKER_HOST` | — | `tcp://docker-proxy:2375` |
| `DSUI_DS_CONTAINER` | — | DS container name for the Docker API |
| `DSUI_ALLOWED_ORIGINS` | `http://127.0.0.1:8080,http://localhost:8080` | origins under which the UI is opened: the `Host` header of every request (except `/healthz`, `/readyz`) must match one of them (DNS-rebinding protection, otherwise 421), and state-changing requests must carry a matching `Origin`. Add your URL when using an SSH tunnel on another port or a hostname; in compose it can be overridden from the environment |
| `DSUI_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

HTTP endpoints for orchestrators: `/healthz` — the process is alive (independent of DS); `/readyz` — there is an LDAP connection to DS. Image healthcheck: `ds-ui healthcheck`.

## `/data` volume (`ds` image)

The layout is defined by `dscontainer` (verified on 389-ds 3.1.2):

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
