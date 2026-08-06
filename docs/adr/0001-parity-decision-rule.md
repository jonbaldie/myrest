# Parity decision rule: full match, partial match, or not supported

myrest aims at HTTP parity with a pinned PostgREST release over MySQL 8. MySQL cannot honestly implement every Postgres-backed behaviour (for example RLS, request GUCs, Postgres FTS). We reject silent approximation as “close enough.”

**Decision:** Every classified PostgREST behaviour gets exactly one **parity label** — **full match**, **partial match**, or **not supported** — under the **parity decision rule** locked in [Define the parity decision rule](https://github.com/jonbaldie/myrest/issues/5). Prefer an honest full wire match; else a named subset with a stable error on the rest; else refuse. Errors use the PostgREST envelope (`PGRST*` when it fits; a small myrest code family for MySQL gaps). Product out-of-scope items are not a fourth API label.

**Why:** Existing PostgREST clients need a truthful contract. A stretched “equivalent” that lies about semantics is worse than a stable refusal. Capability-area chapters record labels so the parent spec stays consistent as surfaces are decided.
