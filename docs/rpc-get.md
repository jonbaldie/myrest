# GET /rpc read-safe routines

`GET /rpc/<name>` is a **partial match**. myrest runs the call only when the
routine is **read-safe**. Named query-string keys are the argument names, the
same names a `POST` body would use.

```bash
curl "http://127.0.0.1:3000/rpc/add_them?a=1&b=2"
3
```

## How myrest decides that a routine is read-safe

myrest reads `information_schema.ROUTINES.SQL_DATA_ACCESS` into the **schema
cache** and uses only that catalog value:

| `SQL_DATA_ACCESS` | Read-safe for `GET /rpc` |
| --- | --- |
| `NO SQL` | yes |
| `CONTAINS SQL` | yes |
| `READS SQL DATA` | yes |
| `MODIFIES SQL DATA` | no |
| empty or any other value | no |

This gate is the MySQL substitute for PostgREST function volatility. MySQL has
no `IMMUTABLE` / `STABLE` / `VOLATILE` markers. A routine that MySQL reports as
`MODIFIES SQL DATA` cannot run through `GET`. The same routine may still run
through `POST /rpc/<name>` when the active **database role** holds `EXECUTE`.

A `GET` call to a routine that is not read-safe refuses with HTTP 400, the
error envelope, and code `MYREST001`.

See [ADR 0006](adr/0006-write-and-rpc-parity-boundaries.md) and the parent
scenarios `rpc-003` and `rpc-004`.
