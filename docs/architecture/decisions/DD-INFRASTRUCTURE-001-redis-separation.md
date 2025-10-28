# DD-INFRASTRUCTURE-001: Separate Redis Instances for Gateway and Context-API

## Status
**✅ Approved & Implemented** (2025-10-24)
**Last Reviewed**: 2025-10-24
**Confidence**: 95%

## Context & Problem

During Gateway integration test implementation, we discovered that **two services share a single Redis instance**:

1. **Gateway Service**: Uses Redis for deduplication, storm detection, and rate limiting (BR-GATEWAY-008, BR-GATEWAY-009)
2. **Context-API Service**: Uses Redis for L1 cache in multi-tier caching strategy (DD-CONTEXT-002)

**Current State** (Pre-Decision):
```
Single Redis Deployment (1 replica, no HA)
├─→ Gateway: Deduplication, Storm Detection, Rate Limiting (DB 0)
└─→ Context-API: L1 Cache (DB 0)
```

**Problems Identified**:
1. **Single Point of Failure**: One Redis crash affects both services
2. **No Automatic Failover**: Manual intervention required for Redis failures
3. **Resource Contention**: Gateway storm detection could impact Context-API cache performance
4. **Blast Radius**: Redis failure affects multiple critical services
5. **Undocumented Sharing**: No design decision documented the sharing strategy
6. **Key Collision Risk**: Both services use DB 0 without namespacing

---

## Alternatives Considered

### Alternative A: Separate Redis HA Instances (Approved)

**Approach**: Deploy dedicated Redis HA clusters for each service

**Architecture**:
```
redis-gateway-ha (StatefulSet, 3 replicas + Sentinel)
├─→ Gateway Service ONLY
├─→ Deduplication, Storm Detection, Rate Limiting
└─→ Automatic failover (5-10s)

redis (Deployment, 1 replica - existing)
├─→ Context-API Service ONLY
└─→ L1 Cache (graceful degradation to L2/L3 on failure)
```

**Service Names**:
- `redis-gateway-ha.kubernaut-system:6379` → Gateway Service
- `redis.kubernaut-system:6379` → Context-API Service

**Pros**:
- ✅ **Service Isolation**: Gateway and Context-API don't interfere
- ✅ **Production-Ready**: Gateway has automatic failover (HA requirement from DD-GATEWAY-003)
- ✅ **Clear Ownership**: Each service owns its Redis instance
- ✅ **Independent Scaling**: Can scale Redis based on service-specific needs
- ✅ **Failure Isolation**: Context-API Redis failure doesn't affect Gateway (and vice versa)
- ✅ **Integration Tests Work**: Can test Gateway HA failure scenarios
- ✅ **Aligns with Documentation**: Matches planned `deploy/redis-ha/` architecture

**Cons**:
- ⚠️ **Resource Overhead**: 4 Redis pods (3 HA + 1 single) vs. 1 pod
  - **Mitigation**: Gateway HA is mandatory for production (BR-GATEWAY-008/009), Context-API can use single instance (graceful degradation)
- ⚠️ **Deployment Complexity**: StatefulSet + Sentinel configuration
  - **Mitigation**: Automated deployment scripts, well-documented in `deploy/redis-ha/README.md`
- ⚠️ **Operational Overhead**: Two Redis clusters to monitor
  - **Mitigation**: Prometheus metrics + Grafana dashboards for both clusters

**Resource Usage**:
| Component | CPU Request | Memory Request | CPU Limit | Memory Limit |
|-----------|-------------|----------------|-----------|--------------|
| **redis-gateway-ha** (3 pods) | 300m | 384Mi | 1500m | 1536Mi |
| **redis-gateway-sentinel** (3 containers) | 150m | 192Mi | 300m | 384Mi |
| **redis** (1 pod, Context-API) | 100m | 256Mi | 500m | 512Mi |
| **TOTAL** | 550m | 832Mi | 2300m | 2432Mi |

**Confidence**: **95%** - This is the right long-term solution

---

### Alternative B: Shared Redis HA with DB Isolation (Rejected)

**Approach**: Deploy single Redis HA StatefulSet, both services share it with database isolation

**Architecture**:
```
redis-ha (StatefulSet, 3 replicas + Sentinel)
├─→ Gateway Service (DB 1)
└─→ Context-API Service (DB 2)
```

**Pros**:
- ✅ **Resource Efficient**: Single HA cluster vs. 2 separate clusters
- ✅ **Production-Ready**: Automatic failover for both services
- ✅ **Simple**: One Redis cluster to manage

**Cons**:
- ❌ **Shared Resource Contention**: Gateway storm detection could impact Context-API cache
- ❌ **Blast Radius**: Redis failure affects both services (even with failover)
- ❌ **Memory Limits**: 512MB shared between services (may need tuning)
- ❌ **No Failure Isolation**: One service's load can impact the other
- ❌ **Operational Complexity**: Harder to debug performance issues (which service is causing load?)

**Confidence**: **70%** - Acceptable for development, not ideal for production

---

### Alternative C: Keep Single Redis, Document Sharing (Rejected)

**Approach**: Keep existing single Redis, add database isolation, document sharing strategy

**Architecture**:
```
redis (Deployment, 1 replica)
├─→ Gateway Service (DB 1)
└─→ Context-API Service (DB 2)
```

**Pros**:
- ✅ **Quick**: 30 minutes to implement
- ✅ **No Resource Overhead**: Single Redis instance
- ✅ **Tests Continue Working**: No deployment changes

**Cons**:
- ❌ **Single Point of Failure**: Redis crash affects both services
- ❌ **No Automatic Failover**: Manual intervention required
- ❌ **HA Tests Cannot Run**: `redis_ha_failure_test.go` skipped
- ❌ **Not Production-Ready**: Violates HA requirements from DD-GATEWAY-003
- ❌ **Shared Resource Contention**: No isolation

**Confidence**: **40%** - Not acceptable for production

---

## Decision

**APPROVED: Alternative A** - Separate Redis HA Instances

**Rationale**:
1. **Production Requirements**: Gateway MUST have HA (BR-GATEWAY-008/009, DD-GATEWAY-003)
2. **Service Isolation**: Gateway and Context-API have different availability requirements
3. **Failure Isolation**: Context-API Redis failure should not affect Gateway (alert processing is critical)
4. **Clear Ownership**: Each service owns its Redis instance (simplifies operations)
5. **Integration Testing**: Enables testing Gateway HA failure scenarios
6. **Future-Proof**: Easy to scale each Redis cluster independently

**Key Insight**: Gateway requires HA (automatic failover) while Context-API can tolerate Redis failures (graceful degradation to L2/L3 cache). Separate instances allow each service to have the right level of availability.

---

## Implementation

**Deployment Steps**:

### 1. Deploy Gateway Redis HA
```bash
# Apply Sentinel ConfigMap
kubectl apply -f deploy/redis-ha/redis-gateway-sentinel-configmap.yaml

# Apply StatefulSet with 3 replicas + Sentinel
kubectl apply -f deploy/redis-ha/redis-gateway-statefulset.yaml

# Verify deployment
kubectl wait --for=condition=ready pod -l app=redis-gateway -n kubernaut-system --timeout=120s

# Verify Sentinel
kubectl exec -n kubernaut-system redis-gateway-0 -c sentinel -- \
  redis-cli -p 26379 sentinel master gateway-master
```

### 2. Update Gateway Configuration
```yaml
# Gateway connects to redis-gateway-ha service
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config
data:
  redis:
    host: redis-gateway-ha.kubernaut-system.svc.cluster.local
    port: "6379"
    db: 0
```

### 3. Context-API Configuration (Unchanged)
```yaml
# Context-API continues using existing redis service
apiVersion: v1
kind: ConfigMap
metadata:
  name: context-api-config
data:
  redis:
    host: redis.kubernaut-system.svc.cluster.local
    port: "6379"
    db: 0
```

**Service Endpoints**:
- **Gateway**: `redis-gateway-ha.kubernaut-system:6379` (HA cluster)
- **Context-API**: `redis.kubernaut-system:6379` (single instance)

**Sentinel Configuration**:
- **Quorum**: 2 (requires 2 Sentinels to agree on master failure)
- **Down After**: 5000ms (5 seconds to detect failure)
- **Failover Timeout**: 10000ms (10 seconds to complete failover)
- **Replicas**: 3 (1 master + 2 replicas)

**Data Migration**:
- No migration needed (Gateway integration tests will populate new Redis)
- Existing Context-API data remains in `redis` deployment

---

## Consequences

**Positive**:
- ✅ **Production-Ready Gateway**: Automatic failover, no single point of failure
- ✅ **Service Isolation**: Gateway and Context-API don't interfere
- ✅ **Failure Isolation**: Redis failure in one service doesn't affect the other
- ✅ **Integration Tests Enabled**: Can test Gateway HA failure scenarios
- ✅ **Clear Ownership**: Each service owns its Redis instance
- ✅ **Independent Scaling**: Can scale Redis based on service-specific needs
- ✅ **Operational Clarity**: Easy to identify which service is causing Redis load

**Negative**:
- ⚠️ **Resource Overhead**: 4 Redis pods (3 HA + 1 single) vs. 1 pod
  - **Mitigation**: Gateway HA is mandatory, Context-API single instance is acceptable
- ⚠️ **Operational Overhead**: Two Redis clusters to monitor
  - **Mitigation**: Prometheus metrics + Grafana dashboards for both clusters
- ⚠️ **Deployment Complexity**: StatefulSet + Sentinel configuration
  - **Mitigation**: Automated deployment scripts, well-documented

**Neutral**:
- 🔄 **Cost**: ~$10-20/month additional cloud cost (negligible for production)
- 🔄 **Monitoring**: Need separate Prometheus ServiceMonitors for each Redis cluster
- 🔄 **Backup**: Need separate backup strategies (Gateway: RDB snapshots, Context-API: cache, no backup needed)

---

## Validation Results

**Deployment Verification** (2025-10-24):
```bash
# ✅ All 3 Redis HA pods running
kubectl get pods -n kubernaut-system -l app=redis-gateway
NAME              READY   STATUS    RESTARTS   AGE
redis-gateway-0   2/2     Running   0          5m
redis-gateway-1   2/2     Running   0          4m
redis-gateway-2   2/2     Running   0          3m

# ✅ Sentinel monitoring master with 2 slaves, quorum=2
kubectl exec -n kubernaut-system redis-gateway-0 -c sentinel -- \
  redis-cli -p 26379 sentinel master gateway-master
name: gateway-master
num-slaves: 2
num-other-sentinels: 2
quorum: 2
```

**Confidence Assessment Progression**:
- Initial assessment: 90% confidence (concern about resource overhead)
- After deployment: 95% confidence (deployment successful, Sentinel working correctly)
- After integration tests: TBD (will validate HA failure scenarios)

---

## Related Decisions
- **Builds On**: DD-GATEWAY-003 (Redis outage metrics - requires HA)
- **Supports**: BR-GATEWAY-008 (deduplication), BR-GATEWAY-009 (storm detection)
- **Related To**: DD-CONTEXT-002 (multi-tier caching - Context-API graceful degradation)
- **Documented In**:
  - [deploy/redis-ha/README.md](../../../deploy/redis-ha/README.md)
  - [REDIS_FAILURE_HANDLING.md](../../services/stateless/gateway-service/REDIS_FAILURE_HANDLING.md)

---

## Review & Evolution

**When to Revisit**:
- If Context-API requires HA (currently graceful degradation is acceptable)
- If resource overhead becomes problematic (>10% of cluster capacity)
- If operational overhead is too high (>2 hours/week for Redis management)
- After 1 month of production metrics (validate resource usage and failover behavior)

**Success Metrics**:
- **Gateway Availability**: >99.9% uptime (with automatic failover)
- **Failover Time**: <10 seconds (Sentinel target)
- **Resource Usage**: <5% of cluster capacity
- **Operational Overhead**: <1 hour/week for Redis management
- **Integration Test Coverage**: 100% of HA failure scenarios tested

**Future Considerations**:
- **v2.0**: Consider Redis Operator for automated management
- **v2.0**: Consider cloud-managed Redis (AWS ElastiCache, GCP Memorystore) for production
- **v2.0**: Evaluate if Context-API needs HA based on production metrics


