# ADR-048: Rate Limiting Delegation to Ingress/Route Proxy

## Status
**✅ APPROVED** (2025-12-07) — **PARTIALLY SUPERSEDED** by [ADR-048-ADDENDUM-001](ADR-048-ADDENDUM-001-chi-throttle-middleware.md) (2026-01-29)
**Last Reviewed**: 2026-03-02
**Confidence**: 95%
**Next Action**: None — Redis rate limiter removed (complete). Chi Throttle added as defense-in-depth (ADR-048-ADDENDUM-001).

> **Note (2026-03-02)**: This ADR correctly removed the Redis-based rate limiter. However, it framed proxy as the *sole* rate limiting layer. ADR-048-ADDENDUM-001 subsequently added chi's built-in `Throttle` middleware as an application-layer defense-in-depth. The proxy layer is now complementary, not primary. See the addendum for details.

---

## Context & Problem

### Current State

The Gateway service implements rate limiting using Redis:

```go
// pkg/gateway/middleware/ratelimit.go
func NewRedisRateLimiter(redisClient *goredis.Client, limit int, window time.Duration) func(http.Handler) http.Handler {
    // Per-source IP tracking using Redis
    // Sliding window algorithm
    // Fail-open when Redis unavailable
}
```

**Business Requirements**:
- BR-GATEWAY-071: Rate limit webhook requests per source IP
- BR-GATEWAY-072: Prevent DoS attacks through request throttling

### Problems with Current Approach

| Issue | Impact |
|-------|--------|
| **Redis dependency** | External infrastructure required |
| **Per-pod limits** | N pods × limit = effective limit (higher than intended) |
| **Crash recovery** | Rate limit state lost on pod restart |
| **Code complexity** | ~130 lines of rate limiting middleware |
| **Testing burden** | Unit + integration tests for rate limiting |

### Key Insight

**Kubernetes already provides rate limiting at the proxy layer**:
- **Nginx Ingress Controller** (Kubernetes)
- **HAProxy Router** (OpenShift)

Both are:
- ✅ Cluster-wide (exact global limit)
- ✅ Crash-proof (independent of application pods)
- ✅ Zero application code required
- ✅ Already deployed in production clusters

---

## Alternatives Considered

### Alternative 1: Keep Gateway Rate Limiting (Current)

**Approach**: Maintain Redis-based rate limiting in Gateway middleware.

**Pros**:
- ✅ Defense-in-depth (two layers)
- ✅ Per-endpoint granularity possible

**Cons**:
- ❌ Redis dependency (infrastructure cost)
- ❌ Per-pod limits (N × limit effective)
- ❌ Code maintenance burden
- ❌ Testing complexity

**Confidence**: 40% (works but adds unnecessary complexity)

---

### Alternative 2: Delegate to Ingress/Route Proxy (APPROVED)

**Approach**: Remove Gateway rate limiting, rely entirely on Nginx Ingress (K8s) or HAProxy Router (OpenShift).

**Kubernetes (Nginx Ingress)**:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gateway-ingress
  annotations:
    nginx.ingress.kubernetes.io/limit-rps: "100"
    nginx.ingress.kubernetes.io/limit-connections: "50"
    nginx.ingress.kubernetes.io/limit-rpm: "6000"
```

**OpenShift (HAProxy Router)**:
```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: gateway-route
  annotations:
    haproxy.router.openshift.io/rate-limit-connections: "true"
    haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp: "100"
    haproxy.router.openshift.io/rate-limit-connections.rate-http: "1000"
```

**Pros**:
- ✅ **Zero Redis** - removes external dependency
- ✅ **Global limit** - exact cluster-wide enforcement
- ✅ **Crash-proof** - proxy independent of Gateway pods
- ✅ **Zero code** - just YAML annotations
- ✅ **Platform-managed** - ops/platform team owns it
- ✅ **Already deployed** - Ingress/Router exists in every cluster

**Cons**:
- ⚠️ Less granular (per-route, not per-endpoint)
  - **Mitigation**: Sufficient for V1.0; can add endpoint-specific Ingress resources if needed
- ⚠️ Platform dependency
  - **Mitigation**: Both K8s and OCP supported; annotations documented

**Confidence**: 95% (simplifies architecture, leverages existing infrastructure)

---

### Alternative 3: In-Memory Rate Limiting (go-cache)

**Approach**: Replace Redis with in-memory caching (e.g., `github.com/patrickmn/go-cache`).

**Pros**:
- ✅ No Redis dependency
- ✅ Faster (~100ns vs ~1ms)

**Cons**:
- ❌ Per-pod limits (same problem as Redis)
- ❌ State lost on pod crash
- ❌ Still requires code maintenance

**Confidence**: 50% (removes Redis but doesn't solve per-pod problem)

---

## Decision

**APPROVED: Alternative 2 - Delegate to Ingress/Route Proxy**

### Rationale

1. **Simplicity**: Zero application code vs ~130 lines
2. **Correctness**: Global limit vs per-pod limits
3. **Reliability**: Proxy survives pod crashes
4. **Redis deprecation**: Aligns with DD-GATEWAY-011 goal
5. **Platform alignment**: Uses standard K8s/OCP patterns

### Key Insight

> Rate limiting is an **infrastructure concern**, not an application concern. The proxy layer (Ingress/Router) is the correct place for cluster-wide rate limiting.

---

## Implementation

### Phase 1: Add Proxy Rate Limiting (Immediate)

**Kubernetes** - Update `deploy/integration/ingress/ingress-nginx.yaml`:
```yaml
annotations:
  nginx.ingress.kubernetes.io/limit-rps: "100"
  nginx.ingress.kubernetes.io/limit-connections: "50"
```

**OpenShift** - Update `deploy/manifests/route.yaml`:
```yaml
annotations:
  haproxy.router.openshift.io/rate-limit-connections: "true"
  haproxy.router.openshift.io/rate-limit-connections.rate-http: "1000"
```

### Phase 2: Deprecate Gateway Rate Limiting (V1.0)

**Files to Remove**:
- `pkg/gateway/middleware/ratelimit.go` - Rate limiting middleware
- Related test files
- Redis rate limiting configuration

**Files to Update**:
- `pkg/gateway/server.go` - Remove rate limiting middleware chain
- `pkg/gateway/config/config.go` - Remove rate limiting config
- Gateway documentation

### Phase 3: Remove Redis Dependency (After DD-GATEWAY-011)

Once deduplication and storm aggregation move to RR status (per DD-GATEWAY-011), Redis can be fully removed from Gateway.

---

## Consequences

### Positive

- ✅ **Zero Redis for rate limiting** - infrastructure simplification
- ✅ **Global rate limiting** - exact cluster-wide enforcement
- ✅ **Reduced code** - ~130 lines removed
- ✅ **Reduced testing** - no rate limiting unit/integration tests
- ✅ **Platform alignment** - uses standard K8s/OCP patterns
- ✅ **Crash-proof** - proxy independent of Gateway pods

### Negative

- ⚠️ **Less granular** - per-route, not per-endpoint
  - **Mitigation**: Create separate Ingress/Route per endpoint if needed
- ⚠️ **Platform dependency** - requires Ingress Controller or Router
  - **Mitigation**: Standard in all production clusters

### Neutral

- 🔄 BR-GATEWAY-071/072 still satisfied (via proxy)
- 🔄 DoS protection maintained (via proxy)

---

## Validation

### Success Criteria

- [ ] Rate limiting annotations added to Ingress/Route
- [ ] Gateway rate limiting code deprecated
- [ ] Integration tests verify proxy rate limiting works
- [ ] Documentation updated

### Metrics

- **Rate limit effectiveness**: Verify 429 responses from proxy
- **Latency impact**: No change (proxy already in path)
- **Code reduction**: ~130 lines removed from Gateway

---

## Related Decisions

- **Enables**: DD-GATEWAY-011 (Redis deprecation for Gateway)
- **Supports**: BR-GATEWAY-071, BR-GATEWAY-072 (rate limiting requirements)
- **Aligns with**: ADR-048 (infrastructure delegation pattern)

---

## Acknowledgments

| Team | Status | Date | Notes |
|------|--------|------|-------|
| Gateway Service | ✅ **OWNER** | 2025-12-07 | Created this ADR. Will remove rate limiting code. |
| Architecture Team | ✅ **APPROVED** | 2025-12-07 | Aligns with DD-GATEWAY-011 Redis deprecation |
| RO Team | ✅ **ACKNOWLEDGED** | 2025-12-07 | No impact on RO |

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2025-12-07 | Gateway Team | Initial decision - delegate rate limiting to proxy |
| v1.1 | 2025-12-07 | All Teams | Acknowledgments added |


