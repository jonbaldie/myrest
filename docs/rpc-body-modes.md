# RPC whole-body argument modes

PostgREST accepts unusual `POST /rpc` bodies that pass the whole request body
as one unnamed argument. myrest does not. Every mode below has exactly one
**parity label**. Named JSON object arguments stay the only accepted `POST`
body shape (**full match**; see `rpc-001`).

MySQL has no unnamed routine parameters, so myrest cannot match these modes
without a silent semantic stretch. Each mode is **not supported** and refuses
with HTTP 400, the error envelope, and code `MYREST001`.

## Sharp list

| Whole-body mode | Client probe | Parity label | Scenario |
| --- | --- | --- | --- |
| Single unnamed `json` / `jsonb` | `Content-Type: application/json` and a non-object JSON body (array, scalar, or `null`) | not supported | `rpc-007` |
| Single unnamed `bytea` | `Content-Type: application/octet-stream` | not supported | `rpc-008` |
| Single unnamed `text` | `Content-Type: text/plain` | not supported | `rpc-009` |
| Single unnamed `xml` | `Content-Type: text/xml` or `application/xml` | not supported | `rpc-010` |

There is no accepted partial-match subset for these modes.

## Stable refusals

| Scenario | Message |
| --- | --- |
| `rpc-007` | `A single unnamed JSON RPC argument is not supported` |
| `rpc-008` | `A single unnamed bytea RPC argument is not supported` |
| `rpc-009` | `A single unnamed text RPC argument is not supported` |
| `rpc-010` | `A single unnamed xml RPC argument is not supported` |

A `Content-Type` may carry parameters (for example `charset`); myrest matches
only the media type before `;`.

See [ADR 0006](adr/0006-write-and-rpc-parity-boundaries.md) and parent deferred
item 7 (unusual RPC whole-body argument modes).
