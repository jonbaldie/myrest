# MySQL 8 auth and privileges vs PostgREST assumptions

**Ticket:** [Compare MySQL 8 auth and privileges to PostgREST assumptions](https://github.com/jonbaldie/myrest/issues/3)  
**Researched:** 2026-08-05  
**Primary sources:**

- PostgREST 14 [Authentication](https://docs.postgrest.org/en/stable/references/auth.html)
- PostgREST 14 [Database Authorization](https://docs.postgrest.org/en/stable/explanations/db_authz.html)
- PostgREST 14 [Transactions](https://docs.postgrest.org/en/stable/references/transactions.html)
- MySQL 8.0 Reference Manual (live fetch): [Using Roles](https://dev.mysql.com/doc/refman/8.0/en/roles.html), [SET ROLE](https://dev.mysql.com/doc/refman/8.0/en/set-role.html), [GRANT](https://dev.mysql.com/doc/refman/8.0/en/grant.html), [Privileges](https://dev.mysql.com/doc/refman/8.0/en/privileges-provided.html), [Partial Revokes](https://dev.mysql.com/doc/refman/8.0/en/partial-revokes.html)
- Live MySQL **8.0.46** container experiments (Docker `mysql:8.0`) exercising `SET ROLE`, grants, and `CURRENT_USER` / `CURRENT_ROLE`

## What PostgREST assumes (Postgres)

1. **Three role kinds:** login **authenticator** (very limited, often `NOINHERIT`), **anonymous** role, and **user** roles (often `NOLOGIN`).
2. **User impersonation per request:** after JWT verification, run `SET LOCAL ROLE <role>` inside the request transaction (or switch to anon).
3. **Privilege enforcement in the database:** table/column/function grants + optional **RLS** policies keyed on `current_user` and transaction GUCs.
4. **JWT is only authentication:** authorization is SQL privileges + RLS, not app ACLs.
5. **Request context in SQL:** `current_setting('request.jwt.claims')`, headers, cookies, path, method as transaction-scoped settings.
6. **Pool-safe reset:** `SET LOCAL` ends with the transaction; next checkout is clean.

## What MySQL 8.0 provides

### Roles

From MySQL docs + experiment:

- Roles are named privilege collections (`CREATE ROLE`).
- Roles are granted to users (or other roles) with `GRANT role TO user`.
- Session activation: `SET ROLE role | ALL | ALL EXCEPT … | DEFAULT | NONE`.
- `CURRENT_ROLE()` shows active roles; `mandatory_roles` / `activate_all_roles_on_login` / `SET DEFAULT ROLE` control defaults.
- Role names use `user@host` form (host defaults to `%`).

### Privileges

- Static privileges include table-level `SELECT/INSERT/UPDATE/DELETE`, column-level variants, `EXECUTE` on routines, `SHOW VIEW`, etc.
- Dynamic privileges exist for admin features (not central to API row access).
- Privilege data lives in `mysql.*` grant tables; also visible via `information_schema` privilege tables and `ROLE_*_GRANTS`.
- **Partial revokes** (8.0.16+, `partial_revokes`): can carve schema exceptions out of global grants — schema-level only, not row-level.

### Session identity (critical difference)

Experiment on 8.0.46 with login user `authenticator_login` granted roles `anonymous` and `webuser`:

| After | `USER()` / `CURRENT_USER()` | `CURRENT_ROLE()` | Effective table privs |
| --- | --- | --- | --- |
| Connect (default roles ALL) | stay `authenticator_login@…` | all granted roles | union of active role privs |
| `SET ROLE 'webuser'` | **unchanged** login identity | `webuser` | webuser privs |
| `SET ROLE 'anonymous'` | **unchanged** | `anonymous` | anonymous privs |
| `SET ROLE NONE` | **unchanged** | `NONE` | only direct user privs |

**MySQL does not change `CURRENT_USER()` when activating roles.**  
Postgres `SET ROLE` / `SET LOCAL ROLE` **does** change the session’s current user identity for privilege checks and `current_user`.

### No row-level security

- MySQL 8.0 has **no** RLS / row security policies comparable to Postgres.
- Closest tools: views, stored routines (`SQL SECURITY DEFINER`/`INVOKER`), application predicates, or external policy engines.
- `partial_revokes` is schema-scoped, not row-scoped.

### PROXY users

- MySQL supports `GRANT PROXY ON target TO proxy_user` (connection-time proxying via auth plugins / client proxy mechanism).
- This is **not** the same operational shape as PostgREST’s mid-request `SET LOCAL ROLE` on a pooled connection.
- Viable as an alternative design (separate OS/DB users per API role, proxy at connect), but heavier for per-request switching on a pool.

### Request-scoped settings

- No built-in equivalent of Postgres `set_config(..., true)` transaction GUCs for arbitrary `request.jwt.claims`.
- Workarounds: user variables (`@jwt_sub`), temp tables, `sys_exec`-style hacks (unsuitable), or keep claim context only in the application and pass as routine args.
- SQL that PostgREST apps write as `current_setting('request.jwt.claims', true)::json->>'email'` **will not port**.

### Connection pooling implications

- `SET ROLE` is **session**-scoped in MySQL, not automatically transaction-local.
- A pool must **reset roles** (e.g. `SET ROLE NONE` or restore default) after every request, or use a fresh connection per role strategy.
- Failure to reset ⇒ privilege bleed across requests (security bug).

## Side-by-side gap matrix

| PostgREST / Postgres assumption | MySQL 8.0 reality | Blocks JWT→database-role parity? |
| --- | --- | --- |
| `SET LOCAL ROLE` impersonation changes `current_user` | `SET ROLE` activates granted roles; `CURRENT_USER` stays login account | **Partial.** Privilege switching works if roles are granted to the authenticator login; identity-based SQL (`current_user`, DEFINER edge cases) does not match |
| Authenticator `NOINHERIT` chameleon | Login user with **no direct table privs**, only `GRANT role TO login`, then `SET ROLE` per request | **No block** if operational discipline matches (verified: insert denied under `anonymous`, allowed under `webuser`) |
| JWT `role` claim names a DB role | Role must exist and be **granted to** the connecting account; name mapping must include MySQL `user@host` rules | **Soft block** — naming/host part must be specified in auth design |
| RLS for per-user row filters | **Absent** | **Hard block** for Postgres-RLS-shaped apps; not required for pure GRANT-shaped PostgREST deployments |
| Column privileges | Supported (`GRANT SELECT (col) …`) | No block |
| Routine `EXECUTE` + invoker/definer security | Supported (`SECURITY_TYPE` DEFINER/INVOKER on routines/views) | No block for basic RPC; semantics of DEFINER vs active roles need care |
| SQL reads JWT claims via GUC | No GUC; need alternative | **Hard block** for claim-driven SQL/RLS patterns; app-mediated claims still fine for HTTP auth |
| Pool-safe local role | Session `SET ROLE` must be explicitly cleared | **Operational block** if ignored; solvable in server design |
| `GRANT user TO authenticator` | `GRANT 'webuser' TO 'authenticator_login'@'%'` (roles interchangeable with users in GRANT role syntax) | No block |
| Anonymous when no JWT | `SET ROLE 'anonymous'` or default role config | No block |

## What still enables a PostgREST-like JWT→role story

Feasible core (experiment-backed):

1. Connect pool as a single login account (authenticator) with **USAGE only** + role grants.
2. Validate JWT in myrest.
3. Map claim → MySQL role name.
4. On a checked-out connection: `SET ROLE <mapped_role>;` run statements; on return: `SET ROLE NONE` (or DEFAULT).
5. Rely on MySQL grants (table/column/routine) for authorization.
6. Document **no RLS**; row filters must be views/routines or unsupported.

## Gaps that block “full” parity with typical PostgREST+RLS deployments

1. **No RLS** — any client/DB design depending on policies cannot be equivalent.
2. **`CURRENT_USER` stable under role switch** — SQL and view/routine patterns that key off `current_user` do not see the JWT role.
3. **No request GUCs** — claim-driven SQL authorization patterns need redesign.
4. **Session-level role state** — must be part of the server connection lifecycle, not inherited from Postgres docs.
5. **Role name@host** — PostgREST role strings are simple identifiers; MySQL role identity includes host.

## Answer (ticket resolution)

- MySQL 8 can **activate database roles per request** via `SET ROLE` on an authenticator login that holds those roles, which is enough for **grant-based** JWT→database-role auth similar in *effect* to PostgREST’s privilege switch.
- MySQL does **not** provide Postgres-equivalent **impersonation identity**, **RLS**, or **request GUCs**; those are the hard gaps.
- Auth design ticket should choose among: (A) SET ROLE on pooled authenticator (closest operationally), (B) PROXY/connect-as per role, (C) app-enforced auth abandoning DB role switch — and must mark RLS/claim-GUC patterns unsupported unless a MySQL-native equivalent is defined.
