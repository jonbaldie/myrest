# myrest

[![Go Report Card](https://github.com/jonbaldie/myrest/actions/workflows/goreportcard.yml/badge.svg)](https://github.com/jonbaldie/myrest/actions/workflows/goreportcard.yml)
[![Security](https://github.com/jonbaldie/myrest/actions/workflows/security.yml/badge.svg)](https://github.com/jonbaldie/myrest/actions/workflows/security.yml)
[![Release](https://img.shields.io/github/v/release/jonbaldie/myrest)](https://github.com/jonbaldie/myrest/releases/latest)

myrest exposes MySQL 8.0+ as a REST API. It supports the parts of the
[PostgREST](https://postgrest.org/) client contract that map clearly to MySQL.

Use myrest when you want PostgREST-style reads, writes, embedded resources,
JWT roles, and routine calls for a MySQL database.

## Features

- Read and write MySQL tables and updatable views through HTTP.
- Filter, select, order, and page rows with PostgREST query parameters.
- Embed related rows through declared foreign keys.
- Call MySQL functions and procedures through `/rpc` routes.
- Use Bearer JWTs to select MySQL database roles.
- Expose more than one database with profile headers.
- Return JSON, CSV, and PostgREST error envelopes.
- Generate an OpenAPI document from the live schema cache.
- Reload the schema cache without a process restart.

myrest targets PostgREST v14.16. Compatibility is tested, but it is not
complete. See [Compatibility](#compatibility) before you use it as a
replacement for PostgREST.

## Requirements

- Go 1.26.5 or later
- MySQL 8.0 or later
- Docker for the development database and acceptance tests

## Install

Install the latest release with Go:

```bash
go install github.com/jonbaldie/myrest/cmd/myrest@latest
```

Or build it from source:

```bash
git clone https://github.com/jonbaldie/myrest.git
cd myrest
make build
```

The source build writes the binary to `bin/myrest`.

## Quick start

The repository includes a disposable MySQL fixture. From a source checkout,
start it in one terminal:

```bash
make mysql-fixtures
```

The command prints the random host port for MySQL. Replace `PORT` in this
configuration with that value:

```conf
# myrest.conf
db-uri = "mysql://authenticator:secret@127.0.0.1:PORT/"
db-schemas = "myrest_fixture"
db-anon-role = "myrest_anon"
```

Start myrest in a second terminal:

```bash
make build
MYREST_LISTEN=127.0.0.1:3000 ./bin/myrest ./myrest.conf
```

Read the fixture data:

```bash
curl "http://127.0.0.1:3000/items?select=id,name&order=id.asc&limit=1"
```

The response is:

```json
[{"id":1,"name":"alpha"}]
```

Stop the fixture with Ctrl+C when you finish.

## Configuration

Pass a configuration file as the optional command-line argument:

```bash
myrest /etc/myrest.conf
```

You can also use `MYREST_*` environment variables. An environment value takes
precedence over the matching file value. For example, `db-uri` maps to
`MYREST_DB_URI`.

The minimum configuration is:

| Setting | Purpose |
| --- | --- |
| `db-uri` | MySQL URI for the authenticator account |
| `db-schemas` | Comma-separated databases to expose |
| `jwt-secret` and/or `db-anon-role` | JWT secret, anonymous role, or both |

Common optional settings include:

| Setting | Default | Purpose |
| --- | --- | --- |
| `db-aggregates-enabled` | `false` | Enable aggregate queries |
| `db-max-rows` | no limit | Set a maximum row count |
| `db-tx-end` | `commit` | Set write and RPC transaction behavior |
| `server-cors-allowed-origins` | all origins | Set the browser origin policy |
| `openapi-mode` | `follow-privileges` | Control OpenAPI output |
| `openapi-server-proxy-uri` | none | Set the public base URL in OpenAPI |

`MYREST_LISTEN` sets the listen address. Its default is `127.0.0.1:3000`.
Configuration changes need a process restart.

See the [configuration documentation](docs/config.md),
[authentication documentation](docs/auth.md), and
[transaction documentation](docs/transactions.md) for more details.

## Database access

myrest connects as one MySQL authenticator account. For each request, it
activates the database role from the JWT or `db-anon-role`. MySQL grants decide
which resources and methods the client can use.

A minimal anonymous setup looks like this:

```sql
CREATE ROLE 'myrest_anon';
CREATE USER 'authenticator'@'%' IDENTIFIED BY 'secret';
GRANT 'myrest_anon' TO 'authenticator'@'%';
SET DEFAULT ROLE NONE TO 'authenticator'@'%';
GRANT SELECT ON shop.items TO 'myrest_anon';
```

Grant each selectable role to the authenticator. Give table and routine grants
to those roles, not directly to the authenticator.

## API examples

Read and filter rows:

```bash
curl "http://127.0.0.1:3000/items?select=id,name&name=eq.alpha&limit=1"
```

Embed a related resource:

```bash
curl "http://127.0.0.1:3000/orders?select=id,items(id,name)"
```

Insert a row and return it:

```bash
curl -X POST http://127.0.0.1:3000/items \
  -H "Content-Type: application/json" \
  -H "Prefer: return=representation" \
  -d '{"name":"bravo"}'
```

Call a routine:

```bash
curl -X POST http://127.0.0.1:3000/rpc/add_them \
  -H "Content-Type: application/json" \
  -d '{"a":1,"b":2}'
```

Send a JWT with the standard authorization header:

```bash
curl http://127.0.0.1:3000/secrets \
  -H "Authorization: Bearer TOKEN"
```

The JWT role claim is `role` by default.

## Main routes

| Route | Purpose |
| --- | --- |
| `GET /<table>`, `HEAD /<table>` | Read rows |
| `POST /<table>` | Insert rows |
| `PATCH /<table>` | Update filtered rows |
| `DELETE /<table>` | Delete filtered rows |
| `PUT /<table>` | Upsert one row by primary key |
| `GET /rpc/<name>`, `POST /rpc/<name>` | Call a function or procedure |
| `GET /` | Get the OpenAPI 2.0 document |
| `OPTIONS /<resource>` | Get allowed methods |

## Schema changes

myrest reads tables, views, grants, routines, and relationships into a schema
cache at start-up. Send `SIGUSR1` after a DDL or grant change:

```bash
kill -USR1 12345
```

Replace `12345` with the myrest process ID. This reloads the schema cache. It
does not reload the configuration.

## Compatibility

The parity target is PostgREST v14.16. MySQL and PostgreSQL do not have the
same feature set, so myrest documents each full match, partial match, and
unsupported behavior.

Start with these documents:

- [Verification matrix and known gaps](docs/verification.md)
- [Read filters and query options](docs/ordinary-read.md)
- [Read compatibility boundaries](docs/read-parity-boundaries.md)
- [Embedded resources](docs/embed.md)
- [Writes](docs/write.md)
- [RPC behavior](docs/rpc-procedures.md)
- [Media types and Prefer values](docs/media-types-and-prefer.md)

## Development

Install the quality tools when you need the full local checks:

```bash
go install github.com/quality-gates/messgo/cmd/messgo@latest
go install github.com/quality-gates/mutago/v2/cmd/mutago@latest
```

| Command | Purpose |
| --- | --- |
| `make test` | Run all Go tests, including MySQL acceptance tests |
| `make scenarios` | Run the normative behavior scenarios |
| `make build` | Build `bin/myrest` |
| `make mysql-fixtures` | Start MySQL and load the test fixture |
| `make messgo` | Run design and code-size checks |
| `make mutago` | Run mutation tests with the required score |
| `make verification-docs` | Rebuild the verification matrix |

The acceptance tests use a disposable `mysql:8.0` Docker container. Set
`MYREST_MYSQL_HARNESS_PORT` to use an existing local MySQL test server instead.

Architecture decisions are in [docs/adr](docs/adr). Domain terms are in
[CONTEXT.md](CONTEXT.md).
