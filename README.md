# myrest

myrest will be an HTTP API service that exposes MySQL 8.0+ with PostgREST-compatible client contracts.

The service serves Bearer JWT authentication, ordinary reads of one exposed
table (column select, full-match filters, order, page, HEAD, exact count, and
aggregates when enabled), **embed** of related resources over declared foreign
keys, and `POST /rpc` for functions and procedures. Every other part of the
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
operator list, and [Read parity boundaries](docs/read-parity-boundaries.md)
for the text-case and JSON path subsets and the stable refusals
(`ilike`, JSON `->` / `->>`, and refused FTS / array / range / planned count).

## Embed

A nested select loads related rows when a **relationship** is in the **schema
cache**. Relationships come only from declared foreign keys:

```bash
curl "http://127.0.0.1:3000/orders?select=id,items(id,name)&id=eq.1"
[{"id":1,"items":{"id":1,"name":"alpha"}}]
```

Nested filter, order, and limit use the embed name as a prefix
(`orders.order=id.desc&orders.limit=1`). Many-to-many uses a declared join
table. When more than one foreign key applies, disambiguate with `!fk_name`.
See [Embed](docs/embed.md).

## Aggregates

Aggregates are off by default (`db-aggregates-enabled = false`). When the
operator turns them on, `sum` / `count` / `avg` / `min` / `max` select forms
and automatic group behaviour match the parity target:

```bash
curl "http://127.0.0.1:3000/orders?select=count(),item_id"
[{"count":2,"item_id":1},{"count":1,"item_id":2}]
```

While the gate is off, an aggregate select refuses with `PGRST123`. Aggregate
plus **embed** works when the parity target allows it and the **relationship**
is in the **schema cache**. Aggregates inside a to-many spread refuse with
`PGRST127`. See [Aggregates](docs/aggregates.md).

myrest opens pooled MySQL connections as the **authenticator** of `db-uri` and activates the database role for each request, so MySQL grants — not a second access list — say what a client may read. After the **role switch**, grants follow the active role, but SQL `CURRENT_USER()` stays the authenticator (a documented **partial match**). See [Authentication](docs/auth.md).

A table is a **resource** of the request only when it is in the selected MySQL database and the active role holds `SELECT` on it, of itself or through a role granted to it. With no profile header the selected database is the **default database** (the first of `db-schemas`). `Accept-Profile` selects the database for a read; `Content-Profile` selects it for a write. A profile outside `db-schemas` refuses with `PGRST106`. Any other table name gets the PostgREST error envelope:

```bash
curl http://127.0.0.1:3000/secrets
{"code":"PGRST205","message":"Could not find the table 'shop.secrets' in the schema cache","details":null,"hint":null}
```

```bash
curl http://127.0.0.1:3000/items -H 'Accept-Profile: warehouse'
# reads warehouse.items when warehouse is in db-schemas
```

When MySQL itself refuses a read — a grant taken away after start-up, for example — the client gets the same envelope with a message of myrest. What MySQL said names the accounts of the deployment, so it goes to the log of the operator and not to the client.

myrest builds the **schema cache** from the MySQL catalog at start-up. Send `SIGUSR1` to reload it after DDL or grant changes; a process restart is not required for that refresh. Config changes still need a restart.

## Calling a routine

`POST /rpc/<name>` calls a MySQL function or procedure in the selected database when the active **database role** holds `EXECUTE` on it. With no profile header that database is the **default database**. `Content-Profile` selects the database for `POST /rpc`; `Accept-Profile` selects it for `GET /rpc`. Named JSON object keys are the argument names:

```bash
curl -X POST http://127.0.0.1:3000/rpc/add_them \
  -H 'Content-Type: application/json' \
  -d '{"a":1,"b":2}'
3
```

Functions match the PostgREST scalar body. Procedures use the same path and return one stable JSON object of `OUT`/`INOUT` values (or `{}` when there are none). See [Procedure RPC response shape](docs/rpc-procedures.md).

`GET /rpc/<name>` is a **partial match**: it runs only when the routine is **read-safe** under MySQL `SQL_DATA_ACCESS`. Named query-string keys are the argument names. A non-read-safe routine refuses stably. See [GET /rpc read-safe routines](docs/rpc-get.md).

Unusual whole-body `POST /rpc` argument modes (single unnamed `json`/`jsonb`/`bytea`/`text`/`xml`) are **not supported** and refuse stably. See [RPC whole-body argument modes](docs/rpc-body-modes.md).

## CORS and proxy URLs

`server-cors-allowed-origins` sets the browser origin policy. An empty list accepts every origin. Allowed origins get the PostgREST CORS response and preflight headers; an origin outside the list gets no `Access-Control-Allow-Origin`. myrest never takes host or scheme from `X-Forwarded-*` or `Forwarded`. When it reports an absolute base URL, `openapi-server-proxy-uri` wins when set. See [CORS origins and proxy header behaviour](docs/cors-and-proxy.md) and [ADR 0012](docs/adr/0012-cors-and-proxy-headers.md).

## Discovery

`OPTIONS` on a table or `/rpc` path reports `Allow` from the grants of the active **database role** in the **schema cache**. `GET /` serves an OpenAPI 2.0 document from the same cache and privileges. `openapi-mode`, `openapi-security-active`, `openapi-server-proxy-uri`, and `db-root-spec` change that output as documented. See [Discovery: OPTIONS and OpenAPI](docs/discovery.md).

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

`server-cors-allowed-origins` and `openapi-server-proxy-uri` already have the CORS and reported-base-URL behaviour above. The JWT knobs (`jwt-secret-is-base64`, `jwt-aud`, `jwt-role-claim-key`, `jwt-cache-max-entries`) already drive Bearer JWT verification; see [Authentication](docs/auth.md). The OpenAPI knobs (`openapi-mode`, `openapi-security-active`, `openapi-server-proxy-uri`, `db-root-spec`) drive discovery; see [Discovery: OPTIONS and OpenAPI](docs/discovery.md). The other knobs are configurable and readable now; later parity slices give them their remaining behaviour.

| Knob | Type | Default |
| --- | --- | --- |
| `jwt-secret-is-base64` | boolean | `false` |
| `jwt-aud` | text | none |
| `jwt-role-claim-key` | text | `.role` |
| `jwt-cache-max-entries` | count | `1000` |
| `db-aggregates-enabled` | boolean | `false` |
| `db-max-rows` | count | no cap |
| `db-pre-request` | text (`database.routine`) | none |
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
