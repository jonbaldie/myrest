# CORS origins and proxy header behaviour

**Ticket:** [CORS origins and proxy header behaviour](https://github.com/jonbaldie/myrest/issues/45)  
**Closes:** deferred item 3 of [Parent spec: myrest PostgREST parity over MySQL 8](https://github.com/jonbaldie/myrest/issues/20)  
**Parity target:** PostgREST v14.16 (from `CONTEXT.md`)

This page states the wire contract and the **parity label** for each behaviour in this area. Proof lives at the HTTP API boundary.

## CORS origins

Knob: `server-cors-allowed-origins` (list). An empty list, or a knob nobody set, accepts every origin.

| Behaviour | Parity label | Contract |
| --- | --- | --- |
| Origin policy from `server-cors-allowed-origins` | **full match** | A listed `Origin` is reflected in `Access-Control-Allow-Origin`. An origin outside the list gets no `Access-Control-Allow-Origin`. An empty list answers with `*`. |
| Actual request CORS headers | **full match** | An allowed cross-origin request also gets `Access-Control-Expose-Headers` with `Content-Encoding, Content-Location, Content-Range, Content-Type, Date, Location, Server, Transfer-Encoding, Range-Unit`. When the allow list reflects a concrete origin, the answer also holds `Access-Control-Allow-Credentials: true`. |
| Preflight `OPTIONS` | **full match** | A preflight (`OPTIONS` with `Access-Control-Request-Method`) from an allowed origin gets status 200, an empty body, `Access-Control-Allow-Methods: GET, POST, PATCH, PUT, DELETE, OPTIONS, HEAD`, `Access-Control-Allow-Headers` starting with `Authorization` then the requested names then `Accept, Accept-Language, Content-Language`, and `Access-Control-Max-Age: 86400`. A refused origin gets no `Access-Control-Allow-Origin` and the request continues without those CORS headers. |
| Request with no `Origin` | **full match** | No CORS response headers. |

## Proxy headers and reported URLs

| Behaviour | Parity label | Contract |
| --- | --- | --- |
| `X-Forwarded-Host`, `X-Forwarded-Proto`, and `Forwarded` | **full match** | myrest does not read these headers when it chooses a URL to report. The parity target does the same. Ordinary responses do not take host or scheme from them. |
| Absolute base URL selection | **full match** | When myrest reports an absolute base URL, `openapi-server-proxy-uri` wins when set (trailing `/` removed); otherwise the listen URL of the process wins. The OpenAPI document that emits that base URL is [#44](https://github.com/jonbaldie/myrest/issues/44). |

## Gap list rows

Every behaviour in this area is **full match**. This area adds no row to the **gap list**.
