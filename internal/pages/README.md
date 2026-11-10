# Settings pages

A settings page is one LDAP entry (usually under `cn=config`) and some of its
attributes, described in YAML under `defs/`. The API serves every page through
the same three endpoints (`GET /pages`, `GET /pages/{id}`, `PATCH /pages/{id}`),
and the UI renders it with a single generic component, so a new page needs no
Go or Vue code (ADR-005).

## File format

Each `defs/*.yaml` file holds a list of pages and, optionally, named
fieldsets that pages of the same file share:

```yaml
fieldsets:
  rotation:
    - {attr: "nsslapd-{log}-maxlogsize", type: int, unit: MB, text: log.maxlogsize, group: rotation}

pages:
  - id: log-errors              # [a-z][a-z0-9-]*, unique; also the URL ?page=
    section: logging            # menu section: server, database, logging, security, …
    order: 20                   # position inside the section
    dn: cn=config               # the entry
    when: {dbImpl: mdb}         # optional: only for servers matching these facts
    vars: {log: errorlog}       # optional: {log} is replaced in id, dn and attr
    fields:
      - {attr: nsslapd-errorlog-level, type: flags, group: general, flags: [{name: default, value: 16384}]}
    use: [rotation]             # fieldsets appended after fields
```

Unknown keys are rejected when the definitions load, so a typo fails at startup
and in `go test`.

### Fields

| Key | Meaning |
|---|---|
| `attr` | Attribute name, as the UI shows it |
| `type` | `string`, `dn`, `int`, `bool`, `enum`, `list` (multi-valued), `size` (bytes), `duration` (seconds), `flags` (bit mask) |
| `group` | Heading the field appears under; texts in `pages._groups.<group>` |
| `text` | Use the shared texts `pages._fields.<text>` instead of the page's own |
| `min`, `max` | Range of numeric types, checked on the server |
| `unit` | Unit label next to an `int` (`MB`, `ms`, `min`; texts in `pages._units`) |
| `values` | Allowed values of an `enum` |
| `optionsFrom` | `{base, attr}`: more `enum` values, read from the children of `base` (e.g. the password storage schemes) |
| `bool` | `[true, false]` literals; default `[on, off]` |
| `flags` | `[{name, value}]` named bits of a `flags` field; unknown bits are kept |
| `restart` | The server applies the change only after a restart (see `nsslapd-requiresrestart` in `cn=config`) |
| `danger` | A wrong value can cut ds-ui off from the server; the UI asks for confirmation |
| `readOnly` | Shown but not editable |

Page-level keys besides the ones above: `plugin` (the cn of the plug-in the
page configures; such pages appear on the Plugins page, not in the settings
menu) and `sparse` (attributes may be absent until set; the integration test
then checks that the entry's object classes allow them instead of requiring
them on the server).

`entryClass` (instead of `dn`): the page edits whichever entry of that object
class the caller names with `?dn=` — e.g. a local password policy. Such pages
are not in the settings menu.

Facts available to `when`: `dbImpl` (`mdb` or `bdb`).

## Reading and writing

`GET` returns the page, the current values (an absent attribute means the
server default) and the server-read `enum` options. `PATCH` takes
`{values: {attr: [..]}}`; only attributes whose values differ are validated and
written, all in a single modify. An empty list removes the attribute, which
returns it to the server default. Changed `restart` attributes are added to the
restart registry and shown in the restart banner.

## Texts

The YAML holds no human-readable text. The UI looks up, in the locale files:

- `pages.<id>.title`, `pages.<id>.description` (optional)
- `pages.<id>.fields.<attr>.label`, `.help` (optional), `.values.<value>` (optional, enum labels)
- or `pages._fields.<text>.*` for fields with `text`
- `pages.<id>.flags.<name>` for `flags` fields
- `pages._sections.<section>`, `pages._groups.<group>`, `pages._units.<unit>`

`TestLocalesCoverPages` fails when a text is missing from `en.json` or `ru.json`.

## Checklist for a new page

1. Add the page to a file in `defs/` (or a new file).
2. Add its texts to `web/src/i18n/locales/en.json` and `ru.json`.
3. `go test ./internal/pages` checks the definitions and the texts;
   `make itest` checks that every attribute exists on a real server and that
   the enum options are not empty.
