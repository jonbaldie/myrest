# Exploratory testing report: profiles, schema reload, and the row cap

**Date:** 2026-10-10  
**Build:** `d856737` (`origin/main`), `go build -o bin/myrest ./cmd/myrest`  
**Parity target:** PostgREST v14.16  
**Evidence:** [`2026-10-10-profiles-reload-limits/`](2026-10-10-profiles-reload-limits/)  
**Previous pass:** [2026-10-03 aggregates, media types, and auth](2026-10-03-aggregates-media-auth.md)

## 1. Setup and starting state

- MySQL 8.0.46 in Docker (container `et-myrest-20261010`, host port 32820), using the same image and flags as `internal/mysqltest`.
- Fixtures: `testdata/fixtures/schema.sql`, loaded with `MYREST_MYSQL_HARNESS_PORT=32820` through [`reload.go`](2026-10-10-profiles-reload-limits/reload.go). The `mysql` client cannot load the procedure bodies; the Go harness can.
- Configuration for journeys 1 and 2: [`myrest.conf`](2026-10-10-profiles-reload-limits/myrest.conf) (`db-schemas = "myrest_fixture, myrest_hidden"`, `db-anon-role = "myrest_anon"`, `db-tx-end = "commit-allow-override"`, `jwt-secret`, `openapi-security-active = true`, `openapi-server-proxy-uri = "https://api.example.com/rest"`). Listen address `127.0.0.1:3111`.
- Journey 2 hook variation: [`myrest-prerequest.conf`](2026-10-10-profiles-reload-limits/myrest-prerequest.conf), then `MYREST_DB_PRE_REQUEST=myrest_fixture.before_request_fail` over `myrest.conf`.
- Journey 3: [`myrest-maxrows.conf`](2026-10-10-profiles-reload-limits/myrest-maxrows.conf) (`db-max-rows = 1`).
- Driver: `curl` through [`req.sh`](2026-10-10-profiles-reload-limits/req.sh). Requests and responses are in [`log.txt`](2026-10-10-profiles-reload-limits/log.txt). The service log is [`server.log`](2026-10-10-profiles-reload-limits/server.log).
- Replay: [`replay.sh`](2026-10-10-profiles-reload-limits/replay.sh) reloads the fixture and replays every confirmed failure. Outputs: [`replay-1.txt`](2026-10-10-profiles-reload-limits/replay-1.txt), [`replay-2.txt`](2026-10-10-profiles-reload-limits/replay-2.txt).

### Environment interventions

1. The pass created `myrest_hidden.items` (SELECT only) to give the two databases a table of the same name.
2. The pass created `myrest_fixture.pay_slips` and granted `SELECT (id, employee)` to `myrest_anon`.
3. The pass granted `UPDATE` and `DELETE` on `myrest_hidden.outside_items`, added `items.sku`, created `notes`, and revoked `SELECT` on `notes`, each followed by `SIGUSR1` when the check needed a fresh schema cache.
4. `curl -I` duplicates header lines when combined with `-D -`. Those checks were repeated with `-D -` only.

## 2. Journeys

### Journey 1: Use two configured databases

**Goal:** Read and write the default database without a profile header, then read and write the second database with `Accept-Profile` and `Content-Profile`, and see that each change stays in the selected database (`docs/adr/0011`, `docs/media-types-and-prefer.md`).

Ordinary path, as documented:

- `GET /items` without a profile returns the fixture rows. `GET /outside_items` without a profile returns 404 `PGRST205` for `myrest_fixture.outside_items`.
- `GET /outside_items` with `Accept-Profile: myrest_hidden` returns `[{"id":1,"name":"hidden"}]`.
- `Content-Profile` on `GET` does not select the database. `Accept-Profile` on `POST` does not select it.
- A profile outside `db-schemas` returns 406 `PGRST106` and names both databases.
- `POST`, `PATCH`, `PUT`, and `DELETE` with `Content-Profile: myrest_hidden` change only that database after the role holds the matching grant. `Prefer: tx=rollback` returns the representation and leaves no row.
- `GET /rpc/add_them` uses `Accept-Profile`. `POST /rpc/add_them` uses `Content-Profile`. The other header is ignored, as the parity target does.
- `OPTIONS` follows the selected database and the grants. Hidden `outside_items` advertised `PUT` with `INSERT` and no `UPDATE`, then advertised `PATCH` and `DELETE` after those grants and `SIGUSR1`.
- OpenAPI `host` is `api.example.com`, `basePath` is `/rest`, and `securityDefinitions.JWT` is present.
- CORS preflight for `http://shop.example` echoes `accept-profile` and `content-profile`. A refused origin gets no CORS headers. The read still succeeds.

Variations and findings:

- Successful responses never set `Content-Profile` (finding A, [#235](https://github.com/jonbaldie/myrest/issues/235)).
- `GET /` ignores `Accept-Profile` and merges every configured database. A same-named table overwrites the other path (finding B, [#234](https://github.com/jonbaldie/myrest/issues/234)).
- A quoted profile (`"myrest_hidden"`) and a different case (`Myrest_Hidden`) return 406 `PGRST106`. A trailing space is trimmed and selects the database.
- A JWT role without a grant on `myrest_hidden.outside_items` gets 404. The anonymous role still cannot read `secrets`.

### Journey 2: Change the schema while the API is running

**Goal:** Add a column and a table, reload the schema cache with `SIGUSR1`, and use the new shape. A configured `db-pre-request` hook runs before the request and shares the write transaction (`docs/schema-cache.md`, `docs/auth.md`).

Ordinary path, as documented:

- `GET /items?select=id,sku` returns 400 `PGRST204` before reload and the new values after `SIGUSR1`. `POST` can set `sku`.
- A new granted table is 404 before reload and readable after reload. `REVOKE SELECT` makes the next read 403 `MYREST002` until reload, then 404 `PGRST205`.
- Forty reads during `SIGUSR1` all returned 200.
- `db-pre-request = myrest_fixture.before_request` inserts a hook row on `GET` and on `POST`. `Prefer: tx=rollback` rolls the hook row back with the insert. A hook that signals returns 500 `MYREST002` and does not insert.
- An updatable view accepts `POST` without `return=representation`. `items_stats` refuses with `The view is not updatable`. A generated column in the body refuses with `Cannot insert into generated column name_len` and does not write.
- Unicode insert and filter, quoted `in` lists, `not.eq`, and a 30-digit decimal amount round-trip.

Variations and findings:

- A table with only `GRANT SELECT (id, employee)` is not a resource (finding C, [#236](https://github.com/jonbaldie/myrest/issues/236)). MySQL returns the granted columns after `SET ROLE myrest_anon`.

### Journey 3: Page under `db-max-rows = 1`

**Goal:** A read returns at most one row, a lower client limit still wins, and a write is not capped (`docs/ordinary-read.md`).

Ordinary path, as documented:

- `GET /items?order=id.asc` and `limit=10` each return one row, `Content-Range: 0-0/*`, status 200.
- `offset=1` returns the second row, `Content-Range: 1-1/*`.
- `Prefer: count=exact` returns 206 and `Content-Range: 0-0/7`.
- `Range: 0-9` is capped to one row with status 200.
- `limit=0` returns `[]` and `Content-Range: */*`.
- CSV, `HEAD`, RPC `list_items`, and `Accept-Profile: myrest_hidden` use the same cap.
- `POST` of two rows returns both rows. `select=count()` returns the real total.
- An embed still returns every related order for the one parent row. PostgREST v14.16 `QueryLimitedSpec` expects that (`succeeds in getting parent embeds despite the limit`).

No new bug in this journey.

## 3. Confirmed bugs

### A. [#235](https://github.com/jonbaldie/myrest/issues/235) Multi-database responses omit `Content-Profile`

- **Impact:** A client cannot read which database answered. A proxy cannot route on `Content-Profile`.
- **Expected:** PostgREST v14.16 sends `Content-Profile` when more than one schema is configured, including when the client sends no profile header. `docs/media-types-and-prefer.md` labels Accept-Profile and Content-Profile as full match.
- **Actual:** Status 200 with no `Content-Profile` on `GET /items` and on `GET /outside_items` with `Accept-Profile: myrest_hidden`.
- **Repeats:** [`replay-1.txt`](2026-10-10-profiles-reload-limits/replay-1.txt) and [`replay-2.txt`](2026-10-10-profiles-reload-limits/replay-2.txt).

### B. [#234](https://github.com/jonbaldie/myrest/issues/234) OpenAPI merges every configured database and ignores `Accept-Profile`

- **Impact:** A generated client sees resources from every database on one path map. A table name that exists in two databases keeps only one method list, and that list can change after reload.
- **Expected:** `GET /` describes the selected database. Without a profile header that is the first database of `db-schemas`. PostgREST v14.16 scopes OpenAPI this way. `OPTIONS` already does.
- **Actual:** `GET /` and `GET /` with `Accept-Profile: myrest_hidden` return the same document. It includes `/outside_items` and `/orders`. After `myrest_hidden.items` exists with `SELECT` only, `/items` was get-only in replay 1 and advertised writes in replay 2. `OPTIONS` stayed `POST,PUT,PATCH,DELETE` for the default database and `GET,HEAD` for the hidden database.
- **Repeats:** both replay files.

### C. [#236](https://github.com/jonbaldie/myrest/issues/236) A column `SELECT` grant hides the table

- **Impact:** Least-privilege column grants do not expose the table. The client gets 404 for columns MySQL allows.
- **Expected:** `GET /pay_slips?select=id,employee` returns the row. The README says MySQL grants decide which resources the client can use.
- **Actual:** 404 `PGRST205` for `myrest_fixture.pay_slips`. `SET ROLE myrest_anon` then `SELECT id, employee` returns `1 ada`.
- **Repeats:** both replay files.

## 4. Candidates rejected with evidence

- `PATCH`, `PUT`, and `DELETE` on `myrest_hidden.outside_items` returned 404 before `UPDATE` and `DELETE` were granted. After the grants and `SIGUSR1`, those methods changed the row and the later read showed the change.
- Embedded orders were not capped by `db-max-rows`. PostgREST v14.16 `QueryLimitedSpec` expects parent embeds to succeed despite that limit.
- `Prefer: timezone=UTC` returned 400 `MYREST001` under `handling=lenient`. That refusal is documented. An unknown `made-up=1` preference was ignored when lenient and refused with `PGRST122` when strict.
- A mutating `db-pre-request` on `GET` committed. `docs/transactions.md` says an ordinary read has no request transaction.
- `curl -I` with `-D -` printed each header twice. `-D -` alone did not.

## 5. Unresolved

- None from this pass.

## 6. Usability observations

1. `Location` after an insert into the second database is `/outside_items?id=eq.N`. A later `GET` of that path without `Accept-Profile` returns 404. Finding A removes the header that would name the database.
2. A failing `db-pre-request` returns `MYREST002` and drops the MySQL `SIGNAL` text (`pre-request refused`). The insert does not remain.
3. OpenAPI lists `/outside_items` for a client that has not sent `Accept-Profile`. That call returns 404. This is part of finding B.

## 7. Not explored

- `openapi-mode=disabled`, `openapi-mode=ignore-privileges`, and `db-root-spec`.
- `db-tx-end=rollback` as the process setting. This pass used `commit-allow-override` and `Prefer: tx=rollback`.
- JWT audience, base64 secrets, and asymmetric keys.
- Pool exhaustion under sustained concurrency.

## 8. Cleanup

The myrest process, container `et-myrest-20261010`, and its anonymous volume were removed at the end of the pass. The `mysql:8.0` image was already on the host and was left in place.
