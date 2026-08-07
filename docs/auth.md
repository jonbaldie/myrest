# Authentication

myrest authenticates clients with a Bearer JWT and authorizes them with MySQL
grants of the active **database role**. There is no second application ACL.

## Bearer JWT

Send `Authorization: Bearer <jwt>` (the word `Bearer` may be lower case). myrest
verifies:

- the signature against `jwt-secret` (with `jwt-secret-is-base64` when set)
- the time claims `exp`, `nbf`, and `iat`, with a 30-second clock skew
- the audience claim against `jwt-aud` when that knob is set

The claim named by `jwt-role-claim-key` (default `.role`) names the **database
role**. myrest activates that role on a pooled **authenticator** connection for
the request. `jwt-cache-max-entries` bounds the token cache; `0` turns the cache
off.

When there is no usable JWT and `db-anon-role` is set, the request runs as the
**anonymous database role**. When there is no usable JWT and no anonymous role,
myrest answers with `PGRST302`.

| Failure | Code | HTTP status |
| --- | --- | --- |
| Invalid or undecodable JWT | `PGRST301` | 401 |
| No Bearer JWT and anonymous access disabled | `PGRST302` | 401 |
| JWT claims validation failed (expired, audience, …) | `PGRST303` | 401 |

## Partial match: role identity

After a successful **role switch**, MySQL grants follow the active database
role, but SQL `CURRENT_USER()` stays the **authenticator**. Design policies and
routines on grants and `CURRENT_ROLE()`, not on `CURRENT_USER()` equality with
the JWT role.

## Not supported

| Feature | Refusal |
| --- | --- |
| Postgres row-level security | Prefer `row-security` → `MYREST001`. myrest offers no fake RLS. |
| Request GUCs / `request.jwt.claims` in SQL | Prefer `jwt-claims` → `MYREST001`. Claims stay on the HTTP wire. |
| Non-Bearer credential schemes | `Authorization` schemes other than Bearer → `MYREST001`. |
