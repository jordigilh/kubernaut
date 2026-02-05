# ADR-048 Addendum 001: Chi Throttle Middleware for Defense-in-Depth

**Date**: January 29, 2026  
**Status**: ✅ **APPROVED**  
**Supersedes**: Nothing (enhances ADR-048)  
**Related**: ADR-048 (Rate Limiting Proxy Delegation), DD-GATEWAY-014 (Circuit Breaker Deferral)

---

## Context

After ADR-048 was approved (Dec 7, 2025), we discovered chi router's built-in `Throttle` middleware provides simple per-pod concurrency limiting with **zero custom code**. This was not known when ADR-048 was written.

**ADR-048 Decision**: Remove Redis-based rate limiting (~130 lines custom code), delegate to Nginx Ingress/HAProxy Router (infrastructure layer).

**Gap Discovered**: E2E tests bypass Ingress/Route completely, sending requests directly to Gateway pods via NodePort:

```
E2E Test → localhost:8080 (NodePort) → Gateway Pod
            ↑
            No Ingress/Route in the path!
```

**Impact**: Rate limit stress test sends 50 rapid requests → Gateway overload → system crash → 15 test failures.

---

## Problem Statement

**Without application-layer concurrency control**:

1. **E2E Tests Unprotected**: Tests bypass Ingress → no rate limiting → system crashes
2. **Misconfigured Ingress Risk**: If Ingress annotations are wrong/missing → Gateway exposed
3. **Single Layer Protection**: Nginx/HAProxy alone = single point of failure

**With chi's built-in middleware** (discovered post-ADR-048):

```go
// That's it! No custom code to maintain
r.Use(chimiddleware.Throttle(100))
```

---

## Decision

**APPROVED: Add chi Throttle middleware as defense-in-depth layer**

### Architecture (Two Layers)

**Layer 1: Nginx/HAProxy (Primary)** - Per ADR-048
```yaml
# Cluster-wide rate limiting
annotations:
  nginx.ingress.kubernetes.io/limit-rps: "100"
  nginx.ingress.kubernetes.io/limit-connections: "50"
```

**Layer 2: Chi Throttle (Secondary)** - NEW
```go
// Per-pod concurrency limiting
r.Use(chimiddleware.Throttle(config.MaxConcurrentRequests))
```

### When Each Layer Protects

| Scenario | Nginx/HAProxy | Chi Throttle |
|----------|---------------|--------------|
| Normal production traffic | ✅ Primary defense | ⏸️ Standby |
| E2E tests (bypass Ingress) | ❌ Not in path | ✅ Protects |
| Misconfigured Ingress | ❌ Failed | ✅ Protects |
| Internal cluster traffic | ❌ Not in path | ✅ Protects |

---

## Implementation

### 1. Configuration (ADR-030 Compliant)

**File**: `pkg/gateway/config/config.go`

```go
type ServerSettings struct {
	ListenAddr            string        `yaml:"listenAddr"`
	MaxConcurrentRequests int           `yaml:"maxConcurrentRequests"` // NEW
	ReadTimeout           time.Duration `yaml:"readTimeout"`
	WriteTimeout          time.Duration `yaml:"writeTimeout"`
	IdleTimeout           time.Duration `yaml:"idleTimeout"`
}

// In LoadFromFile() - apply defaults
if cfg.Server.MaxConcurrentRequests == 0 {
	cfg.Server.MaxConcurrentRequests = 100 // Sensible default
}

// In Validate() - ensure valid range
if c.Server.MaxConcurrentRequests < 0 {
	return error("must be >= 0")
}
```

### 2. Middleware Integration

**File**: `pkg/gateway/server.go`

```go
func (s *Server) setupRoutes() chi.Router {
	r := chi.NewRouter()

	// CORS
	r.Use(kubecors.Handler(corsOpts))

	// ADR-048-ADDENDUM-001: Concurrency limiting
	if s.config.Server.MaxConcurrentRequests > 0 {
		s.logger.Info("Concurrency throttling enabled",
			"max_concurrent_requests", s.config.Server.MaxConcurrentRequests)
		r.Use(chimiddleware.Throttle(s.config.Server.MaxConcurrentRequests))
	}

	// Existing middleware...
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	// ...
}
```

### 3. Configuration Files

**Production** (`config/gateway.yaml`):
```yaml
server:
  listenAddr: ":8080"
  maxConcurrentRequests: 100  # Defense-in-depth with Nginx/HAProxy
  readTimeout: "30s"
  writeTimeout: "30s"
  idleTimeout: "120s"
```

**E2E Tests** (`test/e2e/gateway/gateway-deployment.yaml`):
```yaml
server:
  listenAddr: ":8080"
  maxConcurrentRequests: 100  # Prevents test overload (bypasses Ingress)
  readTimeout: 30s
  writeTimeout: 30s
```

---

## Rationale

### Why This Complements (Not Contradicts) ADR-048

**ADR-048 Goal**: Remove complex custom rate limiting code (Redis-based, ~130 lines)

**This Addendum**:
- ✅ **Zero custom code** - uses chi's built-in 1-line middleware
- ✅ **No Redis** - still aligns with ADR-048's goal
- ✅ **Defense-in-depth** - complements Nginx/HAProxy, doesn't replace it
- ✅ **Simple configuration** - single YAML field per ADR-030

**Key Difference**:
- **ADR-048 rejected**: Complex custom rate limiting with Redis
- **This addendum adds**: Simple built-in throttling with zero dependencies

### Why Chi Throttle Is Different from Redis Rate Limiting

| Feature | Old (Redis Rate Limiting) | New (Chi Throttle) |
|---------|---------------------------|---------------------|
| **Code complexity** | ~130 lines custom code | 1 line (`r.Use(...)`) |
| **External dependencies** | Redis required | None |
| **Maintenance burden** | High (custom logic) | Zero (chi built-in) |
| **Testing burden** | Unit + integration tests | Zero (chi tested) |
| **State management** | Redis keys, sliding window | In-memory counter |
| **Crash recovery** | State lost | N/A (stateless) |
| **Per-pod limits** | Yes (problematic) | Yes (acceptable) |

**Conclusion**: Chi throttle is fundamentally simpler than the Redis approach ADR-048 rejected.

---

## Behavior

### HTTP 503 Response (Limit Exceeded)

When `MaxConcurrentRequests` limit is reached:

**Request**:
```http
POST /api/v1/signals/prometheus HTTP/1.1
```

**Response**:
```http
HTTP/1.1 503 Service Unavailable
Content-Type: text/plain

Service at capacity. Retry after a brief wait.
```

**Client Action**: Retry with exponential backoff (standard HTTP 503 behavior)

### Configuration Values

| Value | Behavior | Use Case |
|-------|----------|----------|
| `0` | Unlimited (no throttling) | NOT RECOMMENDED |
| `50` | Conservative (low-traffic environments) | Dev/staging |
| `100` | **Default** (recommended) | Production |
| `500` | Aggressive (high-traffic) | Large clusters |
| `1000+` | Very aggressive | Special cases only |

**Default**: 100 concurrent requests (applied if not specified in YAML)

---

## Validation

### Test Evidence (January 29, 2026)

**Before This Change**:
```
Rate Limit Test: 50 rapid requests → Gateway crash at request #19
Result: 79 passed, 15 failed (cascade failure)
```

**After This Change** (Expected):
```
Rate Limit Test: 50 rapid requests → First 100 succeed, rest get HTTP 503
Result: Graceful degradation, no crash
```

### E2E Test Strategy

1. **Test Focus**: `make test-e2e-gateway` (full suite)
2. **Validation**: Rate limit test should gracefully handle overload
3. **Metric**: 0 system crashes, proper 503 responses for excess requests

---

## Positive Consequences

1. ✅ **E2E Test Safety**: Tests no longer crash Gateway
2. ✅ **Defense-in-Depth**: Two layers of protection (Nginx/HAProxy + chi)
3. ✅ **Zero Custom Code**: Uses chi's built-in, battle-tested middleware
4. ✅ **Simple Configuration**: Single YAML field per ADR-030
5. ✅ **ADR-048 Compliant**: Still no Redis, no complex custom code
6. ✅ **Production Safe**: Protects against misconfigured Ingress

---

## Negative Consequences

1. ⚠️ **Per-Pod Limits**: Each pod has independent 100-request limit
   - **Impact**: 3 replicas = 300 total concurrent (not 100)
   - **Mitigation**: Acceptable for defense-in-depth layer; Nginx/HAProxy enforces global limit

---

## Compliance

- ✅ **ADR-030**: Configuration via YAML only (no environment variables for functional config)
- ✅ **camelCase Convention**: YAML fields use camelCase per `docs/architecture/CRD_FIELD_NAMING_CONVENTION.md`
- ✅ **ADR-048**: Still delegates primary rate limiting to Nginx/HAProxy
- ✅ **DD-GATEWAY-014**: Aligns with "defense-in-depth" rationale for circuit breaker

---

## References

- **ADR-048**: Rate Limiting Proxy Delegation (Primary decision)
- **ADR-030**: Service Configuration Management (YAML-only functional config)
- **Chi Throttle Middleware**: https://github.com/go-chi/chi/blob/master/middleware/throttle.go
- **E2E Test Failure Analysis**: `docs/handoff/GATEWAY_E2E_COMPLETE_FIX_JAN_29_2026.md`

---

## Acknowledgments

| Team | Status | Date | Notes |
|------|--------|------|-------|
| Gateway Team | ✅ **OWNER** | 2026-01-29 | Discovered chi middleware post-ADR-048 |
| Architecture Team | ✅ **APPROVED** | 2026-01-29 | Defense-in-depth aligns with DD-GATEWAY-014 |

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2026-01-29 | Gateway Team | Initial addendum - add chi throttle for defense-in-depth |
