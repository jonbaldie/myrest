# myrest

myrest will be an HTTP API service that exposes MySQL 8.0+ with PostgREST-compatible client contracts.

This repository is in **prefactor**: the Go module, quality gates, MySQL fixture harness, and HTTP test seam are in place. The running service does **not** claim PostgREST parity yet (`GET /` returns `"parity":"none"`).

## Requirements

- Go 1.26+
- Docker (for the MySQL 8.0+ test harness)
- [`messgo`](https://github.com/quality-gates/messgo) and [`mutago`](https://github.com/quality-gates/mutago) on `PATH`

```bash
go install github.com/quality-gates/messgo/cmd/messgo@latest
go install github.com/quality-gates/mutago/v2/cmd/mutago@latest
```

## Commands

| Command | Purpose |
| --- | --- |
| `go run ./cmd/myrest [config-file]` | Start the myrest service (`MYREST_LISTEN`, default `127.0.0.1:3000`) |
| `make test` | Run tests (HTTP seam + MySQL harness) |
| `make messgo` | Run messgo `design` and `codesize` rulesets (must report no violations) |
| `make mutago` | Run mutago on `./internal/config` and `./internal/httpapi` with `--coverage --min-covered-msi 80` |
| `make mysql-fixtures` | Start MySQL 8.0+ and load `testdata/fixtures/schema.sql` |

## Configuration

Give myrest its settings in a config file, in `MYREST_*` environment variables, or in both. The one optional argument of the process is the path of the config file. An environment variable with a value wins over the same knob in the file; an empty variable counts as a variable nobody set. A restart applies a changed value; there is no live configuration reload.

```conf
# myrest.conf
db-uri = "mysql://authenticator:secret@127.0.0.1:3306/"
db-schemas = "shop, warehouse"
db-anon-role = "myrest_anon"
db-aggregates-enabled = false
db-max-rows = 1000
```

Each knob keeps its PostgREST kebab-case name. Its environment variable is the name in capitals, with `MYREST_` in front and underscores instead of dashes: `db-uri` becomes `MYREST_DB_URI`.

### Minimum run set

myrest serves the API only when it has all of these. If one is missing, the process says which knob is missing and stops.

| Knob | Meaning |
| --- | --- |
| `db-uri` | The MySQL authenticator URI |
| `db-schemas` | One or more MySQL databases to expose |
| `jwt-secret` and/or `db-anon-role` | The JWT secret, the anonymous database role, or both |

### Other kept knobs

These knobs are configurable and readable now. Later parity slices give them their behaviour.

| Knob | Type | Default |
| --- | --- | --- |
| `jwt-secret-is-base64` | boolean | `false` |
| `jwt-aud` | text | none |
| `jwt-role-claim-key` | text | `.role` |
| `jwt-cache-max-entries` | count | `1000` |
| `db-aggregates-enabled` | boolean | `false` |
| `db-max-rows` | count | no cap |
| `db-pre-request` | text | none |
| `db-tx-end` | `commit`, `commit-allow-override`, `rollback`, `rollback-allow-override` | `commit` |
| `server-cors-allowed-origins` | list | every origin |
| `openapi-mode` | `follow-privileges`, `ignore-privileges`, `disabled` | `follow-privileges` |
| `openapi-security-active` | boolean | `false` |
| `openapi-server-proxy-uri` | text | none |
| `db-root-spec` | text | none |

Knobs on the drop list of [ADR 0007](docs/adr/0007-config-surface-mapping.md) — in-database config, NOTIFY channel, `search_path` extras, GUC or `app.settings` injection, plan-media gate, admin listen — are not on this surface. myrest refuses a config file that holds a name it does not know. `MYREST_LISTEN` is process tuning, not parity law, so it has no config file entry.

## Fixture DDL

`testdata/fixtures/schema.sql` creates database `myrest_fixture` and table `items` for later parity slices. Parent-spec fixtures stay intent-only; this file is the concrete DDL for the harness.
