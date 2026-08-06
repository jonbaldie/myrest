# myrest

myrest is an HTTP API service that exposes a MySQL database with the same client contracts as PostgREST.

## Language

**myrest**:
The product and service this repository defines. A PostgREST-compatible HTTP API over MySQL.
_Avoid_: MyREST (except in display titles), PostgREST-for-MySQL as the product name

**PostgREST**:
The external reference system whose HTTP API contracts myrest aims to match.
_Avoid_: postgrest (when referring to the project as a proper name in prose), Supabase

**Parity target**:
The concrete PostgREST release whose documented HTTP behaviour myrest treats as normative.
_Avoid_: “latest” without a version pin, “compatible enough”

**Capability area**:
A slice of the PostgREST surface used to structure the myrest spec (for example read, write, embed, rpc, auth).
_Avoid_: module, epic, milestone (when you mean a spec chapter)

**Schema cache**:
The in-memory model of database objects myrest serves, built from MySQL catalog data and privilege information.
_Avoid_: metadata store, dictionary, information schema (the MySQL source is not the cache)

**Resource**:
A database object exposed over HTTP (table, view, or callable routine) according to config and privileges.
_Avoid_: endpoint, entity, model (when you mean the exposed DB object)

**Embed**:
A request that nests related resources in one response, driven by declared relationships.
_Avoid_: join, include, expand (unless quoting a query parameter name)

**Aggregate**:
A read that reduces column values on a **resource** (for example sum or count) under the PostgREST-style select contract.
_Avoid_: rollup, group query, analytics query (when you mean this API behaviour)

**Relationship**:
A link between resources in the schema cache, taken only from foreign keys the catalog declares (including a join table used for many-to-many).
_Avoid_: association, join path, inferred view link (when you mean a cache relationship)

**RPC**:
A call to an exposed database routine through the PostgREST-style function calling interface.
_Avoid_: stored procedure endpoint, remote procedure (unless explaining the acronym)

**Database role**:
The MySQL account (or role) selected for a request after authentication, used for privilege checks and execution.
_Avoid_: user (when you mean the DB principal), JWT role claim (the claim names the role; it is not the role)

**Authenticator**:
The MySQL login account myrest uses for pooled database connections before it selects a **database role** for the request.
_Avoid_: superuser, admin connection, service account (when you mean this login)

**Anonymous database role**:
The **database role** myrest selects when the request has no usable JWT and anonymous access is configured.
_Avoid_: guest, public role, unauthenticated user (when you mean this DB role)

**Role switch**:
The per-request step that activates the chosen **database role** on the **authenticator** connection so MySQL grants apply.
_Avoid_: impersonation (Postgres-style identity change; myrest does not claim that), SET ROLE (implementation phrase in prose)

**Parity decision rule**:
The rule that assigns each PostgREST behaviour one parity label for myrest against the parity target.
_Avoid_: compatibility policy, support matrix rule (when you mean this rule)

**Parity label**:
One of: full match, partial match, or not supported. The parent spec puts exactly one parity label on each classified behaviour.
_Avoid_: equivalent/narrowed/unsupported (informal synonyms only), status, support level

**Full match**:
Parity label meaning myrest keeps the same client HTTP contract as the parity target for that behaviour, on success and on errors it claims to support.
_Avoid_: required equivalent, complete parity, compatible

**Partial match**:
Parity label meaning only a named stable subset of the behaviour is supported; other input gets a stable error.
_Avoid_: narrowed, best-effort, mostly supported

**Not supported**:
Parity label meaning myrest refuses the behaviour with a stable error and does not implement it.
_Avoid_: unsupported, out of scope (product scope is separate), wontfix

**Normative scenario**:
A binding acceptance case in the parent spec for one labelled behaviour (or one fixed cross-area smoke path).
_Avoid_: test case (when you mean the spec artifact), example (when the case is binding), PostgREST test (upstream source, not the myrest law)

**Gap list**:
The Verification roll-up of every **partial match** and **not supported** item taken from the **capability area** chapters.
_Avoid_: global matrix, unsupported matrix, backlog (when you mean this roll-up)
