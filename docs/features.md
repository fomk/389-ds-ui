# Feature matrix

Modes: **A** — all-in-one, **B** — remote (LDAP only), **C** — sidecar.
✅ available, 🔄 partially / via a workaround, ❌ unavailable.

Mechanism: **LDAP** — entry operations; **Task** — a task in `cn=tasks,cn=config`; **File** — files on `/data`; **CLI** — an external utility; **Docker** — Docker API via proxy; **s6** — supervisor in mode A.

## LDAP editor (stage 1)

| Feature | Mechanism | A | B | C |
|---|---|---|---|---|
| Lazy-loading tree, pagination (sorted up to 2000 children, server order above) | LDAP (one-level, Paged Results, `numSubordinates`) | ✅ | ✅ | ✅ |
| Entry view, operational attributes | LDAP | ✅ | ✅ | ✅ |
| Schema-driven forms, objectClass add/remove | LDAP (`cn=schema`) | ✅ | ✅ | ✅ |
| Creation from templates | LDAP | ✅ | ✅ | ✅ |
| Rename / move | LDAP (ModifyDN) | ✅ | ✅ | ✅ |
| Deletion (including subtrees) | LDAP | ✅ | ✅ | ✅ |
| Password change (server-generated over LDAPI/TLS only) | LDAP (Password Modify extop; modify fallback on plain LDAP) | ✅ | ✅ | ✅ |
| Binary attributes (photos, certificates) | LDAP | ✅ | ✅ | ✅ |
| Search, LDIF/CSV export of results | LDAP | ✅ | ✅ | ✅ |
| LDIF fragment import | LDAP (add/modify) | ✅ | ✅ | ✅ |
| Group members (member/uniqueMember), "member of" (by search, no plugin needed) | LDAP | ✅ | ✅ | ✅ |
| Entry LDIF view/edit, subtree export | LDAP | ✅ | ✅ | ✅ |

## Server and monitoring (stages 2–3)

| Feature | Mechanism | Stage | A | B | C |
|---|---|---|---|---|---|
| Version, suffixes, capabilities | LDAP (root DSE) | 0 | ✅ | ✅ | ✅ |
| Server, connection, operation monitoring | LDAP (`cn=monitor`, `cn=snmp,cn=monitor`) | 2 | ✅ | ✅ | ✅ |
| DB and backend monitoring, caches | LDAP (`cn=monitor,cn=ldbm database…`) | 2 | ✅ | ✅ | ✅ |
| Disk space | LDAP (`cn=disk space,cn=monitor`) | 2 | ✅ | ✅ | ✅ |
| Restart / stop / start | s6 / Docker | 2 | ✅ | 🔄 (if a proxy is available) | ✅ restart |
| "Restart required" banner | attribute registry | 2 | ✅ | ✅ | ✅ |
| General settings, limits, tuning | LDAP (`cn=config`) | 3 | ✅ | ✅ | ✅ |
| DB settings (LMDB/BDB) | LDAP | 3 | ✅ | ✅ | ✅ |
| Root DN, DM password change | LDAP (`nsslapd-rootpw`) | 3 | ✅ | ✅ | ✅ |
| SASL: mechanisms, mappings | LDAP (`cn=sasl,cn=config`) | 3 | ✅ | ✅ | ✅ |
| LDAPI settings | LDAP | 3 | ✅ | ✅ | ✅ |
| Healthcheck | LDAP + File (+ CLI in A) | 7 | ✅ | 🔄 | 🔄 |

## Logs (stages 2–3)

| Feature | Mechanism | Stage | A | B | C |
|---|---|---|---|---|---|
| Log settings (levels, rotation, deletion) | LDAP | 3 | ✅ | ✅ | ✅ |
| Viewing log files | File | 2 | ✅ | ❌ | ✅ |
| Real-time tail, filters (conn=N/op=N tokens, click-to-filter by connection) | File + SSE | 2 | ✅ | ❌ | ✅ |
| Change history (Retro Changelog) | LDAP (`cn=changelog`) | 4 | ✅ | ✅ | ✅ |

## Databases (stage 2)

| Feature | Mechanism | A | B | C |
|---|---|---|---|---|
| Suffixes/backends: CRUD, read-only, referrals | LDAP | ✅ | ✅ | ✅ |
| Indexes: CRUD, reindex | LDAP + Task | ✅ | ✅ | ✅ |
| VLV indexes (listing; create/delete in stage 3) | LDAP + Task | ✅ | ✅ | ✅ |
| Backup / restore (online) | Task | ✅ | ✅ | ✅ |
| Listing, downloading, deleting backups | File | ✅ | ❌ | ✅ |
| LDIF export / import (via task) | Task + File | ✅ | 🔄 run without file access | ✅ |
| LDIF export / import through the UI (no task) | LDAP | ✅ | ✅ | ✅ |
| Offline: dbverify, db2ldif/ldif2db, BDB→LMDB | CLI + s6 | ✅ (after 1.0) | ❌ | ❌ |

## Schema, policies, accounts (stages 3–4)

| Feature | Mechanism | Stage | A | B | C |
|---|---|---|---|---|---|
| Schema browsing (search, dependencies, inherited attributes) — done | LDAP | 4 | ✅ | ✅ | ✅ |
| Creating/changing/deleting custom AT/OC — done (deletion only while unused) | LDAP (`cn=schema`) | 4 | ✅ | ✅ | ✅ |
| Schema reload (after editing schema files on disk; needs the Schema Reload plug-in) | Task | later | ✅ | ✅ | ✅ |
| Global password policy — done | LDAP | 3 | ✅ | ✅ | ✅ |
| Local password policies (subtree via CoS, user via `pwdpolicysubentry`) — done | LDAP (CoS) | 4 | ✅ | ✅ | ✅ |
| Account status (lock, role/CoS lock, lockout, expiry, must-change, policy, last login), lock/unlock — done (LDAP browser) | LDAP | 4 | ✅ | ✅ | ✅ |
| Roles (managed/filtered/nested, members) and CoS (pointer/classic/indirect, templates) — done | LDAP | 4 | ✅ | ✅ | ✅ |
| ACI: view/edit with highlighting and syntax check, inherited ACIs, effective rights (GER) — done (LDAP browser) | LDAP | 4 | ✅ | ✅ | ✅ |
| Permission check (Get Effective Rights) | LDAP (control) | 4 | ✅ | ✅ | ✅ |

## Plugins (stages 3–4)

| Plugin | Stage | Notes |
|---|---|---|
| List, on/off, dynamic plugins | 3 ✅ | required plug-ins (syntaxes, matching rules, ldbm, ACL) cannot be disabled |
| Referential Integrity, Attribute Uniqueness (default instance), Retro Changelog, Account Policy, Root DN Access | 3 ✅ | YAML pages (`internal/pages/defs/plugins.yaml`); USN is on/off only |
| MemberOf | 3 ✅ / 4 ✅ | settings, fixup task |
| DNA | 4 ✅ | ranges (collection `dna-ranges`), shared config DN; range transfer between replicas — stage 5 |
| Auto Membership | 4 ✅ | definitions, nested regex rules, rebuild task |
| Managed Entries | 4 ✅ | definitions; the template entry is edited in the LDAP browser |
| Linked Attributes | 4 ✅ | link pairs, fixup task |
| PAM Pass-Through | 4 ✅ | YAML page |
| Chaining, Winsync | after 1.0 | on request |

All plugins are LDAP-based and available in all modes.

## Replication (stages 5, 7)

| Feature | Mechanism | Stage | A | B | C |
|---|---|---|---|---|---|
| Enabling replication, role, replica ID | LDAP | 5 | ✅ | ✅ | ✅ |
| Changelog | LDAP | 5 | ✅ | ✅ | ✅ |
| Replication manager | LDAP | 5 | ✅ | ✅ | ✅ |
| Agreements: CRUD, schedule, fractional | LDAP | 5 | ✅ | ✅ | ✅ |
| Replica initialization | LDAP | 5 | ✅ | ✅ | ✅ |
| Agreement status, RUV | LDAP | 5 | ✅ | ✅ | ✅ |
| CleanAllRUV | Task | 5 | ✅ | ✅ | ✅ |
| Replication conflicts | LDAP | 5 | ✅ | ✅ | ✅ |
| Topology lag (RUV comparison) | LDAP to multiple nodes | 7 | ✅ | ✅ | ✅ |

## TLS (stage 6)

| Feature | Mechanism | A | B | C |
|---|---|---|---|---|
| Enabling LDAPS, TLS versions, ciphers | LDAP (`cn=encryption,cn=config`) | ✅ | ✅ | ✅ |
| Server certificate selection | LDAP | ✅ | ✅ | ✅ |
| Certificates: list, import, CSR, deletion, CA trust | CLI (`dsconf -j` in A, `certutil` in C) | ✅ | ❌ | ✅ |
| Verifying the served certificate | TLS handshake | ✅ | ✅ | ✅ |

## Out of scope

- Instance creation/deletion — handled by the DS image entrypoint.
- Full `logconv`-level log analysis — after 1.0.
- OS metrics (CPU/RAM) — via Docker/Prometheus.
