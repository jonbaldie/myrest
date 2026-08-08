# Transaction end and isolation

A client knows how myrest bounds a transaction for a write and for an **RPC**
call, and which transaction preferences it honours. This page closes deferred
item 4 of the parent spec for `db-tx-end` and isolation on MySQL 8.

## Request units

| Request | Transaction unit | Parity label |
| --- | --- | --- |
| `POST` / `PATCH` / `PUT` / `DELETE` on a table or view | One `READ COMMITTED` transaction: optional `db-pre-request`, then the write work (including representation re-reads) | **full match** |
| `POST` / `GET` `/rpc/<name>` | One `READ COMMITTED` transaction: optional `db-pre-request`, then the routine call | **full match** |
| Ordinary `GET` / `HEAD` table reads | No request transaction; `db-tx-end` and `Prefer: tx=` do not apply | **not supported** (reads stay outside this claim) |

A failing write or **RPC** rolls back that unit. A successful unit ends by
`db-tx-end` and, when enabled, `Prefer: tx=`.

## `db-tx-end` values

The knob accepts the same value set as the **parity target**. Each value has
one **parity label**:

| Value | Behaviour | Label |
| --- | --- | --- |
| `commit` (default) | Always commit a successful write or **RPC** unit. `Prefer: tx=` is off. | **full match** |
| `commit-allow-override` | Commit unless `Prefer: tx=rollback`. `Prefer: tx=commit` is applied when sent. | **full match** |
| `rollback` | Always roll back a successful write or **RPC** unit. `Prefer: tx=` is off. | **full match** |
| `rollback-allow-override` | Roll back unless `Prefer: tx=commit`. `Prefer: tx=rollback` is applied when sent. | **full match** |

When a Prefer `tx` value is applied, the response sets `Preference-Applied`
with `tx=commit` or `tx=rollback`. Invalid `tx` values refuse under
`handling=strict` with `PGRST122`, and are ignored under lenient handling.

`Prefer: tx=` on a mode that does not allow override stays known (not a strict
error) and is not applied, as the **parity target** does.

## Isolation

| Behaviour | Label | Notes |
| --- | --- | --- |
| Default isolation for write and **RPC** units | **full match** | myrest opens each unit at `READ COMMITTED`, the PostgREST default. |
| Role-level isolation override (`ALTER ROLE ... SET default_transaction_isolation`) | **not supported** | MySQL has no PostgREST-shaped impersonated-role GUC for this. |
| Routine-level isolation override (`SET default_transaction_isolation` on a function) | **not supported** | myrest does not hoist Postgres function GUCs onto the request transaction. |
| Transaction-scoped request GUCs / `request.jwt.claims` | **not supported** | Already refused; see [Authentication](auth.md). |

## Gap list (this area)

**Not supported:** ordinary read request transactions; role-level isolation
override; routine-level isolation override; transaction-scoped request GUCs.

**Partial match:** none for this area. Write and **RPC** transaction end and
default isolation are **full match** under the labels above.
