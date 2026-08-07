# Procedure RPC response shape

PostgREST exposes functions on `POST /rpc/<name>` and does not support stored
procedures. myrest keeps that function contract as a **full match**, and also
exposes MySQL procedures on the same `/rpc/<name>` path.

A successful `POST /rpc/<procedure>` answer is always:

- HTTP status `200`
- `Content-Type: application/json`
- a JSON **object** whose keys are the `OUT` and `INOUT` parameter names of the
  procedure, in catalog ordinal order, with the values after `CALL`

When the procedure has no `OUT` or `INOUT` parameters, the body is an empty
object:

```json
{}
```

Example with an `OUT` parameter:

```bash
curl -X POST http://127.0.0.1:3000/rpc/echo_name \
  -H 'Content-Type: application/json' \
  -d '{"src":"alpha"}'
{"dst":"alpha"}
```

Result sets that a procedure may return are outside this shape. Later RPC
tickets cover read features on row-set results. This ticket keeps one stable
procedure body so clients do not need a second URL family.
