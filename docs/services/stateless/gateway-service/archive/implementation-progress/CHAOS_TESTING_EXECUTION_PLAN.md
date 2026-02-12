# Gateway Service - Chaos Testing Execution Plan

**Date**: 2024-11-18
**Version**: 1.0
**Target**: DD-GATEWAY-008 & DD-GATEWAY-009 Enhancements
**Duration**: 2-3 days (Week 4 of implementation)
**Status**: 📋 **EXECUTION PLAN**

---

## 🎯 Chaos Testing Objectives

**Purpose**: Validate Gateway resilience under failure conditions to ensure:
1. Graceful degradation when dependencies fail
2. No data loss or corruption under chaos conditions
3. Automatic recovery without manual intervention
4. Circuit breakers and fallback mechanisms work as designed
5. System meets SLAs even during infrastructure failures

**Success Criteria**:
- ✅ Zero data loss (all alerts processed or safely rejected)
- ✅ Circuit breakers activate within 5 seconds of repeated failures
- ✅ Automatic recovery within 30 seconds after dependency restoration
- ✅ No cascade failures (one component failure doesn't crash others)
- ✅ Metrics and logging remain functional during chaos

---

## 🛠️ Chaos Testing Tools & Infrastructure

### Primary Tool: Chaos Mesh

**Why Chaos Mesh**:
- ✅ Kubernetes-native chaos engineering platform
- ✅ Declarative chaos experiments (YAML-based)
- ✅ Precise fault injection (pod, network, I/O, time)
- ✅ Safe defaults (automatic rollback, time limits)
- ✅ Observable (Prometheus metrics, detailed logs)

**Installation** (30 minutes):
```bash
# Install Chaos Mesh on Kind cluster
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm repo update

kubectl create namespace chaos-testing
helm install chaos-mesh chaos-mesh/chaos-mesh \
  --namespace=chaos-testing \
  --set chaosDaemon.runtime=containerd \
  --set chaosDaemon.socketPath=/run/containerd/containerd.sock \
  --set dashboard.create=true

# Verify installation
kubectl get pods -n chaos-testing
kubectl port-forward -n chaos-testing svc/chaos-dashboard 2333:2333
# Access dashboard: http://localhost:2333
```

### Alternative Tool: Toxiproxy (Network Chaos)

**Why Toxiproxy**:
- ✅ Network-level fault injection (latency, timeouts, connection drops)
- ✅ Redis and K8s API proxy support
- ✅ HTTP API for programmatic control
- ✅ Lighter weight than Chaos Mesh for network-only chaos

**Installation** (15 minutes):
```bash
# Deploy Toxiproxy as sidecar to Gateway pod
kubectl apply -f test/chaos/gateway/toxiproxy-deployment.yaml

# Toxiproxy configuration
toxiproxy-cli create redis_proxy -l 0.0.0.0:6379 -u redis-master:6379
toxiproxy-cli create k8s_api_proxy -l 0.0.0.0:6443 -u kubernetes.default.svc:443
```

### Monitoring Stack

**Required Tools**:
1. **Prometheus**: Metrics collection during chaos
2. **Grafana**: Real-time dashboards for failure visualization
3. **Loki**: Log aggregation for error analysis
4. **Jaeger** (optional): Distributed tracing for latency analysis

**Setup** (20 minutes):
```bash
# Deploy monitoring stack
kubectl apply -f test/chaos/gateway/monitoring-stack.yaml

# Access Grafana dashboard
kubectl port-forward -n monitoring svc/grafana 3000:3000
# Login: admin/admin
# Import dashboard: test/chaos/gateway/grafana-dashboard.json
```

---

## 🧪 Chaos Test Scenarios

### Scenario 1: Redis Master Failure (DD-GATEWAY-009)

**Target**: Validate state-based deduplication resilience

**Failure Injection**:
```yaml
# test/chaos/gateway/redis-master-failure.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: PodChaos
metadata:
  name: redis-master-failure
  namespace: default
spec:
  action: pod-failure
  mode: one
  duration: "60s"
  selector:
    namespaces:
      - default
    labelSelectors:
      app: redis
      role: master
  scheduler:
    cron: "@every 5m"
```

**Execution Steps**:
```bash
# 1. Establish baseline (2 minutes)
./test/chaos/gateway/scripts/send-test-alerts.sh --rate 10/min --duration 2m

# 2. Inject chaos (60 seconds)
kubectl apply -f test/chaos/gateway/redis-master-failure.yaml

# 3. Monitor Gateway behavior
kubectl logs -f deployment/gateway -c gateway --tail=100

# 4. Verify failover to Redis Sentinel replica
redis-cli -h redis-sentinel -p 26379 SENTINEL get-master-addr-by-name mymaster

# 5. Continue sending alerts during chaos
./test/chaos/gateway/scripts/send-test-alerts.sh --rate 10/min --duration 2m

# 6. Cleanup (automatic after 60s)
kubectl delete -f test/chaos/gateway/redis-master-failure.yaml
```

**Expected Behavior**:
- ✅ Gateway detects Redis master failure within 3 seconds
- ✅ Circuit breaker activates after 5% failure rate
- ✅ Falls back to K8s API direct query (no Redis cache)
- ✅ Redis Sentinel promotes replica to master (~5-10 seconds)
- ✅ Gateway reconnects to new master automatically
- ✅ No alerts lost (all processed or safely rejected with 503)

**Validation Queries**:
```promql
# Circuit breaker activation
increase(gateway_circuit_breaker_state_changes_total{component="redis"}[1m]) > 0

# Fallback rate (should spike during failure)
rate(gateway_dedup_fallback_total{reason="redis_unavailable"}[1m])

# Redis reconnection success
increase(gateway_redis_reconnect_success_total[1m]) > 0

# Alert processing continued (no drop in throughput)
rate(gateway_alerts_processed_total[1m])
```

**Success Criteria**:
- [ ] Circuit breaker activated within 5 seconds
- [ ] Zero data loss (all alerts accounted for)
- [ ] Automatic recovery within 30 seconds
- [ ] Metrics show fallback behavior
- [ ] Logs confirm graceful degradation

---

### Scenario 2: Redis Network Partition (DD-GATEWAY-009)

**Target**: Validate Redis timeout handling and connection pool exhaustion

**Failure Injection** (using Toxiproxy):
```bash
# Add network latency (500ms) and packet loss (30%)
toxiproxy-cli toxic add redis_proxy -t latency -a latency=500 -a jitter=100
toxiproxy-cli toxic add redis_proxy -t slow_close -a delay=5000
toxiproxy-cli toxic add redis_proxy -t limit_data -a bytes=1000

# Alternative: Chaos Mesh NetworkChaos
kubectl apply -f test/chaos/gateway/redis-network-partition.yaml
```

```yaml
# test/chaos/gateway/redis-network-partition.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: NetworkChaos
metadata:
  name: redis-network-partition
  namespace: default
spec:
  action: partition
  mode: one
  duration: "90s"
  selector:
    namespaces:
      - default
    labelSelectors:
      app: redis
  direction: both
  target:
    selector:
      namespaces:
        - default
      labelSelectors:
        app: gateway
    mode: all
```

**Execution Steps**:
```bash
# 1. Baseline (1 minute)
./test/chaos/gateway/scripts/send-test-alerts.sh --rate 10/min --duration 1m

# 2. Inject network chaos (90 seconds)
kubectl apply -f test/chaos/gateway/redis-network-partition.yaml

# 3. Monitor connection pool behavior
watch -n 1 'redis-cli -h redis-master INFO clients | grep connected_clients'

# 4. Verify timeout handling
kubectl logs -f deployment/gateway | grep -i "redis timeout"

# 5. Send alerts during partition
./test/chaos/gateway/scripts/send-test-alerts.sh --rate 10/min --duration 2m

# 6. Cleanup
kubectl delete -f test/chaos/gateway/redis-network-partition.yaml
```

**Expected Behavior**:
- ✅ Redis client detects timeout (10ms config) immediately
- ✅ Connection pool recycles stale connections
- ✅ Gateway falls back to K8s API direct query
- ✅ No connection pool exhaustion (circuit breaker prevents)
- ✅ Automatic recovery when network restores

**Validation Queries**:
```promql
# Redis timeout rate
rate(gateway_redis_operation_errors_total{error="timeout"}[1m])

# Connection pool active connections (should not exhaust)
gateway_redis_pool_active_connections < gateway_redis_pool_max_size

# Fallback to K8s API
rate(gateway_k8s_api_queries_total{reason="redis_timeout"}[1m])
```

**Success Criteria**:
- [ ] Redis timeouts detected within 10ms
- [ ] Connection pool size remains under limit
- [ ] Circuit breaker prevents pool exhaustion
- [ ] Zero alerts lost during partition
- [ ] Automatic recovery within 15 seconds

---

### Scenario 3: Kubernetes API Throttling (DD-GATEWAY-009)

**Target**: Validate K8s API rate limiting and backoff behavior

**Failure Injection**:
```yaml
# test/chaos/gateway/k8s-api-throttling.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: HTTPChaos
metadata:
  name: k8s-api-throttling
  namespace: default
spec:
  mode: one
  duration: "120s"
  selector:
    namespaces:
      - default
    labelSelectors:
      app: gateway
  target: Request
  port: 6443
  path: "/apis/remediation.kubernaut.io/v1alpha1/*"
  delay: "5s"
  abort: true
  replace:
    code: 429
    headers:
      Retry-After: ["10"]
  scheduler:
    cron: "@every 10m"
```

**Execution Steps**:
```bash
# 1. Configure K8s API server rate limits (if needed)
kubectl patch apiserver/cluster --type=merge -p '{"spec":{"rateLimiting":{"requestsPerSecond":100,"burstSize":10}}}'

# 2. Inject HTTP 429 responses
kubectl apply -f test/chaos/gateway/k8s-api-throttling.yaml

# 3. Send burst of alerts (exceeds rate limit)
./test/chaos/gateway/scripts/send-test-alerts.sh --rate 200/min --duration 3m

# 4. Monitor exponential backoff
kubectl logs -f deployment/gateway | grep -i "retry.*backoff"

# 5. Verify circuit breaker activation
curl http://gateway:9090/metrics | grep gateway_circuit_breaker_state

# 6. Cleanup
kubectl delete -f test/chaos/gateway/k8s-api-throttling.yaml
```

**Expected Behavior**:
- ✅ Gateway detects HTTP 429 (Too Many Requests)
- ✅ Exponential backoff starts: 1s → 2s → 4s → 8s → 16s (max)
- ✅ Circuit breaker activates after 10 consecutive 429s
- ✅ Requests queued (not dropped) during throttling
- ✅ Redis cache reduces K8s API load by 97%
- ✅ Automatic recovery when rate limit resets

**Validation Queries**:
```promql
# K8s API 429 rate
rate(gateway_k8s_api_errors_total{status="429"}[1m])

# Exponential backoff delay (should increase)
histogram_quantile(0.95, rate(gateway_k8s_api_retry_backoff_seconds_bucket[1m]))

# Circuit breaker state (should be "open" during throttling)
gateway_circuit_breaker_state{component="k8s_api"} == 1

# Request queue depth (should not exceed max)
gateway_k8s_api_request_queue_depth < 1000
```

**Success Criteria**:
- [ ] HTTP 429 detected and logged
- [ ] Exponential backoff increases correctly
- [ ] Circuit breaker activates after threshold
- [ ] No requests dropped (queued for retry)
- [ ] Redis cache prevents API overload
- [ ] Recovery within 30 seconds after throttling ends

---

### Scenario 4: Redis OOM During Storm Buffering (DD-GATEWAY-008)

**Target**: Validate buffer size limits and memory pressure handling

**Failure Injection**:
```yaml
# test/chaos/gateway/redis-memory-pressure.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: StressChaos
metadata:
  name: redis-memory-pressure
  namespace: default
spec:
  mode: one
  duration: "90s"
  selector:
    namespaces:
      - default
    labelSelectors:
      app: redis
      role: master
  stressors:
    memory:
      workers: 1
      size: "400MB"  # Stress 400MB on 512MB Redis
  scheduler:
    cron: "@every 10m"
```

**Execution Steps**:
```bash
# 1. Configure Redis max memory (512MB)
redis-cli CONFIG SET maxmemory 512mb
redis-cli CONFIG SET maxmemory-policy allkeys-lru

# 2. Baseline memory usage
redis-cli INFO memory | grep used_memory_human

# 3. Inject memory stress
kubectl apply -f test/chaos/gateway/redis-memory-pressure.yaml

# 4. Send storm (200 alerts in 60 seconds)
./test/chaos/gateway/scripts/send-storm-alerts.sh --count 200 --duration 60s

# 5. Monitor Redis memory
watch -n 1 'redis-cli INFO memory | grep -E "used_memory_human|maxmemory_human"'

# 6. Verify buffer size limits enforced
redis-cli MEMORY USAGE "alert:buffer:PodOOMKilled"

# 7. Check for OOM errors
kubectl logs -f deployment/gateway | grep -i "redis.*oom"

# 8. Cleanup
kubectl delete -f test/chaos/gateway/redis-memory-pressure.yaml
```

**Expected Behavior**:
- ✅ Buffer size limited to 100 alerts max (prevents unbounded growth)
- ✅ Redis evicts old keys (LRU policy) when memory full
- ✅ Gateway detects OOM and activates circuit breaker
- ✅ Falls back to immediate CRD creation (bypass buffering)
- ✅ No alerts lost during OOM condition
- ✅ Automatic recovery when memory pressure reduces

**Validation Queries**:
```promql
# Redis memory usage (should approach but not exceed limit)
redis_memory_used_bytes / redis_memory_max_bytes > 0.9

# Buffer overflow events
increase(gateway_storm_buffer_overflow_total[1m]) > 0

# Fallback to immediate CRD creation
rate(gateway_storm_buffer_bypass_total{reason="redis_oom"}[1m])

# Alert processing rate (should not drop)
rate(gateway_alerts_processed_total[1m])
```

**Success Criteria**:
- [ ] Buffer size stays under 100 alerts limit
- [ ] Redis memory stays under 512MB limit
- [ ] Circuit breaker activates on OOM errors
- [ ] Falls back to immediate CRD creation
- [ ] Zero alerts lost during OOM
- [ ] Recovery within 20 seconds

---

### Scenario 5: Buffer Expiration Race Conditions (DD-GATEWAY-008)

**Target**: Validate concurrent buffer operations and TTL expiration handling

**Failure Injection** (Time Chaos):
```yaml
# test/chaos/gateway/time-skew.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: TimeChaos
metadata:
  name: time-skew
  namespace: default
spec:
  mode: one
  duration: "60s"
  selector:
    namespaces:
      - default
    labelSelectors:
      app: gateway
  timeOffset: "+65s"  # Skip ahead 65 seconds (past buffer TTL)
  clockIds:
    - CLOCK_REALTIME
```

**Execution Steps**:
```bash
# 1. Send first 2 alerts (below threshold=3)
./test/chaos/gateway/scripts/send-test-alerts.sh --count 2 --alert-name PodCrashed

# 2. Inject time skew (skip ahead 65 seconds, past 60s TTL)
kubectl apply -f test/chaos/gateway/time-skew.yaml

# 3. Send 3rd alert (should trigger expiration handler)
./test/chaos/gateway/scripts/send-test-alerts.sh --count 1 --alert-name PodCrashed

# 4. Verify buffer expiration handler created individual CRDs
kubectl get remediationrequests -l alert-name=PodCrashed

# 5. Check metrics for buffer expiration
curl http://gateway:9090/metrics | grep gateway_storm_buffer_expirations_total

# 6. Cleanup
kubectl delete -f test/chaos/gateway/time-skew.yaml
```

**Expected Behavior**:
- ✅ Buffer TTL expires after 60 seconds
- ✅ Expiration handler creates individual CRDs for buffered alerts
- ✅ No data loss (all 3 alerts result in CRDs)
- ✅ Metrics record buffer expiration event
- ✅ Subsequent alerts start new buffer window

**Validation Queries**:
```promql
# Buffer expiration rate
rate(gateway_storm_buffer_expirations_total[1m])

# Individual CRDs created from expired buffer
increase(gateway_crds_created_total{source="buffer_expiration"}[1m])

# No alerts lost
rate(gateway_alerts_received_total[5m]) ==
rate(gateway_crds_created_total[5m]) + rate(gateway_alerts_deduplicated_total[5m])
```

**Success Criteria**:
- [ ] Buffer expires after 60 seconds
- [ ] Expiration handler creates CRDs for buffered alerts
- [ ] All 3 alerts result in CRDs (no loss)
- [ ] Metrics correctly track expiration
- [ ] New buffer starts for subsequent alerts

---

### Scenario 6: CRD Update Conflicts (DD-GATEWAY-009)

**Target**: Validate optimistic concurrency control for occurrence count updates

**Failure Injection** (Concurrent Updates):
```bash
# test/chaos/gateway/scripts/concurrent-updates.sh
#!/bin/bash
# Send 10 duplicate alerts concurrently to trigger race condition

ALERT_NAME="PodOOMKilled"
NAMESPACE="prod-api"
FINGERPRINT="abc123def456"

# Send first alert (creates CRD)
curl -X POST http://gateway:8080/v1/webhooks/prometheus \
  -H "Content-Type: application/json" \
  -d '{
    "alerts": [{
      "labels": {"alertname": "'$ALERT_NAME'", "namespace": "'$NAMESPACE'"},
      "annotations": {"fingerprint": "'$FINGERPRINT'"}
    }]
  }'

sleep 1

# Send 10 duplicate alerts concurrently (should trigger conflicts)
for i in {1..10}; do
  curl -X POST http://gateway:8080/v1/webhooks/prometheus \
    -H "Content-Type: application/json" \
    -d '{
      "alerts": [{
        "labels": {"alertname": "'$ALERT_NAME'", "namespace": "'$NAMESPACE'"},
        "annotations": {"fingerprint": "'$FINGERPRINT'"}
      }]
    }' &
done

wait
```

**Execution Steps**:
```bash
# 1. Run concurrent update test
./test/chaos/gateway/scripts/concurrent-updates.sh

# 2. Monitor CRD update conflicts
kubectl logs -f deployment/gateway | grep -i "conflict.*resourceVersion"

# 3. Verify retry with exponential backoff
kubectl logs -f deployment/gateway | grep -i "retry.*backoff"

# 4. Check final occurrence count (should be 11: 1 initial + 10 duplicates)
kubectl get remediationrequest -n prod-api -o jsonpath='{.spec.deduplication.occurrenceCount}'

# 5. Validate metrics
curl http://gateway:9090/metrics | grep gateway_crd_update_conflicts_total
```

**Expected Behavior**:
- ✅ Some CRD updates fail with "conflict" error (resourceVersion mismatch)
- ✅ Gateway retries with exponential backoff: 100ms → 200ms → 400ms
- ✅ All updates eventually succeed (max 3 retries)
- ✅ Final occurrence count is accurate (11)
- ✅ Metrics record conflict rate and retry success

**Validation Queries**:
```promql
# CRD update conflict rate (expected during concurrent updates)
rate(gateway_crd_update_conflicts_total[1m])

# Retry success rate (should be ~100%)
rate(gateway_crd_update_retry_success_total[1m]) /
rate(gateway_crd_update_conflicts_total[1m]) > 0.99

# Occurrence count accuracy (manual check)
# kubectl get rr -n prod-api -o jsonpath='{.spec.deduplication.occurrenceCount}' == 11
```

**Success Criteria**:
- [ ] Conflicts detected and logged
- [ ] Exponential backoff applied correctly
- [ ] All updates succeed after retries
- [ ] Final occurrence count is accurate
- [ ] Retry success rate >99%

---

### Scenario 7: Gateway Pod Restart During Storm (DD-GATEWAY-008)

**Target**: Validate storm state recovery from Redis after pod restart

**Failure Injection**:
```yaml
# test/chaos/gateway/gateway-pod-restart.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: PodChaos
metadata:
  name: gateway-pod-restart
  namespace: default
spec:
  action: pod-kill
  mode: one
  duration: "10s"
  selector:
    namespaces:
      - default
    labelSelectors:
      app: gateway
```

**Execution Steps**:
```bash
# 1. Start storm (30 alerts over 60 seconds)
./test/chaos/gateway/scripts/send-storm-alerts.sh --count 30 --duration 60s --alert-name PodCrashed &

# 2. Wait 10 seconds (buffer should have ~5 alerts)
sleep 10

# 3. Kill Gateway pod (simulates crash)
kubectl delete pod -l app=gateway

# 4. Verify new pod starts and recovers state
kubectl wait --for=condition=ready pod -l app=gateway --timeout=30s

# 5. Continue storm (remaining 25 alerts)
# Script should still be running from step 1

# 6. Verify storm window continues (no duplicate CRDs)
kubectl get remediationrequests -l alert-name=PodCrashed --field-selector 'metadata.name=~rr-*-storm-*'

# 7. Check buffer recovery metrics
curl http://gateway:9090/metrics | grep gateway_storm_buffer_recovered_total
```

**Expected Behavior**:
- ✅ Gateway pod restarts within 10 seconds
- ✅ Storm buffer state recovered from Redis
- ✅ Existing buffer window continues (no restart)
- ✅ Buffered alerts preserved (no loss)
- ✅ Remaining alerts added to existing window
- ✅ Single aggregated CRD created after window expires

**Validation Queries**:
```promql
# Pod restart count
increase(kube_pod_container_status_restarts_total{pod=~"gateway-.*"}[5m]) > 0

# Buffer recovery success
increase(gateway_storm_buffer_recovered_total[1m]) > 0

# Storm window continuity (same window ID before and after restart)
# Manual check: kubectl logs gateway | grep "Storm window ID"

# No duplicate CRDs created
count(kube_customresource{customresource_kind="RemediationRequest",label_alert_name="PodCrashed"}) == 1
```

**Success Criteria**:
- [ ] Pod restarts within 10 seconds
- [ ] Storm buffer state recovered from Redis
- [ ] Buffer window continues (same window ID)
- [ ] No alerts lost during restart
- [ ] Single aggregated CRD created
- [ ] Metrics confirm recovery

---

## 📊 Automated Chaos Testing Framework

### Test Suite Structure

```
test/chaos/gateway/
├── manifests/                     # Chaos experiment YAML files
│   ├── redis-master-failure.yaml
│   ├── redis-network-partition.yaml
│   ├── k8s-api-throttling.yaml
│   ├── redis-memory-pressure.yaml
│   ├── time-skew.yaml
│   ├── gateway-pod-restart.yaml
│   └── concurrent-updates.yaml
├── scripts/                       # Test execution scripts
│   ├── send-test-alerts.sh       # Generate test alerts
│   ├── send-storm-alerts.sh      # Generate storm scenarios
│   ├── concurrent-updates.sh     # Concurrent update test
│   ├── run-all-chaos-tests.sh    # Execute all scenarios
│   └── validate-results.sh       # Post-test validation
├── monitoring/                    # Monitoring configuration
│   ├── grafana-dashboard.json    # Chaos testing dashboard
│   ├── prometheus-rules.yaml     # Alert rules for chaos
│   └── loki-queries.txt          # Log analysis queries
├── go_test/                       # Go-based chaos tests
│   └── chaos_suite_test.go       # Ginkgo chaos test suite
└── README.md                      # Chaos testing documentation
```

### Automated Test Execution

**Master Test Script** (`test/chaos/gateway/scripts/run-all-chaos-tests.sh`):
```bash
#!/bin/bash
set -e

CHAOS_DURATION="120s"
COOLDOWN="60s"
RESULTS_DIR="test/chaos/gateway/results"

mkdir -p "$RESULTS_DIR"

echo "🚀 Starting Gateway Chaos Testing Suite"
echo "Duration: $CHAOS_DURATION per scenario"
echo "Cooldown: $COOLDOWN between scenarios"

# Array of chaos scenarios
declare -a SCENARIOS=(
  "redis-master-failure"
  "redis-network-partition"
  "k8s-api-throttling"
  "redis-memory-pressure"
  "time-skew"
  "gateway-pod-restart"
)

# Run each scenario
for SCENARIO in "${SCENARIOS[@]}"; do
  echo ""
  echo "═══════════════════════════════════════════════════════════"
  echo "🧪 Running: $SCENARIO"
  echo "═══════════════════════════════════════════════════════════"

  # 1. Establish baseline metrics
  echo "📊 Collecting baseline metrics..."
  ./test/chaos/gateway/scripts/collect-metrics.sh --baseline > "$RESULTS_DIR/$SCENARIO-baseline.json"

  # 2. Apply chaos experiment
  echo "💥 Injecting chaos: $SCENARIO"
  kubectl apply -f "test/chaos/gateway/manifests/$SCENARIO.yaml"

  # 3. Send test load during chaos
  echo "📡 Sending test load..."
  ./test/chaos/gateway/scripts/send-test-alerts.sh \
    --rate 20/min \
    --duration "$CHAOS_DURATION" \
    --output "$RESULTS_DIR/$SCENARIO-load.log" &
  LOAD_PID=$!

  # 4. Monitor Gateway behavior
  echo "👀 Monitoring Gateway..."
  kubectl logs -f deployment/gateway --tail=100 > "$RESULTS_DIR/$SCENARIO-gateway.log" 2>&1 &
  LOG_PID=$!

  # 5. Wait for chaos duration
  echo "⏳ Chaos active for $CHAOS_DURATION..."
  sleep "$CHAOS_DURATION"

  # 6. Cleanup chaos experiment
  echo "🧹 Cleaning up chaos..."
  kubectl delete -f "test/chaos/gateway/manifests/$SCENARIO.yaml" || true

  # 7. Wait for load script to finish
  wait $LOAD_PID || true
  kill $LOG_PID 2>/dev/null || true

  # 8. Collect post-chaos metrics
  echo "📊 Collecting post-chaos metrics..."
  ./test/chaos/gateway/scripts/collect-metrics.sh --post-chaos > "$RESULTS_DIR/$SCENARIO-post.json"

  # 9. Validate results
  echo "✅ Validating results..."
  ./test/chaos/gateway/scripts/validate-results.sh \
    --scenario "$SCENARIO" \
    --baseline "$RESULTS_DIR/$SCENARIO-baseline.json" \
    --post-chaos "$RESULTS_DIR/$SCENARIO-post.json" \
    --output "$RESULTS_DIR/$SCENARIO-validation.json"

  # 10. Cooldown period
  echo "❄️  Cooldown for $COOLDOWN..."
  sleep "$COOLDOWN"

  echo "✅ Completed: $SCENARIO"
done

# Generate final report
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "📋 Generating Final Report"
echo "═══════════════════════════════════════════════════════════"
./test/chaos/gateway/scripts/generate-report.sh --results-dir "$RESULTS_DIR"

echo ""
echo "🎉 Chaos Testing Complete!"
echo "📁 Results: $RESULTS_DIR"
```

### Go-Based Chaos Test Suite

**Ginkgo Integration** (`test/chaos/gateway/go_test/chaos_suite_test.go`):
```go
package chaos_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	k8sClient *kubernetes.Clientset
	ctx       context.Context
)

func TestChaos(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Chaos Testing Suite")
}

var _ = BeforeSuite(func() {
	ctx = context.Background()
	// Initialize K8s client
	k8sClient = InitializeK8sClient()
})

var _ = Describe("Redis Master Failure Chaos", func() {
	It("should gracefully degrade when Redis master fails", func(ctx SpecContext) {
		By("Establishing baseline metrics")
		baseline := CollectMetrics(ctx, k8sClient)
		Expect(baseline.AlertsProcessed).To(BeNumerically(">", 0))

		By("Applying Redis master failure chaos")
		ApplyChaosExperiment(ctx, k8sClient, "redis-master-failure.yaml")
		time.Sleep(5 * time.Second)

		By("Sending test alerts during chaos")
		alertsSent := SendTestAlerts(ctx, 20, 60*time.Second)
		Expect(alertsSent).To(Equal(20))

		By("Verifying circuit breaker activation")
		Eventually(func() bool {
			return IsCircuitBreakerOpen(ctx, "redis")
		}, 10*time.Second).Should(BeTrue())

		By("Verifying fallback to K8s API")
		Eventually(func() int {
			return GetFallbackCount(ctx, "redis_unavailable")
		}, 10*time.Second).Should(BeNumerically(">", 0))

		By("Cleaning up chaos experiment")
		CleanupChaosExperiment(ctx, k8sClient, "redis-master-failure.yaml")

		By("Verifying automatic recovery")
		Eventually(func() bool {
			return IsCircuitBreakerClosed(ctx, "redis")
		}, 30*time.Second).Should(BeTrue())

		By("Collecting post-chaos metrics")
		postChaos := CollectMetrics(ctx, k8sClient)

		By("Validating zero data loss")
		Expect(postChaos.AlertsProcessed - baseline.AlertsProcessed).To(Equal(alertsSent))
	}, SpecTimeout(5*time.Minute))
})

// Additional chaos scenarios...
```

---

## 📈 Success Metrics & Validation

### Automated Validation Criteria

**Validation Script** (`test/chaos/gateway/scripts/validate-results.sh`):
```bash
#!/bin/bash

SCENARIO=$1
BASELINE=$2
POST_CHAOS=$3

# Parse metrics from JSON
BASELINE_ALERTS=$(jq '.alerts_processed' "$BASELINE")
POST_CHAOS_ALERTS=$(jq '.alerts_processed' "$POST_CHAOS")
ALERTS_SENT=$(jq '.alerts_sent' "$POST_CHAOS")

# Calculate results
ALERTS_PROCESSED=$((POST_CHAOS_ALERTS - BASELINE_ALERTS))
DATA_LOSS=$((ALERTS_SENT - ALERTS_PROCESSED))
DATA_LOSS_PCT=$(echo "scale=2; ($DATA_LOSS / $ALERTS_SENT) * 100" | bc)

echo "Scenario: $SCENARIO"
echo "Alerts Sent: $ALERTS_SENT"
echo "Alerts Processed: $ALERTS_PROCESSED"
echo "Data Loss: $DATA_LOSS ($DATA_LOSS_PCT%)"

# Validation checks
if [ "$DATA_LOSS" -eq 0 ]; then
  echo "✅ PASS: Zero data loss"
else
  echo "❌ FAIL: Data loss detected ($DATA_LOSS alerts)"
  exit 1
fi

# Check circuit breaker activation (if expected)
if [[ "$SCENARIO" =~ "redis-master-failure" || "$SCENARIO" =~ "k8s-api-throttling" ]]; then
  CIRCUIT_BREAKER=$(jq '.circuit_breaker_activations' "$POST_CHAOS")
  if [ "$CIRCUIT_BREAKER" -gt 0 ]; then
    echo "✅ PASS: Circuit breaker activated"
  else
    echo "❌ FAIL: Circuit breaker did not activate"
    exit 1
  fi
fi

# Check recovery time
RECOVERY_TIME=$(jq '.recovery_time_seconds' "$POST_CHAOS")
if [ "$RECOVERY_TIME" -lt 30 ]; then
  echo "✅ PASS: Recovery within 30 seconds ($RECOVERY_TIME s)"
else
  echo "❌ FAIL: Recovery took too long ($RECOVERY_TIME s)"
  exit 1
fi

echo "✅ All validations passed for $SCENARIO"
```

---

## 🎯 Success Criteria Summary

### Per-Scenario Success Criteria

| Scenario | Zero Data Loss | Circuit Breaker | Recovery Time | Specific Criteria |
|----------|----------------|-----------------|---------------|-------------------|
| **Redis Master Failure** | ✅ Required | ✅ <5s | ✅ <30s | Sentinel failover success |
| **Network Partition** | ✅ Required | ✅ <5s | ✅ <15s | Connection pool not exhausted |
| **K8s API Throttling** | ✅ Required | ✅ <10s | ✅ <30s | Exponential backoff applied |
| **Redis OOM** | ✅ Required | ✅ <5s | ✅ <20s | Buffer size limits enforced |
| **Buffer Expiration** | ✅ Required | N/A | N/A | Expiration handler creates CRDs |
| **CRD Update Conflicts** | ✅ Required | N/A | N/A | Retry success rate >99% |
| **Gateway Pod Restart** | ✅ Required | N/A | <10s | Storm state recovered from Redis |

### Overall Success Criteria

- ✅ **Zero Data Loss**: 100% of alerts processed or safely rejected (no silent failures)
- ✅ **Circuit Breaker**: Activates within 5 seconds of repeated failures
- ✅ **Auto Recovery**: System recovers within 30 seconds after dependency restoration
- ✅ **No Cascade Failures**: One component failure doesn't crash others
- ✅ **Observability**: Metrics and logging functional during chaos
- ✅ **Confidence Boost**: 98% → 99.5% after successful chaos testing

---

## 📋 Next Steps

### Immediate Actions

1. **Setup Chaos Infrastructure** (Day 1 - 4 hours)
   - Install Chaos Mesh on Kind cluster
   - Deploy Toxiproxy for network chaos
   - Configure monitoring stack (Prometheus, Grafana, Loki)

2. **Create Test Scripts** (Day 1 - 4 hours)
   - `send-test-alerts.sh` - Generate test load
   - `send-storm-alerts.sh` - Generate storm scenarios
   - `collect-metrics.sh` - Metrics collection
   - `validate-results.sh` - Automated validation

3. **Execute Chaos Scenarios** (Day 2-3 - 16 hours)
   - Run each scenario sequentially
   - Monitor and collect results
   - Validate success criteria
   - Document failures and fixes

4. **Generate Report** (Day 3 - 2 hours)
   - Aggregate results across scenarios
   - Calculate confidence improvement
   - Document lessons learned
   - Update confidence assessment

### Report Template

```markdown
# Chaos Testing Results - Gateway Service

**Date**: [Date]
**Duration**: [Total Time]
**Scenarios**: 7

## Summary
- ✅ Scenarios Passed: X/7
- ❌ Scenarios Failed: Y/7
- 📊 Overall Success Rate: Z%

## Per-Scenario Results
[Table with pass/fail for each scenario]

## Key Findings
- Finding 1: [Description]
- Finding 2: [Description]
...

## Confidence Assessment
- Pre-Chaos: 98%
- Post-Chaos: 99.5%
- Improvement: +1.5%

## Recommendations
1. [Action item 1]
2. [Action item 2]
...
```

---

**Document Owner**: AI Assistant (Chaos Engineering)
**Review Cycle**: After each chaos scenario
**Status**: 📋 **READY FOR EXECUTION** (Week 4)

