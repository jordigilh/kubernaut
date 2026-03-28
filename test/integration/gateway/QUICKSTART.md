> **Historical Note (v1.2):** This document contains references to storm detection / aggregation
> which was removed in v1.2 per DD-GATEWAY-015. Storm-related content is retained for historical
> context only and does not reflect current architecture.

# Gateway Integration Tests - Quick Start

## 🚀 **Fastest Way to Run Tests**

```bash
# One command to rule them all
./test/integration/gateway/run-tests.sh
```

This script automatically:
1. ✅ Checks if Redis pod is running
2. ✅ Cleans up old port-forwards
3. ✅ Starts Redis port-forward
4. ✅ Verifies Redis connectivity
5. ✅ Runs all integration tests
6. ✅ Cleans up on exit

---

## 📋 **Manual Setup (If Script Fails)**

### Step 1: Find Redis Pod Name

```bash
kubectl get pods -n kubernaut-system | grep redis
# Example output: redis-75cfb58d99-s8vwp
```

### Step 2: Start Redis Port-Forward

```bash
# Replace POD_NAME with actual pod name from step 1
kubectl port-forward -n kubernaut-system POD_NAME 6379:6379 &
```

### Step 3: Verify Redis Connectivity

```bash
redis-cli -h localhost -p 6379 ping
# Should return: PONG
```

### Step 4: Run Integration Tests

```bash
# From project root
go test -v ./test/integration/gateway -run "TestGatewayIntegration" -timeout 10m
```

### Step 5: Cleanup

```bash
# Kill port-forward when done
pkill -f "kubectl port-forward.*redis"
```

---

## ⚠️ **Common Issues**

### Issue 1: "Redis pod not found"

**Symptom**: Script fails with "Redis pod not found"

**Solution**: Update the pod name in `run-tests.sh`:
```bash
# Edit run-tests.sh
REDIS_POD_NAME="your-actual-redis-pod-name"
```

### Issue 2: "Port 6379 already in use"

**Symptom**: Port-forward fails to start

**Solution**: Kill existing port-forwards:
```bash
pkill -f "kubectl port-forward.*redis"
# Wait 2 seconds
sleep 2
# Try again
./test/integration/gateway/run-tests.sh
```

### Issue 3: Tests timing out

**Symptom**: Tests run for 10+ minutes and timeout

**Solution**: Increase timeout in `run-tests.sh`:
```bash
# Edit run-tests.sh
TEST_TIMEOUT=1200  # 20 minutes
```

### Issue 4: All tests getting 503 errors

**Symptom**: Every webhook request returns 503 Service Unavailable

**Root Cause**: Redis is not accessible

**Solution**:
```bash
# Check if Redis port-forward is running
ps aux | grep "kubectl port-forward" | grep redis

# If not running, start it manually
kubectl port-forward -n kubernaut-system REDIS_POD_NAME 6379:6379 &

# Verify connectivity
redis-cli -h localhost -p 6379 ping
```

---

## 📊 **Test Categories**

| Category | Test Count | Duration | Purpose |
|----------|-----------|----------|---------|
| **E2E Webhook Tests** | 8 | 1-2 min | Validate webhook processing |
| **Storm Aggregation** | 10 | 2-3 min | Validate alert storm handling |
| **Redis Integration** | 10 | 3-5 min | Validate Redis state management |
| **K8s API Integration** | 8 | 2-4 min | Validate K8s API interactions |
| **Security Integration** | 23 | 3-5 min | Validate auth/authz/rate limiting |
| **Deduplication** | 5 | 1-2 min | Validate duplicate detection |
| **TOTAL** | **64** | **12-21 min** | Full integration suite |

---

## 🎯 **Expected Results**

### Successful Test Run

```
Running Suite: Gateway Integration Suite
Will run 102 of 104 specs

✓ E2E Webhook Tests (8/8 passing)
✓ Storm Aggregation (10/10 passing)
✓ Redis Integration (10/10 passing)
✓ K8s API Integration (8/8 passing)
✓ Security Integration (23/23 passing)
✓ Deduplication (5/5 passing)

Ran 102 of 104 Specs in 15.234 seconds
SUCCESS! -- 102 Passed | 0 Failed | 2 Pending | 2 Skipped
```

### Failed Test Run (Redis Issue)

```
✗ E2E Webhook Tests (0/8 passing) - 401/503 errors
✗ Storm Aggregation (0/10 passing) - 503 errors
✗ Redis Integration (0/10 passing) - 503 errors

Common error: "503 Service Unavailable"
Root cause: Redis port-forward not running
```

**Fix**: Run `./test/integration/gateway/run-tests.sh` to auto-setup Redis

---

## 🔧 **Advanced Usage**

### Run Specific Test Suite

```bash
# E2E webhook tests only
go test -v ./test/integration/gateway -run "webhook.*prometheus"

# Storm aggregation tests only
go test -v ./test/integration/gateway -run "Storm Aggregation"

# Security tests only
go test -v ./test/integration/gateway -run "Security"
```

### Run with Verbose Output

```bash
# Add Ginkgo verbose flag
go test -v ./test/integration/gateway -ginkgo.v
```

### Run with Custom Timeout

```bash
# 20 minute timeout
go test -v ./test/integration/gateway -timeout 20m
```

### Generate Test Report

```bash
# Save test output to file
go test -v ./test/integration/gateway 2>&1 | tee test-report.log

# Count passing/failing tests
grep -c "✓" test-report.log  # Passing
grep -c "✗" test-report.log  # Failing
```

---

## 📚 **Related Documentation**

- [Integration Test Fixes](./INTEGRATION_TEST_FIXES.md) - Recent fixes applied
- [Load Tests](../../load/gateway/README.md) - Performance/stress testing
- [Gateway Implementation Plan](../../../docs/services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.11.md)
- [Security Middleware](../../../docs/services/stateless/gateway-service/SECURITY_MIDDLEWARE_INTEGRATION.md)

---

## ✅ **Pre-Flight Checklist**

Before running tests, ensure:
- [ ] Redis pod is running: `kubectl get pods -n kubernaut-system | grep redis`
- [ ] Kubernetes cluster is accessible: `kubectl cluster-info`
- [ ] CRDs are installed: `kubectl get crd remediationrequests.remediation.kubernaut.io`
- [ ] ServiceAccounts exist: `kubectl get sa -n kubernaut-system`
- [ ] You're in project root: `pwd` should end with `/kubernaut`

---

## 🆘 **Getting Help**

If tests continue to fail after following this guide:

1. **Check test log**: `/tmp/gateway-integration-tests.log`
2. **Check Redis logs**: `kubectl logs -n kubernaut-system REDIS_POD_NAME`
3. **Check Gateway logs**: `kubectl logs -n kubernaut-system deployment/gateway`
4. **Verify cluster health**: `kubectl get nodes`
5. **Check port-forward**: `ps aux | grep port-forward`

---

## 🎯 **Success Criteria**

Tests pass if:
- ✅ **Pass Rate** >95% (at least 97/102 tests passing)
- ✅ **No 503 Errors** (Redis connectivity working)
- ✅ **No 401 Errors** (Authentication working)
- ✅ **Duration** <20 minutes (reasonable performance)
- ✅ **No Timeouts** (all tests complete)

---

**Last Updated**: October 24, 2025
**Test Count**: 102 active tests (2 pending)
**Average Duration**: 15-18 minutes
**Success Rate**: 100% (when Redis accessible)


