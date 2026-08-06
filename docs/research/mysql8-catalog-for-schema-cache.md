# MySQL 8 catalog data for a PostgREST-like schema cache

**Ticket:** [Assess MySQL catalog data for a PostgREST-like schema cache](https://github.com/jonbaldie/myrest/issues/4)  
**Researched:** 2026-08-05  
**Primary sources:**

- PostgREST 14 [Schema Cache](https://docs.postgrest.org/en/stable/references/schema_cache.html), [Resource Embedding](https://docs.postgrest.org/en/stable/references/api/resource_embedding.html), [OPTIONS](https://docs.postgrest.org/en/stable/references/api/options.html), [OpenAPI](https://docs.postgrest.org/en/stable/references/api/openapi.html)
- MySQL 8.0 Reference Manual index of Information Schema tables (live fetch of <https://dev.mysql.com/doc/refman/8.0/en/information-schema.html> link set)
- Live MySQL **8.0.46** (`mysql:8.0` Docker): `information_schema` column inventories and sample FK/routine/view/privilege queries against a miniature `api` schema

> Note: Individual MySQL manual pages for each IS table returned Oracle “Technical Difficulties” during this session; **column lists and query results below are from the running 8.0.46 server**, which is authoritative for on-wire catalog shape.

## What PostgREST’s schema cache must support

From docs, the cache is the in-memory model that enables at least:

| Consumer | Needs from catalog |
| --- | --- |
| Resource list | Tables, views (and exposed functions) in configured schemas, filtered by privileges |
| Vertical/horizontal query build | Columns, nullability, types, PK/unique keys |
| Embed | Foreign key graph (including composite), relationship direction, join-table detection |
| RPC | Function/procedure signatures, arg names/types/modes, return type, volatility/security |
| OPTIONS | Allowed methods from object kind / updatability rules |
| OpenAPI | Same metadata + comments/descriptions |
| Writes | PK columns (PUT/upsert, Location headers), updatable view flags |
| Errors | Stale-cache detection classes (`PGRST200`–`205` analogues) |

Reload semantics (SIGUSR1 / NOTIFY / debounce) are operational; MySQL will need its own reload triggers (no PG `NOTIFY`/`event_trigger`).

## MySQL sources available

All listed tables exist in 8.0.46 `information_schema`:

`SCHEMATA`, `TABLES`, `COLUMNS`, `VIEWS`, `TABLE_CONSTRAINTS`, `KEY_COLUMN_USAGE`, `REFERENTIAL_CONSTRAINTS`, `CHECK_CONSTRAINTS`, `STATISTICS`, `ROUTINES`, `PARAMETERS`, `TRIGGERS`, `TABLE_PRIVILEGES`, `COLUMN_PRIVILEGES`, `SCHEMA_PRIVILEGES`, `ROLE_TABLE_GRANTS`, `ROLE_COLUMN_GRANTS`, `ROLE_ROUTINE_GRANTS`, `APPLICABLE_ROLES`, `ENABLED_ROLES`, `ADMINISTRABLE_ROLE_AUTHORIZATIONS`.

Also relevant outside IS: `mysql` grant tables (if super-user load path), `SHOW` statements (less ideal for bulk cache build).

### Object inventory

| Need | MySQL source | Key columns (8.0.46) | Adequacy |
| --- | --- | --- | --- |
| Schemas/databases | `SCHEMATA` | schema name | **Good** — MySQL “schema” = database |
| Tables | `TABLES` | `TABLE_SCHEMA`, `TABLE_NAME`, `TABLE_TYPE`, `TABLE_COMMENT`, engine, approx `TABLE_ROWS` | **Good** |
| Views | `TABLES` (`VIEW`) + `VIEWS` | `VIEW_DEFINITION`, `IS_UPDATABLE`, `SECURITY_TYPE`, `DEFINER`, `CHECK_OPTION` | **Good** for listing/updatability; definition parsing optional |
| Columns | `COLUMNS` | name, ordinal, nullability, `DATA_TYPE`/`COLUMN_TYPE`, default, extra, `COLUMN_KEY`, comment, generated expr, charset | **Good** |
| Primary / unique keys | `TABLE_CONSTRAINTS` + `KEY_COLUMN_USAGE` / `STATISTICS` | constraint type, column order | **Good** |
| Foreign keys | `KEY_COLUMN_USAGE` (referenced_* set) + `REFERENTIAL_CONSTRAINTS` (match/update/delete rules) | full composite ordinal via `ORDINAL_POSITION` | **Good** for embed graph |
| Indexes (optional) | `STATISTICS` | columns, uniqueness, type | **Good** (perf/planning aids, not required for basic parity) |
| Check constraints | `CHECK_CONSTRAINTS` + `TABLE_CONSTRAINTS` | present in 8.0 | **Good** (errors/docs; not required for routing) |
| Routines | `ROUTINES` | name, `ROUTINE_TYPE` (PROCEDURE/FUNCTION), return `DTD_IDENTIFIER`, `IS_DETERMINISTIC`, `SQL_DATA_ACCESS`, `SECURITY_TYPE`, `DEFINER`, comment | **Good enough** with mapping |
| Parameters | `PARAMETERS` | name, mode IN/OUT/INOUT, ordinal, types; ordinal 0 = function return | **Good** |
| Table privileges | `TABLE_PRIVILEGES`, `ROLE_TABLE_GRANTS` | grantee, privilege type | **Usable** with role expansion |
| Column privileges | `COLUMN_PRIVILEGES`, `ROLE_COLUMN_GRANTS` | per column | **Usable** |
| Routine privileges | `ROLE_ROUTINE_GRANTS` / mysql procs priv | execute grants | **Usable** (load path must be defined) |
| Comments | `TABLE_COMMENT`, `COLUMN_COMMENT`, `ROUTINE_COMMENT` | text | **Good** for OpenAPI descriptions |

### Sample FK discovery (live)

On schema `api` with `films.director_id → directors.id` and `roles_cast.film_id → films.id`:

```text
KEY_COLUMN_USAGE (referenced not null):
  fk_films_director  films.director_id → directors.id
  fk_roles_film      roles_cast.film_id → films.id

REFERENTIAL_CONSTRAINTS:
  rules NO ACTION / NO ACTION for both
```

This is sufficient to build the same relationship directions PostgREST documents (many-to-one / one-to-many; many-to-many when a table’s PK columns are exactly two FKs to other tables — detectable algorithmically).

### Sample routine discovery (live)

```text
ROUTINES:
  film_count FUNCTION  returns int  DETERMINISTIC  SECURITY DEFINER
  add_film   PROCEDURE              SECURITY DEFINER

PARAMETERS:
  add_film 1 IN p_title varchar(200)
  add_film 2 IN p_director_id int
  film_count 0 (return) int
```

## Hard limits vs Postgres system catalogs

| Area | Postgres (PostgREST world) | MySQL 8 limit | Impact on schema cache / API |
| --- | --- | --- | --- |
| Namespace | Schemas inside one database; `search_path` | Database = schema; no search_path multi-schema resolution per session the same way | Map `db-schemas` → MySQL database list; multi-db connections/`USE` policy required |
| Tables/views/cols/FKs | Rich `pg_catalog` | IS tables adequate | **No hard block** for core cache |
| Embed relationships | FKs + view rewriting over base FKs + computed rel functions | FKs yes; **view-base FK inference** requires parsing `VIEW_DEFINITION` or relying only on FKs declared on views (MySQL rarely has FKs on views) | **Hard limit:** embed through views is weaker unless myrest implements SQL/view analysis |
| Functions vs procedures | PostgREST: functions only; volatility IMMUTABLE/STABLE/VOLATILE | Functions **and** procedures; volatility approx via `IS_DETERMINISTIC` + `SQL_DATA_ACCESS` (not the same 3-way marker) | Spec must map RPC to MySQL routines and define GET eligibility rules without PG volatility |
| Overloaded functions | Common in PG; cache disambiguates by arg types | MySQL routines: **no overloading** by signature in the same schema (name unique per type) | **Simplifies** cache vs PG |
| Composite / domain types | First-class; domain representations feature | No PG domains; limited composite story | Domain-representation feature **unsupported** without redesign |
| Computed fields | Functions on table row types | No equivalent row-type functions | Computed-fields feature **unsupported** or needs generated columns / views |
| Cast graph | `pg_cast` drives domain reps / media handlers | No equivalent app-level cast catalog | Media-type handlers / domain reps **unsupported** as specified |
| Privileges in cache | Role-aware exposure | IS privilege tables show grants visible to current user; full cross-role cache may need privileged loader + `ROLE_*` expansion | Design choice: build cache as superuser then filter per request role, or rebuild/filter per role |
| RLS policies | Cached/enforced by PG | None | Nothing to load; row policy not part of cache |
| Comments | `COMMENT ON` rich | Table/column/routine comments yes; less uniform for all object kinds | OpenAPI descriptions partial |
| Estimated counts | PG planner stats | `TABLE_ROWS` in `TABLES` is approximate (engine-dependent) | `count=planned/estimated` needs MySQL-native definition or unsupported |
| Live DDL notify | `NOTIFY` + event triggers | No drop-in; alternatives: poll, binlog/CDC, manual admin reload, MySQL 8.0 optional components | Reload mechanism **must differ** |
| Polymorphism / inheritance | Table inheritance (legacy) | None | N/A |
| Partitioned tables | PG partitions + FKs quirks | MySQL partitioning exists; FK support historically constrained vs non-partitioned | Edge-case embed/write behaviour needs explicit rules later |

## Minimum viable schema cache contents (MySQL)

Enough to serve PostgREST-like routing on MySQL:

1. **Databases** allowed by config.  
2. **Relations:** base tables + views with updatability + comments.  
3. **Columns:** types, nullability, defaults, generated, keys, comments.  
4. **Primary key** column lists (ordered).  
5. **Foreign keys:** ordered column pairs + referenced relation + rules.  
6. **Routines:** type (FUNCTION/PROCEDURE), parameters, return type, security type, determinism/data-access flags, comments.  
7. **Privilege snapshot** sufficient to filter resources for the active database role (or deferred check to MySQL at execution — product decision).  
8. **Cache generation / version** token for stale detection and admin reload.

## Recommended non-goals for first cache (from limits)

- View-chain FK embed without a SQL analyzer  
- Domain representations / media-type handler catalogs  
- Computed fields as PG row functions  
- PG-identical volatility classification  
- Event-trigger auto-reload (replace with explicit reload API + optional poll)

## Answer (ticket resolution)

- MySQL 8.0 **can** supply catalog data for a PostgREST-like **schema cache** covering tables, views, columns, PK/FK graphs, routines/parameters, comments, and privileges via `information_schema` (verified on 8.0.46).
- **Hard limits** vs Postgres: no RLS catalog; weak view-relationship inference; no domain/cast/computed-field catalogs; different routine volatility model; no `NOTIFY` DDL bus; schema = database.
- Core read/write/embed(on base tables)/RPC cache is **feasible**; several PostgREST features are **catalog-impossible** and must fall through the parity decision rule as unsupported or MySQL-native redesigns.
