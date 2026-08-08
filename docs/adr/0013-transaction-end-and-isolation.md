# Write and RPC transactions follow db-tx-end; reads stay outside

Clients need honest bounds for what one write or **RPC** request commits or
rolls back, and which isolation level that unit uses. PostgREST wraps every
resource request in a transaction ended by `db-tx-end` / `Prefer: tx=`, at
Postgres `READ COMMITTED` by default. **Decision** (from
[Transaction end and isolation behaviour (db-tx-end)](https://github.com/jonbaldie/myrest/issues/43)):
myrest opens one `READ COMMITTED` transaction for each write and each **RPC**
call (including optional `db-pre-request`), ends it with the same `db-tx-end`
value set and Prefer override rules as the **parity target**, and labels that
surface **full match**. Ordinary table reads stay outside a request
transaction and outside `db-tx-end` (**not supported** for this claim).
Role-level and routine-level isolation overrides are **not supported**.

**Why:** Matching the write and **RPC** end contract keeps Prefer `tx=` and
test-oriented rollback modes honest on MySQL. Stretching full match over
Postgres role/function GUCs or over every GET would lie about MySQL and about
the deferred read scope of this ticket.
