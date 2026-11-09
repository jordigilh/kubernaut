# API Frontend backend-incarnation contract

**Issue:** kubernaut#2614
**Consumer:** kubernaut-console#168
**Scope:** one API Frontend process; HA/failover is kubernaut#2606

## Wire contract

The Console same-origin path exposes:

```http
GET /.well-known/kubernaut-console-contract.json
```

The response is public and exact:

```http
HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store

{"backend_incarnation":"01234567-89ab-4cde-8123-456789abcdef"}
```

The JSON object contains exactly one property, `backend_incarnation`. Its value
is opaque, server-generated, bounded to 36 ASCII bytes, and must be a canonical
lowercase UUIDv4 matching:

```text
^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$
```

API Frontend generates the value once during startup. It remains stable for the
life of that process and changes when a replacement process starts. The value
is not a credential, user identity, request ID, metric label, or replay token.
The public response and the new lifecycle header never echo bearer tokens,
cookies, raw `Authorization` values, or user identity.

The same value is returned in the exact response header below, but only after
successful authentication, on these POST routes:

```text
X-Kubernaut-Backend-Incarnation: <backend_incarnation>

POST /a2a/invoke
POST /
POST /mcp
POST /a2a/status
```

The header is present on authenticated success, handler errors, panic recovery,
post-authentication rate-limit responses, MCP-disabled responses, SSE-cap
responses, and authenticated stream responses. It is absent from the public
contract, agent card, health, readiness, metrics, `/a2a/access`, missing or
invalid authentication, and pre-authentication IP-limit responses. A `429` is
therefore conditional: pre-authentication rejection has no lifecycle header;
authenticated post-authentication rejection has it.

Existing protocol behavior remains authoritative. A2A and root oversized-body
responses retain the current a2a-go JSON-RPC status/body baseline; this contract
does not promise a generic `413`. `/a2a/status` body-limit and parse failures
remain HTTP `400`, JSON-RPC code `-32600`, message `invalid_request`. MCP and
status SSE content types, cache behavior, flushing, connection handling, and
write-deadline behavior remain unchanged.

## Console #168 rollout and evidence

Console qualification must verify the live same-origin route and an
authenticated lifecycle route through the deployed ingress. The minimum AF
evidence is:

1. Start with exactly one API Frontend replica. The existing two-replica
   architecture documentation is not evidence that a process-local value is
   shared across pods.
2. Capture the public JSON, `Content-Type`, and `Cache-Control: no-store`.
3. Capture the lifecycle header on authenticated A2A, MCP, and status responses
   and confirm all values match the public contract.
4. Replace the AF process, capture the new public value, and confirm it differs;
   then confirm an unchanged value is stable during one process lifetime.
5. Record external OAuth2 Proxy authentication evidence separately. This backend
   repository does not claim to qualify the browser, ingress, cookies, or
   OAuth2 Proxy.

Console-owned qualification behavior is explicit: a missing, malformed, or
changed lifecycle signal fences stale work and triggers revalidation; a
transient lifecycle-check failure is retryable and must not log the user out;
ambiguous non-idempotent operations are never automatically replayed. These
browser behaviors are not implemented by API Frontend.

Multi-replica epochs, failover, stream recovery, ownership fencing across pods,
and automatic replay are excluded from #2614 and remain tracked by
kubernaut#2606.
