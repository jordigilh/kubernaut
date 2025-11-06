# DD-INFRASTRUCTURE-002: Data Storage Service Redis Strategy

**Status**: ✅ Approved (2025-11-03)  
**Date**: 2025-11-03  
**Decision Makers**: Development Team  
**Supersedes**: None  
**Related To**: DD-INFRASTRUCTURE-001 (Gateway/Context-API Redis Separation), DD-009 (DLQ Pattern)

---

## 📋 **Context**

Data Storage Service requires Redis for Dead Letter Queue (DLQ) functionality (DD-009). When audit writes to PostgreSQL fail, the service falls back to Redis Streams for async retry.

**Current State**:
- Gateway Service: Dedicated `redis-gateway-ha` (3 replicas + Sentinel, HA required)
- Context API Service: Dedicated `redis` (1 replica, graceful degradation)
- Data Storage Service: **NO REDIS INSTANCE SPECIFIED** ❌

**Problem**: What Redis instance should Data Storage Service use for DLQ?

**Business Requirements**:
- BR-AUDIT-001: Complete audit trail with no data loss
- DD-009: DLQ fallback pattern for audit write failures
- ADR-032: "No Audit Loss" mandate for 7+ year compliance

---

## 🎯 **Decision**

**APPROVED**: Data Storage Service uses **shared Redis instance** with database isolation.

**Pattern**: Share Context API's Redis instance (`redis.kubernaut-system:6379`) with database isolation.

**Database Allocation**:
- **DB 0**: Context API L1 Cache
- **DB 1**: Data Storage DLQ (audit write fallback)

---

## 🏗️ **Architecture**

### **Production Deployment**

```
redis.kubernaut-system:6379 (Deployment, 1 replica)
├─→ Context API (DB 0): L1 Cache
│   └─→ Graceful degradation to L2/L3 on failure
│
└─→ Data Storage (DB 1): DLQ for audit writes
    └─→ Async retry with exponential backoff
```

### **Integration Test Pattern**

```
localhost:6379 (Podman container: datastorage-redis-test)
├─→ Repository/DLQ Tests (DB 0): Direct Redis access
└─→ HTTP API Tests (DB 1): Via Data Storage Service container
```

**Note**: Integration tests use `--network host` for Data Storage Service container to access PostgreSQL (5433) and Redis (6379) on localhost.

---

## 🔍 **Alternatives Considered**

### **Alternative A: Shared Redis with DB Isolation** ✅ **APPROVED**

**Approach**: Share Context API's Redis instance with database isolation

**Architecture**:
```
redis (Deployment, 1 replica)
├─→ Context API (DB 0)
└─→ Data Storage DLQ (DB 1)
```

**Pros**:
- ✅ **Resource Efficient**: No additional Redis instance (saves ~256MB memory)
- ✅ **Operational Simplicity**: One Redis instance to manage
- ✅ **Quick Implementation**: 30 minutes to deploy
- ✅ **Acceptable Risk**: DLQ can tolerate failures (audit data queued, retry later)
- ✅ **Database Isolation**: No key collision risk
- ✅ **Follows Pattern**: Similar to Gateway/Context-API sharing (pre-DD-INFRASTRUCTURE-001)

**Cons**:
- ⚠️ **Shared Resource Contention**: DLQ writes could impact Context API cache performance
  - **Mitigation**: DLQ writes are infrequent (only on DB failures), Context API degrades gracefully
- ⚠️ **No Failure Isolation**: Redis failure affects both services
  - **Mitigation**: Both services can tolerate Redis failures (Context API → L2/L3, Data Storage → retry later)
- ⚠️ **Memory Limits**: 512MB shared between Context API cache + DLQ
  - **Mitigation**: DLQ capped at 10K messages (~10MB), Context API LRU eviction

**Resource Usage**:
| Component | CPU Request | Memory Request | CPU Limit | Memory Limit |
|-----------|-------------|----------------|-----------|--------------|
| **redis** (1 pod) | 100m | 256Mi | 500m | 512Mi |

**Confidence**: **90%** - Right choice for V1.0, acceptable risk/benefit trade-off

---

### **Alternative B: Dedicated Data Storage Redis** ❌ **REJECTED**

**Approach**: Deploy dedicated `redis-datastorage` instance

**Architecture**:
```
redis-datastorage (Deployment, 1 replica)
└─→ Data Storage DLQ ONLY
```

**Pros**:
- ✅ **Service Isolation**: Data Storage and Context API don't interfere
- ✅ **Failure Isolation**: Redis failure in one service doesn't affect the other
- ✅ **Independent Scaling**: Can scale Redis based on DLQ-specific needs
- ✅ **Follows DD-INFRASTRUCTURE-001 Pattern**: Separate Redis per service

**Cons**:
- ❌ **Resource Overhead**: Additional 256MB memory + 100m CPU for minimal usage
  - **Impact**: DLQ is rarely used (only on DB failures), dedicated instance is overkill
- ❌ **Operational Overhead**: Another Redis instance to monitor/manage
- ❌ **Over-Engineering**: DLQ can tolerate failures, doesn't need dedicated instance
- ❌ **Cost**: ~$5-10/month additional cloud cost for rarely-used service

**Confidence**: **60%** - Acceptable for V2.0 if DLQ usage increases, overkill for V1.0

---

### **Alternative C: Share Gateway Redis HA** ❌ **REJECTED**

**Approach**: Use `redis-gateway-ha` with database isolation

**Architecture**:
```
redis-gateway-ha (StatefulSet, 3 replicas + Sentinel)
├─→ Gateway (DB 0)
└─→ Data Storage DLQ (DB 2)
```

**Pros**:
- ✅ **High Availability**: Automatic failover for DLQ
- ✅ **Production-Ready**: Sentinel monitoring

**Cons**:
- ❌ **Critical Path Sharing**: Gateway is critical for alert processing, shouldn't share with DLQ
- ❌ **Resource Contention**: DLQ writes could impact Gateway performance
- ❌ **Blast Radius**: DLQ issues could affect Gateway (alert processing is critical)
- ❌ **Operational Complexity**: Gateway HA is tuned for Gateway workload, not DLQ

**Confidence**: **30%** - Not recommended, violates service isolation principle

---

## 📊 **Decision Rationale**

**Why Alternative A (Shared Redis)?**

1. **DLQ Usage is Infrequent**: DLQ only activates on PostgreSQL failures (rare in production)
2. **Both Services Tolerate Failures**: 
   - Context API: Graceful degradation to L2/L3 cache
   - Data Storage: Audit data queued, retry later (eventual consistency acceptable)
3. **Resource Efficiency**: Saves 256MB memory + 100m CPU (significant for V1.0)
4. **Operational Simplicity**: One Redis instance to manage vs. two
5. **Database Isolation**: No key collision risk (DB 0 vs DB 1)
6. **Quick Implementation**: 30 minutes vs. 2-3 hours for dedicated instance

**Key Insight**: DLQ is a **fallback mechanism**, not a primary data path. Sharing Redis with Context API is acceptable because both services can tolerate Redis failures and DLQ usage is infrequent.

---

## 🔧 **Implementation**

### **Production Configuration**

**Data Storage Service ConfigMap**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: kubernaut-system
data:
  config.yaml: |
    database:
      host: postgres.kubernaut-system.svc.cluster.local
      port: 5432
      name: action_history
      user: slm_user
    redis:
      addr: redis.kubernaut-system.svc.cluster.local:6379
      db: 1  # DB 1 for DLQ (Context API uses DB 0)
      dlq:
        stream_prefix: "audit:dlq:"
        max_length: 10000  # Cap at 10K messages (~10MB)
        ttl: 168h  # 7 days
```

**Data Storage Service Deployment**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: datastorage
        image: datastorage:v1.0.0
        env:
        - name: REDIS_ADDR
          value: "redis.kubernaut-system.svc.cluster.local:6379"
        - name: REDIS_DB
          value: "1"  # DB 1 for DLQ
        - name: DB_HOST
          value: "postgres.kubernaut-system.svc.cluster.local"
        - name: DB_PORT
          value: "5432"
```

### **Integration Test Configuration**

**BeforeSuite Setup** (`test/integration/datastorage/suite_test.go`):
```go
// Start Redis container (shared with Context API pattern)
startRedis()  // Starts redis:7-alpine on port 6379

// Start Data Storage Service with --network host
podman run -d \
  --name datastorage-service-test \
  --network host \
  -e DB_HOST=localhost \
  -e DB_PORT=5433 \
  -e REDIS_ADDR=localhost:6379 \
  -e REDIS_DB=1 \
  data-storage:test
```

**Database Isolation**:
- Repository/DLQ unit tests: Use DB 0 (direct Redis access)
- HTTP API integration tests: Use DB 1 (via Data Storage Service)

---

## 📈 **Consequences**

### **Positive**

- ✅ **Resource Efficiency**: Saves 256MB memory + 100m CPU
- ✅ **Operational Simplicity**: One Redis instance to manage
- ✅ **Quick Implementation**: 30 minutes to deploy
- ✅ **Database Isolation**: No key collision risk
- ✅ **Acceptable Risk**: Both services tolerate Redis failures
- ✅ **Cost Savings**: ~$5-10/month cloud cost savings

### **Negative**

- ⚠️ **Shared Resource Contention**: DLQ writes could impact Context API cache
  - **Mitigation**: DLQ writes are infrequent (only on DB failures)
  - **Monitoring**: Track DLQ depth, Context API cache hit rate
- ⚠️ **No Failure Isolation**: Redis failure affects both services
  - **Mitigation**: Both services degrade gracefully
  - **Monitoring**: Redis uptime, failover alerts
- ⚠️ **Memory Limits**: 512MB shared between services
  - **Mitigation**: DLQ capped at 10K messages (~10MB), Context API LRU eviction
  - **Monitoring**: Redis memory usage, eviction rate

### **Neutral**

- 🔄 **V2.0 Migration Path**: Easy to split into dedicated instances if needed
- 🔄 **Monitoring**: Need to track both Context API and DLQ metrics
- 🔄 **Backup**: Context API cache (no backup), DLQ (no backup, retry on failure)

---

## 📊 **Monitoring & Observability**

### **Key Metrics**

**Redis Metrics** (Prometheus):
```yaml
# Memory usage (should stay < 512MB)
redis_memory_used_bytes{service="redis",namespace="kubernaut-system"}

# DLQ depth (should be 0 in normal operation)
redis_stream_length{stream="audit:dlq:notification"}

# Context API cache hit rate (should stay > 80%)
cache_hit_rate{service="context-api",tier="L1"}

# Eviction rate (should be low)
redis_evicted_keys_total{service="redis"}
```

**Alerts**:
```yaml
# DLQ depth > 100 (PostgreSQL issues)
- alert: DataStorageDLQDepthHigh
  expr: redis_stream_length{stream=~"audit:dlq:.*"} > 100
  for: 5m
  severity: warning

# Redis memory > 450MB (approaching limit)
- alert: RedisMemoryHigh
  expr: redis_memory_used_bytes > 450000000
  for: 5m
  severity: warning

# Context API cache hit rate < 70% (degradation)
- alert: ContextAPICacheHitRateLow
  expr: cache_hit_rate{service="context-api",tier="L1"} < 0.7
  for: 10m
  severity: info
```

---

## 🔄 **Review & Evolution**

### **When to Revisit**

1. **DLQ Usage Increases**: If DLQ depth consistently > 1000 messages
2. **Resource Contention**: If Context API cache hit rate drops < 70%
3. **Memory Pressure**: If Redis memory usage consistently > 450MB
4. **Production Metrics**: After 1 month of production data

### **Success Metrics**

| Metric | Target | Actual (TBD) |
|--------|--------|--------------|
| **DLQ Depth** | < 10 messages (p95) | TBD |
| **Redis Memory** | < 400MB (p95) | TBD |
| **Context API Cache Hit Rate** | > 80% | TBD |
| **Redis Uptime** | > 99.5% | TBD |
| **DLQ Write Latency** | < 10ms (p95) | TBD |

### **V2.0 Considerations**

**Triggers for Dedicated Redis**:
- DLQ depth consistently > 1000 messages
- Context API cache hit rate < 70% due to DLQ contention
- Redis memory usage > 450MB consistently
- Production metrics show resource contention

**Migration Path**:
1. Deploy `redis-datastorage` (1 replica, no HA)
2. Update Data Storage Service config (REDIS_ADDR)
3. Migrate DLQ data (optional, can start fresh)
4. Monitor for 1 week
5. Remove DLQ from shared Redis

**Estimated Effort**: 2-3 hours

---

## 🔗 **Related Decisions**

- **Builds On**: 
  - DD-INFRASTRUCTURE-001 (Gateway/Context-API Redis Separation)
  - DD-009 (DLQ Pattern for Audit Write Error Recovery)
- **Supports**: 
  - BR-AUDIT-001 (Complete audit trail)
  - ADR-032 (Data Access Layer Isolation, "No Audit Loss")
- **Related To**:
  - DD-CONTEXT-002 (Multi-tier caching - Context API graceful degradation)

---

## 📚 **References**

- **DD-INFRASTRUCTURE-001**: Gateway/Context-API Redis separation pattern
- **DD-009**: DLQ pattern and Redis Streams implementation
- **ADR-032**: Data Access Layer Isolation ("No Audit Loss" mandate)
- **Integration Test Pattern**: `test/integration/datastorage/suite_test.go`

---

## ✅ **Approval**

**Decision**: Alternative A - Shared Redis with DB Isolation  
**Confidence**: **90%**  
**Status**: ✅ **APPROVED** (2025-11-03)

**Rationale**: Resource-efficient, operationally simple, acceptable risk for V1.0. DLQ usage is infrequent and both services tolerate Redis failures. Easy migration path to dedicated instance in V2.0 if needed.

---

## 📝 **Implementation Checklist**

### **Production Deployment**
- [ ] Update Data Storage ConfigMap (REDIS_ADDR, REDIS_DB=1)
- [ ] Update Data Storage Deployment (env vars)
- [ ] Verify Redis connection (DB 1)
- [ ] Test DLQ fallback (stop PostgreSQL, verify DLQ write)
- [ ] Monitor DLQ depth, Context API cache hit rate
- [ ] Document in deployment guide

### **Integration Tests**
- [x] Update `suite_test.go` (--network host)
- [ ] Verify tests pass (4 HTTP API scenarios)
- [ ] Document Redis sharing pattern
- [ ] Add DLQ depth verification

### **Monitoring**
- [ ] Add Prometheus metrics (DLQ depth, Redis memory)
- [ ] Create Grafana dashboard (Redis + DLQ metrics)
- [ ] Configure alerts (DLQ depth, memory usage)
- [ ] Document runbook (DLQ troubleshooting)

---

**Last Updated**: 2025-11-03  
**Next Review**: 2025-12-03 (1 month after production deployment)

