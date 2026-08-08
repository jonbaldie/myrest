# Ordinary write

Ordinary writes create, change, and remove rows on a table **resource**. The
HTTP shapes follow **PostgREST** v14.16. This ticket covers `POST` insert,
`PATCH` by filter, `DELETE` by filter, `PUT` upsert by primary key, and the
unbounded-write gate. Prefer return values beyond the default, and view
writes, stay in later tickets.

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

## Default response

A successful write uses the default Prefer of the parity target:
`return=minimal`. `POST` answers with status 201 and an empty body. `PATCH`
and `DELETE` answer with status 204 and an empty body. `PUT` answers with
status 201 when MySQL inserts the row, and status 204 when MySQL updates or
ignores an existing row.

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

## Filters

`PATCH` and `DELETE` use the same filter surface as ordinary read. See
[Ordinary read](ordinary-read.md).
