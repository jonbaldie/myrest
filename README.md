# myrest

myrest will be an HTTP API service that exposes MySQL 8.0+ with PostgREST-compatible client contracts.

The service serves Bearer JWT authentication, ordinary reads of one exposed
table (column select, full-match filters, order, page, HEAD, and exact count),
and `POST /rpc` for functions and procedures. Every other part of the
PostgREST surface is still to come.

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
curl "http://127.0.0.1:3000/items?select=id,name&name=eq.alpha&order=id.asc&limit=1"
[{"id":1,"name":"alpha"}]
```

A client that sends a valid Bearer JWT reads as the **database role** named by the role claim (default `role`):

```bash
curl http://127.0.0.1:3000/secrets \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
[{"id":1,"payload":"top-secret"}]
```

`HEAD` uses the same read intent and returns no body. `Prefer: count=exact`
puts the exact total in `Content-Range`. `db-max-rows` is a hard row cap when
set. See [Ordinary read](docs/ordinary-read.md) for the full-match filter
operator list.

myrest opens pooled MySQL connections as the **authenticator** of `db-uri` and activates the database role for each request, so MySQL grants — not a second access list — say what a client may read. After the **role switch**, grants follow the active role, but SQL `CURRENT_USER()` stays the authenticator (a documented **partial match**). See [Authentication](docs/auth.md).

A table is a **resource** of the request only when it is in the **default database** (the first of `db-schemas`) and the active role holds `SELECT` on it, of itself or through a role granted to it. A table of another configured database waits for the content negotiation that names its database. Any other name gets the PostgREST error envelope:

```bash
curl http://127.0.0.1:3000/secrets
{"code":"PGRST205","message":"Could not find the table 'shop.secrets' in the schema cache","details":null,"hint":null}
```

When MySQL itself refuses a read — a grant taken away after start-up, for example — the client gets the same envelope with a message of myrest. What MySQL said names the accounts of the deployment, so it goes to the log of the operator and not to the client.

myrest builds the **schema cache** from the MySQL catalog at start-up. Send `SIGUSR1` to reload it after DDL or grant changes; a process restart is not required for that refresh. Config changes still need a restart.

## Calling a routine

`POST /rpc/<name>` calls a MySQL function or procedure in the **default database** when the active **database role** holds `EXECUTE` on it. Named JSON object keys are the argument names:

```bash
curl -X POST http://127.0.0.1:3000/rpc/add_them \
  -H 'Content-Type: application/json' \
  -d '{"a":1,"b":2}'
3
```

Functions match the PostgREST scalar body. Procedures use the same path and return one stable JSON object of `OUT`/`INOUT` values (or `{}` when there are none). See [Procedure RPC response shape](docs/rpc-procedures.md).

## CORS and proxy URLs

`server-cors-allowed-origins` sets the browser origin policy. An empty list accepts every origin. Allowed origins get the PostgREST CORS response and preflight headers; an origin outside the list gets no `Access-Control-Allow-Origin`. myrest never takes host or scheme from `X-Forwarded-*` or `Forwarded`. When it reports an absolute base URL, `openapi-server-proxy-uri` wins when set. See [CORS origins and proxy header behaviour](docs/cors-and-proxy.md) and [ADR 0012](docs/adr/0012-cors-and-proxy-headers.md).

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

`server-cors-allowed-origins` and `openapi-server-proxy-uri` already have the CORS and reported-base-URL behaviour above. The JWT knobs (`jwt-secret-is-base64`, `jwt-aud`, `jwt-role-claim-key`, `jwt-cache-max-entries`) already drive Bearer JWT verification; see [Authentication](docs/auth.md). The other knobs are configurable and readable now; later parity slices give them their remaining behaviour.

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

The authenticator must hold every database role myrest activates, because MySQL shows catalog rows only to an account that holds a privilege on them. See [ADR 0010](docs/adr/0010-catalog-read-under-authenticator-roles.md). A role granted to `myrest_anon` widens what an anonymous client reads, because MySQL reads with the privileges of the roles granted to the active role. See [ADR 0011](docs/adr/0011-bare-table-name-reads-the-default-database.md).

## Fixture DDL

`testdata/fixtures/schema.sql` creates the databases `myrest_fixture` and `myrest_hidden`, the tables the tests read, the authenticator login, the anonymous database role, and the JWT role `myrest_user`. Parent-spec fixtures stay intent-only; this file is the concrete DDL for the harness.
