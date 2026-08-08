# Schema cache and resource exposure

myrest builds a **schema cache** from MySQL catalog data for the databases in
`db-schemas`. A table, view, or routine is a **resource** only when the active
**database role** holds a relevant privilege on it. **Relationships** for
**embed** come only from declared foreign keys. Reload is by explicit signal
(`SIGUSR1`), not by a Postgres NOTIFY bus.

See [ADR 0003](adr/0003-schema-cache-and-resource-exposure.md).

## Full match rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| Privilege-filtered resource exposure | full match | cache-001, cache-002 |
| Explicit schema cache reload | full match | cache-003 |

## Gap list rows

| Item | Parity label | Scenarios |
| --- | --- | --- |
| Postgres NOTIFY reload bus | not supported | cache-004 |
| Domains, casts, and row computed-fields | not supported | cache-005 |
