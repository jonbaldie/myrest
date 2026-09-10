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

A filter value wrapped in double quotes is one literal for every scalar
operator: `name=eq."a,b"` matches the value `a,b`, the way the `in` list
decodes a quoted element. A doubled quote inside the value is an escaped
quote, so `name=eq."say ""hi"""` matches `say "hi"`. A quoted value that
does not close, or that carries text after its closing quote, refuses as
`PGRST100`.

Also full match on this path:

- `not.` before an operator
- top-level `and=(...)` and `or=(...)` groups (including nested groups)
- column `select` (with optional `alias:column`)
- `order=column.asc|desc`
- `limit` / `offset` pagination and `Content-Range`
- `Range` request header pagination with `Range-Unit: items`
- `HEAD` with the same read intent and no body
- `Prefer: count=exact` with an exact total in `Content-Range`
- `db-max-rows` as a hard row cap

## The Range request header

A read answers an inclusive item window given in the request headers. `Range:
3-7` with `Range-Unit: items` reads the same page as `?offset=3&limit=5`, and
gives the same rows, status, and `Content-Range`. An open-ended `Range: 5-`
skips the first five matching rows and does not cap the page. A missing
`Range-Unit` means `items`.

`limit` or `offset` in the query string owns the page: myrest keeps the query
string page and ignores the `Range` header for that request.

A `Range` header that myrest cannot use refuses as `PGRST100` and reads no
rows: a range that is not `start-end` or `start-`, a range whose end is less
than its start, and a range unit other than `items`.

A method stays available only when the active **database role** holds the
matching grant. Exposure of a **resource** does not imply every method; a
table without `SELECT` is not a usable resource (`PGRST205`).

## Related partial matches and refusals

Ticket [#28](https://github.com/jonbaldie/myrest/issues/28) owns text-case and
JSON path subsets, and the refusals for FTS, array/range operators, and
`count=planned` / `count=estimated`. See
[Read parity boundaries](read-parity-boundaries.md).

Ticket [#31](https://github.com/jonbaldie/myrest/issues/31) owns aggregates
(`sum` / `count` / `avg` / `min` / `max`) behind `db-aggregates-enabled`. See
[Aggregates](aggregates.md).

## Full match rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| Ordinary read select, filter, order, page, and HEAD | full match | read-001 |
| Exact count | full match | read-002 |
