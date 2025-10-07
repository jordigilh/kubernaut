# Rate Limiting Standard - Kubernaut Services

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All 6 Stateless HTTP Services

---

## 📋 **Rate Limiting Strategy**

### **Two-Tier Rate Limiting**

1. **Per-Client Rate Limiting** (Primary): Based on ServiceAccount identity from TokenReviewer
2. **Per-Replica Rate Limiting** (Secondary): Global limit per service replica

---

## 🎯 **Standardized Limits**

### **By Service Type**

| Service | Per-Client Limit | Per-Replica Limit | Burst | Rationale |
|---------|-----------------|-------------------|-------|-----------|
| **Gateway** | 100 req/s per source | 1000 req/s | 150 | High ingestion volume |
| **Context API** | 50 req/s per client | 1000 req/s | 75 | Read-heavy queries |
| **Data Storage** | 20 writes/s per client | 500 writes/s | 30 | Write operations expensive |
| **HolmesGPT API** | **5 investigations/min** | 10 investigations/min | N/A | **LLM cost protection** |
| **Notification** | 10 req/s per client | 100 req/s | 15 | External API limits |
| **Dynamic Toolset** | 10 req/s per client | 100 req/s | 15 | Low traffic service |

---

## 🔴 **Critical: HolmesGPT API Rate Limiting**

### **Problem Identified**
HolmesGPT API has **NO rate limiting** specified → Risk of **LLM cost explosion**

### **Solution**
```go
// pkg/holmesgpt/middleware/rate_limit.go
package middleware

import (
    "net/http"
    "sync"
    "time"

    "golang.org/x/time/rate"
    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/correlation"
)

type InvestigationRateLimiter struct {
    // Per-client limiters (keyed by ServiceAccount name)
    clients map[string]*rate.Limiter
    mu      sync.RWMutex

    // Per-client limit: 5 investigations per minute
    clientRate  rate.Limit
    clientBurst int

    // Global replica limit: 10 investigations per minute
    globalLimiter *rate.Limiter

    logger *zap.Logger
}

func NewInvestigationRateLimiter(logger *zap.Logger) *InvestigationRateLimiter {
    return &InvestigationRateLimiter{
        clients:     make(map[string]*rate.Limiter),
        clientRate:  rate.Every(12 * time.Second), // 5 per minute
        clientBurst: 1,                             // No burst for investigations
        globalLimiter: rate.NewLimiter(
            rate.Every(6 * time.Second), // 10 per minute
            2,                           // Small burst
        ),
        logger: logger,
    }
}

func (rl *InvestigationRateLimiter) getClientLimiter(clientID string) *rate.Limiter {
    rl.mu.RLock()
    limiter, exists := rl.clients[clientID]
    rl.mu.RUnlock()

    if exists {
        return limiter
    }

    // Create new limiter for client
    rl.mu.Lock()
    defer rl.mu.Unlock()

    // Double-check after acquiring write lock
    if limiter, exists := rl.clients[clientID]; exists {
        return limiter
    }

    limiter = rate.NewLimiter(rl.clientRate, rl.clientBurst)
    rl.clients[clientID] = limiter
    return limiter
}

func (rl *InvestigationRateLimiter) Limit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        correlationID := correlation.FromContext(r.Context())
        log := rl.logger.With(
            zap.String("correlation_id", correlationID),
            zap.String("path", r.URL.Path),
        )

        // Extract client identity from TokenReviewer (stored in context)
        clientID, ok := r.Context().Value("serviceaccount").(string)
        if !ok {
            clientID = "unknown"
        }

        // Check global rate limit (per replica)
        if !rl.globalLimiter.Allow() {
            log.Warn("Global rate limit exceeded",
                zap.String("client", clientID),
            )
            http.Error(w, "Too many requests (global limit)", http.StatusTooManyRequests)
            w.Header().Set("Retry-After", "60")
            return
        }

        // Check per-client rate limit
        clientLimiter := rl.getClientLimiter(clientID)
        if !clientLimiter.Allow() {
            log.Warn("Client rate limit exceeded",
                zap.String("client", clientID),
            )
            http.Error(w, "Too many requests (client limit)", http.StatusTooManyRequests)
            w.Header().Set("Retry-After", "60")
            w.Header().Set("X-RateLimit-Limit", "5")
            w.Header().Set("X-RateLimit-Window", "60s")
            return
        }

        // Allow request
        log.Info("Investigation rate limit passed",
            zap.String("client", clientID),
        )
        next.ServeHTTP(w, r)
    })
}
```

**Critical Metrics**:
```
holmesgpt_rate_limit_exceeded_total{client="ai-analysis-sa"} 5
holmesgpt_llm_cost_usd_total{client="ai-analysis-sa"} 12.50
```

---

## 🔧 **Implementation Patterns**

### **Pattern 1: Per-Client Token Bucket** (Recommended)

```go
// pkg/middleware/rate_limit.go
package middleware

import (
    "net/http"
    "sync"

    "golang.org/x/time/rate"
    "go.uber.org/zap"
)

type ClientRateLimiter struct {
    clients map[string]*rate.Limiter  // Key: ServiceAccount name
    mu      sync.RWMutex
    rate    rate.Limit
    burst   int
    logger  *zap.Logger
}

func NewClientRateLimiter(rps int, burst int, logger *zap.Logger) *ClientRateLimiter {
    return &ClientRateLimiter{
        clients: make(map[string]*rate.Limiter),
        rate:    rate.Limit(rps),
        burst:   burst,
        logger:  logger,
    }
}

func (rl *ClientRateLimiter) getLimiter(clientID string) *rate.Limiter {
    rl.mu.RLock()
    limiter, exists := rl.clients[clientID]
    rl.mu.RUnlock()

    if exists {
        return limiter
    }

    rl.mu.Lock()
    defer rl.mu.Unlock()

    limiter = rate.NewLimiter(rl.rate, rl.burst)
    rl.clients[clientID] = limiter
    return limiter
}

func (rl *ClientRateLimiter) Limit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract ServiceAccount from TokenReviewer context
        clientID, ok := r.Context().Value("serviceaccount").(string)
        if !ok {
            clientID = "unknown"
        }

        limiter := rl.getLimiter(clientID)

        if !limiter.Allow() {
            rl.logger.Warn("Rate limit exceeded",
                zap.String("client", clientID),
                zap.String("path", r.URL.Path),
            )

            w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", int(rl.rate)))
            w.Header().Set("X-RateLimit-Remaining", "0")
            w.Header().Set("Retry-After", "1")
            http.Error(w, "Too many requests", http.StatusTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

---

### **Pattern 2: Global Rate Limiter** (Backup/Secondary)

```go
type GlobalRateLimiter struct {
    limiter *rate.Limiter
}

func NewGlobalRateLimiter(rps int, burst int) *GlobalRateLimiter {
    return &GlobalRateLimiter{
        limiter: rate.NewLimiter(rate.Limit(rps), burst),
    }
}

func (rl *GlobalRateLimiter) Limit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !rl.limiter.Allow() {
            http.Error(w, "Service overloaded", http.StatusServiceUnavailable)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

---

## 📊 **Rate Limit Headers**

### **Standard Headers** (All Services)

```http
X-RateLimit-Limit: 100           # Requests per window
X-RateLimit-Remaining: 75        # Remaining requests
X-RateLimit-Reset: 1633459200    # Unix timestamp of reset
X-RateLimit-Window: 60s          # Window duration
Retry-After: 30                  # Seconds until retry (429 responses)
```

**Go Implementation**:
```go
func setRateLimitHeaders(w http.ResponseWriter, limit int, remaining int, reset time.Time) {
    w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
    w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
    w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", reset.Unix()))
}
```

---

## 🎯 **Rate Limiting by Service**

### **1. Gateway Service**

**Strategy**: Per-source rate limiting (already implemented)

```go
// Using golang.org/x/time/rate
rateLimiter := rate.NewLimiter(100, 150)  // 100 req/s, burst 150

// Per-source tracking by fingerprint
sourceLimiters := make(map[string]*rate.Limiter)
```

**Metrics**:
```
gateway_rate_limit_exceeded_total{source="prometheus-alertmanager"} 10
```

---

### **2. Context API Service**

**Strategy**: Per-client + global rate limiting

```go
// Per-client: 50 req/s
clientLimiter := NewClientRateLimiter(50, 75, logger)

// Global: 1000 req/s per replica
globalLimiter := NewGlobalRateLimiter(1000, 1500)

// Chain middlewares
http.Handle("/api/v1/", clientLimiter.Limit(globalLimiter.Limit(handler)))
```

**Metrics**:
```
contextapi_rate_limit_exceeded_total{client="remediation-processor-sa"} 5
```

---

### **3. Data Storage Service**

**Strategy**: Per-client write rate limiting

```go
// Per-client: 20 writes/s (more restrictive due to write cost)
writeLimiter := NewClientRateLimiter(20, 30, logger)

// Global: 500 writes/s per replica
globalWriteLimiter := NewGlobalRateLimiter(500, 750)
```

**Metrics**:
```
datastorage_write_rate_limit_exceeded_total{client="workflow-execution-sa"} 3
```

---

### **4. HolmesGPT API Service** (CRITICAL)

**Strategy**: Investigation-specific rate limiting

```go
// Per-client: 5 investigations/min (LLM cost protection)
investigationLimiter := NewInvestigationRateLimiter(logger)

// Global: 10 investigations/min per replica
```

**Cost Protection**:
- Average investigation: $0.10 (GPT-4)
- Without rate limit: Unlimited cost
- With 5/min limit: Max $0.50/min per client = $30/hour
- Global 10/min limit: Max $1/min = $60/hour per replica

**Metrics**:
```
holmesgpt_rate_limit_exceeded_total{client="ai-analysis-sa"} 12
holmesgpt_llm_cost_prevented_usd{client="ai-analysis-sa"} 15.00
```

---

### **5. Notification Service**

**Strategy**: Per-client + per-channel rate limiting

```go
// Per-client: 10 req/s
notificationLimiter := NewClientRateLimiter(10, 15, logger)

// Per-channel limits (Slack, Teams, etc.)
channelLimiters := map[string]*rate.Limiter{
    "slack": rate.NewLimiter(1, 5),    // Slack API limit
    "teams": rate.NewLimiter(1, 5),    // Teams API limit
    "email": rate.NewLimiter(10, 20),  // Email more permissive
}
```

**Metrics**:
```
notification_rate_limit_exceeded_total{client="workflow-execution-sa",channel="slack"} 8
```

---

### **6. Dynamic Toolset Service**

**Strategy**: Low-traffic service, simple rate limiting

```go
// Per-client: 10 req/s (discovery triggers rare)
toolsetLimiter := NewClientRateLimiter(10, 15, logger)

// Global: 100 req/s per replica
globalLimiter := NewGlobalRateLimiter(100, 150)
```

---

## 📊 **Monitoring & Alerting**

### **Key Metrics** (All Services)

```
# Rate limit exceeded counter
{service}_rate_limit_exceeded_total{client="<serviceaccount>"}

# Current rate limit (gauge)
{service}_rate_limit_current{client="<serviceaccount>"}

# Rate limit window
{service}_rate_limit_window_seconds

# Requests allowed
{service}_requests_allowed_total{client="<serviceaccount>"}

# Requests denied
{service}_requests_denied_total{client="<serviceaccount>"}
```

---

### **Prometheus AlertRules**

```yaml
- alert: HighRateLimitExceeded
  expr: rate({service}_rate_limit_exceeded_total[5m]) > 10
  for: 5m
  labels:
    severity: warning
    priority: P1
  annotations:
    summary: "High rate limit exceeded rate for {{ $labels.service }}"
    description: "Client {{ $labels.client }} is exceeding rate limits frequently"

- alert: HolmesGPTCostRisk
  expr: rate(holmesgpt_llm_cost_usd_total[1h]) > 50
  for: 10m
  labels:
    severity: critical
    priority: P0
  annotations:
    summary: "HolmesGPT LLM cost exceeding $50/hour"
    description: "Potential cost explosion detected"
```

---

## ✅ **Implementation Checklist**

### **For Each Service**:

1. ✅ **Per-Client Rate Limiter**: Based on ServiceAccount identity
2. ✅ **Global Rate Limiter**: Per-replica backup limit
3. ✅ **Rate Limit Headers**: `X-RateLimit-*` headers in responses
4. ✅ **Metrics**: Prometheus metrics for monitoring
5. ✅ **429 Responses**: Proper `Retry-After` header
6. ✅ **Logging**: Structured logs for rate limit events
7. ✅ **Documentation**: Rate limits documented in API spec

---

## 🎯 **Testing Rate Limits**

### **Load Testing Script**

```bash
#!/bin/bash
# test-rate-limit.sh

SERVICE_URL="http://context-api:8080/api/v1/context"
TOKEN="<serviceaccount-token>"

# Send 100 requests as fast as possible
for i in {1..100}; do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -H "Authorization: Bearer $TOKEN" \
    "$SERVICE_URL?namespace=production&targetType=deployment&targetName=api"
done | sort | uniq -c

# Expected output:
# 50 200  (allowed)
# 50 429  (rate limited)
```

---

## 📚 **Related Documentation**

- [KUBERNETES_TOKENREVIEWER_AUTH.md](./KUBERNETES_TOKENREVIEWER_AUTH.md) - Client identity extraction
- [PROMETHEUS_ALERTRULES.md](./PROMETHEUS_ALERTRULES.md) - Rate limit alerting
- [SERVICEACCOUNT_NAMING_STANDARD.md](./SERVICEACCOUNT_NAMING_STANDARD.md) - Client identifiers

---

**Document Status**: ✅ Complete
**Compliance**: 6/6 services covered
**Last Updated**: October 6, 2025
**Version**: 1.0
