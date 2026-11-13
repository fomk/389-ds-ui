# Roadmap

Estimates assume **one full-time developer**. Total to 1.0: **~19–26 weeks (≈5–6 months)**; MVP (v0.1) in **~8 weeks**.

## Overview

| Stage | Name | Duration | Milestone |
|---|---|---|---|
| 0 | Foundation | 1–1.5 wk | — |
| 1 | LDAP editor | 3–4 wk | **M1**: v0.0.x "editor alpha" |
| 2 | Administration core | 3–4 wk | **M2**: v0.1 MVP |
| 3 | Declarative settings | 2–3 wk | v0.2 |
| 4 | Complex screens (schema, policies, plugins, ACI) | 3–4 wk | v0.3 |
| 5 | Replication | 3–4 wk | v0.4 |
| 6 | TLS and certificates | 1.5–2 wk | v0.5 |
| 7 | Stabilization, multi-server, release | 2–3 wk | **v1.0** |
| — | After 1.0 | — | OIDC / proxy sign-in, visual ACI editor, log statistics |

Dependencies: `0 → 1 → 2 → {3, 4, 5, 6} → 7`. After M2, stages 3–6 can be reordered by priority.

---

## Stage 0. Foundation (1–1.5 wk) — ✅ done

**Goal:** a working skeleton in all three modes, CI, integration tests against a real 389-ds.

**Outcome:** modes C (sidecar) and B (remote) work and are verified manually and by tests; the all-in-one image (A) and the s6 provider were moved to stage 7 and later dropped (ADR-012). Answers to the open questions are at the end of this document. Deviations from the plan:
- docker-socket-proxy replaced with `wollomatic/socket-proxy`: tecnativa cannot allow restart without allowing all POSTs (including `create`/`exec`);
- ds-ui runs as uid 389, not 0 — 389-ds maps both root and its own user to DM (see ADR-003);
- UI library: PrimeVue replaced with Naive UI — PrimeVue 5.x moved to a commercial license (see ADR-004);
- added beyond the plan: multilingual UI (en + ru, a language is added with a single file, ADR-009), MIT project license, dependency license checks and translation completeness checks in CI.

**Definition of done for all subsequent stages:** all UI texts go through i18n keys present in `en.json` and `ru.json` ([i18n.md](i18n.md)).

Work items:
- DS image `deploy/ds/`: `debian:trixie-slim` + `389-ds-base`; the entrypoint creates the instance in `/data` (via `dscontainer` if the Debian package ships it, otherwise `dscreate from-file`), ns-slapd in the foreground; LDAPI socket in `/data/run/`, `nsslapd-ldapiautobind: on`, root → DM mapping.
- Go skeleton: `cmd/ds-ui`, config from env, logging (`slog`), chi + huma, `/healthz`, `/api/v1/system/info`, `/api/v1/system/capabilities`.
- Provider interfaces (`LDAPConn`, `Lifecycle`, `Files`, `Certs`, `Offline`) + stub implementations; availability auto-detection.
- LDAP pool: ldapi+EXTERNAL and ldap(s)+simple bind, reconnect, timeouts.
- SPA skeleton: Vue 3 + Vite + Naive UI + vue-router + Pinia, layout (sidebar navigation, header with server info), TS client generation from OpenAPI.
- `go:embed` SPA, multi-stage UI Dockerfile.
- `compose.yaml` (mode C), `compose.remote.yaml` (B) (the all-in-one skeleton was dropped, ADR-012).
- CI: lint (golangci-lint, eslint), unit, integration tests (testcontainers-go with the DS image), image builds.
- Verify and document the open questions (see "Open questions").

**Done when:** `docker compose up` brings up DS + UI; the UI shows the root DSE (version, suffixes) via LDAPI autobind without a password; CI is green; the same binary in mode B connects over `ldap://` with a password.

## Stage 1. LDAP editor (3–4 wk) → M1 — ✅ done

**Goal:** a full-featured data editor — the main value for users.

**Outcome:** all items below are implemented and covered by integration tests against 389-ds 3.1.2 and by UI walkthroughs. Deviations from the plan:
- *VLV + SSS* was replaced by server-side paging of large branches (ADR-010): VLV in 389-ds needs a VLV index per branch, and reading 100k children took the server 8–12 s. Branches above 2000 children are now paged in server order (first page ≈ 40 ms on 100k entries; ds-ui RSS 7 MB instead of 650 MB).
- *Password Modify* works over LDAPI/TLS; on plain LDAP (remote mode without TLS) 389-ds refuses it, ds-ui falls back to replacing `userPassword` and server-generated passwords are unavailable.
- LDIF import does not support `modrdn` (parser limitation) and refuses URL values (`attr:< file://…`) because the parser would read files on the ds-ui host.
- Found and fixed along the way: flaky suffix creation in the DS image (dscontainer runs a temporary server during setup), DNS-rebinding exposure (`Host` allowlist), password hashes in exports (excluded by default), unindexed quick-search filter (3.5 s → 30 ms).

Work items:
- RFC 4512 schema parser (`internal/ldapx/schema`): attributeTypes, objectClasses (SUP inheritance, must/may), syntaxes, matching rules; cache invalidated by the `modifyTimestamp` of `cn=schema`.
- Tree: lazy loading (one-level + `numSubordinates`), Paged Results for large branches, roots from `namingContexts`.
- Entry view: attributes (user/operational), typed rendering (DN links, GeneralizedTime dates, binary, jpegPhoto, certificates).
- Editing: schema-driven form (must/may, single/multi-value, syntax → widget), adding/removing objectClass, diff → modify.
- Creation: templates (user/inetOrgPerson+posixAccount, group/groupOfNames/posixGroup, OU, arbitrary entry), RDN selection; template texts live in the locale files (`templates.*`, ADR-009).
- Rename / move (ModifyDN with newSuperior), delete (subtree deletion with confirmation, page by page).
- Password: modify `userPassword` / Password Modify extop.
- Raw LDIF mode: view/edit an entry; import an LDIF fragment (add/modify via LDAP); export a subtree to LDIF (via search).
- Search: base/scope/filter/attributes, results table, LDIF/CSV export.
- Groups: add/remove members with search, display `memberOf` (read-only).
- ~~VLV control + SSS (custom ASN.1)~~ → server-side paging of large branches (ADR-010).

**Done when:** you can create an OU/user/group, edit, move, delete, search, export and import LDIF; forms correctly reflect the schema; a branch of 100k entries opens without freezing (pagination).

## Stage 2. Administration core (3–4 wk) → M2 / v0.1 MVP — ✅ done

**Goal:** day-to-day administration of a single server.

**Outcome:** every item of the definition of done works through the UI and is covered by integration tests (backup → change → restore verified to roll the change back). Deviations:
- *VLV indexes* are listed, not created or deleted yet (moved to stage 3, together with *default indexes*).
- *"Server" page* merged into Monitoring (version, uptime, start time) and Overview (paths, mode).
- *Restart* uses the Docker proxy (sidecar); there is no s6 provider (ADR-012).
- Findings: the default entry cache of the container (512 KB, 16 % hit ratio on 100k entries) is flagged on the monitoring page; backups contain TLS private keys (`config_files`) — the UI warns; huma silently dropped path parameters of inputs embedding an unexported struct (fixed, HTTP-level tests added); `cn=config` answers searches under a missing base with success (handled).

Work items:
- Task engine `cn=tasks,cn=config`: creation, polling `nsTaskStatus/nsTaskLog/nsTaskExitCode`, progress via SSE, task history in the UI (in-memory + reading existing tasks).
- Monitoring: server (`cn=monitor`, `cn=snmp,cn=monitor`), DB (`cn=monitor,cn=ldbm database`), backends, disk space (`cn=disk space,cn=monitor`); auto-refresh, simple charts (in-memory ring buffer).
- Suffixes/backends: list, creation (backend + mapping tree + root entry), deletion, read-only, mapping tree state, referrals, caches.
- Indexes: list, create/modify/delete, reindex task; default indexes; VLV indexes.
- Backup/restore (online tasks), list of backups from `nsslapd-bakdir`, download/delete (modes A/C).
- LDIF export/import via tasks; upload/download of files in `nsslapd-ldifdir` (A/C), files owned by uid `dirsrv`.
- Restart: Lifecycle provider (docker-socket-proxy for C/B); "restart required" banner (registry of attributes that require a restart).
- Logs: list of files from `cn=config`, viewing from the end, tail via SSE, filter by substring/conn/op; rotated files.
- "Server" page: version, uptime, ports, paths, state.

**Done when:** through the UI you can create a suffix, configure indexes, make a backup and restore from it, export/import LDIF, watch logs in real time, restart the server; everything is covered by integration tests.

## Stage 3. Declarative settings (2–3 wk) → v0.2 — ✅ done

**Goal:** cover the "long tail" of similar forms with data rather than code.

**Outcome:** 13 settings pages and 6 plug-in pages are YAML (`internal/pages/defs`); a page needs YAML + texts and no code, and tests check every attribute against a real server and every text in en/ru. Deviations:
- Field types are string, dn, int, bool, enum (also read from the server), list, size, duration, flags (bit masks); no `minVersion` condition yet (only `dbImpl`), no hints from `cn=schema` (the texts are written by hand).
- SASL mappings and the root DN password are hand-written screens in the same menu (a list and a password form do not fit one-entry pages).
- Attribute Uniqueness: only the default instance; more instances in stage 4. USN is on/off only. BDB settings page: none (LMDB is the default and BDB is deprecated).
- Restart hints are tracked per setting and cleared when a setting is changed back.
- 389-ds 3.1.2 once crashed while building a VLV index on a long-running development server and lost that backend's data; it could not be reproduced (fresh server, the same data from a backup, deleting the VLV index during the build). ds-ui refuses to delete a VLV index while an index task runs and advises a backup before building one.

Work items:
- Page engine `internal/pages`: YAML descriptions (DN, fields, types, `restartRequired`, `minVersion`/`dbImpl` conditions; labels and hints live in the locale files `pages.*`, ADR-009), `GET/PATCH /api/v1/pages/{id}`, diff → modify; field types: bool(on/off), int, size, duration, enum, string, dn, attr-name, multi.
- Frontend widget generation from the description; type hints from `cn=schema` for `nsslapd-*`.
- Pages: general server settings, limits and performance, DB settings (LMDB/BDB), logs (levels, rotation, deletion) for access/error/audit/auditfail/security, SASL (mechanisms + mappings), LDAPI, root DN (password change), global password policy.
- Simple plugins (on/off + attributes): Referential Integrity, Attribute Uniqueness, USN, Retro Changelog, Account Policy, MemberOf (without fixup), Root DN Access, etc.
- Common "Plugins" page: list, status, enable/disable, `nsslapd-dynamic-plugins`.

**Done when:** the general server, database, logging, SASL, LDAPI and password-policy settings listed above are editable via YAML pages; adding a new page requires no Go code.

## Stage 4. Complex screens (3–4 wk) → v0.3 — ✅ done

**Outcome:** every item below works on 389-ds 3.1 and has integration tests. Deviations:
- *Schema reload* is not offered: changes over LDAP do not need it, and it needs the Schema Reload plug-in (off by default); it matters only after editing schema files on disk.
- *Plug-ins with several configuration entries* are YAML "collections" (no per-plug-in code); the Managed Entries template is edited in the LDAP browser; DNA range transfer between replicas comes with replication (stage 5).
- *Accounts* and *ACI* live in the LDAP browser (cards on the entry), password reset already existed there; Account Policy inactivity is shown as the last login, not computed against the limit.
- *Role members* are found from nsRoleDN or the role filter in the role's subtree: filtering on the computed nsRole scans every entry (seconds on a large suffix).

Work items:
- Schema: viewing (search, dependencies), creating/deleting custom attributeTypes/objectClasses, schema reload task.
- Local (fine-grained) password policies: per subtree/user (`nsPwPolicyContainer` + CoS), list, editing.
- Accounts: status (active/locked/expired — taking roles and Account Policy into account), lock/unlock, password reset.
- Plugins with complex configuration: MemberOf (+ fixup task), DNA (ranges), Auto Membership (definitions + regex rules + rebuild), Managed Entries (templates + definitions), Linked Attributes (+ fixup), PAM Pass-Through.
- Roles and CoS: viewing and creation (managed/filtered/nested roles, CoS templates).
- ACI: list of ACIs per subtree, view/edit as text with highlighting and syntax checking; permission checks via Get Effective Rights.

**Done when:** all listed features work on 389-ds 3.x and are covered by integration tests.

## Stage 5. Replication (3–4 wk) → v0.4 — ✅ done

**Outcome:** the integration suite (run in CI) starts two DS containers on one network and sets up multi-supplier replication only through the ds-ui API: replication manager, two suppliers, changelog, agreements both ways, initialization, changes in both directions, status, RUV, fractional replication, a real conflict resolved by keeping the entry under a new RDN (the change replicates), disabling. The UI was checked against a supplier and a temporary consumer. Deviations:
- The e2e check is the Go integration test against the API, not a browser test (Playwright arrives in stage 7).
- The RUV is read from the replica entry: the RUV tombstone under the suffix lags by up to ~30 s.
- TLS transports and SASL/GSSAPI bind methods can be configured but are not tested yet (certificates come in stage 6).

Work items:
- Enabling/disabling replication for a suffix: role (supplier/hub/consumer), replica ID, bind DN / bind group.
- Changelog: settings (max age/entries, encryption), per-backend (`cn=changelog,cn=<be>`).
- Replication manager: account creation.
- Agreements: CRUD, schedule, fractional replication, initialization (`nsds5BeginReplicaRefresh`), enable/disable, status.
- Monitoring on the local node: agreement statuses, RUV (`nsds50ruv`), CleanAllRUV task.
- Replication conflicts: finding conflict entries, resolution (convert/delete).

**Done when:** in a test setup of two DS instances, multi-supplier is configured through the UI, a replica is initialized, statuses are visible; an e2e test of the setup runs in CI.

## Stage 6. TLS and certificates (1.5–2 wk) → v0.5 — ✅ done

**Outcome:** in the integration suite a fresh server gets a new certificate only through the API — a key and CSR made in its NSS database, signed by a test CA, imported, chosen as the server certificate, TLS 1.3 required — and after a restart the LDAPS handshake shows exactly that certificate and version. The UI shows the presented certificate, the TLS settings and the certificate database. Deviations:
- The NSS tools run in the sidecar image (`certutil`, `pk12util`, `openssl`); in remote mode only the LDAP settings and the handshake check are available.
- A CSR leaves its key in the database until the issued certificate arrives; keys without a certificate are not listed or removed yet.

Work items:
- Settings: `nsslapd-security`, LDAPS port, `cn=encryption,cn=config` (TLS versions, ciphers), server certificate selection.
- NSS DB (Certs provider): list of server and CA certificates, details view, PEM/PKCS#12 import, CSR, deletion, CA trust flags. Implementation: `certutil`/`pk12util` (C) or `dsconf ... -j` (A).
- Verifying the certificate actually served, via a TLS handshake on LDAPS.

**Done when:** the server certificate can be replaced and LDAPS enabled entirely through the UI.

## Stage 7. Stabilization and 1.0 release (2–3 wk)

Work items:
- Multi-server: registry of remote servers (mode B), context switching in the UI; replication lag monitoring across the topology (comparing RUVs between nodes).
- ✅ Health check: a set of checks over LDAP and the NSS database, "Health" page.
- ~~Mode A (all-in-one)~~ — dropped (ADR-012): the DS image is built in the separate 389-ds-base-docker repository, ds-ui runs next to it (modes C and B).
- ✅ e2e tests (Playwright) for key scenarios, in CI; they found a race in the entry view (a slower answer for the previous entry overwrote the new one) and unlabelled inputs.
- User documentation, CHANGELOG, image versioning, publishing to a registry.
- ✅ Server certificate from files of an external ACME client (certbot, lego, cert-manager), ADR-013.
- ✅ Sign-in (moved ahead of stage 7, ADR-011): directory accounts, administrator/operator roles from groups, directory data with Proxied Authorization.

**Done when:** all features from [features.md](features.md) with stage ≤ 7 are implemented, e2e tests are green, release images exist.

## After 1.0

- Built-in ACME client (DNS-01 via RFC 2136 or DNS provider APIs, HTTP-01 for internal CAs), if following files is not enough (ADR-013).
- Sign-in through a reverse proxy (trusted header) or OIDC, as alternatives to directory accounts.
- Visual ACI builder.
- Access log statistics (similar to `logconv`).
- Chaining, Winsync (on request).

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `cn=config` differences between 389 versions (BDB→LMDB, changelog, etc.) | High | Medium | Detect version and `nsslapd-backend-implement`; conditions in YAML pages; CI on two versions |
| No ready-made library (an equivalent of lib389) | Certain | High (volume) | 389 documentation and experiments on a live server → `ds-reference.md`; declarative forms; integration tests against a real DS |
| Replication complexity (roles, RUV, conflicts) | Medium | High | Separate stage, setup of 2–3 DS instances in CI; topology lag moved to stage 7 |
| Debian build of 389 differs in paths / availability of `dscontainer` | Medium | Low | Check in stage 0, custom entrypoint |
| UI reachable from outside | Medium | Critical | Sign-in on by default; with `DSUI_AUTH=none` publish only on 127.0.0.1, explicit warning in the UI and docs |
| Dependency licenses (as with PrimeVue 5) | Medium | Medium | Check dependency licenses before adding and in CI (ADR-004); no third-party code is borrowed (ADR-008) |

## Stage 0 open questions — answers

| # | Question | Answer (verified 2026-11-09) |
|---|---|---|
| 1 | 389-ds-base version in Debian trixie, DB backend | **3.1.2** (`3.1.2+dfsg1-1+deb13u1`), **LMDB** by default (`nsslapd-backend-implement: mdb`) |
| 2 | Is `dscontainer` available | Yes, `/usr/libexec/dirsrv/dscontainer`. Used as is; it does **not** create the suffix — our `deploy/ds/entrypoint.sh` does that (`DS_SUFFIX`) |
| 3 | LDAPI socket path | `/data/run/slapd-localhost.socket` (mode `srw-rw-rw-`), set by `dscontainer` |
| 4 | Autobind from another container | Works via the socket on a shared volume. uid 0 **and uid 389 (`dirsrv`, `nsslapd-localuser`)** → `cn=Directory Manager`; other uids are rejected |
| 5 | `dirsrv` uid/gid | Assigned dynamically by the package (was 100) — we pin **389:389** in both images |
| 6 | Project license | **MIT** (`LICENSE`); no code is borrowed (ADR-008) |

## Open questions

None.
