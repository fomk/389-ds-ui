# 389-ds reference for developers

Cheat sheet: where in DS the things ds-ui works with live. Sources: Red Hat Directory Server / 389 documentation, the `01core389.ldif` schema, and checks against a live server. Code from other projects is not borrowed (ADR-008).

> ⚠️ Compiled from memory and documentation; **every item is confirmed by an integration test** on the target version. Fix discrepancies right here. Items marked "(verify)" are the ones most likely to differ between versions.

Abbreviation: `LDBM = cn=ldbm database,cn=plugins,cn=config`.

**Confirmed on 389-ds 3.1.2 (Debian trixie, stage 0):** root DSE (`vendorVersion: 389-Directory/3.1.2 B2026.125.0709`, 25 controls, 15 extensions, Paged Results present); `nsslapd-backend-implement: mdb` in `cn=config,LDBM`; in the container `nsslapd-ldapiautobind: on`, `nsslapd-ldapimaprootdn: cn=Directory Manager`, `nsslapd-ldapimaptoentries: off`, `nsslapd-localuser: dirsrv` — autobind yields DM for uid 0 and the uid of `nsslapd-localuser`.

## Root DSE (`""`, scope base)

`vendorName`, `vendorVersion`, `namingContexts`, `defaultNamingContext`, `supportedControl`, `supportedExtension`, `supportedSASLMechanisms`, `supportedLDAPVersion`. Request explicitly (`+` or by listing them).

## Server configuration — `cn=config`

| Group | Attributes |
|---|---|
| Ports, network | `nsslapd-port`, `nsslapd-securePort`, `nsslapd-security`, `nsslapd-listenhost` |
| Root DN | `nsslapd-rootdn`, `nsslapd-rootpw` (the server hashes it on modify) |
| LDAPI | `nsslapd-ldapilisten`, `nsslapd-ldapifilepath`, `nsslapd-ldapiautobind`, `nsslapd-ldapimaprootdn`, `nsslapd-ldapimaptoentries` |
| Paths | `nsslapd-ldifdir`, `nsslapd-bakdir`, `nsslapd-schemadir`, `nsslapd-certdir`, `nsslapd-instancedir` (verify) |
| Limits | `nsslapd-sizelimit`, `nsslapd-timelimit`, `nsslapd-idletimeout`, `nsslapd-maxdescriptors`, `nsslapd-threadnumber`, `nsslapd-ioblocktimeout` |
| Plugins | `nsslapd-dynamic-plugins` |
| Password policy (global) | `passwordStorageScheme`, `passwordChange`, `passwordMustChange`, `passwordExp`, `passwordMaxAge`, `passwordMinAge`, `passwordWarning`, `passwordHistory`, `passwordInHistory`, `passwordCheckSyntax`, `passwordMinLength`, `passwordMin*` (character classes), `passwordLockout`, `passwordMaxFailure`, `passwordLockoutDuration`, `passwordResetFailureCount`, `passwordUnlock`, `nsslapd-pwpolicy-local` |
| Logs (for each of access/error/audit/auditfail/security) | `nsslapd-<log>log` (path), `nsslapd-<log>log-logging-enabled`, `nsslapd-<log>log-level` (access/error), `nsslapd-<log>log-maxlogsize`, `-maxlogsperdir`, `-logrotationtime(unit)`, `-logexpirationtime(unit)`, `-logmaxdiskspace`, `-logminfreediskspace`, `nsslapd-accesslog-logbuffering` |

Changes are written to `dse.ldif` automatically. Which attributes require a restart is tracked in our own registry (`internal/ds/restartreg`); the server does not report it.

## Databases

| Object | DN | Key attributes |
|---|---|---|
| Global LDBM settings | `cn=config,LDBM` | `nsslapd-backend-implement` (`bdb`/`mdb`), `nsslapd-lookthroughlimit`, `nsslapd-idlistscanlimit` |
| LMDB | `cn=mdb,cn=config,LDBM` | `nsslapd-mdb-max-size`, `nsslapd-mdb-max-readers`, `nsslapd-mdb-max-dbs` (verify) |
| BDB | `cn=bdb,cn=config,LDBM` | `nsslapd-dbcachesize`, `nsslapd-db-*` |
| Backend | `cn=<be>,LDBM` (oc `nsBackendInstance`) | `nsslapd-suffix`, `nsslapd-readonly`, `nsslapd-require-index`, `nsslapd-cachememsize`, `nsslapd-dncachememsize` |
| Mapping tree | `cn="<suffix>",cn=mapping tree,cn=config` (oc `nsMappingTree`) | `nsslapd-state` (`backend`/`disabled`/`referral`/`referral on update`), `nsslapd-backend`, `nsslapd-referral`, `nsslapd-parent-suffix` |
| Index | `cn=<attr>,cn=index,cn=<be>,LDBM` (oc `nsIndex`) | `nsIndexType` (`eq`,`pres`,`sub`,`approx`), `nsSystemIndex`, `nsMatchingRule` |
| Default indexes | `cn=default indexes,cn=config,LDBM` | — |
| VLV search | `cn=<name>,cn=<be>,LDBM` (oc `vlvSearch`) | `vlvBase`, `vlvScope`, `vlvFilter` |
| VLV index | `cn=<name>,cn=<vlvsearch>,cn=<be>,LDBM` (oc `vlvIndex`) | `vlvSort` |

Creating a suffix: 1) backend, 2) mapping tree, 3) suffix root entry. Deletion goes in reverse order (deleting the backend deletes the data).

## Tasks — `cn=tasks,cn=config`

Task entry: `cn=<unique name>,cn=<type>,cn=tasks,cn=config` with `objectClass: extensibleObject` and `cn`.

| Type (container) | Input attributes |
|---|---|
| `cn=export` | `nsInstance`, `nsFilename`, `nsIncludeSuffix`, `nsExcludeSuffix`, `nsExportReplica`, `nsNoWrap`, `nsUseOneFile` |
| `cn=import` | `nsInstance`, `nsFilename`, `nsIncludeSuffix`, `nsExcludeSuffix` |
| `cn=backup` | `nsArchiveDir`, `nsDatabaseType` |
| `cn=restore` | `nsArchiveDir`, `nsDatabaseType` |
| `cn=index` | `nsInstance`, `nsIndexAttribute` (`attr:type,type`), `nsIndexVLVAttribute` |
| `cn=memberof task` | `basedn`, `filter` |
| `cn=fixup linked attributes` | `linkdn` |
| `cn=automember rebuild membership` | `basedn`, `filter`, `scope` |
| `cn=schema reload task` | `schemadir` |
| `cn=syntax validate` | `basedn`, `filter` |
| `cn=cleanallruv` | `replica-base-dn`, `replica-id`, `replica-force-cleaning` |
| `cn=USN tombstone cleanup task` | `suffix`/`backend`, `maxusn_to_delete` |

Status (in the same entry): `nsTaskStatus` (text), `nsTaskLog`, `nsTaskExitCode` (appears on completion, `0` means success), `nsTaskCurrentItem`, `nsTaskTotalItems`. Completed tasks are removed by the server after a TTL (`ttl`, default ~ a few minutes — verify).

## Monitoring

| DN | Contents |
|---|---|
| `cn=monitor` | `version`, `threads`, `currentconnections`, `totalconnections`, `opsinitiated`, `opscompleted`, `entriessent`, `bytessent`, `starttime`, `currenttime`, `connection` (per connection) |
| `cn=snmp,cn=monitor` | operation counters by type, errors, binds |
| `cn=monitor,LDBM` | DB cache, global statistics |
| `cn=monitor,cn=<be>,LDBM` | entry cache / DN cache (hits, tries, size), index file statistics |
| `cn=disk space,cn=monitor` | `dsDisk` (partition, size, used, available) |
| `cn=counters,cn=monitor` | (verify it exists) |

## Schema — `cn=schema`

Read: `attributeTypes`, `objectClasses`, `matchingRules`, `ldapSyntaxes`, `nsSchemaCSN`. Modify: `modify add/delete` of `attributeTypes`/`objectClasses` values (stored in `99user.ldif`, attribute `X-ORIGIN 'user defined'`). The standard schema cannot be deleted.

## Plugins — `cn=<Plugin Name>,cn=plugins,cn=config`

Common attributes: `nsslapd-pluginEnabled`, `nsslapd-pluginPath`, `nsslapd-pluginInitfunc`, `nsslapd-pluginType`, `nsslapd-pluginId`, `nsslapd-pluginVersion`, `nsslapd-plugin-depends-on-*`, `nsslapd-pluginConfigArea` (shared config).

| Plugin | Configuration |
|---|---|
| MemberOf Plugin | `memberOfAttr`, `memberOfGroupAttr`, `memberOfAllBackends`, `memberOfSkipNested`, `memberOfEntryScope`, `memberOfEntryScopeExcludeSubtree`, `memberOfAutoAddOC` |
| referential integrity postoperation | `referint-membership-attr`, `referint-update-delay`, `nsslapd-pluginEntryScope`, `nsslapd-pluginContainerScope` |
| attribute uniqueness | `uniqueness-attribute-name`, `uniqueness-subtrees`, `uniqueness-across-all-subtrees` |
| Distributed Numeric Assignment Plugin | child entries: `dnaType`, `dnaNextValue`, `dnaMaxValue`, `dnaFilter`, `dnaScope`, `dnaMagicRegen`, `dnaSharedCfgDN` |
| Auto Membership Plugin | child `autoMemberDefinition` entries (`autoMemberScope`, `autoMemberFilter`, `autoMemberGroupingAttr`, `autoMemberDefaultGroup`) + `autoMemberRegexRule` |
| Managed Entries | child definitions `originScope`, `originFilter`, `managedBase`, `managedTemplate`; `mepTemplateEntry` templates |
| Linked Attributes | child `linkType`, `managedType`, `linkScope` |
| Retro Changelog Plugin | `nsslapd-changelogmaxage`, `nsslapd-include-suffix`, `nsslapd-exclude-suffix`; data in `cn=changelog` |
| Account Policy Plugin | `nsslapd-pluginarg0` → config entry (`alwaysrecordlogin`, `stateattrname`, `limitattrname`) |
| RootDN Access Control | `rootdn-open-time`, `rootdn-close-time`, `rootdn-days-allowed`, `rootdn-allow-host/ip`, `rootdn-deny-host/ip` |
| USN | enabling + `nsslapd-entryusn-global` in `cn=config` |

## Password policies and accounts

- Local policy: container `cn=nsPwPolicyContainer,<subtree>`, policy entry `cn=cn\3DnsPwPolicyEntry_subtree\2C<subtree>,cn=nsPwPolicyContainer,…` (`objectClass: passwordpolicy`), plus a CoS template and CoS definition that assign `pwdpolicysubentry`. For a user — `pwdpolicysubentry` directly in the entry. Take the exact naming scheme from lib389 `pwpolicy.py` (verify).
- Locking: `nsAccountLock: true` (for roles — via `nsDisabledRole`/`nsManagedDisabledRole`).
- Status: `passwordRetryCount`, `retryCountResetTime`, `accountUnlockTime`, `passwordExpirationTime`, `nsRoleDN`, `nsRole` (computed), `lastLoginTime` (Account Policy).

## Replication

| Object | DN | Attributes |
|---|---|---|
| Replica | `cn=replica,cn="<suffix>",cn=mapping tree,cn=config` (oc `nsds5Replica`) | `nsDS5ReplicaRoot`, `nsDS5ReplicaId` (supplier 1–65534, consumer/hub 65535), `nsDS5ReplicaType` (3 supplier / 2 hub, consumer), `nsDS5Flags` (1 — writes changelog), `nsDS5ReplicaBindDN`, `nsDS5ReplicaBindDNGroup`, `nsds5ReplicaPurgeDelay` |
| Changelog | `cn=changelog,cn=<be>,LDBM` | `nsslapd-changelogmaxage`, `nsslapd-changelogmaxentries`, `nsslapd-encryptionalgorithm` |
| Agreement | `cn=<name>,cn=replica,cn="<suffix>",cn=mapping tree,cn=config` (oc `nsds5ReplicationAgreement`) | `nsDS5ReplicaHost`, `nsDS5ReplicaPort`, `nsDS5ReplicaTransportInfo` (LDAP/TLS/SSL), `nsDS5ReplicaBindMethod` (SIMPLE/SSLCLIENTAUTH/SASL/GSSAPI), `nsDS5ReplicaBindDN`, `nsDS5ReplicaCredentials`, `nsDS5ReplicaRoot`, `nsDS5ReplicaUpdateSchedule`, `nsDS5ReplicatedAttributeList` (fractional), `nsds5ReplicaEnabled` |
| Initialization | modify of the agreement | `nsds5BeginReplicaRefresh: start` |
| Agreement status | same entry (operational) | `nsds5replicaLastUpdateStatus`, `nsds5replicaLastUpdateStart/End`, `nsds5replicaUpdateInProgress`, `nsds5replicaLastInitStatus`, `nsds5replicaLastInitStart/End`, `nsds5replicaChangesSentSinceStartup` |
| RUV | base search on `<suffix>` with filter `(&(nsuniqueid=ffffffff-ffffffff-ffffffff-ffffffff)(objectclass=nstombstone))` | `nsds50ruv`, `nsruvReplicaLastModified` |
| Conflicts | search `(nsds5ReplConflict=*)` under the suffix | `nsds5ReplConflict` |

## TLS

| Object | DN | Attributes |
|---|---|---|
| Enabling | `cn=config` | `nsslapd-security`, `nsslapd-securePort` |
| TLS parameters | `cn=encryption,cn=config` | `sslVersionMin`, `sslVersionMax`, `nsSSL3Ciphers`, `allowWeakCipher`, `nsSSLClientAuth` |
| Server certificate | `cn=RSA,cn=encryption,cn=config` | `nsSSLPersonalitySSL` (nickname in the NSS DB), `nsSSLToken`, `nsSSLActivation` |
| NSS DB | files in `nsslapd-certdir` | `cert9.db`, `key4.db`, `pkcs11.txt`, `pwdfile.txt`/`pin.txt` |

Replacing the certificate requires a DS restart.

## Useful controls and extensions

| OID | Purpose |
|---|---|
| `1.2.840.113556.1.4.319` | Paged Results |
| `2.16.840.1.113730.3.4.9` | VLV request |
| `1.2.840.113556.1.4.473` | Server Side Sort |
| `1.3.6.1.4.1.42.2.27.9.5.2` | Get Effective Rights |
| `2.16.840.1.113730.3.4.2` | ManageDsaIT |
| `1.2.840.113556.1.4.805` | Tree Delete (verify support in 389) |
| `1.3.6.1.4.1.4203.1.11.1` | Password Modify (extop) |
| `1.3.6.1.4.1.4203.1.11.3` | Who am I (extop) |
