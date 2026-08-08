# Views as resources

A MySQL view in a configured database is a **resource** under the same exposure
rule as a table. The active **database role** must hold the relevant privilege
(`SELECT`, `INSERT`, `UPDATE`, or `DELETE`) on the view, of itself or through a
role grant. A view outside `db-schemas`, or a view the role cannot privilege-use,
is not a usable resource (`PGRST205`).

## Read

A read through an exposed view uses the ordinary read surface: column select,
full-match filters, order, page, `HEAD`, and exact count. See
[Ordinary read](ordinary-read.md).

## Write

A write through a view uses the same `POST` / `PATCH` / `DELETE` shapes as a
table write. See [Ordinary write](write.md).

### How myrest decides that a view is writable

myrest does not parse the view definition. At **schema cache** build (and on
explicit reload) it reads MySQL catalog data:

| Source | Field | Meaning |
| --- | --- | --- |
| `information_schema.VIEWS` | `IS_UPDATABLE` | `YES` → the view is a writable relation for method allow-lists and write handlers; any other value → not writable |

A base table is always treated as writable at this layer. Grants still decide
which write method the role may use.

| Case | Client result |
| --- | --- |
| Base table with matching write grant | Write proceeds |
| View with `IS_UPDATABLE = YES` and matching write grant | Write proceeds |
| View with write grant and `IS_UPDATABLE ≠ YES` | Stable refuse: status 400, `MYREST001`, message `The view is not updatable` |
| View or table without the matching write grant | `PGRST205` (not a resource for that method) |

MySQL still enforces grants on the underlying base tables after **role switch**.
A view marked updatable can still fail at runtime when the active role lacks
privilege on a base table; that failure uses `MYREST002`.

## Embed

Relationships still come only from declared foreign keys. A view chain with no
declared foreign key is not supported. See [Embed](embed.md).
