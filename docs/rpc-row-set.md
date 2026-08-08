# RPC row-set results

A client may put ordinary-read filter, order, pagination, and embed on an
**RPC** result when that result is a **row set**. The same features on a scalar
or other non-tabular result refuse stably (`rpc-005`, `rpc-006`).

See [ADR 0006](adr/0006-write-and-rpc-parity-boundaries.md).

## Which routine result shapes count as a row set

| Result shape | How myrest gets it | Row set? |
| --- | --- | --- |
| Function return value | `SELECT db.fn(args)` | no (scalar) |
| Procedure with no SELECT result set | `CALL` then `OUT` / `INOUT` object | no (stable procedure object) |
| Procedure with one or more SELECT result sets | first result set of `CALL` | **yes** |

Notes:

- MySQL functions do not return tables. A function body is always scalar for
  this surface.
- When a procedure both returns a SELECT result set and has `OUT` / `INOUT`
  parameters, the HTTP body is the row set. The stable procedure object is
  used only when there is no result set.
- When a procedure returns more than one result set, myrest uses the **first**
  result set only.
- An empty first result set (zero rows, with column metadata) is still a row
  set. Clients may filter and page it; the body is `[]`.

## Read features on a row set

On a row-set result, these behave as **full match** with the ordinary-read
surface:

- column filters (`col=eq.value`, and the other full-match operators)
- `order`
- `limit` / `offset`
- `select` projection
- **embed** in `select`, when a **relationship** in the **schema cache** can
  resolve from a table resource whose columns cover the result columns and the
  embed join keys

`Prefer: count=exact` and `db-max-rows` apply the same way as on a table read.

Argument names stay out of the filter surface:

- `POST /rpc/<name>`: named JSON body holds arguments; the query string holds
  read features.
- `GET /rpc/<name>`: query keys that name an `IN` / `INOUT` parameter are
  arguments; the other query keys are read features (and must pass the
  read-safe gate).

## Refusals on scalar and non-tabular results

When the result is not a row set, any of filter, order, pagination, or embed
refuses with HTTP 400, the error envelope, and code `MYREST001`.
