# Ordinary read

Ordinary list and detail reads of one **resource** use the PostgREST query
string and Prefer shapes claimed as **full match** for this area.

## Full-match filter operators

This ticket claims these filter operators as **full match**:

| Operator | Meaning |
| --- | --- |
| `eq` | equals |
| `neq` | not equal |
| `gt` | greater than |
| `gte` | greater than or equal |
| `lt` | less than |
| `lte` | less than or equal |
| `like` | SQL `LIKE` (`*` in the pattern becomes `%`) |
| `in` | membership in a list, for example `id=in.(1,2)` |
| `is` | `null`, `not_null`, `true`, `false`, or `unknown` |
| `isdistinct` | `IS DISTINCT FROM` |

Also full match on this path:

- `not.` before an operator
- top-level `and=(...)` and `or=(...)` groups (including nested groups)
- column `select` (with optional `alias:column`)
- `order=column.asc|desc`
- `limit` / `offset` pagination and `Content-Range`
- `HEAD` with the same read intent and no body
- `Prefer: count=exact` with an exact total in `Content-Range`
- `db-max-rows` as a hard row cap

A method stays available only when the active **database role** holds the
matching grant. Exposure of a **resource** does not imply every method; a
table without `SELECT` is not a usable resource (`PGRST205`).

## Left for later tickets

Ticket [#28](https://github.com/jonbaldie/myrest/issues/28) owns text-case and
JSON path subsets, and the refusals for FTS, array/range operators, and
`count=planned` / `count=estimated`. Operators outside the full-match list
above are not claimed here.
