# TokenReview Optimization: Avoiding K8s API Throttling

**Date**: 2025-10-24
**Problem**: TokenReview API calls are being throttled (1-29s waits)
**Impact**: 503 errors, 62% test failure rate
**Goal**: Reduce K8s API load while maintaining security

---

## 🎯 **PROBLEM STATEMENT**

**Current Implementation**:
- **Every webhook request** → 1 TokenReview API call
- **100 concurrent requests** → 100 TokenReview API calls
- **Result**: K8s API throttling → 1-29s waits → 503 errors

**Requirements**:
- ✅ Maintain security (validate ServiceAccount tokens)
- ✅ Reduce K8s API load (avoid throttling)
- ✅ Minimize request rejection (avoid 503 errors)
- ✅ Low latency (<100ms per request)

---

## 🔍 **ALTERNATIVE APPROACHES**

### **Option A: TokenReview Result Caching** (RECOMMENDED)

**Approach**: Cache TokenReview results with short TTL (1-5 minutes)

**How it works**:
1. First request with token → TokenReview API call → cache result
2. Subsequent requests with same token → read from cache (no API call)
3. Cache expires after TTL → next request triggers new TokenReview

**Implementation**:
```go
type TokenCache struct {
    cache map[string]*CachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type CachedTokenReview struct {
    Username      string
    Authenticated bool
    ExpiresAt     time.Time
}

func (tc *TokenCache) ValidateToken(ctx context.Context, token string, k8sClient kubernetes.Interface) (*CachedTokenReview, error) {
    // Check cache first
    tc.mu.RLock()
    if cached, ok := tc.cache[token]; ok && time.Now().Before(cached.ExpiresAt) {
        tc.mu.RUnlock()
        return cached, nil
    }
    tc.mu.RUnlock()

    // Cache miss or expired → call TokenReview API
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    tr := &authv1.TokenReview{
        Spec: authv1.TokenReviewSpec{Token: token},
    }

    result, err := k8sClient.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})
    if err != nil {
        return nil, err
    }

    // Cache the result
    cached := &CachedTokenReview{
        Username:      result.Status.User.Username,
        Authenticated: result.Status.Authenticated,
        ExpiresAt:     time.Now().Add(tc.ttl),
    }

    tc.mu.Lock()
    tc.cache[token] = cached
    tc.mu.Unlock()

    return cached, nil
}
```

**Pros**:
- ✅ **Massive K8s API load reduction**: 100 requests → 1-5 TokenReview calls (95-99% reduction)
- ✅ **Low latency**: Cache hits are <1ms (vs 100-1000ms for API calls)
- ✅ **Simple implementation**: ~100 lines of code
- ✅ **No infrastructure changes**: Works with existing K8s API
- ✅ **Graceful degradation**: Falls back to API on cache miss

**Cons**:
- ⚠️ **Token revocation delay**: Revoked tokens remain valid until cache expires (1-5 min)
- ⚠️ **Memory usage**: Stores token hashes in memory (mitigated by TTL and LRU eviction)
- ⚠️ **Cache invalidation complexity**: No automatic invalidation on ServiceAccount deletion

**Security Considerations**:
- **Token revocation delay**: 1-5 minute window where revoked tokens are still valid
  - **Mitigation**: Use short TTL (1-2 minutes for production)
  - **Risk**: LOW - ServiceAccount tokens are rarely revoked in normal operation
- **Token storage**: Store token hash (SHA-256), not plaintext
  - **Mitigation**: `cache[sha256(token)]` instead of `cache[token]`
- **Memory exhaustion**: Unbounded cache could consume all memory
  - **Mitigation**: LRU eviction policy with max size (e.g., 10,000 entries)

**Recommended TTL**:
- **Development/Testing**: 5 minutes (balance between cache hits and security)
- **Production**: 1-2 minutes (minimize revocation delay)
- **High-security environments**: 30 seconds (near real-time revocation)

**Expected Impact**:
- **K8s API calls**: 95-99% reduction (100 requests → 1-5 API calls)
- **Latency**: <1ms for cache hits (vs 100-1000ms for API calls)
- **Pass rate**: >95% (throttling eliminated)

**Confidence**: 95%

---

### **Option B: ServiceAccount Informer Pattern** (COMPLEX)

**Approach**: Use Kubernetes Informer to watch ServiceAccount and Secret resources, validate tokens locally

**How it works**:
1. Start Informer to watch ServiceAccount and Secret resources
2. Build local cache of ServiceAccount → Secret mappings
3. On webhook request, extract token and validate against local cache
4. No TokenReview API calls needed

**Implementation Complexity**:
```go
// Pseudo-code (simplified)
type ServiceAccountCache struct {
    informer cache.SharedIndexInformer
    secrets  map[string]*corev1.Secret
}

func (sac *ServiceAccountCache) ValidateToken(token string) (string, bool) {
    // Extract ServiceAccount from token JWT claims
    // Look up ServiceAccount in local cache
    // Validate token signature against cached Secret
    // Return username and authenticated status
}
```

**Pros**:
- ✅ **Zero TokenReview API calls**: All validation is local
- ✅ **Real-time token revocation**: Informer updates on ServiceAccount/Secret deletion
- ✅ **Lowest latency**: <1ms for all requests (no API calls)
- ✅ **Scalable**: Handles 1000+ req/s without K8s API load

**Cons**:
- ❌ **Very complex**: Requires JWT parsing, signature validation, Informer setup (~500+ lines)
- ❌ **Token format assumptions**: Assumes ServiceAccount tokens are JWTs (may break with projected tokens)
- ❌ **Secret access required**: Gateway needs RBAC to list/watch Secrets (security risk)
- ❌ **Memory overhead**: Caches all ServiceAccounts and Secrets in memory
- ❌ **Informer startup delay**: 5-10 seconds for initial cache sync
- ❌ **Maintenance burden**: Complex code to maintain and debug

**Security Considerations**:
- **Secret access**: Gateway needs `list/watch` on Secrets (broad permissions)
  - **Risk**: HIGH - Secrets contain sensitive data (tokens, passwords, certificates)
  - **Mitigation**: Scope to specific namespace, but still risky
- **JWT parsing vulnerabilities**: Custom JWT validation may have security bugs
  - **Risk**: MEDIUM - TokenReview API is battle-tested, custom code is not
  - **Mitigation**: Use well-tested JWT libraries (e.g., `github.com/golang-jwt/jwt`)

**Recommended**: ❌ **NOT RECOMMENDED**
- Complexity outweighs benefits
- Security risk (Secret access)
- TokenReview API is designed for this use case

**Confidence**: 60% (works, but too complex)

---

### **Option C: Webhook Token Authentication** (KUBERNETES NATIVE)

**Approach**: Configure Kubernetes API server to use Gateway as a webhook authenticator

**How it works**:
1. Configure K8s API server with `--authentication-token-webhook-config-file`
2. K8s API server calls Gateway webhook to validate tokens
3. Gateway validates tokens locally (no TokenReview needed)

**Pros**:
- ✅ **Kubernetes native**: Uses built-in webhook authentication
- ✅ **Zero TokenReview API calls**: K8s API server calls Gateway instead
- ✅ **Centralized authentication**: Single source of truth

**Cons**:
- ❌ **Requires K8s API server reconfiguration**: Not feasible in managed clusters (EKS, GKE, AKS)
- ❌ **Circular dependency**: Gateway authenticates requests, but K8s API server needs Gateway to authenticate
- ❌ **Operational complexity**: Requires cluster admin access and API server restart

**Recommended**: ❌ **NOT FEASIBLE**
- Requires cluster admin access
- Not supported in managed Kubernetes (EKS, GKE, AKS, OpenShift)
- Circular dependency issue

**Confidence**: 30% (works in theory, impractical)

---

### **Option D: Client Certificate Authentication** (ALTERNATIVE AUTH)

**Approach**: Use client certificates (mTLS) instead of ServiceAccount tokens

**How it works**:
1. Issue client certificates to webhook senders
2. Configure Gateway to require client certificates
3. Validate certificates locally (no K8s API calls)

**Pros**:
- ✅ **Zero K8s API calls**: Certificate validation is local
- ✅ **Industry standard**: mTLS is widely used
- ✅ **High security**: Certificate-based auth is more secure than tokens

**Cons**:
- ❌ **Breaking change**: Requires all webhook senders to use certificates
- ❌ **Certificate management**: Requires PKI infrastructure (cert rotation, revocation)
- ❌ **Not Kubernetes native**: Doesn't integrate with ServiceAccount RBAC

**Recommended**: ❌ **NOT RECOMMENDED**
- Breaking change for existing integrations
- Adds operational complexity (PKI management)
- Doesn't leverage Kubernetes RBAC

**Confidence**: 50% (works, but breaks existing integrations)

---

### **Option E: Rate Limiting + Timeout (BASELINE FIX)**

**Approach**: Add timeout to TokenReview calls + implement request queuing

**How it works**:
1. Add 5s timeout to TokenReview API calls
2. Implement request queue with backpressure
3. Reject requests with 503 when queue is full

**Pros**:
- ✅ **Simple**: ~50 lines of code
- ✅ **No caching complexity**: No cache invalidation issues
- ✅ **Fail fast**: 5s timeout prevents indefinite waits

**Cons**:
- ⚠️ **Still calls K8s API**: Doesn't reduce API load
- ⚠️ **Still throttled**: K8s API throttling still occurs
- ⚠️ **503 errors**: Requests still rejected when throttled

**Recommended**: ✅ **BASELINE FIX** (implement first, then add caching)

**Confidence**: 99%

---

## 🎯 **RECOMMENDED SOLUTION**

### **Hybrid Approach: Option E + Option A**

**Phase 1: Baseline Fix (Option E)** - 30 minutes
1. Add 5s timeout to TokenReview/SubjectAccessReview calls
2. Fail fast instead of hanging indefinitely
3. **Expected**: 503 errors reduced, but still some throttling

**Phase 2: Caching (Option A)** - 2 hours
1. Implement TokenReview result caching with 2-minute TTL
2. Use SHA-256 token hashes as cache keys
3. Implement LRU eviction (max 10,000 entries)
4. **Expected**: 95-99% K8s API load reduction, <1ms latency

**Phase 3: Metrics & Monitoring** - 1 hour
1. Add Prometheus metrics:
   - `gateway_tokenreview_cache_hits_total`
   - `gateway_tokenreview_cache_misses_total`
   - `gateway_tokenreview_api_calls_total`
   - `gateway_tokenreview_api_latency_seconds`
2. Add logging for cache hits/misses
3. Monitor cache hit rate (target >95%)

---

## 📊 **COMPARISON MATRIX**

| Option | K8s API Reduction | Latency | Complexity | Security Risk | Recommended |
|---|---|---|---|---|---|
| **A: Caching** | 95-99% | <1ms (cache hit) | Low | Low (1-5 min revocation delay) | ✅ **YES** |
| **B: Informer** | 100% | <1ms | Very High | High (Secret access) | ❌ NO |
| **C: Webhook Auth** | 100% | <1ms | High | Low | ❌ NO (infeasible) |
| **D: mTLS** | 100% | <1ms | Medium | Low | ❌ NO (breaking change) |
| **E: Timeout** | 0% | 100-1000ms | Very Low | None | ✅ **BASELINE** |

---

## 🚀 **IMPLEMENTATION PLAN**

### **Phase 1: Baseline Fix (30 minutes) - IMMEDIATE**

**Goal**: Stop 503 errors from indefinite waits

**Changes**:
1. Add 5s timeout to `TokenReviewAuth` (`pkg/gateway/middleware/auth.go`)
2. Add 5s timeout to `SubjectAccessReviewAuthz` (`pkg/gateway/middleware/authz.go`)
3. Add error handling for `context.DeadlineExceeded`

**Expected Results**:
- 503 errors reduced (fail fast at 5s instead of hanging)
- Some requests still throttled (but predictable)

---

### **Phase 2: TokenReview Caching (2 hours) - FOLLOW-UP**

**Goal**: Eliminate K8s API throttling

**Changes**:
1. Create `pkg/gateway/middleware/token_cache.go`:
   - `TokenCache` struct with `sync.RWMutex`
   - `ValidateToken()` method with cache-first logic
   - LRU eviction policy (max 10,000 entries)
   - SHA-256 token hashing for cache keys
2. Update `TokenReviewAuth` to use `TokenCache`
3. Add configuration for TTL (default 2 minutes)

**Expected Results**:
- 95-99% K8s API call reduction
- <1ms latency for cache hits
- >95% test pass rate

---

### **Phase 3: Metrics & Monitoring (1 hour) - FOLLOW-UP**

**Goal**: Observability for cache performance

**Changes**:
1. Add Prometheus metrics to `token_cache.go`
2. Add structured logging for cache operations
3. Add Grafana dashboard for cache metrics

**Expected Results**:
- Real-time visibility into cache hit rate
- Alerts for low cache hit rate (<90%)
- Performance monitoring

---

## ✅ **DECISION MATRIX**

### **Immediate Action (Today)**

**Implement**: ✅ **Phase 1: Baseline Fix (Option E)**
- **Why**: Stops indefinite waits, fails fast
- **Time**: 30 minutes
- **Risk**: Very low
- **Impact**: Reduces 503 errors

### **Follow-Up (This Week)**

**Implement**: ✅ **Phase 2: TokenReview Caching (Option A)**
- **Why**: Eliminates K8s API throttling
- **Time**: 2 hours
- **Risk**: Low (1-5 min revocation delay)
- **Impact**: 95-99% K8s API load reduction

### **Future (Next Sprint)**

**Implement**: ✅ **Phase 3: Metrics & Monitoring**
- **Why**: Observability and performance tracking
- **Time**: 1 hour
- **Risk**: None
- **Impact**: Better visibility into system behavior

---

## 🔒 **SECURITY CONSIDERATIONS**

### **Token Caching Security**

**Threat**: Revoked ServiceAccount tokens remain valid until cache expires

**Mitigation**:
1. **Short TTL**: 1-2 minutes (production), 5 minutes (dev/test)
2. **Cache invalidation API**: Endpoint to manually flush cache on token revocation
3. **Metrics**: Track cache age and alert on stale entries

**Risk Assessment**: **LOW**
- ServiceAccount tokens are rarely revoked in normal operation
- 1-2 minute delay is acceptable for most use cases
- Emergency cache flush available if needed

---

## 📈 **EXPECTED OUTCOMES**

### **After Phase 1 (Baseline Fix)**
- **Pass Rate**: 70-80% (improved from 38%)
- **503 Errors**: Reduced (fail fast at 5s)
- **K8s API Load**: Same (still calls API)

### **After Phase 2 (Caching)**
- **Pass Rate**: >95%
- **503 Errors**: <5%
- **K8s API Load**: 95-99% reduction
- **Latency**: <1ms (cache hits)

### **After Phase 3 (Metrics)**
- **Observability**: Real-time cache performance
- **Alerts**: Low cache hit rate detection
- **Debugging**: Cache behavior visibility

---

## 🎯 **CONFIDENCE ASSESSMENT**

**Overall Confidence**: 95%

**Justification**:
- **Option A (Caching)** is proven pattern (used by kubectl, client-go)
- **Security risk is low** (1-2 min revocation delay acceptable)
- **Implementation is straightforward** (~200 lines of code)
- **Expected impact is high** (95-99% API load reduction)

**Risk**: Low
- Caching is well-understood pattern
- Short TTL minimizes revocation delay
- Graceful degradation (falls back to API on cache miss)

---

## 📚 **REFERENCES**

- [Kubernetes TokenReview API](https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/)
- [Client-Go Caching](https://github.com/kubernetes/client-go/tree/master/tools/cache)
- [Go sync.Map for Concurrent Caching](https://pkg.go.dev/sync#Map)
- [LRU Cache Implementation](https://github.com/hashicorp/golang-lru)


