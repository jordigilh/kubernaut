# Pre-Day 10 Validation - Execution Plan

**Date**: October 28, 2025  
**Plan Version**: v2.19  
**Status**: 🚀 **STARTING** - Comprehensive Validation Before Day 10  
**Estimated Duration**: 3.5-4 hours  
**Target Confidence**: 100%

---

## 🎯 Objective

Comprehensive validation of all unit tests, integration tests, business logic, Kubernetes deployment, and end-to-end workflows before proceeding to Day 10 final BR coverage.

---

## 📋 Validation Tasks

### **Task 1: Unit Test Validation** (1 hour)
**Status**: ⏭️ PENDING  
**Purpose**: Verify all Gateway unit tests pass with zero errors

**Steps**:
1. Run all Gateway unit tests: `go test ./test/unit/gateway/... -v`
2. Verify zero build errors
3. Verify zero lint errors: `golangci-lint run ./pkg/gateway/...`
4. Triage any failures from Day 1-9 features
5. Target: 100% unit test pass rate

**Success Criteria**:
- ✅ All unit tests pass (100%)
- ✅ Zero build errors
- ✅ Zero lint errors

---

### **Task 2: Integration Test Validation** (1 hour)
**Status**: ⏭️ PENDING  
**Purpose**: Fix disabled integration tests and verify 100% pass rate

**Disabled Tests to Fix** (8 files):
1. `deduplication_ttl_test.go.NEEDS_UPDATE`
2. `error_handling_test.go.NEEDS_UPDATE`
3. `health_integration_test.go.NEEDS_UPDATE`
4. `k8s_api_failure_test.go.NEEDS_UPDATE`
5. `redis_resilience_test.go.NEEDS_UPDATE`
6. `storm_aggregation_test.go.NEEDS_UPDATE`
7. `webhook_integration_test.go.NEEDS_UPDATE`
8. `metrics_integration_test.go.CORRUPTED` (may need reconstruction)
9. `redis_ha_failure_test.go.CORRUPTED` (may need reconstruction)

**Steps**:
1. Refactor integration test helpers if needed (`test/integration/gateway/helpers.go`)
2. Fix disabled tests to use new `ServerConfig` API
3. Run all Gateway integration tests: `./test/integration/gateway/run-tests-kind.sh`
4. Verify infrastructure (Redis, Kind cluster) is healthy
5. Triage any failures
6. Target: 100% integration test pass rate

**Success Criteria**:
- ✅ All 8+ disabled tests fixed and re-enabled
- ✅ All integration tests pass (100%)
- ✅ Infrastructure healthy (Redis, Kind cluster)

---

### **Task 3: Business Logic Validation** (30 minutes)
**Status**: ⏭️ PENDING  
**Purpose**: Verify all Day 1-9 business requirements have passing tests

**Steps**:
1. Verify all Day 1-9 business requirements have passing tests
2. Confirm no orphaned business code (code without tests or main app integration)
3. Run full build: `go build ./cmd/gateway`
4. Target: Zero compilation errors, zero lint warnings

**Success Criteria**:
- ✅ All Day 1-9 BRs have tests
- ✅ No orphaned business code
- ✅ Full build succeeds
- ✅ Zero lint warnings

---

### **Task 4: Kubernetes Deployment Validation** (30-45 minutes)
**Status**: ⏭️ PENDING  
**Purpose**: Validate all Kubernetes manifests in real cluster

**Steps**:
1. Deploy Gateway to Kind cluster:
   ```bash
   kubectl apply -k deploy/gateway/
   ```

2. Verify all pods running:
   ```bash
   kubectl get pods -n kubernaut-gateway -w
   ```

3. Check Gateway logs:
   ```bash
   kubectl logs -n kubernaut-gateway deployment/gateway --tail=100
   ```

4. Verify Redis connectivity:
   - Check logs for "Connected to Redis"

5. Test health endpoint:
   ```bash
   kubectl port-forward -n kubernaut-gateway svc/gateway 8080:8080 &
   curl http://localhost:8080/health
   ```

6. Test readiness endpoint:
   ```bash
   curl http://localhost:8080/ready
   ```

7. Verify metrics endpoint:
   ```bash
   curl http://localhost:8080/metrics | grep gateway_
   ```

**Success Criteria**:
- ✅ All pods Running
- ✅ Zero errors in logs
- ✅ Health endpoint responding (200 OK)
- ✅ Readiness endpoint responding (200 OK)
- ✅ Metrics endpoint responding with Gateway metrics

---

### **Task 5: End-to-End Deployment Test** (30-45 minutes)
**Status**: ⏭️ PENDING  
**Purpose**: Validate complete signal processing workflow in deployed environment

**Steps**:
1. Port-forward Gateway service:
   ```bash
   kubectl port-forward -n kubernaut-gateway svc/gateway 8080:8080
   ```

2. Send test Prometheus alert:
   ```bash
   curl -X POST http://localhost:8080/api/v1/signals/prometheus \
     -H "Content-Type: application/json" \
     -d '{
       "alerts": [{
         "status": "firing",
         "labels": {
           "alertname": "HighMemoryUsage",
           "severity": "critical",
           "namespace": "prod-payment-service",
           "pod": "payment-api-789"
         },
         "annotations": {
           "summary": "Memory usage above 90%"
         }
       }]
     }'
   ```

3. Verify RemediationRequest CRD created:
   ```bash
   kubectl get remediationrequest -n prod-payment-service
   ```

4. Send duplicate alert, verify deduplication (202 response):
   ```bash
   # Send same alert again
   curl -X POST http://localhost:8080/api/v1/signals/prometheus \
     -H "Content-Type: application/json" \
     -d '{ ... same alert ... }'
   ```

5. Send 15 alerts rapidly, verify storm detection and aggregation:
   ```bash
   for i in {1..15}; do
     curl -X POST http://localhost:8080/api/v1/signals/prometheus \
       -H "Content-Type: application/json" \
       -d '{
         "alerts": [{
           "status": "firing",
           "labels": {
             "alertname": "HighCPU",
             "severity": "warning",
             "namespace": "prod-api",
             "pod": "api-'$i'"
           }
         }]
       }'
   done
   ```

6. Verify Gateway metrics updated:
   ```bash
   curl http://localhost:8080/metrics | grep gateway_signals_received_total
   ```

**Success Criteria**:
- ✅ Alert accepted (200 OK)
- ✅ RemediationRequest CRD created
- ✅ Duplicate alert deduplicated (202 Accepted)
- ✅ Storm detection triggered (aggregated CRD created)
- ✅ Metrics updated correctly

---

## 📊 Success Criteria (Overall)

After completing all 5 tasks:
- ✅ All unit tests pass (100%)
- ✅ All integration tests pass (100%)
- ✅ Zero build errors
- ✅ Zero lint errors
- ✅ All Day 1-9 BRs validated
- ✅ Gateway deploys successfully to Kubernetes
- ✅ All pods Running with zero errors
- ✅ Health and readiness endpoints responding
- ✅ End-to-end signal processing works (Prometheus → Gateway → CRD)
- ✅ Deduplication works in deployed environment
- ✅ Storm detection works in deployed environment

**Result**: **100% Confidence** before Day 10

---

## 🚨 If Validation Fails

- **STOP**: Fix failures before proceeding to Day 10
- **DOCUMENT**: Record any deferred fixes with justification
- **UPDATE**: Adjust confidence assessment accordingly

---

## 📈 Confidence Tracking

| Task | Before | After | Status |
|------|--------|-------|--------|
| Unit Tests | 100% | TBD | ⏭️ PENDING |
| Integration Tests | 70% | TBD | ⏭️ PENDING |
| Business Logic | 100% | TBD | ⏭️ PENDING |
| Kubernetes Deployment | 95% | TBD | ⏭️ PENDING |
| E2E Validation | 80% | TBD | ⏭️ PENDING |
| **Overall** | **90%** | **TBD** | ⏭️ **PENDING** |

**Target**: **100% Overall Confidence**

---

## 🔗 Related Documents

- **Implementation Plan**: `docs/services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.19.md`
- **v2.19 Update Summary**: `IMPLEMENTATION_PLAN_V2.19_UPDATE_COMPLETE.md`
- **Day 9 Summary**: `CONFIG_REFACTORING_V2.18_COMPLETE.md`

---

## ⏭️ Next Steps

1. ✅ **Start Task 1**: Unit Test Validation (1h)
2. ⏭️ **Start Task 2**: Integration Test Validation (1h)
3. ⏭️ **Start Task 3**: Business Logic Validation (30min)
4. ⏭️ **Start Task 4**: Kubernetes Deployment Validation (30-45min)
5. ⏭️ **Start Task 5**: E2E Deployment Test (30-45min)

**Status**: 🚀 **READY TO START TASK 1**


