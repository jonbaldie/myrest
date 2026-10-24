# Config surface

myrest keeps a short normative **knob** map with PostgREST kebab-case names.
The **serve gate** requires the **minimum run set** before the process may
listen. Delivery is a config file and/or `MYREST_*` environment variables.
Restart applies config. Live config reload is **not supported**; **schema cache**
reload stays separate.

See [ADR 0007](adr/0007-config-surface-mapping.md) and
[ADR 0009](adr/0009-config-file-format-and-precedence.md).

## File values

Use double quotes for text values, including comma-separated lists and choices:

```text
db-schemas = "myrest_fixture"
db-tx-end = "commit"
db-aggregates-enabled = true
db-max-rows = 500
```

Text without double quotes, including single-quoted text, is refused with the
file line number and knob name. Only boolean and integer knobs accept bare
values. Put comments on separate lines that start with `#`; inline comments are
not supported. A `#` inside double quotes is part of the text.

`MYREST_*` environment values do not need these file quotes.

## Startup checks

After file and environment values are resolved, myrest checks the minimum run
set and local knob shapes before database login or listener creation. It reports
all these faults together, in a fixed order, without secret values:

- `db-pre-request`: empty, or a `database.routine` name. Back quotes in either
  identifier remain valid; the database adapter escapes them.
- `db-anon-role`: a name or `name@host`. Names and explicit hosts must not be
  empty or contain quote, escape, or control characters.
- `jwt-secret`: a nonempty secret must decode as standard base64 when
  `jwt-secret-is-base64` is `true`. Raw secrets and anonymous-only operation
  remain valid.

These checks do not test database connectivity, object existence, or privileges.
A malformed setting that previously failed each request now stops startup.

## Full match rows

| Item | Parity label | Scenarios | Client-visible error |
| --- | --- | --- | --- |
| Serve gate for an incomplete minimum run set | full match | cfg-001 | yes |
| Config file and MYREST_* environment delivery | full match | cfg-002 | |

## Gap list rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| Config drop-list knobs (in-database config, NOTIFY channel, search_path extras, GUC hoist, plan-media gate, admin listen) | not supported | cache-004 |
| Live config reload | not supported | cfg-003 |

Drop-list knobs share one refuse path: the process refuses an unknown knob in
the config file. `cache-004` proves that path with `db-channel` (NOTIFY).
