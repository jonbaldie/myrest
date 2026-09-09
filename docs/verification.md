# Verification: scenario index, gap list, and smoke set

**Ticket:** [Verification roll-up: scenario index, gap list, and smoke set](https://github.com/jonbaldie/myrest/issues/48)
**Closes:** [Parent spec: myrest PostgREST parity over MySQL 8](https://github.com/jonbaldie/myrest/issues/20) once Implementation done holds on the HTTP seam
**Parity target:** PostgREST v14.16 (see `CONTEXT.md`)

A client author and an operator can read one accurate statement of what myrest
supports, and one command proves it. This page is the Verification roll-up:
method, done rules, the scenario index, the derived **gap list**, and the fixed
cross-area smoke set. Capability-area chapters stay the source of truth for
**parity labels**. See [ADR 0008](adr/0008-normative-verification-approach.md).

## Method

- Prove parity at the HTTP API boundary with rewritten **normative scenarios**.
- Coverage follows the **parity label**:
  - **full match** — one success path. When the labelled item is itself a client-visible error, one refuse path instead.
  - **partial match** — one in-subset success and one outside-subset refuse
  - **not supported** — one stable refuse and no success claim for that behaviour. Refuse is an error envelope, or a stable non-offer when the chapter names no client input path.
- Scenario bodies live in capability-area chapters and in the HTTP acceptance
  tests under `test/acceptance`. This page indexes them by stable `area-nnn` id.

## Done rules

- **Capability area done:** every labelled behaviour in that area has its
  required normative scenario(s).
- **Verification done:** the smoke set passes at the HTTP seam; the gap list
  matches a fresh derivation from chapter Gap list rows; every labelled
  behaviour meets its coverage duty.
- **Implementation done (parent #20):** normative scenarios pass at the HTTP
  seam (`make scenarios`); deferred Representation, OpenAPI, CORS,
  transaction, gap-code, and related edges are labelled under the parity
  decision rule or refused in the derived gap list.

## Cross-area smoke set

| Id | Intent |
| --- | --- |
| `smoke-001` | Anonymous database role ordinary read succeeds |
| `smoke-002` | JWT → database role ordinary read succeeds |
| `smoke-003` | Write with Prefer return=representation succeeds with a representation body |
| `smoke-004` | POST /rpc/... succeeds |
| `smoke-005` | Embed read on a cache relationship succeeds |
| `smoke-006` | Deliberate not-supported path (FTS) stable refuse |

## Run the whole scenario set

`make scenarios` runs the normative scenario packages: `./cmd/myrest`,
`./internal/httpapi`, `./test/acceptance`, and `./internal/verification`.
`make test` runs the full suite.

## Scenario index

<!-- scenario-index:begin -->
| Id | Capability area | Parity label | Outcome |
| --- | --- | --- | --- |
| `auth-001` | auth | full match | success |
| `auth-002` | auth | full match | success |
| `auth-003` | auth | full match | refuse |
| `auth-004` | auth | partial match | success |
| `auth-005` | auth | not supported | refuse |
| `auth-006` | auth | not supported | refuse |
| `auth-007` | auth | not supported | refuse |
| `auth-008` | auth | partial match | refuse |
| `cache-001` | schema-cache | full match | success |
| `cache-002` | schema-cache | full match | refuse |
| `cache-003` | schema-cache | full match | success |
| `cache-004` | schema-cache | not supported | refuse |
| `cache-005` | schema-cache | not supported | refuse |
| `cfg-001` | config | full match | refuse |
| `cfg-002` | config | full match | success |
| `cfg-003` | config | not supported | refuse |
| `read-001` | read | full match | success |
| `read-002` | read | full match | success |
| `read-003` | read | partial match | success |
| `read-004` | read | partial match | refuse |
| `read-005` | read | partial match | success |
| `read-006` | read | partial match | refuse |
| `read-007` | read | not supported | refuse |
| `read-008` | read | not supported | refuse |
| `read-009` | read | not supported | refuse |
| `read-010` | read | full match | success |
| `read-011` | read | full match | refuse |
| `read-012` | read | full match | success |
| `read-013` | read | not supported | refuse |
| `embed-001` | embed | full match | success |
| `embed-002` | embed | full match | success |
| `embed-003` | embed | not supported | refuse |
| `embed-004` | embed | not supported | refuse |
| `write-001` | write | full match | success |
| `write-002` | write | full match | success |
| `write-003` | write | full match | success |
| `write-004` | write | full match | success |
| `write-005` | write | full match | refuse |
| `write-006` | write | full match | success |
| `write-007` | write | full match | success |
| `write-008` | write | partial match | success |
| `write-009` | write | partial match | refuse |
| `write-010` | write | full match | success |
| `write-011` | write | full match | success |
| `write-012` | write | not supported | refuse |
| `write-013` | write | partial match | success |
| `write-014` | write | partial match | refuse |
| `rpc-001` | rpc | full match | success |
| `rpc-002` | rpc | full match | success |
| `rpc-003` | rpc | partial match | success |
| `rpc-004` | rpc | partial match | refuse |
| `rpc-005` | rpc | full match | success |
| `rpc-006` | rpc | not supported | refuse |
| `rpc-007` | rpc | not supported | refuse |
| `rpc-008` | rpc | not supported | refuse |
| `rpc-009` | rpc | not supported | refuse |
| `rpc-010` | rpc | not supported | refuse |
| `repr-001` | representation | full match | success |
| `repr-002` | representation | full match | success |
| `repr-003` | representation | not supported | refuse |
| `repr-004` | representation | full match | success |
| `repr-005` | representation | full match | success |
| `repr-006` | representation | full match | success |
| `repr-007` | representation | not supported | refuse |
| `repr-008` | representation | full match | refuse |
| `repr-009` | representation | not supported | refuse |
| `repr-010` | representation | not supported | refuse |
| `prefer-001` | representation | not supported | refuse |
| `err-001` | errors | full match | refuse |
| `err-002` | errors | full match | refuse |
| `err-003` | errors | full match | refuse |
| `err-004` | errors | partial match | success |
| `err-005` | errors | partial match | refuse |
| `discovery-001` | discovery | partial match | success |
| `discovery-002` | discovery | partial match | success |
| `discovery-003` | discovery | partial match | success |
| `discovery-004` | discovery | not supported | refuse |
| `discovery-005` | discovery | partial match | refuse |
| `discovery-006` | discovery | partial match | refuse |
| `discovery-007` | discovery | partial match | refuse |
| `discovery-008` | discovery | full match | refuse |
| `discovery-009` | discovery | full match | success |
| `discovery-010` | discovery | full match | success |
| `discovery-011` | discovery | full match | success |
| `discovery-012` | discovery | full match | refuse |
| `discovery-013` | discovery | full match | success |
| `discovery-014` | discovery | full match | success |
| `discovery-015` | discovery | full match | success |
| `discovery-016` | discovery | full match | success |
| `discovery-017` | discovery | full match | success |
| `discovery-018` | discovery | full match | success |
| `discovery-019` | discovery | full match | success |
| `cors-001` | cors-proxy | full match | success |
| `cors-002` | cors-proxy | full match | success |
| `cors-003` | cors-proxy | full match | success |
| `cors-004` | cors-proxy | full match | success |
| `cors-005` | cors-proxy | full match | success |
| `cors-006` | cors-proxy | full match | success |
| `tx-001` | transactions | full match | success |
| `tx-002` | transactions | not supported | refuse |
| `tx-003` | transactions | not supported | refuse |
| `smoke-001` | verification | full match | success |
| `smoke-002` | verification | full match | success |
| `smoke-003` | verification | partial match | success |
| `smoke-004` | verification | full match | success |
| `smoke-005` | verification | full match | success |
| `smoke-006` | verification | not supported | refuse |
<!-- scenario-index:end -->

## Gap list (derived)

This table is derived from the `## Gap list rows` section of each capability-area chapter.
Do not edit it by hand. When a chapter label changes, update that chapter and
re-derive; `go test ./internal/verification` fails when this table drifts.

<!-- gap-list:begin -->
| Chapter | Item | Parity label | Scenarios |
| --- | --- | --- | --- |
| aggregates.md | Aggregate inside a one-to-many or many-to-many spread | not supported | read-013 |
| auth.md | Role impersonation identity (`CURRENT_USER`) | partial match | auth-004, auth-008 |
| auth.md | Postgres row-level security | not supported | auth-005 |
| auth.md | Request GUCs / `request.jwt.claims` in SQL | not supported | auth-006 |
| auth.md | Non-Bearer credential schemes | not supported | auth-007 |
| config.md | Config drop-list knobs (in-database config, NOTIFY channel, search_path extras, GUC hoist, plan-media gate, admin listen) | not supported | cache-004 |
| config.md | Live config reload | not supported | cfg-003 |
| discovery.md | OPTIONS method source (grants, not object-kind / view triggers) | partial match | discovery-001, discovery-005 |
| discovery.md | OpenAPI `info` from schema comments | partial match | discovery-002, discovery-006 |
| discovery.md | OpenAPI path verbs from grants vs insertable flags | partial match | discovery-003, discovery-007 |
| discovery.md | OpenAPI parameters, definitions, consumes/produces matrix, examples, externalDocs | not supported | discovery-004 |
| embed.md | Embed with no declared foreign-key path | not supported | embed-003 |
| embed.md | Computed relationship embed | not supported | embed-004 |
| error-contract.md | MySQL SQLSTATE to HTTP published subset and fallback | partial match | err-004, err-005 |
| media-types-and-prefer.md | `application/geo+json` | not supported | repr-007 |
| media-types-and-prefer.md | Plan media types | not supported | repr-007 |
| media-types-and-prefer.md | Custom media type handlers | not supported | repr-007 |
| media-types-and-prefer.md | Unclaimed `Accept` values | not supported | repr-007 |
| media-types-and-prefer.md | `text/csv` request body | not supported | repr-009 |
| media-types-and-prefer.md | `application/x-www-form-urlencoded` request body | not supported | repr-010 |
| media-types-and-prefer.md | Prefer `timezone` | not supported | prefer-001 |
| read-parity-boundaries.md | Text-case subset | partial match | read-003, read-004 |
| read-parity-boundaries.md | JSON path subset | partial match | read-005, read-006 |
| read-parity-boundaries.md | FTS operators | not supported | read-007, smoke-006 |
| read-parity-boundaries.md | Postgres array/range operators | not supported | read-008 |
| read-parity-boundaries.md | Prefer count=planned or estimated | not supported | read-009, repr-003 |
| rpc-body-modes.md | Single unnamed `json` / `jsonb` whole-body argument | not supported | rpc-007 |
| rpc-body-modes.md | Single unnamed `bytea` whole-body argument | not supported | rpc-008 |
| rpc-body-modes.md | Single unnamed `text` whole-body argument | not supported | rpc-009 |
| rpc-body-modes.md | Single unnamed `xml` whole-body argument | not supported | rpc-010 |
| rpc-get.md | GET /rpc read-safe routines only | partial match | rpc-003, rpc-004 |
| rpc-row-set.md | Filter, order, pagination, or embed on scalar or non-tabular RPC results | not supported | rpc-006 |
| schema-cache.md | Postgres NOTIFY reload bus | not supported | cache-004 |
| schema-cache.md | Domains, casts, and row computed-fields | not supported | cache-005 |
| transactions.md | Ordinary read request transactions | not supported | tx-002 |
| transactions.md | Role-level isolation override | not supported | tx-003 |
| transactions.md | Routine-level isolation override | not supported | tx-003 |
| transactions.md | Transaction-scoped request GUCs / `request.jwt.claims` | not supported | auth-006 |
| write.md | Prefer `return=representation` honesty limit | partial match | write-008, write-009, smoke-003 |
| write.md | Embed after write without a cache relationship | not supported | write-012 |
| write.md | Nested JSON in the write body on JSON and non-JSON columns | partial match | write-013, write-014 |
<!-- gap-list:end -->
