# K8s API Throttling Solutions

## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation



## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation

# K8s API Throttling Solutions

## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation

# K8s API Throttling Solutions

## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation



## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation

# K8s API Throttling Solutions

## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation

# K8s API Throttling Solutions

## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation



## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation

# K8s API Throttling Solutions

## 🚨 **PROBLEM STATEMENT**

Integration tests are hitting K8s API rate limits when running 50+ concurrent webhook requests, each requiring:
1. **TokenReview** API call (authentication)
2. **SubjectAccessReview** API call (authorization)

**Result**: Tests fail with 503 errors due to K8s API throttling.

---

## ✅ **SOLUTION 1: Token Caching (RECOMMENDED)**

### Approach
Cache TokenReview results for valid tokens with short TTL (5 minutes).

### Implementation

**File**: `pkg/gateway/middleware/token_cache.go` (NEW)
```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"

    authv1 "k8s.io/api/authentication/v1"
)

// TokenCache caches TokenReview results to reduce K8s API calls
// BR-GATEWAY-066: Authentication with caching for performance
type TokenCache struct {
    cache map[string]*cachedTokenReview
    mu    sync.RWMutex
    ttl   time.Duration
}

type cachedTokenReview struct {
    review    *authv1.TokenReview
    expiresAt time.Time
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        cache: make(map[string]*cachedTokenReview),
        ttl:   ttl,
    }
}

// Get retrieves cached TokenReview if valid
func (tc *TokenCache) Get(token string) (*authv1.TokenReview, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    key := tc.hashToken(token)
    cached, exists := tc.cache[key]

    if !exists || time.Now().After(cached.expiresAt) {
        return nil, false
    }

    return cached.review, true
}

// Set stores TokenReview result in cache
func (tc *TokenCache) Set(token string, review *authv1.TokenReview) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    key := tc.hashToken(token)
    tc.cache[key] = &cachedTokenReview{
        review:    review,
        expiresAt: time.Now().Add(tc.ttl),
    }
}

// hashToken creates SHA256 hash of token for cache key
func (tc *TokenCache) hashToken(token string) string {
    hash := sha256.Sum256([]byte(token))
    return hex.EncodeToString(hash[:])
}

// Cleanup removes expired entries (run periodically)
func (tc *TokenCache) Cleanup() {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    now := time.Now()
    for key, cached := range tc.cache {
        if now.After(cached.expiresAt) {
            delete(tc.cache, key)
        }
    }
}
```

**File**: `pkg/gateway/middleware/auth.go` (MODIFY)
```go
var tokenCache = NewTokenCache(5 * time.Minute) // 5 minute TTL

func TokenReviewAuth(clientset kubernetes.Interface, metrics *gatewayMetrics.Metrics) func(http.Handler) http.Handler {
    // Start cleanup goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            tokenCache.Cleanup()
        }
    }()

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)

            // Check cache first
            if cachedReview, found := tokenCache.Get(token); found {
                if cachedReview.Status.Authenticated {
                    // Use cached result - NO K8s API call
                    ctx := context.WithValue(r.Context(), "username", cachedReview.Status.User.Username)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Cache miss - perform TokenReview
            review := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := clientset.AuthenticationV1().TokenReviews().Create(r.Context(), review, metav1.CreateOptions{})
            if err != nil {
                http.Error(w, "Authentication failed", http.StatusUnauthorized)
                return
            }

            // Cache the result
            tokenCache.Set(token, result)

            if !result.Status.Authenticated {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "username", result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Pros**:
- ✅ Reduces K8s API calls by 95%+ (5 minute cache)
- ✅ Maintains full security (tokens are validated)
- ✅ No changes to test infrastructure needed
- ✅ Production-ready solution

**Cons**:
- ⚠️ Token revocation takes up to 5 minutes to propagate
- ⚠️ Memory usage for cache (minimal: ~1KB per token)

**Confidence**: 95%

---

## ✅ **SOLUTION 2: Batch TokenReview (ALTERNATIVE)**

### Approach
Batch multiple TokenReview requests into a single K8s API call.

**Note**: This requires custom K8s API support and is NOT standard. **Not recommended**.

**Confidence**: 30%

---

## ✅ **SOLUTION 3: ServiceAccount Token Projection (PRODUCTION)**

### Approach
Use ServiceAccount token projection with audience-specific tokens for production deployments.

**File**: `deploy/kubernetes/gateway-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        volumeMounts:
        - name: token
          mountPath: /var/run/secrets/tokens
          readOnly: true
      volumes:
      - name: token
        projected:
          sources:
          - serviceAccountToken:
              path: gateway-token
              expirationSeconds: 3600
              audience: gateway.kubernaut.io
```

**Pros**:
- ✅ Reduces TokenReview calls (tokens are pre-validated)
- ✅ Better security (audience-specific tokens)
- ✅ Standard Kubernetes feature

**Cons**:
- ⚠️ Requires Kubernetes 1.20+
- ⚠️ Doesn't help with integration tests

**Confidence**: 85% (for production)

---

## ✅ **SOLUTION 4: Increase K8s API QPS/Burst (TEMPORARY)**

### Approach
Increase client-side rate limits for integration tests.

**File**: `test/integration/gateway/helpers.go` (ALREADY IMPLEMENTED)
```go
config.QPS = 50    // Default: 5
config.Burst = 100 // Default: 10
```

**Pros**:
- ✅ Already implemented
- ✅ Helps with concurrent tests

**Cons**:
- ❌ Doesn't solve server-side throttling
- ❌ Can overwhelm K8s API server

**Confidence**: 40%

---

## 🎯 **RECOMMENDED SOLUTION**

**Implement Solution 1 (Token Caching)** immediately:

1. ✅ **Add TokenCache** (15 minutes)
   - Create `pkg/gateway/middleware/token_cache.go`
   - Implement cache with 5-minute TTL
   - Add cleanup goroutine

2. ✅ **Modify TokenReviewAuth** (10 minutes)
   - Check cache before K8s API call
   - Store results in cache
   - Maintain security guarantees

3. ✅ **Add SubjectAccessReview Cache** (5 minutes)
   - Same pattern for authorization
   - Cache permission checks

4. ✅ **Run Integration Tests** (5 minutes)
   - Verify 95%+ reduction in K8s API calls
   - Confirm all tests pass

**Total Time**: 35 minutes

**Expected Result**:
- First request: 2 K8s API calls (TokenReview + SubjectAccessReview)
- Next 50 requests with same token: 0 K8s API calls (cached)
- **95%+ reduction in K8s API load**

---

## 📊 **CACHE INVALIDATION STRATEGY**

### When to Invalidate Cache

1. **Time-based** (IMPLEMENTED)
   - 5-minute TTL for TokenReview
   - 5-minute TTL for SubjectAccessReview

2. **Event-based** (FUTURE)
   - Watch ServiceAccount deletions
   - Watch RoleBinding changes
   - Invalidate affected cache entries

3. **Manual** (FUTURE)
   - Admin API to flush cache
   - Useful for security incidents

---

## ✅ **SUCCESS CRITERIA**

After implementing Solution 1:

1. ✅ Integration tests pass without K8s API throttling
2. ✅ Authentication remains ALWAYS enabled (no DisableAuth)
3. ✅ K8s API call reduction: >95%
4. ✅ Security maintained: tokens validated on first use
5. ✅ Production-ready: cache TTL prevents stale permissions

---

## 🔗 **RELATED DECISIONS**

This should be documented as:
- **DD-GATEWAY-004**: Token Caching for K8s API Throttling Mitigation




