# Aggregates on read

Clients may request aggregate reads against one **resource** with the PostgREST
select forms. The operator turns the feature on with `db-aggregates-enabled`
(default off). Labels live in the Read **capability area**; see
[ADR 0005](adr/0005-aggregate-query-parity-boundaries.md).

## Full-match aggregate forms

When `db-aggregates-enabled` is true, this ticket claims these forms as
**full match**:

| Form | Meaning |
| --- | --- |
| `column.sum()` | sum of non-null values |
| `column.avg()` | average of non-null values |
| `column.min()` | minimum non-null value |
| `column.max()` | maximum non-null value |
| `column.count()` | count of non-null values |
| `count()` | count of rows |
| `alias:…` on any form above | rename the result key |

Automatic `GROUP BY` applies to every selected column that has no aggregate.
Filters, order on group columns, and limit/offset stay available on this path.

## Gate when off

With aggregates off, any aggregate in `select` (including inside an **embed**)
refuses with `PGRST123` and the message
`Use of aggregate functions is not allowed` (HTTP 400).

## Aggregates and embed

| Shape | Status |
| --- | --- |
| Aggregate inside a nested embed over a cache **relationship** | full match |
| Aggregate grouped by an embedded resource over a cache **relationship** | full match |
| Aggregate inside a one-to-many or many-to-many spread (`...resource(...)`) | not supported (`PGRST127`) |

Spread embeds beyond that refuse path are not part of this ticket.
