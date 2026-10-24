# Exploratory testing report: embeds, upserts, and configuration

**Date:** 2026-10-24  
**Build:** `098f23f` (v0.1.20, `origin/main`), `go build -o bin/myrest ./cmd/myrest`  
**Parity target:** PostgREST v14.16  
**Evidence:** [`2026-10-24-embeds-upserts-config/`](2026-10-24-embeds-upserts-config/)  
**Previous pass:** [2026-10-10 profiles, schema reload, and the row cap](2026-10-10-profiles-reload-limits.md)

## 1. Setup and starting state

- MySQL 8.0.46 in Docker (container `et-myrest-20261024`, host port 32891, root password `myrest`), with the same image and flags as `internal/mysqltest`.
- Fixtures: `testdata/fixtures/schema.sql`, loaded through [`reload/main.go`](2026-10-24-embeds-upserts-config/reload/main.go) (`go run ./docs/exploratory-testing/2026-10-24-embeds-upserts-config/reload 32891 testdata/fixtures/schema.sql`).
- Configuration: [`myrest.conf`](2026-10-24-embeds-upserts-config/myrest.conf) (authenticator login, `db-schemas = "myrest_fixture"`, `db-anon-role = "myrest_anon"`, `db-aggregates-enabled = true`, `jwt-secret`). Listen address `127.0.0.1:3124` (`MYREST_LISTEN`).
- Journey 3 configurations: [`config/`](2026-10-24-embeds-upserts-config/config/), run one at a time on port 3126 through [`startup.sh`](2026-10-24-embeds-upserts-config/startup.sh). Output: [`startup-log.txt`](2026-10-24-embeds-upserts-config/startup-log.txt). JWT tokens: [`jwt.py`](2026-10-24-embeds-upserts-config/jwt.py).
- Driver: `curl` through [`req.sh`](2026-10-24-embeds-upserts-config/req.sh). Requests and responses are in [`log.txt`](2026-10-24-embeds-upserts-config/log.txt). The service log is [`server.log`](2026-10-24-embeds-upserts-config/server.log).
- Oracle: PostgREST 13.0.4 on Postgres 17 (container `et-pg-20261024`, port 32892) with a copy of the tables `items`, `orders`, `colors`, and `notes`: [`oracle/`](2026-10-24-embeds-upserts-config/oracle/). Output: [`oracle/log.txt`](2026-10-24-embeds-upserts-config/oracle/log.txt).
- Replay: [`replay.sh`](2026-10-24-embeds-upserts-config/replay.sh) reloads the fixtures, adds the `notes` table, starts myrest, and replays each confirmed bug. Outputs: [`replay-1.txt`](2026-10-24-embeds-upserts-config/replay-1.txt), [`replay-2.txt`](2026-10-24-embeds-upserts-config/replay-2.txt). The service logs of the replays are `replay-1.txt.server` and `replay-2.txt.server`.

### Environment interventions

1. The pass created `myrest_fixture.notes` (`id` auto-increment key, `title` and `body` nullable), granted `SELECT, INSERT, UPDATE, DELETE` to `myrest_anon`, and sent `SIGUSR1`. The fixtures have no write table that has only nullable columns.
2. The oracle is PostgREST 13.0.4, not v14.16. Use oracle results only for behaviour that did not change between these versions. Section 5 lists the results that can come from the version gap.
3. The first `replay.sh` run did not set `MYREST_LISTEN`, so myrest listened on port 3000 and the requests got no answer. This was a driver fault. The script was corrected and both replays were run again.

## 2. Journeys

### Journey 1: read rows with embedded resources

**Goal:** read parent rows with related rows, filter and shape the embedded rows, and keep only the parents that have matching related rows (`docs/embed.md`).

Ordinary path, all as documented:

- Many-to-one, one-to-many, and many-to-many embeds; embeds in more than one level; aliases; `select=*,orders(*)`.
- Embed filter, `or`, order, `limit`, and `offset` (`orders.id=eq.3`, `orders.order=id.desc`, `orders.limit=1`).
- Composite foreign-key embeds in both directions; self-referential embeds with a hint (`manager`, `reports`).
- `text/csv` with an embed; embed in a `POST` representation.
- An unknown column in an embed gives 400.

Variations and findings:

- The join type `!inner` or `!left` gives 400 `PGRST200` (finding A).
- A top-level filter on the embed (`orders=not.is.null`) gives 400 `PGRST204` (finding B).

### Journey 2: create, upsert, and change rows

**Goal:** insert one row and bulk rows, upsert with `PUT` and `Prefer: resolution`, and read back what changed (`docs/write.md`).

Ordinary path, all as documented:

- `PUT` insert (201), update, and partial body; `PUT` key mismatch gives 400 `PGRST105`.
- `resolution=merge-duplicates` and `resolution=ignore-duplicates` on bulk `POST`.
- `Prefer: missing=default` on bulk `POST` uses the column default.
- `DELETE` and `PATCH` with `return=representation`.
- Foreign-key violation, duplicate key, value too long, and wrong type give an opaque `MYREST002` (documented, `docs/error-contract.md`).

Variations and findings:

- The `columns` query parameter has no effect (finding C).
- Bulk objects with different keys are not refused (finding D).

### Journey 3: start the service from a config file

**Goal:** an operator writes a config file, starts myrest, and gets a service that answers. A bad config stops the start with a clear message (`docs/config.md`, ADR 0009).

Ordinary path, all as documented:

- A base file starts. An unknown knob is refused with its line number. A bad boolean is refused in the file and in the environment.
- An environment value overrides the file (`MYREST_DB_MAX_ROWS=1` over `db-max-rows = 5`). An empty environment variable does not remove the file value. A start with only environment variables works.
- No anon role and no secret stops the start (serve gate). A missing file path, a bad password, and a bad `MYREST_LISTEN` each exit with 1.
- JWT: a nested `jwt-role-claim-key` (`.app.db_role`) selects the role; an expired token gives 401 `PGRST303`.

Variations and findings:

- A text value without double quotes is used as raw text. Single quotes and an inline comment become part of the value (finding E).

## 3. Confirmed bugs

Each bug was seen first in `log.txt` or `startup-log.txt`. Then it was replayed two times from a fresh fixture load (`replay-1.txt`, `replay-2.txt`). The two replays gave the same responses.

Starting conditions for A to D: fresh `testdata/fixtures/schema.sql`, the `notes` table from intervention 1, and `myrest.conf` on port 3124. Starting conditions for E: loaded fixtures and the files in `config/`.

### A. [#263](https://github.com/jonbaldie/myrest/issues/263) Embed join type `!inner` or `!left` returns `PGRST200`

- **Impact:** a client cannot keep only the parent rows that have matching embedded rows. The message says that the relationship does not exist, which is not the cause.
- **Replay:** `GET /items?select=id,orders!inner(id)&orders.id=eq.3`. Also `select=id,orders!left(id)` and `GET /deliveries?select=id,addresses!deliveries_from!inner(label)`.
- **Expected:** 200 `[{"id":2,"orders":[{"id":3}]}]` (oracle). `docs/research/postgrest-api-surface-and-version-pin.md` lists `!inner` on the target surface. If myrest does not support it, a `MYREST001` refusal and a parity label.
- **Actual:** 400 `PGRST200` "Could not find a relationship between 'items' and 'orders' in the schema cache". The same embed without `!inner` gives 200.
- **Probable cause:** `splitEmbedHead` in `internal/readquery/parse.go` keeps all text after the first `!` as a foreign-key hint.

### B. [#264](https://github.com/jonbaldie/myrest/issues/264) Top-level filter on an embed returns `PGRST204`

- **Impact:** the PostgREST alternative to `!inner` also fails. The message says that a column is missing.
- **Replay:** `GET /items?select=id,orders(id)&orders.id=eq.3&orders=not.is.null`.
- **Expected:** 200 `[{"id":2,"orders":[{"id":3}]}]` (oracle), or a `MYREST001` refusal.
- **Actual:** 400 `PGRST204` "column not found: orders".

### C. [#265](https://github.com/jonbaldie/myrest/issues/265) The `columns` query parameter is silently ignored

- **Impact:** silent data change. A client that limits the written keys with `columns` gets all body keys written. A body key that is not a column gives 400.
- **Replay:** `POST /notes?columns=title` with `[{"title":"g","body":"should-be-ignored"}]` and `return=representation`. Then the same with an `extra` key, and `PATCH /notes?title=eq.g&columns=title`.
- **Expected:** `body` stays null, and `extra` is ignored (oracle, 201). If myrest does not support `columns`, a `MYREST001` refusal (`docs/write.md`: "refuses instead of guessing").
- **Actual:** 201 `[{"id":1,"title":"g","body":"should-be-ignored"}]`. With `extra`: 400 `PGRST204` "column not found: extra". `PATCH` writes `body` too.
- **Probable cause:** `reservedQueryKeys` in `internal/readquery/parse.go` removes `columns` from the filters, and no write code reads it.

### D. [#266](https://github.com/jonbaldie/myrest/issues/266) Bulk `POST` with different object keys is not refused

- **Impact:** a missing key becomes NULL with 201 on a nullable column. On a `NOT NULL DEFAULT` column the client gets an opaque 409 and cannot see that the body shape is the cause. `docs/write.md` labels "POST insert, including bulk" as full match.
- **Replay:** `POST /notes` with `[{"title":"a"},{"body":"z"}]`; `POST /colors` with `[{"name":"pink"},{"name":"grey","tone":"mid"}]`.
- **Expected:** 400 `PGRST102` "All object keys must match", with no write (oracle, both requests).
- **Actual:** `notes`: 201 with NULL in each missing key. `colors`: 409 `MYREST002`; the operator log has MySQL 1048 "Column 'tone' cannot be null". No `colors` row is written.
- **Probable cause:** `insertColumns` in `internal/mysqldb/write.go` uses the union of the keys of all rows.

### E. [#267](https://github.com/jonbaldie/myrest/issues/267) Config text values without double quotes are used as raw text

- **Impact:** with `db-schemas = 'myrest_fixture'` or `db-schemas = myrest_fixture # main db`, myrest starts, logs "listening", and returns 404 `PGRST205` for each table. Single quotes on `db-uri` or `db-anon-role` stop the start with a message that does not name the quotes.
- **Replay:** start myrest with `config/single-quoted-schemas.conf`, `config/bare-inline-comment.conf`, and `config/single-quotes.conf`, then `GET /items?limit=1`.
- **Expected:** ADR 0009: text is double-quoted, and a value of the wrong type is refused with its line number. A quoted value with an inline comment is refused in this way (`config/inline-comment.conf`: "config file line 2: db-schemas: ... is not a quoted value").
- **Actual:** `databases='myrest_fixture'` and `databases=myrest_fixture # main db` in the start log, then 404 "Could not find the table ''myrest_fixture'.items'". With single quotes on all text knobs: "db-anon-role: the database role name holds a character which myrest cannot quote". With single quotes on `db-uri`: a URL parse error.
- **Probable cause:** `parseValue` in `internal/config/file.go` returns a value that does not start with `"` unchanged.

## 4. Candidates rejected with evidence

- Each database error gives `MYREST002` with "The database did not answer the request", also when the database answered (for example a duplicate key). This is the documented error contract (`docs/error-contract.md`). See usability observation 1.
- `text/csv` and form bodies on `POST` are refused. Labelled not supported (`repr-009`, `repr-010`).
- `Prefer: tx=rollback` has no effect with the default `db-tx-end = commit`. Documented in `docs/transactions.md`.
- A database in `db-schemas` that does not exist starts and gives 404. `docs/config.md` says that the start checks do not check that objects exist.
- `MYREST_DB_SCHEMAS='"myrest_fixture"'` keeps the quotes, and each request gets 404. ADR 0009 gives the quote syntax for the file only. No document says that myrest removes quotes from an environment value, so this is not a grounded failure.
- A token role that is not granted to the authenticator (`root`) gives 404 `PGRST205`. ADR 0010 says that such a role cannot be seen by the cache and cannot be activated.

## 5. Unresolved

- `POST` with an empty array gives 400 `PGRST102` "Empty JSON array". The oracle (13.0.4) gives 201 `[]`. The 2026-09-26 pass accepted the myrest result. A v14.16 oracle is necessary to decide.
- `PUT` insert gives 201 in myrest (documented in `docs/write.md`) and 204 in the oracle (13.0.4). The version gap can be the cause.
- `jwt-secret = "short"` (5 characters) is accepted, and a token signed with it is valid. PostgREST refuses a secret shorter than 32 characters. No myrest document gives a minimum length, and this pass did not test the oracle with a short secret.

## 6. Usability observations

1. The `MYREST002` message "The database did not answer the request" is not correct when MySQL answered with an error such as 1048. The client gets no information that it can correct (finding D shows this). Suggested improvement: a message such as "The database refused the request".
2. A quoted value with an inline comment is refused, but a bare value with an inline comment is accepted with the comment in the value (finding E). The two results disagree. Suggested improvement: one rule for comments, written in `docs/config.md`.
3. Finding A and finding B give PostgREST error codes that point to a missing relationship or column. Other gaps in myrest give `MYREST001` with a parity label. A client cannot easily see that these are gaps.

## 7. Not explored

- Spread embeds (`...orders(*)`) with join types; embeds on views; embeds through RPC row sets.
- `Prefer: resolution` with `on_conflict` on a unique key that is not the primary key.
- Concurrent upserts.
- `jwt-aud`, `jwt-secret-is-base64`, and `jwt-cache-max-entries`.

## 8. Cleanup

The myrest and PostgREST processes, the containers `et-myrest-20261024` and `et-pg-20261024` with their volumes, and the `postgres:17-alpine` image (pulled for this pass) were removed after the pass. The `mysql:8.0` image was present before the pass and was kept. To replay, start a MySQL 8.0 container on port 32891 with root password `myrest`, build `bin/myrest`, and run `replay.sh OUT` from the repository root. The script also expects the container name `et-myrest-20261024` for the `notes` table.
