# Embed over declared foreign keys

Clients nest related **resources** in one read with the PostgREST nested
select syntax. myrest nests only when a **relationship** is in the **schema
cache**. Relationships come only from declared foreign keys, including a
declared join table for many-to-many.

## Supported shapes

| Shape | Example | Response nest |
| --- | --- | --- |
| Many-to-one | `/orders?select=id,items(id,name)` | object or `null` |
| One-to-many | `/items?select=id,orders(id)` | array |
| Many-to-many | `/items?select=id,tags(name)` | array through join table |
| Disambiguation | `/deliveries?select=id,addresses!deliveries_from(label)` | one chosen FK |

Nested filter, order, and page use the embed key as a prefix:

```text
/items?select=id,orders(id)&orders.id=gt.1&orders.order=id.desc&orders.limit=1
```

## Refusals

| Case | Code | HTTP |
| --- | --- | --- |
| No declared foreign-key path | `PGRST200` | 400 |
| More than one relationship, no `!hint` | `PGRST201` | 300 |
| Computed relationship (routine name as embed) | `MYREST001` | 400 |

myrest never invents a relationship. A view chain with no declared foreign key
is not supported. A computed relationship is not supported.
