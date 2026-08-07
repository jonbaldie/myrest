# Ordinary write

Ordinary writes create, change, and remove rows on a table **resource**. The
HTTP shapes follow **PostgREST** v14.16. This ticket covers `POST` insert,
`PATCH` by filter, `DELETE` by filter, and the unbounded-write gate. `PUT`
upsert, Prefer return values beyond the default, and view writes stay in
later tickets.

## Methods

| Method | Meaning | Grant |
| --- | --- | --- |
| `POST /{table}` | Insert one JSON object or a JSON array of objects | `INSERT` |
| `PATCH /{table}` | Update rows that match the ordinary-read filters | `UPDATE` |
| `DELETE /{table}` | Delete rows that match the ordinary-read filters | `DELETE` |

Exposure of a **resource** does not imply every method. A write without the
matching grant is denied with `PGRST205`, the same privilege-filtering rule as
a missing table.

## Default response

A successful write uses the default Prefer of the parity target:
`return=minimal`. `POST` answers with status 201 and an empty body. `PATCH`
and `DELETE` answer with status 204 and an empty body.

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
