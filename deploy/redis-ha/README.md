# ⚠️ **DEPRECATED** - Redis High Availability (HA) for Gateway Service

**Status**: ❌ **DEPRECATED as of V1.0**
**Date**: December 10, 2025
**Reason**: Gateway migrated to Kubernetes-native state management (DD-GATEWAY-012)
**Authority**: [DD-GATEWAY-011 - Shared Status Deduplication](../../docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)

---

## 🚫 **Do Not Deploy - For Historical Reference Only**

**Redis is no longer used by the Gateway service.** Deduplication and state management now use RemediationRequest CRD status fields (DD-GATEWAY-011).

**Current Architecture** (V1.0):
- ✅ **Deduplication**: `status.deduplication` field in RemediationRequest CRDs
- ✅ **State Management**: Kubernetes-native (no external dependencies)
- ✅ **High Availability**: Kubernetes provides built-in CRD persistence and replication

**See**: [NOTICE_DD_GATEWAY_012_REDIS_REMOVAL_COMPLETE.md](../../docs/handoff/NOTICE_DD_GATEWAY_012_REDIS_REMOVAL_COMPLETE.md)

---

## 📚 **Historical Context** (Pre-V1.0 Architecture)

This directory contains Redis HA deployment with Sentinel that was used by the Gateway Service before V1.0. Redis HA ensured that deduplication and storm detection services remained available during Redis instance failures.

## Business Requirements

- **BR-GATEWAY-008**: MUST deduplicate identical signals within 5-minute window
- **BR-GATEWAY-009**: MUST detect alert storms (>10 alerts in 60s)
- **BR-GATEWAY-010**: MUST persist deduplication state in Redis

**Why HA?** Without HA, a single Redis failure would cause the Gateway to reject all requests (503), preventing alert processing. With HA, the system continues processing alerts during partial failures.

## Architecture

### Components

1. **Redis StatefulSet** (3 replicas)
   - `redis-0`: Initial master
   - `redis-1`: Replica
   - `redis-2`: Replica

2. **Redis Sentinel** (3 instances, co-located with Redis)
   - Monitors Redis master health
   - Automatic failover (5-10 seconds)
   - Quorum: 2 (requires 2 Sentinels to agree on failure)

3. **Services**
   - `redis-headless`: Headless service for StatefulSet DNS
   - `redis`: ClusterIP service for client connections

### Failover Behavior

```
Normal Operation:
  Gateway → Redis Master (redis-0) → Success

Master Failure:
  T+0s:   redis-0 crashes
  T+0-5s: Gateway requests → 503 Service Unavailable (Redis down)
  T+5s:   Sentinel detects failure (down-after-milliseconds)
  T+5-10s: Sentinel promotes redis-1 to master
  T+10s+: Gateway requests → Success (automatic reconnection)
```

### Data Integrity Guarantees

| Scenario | Gateway Behavior | Duplicate CRDs? | Storm Protection? |
|----------|------------------|-----------------|-------------------|
| All Redis up | Process normally | ✅ Zero | ✅ Yes |
| 1 replica down | Process normally | ✅ Zero | ✅ Yes |
| Master down (pre-failover) | 503 Service Unavailable | ✅ Zero | ✅ Yes |
| Master down (post-failover) | Process normally | ✅ Zero | ✅ Yes |
| All Redis down | 503 Service Unavailable | ✅ Zero | ✅ Yes |

**Key Insight**: Gateway **never** creates duplicate CRDs, even during Redis failures. It rejects requests (503) rather than compromising data integrity.

## Deployment

### Prerequisites

- Kubernetes cluster (Kind, OCP, or production)
- `kubectl` configured
- `kubernaut-system` namespace exists

### Deploy Redis HA

```bash
# Deploy Redis HA with Sentinel
make deploy-redis-ha

# Verify deployment
make test-redis-ha-status
```

### Manual Deployment

```bash
# Apply configurations
kubectl apply -f deploy/redis-ha/redis-sentinel-configmap.yaml
kubectl apply -f deploy/redis-ha/redis-statefulset.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app=redis -n kubernaut-system --timeout=120s

# Verify Sentinel status
kubectl exec -n kubernaut-system redis-0 -c sentinel -- \
  redis-cli -p 26379 sentinel master mymaster
```

## Configuration

### Redis Configuration

**File**: `redis-statefulset.yaml` → ConfigMap `redis-config`

Key settings:
- `maxmemory: 256mb` - Memory limit per instance
- `maxmemory-policy: allkeys-lru` - Eviction policy
- `save 900 1` - Persistence (RDB snapshots)

### Sentinel Configuration

**File**: `redis-sentinel-configmap.yaml`

Key settings:
- `sentinel monitor mymaster ... 2` - Quorum (2 Sentinels must agree)
- `sentinel down-after-milliseconds mymaster 5000` - Failure detection (5s)
- `sentinel failover-timeout mymaster 10000` - Failover timeout (10s)

## Testing

### Run Redis HA Failure Tests

```bash
# Run all Redis HA failure scenario tests
make test-integration-redis-ha
```

### Test Scenarios

1. **One Replica Down** → Gateway continues processing
2. **Master Down (pre-failover)** → Gateway returns 503
3. **Master Down (post-failover)** → Gateway auto-recovers
4. **All Redis Down** → Gateway returns 503 for all requests

### Manual Failure Testing

```bash
# Test 1: Kill one replica
kubectl delete pod redis-1 -n kubernaut-system
# Expected: Gateway continues processing normally

# Test 2: Kill master
kubectl delete pod redis-0 -n kubernaut-system
# Expected: 503 for 5-10s, then auto-recovery

# Test 3: Scale to zero
kubectl scale statefulset redis -n kubernaut-system --replicas=0
# Expected: All requests return 503

# Test 4: Recover
kubectl scale statefulset redis -n kubernaut-system --replicas=3
# Expected: Automatic recovery after pods ready
```

## Monitoring

### Check Redis HA Status

```bash
# Quick status check
make test-redis-ha-status

# Manual checks
kubectl get pods -n kubernaut-system -l app=redis

# Check Sentinel master
kubectl exec -n kubernaut-system redis-0 -c sentinel -- \
  redis-cli -p 26379 sentinel master mymaster

# Check replicas
kubectl exec -n kubernaut-system redis-0 -c sentinel -- \
  redis-cli -p 26379 sentinel replicas mymaster

# Check Sentinel instances
kubectl exec -n kubernaut-system redis-0 -c sentinel -- \
  redis-cli -p 26379 sentinel sentinels mymaster
```

### Metrics to Monitor

1. **Redis Pod Health**: All 3 pods should be `Running` and `Ready`
2. **Sentinel Quorum**: At least 2 Sentinels must be healthy
3. **Master Status**: One Redis instance should be master, others replicas
4. **Gateway 503 Rate**: Should be near zero (only during failovers)
5. **Failover Events**: Monitor Sentinel logs for failover events

## Troubleshooting

### Gateway Returns 503

**Symptom**: Gateway returns "deduplication service unavailable"

**Diagnosis**:
```bash
# Check Redis pods
kubectl get pods -n kubernaut-system -l app=redis

# Check Sentinel status
make test-redis-ha-status

# Check Gateway logs
kubectl logs -n kubernaut-system deployment/gateway -f | grep "deduplication service unavailable"
```

**Resolution**:
- If all Redis down: Wait for pods to recover or scale up
- If master down: Wait 5-10s for Sentinel failover
- If Sentinel unhealthy: Check Sentinel logs and configuration

### Sentinel Not Detecting Failures

**Symptom**: Master crashes but Sentinel doesn't promote replica

**Diagnosis**:
```bash
# Check Sentinel logs
kubectl logs -n kubernaut-system redis-0 -c sentinel

# Check Sentinel configuration
kubectl exec -n kubernaut-system redis-0 -c sentinel -- \
  redis-cli -p 26379 sentinel master mymaster
```

**Common Issues**:
- Quorum not met (need 2 Sentinels, only 1 healthy)
- `down-after-milliseconds` too high (increase to detect faster)
- Network issues preventing Sentinel communication

### Split-Brain Scenario

**Symptom**: Multiple Redis instances think they're master

**Diagnosis**:
```bash
# Check all Redis instances
for i in 0 1 2; do
  echo "redis-$i:"
  kubectl exec -n kubernaut-system redis-$i -c redis -- \
    redis-cli info replication | grep role
done
```

**Resolution**:
- Restart all Sentinel instances
- Manually force failover: `sentinel failover mymaster`
- In production: Ensure proper network policies and pod anti-affinity

## Cleanup

```bash
# Remove Redis HA deployment
make cleanup-redis-ha

# Manual cleanup
kubectl delete -f deploy/redis-ha/redis-statefulset.yaml
kubectl delete -f deploy/redis-ha/redis-sentinel-configmap.yaml
kubectl delete pvc -n kubernaut-system -l app=redis
```

## Production Considerations

### Resource Requirements

**Per Redis Instance**:
- CPU: 100m request, 500m limit
- Memory: 128Mi request, 512Mi limit
- Storage: 1Gi PVC

**Total (3 instances)**:
- CPU: 300m request, 1.5 cores limit
- Memory: 384Mi request, 1.5Gi limit
- Storage: 3Gi

### High Availability Best Practices

1. **Pod Anti-Affinity**: Spread Redis pods across nodes
   ```yaml
   affinity:
     podAntiAffinity:
       requiredDuringSchedulingIgnoredDuringExecution:
       - labelSelector:
           matchLabels:
             app: redis
         topologyKey: kubernetes.io/hostname
   ```

2. **Node Affinity**: Pin to specific node pool (e.g., stateful workloads)
3. **PVC Storage Class**: Use fast storage (SSD) for Redis
4. **Monitoring**: Set up alerts for Redis pod failures
5. **Backup**: Regular RDB snapshots to external storage

### Security

1. **Authentication**: Add Redis password (update Sentinel config)
2. **Network Policies**: Restrict Redis access to Gateway pods only
3. **TLS**: Enable TLS for Redis and Sentinel connections
4. **RBAC**: Limit access to Redis pods and PVCs

## References

- [Redis Sentinel Documentation](https://redis.io/topics/sentinel)
- [Kubernetes StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [BR-GATEWAY-008](../../docs/requirements/GATEWAY_REQUIREMENTS.md#br-gateway-008)
- [BR-GATEWAY-009](../../docs/requirements/GATEWAY_REQUIREMENTS.md#br-gateway-009)
- [BR-GATEWAY-010](../../docs/requirements/GATEWAY_REQUIREMENTS.md#br-gateway-010)


