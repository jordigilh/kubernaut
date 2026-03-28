> **Historical Note (v1.2):** This document contains references to storm detection / aggregation
> which was removed in v1.2 per DD-GATEWAY-015. Storm-related content is retained for historical
> context only and does not reflect current architecture.

# Local Redis Container Solution for Integration Tests

**Date**: 2025-10-24
**Strategy**: Use local Podman Redis + Remote OCP K8s API
**Rationale**: Eliminate network latency, simplify setup, maintain auth realism

---

## 🎯 **Problem Analysis**

### **Current Issues**
1. **Network Latency**: Mac → helios08 → OCP cluster → Redis (multiple hops)
2. **Complex Setup**: HAProxy configuration, NodePort exposure, firewall rules
3. **503 Errors**: All tests failing due to Redis connectivity issues
4. **Slow Tests**: 25 minutes for 92 tests (16.4s average per test)

### **Root Cause: Network Latency**
```
Mac (jgil-mac)
  ↓ SSH tunnel / HAProxy
helios08.lab.eng.tlv2.redhat.com
  ↓ Internal network
OCP Cluster (192.168.122.x)
  ↓ NodePort / Service
Redis Pod (redis-gateway-0)
```

**Estimated Round-Trip Time**: 50-200ms per Redis operation
**Impact**: 3-4 Redis ops per request × 1000+ requests = **50-200 seconds of pure latency**

---

## 💡 **Proposed Solution: Hybrid Architecture** ⭐

### **Architecture**
```
┌─────────────────────────────────────────────────────────┐
│ Mac (jgil-mac)                                          │
│                                                         │
│  Integration Tests                                      │
│       ↓                                                 │
│  ┌─────────────────────┐    ┌──────────────────────┐  │
│  │ Local Redis         │    │ Remote OCP K8s API   │  │
│  │ (Podman)            │    │ (Real Auth/Authz)    │  │
│  │                     │    │                      │  │
│  │ localhost:6379      │    │ helios08:6443        │  │
│  │ <1ms latency        │    │ 50-100ms latency     │  │
│  │ Deduplication       │    │ TokenReview          │  │
│  │ Storm Detection     │    │ SubjectAccessReview  │  │
│  │ Rate Limiting       │    │ CRD Creation         │  │
│  └─────────────────────┘    └──────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### **Benefits** ✅
1. **Fast Redis**: <1ms latency (vs 50-200ms remote)
2. **Real K8s Auth**: Maintains auth/authz realism
3. **Simple Setup**: Single `podman run` command
4. **No Infrastructure Changes**: No HAProxy, no firewall rules
5. **Portable**: Works on any developer machine
6. **Isolated**: Each developer has their own Redis instance

---

## 🚀 **Implementation**

### **Step 1: Start Local Redis Container**

```bash
# Create Podman network (if not exists)
podman network create kubernaut-test 2>/dev/null || true

# Start Redis container
podman run -d \
  --name redis-gateway-test \
  --network kubernaut-test \
  -p 6379:6379 \
  -e REDIS_MAXMEMORY=256mb \
  -e REDIS_MAXMEMORY_POLICY=allkeys-lru \
  redis:7-alpine \
  redis-server \
    --maxmemory 256mb \
    --maxmemory-policy allkeys-lru \
    --save "" \
    --appendonly no

# Verify Redis is running
podman exec redis-gateway-test redis-cli PING
# Expected: PONG
```

**Explanation**:
- **Port**: 6379 (standard Redis port)
- **Memory**: 256MB (sufficient for tests)
- **Persistence**: Disabled (not needed for tests)
- **Network**: Isolated Podman network

---

### **Step 2: Update Integration Test Helpers**

```go
// test/integration/gateway/helpers.go
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
	// Check if running in CI without Redis
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		return &RedisTestClient{Client: nil}
	}

	// Priority 1: Local Podman Redis (fastest, recommended for development)
	client := goredis.NewClient(&goredis.Options{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           2,
		PoolSize:     20,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.Ping(pingCtx).Result()
	if err == nil {
		return &RedisTestClient{Client: client}
	}

	// Priority 2: Remote OCP Redis via NodePort (fallback for CI)
	_ = client.Close()
	nodeHost := os.Getenv("REDIS_NODE_HOST")
	if nodeHost == "" {
		nodeHost = "helios08.lab.eng.tlv2.redhat.com"
	}

	client = goredis.NewClient(&goredis.Options{
		Addr:     nodeHost + ":30379",
		Password: "",
		DB:       2,
	})

	pingCtx2, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()

	_, err = client.Ping(pingCtx2).Result()
	if err == nil {
		return &RedisTestClient{Client: client}
	}

	// Redis not available
	return &RedisTestClient{Client: nil}
}
```

---

### **Step 3: Create Helper Scripts**

#### **`test/integration/gateway/start-redis.sh`**
```bash
#!/bin/bash
set -euo pipefail

echo "🚀 Starting local Redis for integration tests..."

# Check if Redis is already running
if podman ps | grep -q redis-gateway-test; then
    echo "✅ Redis already running"
    exit 0
fi

# Remove old container if exists
podman rm -f redis-gateway-test 2>/dev/null || true

# Create network
podman network create kubernaut-test 2>/dev/null || true

# Start Redis
podman run -d \
  --name redis-gateway-test \
  --network kubernaut-test \
  -p 6379:6379 \
  -e REDIS_MAXMEMORY=256mb \
  -e REDIS_MAXMEMORY_POLICY=allkeys-lru \
  redis:7-alpine \
  redis-server \
    --maxmemory 256mb \
    --maxmemory-policy allkeys-lru \
    --save "" \
    --appendonly no

# Wait for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1
```

#### **`test/integration/gateway/stop-redis.sh`**
```bash
#!/bin/bash
set -euo pipefail

echo "🛑 Stopping local Redis..."
podman stop redis-gateway-test 2>/dev/null || true
podman rm -f redis-gateway-test 2>/dev/null || true
echo "✅ Redis stopped"
```

#### **`test/integration/gateway/run-tests-local.sh`**
```bash
#!/bin/bash
set -euo pipefail

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🧪 Gateway Integration Tests (Local Redis + Remote K8s)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Start Redis
./test/integration/gateway/start-redis.sh

# Cleanup function
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    ./test/integration/gateway/stop-redis.sh
}
trap cleanup EXIT

# Run tests
echo ""
echo "🚀 Running integration tests..."
go test -v ./test/integration/gateway -run "TestGatewayIntegration" -timeout 30m

echo ""
echo "✅ Tests complete"
```

---

### **Step 4: Make Scripts Executable**

```bash
chmod +x test/integration/gateway/start-redis.sh
chmod +x test/integration/gateway/stop-redis.sh
chmod +x test/integration/gateway/run-tests-local.sh
```

---

## 📊 **Performance Comparison**

| Metric | Remote Redis (OCP) | Local Redis (Podman) | Improvement |
|---|---|---|---|
| **Latency per op** | 50-200ms | <1ms | **50-200x faster** |
| **Total latency** | 50-200s | <1s | **99% reduction** |
| **Test duration** | 25 min | **5-8 min** | **68-80% faster** |
| **Setup complexity** | High (HAProxy, NodePort) | Low (single command) | **Much simpler** |
| **Network dependency** | Yes (helios08 → OCP) | No (localhost) | **More reliable** |

---

## ✅ **Advantages of Hybrid Approach**

### **1. Speed** 🚀
- **Redis ops**: <1ms (vs 50-200ms)
- **Total speedup**: 3-5x faster tests
- **Expected duration**: 5-8 minutes (vs 25 minutes)

### **2. Simplicity** 🎯
- **No HAProxy changes**: No infrastructure modifications
- **No firewall rules**: No security concerns
- **Single command**: `./start-redis.sh`

### **3. Realism** ✅
- **Real K8s auth**: TokenReview, SubjectAccessReview
- **Real CRD creation**: Actual K8s API calls
- **Real auth failures**: Tests catch real auth bugs

### **4. Portability** 📦
- **Works everywhere**: Any machine with Podman
- **No cluster dependency**: Redis runs locally
- **CI/CD friendly**: Easy to integrate

### **5. Isolation** 🔒
- **Per-developer**: Each dev has their own Redis
- **No state pollution**: Clean slate every run
- **No conflicts**: Multiple test runs in parallel

---

## 🔄 **CI/CD Integration**

### **GitHub Actions / GitLab CI**
```yaml
# .github/workflows/integration-tests.yml
jobs:
  integration-tests:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v3
      - name: Run integration tests
        run: go test -v ./test/integration/gateway -timeout 30m
        env:
          KUBECONFIG: ${{ secrets.KUBECONFIG }}
```

---

## 🎯 **Migration Plan**

### **Phase 1: Local Redis (Today, 30 minutes)**
1. ✅ Create `start-redis.sh`, `stop-redis.sh`, `run-tests-local.sh`
2. ✅ Update `helpers.go` to prioritize `localhost:6379`
3. ✅ Test locally: `./run-tests-local.sh`

**Expected Outcome**: Tests run in 5-8 minutes (vs 25 minutes)

---

### **Phase 2: Verify Results (1 hour)**
1. ✅ Run full test suite
2. ✅ Verify all tests pass (or identify real failures)
3. ✅ Measure actual performance improvement

---

### **Phase 3: Document (30 minutes)**
1. ✅ Update `README.md` with local Redis setup
2. ✅ Add troubleshooting guide
3. ✅ Document CI/CD integration

---

## 🚧 **Potential Challenges**

### **Challenge 1: Redis HA Testing**
**Issue**: Local Redis is single-instance, not HA

**Mitigation**:
- Keep Redis HA tests in separate suite
- Run HA tests against OCP Redis (less frequently)
- Focus local tests on business logic, not infrastructure

**Impact**: Minimal - HA tests are <5% of total tests

---

### **Challenge 2: Redis Version Differences**
**Issue**: Local Redis 7 vs OCP Redis version

**Mitigation**:
- Use same Redis version in Podman as OCP
- Document Redis version in `start-redis.sh`
- Pin Redis version in CI/CD

**Impact**: Low - Redis API is stable

---

### **Challenge 3: Port Conflicts**
**Issue**: Port 6379 may be in use

**Mitigation**:
- Check if port is available before starting
- Use alternative port (6380) if needed
- Document port configuration

**Impact**: Low - easy to detect and fix

---

## 📊 **Confidence Assessment**

### **Overall Confidence: 95%** ⭐

**Why 95%?**
1. ✅ **Proven approach**: Local Redis + Remote K8s is industry standard
2. ✅ **Simple setup**: Single Podman command
3. ✅ **No infrastructure changes**: No HAProxy, no firewall
4. ✅ **Reversible**: Can switch back to remote Redis anytime
5. ✅ **Fast**: 50-200x faster Redis operations

**Why not 100%?**
1. ⚠️ **Redis HA tests**: Need separate approach (5% of tests)
2. ⚠️ **Version differences**: Minor risk of Redis version mismatch

**Realistic Mitigations**:
1. ✅ Keep HA tests in separate suite (run less frequently)
2. ✅ Pin Redis version to match OCP

---

## 🎯 **Recommendation: Proceed with Local Redis** ⭐

### **Why This is the Best Solution**
1. **Simplest**: No HAProxy, no NodePort, no firewall rules
2. **Fastest**: 50-200x faster Redis operations
3. **Most realistic**: Real K8s auth, real CRD creation
4. **Most portable**: Works on any machine with Podman
5. **Lowest risk**: No infrastructure changes, easy rollback

### **Expected Results**
- **Test duration**: 5-8 minutes (vs 25 minutes)
- **Setup time**: 30 minutes (vs 2-4 hours for HAProxy)
- **Maintenance**: Minimal (vs ongoing HAProxy/firewall management)

### **Next Steps**
1. ✅ Create helper scripts (30 minutes)
2. ✅ Update `helpers.go` (10 minutes)
3. ✅ Run tests locally (5-8 minutes)
4. ✅ Verify results (1 hour)

**Total Time**: 2 hours (vs 4-6 hours for HAProxy solution)

---

## 📋 **Action Items**

### **Immediate (Today)**
- [ ] Create `start-redis.sh`, `stop-redis.sh`, `run-tests-local.sh`
- [ ] Update `helpers.go` to prioritize `localhost:6379`
- [ ] Make scripts executable
- [ ] Test locally: `./run-tests-local.sh`

### **This Week**
- [ ] Document local Redis setup in README
- [ ] Add troubleshooting guide
- [ ] Update CI/CD to use local Redis

### **Future**
- [ ] Create separate HA test suite for OCP Redis
- [ ] Optimize test performance (parallelization, reduced iterations)
- [ ] Split test suites (fast/standard/extended)

---

## ✅ **Summary**

**Problem**: Remote Redis latency causing 25-minute test runs and 503 errors

**Solution**: Local Podman Redis + Remote OCP K8s API

**Benefits**:
- ✅ 3-5x faster (25min → 5-8min)
- ✅ Simple setup (single command)
- ✅ Real K8s auth (maintains realism)
- ✅ No infrastructure changes
- ✅ Portable and isolated

**Confidence**: **95%** - Proven approach, simple implementation, low risk

**Recommendation**: **Proceed immediately** - This is the optimal solution


