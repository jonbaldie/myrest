# Discovery: OPTIONS and OpenAPI

**Ticket:** [Discovery: OPTIONS and OpenAPI from the schema cache](https://github.com/jonbaldie/myrest/issues/44)  
**Closes:** deferred item 1 of [Parent spec: myrest PostgREST parity over MySQL 8](https://github.com/jonbaldie/myrest/issues/20)  
**Parity target:** PostgREST v14.16 (from `CONTEXT.md`)

A client discovers the surface of myrest through `OPTIONS` and the OpenAPI
document on `GET /`. Both take their input only from the **schema cache** and
the privileges of the active **database role**. Proof lives at the HTTP API
boundary.

## OPTIONS

| Behaviour | Parity label | Contract |
| --- | --- | --- |
| Method allow-list from grants | **partial match** | `OPTIONS /{table}` and `OPTIONS /rpc/{name}` answer with status 200, an empty body, and `Allow` listing only methods the active role can use on that **resource**. Table methods need the matching grant (`SELECT` → `GET`/`HEAD`, `INSERT` → `POST`/`PUT`, `UPDATE` → `PATCH`, `DELETE` → `DELETE`). `OPTIONS` is always present when the role can use the resource. Routine `EXECUTE` yields `POST`; read-safe routines also yield `GET`/`HEAD`. The parity target advertises methods from object kind and view triggers, not grants; myrest keeps grant honesty instead. |
| Hidden resource | **full match** | A table or routine the role cannot use is not a resource: status 404 with `PGRST205` (table) or `PGRST202` (routine). |
| `PUT` on tables | **full match** | `OPTIONS` advertises `PUT` when the role holds `INSERT`. Request-time `merge-duplicates` still needs `UPDATE`. |

CORS preflight `OPTIONS` (with `Access-Control-Request-Method`) stays under
[CORS origins and proxy header behaviour](cors-and-proxy.md). It is not this
resource allow-list.

## OpenAPI knobs

| Knob | Parity label | Contract |
| --- | --- | --- |
| `openapi-mode=follow-privileges` (default) | **full match** | `GET /` serves an OpenAPI 2.0 document (`Content-Type: application/openapi+json`) that lists only resources the active role can use. Path methods follow the same grant rules as `OPTIONS`. |
| `openapi-mode=ignore-privileges` | **full match** | The document lists every table and routine in the schema cache for configured databases, and advertises the served table methods (`get`/`post`/`put`/`patch`/`delete`) and routine methods from read-safety. |
| `openapi-mode=disabled` | **full match** | `GET /` answers 404 with the no-handler envelope (`MYREST003`). |
| `openapi-security-active` | **full match** | When true, the document holds `securityDefinitions.JWT` (apiKey in `Authorization`) and a matching `security` requirement. When false, those fields are omitted. |
| `openapi-server-proxy-uri` | **full match** | When set, `host`, `schemes`, and `basePath` come from that URI (trailing `/` removed). Otherwise they come from the listen URL of the process. myrest does not read `X-Forwarded-*` or `Forwarded` for this value. |
| `db-root-spec` | **full match** | When set to `database.routine`, `GET /` runs that routine as the active role (empty named args) and returns its JSON body as `application/json`, replacing the generated document. The role still needs `EXECUTE`. |

## OpenAPI document body depth

Each part below has exactly one **parity label**. myrest omits what it does not
claim.

| Document part | Parity label | Notes |
| --- | --- | --- |
| `swagger: "2.0"` | **full match** | Same OpenAPI 2.0 root marker as the parity target. |
| `info` title / description / version | **partial match** | Fixed myrest title and description. Schema comments as `info.description` / title override are not claimed. |
| `host` / `schemes` / `basePath` | **full match** | From `openapi-server-proxy-uri` or the listen URL. |
| `paths` resource list | **full match** to the privilege / ignore-privileges modes above | Tables as `/{name}`, routines as `/rpc/{name}`, plus `/` introspection. |
| Path HTTP verbs from grants | **partial match** | Verbs follow grants (or ignore-privileges served methods). The parity target uses insertable/updatable/deletable flags and view triggers. |
| Path operation parameters, Prefer enums, row filters | **not supported** | Omitted. |
| `definitions` / column schemas / PK / FK notes | **not supported** | Omitted. |
| Shared `parameters` map | **not supported** | Omitted. |
| `consumes` / `produces` media-type matrix | **not supported** | Omitted at the document root; the introspection path may name OpenAPI and JSON produces only. |
| Example payloads | **not supported** | Omitted. |
| `security` / `securityDefinitions` | **full match** | Controlled by `openapi-security-active` as above. |
| `externalDocs` | **not supported** | Omitted. |

## Gap list rows

| Item | Parity label |
| --- | --- |
| OPTIONS method source (grants, not object-kind / view triggers) | partial match |
| OpenAPI `info` from schema comments | partial match |
| OpenAPI path verbs from grants vs insertable flags | partial match |
| OpenAPI parameters, definitions, consumes/produces matrix, examples, externalDocs | not supported |
