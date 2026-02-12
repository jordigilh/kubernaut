# Gateway Service - Toxiproxy Lightweight Chaos Testing Plan

**Date**: 2024-11-18
**Version**: 1.0
**Target**: DD-GATEWAY-008 & DD-GATEWAY-009 Enhancements
**Tool**: Toxiproxy (Lightweight Network Chaos)
**Duration**: 1-2 days
**Status**: 📋 **READY FOR EXECUTION**

---

## 🎯 Why Toxiproxy?

**Advantages**:
- ✅ Lightweight HTTP proxy (single binary, minimal overhead)
- ✅ Network-level fault injection (latency, timeouts, connection drops)
- ✅ HTTP API for programmatic control (easy automation)
- ✅ No CRDs or Kubernetes dependencies
- ✅ Fast setup (~5 minutes)
- ✅ Runs in-process or as sidecar

**Scope**:
- ✅ Redis failures (master failure, network partition, timeouts)
- ✅ Kubernetes API throttling (HTTP 429, latency)
- ❌ Pod failures (use kubectl delete instead)
- ❌ Memory pressure (use stress-ng or manual Redis CONFIG)
- ❌ Time skew (requires system-level tools)

**Trade-offs**:
- Focuses on **network-level chaos** (80% of critical scenarios)
- Complements manual chaos for pod/time scenarios
- Simpler setup = faster iteration

---

## 🚀 Quick Setup (5 Minutes)

### Step 1: Deploy Toxiproxy as Sidecar

**Deployment Manifest** (`test/chaos/gateway/toxiproxy-deployment.yaml`):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-toxiproxy
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      containers:
      # Gateway container
      - name: gateway
        image: kubernaut/gateway:v1.0
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        # Point to Toxiproxy proxies instead of real services
        - name: REDIS_ADDR
          value: "localhost:6380"  # Toxiproxy Redis proxy
        - name: KUBERNETES_SERVICE_HOST
          value: "localhost"
        - name: KUBERNETES_SERVICE_PORT
          value: "6443"  # Toxiproxy K8s API proxy

      # Toxiproxy sidecar
      - name: toxiproxy
        image: ghcr.io/shopify/toxiproxy:2.9.0
        ports:
        - containerPort: 8474
          name: api
        - containerPort: 6380
          name: redis-proxy
        - containerPort: 6443
          name: k8s-proxy
        command:
        - toxiproxy-server
        args:
        - --host=0.0.0.0
        - --port=8474
        livenessProbe:
          httpGet:
            path: /version
            port: 8474
          initialDelaySeconds: 5
          periodSeconds: 10
```

**Deploy**:
```bash
# Deploy Gateway with Toxiproxy sidecar
kubectl apply -f test/chaos/gateway/toxiproxy-deployment.yaml

# Wait for ready
kubectl wait --for=condition=ready pod -l app=gateway --timeout=60s

# Verify Toxiproxy is running
kubectl exec -it deployment/gateway-toxiproxy -c toxiproxy -- toxiproxy-cli list
```

### Step 2: Configure Proxies

**Setup Script** (`test/chaos/gateway/scripts/setup-toxiproxy.sh`):
```bash
#!/bin/bash
set -e

TOXIPROXY_HOST="localhost:8474"

# Port-forward Toxiproxy API
kubectl port-forward deployment/gateway-toxiproxy 8474:8474 &
PF_PID=$!
sleep 2

# Create Redis proxy
curl -X POST "$TOXIPROXY_HOST/proxies" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis",
    "listen": "0.0.0.0:6380",
    "upstream": "redis-master.default.svc.cluster.local:6379",
    "enabled": true
  }'

# Create K8s API proxy
curl -X POST "$TOXIPROXY_HOST/proxies" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "kubernetes",
    "listen": "0.0.0.0:6443",
    "upstream": "kubernetes.default.svc.cluster.local:443",
    "enabled": true
  }'

# Verify proxies
curl "$TOXIPROXY_HOST/proxies" | jq .

echo "✅ Toxiproxy setup complete"
echo "Redis proxy: localhost:6380 -> redis-master:6379"
echo "K8s API proxy: localhost:6443 -> kubernetes:443"
```

**Run Setup**:
```bash
./test/chaos/gateway/scripts/setup-toxiproxy.sh
```

---

## 🧪 Toxiproxy Chaos Scenarios

### Scenario 1: Redis Connection Failure (DD-GATEWAY-009)

**Toxic**: `down` (simulate Redis master failure)

**Inject Chaos**:
```bash
# Add "down" toxic (blocks all connections)
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis_down",
    "type": "down",
    "attributes": {},
    "toxicity": 1.0
  }'
```

**Test Execution**:
```bash
# 1. Baseline (10 alerts)
./test/chaos/gateway/scripts/send-test-alerts.sh --count 10

# 2. Inject Redis failure
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -d '{"name": "redis_down", "type": "down", "toxicity": 1.0}'

# 3. Send alerts during failure (should fallback to K8s API)
./test/chaos/gateway/scripts/send-test-alerts.sh --count 20

# 4. Verify circuit breaker activated
kubectl logs deployment/gateway-toxiproxy -c gateway | grep "circuit breaker.*open"

# 5. Check fallback metrics
curl http://localhost:9090/metrics | grep gateway_dedup_fallback_total

# 6. Remove toxic (restore Redis)
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_down"

# 7. Verify recovery
./test/chaos/gateway/scripts/send-test-alerts.sh --count 10
```

**Expected Behavior**:
- ✅ Gateway detects Redis connection failure within 3 seconds
- ✅ Circuit breaker opens after 5% failure rate
- ✅ Falls back to K8s API direct query
- ✅ All 20 alerts processed (no loss)
- ✅ Automatic recovery after toxic removed

**Validation**:
```bash
# Zero data loss
ALERTS_SENT=40  # 10 + 20 + 10
CRDS_CREATED=$(kubectl get remediationrequests --no-headers | wc -l)
echo "Sent: $ALERTS_SENT, Created: $CRDS_CREATED, Loss: $((ALERTS_SENT - CRDS_CREATED))"
```

---

### Scenario 2: Redis Network Latency (DD-GATEWAY-009)

**Toxic**: `latency` + `timeout` (simulate network degradation)

**Inject Chaos**:
```bash
# Add latency toxic (500ms ± 100ms jitter)
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis_latency",
    "type": "latency",
    "attributes": {
      "latency": 500,
      "jitter": 100
    },
    "toxicity": 1.0
  }'

# Add timeout toxic (force 5% of connections to timeout)
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis_timeout",
    "type": "timeout",
    "attributes": {
      "timeout": 50
    },
    "toxicity": 0.05
  }'
```

**Test Execution**:
```bash
# 1. Send alerts during latency
./test/chaos/gateway/scripts/send-test-alerts.sh --count 50 --rate 20/min

# 2. Monitor Redis operation duration
kubectl logs deployment/gateway-toxiproxy -c gateway | grep "redis operation duration"

# 3. Check timeout rate
curl http://localhost:9090/metrics | grep gateway_redis_operation_errors_total

# 4. Remove toxics
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_latency"
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_timeout"
```

**Expected Behavior**:
- ✅ Redis operations take 500ms (baseline ~2ms)
- ✅ 5% of operations timeout (trigger fallback)
- ✅ Circuit breaker does NOT open (5% < 10% threshold)
- ✅ Deduplication continues with degraded performance
- ✅ No alerts lost

**Validation**:
```promql
# Latency increase
histogram_quantile(0.95, rate(gateway_redis_operation_duration_seconds_bucket[1m])) > 0.5

# Timeout rate
rate(gateway_redis_operation_errors_total{error="timeout"}[1m]) /
rate(gateway_redis_operations_total[1m]) < 0.1  # Below circuit breaker threshold
```

---

### Scenario 3: Redis Connection Limit (DD-GATEWAY-009)

**Toxic**: `limit_data` + `slow_close` (exhaust connection pool)

**Inject Chaos**:
```bash
# Limit data rate (1KB/s, forces slow operations)
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis_slow",
    "type": "limit_data",
    "attributes": {
      "bytes": 1000
    },
    "toxicity": 1.0
  }'

# Delay connection close (5 seconds, prevents pool recycling)
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis_slow_close",
    "type": "slow_close",
    "attributes": {
      "delay": 5000
    },
    "toxicity": 1.0
  }'
```

**Test Execution**:
```bash
# 1. Monitor connection pool size
watch -n 1 'redis-cli -h localhost -p 6380 CLIENT LIST | wc -l'

# 2. Send burst of alerts (stress connection pool)
./test/chaos/gateway/scripts/send-test-alerts.sh --count 100 --rate 100/min

# 3. Verify pool does not exhaust
kubectl logs deployment/gateway-toxiproxy -c gateway | grep "connection pool exhausted"

# 4. Check circuit breaker activation
curl http://localhost:9090/metrics | grep gateway_circuit_breaker_state

# 5. Remove toxics
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_slow"
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_slow_close"
```

**Expected Behavior**:
- ✅ Connection pool growth slows (delayed recycling)
- ✅ Circuit breaker activates before pool exhaustion
- ✅ Falls back to K8s API (prevents cascade failure)
- ✅ No connection pool exhaustion
- ✅ Automatic recovery after toxics removed

**Validation**:
```bash
# Pool size remains under limit
MAX_POOL_SIZE=50
CURRENT_POOL_SIZE=$(redis-cli -h localhost -p 6380 CLIENT LIST | wc -l)
[ "$CURRENT_POOL_SIZE" -lt "$MAX_POOL_SIZE" ] && echo "✅ PASS: Pool not exhausted" || echo "❌ FAIL"
```

---

### Scenario 4: Kubernetes API HTTP 429 (DD-GATEWAY-009)

**Toxic**: `http` with status code injection (simulate rate limiting)

**Note**: Toxiproxy's `http` toxic is limited. Use alternative approach:

**Option A: nginx Reverse Proxy** (intercepts K8s API):
```yaml
# test/chaos/gateway/nginx-k8s-proxy.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-k8s-proxy
data:
  nginx.conf: |
    events {}
    http {
      upstream k8s_api {
        server kubernetes.default.svc.cluster.local:443;
      }

      server {
        listen 6443;

        location / {
          # Rate limit: 100 req/s
          limit_req_zone $binary_remote_addr zone=k8s_api:10m rate=100r/s;
          limit_req zone=k8s_api burst=10 nodelay;
          limit_req_status 429;

          proxy_pass https://k8s_api;
          proxy_ssl_verify off;
        }
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-k8s-proxy
spec:
  # ... (deploy nginx as sidecar)
```

**Option B: Manual Throttling Simulation** (simpler for testing):
```bash
# Test K8s API throttling handling without Toxiproxy
# Use a custom test that mocks HTTP 429 responses

go test -v ./test/integration/gateway \
  -run TestKubernetesAPIThrottling \
  -args --mock-k8s-429=true
```

**Test Execution** (Option B):
```bash
# Run integration test with mocked 429 responses
cd test/integration/gateway
go test -v -run TestStateBasedDeduplication_KubernetesAPIThrottling

# Test validates:
# - HTTP 429 detection
# - Exponential backoff (1s -> 2s -> 4s -> 8s)
# - Circuit breaker activation
# - Request queuing (no drops)
```

**Expected Behavior**:
- ✅ Gateway detects HTTP 429 from K8s API
- ✅ Exponential backoff applied (1s → 2s → 4s → 8s → 16s max)
- ✅ Circuit breaker activates after 10 consecutive 429s
- ✅ Requests queued (not dropped) during throttling
- ✅ Redis cache reduces K8s API load by 97%

---

### Scenario 5: Redis Packet Loss (DD-GATEWAY-009)

**Toxic**: `slicer` (simulate packet corruption/loss)

**Inject Chaos**:
```bash
# Add packet slicer (randomly drop 30% of packets)
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "redis_packet_loss",
    "type": "slicer",
    "attributes": {
      "average_size": 100,
      "size_variation": 50,
      "delay": 10
    },
    "toxicity": 0.3
  }'
```

**Test Execution**:
```bash
# 1. Send alerts during packet loss
./test/chaos/gateway/scripts/send-test-alerts.sh --count 30

# 2. Monitor Redis protocol errors
kubectl logs deployment/gateway-toxiproxy -c gateway | grep "redis protocol error"

# 3. Check reconnection rate
curl http://localhost:9090/metrics | grep gateway_redis_reconnect_total

# 4. Verify graceful degradation
curl http://localhost:9090/metrics | grep gateway_dedup_fallback_total

# 5. Remove toxic
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_packet_loss"
```

**Expected Behavior**:
- ✅ 30% of Redis operations fail with protocol errors
- ✅ Gateway reconnects to Redis automatically
- ✅ Circuit breaker activates after 10% failure rate
- ✅ Falls back to K8s API direct query
- ✅ No alerts lost

---

### Scenario 6: Combined Chaos (Redis + K8s API)

**Toxic**: Multiple toxics simultaneously

**Inject Chaos**:
```bash
# Redis: Latency + 5% timeouts
curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -d '{"name": "redis_latency", "type": "latency", "attributes": {"latency": 300, "jitter": 50}, "toxicity": 1.0}'

curl -X POST "http://localhost:8474/proxies/redis/toxics" \
  -d '{"name": "redis_timeout", "type": "timeout", "attributes": {"timeout": 50}, "toxicity": 0.05}'

# K8s API: 200ms latency (simulate API server load)
# Note: Requires nginx proxy or use integration test mocks
```

**Test Execution**:
```bash
# 1. Send sustained load (100 alerts over 5 minutes)
./test/chaos/gateway/scripts/send-test-alerts.sh --count 100 --rate 20/min

# 2. Monitor overall system behavior
kubectl logs deployment/gateway-toxiproxy -c gateway --tail=100 -f

# 3. Check Redis cache hit rate (should be high)
curl http://localhost:9090/metrics | grep gateway_dedup_cache_hit_ratio

# 4. Verify K8s API load reduction
curl http://localhost:9090/metrics | grep gateway_k8s_api_queries_total

# 5. Remove all toxics
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_latency"
curl -X DELETE "http://localhost:8474/proxies/redis/toxics/redis_timeout"
```

**Expected Behavior**:
- ✅ Redis cache hit rate >97% (reduces K8s API load)
- ✅ Gateway handles degraded Redis performance gracefully
- ✅ No circuit breaker activation (failures < threshold)
- ✅ All alerts processed successfully
- ✅ Latency increased but no failures

---

## 🛠️ Automated Test Framework

### Master Test Script

**Script** (`test/chaos/gateway/scripts/run-toxiproxy-chaos.sh`):
```bash
#!/bin/bash
set -e

TOXIPROXY_HOST="localhost:8474"
RESULTS_DIR="test/chaos/gateway/results"
SCENARIOS=(
  "redis_down:60s"
  "redis_latency:90s"
  "redis_slow_connections:90s"
  "redis_packet_loss:60s"
  "combined_chaos:120s"
)

mkdir -p "$RESULTS_DIR"

# Setup Toxiproxy
echo "🚀 Setting up Toxiproxy..."
./test/chaos/gateway/scripts/setup-toxiproxy.sh

# Port-forward Toxiproxy API
kubectl port-forward deployment/gateway-toxiproxy 8474:8474 &
PF_PID=$!
sleep 2

# Run each scenario
for SCENARIO_SPEC in "${SCENARIOS[@]}"; do
  SCENARIO=$(echo "$SCENARIO_SPEC" | cut -d':' -f1)
  DURATION=$(echo "$SCENARIO_SPEC" | cut -d':' -f2)

  echo ""
  echo "═══════════════════════════════════════════════════════════"
  echo "🧪 Running: $SCENARIO (Duration: $DURATION)"
  echo "═══════════════════════════════════════════════════════════"

  # Collect baseline
  echo "📊 Baseline metrics..."
  ./test/chaos/gateway/scripts/collect-metrics.sh > "$RESULTS_DIR/$SCENARIO-baseline.json"

  # Apply toxic
  echo "💥 Injecting chaos: $SCENARIO"
  ./test/chaos/gateway/scripts/inject-toxic.sh --scenario "$SCENARIO"

  # Send test load
  echo "📡 Sending test load..."
  ./test/chaos/gateway/scripts/send-test-alerts.sh \
    --count 50 \
    --rate 30/min \
    --output "$RESULTS_DIR/$SCENARIO-load.log" &
  LOAD_PID=$!

  # Wait for chaos duration
  DURATION_SECONDS=$(echo "$DURATION" | sed 's/s//')
  echo "⏳ Chaos active for $DURATION..."
  sleep "$DURATION_SECONDS"

  # Remove toxic
  echo "🧹 Removing toxic..."
  ./test/chaos/gateway/scripts/remove-toxic.sh --scenario "$SCENARIO"

  # Wait for load to finish
  wait $LOAD_PID || true

  # Collect post-chaos metrics
  echo "📊 Post-chaos metrics..."
  ./test/chaos/gateway/scripts/collect-metrics.sh > "$RESULTS_DIR/$SCENARIO-post.json"

  # Validate results
  echo "✅ Validating..."
  ./test/chaos/gateway/scripts/validate-toxiproxy-results.sh \
    --scenario "$SCENARIO" \
    --baseline "$RESULTS_DIR/$SCENARIO-baseline.json" \
    --post "$RESULTS_DIR/$SCENARIO-post.json"

  # Cooldown
  echo "❄️  Cooldown (30s)..."
  sleep 30

  echo "✅ Completed: $SCENARIO"
done

# Cleanup
kill $PF_PID 2>/dev/null || true

# Generate report
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "📋 Generating Final Report"
echo "═══════════════════════════════════════════════════════════"
./test/chaos/gateway/scripts/generate-toxiproxy-report.sh --results-dir "$RESULTS_DIR"

echo ""
echo "🎉 Toxiproxy Chaos Testing Complete!"
echo "📁 Results: $RESULTS_DIR"
```

### Toxic Injection Helper

**Script** (`test/chaos/gateway/scripts/inject-toxic.sh`):
```bash
#!/bin/bash
set -e

SCENARIO=$1
TOXIPROXY_HOST="${TOXIPROXY_HOST:-localhost:8474}"

case "$SCENARIO" in
  redis_down)
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_down", "type": "down", "toxicity": 1.0}'
    ;;

  redis_latency)
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_latency", "type": "latency", "attributes": {"latency": 500, "jitter": 100}, "toxicity": 1.0}'
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_timeout", "type": "timeout", "attributes": {"timeout": 50}, "toxicity": 0.05}'
    ;;

  redis_slow_connections)
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_slow", "type": "limit_data", "attributes": {"bytes": 1000}, "toxicity": 1.0}'
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_slow_close", "type": "slow_close", "attributes": {"delay": 5000}, "toxicity": 1.0}'
    ;;

  redis_packet_loss)
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_packet_loss", "type": "slicer", "attributes": {"average_size": 100, "size_variation": 50, "delay": 10}, "toxicity": 0.3}'
    ;;

  combined_chaos)
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_latency", "type": "latency", "attributes": {"latency": 300, "jitter": 50}, "toxicity": 1.0}'
    curl -X POST "$TOXIPROXY_HOST/proxies/redis/toxics" \
      -d '{"name": "redis_timeout", "type": "timeout", "attributes": {"timeout": 50}, "toxicity": 0.05}'
    ;;

  *)
    echo "Unknown scenario: $SCENARIO"
    exit 1
    ;;
esac

echo "✅ Toxic injected: $SCENARIO"
```

### Toxic Removal Helper

**Script** (`test/chaos/gateway/scripts/remove-toxic.sh`):
```bash
#!/bin/bash
set -e

SCENARIO=$1
TOXIPROXY_HOST="${TOXIPROXY_HOST:-localhost:8474}"

# Remove all toxics for the scenario
case "$SCENARIO" in
  redis_down)
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_down"
    ;;

  redis_latency)
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_latency"
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_timeout"
    ;;

  redis_slow_connections)
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_slow"
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_slow_close"
    ;;

  redis_packet_loss)
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_packet_loss"
    ;;

  combined_chaos)
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_latency"
    curl -X DELETE "$TOXIPROXY_HOST/proxies/redis/toxics/redis_timeout"
    ;;

  *)
    echo "Unknown scenario: $SCENARIO"
    exit 1
    ;;
esac

echo "✅ Toxic removed: $SCENARIO"
```

---

## 📊 Validation & Metrics

### Validation Script

**Script** (`test/chaos/gateway/scripts/validate-toxiproxy-results.sh`):
```bash
#!/bin/bash
set -e

SCENARIO=$1
BASELINE=$2
POST_CHAOS=$3

# Parse metrics
BASELINE_ALERTS=$(jq '.alerts_processed' "$BASELINE")
POST_ALERTS=$(jq '.alerts_processed' "$POST_CHAOS")
ALERTS_SENT=$(jq '.alerts_sent' "$POST_CHAOS")

PROCESSED=$((POST_ALERTS - BASELINE_ALERTS))
LOSS=$((ALERTS_SENT - PROCESSED))
LOSS_PCT=$(echo "scale=2; ($LOSS / $ALERTS_SENT) * 100" | bc)

echo "═══════════════════════════════════════════════════════════"
echo "📊 Validation Results: $SCENARIO"
echo "═══════════════════════════════════════════════════════════"
echo "Alerts Sent: $ALERTS_SENT"
echo "Alerts Processed: $PROCESSED"
echo "Data Loss: $LOSS ($LOSS_PCT%)"

# Validate zero data loss
if [ "$LOSS" -eq 0 ]; then
  echo "✅ PASS: Zero data loss"
else
  echo "❌ FAIL: Data loss detected"
  exit 1
fi

# Scenario-specific validations
case "$SCENARIO" in
  redis_down)
    CB_ACTIVATIONS=$(jq '.circuit_breaker_activations.redis' "$POST_CHAOS")
    if [ "$CB_ACTIVATIONS" -gt 0 ]; then
      echo "✅ PASS: Circuit breaker activated"
    else
      echo "❌ FAIL: Circuit breaker did not activate"
      exit 1
    fi
    ;;

  redis_latency)
    P95_LATENCY=$(jq '.redis_p95_latency_ms' "$POST_CHAOS")
    if (( $(echo "$P95_LATENCY > 400" | bc -l) )); then
      echo "✅ PASS: Latency increased as expected ($P95_LATENCY ms)"
    else
      echo "❌ FAIL: Latency not increased ($P95_LATENCY ms)"
      exit 1
    fi
    ;;

  redis_packet_loss)
    RECONNECTS=$(jq '.redis_reconnections' "$POST_CHAOS")
    if [ "$RECONNECTS" -gt 0 ]; then
      echo "✅ PASS: Redis reconnections occurred ($RECONNECTS)"
    else
      echo "❌ FAIL: No reconnections detected"
      exit 1
    fi
    ;;
esac

# Recovery time
RECOVERY_TIME=$(jq '.recovery_time_seconds' "$POST_CHAOS")
if [ "$RECOVERY_TIME" -lt 30 ]; then
  echo "✅ PASS: Recovery within 30s ($RECOVERY_TIME s)"
else
  echo "❌ FAIL: Recovery too slow ($RECOVERY_TIME s)"
  exit 1
fi

echo "═══════════════════════════════════════════════════════════"
echo "✅ All validations passed for $SCENARIO"
echo "═══════════════════════════════════════════════════════════"
```

---

## 🎯 Scenarios Not Covered by Toxiproxy

**Manual Chaos** (complement Toxiproxy):

### 1. Gateway Pod Restart (DD-GATEWAY-008)
```bash
# Manual test: Kill Gateway pod during storm
./test/chaos/gateway/scripts/send-storm-alerts.sh --count 30 --duration 60s &
sleep 10
kubectl delete pod -l app=gateway
kubectl wait --for=condition=ready pod -l app=gateway --timeout=30s
# Validate storm state recovered from Redis
```

### 2. Redis OOM (DD-GATEWAY-008)
```bash
# Manual test: Configure Redis max memory
redis-cli CONFIG SET maxmemory 256mb
redis-cli CONFIG SET maxmemory-policy allkeys-lru
# Send storm (200 alerts)
./test/chaos/gateway/scripts/send-storm-alerts.sh --count 200 --duration 60s
# Monitor memory usage
redis-cli INFO memory | grep used_memory_human
```

### 3. Buffer Expiration (DD-GATEWAY-008)
```bash
# Manual test: Send 2 alerts, wait 65s, send 3rd
./test/chaos/gateway/scripts/send-test-alerts.sh --count 2 --alert-name PodCrashed
sleep 65
./test/chaos/gateway/scripts/send-test-alerts.sh --count 1 --alert-name PodCrashed
# Verify individual CRDs created (buffer expired)
kubectl get remediationrequests -l alert-name=PodCrashed
```

### 4. CRD Update Conflicts (DD-GATEWAY-009)
```bash
# Manual test: Concurrent duplicate alerts
./test/chaos/gateway/scripts/concurrent-updates.sh
# Validates optimistic concurrency control
```

---

## 📈 Success Criteria

| Scenario | Zero Data Loss | Circuit Breaker | Recovery | Specific |
|----------|----------------|-----------------|----------|----------|
| **Redis Down** | ✅ | ✅ <5s | ✅ <30s | Fallback to K8s API |
| **Redis Latency** | ✅ | ❌ (below threshold) | N/A | P95 >400ms |
| **Connection Limit** | ✅ | ✅ <10s | ✅ <30s | Pool not exhausted |
| **Packet Loss** | ✅ | ✅ <10s | ✅ <20s | Reconnections >0 |
| **Combined Chaos** | ✅ | ❌ (graceful degradation) | ✅ <30s | Cache hit rate >97% |

**Overall Success**:
- ✅ Zero data loss across all scenarios
- ✅ Circuit breakers activate when thresholds exceeded
- ✅ Automatic recovery within 30 seconds
- ✅ Observability maintained during chaos

---

## 🗓️ Execution Timeline

**Day 1 (4 hours)**:
- Setup Toxiproxy sidecar (30 min)
- Configure proxies (30 min)
- Create test scripts (3 hours)

**Day 2 (6 hours)**:
- Run 5 Toxiproxy scenarios (5 hours)
- Validate results (30 min)
- Run 4 manual scenarios (30 min)

**Total**: 1-2 days (10 hours)

---

## 📁 Deliverables

- ✅ Toxiproxy deployment manifest
- ✅ Proxy setup script
- ✅ 5 automated Toxiproxy chaos scenarios
- ✅ 4 manual chaos test procedures
- ✅ Validation scripts
- ✅ Final chaos testing report

**Confidence Improvement**: 98% → **99.5%** (post-chaos validation)

---

**Next Steps**:
1. Deploy Toxiproxy sidecar
2. Run setup script
3. Execute automated chaos scenarios
4. Validate results
5. Update confidence assessment

**Ready to start setup?** 🚀

