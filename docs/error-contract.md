# Error contract

myrest sends every failure as a JSON object with these fields:

```json
{"code":"MYREST002","message":"The database did not answer the request","details":null,"hint":null}
```

`code` and `message` are text. `details` and `hint` are `null` until a
documented failure gives them a value.

## Code catalog

myrest uses a `PGRST*` code only when the failure has the same PostgREST
meaning. For example, `PGRST205` means that a resource is not in the schema
cache. JWT failures use the PostgREST JWT group:

| Code | HTTP status | Meaning |
| --- | --- | --- |
| `PGRST301` | 401 | The Bearer JWT could not be decoded or verified. |
| `PGRST302` | 401 | The request has no usable JWT and anonymous access is disabled. |
| `PGRST303` | 401 | JWT claims validation failed (expired, audience, and related checks). |

The myrest gap codes are:

| Code | Meaning |
| --- | --- |
| `MYREST001` | The request needs PostgreSQL semantics that MySQL does not provide. |
| `MYREST002` | MySQL returned a database error. myrest cannot claim a PostgreSQL SQLSTATE for it. |
| `MYREST003` | The request path and method are not in the current myrest surface. |

`MYREST001` is for a documented MySQL-gap refusal. `MYREST002` is for a MySQL
database error. `MYREST003` is for an unhandled request. All codes have the
error envelope.

For example, myrest refuses the PostgREST `fts`, `plfts`, `phfts`, and `wfts`
full-text search operators, including their `not.` forms, with `MYREST001`.
MySQL has full-text search, but it does not have the same PostgREST semantics.
The same gap code refuses Postgres array and range operators, Prefer
`count=planned` / `count=estimated`, Postgres `match` / `imatch`, and JSON path
forms outside the named MySQL subset. See
[Read parity boundaries](read-parity-boundaries.md).
The same gap code refuses Postgres row-level security (`Prefer: row-security`),
request GUC / `request.jwt.claims` injection (`Prefer: jwt-claims`), and
non-Bearer credential schemes. See [Authentication](auth.md).

Embed failures that match PostgREST use `PGRST*` codes: `PGRST200` when no
declared foreign-key path exists, and `PGRST201` when more than one
relationship applies and the request does not disambiguate. A computed
relationship embed is a MySQL gap and uses `MYREST001`. See [Embed](embed.md).

`GET /rpc` on a routine that is not read-safe under MySQL `SQL_DATA_ACCESS`
also uses `MYREST001`. See [GET /rpc read-safe routines](rpc-get.md). Unusual
`POST /rpc` whole-body argument modes (single unnamed `json`/`jsonb`/`bytea`/
`text`/`xml`) use the same gap code. See
[RPC whole-body argument modes](rpc-body-modes.md).

## MySQL SQLSTATE to HTTP status

For a MySQL database error, myrest first matches the MySQL error number in
this table. It then matches the SQLSTATE class. The number rule has priority.

| MySQL error number or SQLSTATE | HTTP status | Meaning |
| --- | --- | --- |
| `1044`, `1045`, `1142`, `1227` | 403 | Access denied |
| `1062`, `1451`, `1452`, `1213` | 409 | Duplicate key, foreign-key conflict, or deadlock |
| `08*` | 503 | Connection error |
| `22*`, `42*` | 400 | Data or syntax error |
| `23*`, `40*` | 409 | Integrity or transaction conflict |
| `28*` | 403 | Authorization error |
| Any other SQLSTATE or non-MySQL error | 500 | Fallback internal error |

The client always gets `MYREST002` and the error envelope for this table and
for the fallback. myrest writes the MySQL error text to the operator log. It
does not send that text to the client.
