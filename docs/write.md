# Ordinary write

Ordinary writes create, change, and remove rows on a table or view **resource**.
The HTTP shapes follow **PostgREST** v14.16. This page covers `POST` insert,
`PATCH` by filter, `DELETE` by filter, `PUT` upsert by primary key, the
unbounded-write gate, writes through views, and the write Prefer values claimed
for this area.

## Methods

| Method | Meaning | Grant |
| --- | --- | --- |
| `POST /{table}` | Insert one JSON object or a JSON array of objects | `INSERT` |
| `PATCH /{table}` | Update rows that match the ordinary-read filters | `UPDATE` |
| `DELETE /{table}` | Delete rows that match the ordinary-read filters | `DELETE` |
| `PUT /{table}` | Upsert one row by primary key | `INSERT`; `merge-duplicates` also needs `UPDATE` |

Exposure of a **resource** does not imply every method. A write without the
matching grant is denied with `PGRST205`, the same privilege-filtering rule as
a missing table.

## Views

A view in a configured database is a **resource** under the same exposure rule
as a table: the active **database role** must hold the matching privilege. A
read through a view uses the ordinary read surface. See
[Views as resources](views.md).

A write through a view needs both:

1. the matching write grant on the view, and
2. MySQL `information_schema.VIEWS.IS_UPDATABLE = YES` in the **schema cache**.

When the grant is present but the view is not updatable, myrest refuses the
write with status 400 and `MYREST001` (`The view is not updatable`). It does
not send the write to MySQL. `OPTIONS` and OpenAPI omit write methods on a
non-updatable view even when write grants exist.

## PUT upsert by primary key

`PUT` upserts one row. The query string must name **all and only** the primary
key columns with `eq` filters. The JSON body must be one object, and the
primary key values in the body must match the filters. A `PUT` that fails
those rules is refused with `PGRST105`.

### Prefer resolution (full match)

| Prefer value | Parity label | MySQL statement |
| --- | --- | --- |
| `resolution=merge-duplicates` (default when Prefer resolution is absent) | **full match** | `INSERT ... AS new ON DUPLICATE KEY UPDATE` for non-key columns |
| `resolution=ignore-duplicates` | **full match** | `INSERT IGNORE` |

Any other `resolution` value is refused with `PGRST100`.

MySQL fires `ON DUPLICATE KEY` / `INSERT IGNORE` on any unique key conflict,
not only the primary key. The client contract still requires primary-key
filters, as the parity target does. Operators should treat secondary unique
keys as part of that MySQL conflict surface.

## Prefer: return

| Value | Label | Behaviour |
| --- | --- | --- |
| `return=minimal` (default) | **full match** | Empty body. `POST` → 201. `PATCH` / `DELETE` → 204. `PUT` → 201 on insert, 204 on update/ignore. |
| `return=headers-only` | **full match** | Empty body. `POST` may set `Location` when a single inserted primary key is known. |
| `return=representation` | **partial match** | JSON array of affected rows when myrest can return them honestly. |

Successful responses that honour an explicit write Prefer set
`Preference-Applied` with the applied tokens.

### Representation honesty limit (partial match)

MySQL has no `RETURNING` clause for ordinary DML. myrest re-reads affected
rows inside the write transaction. That is honest only for these shapes:

| Write shape | Honest when | How |
| --- | --- | --- |
| `POST` + `return=representation` | Table has a `PRIMARY KEY` and the role holds `SELECT` | Insert, resolve primary-key values (payload or auto-increment `LastInsertId`), `SELECT` by those keys |
| `PATCH` + `return=representation` | Table has a `PRIMARY KEY` and the role holds `SELECT` | `SELECT` primary keys that match the filter, update, `SELECT` by those keys |
| `DELETE` + `return=representation` | Role holds `SELECT` | `SELECT` matching rows, then delete; primary key not required |

Outside that subset myrest refuses with `MYREST001` and does not write. Typical
refuses:

- `POST` or `PATCH` with `return=representation` on a table with no primary key
- `return=representation` when the active role has no `SELECT` on the table
- `PUT` with `return=representation`

The body never invents column values. If myrest cannot re-read the affected
rows from MySQL, it refuses instead of guessing.

### Embed after write (full match)

With `Prefer: return=representation`, a nested select nests related rows in the
write body when the **relationship** is in the **schema cache**. Nested
filters, order, and page on that representation follow the same embed read
rules as `GET`. See [Embed](embed.md).

When the nested select has no cache relationship, myrest refuses with
`PGRST200` and does not write. myrest never invents a relationship for a write
response. Embed after write without an honest representation body is outside
this claim.

## Prefer: missing, max-affected, handling

| Prefer | Label | Behaviour |
| --- | --- | --- |
| `missing=default` | **full match** | On `POST`, columns omitted from a row use the SQL `DEFAULT` instead of `NULL`. |
| `max-affected=<n>` | **full match** | With `handling=strict`, a `PATCH` or `DELETE` that would change more than `n` rows refuses with `PGRST124` and rolls back. Ignored under lenient handling. |
| `handling=strict` | **full match** | Unknown or invalid Prefer tokens refuse with `PGRST122`. |
| `handling=lenient` (default) | **full match** | Unknown Prefer tokens are ignored. |

## Prefer: tx and transaction bounds

Each ordinary write runs as one request transaction. `db-tx-end` and
`Prefer: tx=commit|rollback` end that unit. See
[Transaction end and isolation](transactions.md).

## Unbounded-write gate

A `PATCH` or `DELETE` with no filter and no `Prefer: all-rows` is refused with
`PGRST100`. The gate matches the safety intent of the parity-target
pg-safeupdate path: an unbounded write must be intentional.

Clients unlock an all-rows write with either:

- an ordinary-read filter (including one that matches every row), or
- `Prefer: all-rows`

`Prefer: all-rows` is the explicit all-rows preference named by the parent
spec. Stock PostgREST v14.16 has no Prefer for this unlock; myrest names it so
the gate stays honest on MySQL.

## JSON values in the write body

A write body holds JSON column values as JSON. When the target column holds
JSON (`DATA_TYPE = json` in the schema cache), a nested JSON object or array
in a `POST`, `PATCH`, or `PUT` body stores as that JSON, and the ordinary
write success answers. Read-back of the stored value follows the JSON value
shapes of the read surface; see [Read parity boundaries](read-parity-boundaries.md).

Scalar, string, and null values bind as the body sent them. A nested JSON
object or array for a column that does not hold JSON refuses with status 400
and `MYREST001`. myrest does not send that write to MySQL. The parity target
surfaces the same refusal as a PostgreSQL datatype error.

## Filters

`PATCH` and `DELETE` use the same filter surface as ordinary read. See
[Ordinary read](ordinary-read.md).

## Full match rows

| Item | Parity label | Scenarios | Client-visible error |
| --- | --- | --- | --- |
| POST insert, including bulk | full match | write-001 | |
| PATCH by filter | full match | write-002 | |
| DELETE by filter | full match | write-003 | |
| PUT upsert by primary key and resolution | full match | write-004 | |
| Unbounded write safety gate | full match | write-005 | yes |
| Writes through an updatable view | full match | write-006 | |
| Prefer return=minimal and return=headers-only | full match | write-007 | |
| Prefer missing, max-affected, and handling | full match | write-010 | |
| Embed after write with an honest representation | full match | write-011 | |

## Gap list rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| Prefer `return=representation` honesty limit | partial match | write-008, write-009, smoke-003 |
| Embed after write without a cache relationship | not supported | write-012 |
| Nested JSON in the write body on JSON and non-JSON columns | partial match | write-013, write-014 |
