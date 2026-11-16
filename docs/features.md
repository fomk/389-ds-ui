# Feature matrix

Modes: **B** — remote (LDAP only), **C** — sidecar. There is no all-in-one mode (ADR-012).
✅ available, 🔄 partially / via a workaround, ❌ unavailable.

Mechanism: **LDAP** — entry operations; **Task** — a task in `cn=tasks,cn=config`; **File** — files on `/data`; **CLI** — an external utility; **Docker** — Docker API via proxy.

## LDAP editor (stage 1)

| Feature | Mechanism | B | C |
|---|---|---|---|
| Lazy-loading tree, pagination (sorted up to 2000 children, server order above) | LDAP (one-level, Paged Results, `numSubordinates`) | ✅ | ✅ |
| Entry view, operational attributes | LDAP | ✅ | ✅ |
| Schema-driven forms, objectClass add/remove | LDAP (`cn=schema`) | ✅ | ✅ |
| Creation from templates | LDAP | ✅ | ✅ |
| Rename / move | LDAP (ModifyDN) | ✅ | ✅ |
| Deletion (including subtrees) | LDAP | ✅ | ✅ |
| Password change (server-generated over LDAPI/TLS only) | LDAP (Password Modify extop; modify fallback on plain LDAP) | ✅ | ✅ |
| Binary attributes (photos, certificates) | LDAP | ✅ | ✅ |
| Search, LDIF/CSV export of results | LDAP | ✅ | ✅ |
| LDIF fragment import | LDAP (add/modify) | ✅ | ✅ |
| Group members (member/uniqueMember), "member of" (by search, no plugin needed) | LDAP | ✅ | ✅ |
| Entry LDIF view/edit, subtree export | LDAP | ✅ | ✅ |

## Server and monitoring (stages 2–3)

| Feature | Mechanism | Stage | B | C |
|---|---|---|---|---|
| Version, suffixes, capabilities | LDAP (root DSE) | 0 | ✅ | ✅ |
| Server, connection, operation monitoring | LDAP (`cn=monitor`, `cn=snmp,cn=monitor`) | 2 | ✅ | ✅ |
| DB and backend monitoring, caches | LDAP (`cn=monitor,cn=ldbm database…`) | 2 | ✅ | ✅ |
| Disk space | LDAP (`cn=disk space,cn=monitor`) | 2 | ✅ | ✅ |
| Restart / stop / start | Docker | 2 | 🔄 (if a proxy is available) | ✅ restart |
| "Restart required" banner | attribute registry | 2 | ✅ | ✅ |
| General settings, limits, tuning (incl. turbo mode, listen backlog, TCP keep-alive/FIN, outbound I/O timeout, paged size limit, controls per operation, plug-in bind DN tracking, time skew) | LDAP (`cn=config`) | 3 | ✅ | ✅ |
| HAProxy PROXY protocol: trusted proxy addresses | LDAP (`cn=config`) | 7 | ✅ | ✅ |
| DB settings (LMDB/BDB) | LDAP | 3 | ✅ | ✅ |
| Root DN, DM password change | LDAP (`nsslapd-rootpw`) | 3 | ✅ | ✅ |
| SASL: mechanisms, mappings | LDAP (`cn=sasl,cn=config`) | 3 | ✅ | ✅ |
| LDAPI settings | LDAP | 3 | ✅ | ✅ |
| Health check (bind and password-storage settings, TLS, server/CA certificate expiry, disk space and monitoring, suffixes, entry caches, replication agreements/changelog/conflicts, indexes plug-ins need, retro changelog, debug logging) — done | LDAP (+ NSS database in C) | 7 | ✅ (no certificate expiry) | ✅ |

## Logs (stages 2–3)

| Feature | Mechanism | Stage | B | C |
|---|---|---|---|---|
| Log settings (levels, rotation, deletion) | LDAP | 3 | ✅ | ✅ |
| Viewing log files | File | 2 | ❌ | ✅ |
| Real-time tail, filters (conn=N/op=N tokens, click-to-filter by connection) | File + SSE | 2 | ❌ | ✅ |
| Change history (Retro Changelog) | LDAP (`cn=changelog`) | 4 | ✅ | ✅ |

## Databases (stage 2)

| Feature | Mechanism | B | C |
|---|---|---|---|
| Suffixes/backends: CRUD, read-only, referrals | LDAP | ✅ | ✅ |
| Sub-suffixes (`nsslapd-parent-suffix`); a suffix with sub-suffixes cannot be deleted | LDAP | ✅ | ✅ |
| Attribute encryption (AES): list, add, remove (guarded while values exist); decrypted LDIF export; the server certificate keeps its key while it is on | LDAP + Task | ✅ | ✅ |
| Indexes: CRUD, reindex | LDAP + Task | ✅ | ✅ |
| VLV indexes (listing; create/delete in stage 3) | LDAP + Task | ✅ | ✅ |
| Backup / restore (online) | Task | ✅ | ✅ |
| Listing, downloading, deleting backups | File | ❌ | ✅ |
| LDIF export / import (via task) | Task + File | 🔄 run without file access | ✅ |
| LDIF export / import through the UI (no task) | LDAP | ✅ | ✅ |

## Schema, policies, accounts (stages 3–4)

| Feature | Mechanism | Stage | B | C |
|---|---|---|---|---|
| Schema browsing (search, dependencies, inherited attributes) — done | LDAP | 4 | ✅ | ✅ |
| Creating/changing/deleting custom AT/OC — done (deletion only while unused) | LDAP (`cn=schema`) | 4 | ✅ | ✅ |
| Schema reload (after editing schema files on disk; needs the Schema Reload plug-in) — done | Task | 7 | ✅ | ✅ |
| Global password policy — done | LDAP | 3 | ✅ | ✅ |
| Local password policies (subtree via CoS, user via `pwdpolicysubentry`) — done | LDAP (CoS) | 4 | ✅ | ✅ |
| Account status (lock, role/CoS lock, lockout, expiry, must-change, policy, last login), lock/unlock — done (LDAP browser) | LDAP | 4 | ✅ | ✅ |
| Roles (managed/filtered/nested, members) and CoS (pointer/classic/indirect, templates) — done | LDAP | 4 | ✅ | ✅ |
| Account report: state of every account under a subtree (up to 5000), counts per state, links to the LDAP browser — done | LDAP (+ real-attributes-only control) | 7 | ✅ | ✅ |
| ACI: view/edit with highlighting and syntax check, inherited ACIs, effective rights (GER) — done (LDAP browser) | LDAP | 4 | ✅ | ✅ |
| Permission check (Get Effective Rights) | LDAP (control) | 4 | ✅ | ✅ |

## Plugins (stages 3–4)

| Plugin | Stage | Notes |
|---|---|---|
| List, on/off, dynamic plugins | 3 ✅ | required plug-ins (syntaxes, matching rules, ldbm, ACL) cannot be disabled |
| Referential Integrity, Retro Changelog, Account Policy, Root DN Access | 3 ✅ | YAML pages (`internal/pages/defs/plugins.yaml`) |
| Attribute Uniqueness | 3 ✅ / 7 ✅ | any number of instances (rule sets): add, configure with the shared page (bound by `nsslapd-pluginPath`), delete when disabled; the shipped instance stays |
| USN | 7 ✅ | global counter, entryUSN of imported entries, tombstone cleanup task |
| LDAP Pass-Through Authentication | 7 ✅ | remote server sets (plug-in arguments): hosts, subtree, ldap/ldaps/StartTLS, connection options |
| Schema Reload | 7 ✅ | task, also on the Schema page |
| MemberOf | 3 ✅ / 4 ✅ | settings, fixup task |
| DNA | 4 ✅ | ranges (collection `dna-ranges`), shared config DN; range transfer between replicas — stage 5 |
| Auto Membership | 4 ✅ | definitions, nested regex rules, rebuild task |
| Managed Entries | 4 ✅ | definitions; the template entry is edited in the LDAP browser |
| Linked Attributes | 4 ✅ | link pairs, fixup task |
| PAM Pass-Through | 4 ✅ | YAML page |
| Chaining, Winsync | after 1.0 | on request |

All plugins are LDAP-based and available in all modes.

## Replication (stages 5, 7)

| Feature | Mechanism | Stage | B | C |
|---|---|---|---|---|
| Enabling replication, role, replica ID, bind DNs/group — done | LDAP | 5 | ✅ | ✅ |
| Changelog (max age/entries, trim interval) — done | LDAP | 5 | ✅ | ✅ |
| Replication manager — done | LDAP | 5 | ✅ | ✅ |
| Agreements: CRUD, schedule, fractional — done | LDAP | 5 | ✅ | ✅ |
| Replica initialization — done | LDAP | 5 | ✅ | ✅ |
| Agreement status, RUV — done | LDAP | 5 | ✅ | ✅ |
| CleanAllRUV — done | Task | 5 | ✅ | ✅ |
| Replication conflicts (list, delete, keep under a new RDN) — done | LDAP | 5 | ✅ | ✅ |
| Topology report: servers reached through the agreements, lag per supplier (RUV comparison), agreements between them — done | LDAP to the other servers (credentials per report) | 7 | ✅ | ✅ |

## TLS (stage 6)

| Feature | Mechanism | B | C |
|---|---|---|---|
| Enabling LDAPS, TLS versions, client certificates, ciphers — done; LDAPS listen address, certificate validation at startup, host name check, PEM extraction, client renegotiation, session cache, encryption module (token) | LDAP (`cn=config`, `cn=encryption,cn=config`, `cn=RSA,…`) | ✅ | ✅ |
| Server certificate selection — done | LDAP | ✅ | ✅ |
| Certificates: list, import (CA, PKCS#12, PEM pair, issued), CSR, deletion, trust — done | CLI (`certutil`, `pk12util`, `openssl`) | ❌ | ✅ |
| Verifying the served certificate — done | TLS handshake | ✅ | ✅ |
| Server certificate from files of certbot / lego / cert-manager (check, import, switch, optional restart) | File + CLI + LDAP | ❌ | ✅ |

## Out of scope

- Instance creation/deletion — handled by the DS image entrypoint.
- Full `logconv`-level log analysis — after 1.0.
- OS metrics (CPU/RAM) — via Docker/Prometheus.
