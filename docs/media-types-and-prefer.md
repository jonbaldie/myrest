# Media types and the remaining Prefer values

**Ticket:** [Media types and the remaining Prefer values](https://github.com/jonbaldie/myrest/issues/46)  
**Closes:** deferred item 2 of [Parent spec: myrest PostgREST parity over MySQL 8](https://github.com/jonbaldie/myrest/issues/20)  
**Parity target:** PostgREST v14.16 (from `CONTEXT.md`)

This page is the repository label list for response media types beyond the
locked JSON slice, and for Prefer values outside the locked write and count
slices. Proof lives at the HTTP API boundary.

Locked slices stay where they already live: JSON primary representation
(`repr-001`), profile headers (`repr-002`), Prefer count (`read-002` /
`repr-003`), and write Prefer return / missing / max-affected / handling /
resolution / all-rows (`write-007`–`write-010`). Prefer `tx=` lives in
[Transaction end and isolation](transactions.md).

## Response media types

| Media type | Parity label | Contract | Scenario |
| --- | --- | --- | --- |
| `application/json` | **full match** | Ordinary table/view and row-set **RPC** bodies are a JSON array. | `repr-001`, `repr-004` |
| `application/vnd.pgrst.array+json` (and bare `application/vnd.pgrst.array`) | **full match** | Same JSON array body; `Content-Type` stays the array media type. | `repr-004` |
| `*/*` or no `Accept` | **full match** | Resolves to `application/json` for resource row data. | `repr-004` |
| `application/vnd.pgrst.object+json` (and bare `application/vnd.pgrst.object`) | **full match** | One JSON object when the result has exactly one row; otherwise `PGRST116` and status 406. | `repr-005` |
| `text/csv` | **full match** | CSV with a header row and UTF-8 charset for table/view and row-set bodies. An empty result still emits the header when `select` names columns. | `repr-006` |
| `application/openapi+json` | **full match** | `GET /` discovery only; see [Discovery](discovery.md). | discovery scenarios |
| `application/geo+json` | **not supported** | Refuses with `PGRST107` (no PostGIS / geo handler). | `repr-007` |
| `application/vnd.pgrst.plan` and `application/vnd.pgrst.plan+json` | **not supported** | Refuses with `PGRST107` (plan-media gate is on the config drop list). | `repr-007` |
| Custom media type handlers (Postgres domain / aggregate handlers) | **not supported** | Refuses with `PGRST107`. | `repr-007` |
| Any other `Accept` value | **not supported** | Refuses with `PGRST107` and names the offered types. | `repr-007` |

A claimed media type that myrest does not serve for that path (for example
`text/csv` on a scalar **RPC** body) also refuses with `PGRST107`.

## Request body media types

| Media type | Parity label | Contract |
| --- | --- | --- |
| `application/json` | **full match** | Ordinary writes and named **RPC** arguments. |
| `text/csv` request body | **not supported** | Not claimed. A CSV write body is refused (not valid JSON). |
| `application/x-www-form-urlencoded` | **not supported** | Not claimed. A form write body is refused (not valid JSON). |

## Remaining Prefer values

| Prefer | Parity label | Contract | Scenario |
| --- | --- | --- | --- |
| `tx=commit\|rollback` | **full match** under `db-tx-end` allow-override modes | Ends the write or **RPC** request transaction. See [Transaction end and isolation](transactions.md). | transaction scenarios |
| `timezone=...` | **not supported** | Refuses with `MYREST001`. MySQL session `time_zone` is not the Postgres Prefer timezone GUC contract. | `prefer-001` |

Prefer values already locked elsewhere (`return`, `count`, `resolution`,
`missing`, `max-affected`, `handling`, `all-rows`, and the auth refusals
`row-security` / `jwt-claims`) are not re-labelled here.

## Gap list rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| `application/geo+json` | not supported | repr-007 |
| Plan media types | not supported | repr-007 |
| Custom media type handlers | not supported | repr-007 |
| Unclaimed `Accept` values | not supported | repr-007 |
| Prefer `timezone` | not supported | prefer-001 |

Write Prefer `return=representation` honesty limits stay in
[Ordinary write](write.md).
