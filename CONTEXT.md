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

**RPC**:
A call to an exposed database routine through the PostgREST-style function calling interface.
_Avoid_: stored procedure endpoint, remote procedure (unless explaining the acronym)

**Database role**:
The MySQL account (or role) selected for a request after authentication, used for privilege checks and execution.
_Avoid_: user (when you mean the DB principal), JWT role claim (the claim names the role; it is not the role)
