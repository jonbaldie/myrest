# Exploratory testing report: RPC, writes, and schema-cache reload

**Date:** 2026-09-26
**Build:** `288b48f` (v0.1.12, after the #192 RPC refactor), `make build`
**Parity target:** PostgREST v14.16
**Evidence:** [`2026-09-26-rpc-writes/`](2026-09-26-rpc-writes/)
**Previous pass:** [2026-09-19 API report](2026-09-19-api.md)

## 1. Setup and starting state

- MySQL 8.0.46 in Docker (container `et-myrest-mysql`, host port 43306), same
  image, env, and flags as `internal/mysqltest`.
- Fixtures: `testdata/fixtures/schema.sql`, loaded with
  `MYREST_MYSQL_HARNESS_PORT=43306 go run ./cmd/mysqlharness`.
- Config: [`myrest.conf`](2026-09-26-rpc-writes/myrest.conf)
  (authenticator login, `db-schemas = "myrest_fixture"`,
  `db-anon-role = "myrest_anon"`, `db-tx-end = "commit-allow-override"`,
  `jwt-secret`). Listen address `127.0.0.1:3111`.
- Driver: `curl` through [`req.sh`](2026-09-26-rpc-writes/req.sh). Every
  request and response is in [`log.txt`](2026-09-26-rpc-writes/log.txt).
- Replay: [`reset.sh`](2026-09-26-rpc-writes/reset.sh) reloads the fixtures
  and restarts myrest; [`replay.sh`](2026-09-26-rpc-writes/replay.sh) runs every
  confirmed reproducer. Outputs:
  [`replay-1.txt`](2026-09-26-rpc-writes/replay-1.txt),
  [`replay-2.txt`](2026-09-26-rpc-writes/replay-2.txt).

### Environment interventions

1. `make mysql-fixtures` failed: `start MySQL: MySQL not ready: invalid
   connection`. The harness waits 60 s (`readyTimeout` in
   `internal/mysqltest/harness.go`). On this host (many other containers
   running), first MySQL initialisation took longer. Workaround: start the
   same container by hand, wait for `ready for connections`, then load the
   fixtures through `MYREST_MYSQL_HARNESS_PORT`. The product was not changed.
2. The fixtures have no routine with a `JSON` parameter. For finding E, the
   pass added `et_echo_json(IN doc JSON, OUT back JSON)` as root and reloaded
   the schema cache with `SIGUSR1`. The same 500 also occurs on the fixture
   procedure `echo_name` without this addition.
3. For Journey 3, the pass added a table `et_notes` (with a `DECIMAL(30,10)`
   column) and later revoked its grants.

## 2. Journeys

### Journey 1: call routines through `/rpc`

**Goal:** call functions and procedures with arguments, and get correct values,
row sets, and transaction behaviour (`docs/rpc-*.md`, `docs/transactions.md`).

Ordinary path, all as documented:

- `POST /rpc/add_them {"a":2,"b":3}` and `GET /rpc/add_them?a=2&b=3` → `5`.
- `POST /rpc/ping` → `{}`; `echo_name` → `{"dst":...}`; `bump_label` →
  `{"label":"x!"}`.
- Row set `list_items`: filter, order, limit, `select`, embed
  `select=id,orders(id)`, `text/csv`, `Prefer: count=exact` + `Range: 0-0`
  (206, `Content-Range: 0-0/3`).
- `POST /rpc/write_marker` with `Prefer: tx=rollback` → no row; without it →
  row committed. `mark_and_list` 406 rolls back its insert.
- Missing or extra argument, unknown routine, routine without `EXECUTE` → 404
  `PGRST202`. `text/csv` on a scalar → 415 `PGRST107`. Form body → 400.

Variations and findings:

- Large integer arguments lose precision on `POST` (finding A).
- Singular object on a filtered row set is refused (finding B).
- A JSON object or array argument gives 500 (finding E).

### Journey 2: create and change rows

**Goal:** insert, upsert, update, and delete rows and read back what changed
(`docs/write.md`).

Ordinary path, all as documented:

- Bulk `POST` with `resolution=merge-duplicates`; `PUT` insert (201) then
  update (204); `PATCH`/`DELETE` with `return=representation`; `tx=rollback`.
- Empty array `POST` → 400 `PGRST102 Empty JSON array`; `PATCH {}` → 400
  `PGRST102 Empty body`; `text/csv` body → 400 `PGRST102`.

Variations and findings:

- Values above 2^53 and long decimals are changed before the write
  (finding A).
- Bulk insert with mixed explicit and auto-increment keys returns an
  incomplete representation (finding C).
- `PATCH` that changes the primary key returns `[]` (finding D).

### Journey 3: change the database and reload

**Goal:** an operator adds or revokes objects and grants and reloads the schema
cache; the config surface refuses bad config (`docs/schema-cache.md`,
`docs/config.md`).

- New table and procedure: 404 before `SIGUSR1`, exposed after; log line
  `reloaded the schema cache`. Revoke: 403 `MYREST002` before reload (stale
  cache), 404 `PGRST205` and absent from `GET /` after reload. As documented.
- Incomplete config → exit 1 with the serve-gate message. `db-channel` in the
  file → exit 1, `config file line 4: db-channel is not a myrest knob`.
  `MYREST_DB_ANON_ROLE` overrides the file value
  ([`env-override.log`](2026-09-26-rpc-writes/env-override.log)).

No findings in Journey 3.

## 3. Confirmed bugs

Each was replayed twice from a fresh fixture load (`replay-1.txt`,
`replay-2.txt`) in addition to the first observation in `log.txt`.

### A. [#197](https://github.com/jonbaldie/myrest/issues/197) JSON numbers lose precision on writes and `POST /rpc`

- **Impact:** silent data change. `{"id":9007199254740993}` stores
  `9007199254740992`; `DECIMAL` `12345678901234567890.1234567891` stores
  `12345678901234567000.0000000000`. `POST /rpc/add_them` returns
  `9007199254740992`; `GET` returns the correct `9007199254740993`.
- **Expected:** "Scalar, string, and null values bind as the body sent them"
  (`docs/write.md`).
- **Probable cause:** `json.Unmarshal` into `any` without `UseNumber`
  (`internal/httpapi/write.go`, `internal/httpapi/rpc.go:438`).

### B. [#198](https://github.com/jonbaldie/myrest/issues/198) Singular object on an RPC row set counts rows before filters

- **Impact:** `GET /rpc/list_items?id=eq.1` with
  `Accept: application/vnd.pgrst.object+json` → 406 `PGRST116`, "The result
  contains 2 rows". Without the Accept, the body has one row. The same request
  on `/items` returns 200.
- **Expected:** full match with ordinary read (`docs/rpc-row-set.md`).
- **Probable cause:** `buildValidator` in `internal/rpcexec/executor.go` counts
  before `shapeRepresentation`. Present before #192 too.

### C. [#199](https://github.com/jonbaldie/myrest/issues/199) Bulk `POST` representation omits rows with mixed keys

- **Impact:** `POST /items [{"name":"x"},{"name":"y","id":50}]` with
  `return=representation` → 201 with only `x`; both rows are stored.
- **Expected:** all rows, or a `MYREST001` refusal ("refuses instead of
  guessing", `docs/write.md`).
- **Probable cause:** `insertedKeys` in `internal/mysqldb/write.go` assumes
  `LastInsertId() + i` for every row.

### D. [#195](https://github.com/jonbaldie/myrest/issues/195) `PATCH` that changes the primary key returns `[]`

- **Impact:** `PATCH /items?id=eq.10 {"id":20}` with `return=representation`
  → 200 `[]`; the row is now id 20.
- **Expected:** the updated row, or a refusal.

### E. [#196](https://github.com/jonbaldie/myrest/issues/196) JSON object or array RPC argument returns 500

- **Impact:** `POST /rpc/et_echo_json {"doc":{"k":1}}` (JSON parameter) and
  `POST /rpc/echo_name {"src":{"k":1}}` (VARCHAR) → 500 `MYREST002`. The
  operator log shows a Go driver conversion error, not a MySQL error. A JSON
  `OUT` value comes back as a JSON string.
- **Expected:** the write-surface rule (#119): store JSON for a JSON parameter,
  400 `MYREST001` for another type.

## 4. Candidates rejected with evidence

- `GET /rpc/add_them?a=x&b=3` → 500 `MYREST002`. MySQL error 1366 has SQLSTATE
  `HY000`, which falls back to 500 in the documented table
  (`docs/error-contract.md`). As documented; see usability note 1.
- Schema cache "did not reload" after the first `SIGUSR1`: the SQL had failed
  (the mysql CLI needed `DELIMITER` for the procedure). After the SQL was
  fixed, reload worked. Driver fault, not a product fault.
- `HEAD /rpc/add_them` hung: `curl -X HEAD` waits for a body. Driver fault;
  HEAD was covered in the previous pass (#129).

## 5. Unresolved

- None from this pass.

## 6. Usability observations

1. A wrong-type argument (`a=x` for a BIGINT) is a client error, but it
   returns 500 because MySQL reports it as `HY000`. Observation only; the
   mapping is documented as a partial match.
2. A missing argument gives 404 "Could not find the function ... in the schema
   cache". This matches PostgREST, but the operator log has the clearer text
   `missing required argument "b"`. A client does not see which argument is
   missing.
3. The MySQL harness gives up after 60 s. On a busy host,
   `make mysql-fixtures` fails before the first start of the container
   finishes. Suggested improvement: a longer or configurable readiness timeout.

## 7. Not explored

- JWT roles on `/rpc` and writes; CORS; aggregates; views (covered on
  2026-09-19).
- Concurrent writes and `innodb_autoinc_lock_mode` interleaving for bulk
  inserts (relevant to C, not tested).
- Schema-cache reload during in-flight requests.

## 8. Cleanup

The myrest process, the fixture harness, and the `et-myrest-mysql` container
were stopped and removed after the pass. Scratch files under `/tmp/et-myrest`
were removed. To replay, start a MySQL 8.0 container on port 43306 with root
password `myrest`, set `EVIDENCE_LOG`, and run `replay.sh` (it expects the
scripts under `/tmp/et-myrest`).
