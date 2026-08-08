# Config surface

myrest keeps a short normative **knob** map with PostgREST kebab-case names.
The **serve gate** requires the **minimum run set** before the process may
listen. Delivery is a config file and/or `MYREST_*` environment variables.
Restart applies config. Live config reload is out of scope; **schema cache**
reload stays separate.

See [ADR 0007](adr/0007-config-surface-mapping.md) and
[ADR 0009](adr/0009-config-file-format-and-precedence.md).

## Gap list rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| Config drop-list knobs (in-database config, NOTIFY channel, search_path extras, GUC hoist, plan-media gate, admin listen) | not supported | cfg-003 |
