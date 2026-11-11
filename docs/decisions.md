# Architecture Decision Records (ADR)

Format: context → decision → consequences → alternatives considered. Statuses: *Accepted*, *Proposed*, *Superseded*.

---

## ADR-001. Go backend without lib389

**Status:** Accepted

**Context.** `cockpit-389-ds` builds all of its logic on the Python library lib389 (via `dsconf`/`dsctl`). We need a lightweight service in Docker, independent of Cockpit.

**Decision.** A Go backend: `go-ldap/ldap/v3` plus our own 389-ds knowledge layer (`internal/ds`). lib389 is used neither as a dependency nor as a source of code (ADR-008).

**Consequences.**
- (+) A single static binary, a 20–100 MB image, ~20 MB RAM; a typed API; independence from the `python3-lib389` version.
- (+) The LDAP layer and the editor are portable to other LDAP servers (e.g. OpenLDAP).
- (−) We implement the 389 logic ourselves (tasks, plugins, replication, password policies via CoS, RUV) — hence a longer road to parity.
- (−) We have to track `cn=config` changes between 389 versions ourselves.

**Alternatives.**
- Python + FastAPI + lib389 — parity ~1–1.5 months sooner, but tied to lib389's unstable internal API, GPL-3, more RAM, no portability to OpenLDAP.
- A cockpit API shim under the existing cockpit-389-ds frontend — quick, but fragile and awkward to evolve.
- OpenLDAP instead of 389 — rejected: we would lose CoS/Roles, DNA, Managed Entries, tasks, ACIs in entries.

---

## ADR-002. Sidecar deployment (C) with providers; modes A and B as configurations

**Status:** Accepted

**Context.** Some functions are not available over LDAP (logs, LDIF/backup files, NSS DB, restart, offline operations). Options: everything in one container (A), LDAP only (B), two containers with a shared volume (C).

**Decision.** The primary mode is **C**: `ds` and `ui` containers with a shared volume `/data` (LDAPI socket, logs, ldif, bak, NSS DB) and docker-socket-proxy for restart. Environment-dependent capabilities are factored out into providers; modes A and B are different provider sets of the same binary.

**Consequences.**
- (+) ~95% of cockpit's functionality; UI upgrades without DS downtime; separate logs, limits, healthchecks; maps naturally onto Kubernetes (sidecar in a Pod).
- (+) Mode B allows connecting remote nodes (replicas) with the same UI.
- (−) Offline operations (dbverify, BDB→LMDB migration, repairing a database that won't start) — only via `docker exec`.
- (−) Paths and uids must be aligned between the images; a UI running as uid 0 with access to `/data` is close to mode A in terms of risk.

**Alternatives.** A only (simple, 100% of functions, but DS restarts on every UI upgrade); B only (isolation, but ~85% of functions and the DM password is required).

**Docker API proxy (clarified in stage 0).** We use `wollomatic/socket-proxy` with an allowlist by method and path regex:
`GET (/vX)?/containers/389-ds-ui-ds-1/json` and `POST (/vX)?/containers/389-ds-ui-ds-1/(restart|stop|start)`, source — the `ui` service only.
Verified: create, exec, requests to other containers, and requests not from `ui` → 403.
`tecnativa/docker-socket-proxy` was rejected: its `ALLOW_RESTARTS` only works together with `POST=1`, and `POST=1` + `CONTAINERS=1` opens any POST on containers, including `create` and `exec` (a root shell in DS).

---

## ADR-003. Connecting via LDAPI autobind (UI sign-in: see ADR-011)

**Status:** Accepted (revisit after 1.0)

**Decision.** In modes A/C ds-ui connects to `ldapi://` with SASL EXTERNAL → DS maps it to Directory Manager (`nsslapd-ldapiautobind`, `nsslapd-ldapimaprootdn`). The UI does not store the DM password.

**Clarified in stage 0.** 389-ds 3.1 maps to DM not only uid 0 but also the server process uid (`nsslapd-localuser: dirsrv`). Therefore ds-ui runs **as uid 389 (`dirsrv`), without root**: files it creates on `/data` are owned by `dirsrv` right away. uid/gid 389 are pinned in both images. At first the UI had no sign-in and was published only on `127.0.0.1`/the internal network; sign-in came later (ADR-011).

**Consequences.** Minimal secrets; anyone who reaches the UI port is effectively DM. The API is designed so that later the LDAP connection can be taken "from the request context" (user bind, permissions via ACI).

---

## ADR-004. Frontend: Vue 3 + Naive UI, SPA embedded in the binary

**Status:** Accepted (replaced the PrimeVue option in stage 0)

**Decision.** Vue 3 + TypeScript + Vite + **Naive UI** (MIT) + **@lucide/vue** icons (ISC); a TS client generated from OpenAPI; `go:embed`.

**Rationale for choosing Naive UI.**
- `NTree` supports both lazy node loading (`on-load`) and virtual scrolling at the same time — a key requirement for the LDAP browser (branches with tens of thousands of entries); in Element Plus these are two different components (`ElTree` with lazy, `ElTreeV2` with virtualization).
- `NDataTable` with virtual scrolling and remote mode, `NForm` with dynamic validation — suited to schema-driven forms and YAML pages.
- TypeScript-first, dark theme and Russian locale built in (`darkTheme`, `ruRU`), CSS-in-JS with no global styles.
- MIT, actively developed (2.45, 2026), ~0.9M downloads per month.

**Why not PrimeVue.** Since version 5.0 (2026-07-15) `primevue`, `@primevue/*`, `@primeuix/*`, `primeicons@8` are distributed under the commercial *PrimeUI License* (a license key; an "Invalid PrimeUI License" banner without one; free only for small organizations and non-commercial open source). The last MIT version, 4.5.5, will not be developed further.

**Alternatives.** Element Plus (MIT, the most popular, but its tree is a worse fit), Vuetify 4 (Material, heavier), Quasar (a whole framework), React + PatternFly (a stack change).

**Rule for frontend dependencies.** Before adding a package, check the license of its current version (`npm view <pkg> license`) — MIT, ISC, BSD, Apache-2.0 are allowed. Checked automatically: `npm run check:licenses` (`web/scripts/check-licenses.mjs`) fails in CI on any npm package outside the allowlist.

**Other pins.** TypeScript is pinned to 5.9: TS 7 (the native version) does not provide the JS API that `vue-tsc` needs, and `openapi-typescript` 7 requires TS 5.x. Icons — `@lucide/vue` (the `lucide-vue-next` package is frozen at 1.0.0).

---

## ADR-005. Declarative YAML pages for uniform settings

**Status:** Accepted

**Context.** The bulk of cockpit consists of forms of the "read entry attributes → display → save the diff" kind (cn=config, logs, simple plugins).

**Decision.** The `internal/pages` engine: a YAML description (DN, fields, types, hints, `restartRequired`, conditions on version/DB implementation) + two generic endpoints + a generic renderer on the frontend. Hand-written code only for complex screens.

**Consequences.** Time to parity reduced by 1–2 months; pages can be added without Go code; a single diff/restart/validation logic.

---

## ADR-006. DS base image: Debian trixie + 389-ds-base from the Debian repository

**Status:** Accepted (verified in stage 0)

**Decision.** `debian:trixie-slim` + the `389-ds-base` package (3.1.2, LMDB by default) + `tini`. The instance is created and started by upstream `dscontainer`; the wrapper `deploy/ds/entrypoint.sh` adds `DS_DM_PASSWORD_FILE` and creation of the `DS_SUFFIX` suffix. All state is in `/data`, the socket is `/data/run/slapd-localhost.socket`, ports 3389/3636, the process runs as `dirsrv` (389:389). Image size ~436 MB (Python and lib389 are package dependencies).

---

## ADR-007. HTTP layer: chi + huma v2, OpenAPI as the contract

**Status:** Accepted

**Decision.** huma v2 on top of chi: OpenAPI 3.1 from Go types, errors in RFC 9457 format (`application/problem+json`), SSE on `net/http`. OpenAPI is the source for generating the TS client.

**Alternatives.** Plain `net/http` + hand-written OpenAPI (more boilerplate); gRPC/Connect (overkill for a browser UI).

---

## ADR-008. No code is borrowed from other projects

**Status:** Accepted

**Decision.** Code from lib389, cockpit-389-ds and other projects (including GPL ones) is not brought into the project — neither verbatim nor "translated" into Go/TS. 389-ds behavior is studied from documentation, the schema (`cn=schema`, `01core389.ldif`) and experiments on a live server; the results are recorded in `docs/ds-reference.md` and confirmed by integration tests. Only third-party libraries are used, as dependencies under permissive licenses (MIT, ISC, BSD, Apache-2.0, MPL-2.0 for build tools).

**Consequences.** The project license can be chosen freely: there are no constraints from GPL code. Knowledge about `cn=config` accumulates in our own reference and tests.

---

## ADR-009. Multilingual UI

**Status:** Accepted

**Context.** The interface is needed in Russian and English, with the ability to easily add other languages.

**Decision.**
- `vue-i18n` 11 (MIT), Composition API. One language — one file `web/src/i18n/locales/<code>.json`; files are discovered automatically (`import.meta.glob`); a language appears in the switcher without registration.
- `en.json` is the reference key set and the fallback. `en` and `ru` must be complete; other languages may be partial (missing strings are shown in English).
- Pluralization — a single generic function based on `Intl.PluralRules`: forms in CLDR category order. No rules need to be written for a new language.
- Language selection: the one saved in `localStorage` → browser languages → English.
- Naive UI's built-in texts are wired through the table in `web/src/i18n/naive.ts` (optional for a new language).
- **The backend is not localized**: the API, logs and technical error details are in English; the API returns machine-readable values (`ldapCode`, HTTP status, enums), and the UI translates them. `Accept-Language` is not used.
- YAML settings pages and entry templates contain only identifiers; texts live in the locale files (`pages.*`, `templates.*`), falling back to English, then to the attribute name / `DESC` from the schema.
- The `npm run check:locales` check in CI: completeness of required languages, no extra keys, matching placeholders, existence of all static `t('…')` keys used in code. Reason for a separate check: `t()` in vue-i18n accepts any string, and the `DefineLocaleMessage` type only provides autocompletion.

**Consequences.** Every UI feature is considered done only with keys in `en.json` and `ru.json` (a definition of done for all stages). Details for developers and translators — [i18n.md](i18n.md).

**Alternatives.** Server-side localization (Go, `Accept-Language`) — rejected: it complicates the API and caching, and all UI texts live in the frontend anyway. Texts directly in YAML pages (`label: {en, ru}`) — rejected: adding a language would require editing every YAML file.

---

## ADR-010. Large branches are paged by the server, not with VLV

**Status:** Accepted (stage 1)

**Context.** The tree must open a branch of 100k entries without freezing. The first implementation read all children (Paged Results), sorted them by RDN and cached the list. Measured on 389-ds 3.1.2: reading 100k children takes the server 8–12 s even with the attribute list `1.1`, and ds-ui grew to ~650 MB. The roadmap proposed VLV + Server Side Sort; in 389-ds VLV needs a VLV index configured for each base/filter, which ordinary branches do not have.

**Decision.** Branches with at most 2000 children (`numSubordinates` of the parent) are read completely, sorted by RDN and cached for 30 s. Larger branches are listed in server order with Paged Results: the API cursor carries the server's paging cookie (`p:<base64>`), each "load more" fetches the next server page (~20–40 ms). The response flags `sorted: false`; the tree labels such branches "server order" and points the user to search. Cookies are bound to the LDAP connection: after a reconnect the API answers 409 and the UI reloads the branch.

**Consequences.** 100k branch: first page 39 ms, ds-ui RSS 7 MB. Large branches are not alphabetical; finding a specific entry there is done with search. VLV can be added later for branches where an administrator has configured a VLV index (stage 2 manages VLV indexes).

**Alternatives.** VLV + SSS (needs per-branch indexes); SSS alone (still a full scan per request); full read with a longer cache (first open still 8–12 s, memory proportional to branch size).


## ADR-011. Sign-in with directory accounts; data access as the user

**Context.** ds-ui had no sign-in and acted as Directory Manager for everyone. Options: one shared admin password; a reverse proxy that authenticates (trusted header); directory accounts; OIDC; client certificates.

**Decision.** Users sign in with their directory accounts (the password is checked with a simple bind to the server). The role comes from configured groups: *administrator* (everything) or *operator* (reads the administration, works with data); Directory Manager is always an administrator. Directory data — the LDAP browser, search, LDIF, export — is accessed with the Proxied Authorization control (RFC 4370) as the user, so the server's ACIs decide what they may see and change; server administration stays with ds-ui's own privileges and is limited by role. Sessions are in memory with idle and absolute timeouts; failed sign-ins are rate-limited.

**Consequences.** No extra service and no second user database; password policies, lockout and the security log of 389-ds apply to ds-ui sign-in. Users who should edit data need ACIs. 389-ds records ds-ui's bind DN in `modifiersName` for proxied operations, so authorship lives in the ds-ui log and task history. The Password Modify extended operation cannot carry the control in go-ldap, so signed-in users change passwords with a modify of `userPassword` (no generated passwords). Sessions do not survive a ds-ui restart. A reverse-proxy or OIDC mode can be added later on top of the same roles.
