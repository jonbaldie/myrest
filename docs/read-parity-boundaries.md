# Read parity boundaries

This page names the MySQL **partial match** subsets for text-case and JSON
path on ordinary reads, and the **not supported** refusals that keep the
client contract honest. Ordinary full-match operators stay in
[Ordinary read](ordinary-read.md).

## Text-case subset (partial match)

PostgREST `ilike` is case-insensitive. MySQL has no `ILIKE`. Case folding on
MySQL follows the column collation.

**In subset (succeeds):**

| Client form | MySQL behaviour |
| --- | --- |
| `col=ilike.pattern` | `col LIKE pattern` (`*` in the pattern becomes `%`) |
| `col=not.ilike.pattern` | `col NOT LIKE pattern` |

The subset holds when the filtered column uses a MySQL Unicode
case-insensitive collation (`*_ci`), for example the fixture default
`utf8mb4_0900_ai_ci`. Matching then follows that collation (case and accent
rules of MySQL), not Postgres `ILIKE` Unicode case folding. myrest reads the
column collation from the schema cache: `ilike` on a column whose collation
is not `*_ci` refuses with `MYREST001`.

**Outside subset (stable refuse, `MYREST001`):**

| Client form | Why |
| --- | --- |
| `col=ilike.pattern` on a non-`*_ci` collation | Outside the named MySQL collation subset |
| `col=match.regex` | Postgres POSIX regex (`~`) |
| `col=imatch.regex` | Postgres case-insensitive POSIX regex (`~*`) |
| `not.match` / `not.imatch` | Same Postgres regex family |

myrest does not claim Postgres regex or Postgres `ILIKE` case folding that is
independent of MySQL collation.

## JSON path subset (partial match)

PostgREST projects and filters JSON with `->` and `->>`. MySQL supports the
same arrow operators on `JSON` columns, with a `$`-rooted path.

**In subset (succeeds):**

| Client form | Meaning |
| --- | --- |
| `select=…,col->key` / `col->>key` | Project one JSON member or array index |
| chained `col->a->0->>b` | Nested object keys and zero-based array indexes |
| `col->>key=eq.value` (and other full-match ops) | Filter on an extracted scalar |
| `order=col->>key.asc\|desc` | Order by an extracted scalar |
| optional `alias:col->>key` in `select` | Rename the projected value |

Path legs in the subset are only:

- unquoted object member names that are MySQL JSON path identifiers
  (`blood_type`, not `"weird key"`)
- non-negative decimal array indexes (`0`, `1`, …)

myrest maps each PostgREST leg to a MySQL JSON path leg (`$.blood_type`,
`$[0]`, …) and uses `->` / `->>` on the JSON column the same way the client
wrote them.

**Outside subset (stable refuse, `MYREST001`):**

| Client form | Why |
| --- | --- |
| Postgres `#>` / `#>>` path-array operators | Postgres-only JSON operators |
| Quoted / non-identifier object keys in the path | Outside the named MySQL identifier subset |
| Wildcards (`*`, `**`) or empty path legs | Outside the named path subset |
| Arrow access on a non-`JSON` column | Would need Postgres `to_jsonb(...)` rewriting |

## Not supported refusals

Each refusal uses the stable error envelope and the myrest gap code
`MYREST001` (see [Error contract](error-contract.md)).

| Scenario | Client probe | Label |
| --- | --- | --- |
| `read-007` | `fts` / `plfts` / `phfts` / `wfts` (and `not.` forms) | not supported |
| `read-008` | Postgres array/range operators `cs` `cd` `ov` `sl` `sr` `nxr` `nxl` `adj` | not supported |
| `read-009` / `repr-003` | `Prefer: count=planned` or `count=estimated` | not supported |

MySQL full-text search and approximate table stats exist, but they do not
match the PostgREST operators or Prefer modes above, so myrest refuses them
instead of renaming them.
