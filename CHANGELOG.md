# Changelog

All notable changes to 389 DS Admin (ds-ui) are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

The first release, 1.0.0, is being prepared. It is tested against 389 Directory Server 3.3.1 (`ghcr.io/fomk/389-ds-base-docker:3.3.1`).

### Added

#### Deployment and access

- One Go binary with the web interface embedded; a container image to run next to the 389-ds container with a shared `/data` volume (sidecar), and a small image that connects to a remote server over LDAP.
- Server restart from the interface through a Docker API proxy limited to the directory server container.
- Sign-in with directory accounts: Directory Manager, or members of the administrator and operator groups. Operators can view the server configuration but not change it.
- Directory data is read and written as the signed-in user (Proxied Authorization), so the server's access control applies; the task history and the ds-ui log name the user.
- Protection against DNS rebinding and cross-site requests; sessions in HttpOnly, SameSite=Strict cookies; a limit on failed sign-in attempts.
- English and Russian interface.

#### LDAP browser

- Directory tree with large branches paged on the server.
- Entry view with operational attributes, LDIF view and editing, binary values.
- Schema-driven editing, creation from templates, rename and move, deletion of subtrees, password changes.
- Group members and "member of" without the MemberOf plug-in.
- Account state: locks, role and CoS locks, lockouts, password expiry, the effective password policy, last login; lock and unlock.
- Access control instructions: view and edit with syntax checks, inherited ACIs, effective rights of any bind DN.
- Search by name or LDAP filter with LDIF and CSV export; LDIF import of entries and change records.

#### Server administration

- Monitoring: server, operations, connections, caches and disk space, with one hour of history.
- Health check: binds and password storage, TLS settings and certificate expiry, disk space, suffixes and caches, replication agreements, changelog limits and conflicts, missing indexes for plug-ins, logging; each finding links to where it is fixed.
- Databases:
  - suffixes and sub-suffixes, their settings and referrals;
  - indexes, VLV indexes and default indexes, with reindexing;
  - attribute encryption, guarded so that existing data stays readable;
  - export and import tasks (optionally with replication state or decrypted attributes);
  - online backup and restore; download and deletion of backups; upload, download and deletion of LDIF files.
- Logs: access, error, audit, audit-failure and security logs, with filters, connection tracing and live follow.
- Server settings on declarative pages: general, limits and performance, proxy protocol, disk monitoring, database (LMDB and BDB), logs, binds and encryption, SASL mappings, LDAPI, Directory Manager password. Settings that need a restart or could cut off ds-ui are marked.
- Plug-ins: enable and disable; settings of:
  - Referential Integrity, MemberOf, Account Policy, Retro Changelog and Root DN Access Control;
  - Attribute Uniqueness, with several rule sets;
  - PAM and LDAP Pass-Through Authentication;
  - USN, Linked Attributes, DNA, Auto Membership and Managed Entries.

  Also plug-in tasks: memberOf and membership rebuilds, link fixup, USN tombstone cleanup and schema reload.
- Schema: browse; create, change and delete user-defined attribute types and object classes.
- Password policies: global and local (subtree or user); account report for a subtree.
- Roles (managed, filtered, nested) and Class of Service (pointer, classic, indirect).
- Replication:
  - roles, replica IDs and the replication manager account;
  - agreements with schedules and fractional replication;
  - initialisation, status, RUV and CleanAllRUV;
  - changelog limits;
  - conflict resolution.
- TLS and certificates:
  - TLS settings and the certificate the server actually presents;
  - the NSS certificate database: import of CA certificates and key pairs (PEM, PKCS#12), certificate requests, trust flags, choice of the server certificate.
- Server certificate from files kept up to date by certbot, lego or cert-manager: renewed certificates are imported automatically and, optionally, applied by a restart.
- Task history of server tasks and immediate operations, with logs and the user who started them.

### Known limitations

- Several servers in one interface, replication lag across a topology, Chaining (database links) and Windows synchronisation are not available yet.
- Without access to the server's files (remote mode), logs, backup and LDIF file listings and certificates are not available.
- Sessions are kept in memory: restarting ds-ui signs everyone out.
- 389-ds records ds-ui's own account in `modifiersName`; the user who made a change is in ds-ui's request log and task history.

