# Config surface

myrest keeps a short normative **knob** map with PostgREST kebab-case names.
The **serve gate** requires the **minimum run set** before the process may
listen. Delivery is a config file and/or `MYREST_*` environment variables.
Restart applies config. Live config reload is **not supported**; **schema cache**
reload stays separate.

See [ADR 0007](adr/0007-config-surface-mapping.md) and
[ADR 0009](adr/0009-config-file-format-and-precedence.md).

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
