# PostgREST API surface and parity-target pin

**Ticket:** [Inventory PostgREST API surface and recommend version pin](https://github.com/jonbaldie/myrest/issues/2)  
**Researched:** 2026-08-05  
**Primary sources:**

- GitHub Releases API: `https://api.github.com/repos/PostgREST/postgrest/releases/latest` → tag `v14.16` (published 2026-07-27, not prerelease)
- Stable docs site (branded **PostgREST 14**): <https://docs.postgrest.org/en/stable/>
- Reference pages under <https://docs.postgrest.org/en/stable/references/>

## Recommended parity target

**Pin: PostgREST v14.16** (latest stable release at research time).

- Docs “stable” track documents the **14** line (`PostgREST 14` in site metadata).
- Treat **v14.16** as the concrete **parity target** name in the parent spec.
- Normative behaviour = documented HTTP behaviour of this release line, not unreleased `main`.

## What PostgREST exposes as HTTP resources

From [API](https://docs.postgrest.org/en/stable/references/api.html): three database object kinds in an exposed schema become resources:

1. **Tables**
2. **Views**
3. **Functions** (RPC under `/rpc`)

Routes are one level deep (`/people`, `/rpc/add_them`). Nested REST paths are intentionally not provided; related data uses **embed**.

## Capability inventory (normative surface)

### Auth (client contract)

Source: [Authentication](https://docs.postgrest.org/en/stable/references/auth.html)

| Item | Behaviour |
| --- | --- |
| Credentials | `Authorization: Bearer <jwt>` (optional `bearer` casing) |
| Role claim | JWT claim (default key `role`; path configurable via `jwt-role-claim-key` JSPath DSL) |
| Impersonation | On success: `SET LOCAL ROLE <claim>`; on missing JWT/role: anonymous role if `db-anon-role` set |
| JWT crypto | Symmetric HMAC secret or asymmetric JWK/JWKS (`jwt-secret`); optional `kid` selection |
| Claim checks | `exp` / `iat` / `nbf` (30s skew); optional `aud` via `jwt-aud` |
| Cache | JWT verification cache (`jwt-cache-max-entries`) |
| Pre-request | Optional `db-pre-request` function after impersonation |
| GUC claims | SQL reads claims via `current_setting('request.jwt.claims', true)` |

Roles model: **authenticator** (login, limited) + **anonymous** + **user roles** (typically `NOLOGIN`), with `GRANT <user> TO authenticator`.

### Tables and views — read path

Source: [Tables and Views](https://docs.postgrest.org/en/stable/references/api/tables_views.html)

| Area | Surface |
| --- | --- |
| Methods | `GET`, `HEAD` (no body; skips aggregates as optimisation) |
| Horizontal filters | `col=op.value`; multi-param AND by default |
| Operators | `eq gt gte lt lte neq like ilike match imatch in is isdistinct fts plfts phfts wfts cs cd ov sl sr nxr nxl adj not or and all any` |
| Logical | `or=(...)`, `and=(...)`, `not.` prefix; nested logic |
| Modifiers | `like(any).{...}`, `eq(all).{...}`, etc. |
| Vertical select | `select=col1,alias:col2,json->path,json->>path` |
| Ordering | `order=col.asc\|desc` (nulls options exist in full docs) |
| Pagination | `limit` / `offset` query params **or** `Range` / `Range-Unit: items` headers; response `Content-Range` |
| Count | `Prefer: count=exact\|planned\|estimated` |
| JSON columns | Postgres `->` / `->>` path projection and filters |
| Full-text | Postgres FTS family (`fts` / `plfts` / `phfts` / `wfts`) with optional config language |

### Tables and views — write path

Same page + [Prefer Header](https://docs.postgrest.org/en/stable/references/api/preferences.html)

| Method | Role |
| --- | --- |
| `POST` | Insert (bulk JSON array supported) |
| `PATCH` | Update filtered rows |
| `PUT` | Upsert by primary key (constrained) |
| `DELETE` | Delete filtered rows |
| Preferences | `return=minimal\|headers-only\|representation`; `resolution=merge-duplicates\|ignore-duplicates`; `missing=default`; `max-affected`; `handling=strict\|lenient`; `tx=commit\|rollback`; `timezone=...` |

### Embed (resource embedding)

Source: [Resource Embedding](https://docs.postgrest.org/en/stable/references/api/resource_embedding.html)

- Driven by **foreign keys** discovered in the **schema cache** (tables, views, table-valued functions).
- Relationship shapes: many-to-one, one-to-many, many-to-many (join table), one-to-one.
- Syntax in `select`: `directors(id,last_name)`, aliases, `!inner`, disambiguation `!fk_name` / `!column`.
- Nested embed, embedded filters (`actors.order=...`, `roles.character=in.(...)`), embedded limit/offset.
- Top-level filter on embed presence (`actors=not.is.null`), spread embeds, embed on writes with `return=representation`.
- Computed relationships (DB functions) and multiple FK disambiguation.

### RPC

Source: [Functions as RPC](https://docs.postgrest.org/en/stable/references/api/functions.html)

| Item | Behaviour |
| --- | --- |
| Path | `/rpc/<function_name>` |
| POST args | JSON object keys → named args; single unnamed `json`/`jsonb`/`bytea`/`text`/`xml` body modes |
| GET args | Query string (for non-volatile functions under access-mode rules) |
| Table-valued | Same read filters, ordering, embed as tables when return type is table/setof |
| Overloading | Resolved from schema cache; ambiguity → errors (`PGRST203`) |
| Volatility | Affects GET allowance and transaction access mode |
| Stored procedures | **Not supported** (functions only) |

### Schemas / multi-tenant profile

Source: [Schemas](https://docs.postgrest.org/en/stable/references/api/schemas.html)

- Config `db-schemas` list.
- Switch schema: `Accept-Profile` (GET/HEAD), `Content-Profile` (writes); must be in `db-schemas` or `PGRST106`.

### Representation and media types

Source: [Resource Representation](https://docs.postgrest.org/en/stable/references/api/resource_representation.html), [Media Type Handlers](https://docs.postgrest.org/en/stable/references/api/media_type_handlers.html)

- Content negotiation via `Accept` / `Content-Type`.
- Built-ins: `application/json`, `text/csv`, `application/openapi+json`, `application/geo+json`, `application/vnd.pgrst.object(+json)`, `application/vnd.pgrst.array(+json)`, `application/vnd.pgrst.plan`.
- Singular object coercion (`vnd.pgrst.object`) → `PGRST116` if not exactly one row.
- Extensible handlers via Postgres domains + functions/aggregates (Postgres-specific).

### Aggregates

Source: [Aggregate Functions](https://docs.postgrest.org/en/stable/references/api/aggregate_functions.html)

- `avg count max min sum` on `select` (e.g. `amount.sum()`), auto `GROUP BY`.
- Disabled by default (`db-aggregates-enabled`).

### OpenAPI

Source: [OpenAPI](https://docs.postgrest.org/en/stable/references/api/openapi.html)

- Root path serves OpenAPI derived from schema cache + privileges (mode via `openapi-mode`).
- Optional override function `db-root-spec`.
- Comments on DB objects feed descriptions.

### OPTIONS / CORS

Sources: [OPTIONS](https://docs.postgrest.org/en/stable/references/api/options.html), [CORS](https://docs.postgrest.org/en/stable/references/api/cors.html)

- `OPTIONS` advertises methods from object kind / view updatability (triggers), not only live grants.
- CORS permissive by default; restrict with `server-cors-allowed-origins`.

### Errors

Source: [Errors](https://docs.postgrest.org/en/stable/references/errors.html)

- Body shape: `{ code, message, details, hint }` (Postgres-style).
- Postgres SQLSTATE → HTTP status map (e.g. `23505`→409, `42501`→401/403).
- PostgREST codes `PGRSTgxx` groups: connection (0), API request (1), schema cache (2), JWT (3), internal (X).
- Custom raise paths including `PTxyz` and structured `PGRST` SQLSTATE.
- `Proxy-Status` header on errors.

### Transactions / request GUC

Source: [Transactions](https://docs.postgrest.org/en/stable/references/transactions.html)

Per request, after impersonation:

1. `START TRANSACTION` with access mode from HTTP method (+ function volatility for RPC)
2. Isolation default `READ COMMITTED` (overridable via role/function settings)
3. Transaction-scoped settings: `request.headers`, `request.cookies`, `request.jwt.claims`, `request.path`, `request.method`, search_path from schemas
4. Main query
5. End (`COMMIT` / optional `Prefer: tx=rollback`)

GET/HEAD → `READ ONLY`; writes → `READ WRITE`.

### Schema cache (operational, client-visible effects)

Source: [Schema Cache](https://docs.postgrest.org/en/stable/references/schema_cache.html)

- In-memory metadata required for embed, RPC resolution, OPTIONS, OpenAPI.
- Reload: `SIGUSR1`, `NOTIFY pgrst, 'reload schema'`, optional event triggers; debounced; requests wait during reload.

### Configuration knobs touching the HTTP surface

From [Configuration](https://docs.postgrest.org/en/stable/references/configuration.html) parameter list (names matter for parity of ops/docs):

`db-uri`, `db-schemas`, `db-anon-role`, `db-extra-search-path`, `db-pre-request`, `db-max-rows`, `db-aggregates-enabled`, `db-pool*`, `db-tx-end`, `db-channel*`, `db-prepared-statements`, `db-plan-enabled`, `db-root-spec`, `db-hoisted-tx-settings`, `jwt-secret`, `jwt-secret-is-base64`, `jwt-aud`, `jwt-role-claim-key`, `jwt-cache-max-entries`, `openapi-mode`, `openapi-security-active`, `openapi-server-proxy-uri`, `server-host`, `server-port`, `server-unix-socket*`, `server-cors-allowed-origins`, `server-trace-header`, `server-timing-enabled`, `admin-server-*`, `log-level`, `log-query`, `app.settings.*`.

### Postgres-tied features (likely gap candidates for MySQL)

These are documented as first-class on the v14 surface but depend on Postgres facilities:

| Feature | Postgres dependency |
| --- | --- |
| Role impersonation via `SET LOCAL ROLE` + `current_user` | PG roles / `NOINHERIT` |
| RLS policies using `current_user` / GUCs | PG RLS |
| Request GUCs (`request.jwt.claims`, etc.) | `set_config` / `current_setting` |
| Computed fields (functions on composite row types) | PG |
| Domain representations + casts | PG domains/casts |
| Media type handlers via domains/aggregates | PG |
| FTS operators (`fts` family), `tsvector` | PG FTS |
| Range/array operators (`cs cd ov sl sr …`) | PG types/ops |
| `ILIKE`, regex `~` / `~*` | PG operators (MySQL has analogues, not identical) |
| `Prefer: count=planned/estimated` via PG stats | PG planner stats |
| `NOTIFY` schema reload / listener | PG notify |
| Search path / schemas as namespaces | PG schemas (MySQL: databases) |
| Function volatility labels (IMMUTABLE/STABLE/VOLATILE) | PG |
| Stored procedures unsupported; functions only | PG function model |

## Inventory summary for the parent spec

Normative chapters the parent spec should cover (aligned to docs):

1. Auth and role selection  
2. Schema exposure and schema cache  
3. Read (filter, select, order, page, count)  
4. Embed  
5. Write (POST/PATCH/PUT/DELETE + Prefer)  
6. RPC  
7. Representation / media types  
8. Errors  
9. OpenAPI  
10. CORS / OPTIONS  
11. Config surface mapping  
12. Explicit unsupported / MySQL-equivalent list  

## Answer (ticket resolution)

- **Parity target pin:** PostgREST **v14.16** (latest stable; docs stable = 14.x).  
- **Normative inventory:** the capability tables above, sourced from PostgREST 14 stable reference docs.  
- **Immediate implication:** every later parity decision should cite a row in this inventory and classify it under the map’s parity decision rule (equivalent / unsupported / narrowed).
