# Error contract

myrest sends every failure as a JSON object with these fields:

```json
{"code":"MYREST002","message":"The database did not answer the read","details":null,"hint":null}
```

`code` and `message` are text. `details` and `hint` are `null` until a
documented failure gives them a value.

## Code catalog

myrest uses a `PGRST*` code only when the failure has the same PostgREST
meaning. For example, `PGRST205` means that a resource is not in the schema
cache, and `PGRST301` means that no anonymous database role is configured.

The myrest gap codes are:

| Code | Meaning |
| --- | --- |
| `MYREST001` | The request needs PostgreSQL semantics that MySQL does not provide. |
| `MYREST002` | MySQL returned a database error. myrest cannot claim a PostgreSQL SQLSTATE for it. |

`MYREST001` is for a documented MySQL-gap refusal. `MYREST002` is for a MySQL
database error. Both codes have the error envelope.

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
