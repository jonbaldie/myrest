# myrest

myrest will be an HTTP API service that exposes MySQL 8.0+ with PostgREST-compatible client contracts.

The service serves its first parity slice: an **anonymous read** of one exposed table. Every other part of the PostgREST surface is still to come.

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
| `make test` | Run tests (unit tests, process tests, and the MySQL 8 acceptance tests in `test/acceptance`) |
| `make messgo` | Run messgo `design` and `codesize` rulesets (must report no violations) |
| `make mutago` | Run mutago on the production packages with `--coverage --min-covered-msi 80` |
| `make mysql-fixtures` | Start MySQL 8.0+ and load `testdata/fixtures/schema.sql` |

## Reading a table

A client that sends no JWT reads a table as the **anonymous database role** of `db-anon-role`:

```bash
curl http://127.0.0.1:3000/items
[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]
```

myrest opens pooled MySQL connections as the **authenticator** of `db-uri` and activates the database role for each request, so MySQL grants — not a second access list — say what a client may read. A table is a **resource** of the request only when its database is in `db-schemas` and the active role holds `SELECT` on it. Any other name gets the PostgREST error envelope:

```bash
curl http://127.0.0.1:3000/secrets
{"code":"PGRST205","message":"Could not find the table 'shop.secrets' in the schema cache","details":null,"hint":null}
```

When MySQL itself refuses a read — a grant taken away after start-up, for example — the `message` stays what myrest says, and `details` carries what the database said.

The read is narrow for now: all columns, no filter, no order, and no page. myrest reads the catalog once at start-up; a restart picks up new tables and new grants until the explicit reload arrives.

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

## Database accounts

myrest logs in as one account and takes its privileges from the database role of the request. Give the authenticator no privileges of its own:

```sql
CREATE ROLE 'myrest_anon';
CREATE USER 'authenticator'@'%' IDENTIFIED BY 'secret';
GRANT 'myrest_anon' TO 'authenticator'@'%';
SET DEFAULT ROLE NONE TO 'authenticator'@'%';
GRANT SELECT ON shop.items TO 'myrest_anon';
```

The authenticator must hold every database role myrest activates, because MySQL shows catalog rows only to an account that holds a privilege on them. See [ADR 0010](docs/adr/0010-catalog-read-under-authenticator-roles.md).

## Fixture DDL

`testdata/fixtures/schema.sql` creates the databases `myrest_fixture` and `myrest_hidden`, the tables the tests read, the authenticator login, and the anonymous database role. Parent-spec fixtures stay intent-only; this file is the concrete DDL for the harness.
