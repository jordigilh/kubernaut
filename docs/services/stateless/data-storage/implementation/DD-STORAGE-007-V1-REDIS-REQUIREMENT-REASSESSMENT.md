# DD-STORAGE-007: V1.0 Redis Requirement Reassessment

**Date**: November 13, 2025
**Status**: ✅ **RECOMMENDED - REDIS OPTIONAL**
**Decision Maker**: Kubernaut Data Storage Team
**Affects**: Infrastructure requirements, deployment complexity

---

## Context

**Trigger**: Removed playbook embedding caching from V1.0 (deferred to V1.1 per DD-STORAGE-006)

**Question**: Is Redis still required for Data Storage Service V1.0?

**Previous Redis Use Cases**:
1. ✅ **Playbook embedding cache** - REMOVED (deferred to V1.1)
2. ❓ **Dead Letter Queue (DLQ)** - Audit write error recovery (DD-009)

---

## Current Redis Usage Analysis

### Use Case 1: Playbook Embedding Cache
**Status**: ⏸️ **REMOVED FROM V1.0**

**Previous Purpose**:
- Cache playbook embeddings to avoid regeneration
- 24-hour TTL with Redis

**Current Status**:
- Deferred to V1.1 per DD-STORAGE-006
- V1.0 uses no-cache approach (92% confidence)

**Impact**: Redis no longer needed for caching in V1.0

---

### Use Case 2: Dead Letter Queue (DLQ)
**Status**: ✅ **ACTIVE** (from DATA-STORAGE-WRITE-API-DECISIONS.md)

**Purpose**: Audit write error recovery (DD-009)

**Architecture** (from DATA-STORAGE-WRITE-API-DECISIONS.md lines 40-52):
```
Service → (Write Fails) → Dead Letter Queue (Redis Streams)
                                  ↓
                          Async Retry Worker
                                  ↓
                          Data Storage Service
```

**Implementation Details**:
- **Technology**: Redis Streams
- **Max Retention**: 7 days
- **Retry Strategy**: Exponential backoff (1m, 5m, 15m, 1h, 4h, 24h)
- **Monitoring**: Alert if DLQ depth > 100 messages
- **Component**: `pkg/datastorage/dlq/` - DLQ client library

**Business Value**:
- Aligns with ADR-032 "No Audit Loss" mandate
- Ensures service availability (reconciliation loops don't block on audit writes)
- Provides audit trail even during Data Storage Service outages
- Enables eventual consistency for audit data

---

## Decision: Redis Requirement for V1.0

### **Recommendation: REDIS OPTIONAL**

**Confidence**: 88%

**Rationale**: DLQ is valuable but not critical for V1.0 MVP

---

## Option 1: Redis Optional (Graceful Degradation) ⭐ **RECOMMENDED**

**Confidence**: 88%

### Architecture

**With Redis** (Production):
```
Service → Data Storage API (fails)
    ↓
Redis DLQ → Async Retry Worker → Data Storage API
```

**Without Redis** (Development/Testing):
```
Service → Data Storage API (fails)
    ↓
Log error + Alert → Manual investigation
```

### Implementation

```go
// pkg/datastorage/client/client.go

type Client struct {
    httpClient *http.Client
    baseURL    string
    dlqClient  DLQClient  // Optional - can be nil
    logger     *zap.Logger
}

func (c *Client) WriteAudit(ctx context.Context, audit *Audit) error {
    // Attempt primary write
    err := c.httpClient.Post(c.baseURL+"/api/v1/audit", audit)
    if err == nil {
        return nil  // Success
    }

    // Primary write failed
    if c.dlqClient != nil {
        // DLQ available - write to DLQ for async retry
        c.logger.Warn("primary write failed, writing to DLQ",
            zap.Error(err))
        return c.dlqClient.Write(ctx, audit)
    }

    // No DLQ - log error and return failure
    c.logger.Error("primary write failed, no DLQ available",
        zap.Error(err))
    return fmt.Errorf("audit write failed (no DLQ): %w", err)
}
```

### Configuration

```yaml
# config.yaml
datastorage:
  url: "http://data-storage:8080"
  dlq:
    enabled: true  # Set to false to disable DLQ
    redis:
      host: "redis"
      port: 6379
      stream: "audit-dlq"
```

### Pros

1. ✅ **Flexible Deployment** (90% confidence)
   - Production: Enable DLQ for resilience
   - Development: Disable DLQ for simplicity
   - Testing: Disable DLQ to avoid Redis dependency

2. ✅ **Simpler V1.0 MVP** (85% confidence)
   - No mandatory Redis deployment
   - Faster local development setup
   - Easier integration testing

3. ✅ **Graceful Degradation** (90% confidence)
   - Service works without Redis
   - DLQ is enhancement, not requirement
   - Clear error logging when DLQ unavailable

4. ✅ **Lower Operational Complexity** (80% confidence)
   - One less service to deploy/monitor
   - Fewer failure modes
   - Simpler troubleshooting

### Cons

1. ⚠️ **Audit Loss Risk (No DLQ)** (70% concern)
   - Without DLQ, failed writes are lost
   - **Mitigation**: Alerts fire immediately on write failures
   - **Mitigation**: Reconciliation loops retry on next cycle

2. ⚠️ **Configuration Complexity** (60% concern)
   - Need to handle both DLQ-enabled and DLQ-disabled modes
   - **Mitigation**: Clear configuration with sensible defaults

3. ⚠️ **Production Recommendation Ambiguity** (50% concern)
   - Is DLQ recommended or required for production?
   - **Mitigation**: Document as "strongly recommended for production"

### Deployment Scenarios

| Environment | DLQ Enabled? | Redis Required? | Rationale |
|-------------|--------------|-----------------|-----------|
| **Development** | ❌ No | ❌ No | Simplicity, fast iteration |
| **Testing** | ❌ No | ❌ No | Avoid Redis dependency in tests |
| **Staging** | ✅ Yes | ✅ Yes | Production-like environment |
| **Production** | ✅ Yes | ✅ Yes | Resilience, no audit loss |

---

## Option 2: Redis Required (Always On)

**Confidence**: 60% (NOT RECOMMENDED)

### Architecture

**Always**:
```
Service → Data Storage API (fails) → Redis DLQ → Async Retry Worker
```

### Pros

1. ✅ **No Audit Loss** (95% confidence)
   - All failed writes go to DLQ
   - Guaranteed eventual consistency

2. ✅ **Simpler Code** (80% confidence)
   - No conditional DLQ logic
   - Single code path

### Cons

1. ❌ **Mandatory Redis Deployment** (90% concern) ⭐ **CRITICAL**
   - Development: Must run Redis locally
   - Testing: Must start Redis in CI/CD
   - Deployment: Redis must be available before Data Storage
   - **Impact**: Slower development, more complex setup

2. ❌ **Increased Operational Complexity** (80% concern)
   - One more service to deploy/monitor
   - Redis failures block Data Storage deployment
   - More failure modes to handle

3. ❌ **Overkill for V1.0 MVP** (75% concern)
   - DLQ is valuable but not critical for MVP
   - Can add in V1.1 if needed
   - **Mitigation**: None

### Deployment Scenarios

| Environment | DLQ Enabled? | Redis Required? | Rationale |
|-------------|--------------|-----------------|-----------|
| **Development** | ✅ Yes | ✅ Yes | Must run Redis locally |
| **Testing** | ✅ Yes | ✅ Yes | CI/CD must start Redis |
| **Staging** | ✅ Yes | ✅ Yes | Production-like |
| **Production** | ✅ Yes | ✅ Yes | Resilience |

---

## Option 3: Remove DLQ Entirely (V1.1 Feature)

**Confidence**: 75%

### Architecture

**V1.0**:
```
Service → Data Storage API (fails) → Log error + Alert
```

**V1.1** (Future):
```
Service → Data Storage API (fails) → Redis DLQ → Async Retry Worker
```

### Pros

1. ✅ **Simplest V1.0** (90% confidence)
   - No Redis dependency
   - No DLQ code
   - Faster development

2. ✅ **Clear V1.0 Scope** (85% confidence)
   - DLQ is V1.1 feature
   - V1.0 focuses on core functionality

3. ✅ **No Operational Complexity** (90% confidence)
   - No Redis to deploy/monitor
   - Fewer failure modes

### Cons

1. ⚠️ **Audit Loss Risk** (80% concern)
   - Failed writes are lost (no retry mechanism)
   - **Mitigation**: Alerts fire immediately
   - **Mitigation**: Reconciliation loops retry

2. ⚠️ **Production Readiness** (70% concern)
   - Is V1.0 production-ready without DLQ?
   - **Answer**: Yes, if Data Storage Service is highly available

3. ⚠️ **Code Removal** (60% concern)
   - DLQ code already exists (`pkg/datastorage/dlq/`)
   - **Mitigation**: Keep code, just make it optional

---

## Comparison Matrix

| Aspect | Option 1: Optional | Option 2: Required | Option 3: Remove |
|--------|-------------------|-------------------|------------------|
| **Redis Required (Dev)** | ❌ No | ✅ Yes | ❌ No |
| **Redis Required (Prod)** | ⚠️ Recommended | ✅ Yes | ❌ No |
| **Audit Loss Risk** | ⚠️ Low (with DLQ) | ✅ None | ❌ High |
| **Development Complexity** | ⚠️ Medium | ❌ High | ✅ Low |
| **Operational Complexity** | ⚠️ Medium | ❌ High | ✅ Low |
| **Production Readiness** | ✅ Yes (with DLQ) | ✅ Yes | ⚠️ Depends |
| **V1.0 MVP Fit** | ✅ Good | ⚠️ Overkill | ✅ Good |
| **Code Complexity** | ⚠️ Medium | ✅ Low | ✅ Low |
| **Confidence** | **88%** ⭐ | 60% | 75% |

---

## Recommended Decision

### **Option 1: Redis Optional (Graceful Degradation)**

**Confidence**: 88%

**Rationale**:
1. ✅ **Flexible**: Works with or without Redis
2. ✅ **Simple V1.0**: No mandatory Redis for development/testing
3. ✅ **Production-Ready**: DLQ available when needed
4. ✅ **Graceful Degradation**: Clear error handling when DLQ unavailable

**Implementation**:
1. Make `DLQClient` optional in Data Storage client
2. Add configuration flag: `dlq.enabled` (default: `false` for dev, `true` for prod)
3. Log clear warnings when DLQ unavailable
4. Document DLQ as "strongly recommended for production"

---

## Implementation Plan

### V1.0 Changes

**1. Make DLQClient Optional**

```go
// pkg/datastorage/client/client.go

type Config struct {
    BaseURL    string
    DLQEnabled bool
    DLQConfig  *DLQConfig  // Optional
}

type DLQConfig struct {
    RedisHost   string
    RedisPort   int
    StreamName  string
    MaxRetries  int
}

func NewClient(cfg *Config) (*Client, error) {
    client := &Client{
        httpClient: &http.Client{Timeout: 10 * time.Second},
        baseURL:    cfg.BaseURL,
        logger:     zap.L(),
    }

    // Initialize DLQ if enabled
    if cfg.DLQEnabled {
        if cfg.DLQConfig == nil {
            return nil, fmt.Errorf("DLQ enabled but no DLQ config provided")
        }
        dlqClient, err := dlq.NewClient(cfg.DLQConfig)
        if err != nil {
            return nil, fmt.Errorf("failed to create DLQ client: %w", err)
        }
        client.dlqClient = dlqClient
        client.logger.Info("DLQ enabled", zap.String("redis_host", cfg.DLQConfig.RedisHost))
    } else {
        client.logger.Warn("DLQ disabled - audit writes will fail without retry")
    }

    return client, nil
}
```

**2. Update Configuration**

```yaml
# config/development.yaml
datastorage:
  url: "http://localhost:8080"
  dlq:
    enabled: false  # No Redis in development

# config/production.yaml
datastorage:
  url: "http://data-storage:8080"
  dlq:
    enabled: true  # Redis required in production
    redis:
      host: "redis"
      port: 6379
      stream: "audit-dlq"
      max_retries: 6
```

**3. Update Documentation**

```markdown
# Data Storage Service - Redis Requirement

## V1.0 Redis Usage

**Status**: OPTIONAL (strongly recommended for production)

**Use Case**: Dead Letter Queue (DLQ) for audit write error recovery

**Configuration**:
- **Development**: DLQ disabled (no Redis required)
- **Testing**: DLQ disabled (no Redis required)
- **Production**: DLQ enabled (Redis required)

**Without DLQ**:
- Failed audit writes are logged and alerted
- No automatic retry mechanism
- Reconciliation loops will retry on next cycle

**With DLQ**:
- Failed audit writes go to Redis Streams
- Async retry worker retries with exponential backoff
- Guaranteed eventual consistency
- No audit loss
```

---

## Risks and Mitigations

### Risk 1: Audit Loss Without DLQ

**Likelihood**: 30% (depends on Data Storage availability)
**Impact**: Medium

**Mitigation**:
- **Immediate Alerts**: Fire alert on any audit write failure
- **Reconciliation Retry**: Controllers retry on next reconciliation cycle
- **High Availability**: Deploy Data Storage with 2-3 replicas
- **Production Recommendation**: Always enable DLQ in production

### Risk 2: Configuration Confusion

**Likelihood**: 40%
**Impact**: Low

**Mitigation**:
- **Clear Defaults**: DLQ disabled by default (safe for development)
- **Documentation**: Clear guidance on when to enable DLQ
- **Validation**: Startup validation checks DLQ config if enabled

### Risk 3: DLQ Code Bitrot

**Likelihood**: 20%
**Impact**: Low

**Mitigation**:
- **Integration Tests**: Test both DLQ-enabled and DLQ-disabled modes
- **CI/CD**: Run DLQ tests in CI pipeline
- **Production Usage**: Enable DLQ in staging/production

---

## Success Metrics

### V1.0 (Optional Redis)

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Development Setup Time** | < 5 minutes | Time to run service locally |
| **Test Execution Time** | < 30 seconds | Unit + integration tests |
| **Production Audit Loss** | 0% | With DLQ enabled |
| **DLQ Adoption** | 100% | Staging + production |

### V1.1 (Future)

| Metric | Target | Measurement |
|--------|--------|-------------|
| **DLQ Retry Success Rate** | > 95% | Async retry worker metrics |
| **DLQ Depth** | < 100 messages | Redis Streams monitoring |
| **Audit Recovery Time** | < 5 minutes | Time to clear DLQ after outage |

---

## Confidence Breakdown

| Factor | Confidence | Reasoning |
|--------|-----------|-----------|
| **Flexible Deployment** | 90% | Works with or without Redis |
| **Simpler V1.0** | 85% | No mandatory Redis |
| **Production Readiness** | 85% | DLQ available when needed |
| **Graceful Degradation** | 90% | Clear error handling |
| **Operational Simplicity** | 80% | One less service to manage |
| **Code Complexity** | 75% | Conditional DLQ logic |
| **Overall** | **88%** | Strong recommendation |

---

## Conclusion

**V1.0: Redis OPTIONAL** with **88% confidence**

**Key Reasons**:
1. ✅ **No playbook caching** (deferred to V1.1) - Redis no longer needed for caching
2. ✅ **DLQ is enhancement** - Valuable but not critical for V1.0 MVP
3. ✅ **Flexible deployment** - Works with or without Redis
4. ✅ **Simpler development** - No mandatory Redis for local dev/testing
5. ✅ **Production-ready** - DLQ available when needed (strongly recommended)

**Deployment Recommendation**:
- **Development**: DLQ disabled (no Redis)
- **Testing**: DLQ disabled (no Redis)
- **Staging**: DLQ enabled (Redis required)
- **Production**: DLQ enabled (Redis required)

**Next Steps**:
1. Update Data Storage client to make DLQClient optional
2. Add configuration flag: `dlq.enabled`
3. Update documentation with deployment recommendations
4. Add integration tests for both DLQ-enabled and DLQ-disabled modes

---

**Document Version**: 1.0
**Last Updated**: November 13, 2025
**Status**: ✅ **APPROVED**

