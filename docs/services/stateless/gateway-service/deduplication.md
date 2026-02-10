# Gateway Service - Deduplication & Storm Detection

**Version**: v1.2
**Last Updated**: February 9, 2026
**Status**: ✅ Design Complete

**Changelog**:
- **v1.2** (2026-02-09): Prometheus adapter: alertname excluded from fingerprint; OwnerResolver added for Pod→Deployment resolution (Issue #63). Fingerprint now uses `SHA256(namespace:ownerKind:ownerName)` with OwnerResolver (same pattern as K8s event adapter). This ensures multiple alertnames (KubePodCrashLooping, KubePodNotReady, KubeContainerOOMKilled) for the same resource produce a single fingerprint. Rationale: LLM investigates resource state, not signal type.
- **v1.1** (2026-02-09): Updated fingerprint generation to document adapter-specific strategies. Kubernetes events now use owner-chain-based fingerprinting (`SHA256(namespace:ownerKind:ownerName)`) to deduplicate events across pod restarts and different event reasons.
- **v1.0** (2025-10-04): Initial design.

---

## Table of Contents

1. [Deduplication Overview](#deduplication-overview)
2. [Redis Schema Design](#redis-schema-design)
3. [Fingerprint Generation](#fingerprint-generation)
4. [Deduplication Flow](#deduplication-flow)
5. [Storm Detection](#storm-detection)
6. [Rate Limiting](#rate-limiting)

---

## Deduplication Overview

### Purpose

**Problem**: Prometheus fires alerts every evaluation interval (typically 30-60 seconds) while condition persists. Without deduplication:
- ❌ 100 identical alerts in 1 hour → 100 RemediationRequest CRDs
- ❌ Overwhelms downstream AI Analysis, Workflow Execution
- ❌ Wastes resources analyzing same issue repeatedly

**Solution**: Redis-based fingerprinting tracks active alerts within 5-minute window:
- ✅ First occurrence → Create RemediationRequest CRD
- ✅ Subsequent duplicates → Update count, return 202 (no new CRD)
- ✅ 40-60% reduction in downstream load (typical production)

### Why Redis (Not In-Memory)

**Decision**: Redis persistent storage vs. in-memory cache

| Aspect | Redis | In-Memory | Winner |
|--------|-------|-----------|--------|
| **Survives Gateway Restart** | ✅ Yes | ❌ Lost | Redis |
| **HA Multi-Instance** | ✅ Shared state | ❌ Per-instance | Redis |
| **Performance** | ~1ms lookup | ~0.1ms | In-Memory |
| **TTL Expiration** | ✅ Automatic | Manual cleanup | Redis |
| **Deployment Complexity** | ⚠️ Redis cluster | ✅ Simple | In-Memory |

**Verdict**: Redis wins (survivability + HA > 0.9ms latency penalty)

**Configuration**:
- 5-minute TTL (configurable via ConfigMap)
- Redis Cluster with replication for HA
- Connection pool (100 connections, 10ms timeout)

---

## Redis Schema Design

### Deduplication Metadata Schema

**Key Pattern**: `alert:fingerprint:<sha256-hash>`

**Value Structure** (Redis Hash):
```redis
HSET alert:fingerprint:a1b2c3d4e5... fingerprint "a1b2c3d4e5..."
HSET alert:fingerprint:a1b2c3d4e5... alertName "HighMemoryUsage"
HSET alert:fingerprint:a1b2c3d4e5... namespace "prod-payment-service"
HSET alert:fingerprint:a1b2c3d4e5... resource "payment-api-789"
HSET alert:fingerprint:a1b2c3d4e5... firstSeen "2025-10-04T10:00:00Z"
HSET alert:fingerprint:a1b2c3d4e5... lastSeen "2025-10-04T10:04:30Z"
HSET alert:fingerprint:a1b2c3d4e5... count "5"
HSET alert:fingerprint:a1b2c3d4e5... remediationRequestRef "remediation-abc123"
EXPIRE alert:fingerprint:a1b2c3d4e5... 300  # 5 minutes
```

**Why Hash (not String)**:
- ✅ Atomic field updates (`HINCRBY count`)
- ✅ Structured data (multiple fields)
- ✅ Efficient partial retrieval (`HGET count`)

### Storm Detection Schema

**Rate-Based Storm Detection**:
```redis
# Key: alert:storm:rate:<alertName>
INCR alert:storm:rate:HighMemoryUsage  # Returns current count
EXPIRE alert:storm:rate:HighMemoryUsage 60  # 1-minute window
```

**Pattern-Based Storm Detection**:
```redis
# Key: alert:pattern:<alertName>
# Value: Sorted set (score = timestamp, member = resource identifier)
ZADD alert:pattern:OOMKilled 1728038400 "prod-ns-1:Pod:web-app-789"
ZADD alert:pattern:OOMKilled 1728038405 "prod-ns-2:Pod:api-456"
ZREMRANGEBYSCORE alert:pattern:OOMKilled 0 (NOW-120)  # Remove older than 2min
EXPIRE alert:pattern:OOMKilled 120  # 2-minute window
```

### Rate Limiting Schema

**Per-Source Rate Limiting** (Token Bucket):
```redis
# Key: ratelimit:source:<sourceIP>
# Value: Token count (decrements on each request)
GET ratelimit:source:192.168.1.100  # Returns remaining tokens
DECR ratelimit:source:192.168.1.100  # Consume token
EXPIRE ratelimit:source:192.168.1.100 60  # Refill after 1 minute
```

---

## Fingerprint Generation

### Algorithm

Fingerprint generation uses **adapter-specific strategies** to ensure correct deduplication
for each signal source:

#### Prometheus Alerts (Owner-Based Fingerprint)

Prometheus alerts use **owner chain resolution** to fingerprint at the controller level
(e.g., Deployment, StatefulSet, DaemonSet), similar to Kubernetes events. This is critical because:

- **Different alertnames** (KubePodCrashLooping, KubePodNotReady, KubeContainerOOMKilled) for the same resource should produce the same fingerprint.
- **Different pod names** (after pod recreation by ReplicaSet) from the same Deployment should produce the same fingerprint.
- **Architectural rationale**: The LLM investigates resource state, not signal type. The RCA outcome is independent of which alert triggered it.

The `PrometheusAdapter` resolves the top-level controller owner via the optional
`OwnerResolver` interface, which traverses Kubernetes `ownerReferences`:

```
Pod "payment-api-789abc" → ReplicaSet "payment-api-xyz" → Deployment "payment-api"
```

```go
// Format: SHA256(namespace:ownerKind:ownerName) with OwnerResolver
// Format: SHA256(namespace:kind:name) without OwnerResolver
// alertname is NEVER included in the fingerprint.
func CalculateOwnerFingerprint(resource ResourceIdentifier) string {
    key := fmt.Sprintf("%s:%s:%s",
        resource.Namespace,
        resource.Kind,  // e.g., "Deployment" (resolved owner) or "Pod" (fallback)
        resource.Name,  // e.g., "payment-api" (resolved owner) or "payment-api-789abc" (fallback)
    )
    hash := sha256.Sum256([]byte(key))
    return fmt.Sprintf("%x", hash)
}
```

**Examples** (all produce the same fingerprint):
```
Alert: KubePodCrashLooping on Pod "payment-api-789abc" → Owner: Deployment "payment-api"
Alert: KubePodNotReady     on Pod "payment-api-789abc" → Owner: Deployment "payment-api"
Alert: KubeContainerOOMKilled on Pod "payment-api-def456" → Owner: Deployment "payment-api"
All → SHA256("prod:Deployment:payment-api") → same fingerprint → deduplicated
```

**Fallback**: If OwnerResolver is not configured or resolution fails (RBAC error, timeout), the adapter falls back to fingerprinting with the resource directly (without alertname): `SHA256(namespace:kind:name)`.

#### Kubernetes Events (Owner-Based Fingerprint)

Kubernetes events use **owner chain resolution** to fingerprint at the controller level
(e.g., Deployment, StatefulSet, DaemonSet). This is critical because:

- **Different reasons** (BackOff, OOMKilling, Failed) from the same root cause should
  produce the same fingerprint.
- **Different pod names** (after pod recreation by ReplicaSet) from the same Deployment
  should produce the same fingerprint.

The `KubernetesEventAdapter` resolves the top-level controller owner via the
`OwnerResolver` interface, which traverses Kubernetes `ownerReferences`:

```
Pod "payment-api-789abc" → ReplicaSet "payment-api-xyz" → Deployment "payment-api"
```

```go
// Format: SHA256(namespace:ownerKind:ownerName)
// Reason is EXCLUDED from the fingerprint.
func CalculateOwnerFingerprint(resource ResourceIdentifier) string {
    key := fmt.Sprintf("%s:%s:%s",
        resource.Namespace,
        resource.Kind,  // e.g., "Deployment"
        resource.Name,  // e.g., "payment-api"
    )
    hash := sha256.Sum256([]byte(key))
    return fmt.Sprintf("%x", hash)
}
```

**Examples** (all produce the same fingerprint):
```
Event: BackOff     on Pod "payment-api-789abc" → Owner: Deployment "payment-api"
Event: OOMKilling  on Pod "payment-api-789abc" → Owner: Deployment "payment-api"
Event: BackOff     on Pod "payment-api-def456" → Owner: Deployment "payment-api"
All → SHA256("prod:Deployment:payment-api") → same fingerprint → deduplicated
```

**Fallback**: If owner resolution fails (RBAC error, timeout), the adapter falls back
to fingerprinting with the involvedObject directly (without reason):
`SHA256(namespace:involvedObjectKind:involvedObjectName)`.

### Why SHA256 (Not MD5)

| Algorithm | Speed | Collision Risk | Winner |
|-----------|-------|----------------|--------|
| **MD5** | Faster (~2x) | Higher (broken) | ❌ |
| **SHA256** | ~1-2μs | Negligible | ✅ |
| **SHA512** | Slower | Lower (overkill) | ❌ |

**Verdict**: SHA256 (negligible performance impact, industry standard)

### Collision Handling

**Probability**: SHA256 collision = 2^-256 ≈ 1 in 10^77
**Real-world**: More likely to have hardware failure than SHA256 collision

**If collision occurs** (astronomically unlikely):
- Different alerts get same fingerprint → deduplicated incorrectly
- **Mitigation**: Include timestamp in key if collision detected (v2 enhancement)

---

## Deduplication Flow

### Implementation

**Location**: `pkg/gateway/processing/deduplication.go`

```go
package processing

import (
    "context"
    "fmt"
    "time"

    "github.com/go-redis/redis/v8"
    "github.com/jordigilh/kubernaut/pkg/gateway"
)

// DeduplicationService handles alert deduplication using Redis
type DeduplicationService struct {
    redisClient *redis.Client
    ttl         time.Duration // 5 minutes default
}

func NewDeduplicationService(redisClient *redis.Client) *DeduplicationService {
    return &DeduplicationService{
        redisClient: redisClient,
        ttl:         5 * time.Minute,
    }
}

// Check verifies if alert is duplicate
func (s *DeduplicationService) Check(ctx context.Context, alert *gateway.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
    key := fmt.Sprintf("alert:fingerprint:%s", alert.Fingerprint)

    // Check if key exists
    exists, err := s.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, nil, fmt.Errorf("redis check failed: %w", err)
    }

    if exists == 0 {
        // First occurrence
        deduplicationCacheMissesTotal.Inc()
        return false, nil, nil
    }

    // Duplicate: update metadata
    deduplicationCacheHitsTotal.Inc()

    // Increment count
    count, err := s.redisClient.HIncrBy(ctx, key, "count", 1).Result()
    if err != nil {
        return false, nil, fmt.Errorf("failed to increment count: %w", err)
    }

    // Update lastSeen timestamp
    if err := s.redisClient.HSet(ctx, key, "lastSeen", time.Now().Format(time.RFC3339)).Err(); err != nil {
        return false, nil, fmt.Errorf("failed to update lastSeen: %w", err)
    }

    // Retrieve metadata
    metadata := &DeduplicationMetadata{
        Fingerprint:             alert.Fingerprint,
        Count:                   int(count),
        RemediationRequestRef:   s.redisClient.HGet(ctx, key, "remediationRequestRef").Val(),
        FirstSeen:               s.redisClient.HGet(ctx, key, "firstSeen").Val(),
        LastSeen:                time.Now().Format(time.RFC3339),
    }

    // Update deduplication rate metric
    s.updateDeduplicationRate(ctx)

    return true, metadata, nil
}

// Store saves deduplication metadata for new alert
func (s *DeduplicationService) Store(ctx context.Context, alert *gateway.NormalizedSignal, remediationRequestRef string) error {
    key := fmt.Sprintf("alert:fingerprint:%s", alert.Fingerprint)
    now := time.Now().Format(time.RFC3339)

    // Store as Redis hash
    pipe := s.redisClient.Pipeline()
    pipe.HSet(ctx, key, "fingerprint", alert.Fingerprint)
    pipe.HSet(ctx, key, "alertName", alert.AlertName)
    pipe.HSet(ctx, key, "namespace", alert.Namespace)
    pipe.HSet(ctx, key, "resource", alert.Resource.Name)
    pipe.HSet(ctx, key, "firstSeen", now)
    pipe.HSet(ctx, key, "lastSeen", now)
    pipe.HSet(ctx, key, "count", 1)
    pipe.HSet(ctx, key, "remediationRequestRef", remediationRequestRef)
    pipe.Expire(ctx, key, s.ttl)

    if _, err := pipe.Exec(ctx); err != nil {
        return fmt.Errorf("failed to store deduplication metadata: %w", err)
    }

    return nil
}

// HealthCheck verifies Redis connection
func (s *DeduplicationService) HealthCheck(ctx context.Context) error {
    return s.redisClient.Ping(ctx).Err()
}

// updateDeduplicationRate calculates and updates deduplication rate metric
func (s *DeduplicationService) updateDeduplicationRate(ctx context.Context) {
    hits := float64(deduplicationCacheHitsTotal.Get())
    misses := float64(deduplicationCacheMissesTotal.Get())
    total := hits + misses

    if total > 0 {
        rate := hits / total * 100
        deduplicationRate.Set(rate)
    }
}

// DeduplicationMetadata contains metadata for duplicate alerts
type DeduplicationMetadata struct {
    Fingerprint             string
    Count                   int
    RemediationRequestRef   string
    FirstSeen               string
    LastSeen                string
}
```

### Performance Characteristics

| Operation | Latency (p95) | Latency (p99) |
|-----------|---------------|---------------|
| **Check (exists)** | ~1ms | ~3ms |
| **Check + Update** | ~2ms | ~5ms |
| **Store** | ~3ms | ~8ms |

**Total Impact**: 3-5ms added to 50ms target (6-10% overhead)

---

## Storm Detection

### Rate-Based Detection

**Threshold**: >10 alerts/minute for same alertname

**Implementation**: `pkg/gateway/processing/storm_detection.go`

```go
package processing

import (
    "context"
    "fmt"
    "time"

    "github.com/go-redis/redis/v8"
    "github.com/jordigilh/kubernaut/pkg/gateway"
)

// StormDetector identifies alert storms
type StormDetector struct {
    redisClient *redis.Client
    rateThreshold int // 10 alerts/min
    patternThreshold int // 5 similar alerts
}

func NewStormDetector(redisClient *redis.Client) *StormDetector {
    return &StormDetector{
        redisClient:      redisClient,
        rateThreshold:    10,
        patternThreshold: 5,
    }
}

// Check detects if alert is part of a storm
func (d *StormDetector) Check(ctx context.Context, alert *gateway.NormalizedSignal) (bool, *StormMetadata, error) {
    // Check rate-based storm
    isRateStorm, err := d.checkRateStorm(ctx, alert)
    if err != nil {
        return false, nil, err
    }

    if isRateStorm {
        metadata := &StormMetadata{
            StormType: "rate",
            AlertCount: d.getRateCount(ctx, alert),
        }
        alertStormsDetectedTotal.WithLabelValues("rate", alert.AlertName).Inc()
        return true, metadata, nil
    }

    // Check pattern-based storm
    isPatternStorm, affectedResources, err := d.checkPatternStorm(ctx, alert)
    if err != nil {
        return false, nil, err
    }

    if isPatternStorm {
        metadata := &StormMetadata{
            StormType:         "pattern",
            AlertCount:        len(affectedResources),
            AffectedResources: affectedResources,
        }
        alertStormsDetectedTotal.WithLabelValues("pattern", alert.AlertName).Inc()
        return true, metadata, nil
    }

    return false, nil, nil
}

// checkRateStorm detects if alert firing rate exceeds threshold
func (d *StormDetector) checkRateStorm(ctx context.Context, alert *gateway.NormalizedSignal) (bool, error) {
    key := fmt.Sprintf("alert:storm:rate:%s", alert.AlertName)

    // Increment counter
    count, err := d.redisClient.Incr(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to increment storm counter: %w", err)
    }

    // Set 1-minute TTL on first increment
    if count == 1 {
        d.redisClient.Expire(ctx, key, 60*time.Second)
    }

    // Check if threshold exceeded
    return count > int64(d.rateThreshold), nil
}

// getRateCount retrieves current rate counter value
func (d *StormDetector) getRateCount(ctx context.Context, alert *gateway.NormalizedSignal) int {
    key := fmt.Sprintf("alert:storm:rate:%s", alert.AlertName)
    count, _ := d.redisClient.Get(ctx, key).Int()
    return count
}

// checkPatternStorm detects similar alerts across different resources
func (d *StormDetector) checkPatternStorm(ctx context.Context, alert *gateway.NormalizedSignal) (bool, []gateway.ResourceIdentifier, error) {
    key := fmt.Sprintf("alert:pattern:%s", alert.AlertName)
    now := float64(time.Now().Unix())

    // Add current resource to sorted set
    member := fmt.Sprintf("%s:%s:%s", alert.Namespace, alert.Resource.Kind, alert.Resource.Name)
    if err := d.redisClient.ZAdd(ctx, key, &redis.Z{
        Score:  now,
        Member: member,
    }).Err(); err != nil {
        return false, nil, fmt.Errorf("failed to add to pattern set: %w", err)
    }

    // Remove entries older than 2 minutes
    twoMinutesAgo := now - 120
    d.redisClient.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%f", twoMinutesAgo))

    // Set 2-minute TTL
    d.redisClient.Expire(ctx, key, 2*time.Minute)

    // Count similar alerts in window
    count, err := d.redisClient.ZCard(ctx, key).Result()
    if err != nil {
        return false, nil, fmt.Errorf("failed to count pattern alerts: %w", err)
    }

    if count < int64(d.patternThreshold) {
        return false, nil, nil
    }

    // Retrieve affected resources
    members, err := d.redisClient.ZRange(ctx, key, 0, -1).Result()
    if err != nil {
        return false, nil, fmt.Errorf("failed to retrieve pattern members: %w", err)
    }

    affectedResources := make([]gateway.ResourceIdentifier, len(members))
    for i, member := range members {
        // Parse "namespace:kind:name"
        parts := strings.Split(member, ":")
        if len(parts) == 3 {
            affectedResources[i] = gateway.ResourceIdentifier{
                Namespace: parts[0],
                Kind:      parts[1],
                Name:      parts[2],
            }
        }
    }

    return true, affectedResources, nil
}

// StormMetadata contains metadata for detected storms
type StormMetadata struct {
    StormType         string                        // "rate" or "pattern"
    AlertCount        int
    AffectedResources []gateway.ResourceIdentifier  // for pattern storms
}
```

### Storm Aggregation Strategy

When storm detected, Gateway creates **single RemediationRequest CRD** with storm metadata:

```yaml
apiVersion: remediation.kubernaut.ai/v1
kind: RemediationRequest
metadata:
  name: remediation-storm-xyz
spec:
  isStorm: true
  stormType: "rate"  # or "pattern"
  stormWindow: "5m"
  alertCount: 47
  affectedResources:
    - namespace: prod-ns-1
      kind: Pod
      name: web-app-789
    - namespace: prod-ns-2
      kind: Pod
      name: api-456
    # ... (max 100 resources, then sample)
  totalAffectedResources: 47  # if >100, this shows full count
  # ... rest of normal RemediationRequest fields
```

**Downstream Impact**:
- AI Analysis: Analyzes **cluster-wide pattern** (not individual pods)
- Workflow Execution: Creates **bulk remediation workflow** (e.g., increase memory across deployment)
- Notification: Sends **single aggregated alert** (not 47 separate notifications)

---

## Rate Limiting

### Per-Source Rate Limiting

**Decision**: Rate limit by source IP (not global)

**Implementation**: `pkg/gateway/processing/rate_limiter.go`

```go
package processing

import (
    "context"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// RateLimiter implements per-source token bucket rate limiting
type RateLimiter struct {
    limiters map[string]*rate.Limiter  // sourceIP -> limiter
    mu       sync.RWMutex
    rate     rate.Limit  // tokens per second
    burst    int         // burst size
}

func NewRateLimiter(ratePerMinute int) *RateLimiter {
    return &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        rate:     rate.Limit(float64(ratePerMinute) / 60.0), // convert to per-second
        burst:    ratePerMinute,
    }
}

// Allow checks if request from sourceIP is allowed
func (rl *RateLimiter) Allow(sourceIP string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    limiter, exists := rl.limiters[sourceIP]
    if !exists {
        limiter = rate.NewLimiter(rl.rate, rl.burst)
        rl.limiters[sourceIP] = limiter
    }

    return limiter.Allow()
}

// Middleware wraps HTTP handler with rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        sourceIP := r.RemoteAddr

        if !rl.Allow(sourceIP) {
            rateLimitExceededTotal.WithLabelValues(sourceIP).Inc()
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

### Configuration

**Default**: 100 alerts/min per source IP

**ConfigMap Override**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-rate-limits
  namespace: kubernaut-system
data:
  defaultRateLimit: "100"  # 100 alerts/minute per source
  burstSize: "100"         # Allow burst of 100 alerts

  # Optional: Per-source overrides
  overrides: |
    192.168.1.100: 200  # Alertmanager instance 1 (higher limit)
    192.168.1.101: 200  # Alertmanager instance 2
```

### Rate Limit Response

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 60

{
  "error": "Rate limit exceeded",
  "code": "RATE_LIMITED",
  "details": "100 requests per minute per source. Retry after 60 seconds."
}
```

---

## Summary

Gateway deduplication and storm detection provides:

1. **Redis-Based Deduplication** - 40-60% reduction in downstream load
2. **Hybrid Storm Detection** - Rate-based + pattern-based aggregation
3. **Per-Source Rate Limiting** - Fair multi-tenancy with 100/min default
4. **5-Minute Deduplication Window** - Balances memory usage vs. duplicate prevention
5. **HA-Ready** - Redis Cluster with replication for multi-instance deployments

**Performance**: 3-5ms overhead for deduplication (6-10% of 50ms target)

**Confidence**: 95% (Redis and rate limiting are proven patterns)

**Next**: [CRD Integration](./crd-integration.md)

