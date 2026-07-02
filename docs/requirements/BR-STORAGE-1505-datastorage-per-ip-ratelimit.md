# BR-STORAGE-1505: Data Storage Per-IP Rate Limiting

**Business Requirement ID**: BR-STORAGE-1505
**Category**: Data Storage Service — Security & Availability
**Priority**: **P2 (MEDIUM)** — Closes a MEDIUM-severity GA readiness gap
**Target Version**: **V1.5**
**Status**: ✅ Implemented
**Date**: June 30, 2026
**GitHub Issues**: [#1505](https://github.com/jordigilh/kubernaut/issues/1505) (GAP-09)

---

## Business Need

### Problem Statement

The Data Storage HTTP API (`pkg/datastorage/server`) had no in-process request-rate control: any single client — a misbehaving internal caller, a compromised service account, or an external actor that reached the API — could send an unbounded volume of requests, competing for goroutines, database connections, and CPU with legitimate traffic from Gateway, KubernautAgent, APIFrontend, and the reconciler-driven audit write path.

This was identified as **GAP-09 (MEDIUM severity)** in the GA Readiness Audit (issue #1505): DataStorage was the only core service without a rate-limiting control comparable to APIFrontend's existing per-IP/per-user limiter (`pkg/apifrontend/ratelimit`).

### Impact Without This BR

- **FedRAMP SC-5** (Denial of Service Protection) relies entirely on infrastructure external to the service (ingress/proxy rate limiting), which may not be present in every deployment topology (e.g. direct in-cluster ClusterIP access from other services, bypassing an ingress).
- No audit trail exists for rate-limit denials at the DS layer, so a sustained flood against DataStorage would not be independently observable via its own audit trail (**FedRAMP AU-12**: audit generation for security-relevant events).

---

## Decision: In-Process Per-IP Token Bucket, Opt-In and Self-Audited

A per-IP token-bucket rate limiter (`pkg/datastorage/server/middleware.IPLimiter`, modeled on the existing `pkg/apifrontend/ratelimit.IPLimiter` pattern) is applied as Chi middleware, placed **before** authentication in the middleware chain so an unauthenticated flood does not reach TokenReview/SubjectAccessReview calls.

Key design choices:

1. **Opt-in, disabled by default** (`datastorage.config.server.rateLimit.enabled: false`): existing deployments that already rate limit at an ingress/proxy layer are unaffected on upgrade. Operators without that external control can enable this in-process backstop.
2. **Per-IP, not global**: each client IP gets an independent token bucket (default: 50 req/s sustained, burst 100), so one noisy client cannot exhaust the shared bucket for all others — the same pattern already proven in APIFrontend's `IPLimiter`.
3. **Self-audited denials**: every denial emits a `datastorage.ratelimit.denied` audit event (new `event_category: security` — the first DS event not tied to a single business domain) via DataStorage's existing internal self-audit path (`InternalAuditClient`, bypassing HTTP/its own REST API to avoid a circular dependency), so a sustained flood is independently reconstructable from the audit trail (FedRAMP AU-12, SOC2 CC8.1).
4. **RFC 7807 429 response** with a `Retry-After` header, consistent with existing DataStorage error response conventions (`WriteMaxBytesExceeded`, `response.WriteRFC7807Error`).
5. **New `security` event category** added to the shared `event_category` enum (`api/openapi/data-storage-v1.yaml`): existing categories (`gateway`, `workflow`, `actiontype`, etc.) are all owned by a specific business domain; a pre-auth rate-limit denial has no such domain (the request never reached a handler), so it is classified under a new cross-cutting `security` category rather than forcing it into an unrelated domain category.

### Why Not Reuse the Existing AIAgent/APIFrontend Rate-Limit Audit Payloads?

`AIAgentRatelimitDeniedPayload` (`aiagent.ratelimit.denied`) and `ApifrontendRatelimitDeniedPayload` (`apifrontend.ratelimit.denied`) already exist in the shared OpenAPI schema, but their `event_type` discriminator values are hardcoded to their respective services. Reusing either for DataStorage's own denials would mislabel the emitting service in the audit trail. A new `DatastorageRatelimitDeniedPayload` (`datastorage.ratelimit.denied`) was added instead, mirroring the `AIAgentRatelimitDeniedPayload` shape (`event_id`, `source_ip`, `path`, `method`) for consistency.

---

## Success Criteria

1. `IPLimiter.Allow()` enforces an independent token bucket per client IP; exceeding the burst denies further requests until the bucket refills.
2. `IPRateLimitMiddleware` returns HTTP 429 with a `Retry-After` header and an RFC 7807 `application/problem+json` body when the limit is exceeded, and passes requests through unmodified otherwise.
3. Every denial invokes the configured `AuditFunc` exactly once (non-blocking, does not delay the 429 response).
4. `NewRatelimitDeniedAuditEvent` produces a valid `AuditEventRequest` (non-empty `correlation_id` per OpenAPI `minLength: 1`, category `security`, outcome `failure`) with a typed `DatastorageRatelimitDeniedPayload`.
5. Disabled by default (`datastorage.config.server.rateLimit.enabled: false`): no behavior change for existing deployments on upgrade.
6. Helm: `datastorage.config.server.rateLimit.enabled=true` wires `requestsPerSecond`/`burst` into the DataStorage ConfigMap.

---

## Related Documents

- [Kubernaut Helm Chart README — Optional: Data Storage Per-IP Rate Limiting](../../charts/kubernaut/README.md#optional-data-storage-per-ip-rate-limiting-gap-09)

---

**Document Version**: 1.0
**Last Updated**: June 30, 2026
**Maintained By**: Kubernaut Architecture Team
