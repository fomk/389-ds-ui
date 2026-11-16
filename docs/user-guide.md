# User guide

This guide is for administrators who use 389 DS Admin (ds-ui) to run a 389 Directory Server. It walks through the interface section by section. Installation and configuration are in [deployment.md](deployment.md); the full list of features per deployment mode is in [features.md](features.md).

Contents:

1. [Signing in](#signing-in)
2. [Around the interface](#around-the-interface)
3. [Overview, Monitoring and Health](#overview-monitoring-and-health)
4. [LDAP browser](#ldap-browser)
5. [Databases](#databases)
6. [Logs](#logs)
7. [Server settings](#server-settings)
8. [Plugins](#plugins)
9. [Schema](#schema)
10. [Password policies and accounts](#password-policies-and-accounts)
11. [Roles and Class of Service](#roles-and-class-of-service)
12. [Replication](#replication)
13. [TLS and certificates](#tls-and-certificates)
14. [Troubleshooting](#troubleshooting)

## Signing in

Sign in with a directory account:

- **Directory Manager** — type `Directory Manager` (or its full DN) and its password. It is always an administrator.
- **Other users** — type a `uid` or a full DN. They can sign in only if the server operator put them in one of the ds-ui groups (`DSUI_ADMIN_GROUPS`, `DSUI_OPERATOR_GROUPS`, see [deployment.md](deployment.md#sign-in)).

The password is checked by the directory server itself, so its password policy applies: an expired password or a lockout after failed logins also blocks ds-ui. After five failed attempts for a user name from the same address, ds-ui itself refuses it for five minutes.

**Roles.**

| | Administrator | Operator |
|---|---|---|
| Server settings, plug-ins, schema, replication, TLS, backups | change | view only |
| Directory data (LDAP browser, search, LDIF import and export) | as the account's ACIs allow | as the account's ACIs allow |

Users other than Directory Manager work with directory data **as themselves**: the server applies their access control instructions (ACIs). If an operator cannot save an entry, they need an ACI that allows it — not a different role in ds-ui.

A session ends after 30 minutes without activity and after 12 hours at the latest (the operator can change both). Restarting ds-ui signs everyone out. Sign out from the user menu at the top right.

## Around the interface

- **Sidebar** — the sections of this guide. It can be collapsed with the arrow on its edge.
- **Header** — instance name and server version on the left; on the right:
  - **Restart server** (administrators; needs the Docker API proxy of the standard deployment);
  - the **task list** icon — running and recent tasks (backups, imports, reindexing, imports of certificates, …) with their logs;
  - the **language** switch (English, Russian; the choice is remembered in the browser);
  - the **user menu** with your DN, role and *Sign out*;
  - the **server status**: *running*, *restarting*, *stopped*, *unreachable*.
- **"Restart required" banner** — some settings only take effect after a restart (ports, TLS, some plug-in settings). ds-ui lists them in a banner until the server is restarted. Changing a setting back to its running value removes it from the list.

Most pages keep their state in the address (selected entry, suffix, plug-in, settings page), so links can be bookmarked and shared.

## Overview, Monitoring and Health

**Overview** shows the server version, the instance, the database implementation (LMDB or BDB), the suffixes, how ds-ui is deployed and what the server supports (controls, extensions, SASL mechanisms).

**Monitoring** shows uptime, connections, threads, operations, entries and data sent, with charts of the last hour (ds-ui keeps them in memory). Below are the database caches per suffix — entry cache, DN cache and the server-wide normalized DN cache — with hit ratios, and the list of open connections. A warning appears when a full cache serves too few lookups: the cache is probably too small.

**Health** runs a set of read-only checks and lists what it finds, most serious first: errors, warnings, notes. Each finding explains the problem and how to fix it; **Open** goes to the page where it is fixed. The checks cover:

- binds and password storage (anonymous access, unauthenticated binds, plain-text binds while TLS is on, weak or fast password hashes in the global and local policies and for Directory Manager);
- TLS (off, old protocol versions, weak ciphers) and certificates (server and trusted CA certificates expired or expiring within 30 days, a self-signed server certificate, a server certificate missing from the database);
- disks (85 % and 95 % full, disk monitoring off);
- suffixes (no root entry, read-only, not served from their database) and entry caches that are too small;
- replication (failing or disabled agreements, failed initialisation, a changelog without limits, conflict entries);
- plug-ins (attributes the Referential Integrity and MemberOf plug-ins search by without an equality index; Retro Changelog without a maximum age);
- logs (access log off, debug logging on).

Certificate checks need the NSS database, so they are skipped when ds-ui connects to a remote server.

## LDAP browser

**The tree.** Branches load when opened. Large branches are listed page by page: up to 2000 children sorted by name, beyond that in server order with *Load more* — use search to find a particular entry there. *System trees* also shows `cn=config`, `cn=schema` and `cn=monitor`.

**An entry** shows its object classes and attributes. *Operational attributes* adds the server-maintained ones (`createTimestamp`, `modifiersName`, `nsUniqueId`, …). *LDIF* shows the entry as LDIF and lets you edit it as text. Binary values (photos, certificates) can be downloaded and uploaded. Below the attributes:

- **Members** and **Member of** for groups and their members (`member`, `uniqueMember`; "member of" works without the MemberOf plug-in);
- **Account** for entries with a password: locked, locked by a role, locked out after failed logins, password expired or to be changed, the effective password policy, last login; *Lock* and *Unlock* (a lock that comes from a role or Class of Service is changed there);
- **Access control (ACI)** of the entry, with syntax highlighting and checking before the server sees it, the ACIs inherited from parent entries, and **Effective rights**: what a given bind DN may do with this entry and each attribute, as the server evaluates it.

**Actions:**

- **Edit** — a form built from the schema: required and allowed attributes, single-valued ones, object classes to add or remove. *Save* is refused with a clear message if someone else changed the entry meanwhile.
- **New entry** — from a template (user, POSIX user, group, POSIX group, organizational unit) or as a custom entry with object classes of your choice.
- **More actions** — *Rename / move*, *Set password* (typed, or generated by the server when the connection is secure), *Delete* (with all entries below it, if you tick the option).

**Search** (magnifier above the tree) takes a name, uid or e-mail address, or an LDAP filter. *Advanced* sets the base, scope, attributes and limit. Results export as LDIF or CSV. **Import LDIF** (the upload icon) applies an LDIF file or fragment — added entries and change records — as you; for whole suffixes use the import task under Databases.

## Databases

**Suffixes.** The list shows each suffix and its backend; sub-suffixes appear indented under their parent. **+** creates a suffix with an optional root entry; choosing a *parent suffix* makes a sub-suffix, whose entries appear under the parent in the tree but live in a database of their own. A suffix with sub-suffixes cannot be deleted until they are.

Tabs of a suffix:

- **Settings** — state (active, disabled, referral), read-only, require indexes, entry and DN cache sizes; *Delete suffix* asks you to type its backend name.
- **Indexes** — add, change and delete attribute indexes (equality, presence, substring, approximate, matching rules); a new or changed index is empty until you **Reindex**. System indexes cannot be deleted. Reindexing runs as a task.
- **Export / import** — export the suffix to an LDIF file on the server (optionally with replication state, to initialise a replica, or with encrypted attributes in clear text), or import a file, which **replaces all entries** of the suffix.
- **VLV indexes** — browsing indexes for clients that use virtual list views; created indexes are built at once. Building works on the live database: back up large suffixes first.
- **Encryption** — attributes stored encrypted (AES) in the database files, backups and changelog; clients still read them in clear text. Read this before using it:
  - Values written **before** encryption was turned on stay unencrypted until the suffix is exported and imported again.
  - The encryption key is protected by the key of the **server certificate**. A certificate renewed with a new key makes the server fail to start, so while attributes are encrypted ds-ui refuses to switch to a certificate with another key. Renew certificates with the same key (`certbot --reuse-key`, cert-manager `rotationPolicy: Never`).
  - **Turning encryption off** for an attribute that has values would leave those values unreadable after the next restart. ds-ui refuses unless you confirm; the safe order is: export the suffix with *Write encrypted attributes in clear text*, turn encryption off, import that export.

**Backups** creates online backups of all databases and restores them (restoring replaces **all** databases with the backup and takes them offline meanwhile). Backups can be downloaded and deleted. **LDIF files** lists the files in the server's LDIF directory, for upload, download and deletion. **Default indexes** are the indexes new suffixes get.

Listing, downloading and deleting backup and LDIF files need access to the server's files; when ds-ui connects to a remote server, tasks still run but files are not listed.

## Logs

Access, error, audit, audit-failure and security logs: pick the file (current or rotated), the number of lines, and a filter of words that must all occur (for example `conn=12`, `err=32`, `BIND`). **Follow** shows new lines as they are written. Clicking a line of the access log shows every line of that connection. The access log is buffered by default, so new lines arrive in batches.

Log settings (levels, rotation, retention) are under Server settings.

## Server settings

Settings are grouped into pages — *General*, *Limits and performance*, *Proxy protocol*, *Disk monitoring*, *Global database settings*, *LMDB*, each log, *Binds and encryption*, *SASL mappings*, *LDAPI*, *Directory Manager password*. On every page:

- a field shows the server default when it is not set, and a mark when it is *changed, not saved*;
- **restart** marks settings that take effect after a restart;
- the **warning triangle** marks settings that can cut ds-ui off from the server (LDAP port, listen address, LDAPI); saving them asks for confirmation;
- **Attribute names** (top right) shows the `cn=config` attribute behind every field;
- *Save* and *Reset* act on the whole page; leaving a page with unsaved changes asks first.

**SASL mappings** turns SASL identities into directory entries (regular expression, base and filter templates, priority). **Directory Manager password** changes the root password.

## Plugins

The list shows every plug-in with its type and an on/off switch; *Configurable* narrows it to the ones ds-ui can configure. Plug-ins the server needs cannot be disabled. When *dynamic plug-ins* are off (the default), enabling, disabling and most configuration changes take effect after a restart — the banner at the top says so and links to the setting.

Plug-ins with settings: Referential Integrity, MemberOf, Attribute Uniqueness, Root DN Access Control, Retro Changelog, Account Policy, PAM Pass-Through, LDAP Pass-Through Authentication, USN, Linked Attributes, DNA (number ranges), Auto Membership (definitions and rules), Managed Entries. Some offer tasks: rebuild memberOf, rebuild auto memberships, fix up linked attributes, clean up USN tombstones, reload the schema. A task of a disabled plug-in reports that the plug-in is not running.

- **Attribute Uniqueness** can have several rule sets: *Add rule set* creates another instance with its own attributes and subtrees (disabled until you switch it on). Added rule sets can be deleted when disabled; the server's own instance stays.
- **LDAP Pass-Through Authentication** — *Remote servers* lists the subtrees whose binds are checked by other LDAP servers (hosts tried in order, ldap/ldaps/StartTLS, connection limits). The subtree should not be stored on this server.

## Schema

Attribute types and object classes with search, their definitions, dependencies (which classes use an attribute) and inherited attributes. You can create, change and delete **user-defined** definitions (those stored in `99user.ldif`); definitions in use cannot be deleted. **Reload schema** reads the schema files again after they were edited on disk.

## Password policies and accounts

- **Global policy** applies to everyone without a local policy: storage scheme (for users and for Directory Manager), changes and history, expiration, syntax rules, lockout.
- **Local policies** apply to a subtree (through Class of Service) or to one user entry. Settings left unset take the server defaults. Local policies only work while they are allowed in the global policy; ds-ui offers to turn that on.
- **Account report** lists the state of every account under a subtree (up to 5000): active, locked, locked by a role, locked out, password expired, must change the password, with password expiry, last login and failed logins. The counters filter the list; names open the entry in the LDAP browser, where it can be unlocked.

## Roles and Class of Service

**Roles** — managed (members listed in the role), filtered (members match a filter) and nested (members of other roles); create, change, delete, and see and change the members. **Class of Service** — pointer, classic and indirect definitions and their templates, which give entries computed attribute values.

## Replication

Pick a suffix. If it is not replicated, choose the role of this server — **supplier** (accepts changes and sends them on), **hub** or **consumer** — the replica ID (unique among suppliers) and the account other suppliers bind as; ds-ui can create that account. Tabs:

- **Agreements** — where this server sends changes: host, port, transport (LDAP, StartTLS, LDAPS), bind method and credentials, a schedule, attributes not sent (fractional replication). Each agreement shows its last update and initialisation status. **Initialise** copies the whole suffix to the consumer.
- **Replica** — role, bind accounts, how long deleted entries are kept.
- **Changelog** — maximum age and number of entries; set at least one, or the changelog grows without limit.
- **RUV** — the replica update vector: the latest change this server has seen from each replica. **CleanAllRUV** removes a replica that no longer exists from all servers.
- **Topology** — the state of the whole topology, like Cockpit's synchronization report. Enter a bind DN and password that work on the other servers (Directory Manager, or an account that can read `cn=config`) and **Build report**: ds-ui follows this server's agreements to the other servers, and theirs further on (up to 20 servers), reaching them by the names and transports in the agreements. For every server it shows its role and replica ID and, per supplier, whether it is in sync or how far behind it is (to the second); then the agreements between them and their state. Servers that refuse the credentials get their own fields — enter theirs and build again. The credentials go with that request only and are not stored. Certificates of the other servers are verified (against the system CAs and `DSUI_LDAP_CA_FILE`) unless you tick *Do not verify*; agreements over plain LDAP send the password unencrypted, and the report says so.
- **Conflicts** — entries both servers created or changed at the same time. Delete a conflict entry, or keep it under a new name.

## TLS and certificates

- **Presented on LDAPS** — the certificate the server actually presents, read with a TLS handshake: subject, issuer, expiry, TLS version and cipher, and whether a newly chosen certificate is still waiting for a restart.
- **Certificate from files** — shown when ds-ui follows certificate files of certbot, lego or cert-manager (`DSUI_CERT_FILE`, see [deployment.md](deployment.md#certificate-from-files)): the files, the schedule, the certificate in the files, the last check and import, any error, and **Check now**. A new certificate is imported and made the server certificate automatically; it is presented after the next restart, which ds-ui can do itself if configured to.
- **LDAPS**, **Protocol**, **Encryption module** — TLS on or off, port and listen address, certificate validation at startup, host name checks, minimum and maximum TLS version, client certificates, ciphers, renegotiation, the PKCS#11 token. All take effect after a restart.
- **Certificate database** — the server's NSS database. **Add** imports a CA certificate, a certificate with its private key (PEM or PKCS#12), or creates a key and a certificate request (CSR) for your CA and later adds the certificate it issued. Per certificate: details and PEM, trust flags, *Use as server certificate*, delete. The server certificate cannot be deleted while in use. Server backups contain the private keys of this database.

Certificates can only be managed when ds-ui runs next to the server (the standard deployment).

## Troubleshooting

| What you see | What it means |
|---|---|
| "View only: your role cannot change these settings." | You are an operator. Ask an administrator, or to be added to the administrators' group. |
| Saving an entry fails with an access error | Your account's ACIs do not allow the change; ds-ui acts as you. Check *Effective rights* on the entry. |
| A setting is saved but nothing changes | It needs a restart — see the "restart required" banner. |
| "The password is right, but this account has no access to ds-ui" | The account is in neither ds-ui group. |
| "Too many failed attempts" | Wait five minutes. |
| A certificate switch is refused with "attribute encryption needs the current server key" | Attributes are encrypted; renew the certificate with the same key (see [Databases](#databases)). |
| A plug-in task says the plug-in is not running | Enable the plug-in; without dynamic plug-ins, restart the server. |
| Logs, backups or certificates say they need access to the server's files | ds-ui connects to a remote server; these features need the standard deployment next to the server. |
