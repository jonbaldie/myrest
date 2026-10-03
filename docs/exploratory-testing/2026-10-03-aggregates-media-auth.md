# Exploratory testing report: aggregates, response media types, write preferences, and auth

**Date:** 2026-10-03  
**Build:** `2144c90` (v0.1.16), `make build`  
**Parity target:** PostgREST v14.16  
**Evidence:** [`2026-10-03-aggregates-media-auth/`](2026-10-03-aggregates-media-auth/)  
**Previous pass:** [2026-09-26 RPC and writes report](2026-09-26-rpc-writes.md)

## 1. Setup and starting state

- MySQL 8.0.46 in Docker (container `et-myrest-mysql`, host port 43306), using the same image and flags as `internal/mysqltest`.
- Fixtures: `testdata/fixtures/schema.sql`, loaded via `MYREST_MYSQL_HARNESS_PORT=43306 go run ./cmd/mysqlharness ./testdata/fixtures/schema.sql`.
- Configuration: [`myrest.conf`](2026-10-03-aggregates-media-auth/myrest.conf) (authenticator login, `db-schemas = "myrest_fixture"`, `db-anon-role = "myrest_anon"`, `db-aggregates-enabled = true`, `db-tx-end = "commit-allow-override"`, `jwt-secret`, `server-cors-allowed-origins = "http://example.com,http://allowed.local"`). Listen address `127.0.0.1:3111`.
- Driver: `curl` through [`req.sh`](2026-10-03-aggregates-media-auth/req.sh). All requests and responses are recorded in [`log.txt`](2026-10-03-aggregates-media-auth/log.txt).
- Replay: [`reset.sh`](2026-10-03-aggregates-media-auth/reset.sh) reloads fixtures and restarts the service; [`replay.sh`](2026-10-03-aggregates-media-auth/replay.sh) replays the reproducer. Replay outputs: [`replay-1.txt`](2026-10-03-aggregates-media-auth/replay-1.txt), [`replay-2.txt`](2026-10-03-aggregates-media-auth/replay-2.txt).

### Environment interventions

1. `curl -X HEAD` hangs waiting for a response body; `curl -s -I` was used instead for HEAD request verifications.
2. For Journey 3 schema cache reload verification, the pass granted `EXECUTE ON FUNCTION myrest_fixture.secret_count TO 'myrest_user'` as root, sent `SIGUSR1`, verified exposure, and then revoked the grant and reloaded the cache again.

## 2. Journeys

### Journey 1: Aggregates on table and view reads

**Goal:** Query aggregated data across exposed database relations (tables and views), including count, sum, min, max, avg, automatic grouping, aliases, filtering on group columns, ordering, pagination, combining aggregates with embeds, and verifying gate refusal when disabled (`docs/aggregates.md`, `docs/views.md`).

Ordinary path, as documented:
- `GET /items?select=count()` → `[{"count":2}]`.
- `GET /orders?select=item_id,count()&order=item_id.asc` → `[{"item_id":1,"count":2},{"item_id":2,"count":1}]`.
- `GET /stock_moves?select=tenant_id,sku,qty.sum(),qty.avg(),qty.min(),qty.max(),count()` returns computed aggregate metrics with automatic grouping by `(tenant_id, sku)`.
- Ordering by group column: `GET /orders?select=item_id,count()&order=item_id.desc` returns ordered rows.
- Filtering by group column: `GET /orders?select=item_id,count()&item_id=eq.1` returns matching grouped row.
- Grouped aggregates with `Prefer: count=exact`: `GET /stock_moves?select=sku,qty.sum()&order=sku.asc` returns `Content-Range: 0-1/2`.
- Pagination on grouped aggregates: `limit=1&offset=1` returns status 206 with `Content-Range: 1-1/2`.
- Singular object representation: `GET /items?select=count()` with `Accept: application/vnd.pgrst.object+json` returns 200 `{"count":2}`.
- CSV export: `GET /items?select=count()` with `Accept: text/csv` returns CSV headers and value `2`.
- Aggregate on view: `GET /items_view?select=count()` returns 200 `[{"count":2}]`; reading pre-aggregated view `GET /items_stats` returns `[{"total":2}]`.
- Aggregates inside one-to-many embeds: `GET /items?select=name,orders(count())&order=name.asc` returns `[{"name":"alpha","orders":[{"count":2}]},{"name":"beta","orders":[{"count":1}]}]`.
- Aggregates grouped by embed: `GET /orders?select=items(name),count()` returns `[{"count":2,"items":{"name":"alpha"}},{"count":1,"items":{"name":"beta"}}]`.
- Aggregates inside spread embed: `GET /items?select=name,...orders(count())` returns 400 `PGRST127` as documented.
- Gate when disabled: starting with `MYREST_DB_AGGREGATES_ENABLED=false` refuses all aggregate queries with 400 `PGRST123` "Use of aggregate functions is not allowed".

Variations and findings:
- An aggregate function inside a many-to-many embed (`tags(count())`, `items(id.sum())`) does not aggregate related rows for the parent, but computes aggregates per target row and returns an unaggregated array (finding A, [#220](https://github.com/jonbaldie/myrest/issues/220)).
- Ordering by an aggregate expression or alias (`order=count().desc` or `order=orders_count.asc`) returns 400 `PGRST204` "column not found" as documented ("Filters, order on group columns, and limit/offset stay available on this path").

### Journey 2: Response media types and write preferences

**Goal:** Request various response media types and apply write preferences across insert, update, upsert, and delete operations while observing headers and lasting database effects (`docs/media-types-and-prefer.md`, `docs/write.md`).

Ordinary path, as documented:
- Media types: `application/vnd.pgrst.array+json` and bare `application/vnd.pgrst.array` return full JSON array with array Content-Type.
- Singular object `application/vnd.pgrst.object+json`: returns 200 with object for 1 row; returns 406 `PGRST116` for 0 rows or 2+ rows.
- CSV export: `text/csv` properly quotes and escapes commas, quotes (`say "hi"` -> `"say ""hi"""`), and outputs empty strings for SQL NULLs.
- CSV on empty results: returns only header line when `select` names columns; returns empty body when `select` is omitted.
- Refused Accept types: `application/geo+json`, `application/vnd.pgrst.plan`, `application/xml` all return 415 `PGRST107`.
- Refused request bodies: `text/csv` and `application/x-www-form-urlencoded` write bodies return 400 `PGRST102`.
- Prefer `timezone=...` returns 400 `MYREST001` as documented.
- Prefer `handling=strict` with unknown preference returns 400 `PGRST122`; with `handling=lenient` unknown preference is ignored.
- Prefer `return=headers-only`: returns 201 with empty body, `Preference-Applied: return=headers-only`, and `Location` header.
- Prefer `missing=default`: bulk insert with omitted non-null column with default value succeeds and applies defaults; without header fails with 409 `MYREST002`.
- Prefer `max-affected=1,handling=strict`: refuses write affecting 2 rows with 400 `PGRST124` and leaves data untouched.
- Prefer `resolution=ignore-duplicates` and `resolution=merge-duplicates`: ignore and update existing keys appropriately on POST.
- PUT upsert by primary key: 201 on insert, 204 on update; non-PK filter or body key mismatch returns 400 `PGRST105`.
- Prefer `tx=rollback`: mutations return 201/204 with `Preference-Applied: tx=rollback` and leave database tables unchanged.
- Singular object Accept on multi-row PATCH: returns 406 `PGRST116` and rolls back all mutations.
- Unbounded writes: `DELETE` and `PATCH` without filters return 400 `PGRST100`; `Prefer: all-rows` permits the operation.
- Table without primary key (`loose_notes`): `POST` and `PATCH` with `Prefer: return=representation` return 400 `MYREST001`; `DELETE` with `return=representation` succeeds.

No new bugs found in Journey 2.

### Journey 3: Authentication, role authorization, and CORS

**Goal:** Authenticate requests with Bearer JWTs, switch database roles, verify grant enforcement and schema cache reload, and verify CORS policies (`docs/auth.md`, `docs/cors-and-proxy.md`).

Ordinary path, as documented:
- Protected table `/secrets` and view `/locked_view` return 404 `PGRST205` for anonymous requests.
- Valid Bearer JWT with `role: myrest_user` successfully accesses `/secrets` and `/locked_view`.
- Malformed JWT, signature mismatch, and empty Bearer token return 401 `PGRST301`.
- Expired token (`exp`) and not-yet-valid token (`nbf`) return 401 `PGRST303`.
- Non-Bearer scheme (`Authorization: Basic ...`) returns 400 `MYREST001`.
- Refusal probes `Prefer: row-security` and `Prefer: jwt-claims` return 400 `MYREST001`.
- CORS allowed origin (`http://example.com`): reflects in `Access-Control-Allow-Origin`, includes `Access-Control-Allow-Credentials: true` and exposed headers.
- CORS disallowed origin (`http://evil.com`): omits `Access-Control-Allow-Origin`.
- CORS preflight `OPTIONS`: allowed origin returns 200 with `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, `Access-Control-Max-Age: 86400`.
- Discovery `GET /`: returns OpenAPI document with listen host `127.0.0.1:3111`; ignores incoming `X-Forwarded-Host` and `Forwarded` headers.
- Schema cache reload: procedure `secret_count` without grant returns 404 `PGRST202`. Granting `EXECUTE` and sending `SIGUSR1` exposes `secret_count` (returns 200 `99`). Revoking and sending `SIGUSR1` returns 404 again.

No new bugs found in Journey 3.

## 3. Confirmed bugs

### A. [#220](https://github.com/jonbaldie/myrest/issues/220) Many-to-many embeds with aggregate functions do not aggregate across the embedded rows for the parent

- **Impact:** Querying an aggregate inside a many-to-many embed (e.g. `GET /items?select=id,tags(count())` or `GET /tags?select=id,items(id.sum())`) returns an unaggregated array with one object per target row (e.g. `[{"id":1,"tags":[{"count":1},{"count":1}]}]` instead of `[{"id":1,"tags":[{"count":2}]}]`; and `[{"id":1,"items":[{"sum":1},{"sum":2}]}]` instead of `[{"id":1,"items":[{"sum":3}]}]`).
- **Expected:** `docs/aggregates.md` claims "Aggregate inside a nested embed over a cache relationship: full match (`read-012`)". On one-to-many relationships (e.g. `orders(count())`), myrest aggregates correctly (`[{"orders":[{"count":2}]}]`). Many-to-many relationships should likewise aggregate related rows for the parent.
- **Probable cause:** In `internal/httpapi/embed.go`, `nestManyToMany` reads the join table `item_tags`, extracts distinct target keys, and calls `readByKeys` on the target table `tags`. `readByKeys` calls `ensureColumns(embed.target, query, keyColumns)` which injects the target table primary key (`tags.id`) into `query.Columns`. When aggregate functions are present, `readquery` automatically adds `GROUP BY` on all non-aggregate columns, grouping by `tags.id`. Each target row computes an aggregate over itself (`count = 1`, `sum = tags.id`). `groupManyToMany` then maps each separate target row to its parent item(s), producing multiple single-item objects per parent rather than aggregating across joined rows.

## 4. Candidates rejected with evidence

- `curl -X HEAD /items` hung: rejected as driver fault. `curl -X HEAD` waits for a body from the server; `curl -s -I` completed immediately with 200 OK and no body.
- `GET /stock_moves?select=qty.sum()&id=lt.0`: returned `[{"sum":null}]`. As documented in SQL and PostgREST contracts.
- Ordering by aggregate expressions (`order=count().desc` or `order=alias.asc`): returned 400 `PGRST204`. Documented in `docs/aggregates.md` that order is supported on group columns.
- `PUT` with body primary key mismatch: returned 400 `PGRST105`. Documented in `docs/error-contract.md`.

## 5. Unresolved

- None from this pass.

## 6. Usability observations

1. When `GET /rpc/list_items?select=count()` is called, it returns `400 PGRST204 column not found: ` with an empty column name string in the error message. Issue #212 notes this root cause.
2. In CSV responses for queries with embedded resources (`/items?select=name,orders(id)`), the embedded objects are serialized as JSON strings in the column cell. This behaves cleanly with standard CSV parsers.

## 7. Not explored

- Complex recursive view trees.
- JWT asymmetric RSA/ECDSA keys (`jwt-secret` HS256 was tested).
- Long running connections and pool exhaustion under high concurrency.

## 8. Cleanup

The test server process, fixture harness, and MySQL container `et-myrest-mysql` were cleaned up. Scratch files under `/tmp/et-myrest` were preserved in the evidence directory.
