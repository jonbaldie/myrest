# Media types and remaining Prefer values under the parity decision rule

Clients need a truthful Accept and Prefer matrix beyond the locked JSON and
write/count slices. PostgREST v14.16 offers CSV, geo+json, singular object,
plan media, custom handlers, Prefer `tx=`, and Prefer `timezone`. **Decision**
(from [Media types and the remaining Prefer values](https://github.com/jonbaldie/myrest/issues/46)):
myrest claims JSON array (including `vnd.pgrst.array+json` and `*/*`), singular
`vnd.pgrst.object+json`, and `text/csv` as **full match** for row data; refuses
geo+json, plan media, custom handlers, and every other Accept with `PGRST107`;
keeps Prefer `tx=` under [ADR 0013](0013-transaction-end-and-isolation.md); and
labels Prefer `timezone` **not supported** with a stable `MYREST001` refuse.
The label list lives in [Media types and the remaining Prefer values](../media-types-and-prefer.md).

**Why:** CSV and singular object are honest HTTP shapes on MySQL. Geo, plan,
custom handlers, and Postgres timezone GUCs are not. Silent JSON for an
unclaimed Accept would break the **parity decision rule**.
