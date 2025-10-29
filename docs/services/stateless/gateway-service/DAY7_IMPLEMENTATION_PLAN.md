# Gateway Service - Day 7 Implementation Plan

**Date**: October 22, 2025
**Status**: 🎯 **READY TO START** - Days 1-6 Complete (114/116 tests passing)
**Focus**: Integration Testing + Production Readiness
**Estimated Duration**: 6-8 hours

---

## Executive Summary

Days 1-6 delivered **core Gateway functionality** with 98.3% test coverage. Day 7 focuses on **integration testing** and **production readiness** to validate the complete webhook-to-CRD flow with real infrastructure.

**Key Objectives**:
1. ✅ Complete K8s API failure integration tests (deferred from Day 6)
2. ✅ End-to-end webhook processing validation
3. ✅ Production readiness assessment
4. ✅ Performance baseline establishment

---

## Current State Assessment

### ✅ **What's Complete** (Days 1-6)

```
Core Functionality: 100% ✅
├── Signal Ingestion (Prometheus + K8s Events)
├── Deduplication (Redis-backed, 5-min TTL)
├── Storm Detection (10 alerts/min threshold)
├── Validation (Fail-fast, early rejection)
├── Classification (Namespace-based)
├── Priority Assignment (Severity × Environment matrix)
└── CRD Creation (RemediationRequest)

Unit Tests: 114/116 passing (98.3%) ✅
Integration Tests: 6 tests (Redis resilience + TTL) ✅
E2E Tests: 0 tests ⏸️
```

### ⏸️ **What's Pending** (Day 7 Focus)

```
Integration Testing:
├── ❌ K8s API failure scenarios (deferred from Day 6)
├── ❌ Full webhook-to-CRD flow with real cluster
├── ❌ Multi-adapter concurrent processing
└── ❌ Storm aggregation end-to-end

Production Readiness:
├── ❌ Performance baseline (latency, throughput)
├── ❌ Resource limits validation
├── ❌ Operational runbooks
└── ❌ Deployment automation
```

---

## Day 7 Implementation Schedule

### **Phase 1: K8s API Failure Integration Tests** (2-3 hours)

#### **Objective**
Implement the K8s API failure integration tests that were deferred from Day 6.

#### **APDC Analysis** (30 min)

**Business Context**:
- **BR-GATEWAY-019**: Error handling must return 500 when K8s API unavailable
- **Business Value**: Prometheus automatic retry achieves eventual consistency

**Technical Context**:
- Existing: `CRDCreator` with error handling
- Existing: `Server` with error response formatting
- Missing: Integration tests with K8s API failure simulation

**Integration Points**:
- Real Kubernetes cluster (Kind or OCP)
- OR Error-injectable K8s client wrapper

**Complexity**: Medium (requires K8s cluster or mock infrastructure)

#### **APDC Plan** (30 min)

**TDD Strategy**:
1. **RED**: Write integration test for K8s API failure
2. **GREEN**: Verify existing error handling works
3. **REFACTOR**: Extract K8s client wrapper if needed

**Integration Approach**:
```go
// Option A: Error-injectable client wrapper
type ErrorInjectableClient struct {
    client.Client
    failCreate bool
    errorMsg   string
}

// Option B: Real K8s cluster with API manipulation
// Requires Kind cluster + network policy manipulation
```

**Success Criteria**:
- ✅ Test returns 500 when K8s API fails
- ✅ Test returns 201 when K8s API recovers
- ✅ Error message includes K8s context for debugging

#### **APDC Do** (1-1.5 hours)

**DO-RED** (30 min):
```bash
# Create integration test file
touch test/integration/gateway/k8s_api_failure_test.go

# Write failing tests:
# 1. Webhook returns 500 when K8s API unavailable
# 2. Webhook returns 201 when K8s API recovers
# 3. Error details include K8s context
# 4. Prometheus retry simulation
```

**DO-GREEN** (30 min):
```go
// Implement error-injectable K8s client
// OR
// Use existing CRDCreator with failing client

// Verify existing error handling works:
// - Server.handlePrometheusWebhook() returns 500
// - respondError() includes K8s error details
// - Metrics increment error counter
```

**DO-REFACTOR** (30 min):
```go
// Extract K8s client wrapper if duplication found
// Improve error messages for operational clarity
// Add structured logging for K8s failures
```

#### **APDC Check** (30 min)

**Validation**:
- ✅ All K8s API failure tests passing
- ✅ Error responses include actionable details
- ✅ Metrics correctly track K8s failures
- ✅ Documentation updated with test instructions

**Confidence Assessment**: Target 95%

---

### **Phase 2: End-to-End Webhook Flow** (2-3 hours)

#### **Objective**
Validate complete webhook-to-CRD flow with real Kubernetes cluster.

#### **APDC Analysis** (30 min)

**Business Context**:
- **BR-GATEWAY-001-015**: Complete signal processing pipeline
- **Business Value**: Confidence in production deployment

**Technical Context**:
- All components implemented and unit tested
- Integration: Webhook → Adapter → Deduplication → Storm → Classification → Priority → CRD
- Infrastructure: Requires Kind/OCP cluster + Redis

**Complexity**: Medium-High (full stack integration)

#### **APDC Plan** (30 min)

**Test Strategy**:
```
Test 1: Prometheus Alert → CRD Creation
├── Send Prometheus webhook
├── Verify CRD created with correct fields
├── Verify deduplication metadata in Redis
└── Verify Prometheus metrics incremented

Test 2: Duplicate Alert → 202 Accepted
├── Send same alert twice
├── First: 201 Created
├── Second: 202 Accepted (duplicate)
└── Verify single CRD, metadata updated

Test 3: Storm Detection → Aggregation
├── Send 15 alerts to same namespace
├── Verify storm flag set in Redis
├── Verify storm metadata returned
└── Verify aggregation behavior

Test 4: Multi-Adapter Concurrent
├── Send Prometheus + K8s Event webhooks concurrently
├── Verify both create CRDs
├── Verify no race conditions
└── Verify metrics accurate
```

**Infrastructure**:
```bash
# Option A: Kind cluster (local)
kind create cluster --name gateway-integration

# Option B: OCP cluster (already available)
# Use existing kubernaut-system namespace
```

#### **APDC Do** (1-1.5 hours)

**DO-RED** (30 min):
```bash
# Create E2E test file
touch test/integration/gateway/webhook_e2e_test.go

# Write 4 failing tests (listed above)
```

**DO-GREEN** (45 min):
```bash
# Setup test infrastructure:
# - Port-forward to Redis (kubernaut-system)
# - Create test namespace
# - Deploy Gateway server (or run locally)

# Run tests, verify they pass
go test -v ./test/integration/gateway/webhook_e2e_test.go
```

**DO-REFACTOR** (15 min):
```go
// Extract common test setup (Redis, K8s client)
// Create test helper functions
// Improve test readability
```

#### **APDC Check** (30 min)

**Validation**:
- ✅ All 4 E2E tests passing
- ✅ CRDs created with correct structure
- ✅ Deduplication working end-to-end
- ✅ Storm detection working end-to-end
- ✅ Metrics accurate

**Confidence Assessment**: Target 90%

---

### **Phase 3: Production Readiness** (2 hours)

#### **Objective**
Establish performance baseline and create operational runbooks.

#### **Performance Baseline** (1 hour)

**Metrics to Establish**:
```
Latency (p50, p95, p99):
├── Webhook processing time
├── Redis operations
├── K8s API calls
└── End-to-end request time

Throughput:
├── Requests per second (sustained)
├── Concurrent webhook handling
└── Storm handling capacity

Resource Usage:
├── Memory (baseline, peak)
├── CPU (baseline, peak)
└── Redis connections
```

**Test Approach**:
```bash
# Simple load test with curl
for i in {1..100}; do
  curl -X POST http://localhost:8080/webhook/prometheus \
    -d @test/fixtures/prometheus_alert.json &
done
wait

# Analyze Prometheus metrics
# Check Gateway logs for latency
# Monitor resource usage
```

**Success Criteria**:
- ✅ p95 latency < 500ms
- ✅ Sustained 50 req/s
- ✅ Memory < 100MB baseline
- ✅ CPU < 50% baseline

#### **Operational Runbooks** (1 hour)

**Create Documentation**:
```
docs/services/stateless/gateway-service/operations/
├── 01-deployment.md         # How to deploy Gateway
├── 02-troubleshooting.md    # Common issues + fixes
├── 03-rollback.md           # How to rollback
├── 04-performance-tuning.md # Optimization guide
└── 05-on-call-escalation.md # When to escalate
```

**Key Runbook Sections**:
1. **Deployment**: Makefile targets, environment variables, health checks
2. **Troubleshooting**: Redis connection failures, K8s API errors, high latency
3. **Rollback**: Safe rollback procedure, data migration considerations
4. **Performance**: Redis tuning, K8s resource limits, scaling guidelines
5. **Escalation**: SLA definitions, escalation paths, on-call contacts

---

## Day 7 Deliverables

### **Code Deliverables**

```
test/integration/gateway/
├── k8s_api_failure_test.go      # K8s API failure scenarios (NEW)
├── webhook_e2e_test.go          # End-to-end webhook flow (NEW)
├── redis_resilience_test.go     # Existing Redis tests
└── deduplication_ttl_test.go    # Existing TTL tests

pkg/gateway/
└── (No new implementation files - validation only)
```

### **Documentation Deliverables**

```
docs/services/stateless/gateway-service/
├── DAY7_COMPLETE.md                    # Day 7 summary (NEW)
├── INTEGRATION_TESTS_COMPLETE.md       # Integration test summary (NEW)
├── PERFORMANCE_BASELINE.md             # Performance metrics (NEW)
└── operations/                         # Operational runbooks (NEW)
    ├── 01-deployment.md
    ├── 02-troubleshooting.md
    ├── 03-rollback.md
    ├── 04-performance-tuning.md
    └── 05-on-call-escalation.md
```

---

## Success Criteria

### **Integration Tests**

- ✅ K8s API failure tests: 4-5 tests passing
- ✅ E2E webhook flow tests: 4 tests passing
- ✅ Total integration tests: 14-15 tests (6 existing + 8-9 new)

### **Performance**

- ✅ p95 latency < 500ms
- ✅ Sustained throughput > 50 req/s
- ✅ Memory usage < 100MB baseline
- ✅ CPU usage < 50% baseline

### **Documentation**

- ✅ 5 operational runbooks created
- ✅ Performance baseline documented
- ✅ Integration test README updated

### **Production Readiness**

- ✅ Integration tests passing
- ✅ Performance acceptable
- ✅ Operational runbooks complete
- ✅ Deployment automation working

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| **K8s cluster unavailable** | Medium | Use error-injectable mock for unit-style integration tests |
| **Redis unavailable** | Low | Already have OCP Redis, fallback to local Docker |
| **Performance below target** | Medium | Optimize hot paths, add caching, tune Redis |
| **Integration test flakiness** | Medium | Add retries, improve test isolation, use test fixtures |

---

## Confidence Assessment

**Day 7 Completion Confidence**: 90% ✅ **Very High**

**Justification**:
1. ✅ **Core functionality complete**: Days 1-6 provide solid foundation
2. ✅ **Infrastructure available**: OCP Redis, Kind/OCP clusters accessible
3. ✅ **Clear test strategy**: Well-defined integration test scenarios
4. ✅ **Operational focus**: Runbooks provide production confidence
5. ⚠️ **Performance unknown**: Baseline needs establishment

**Risks**:
- ⚠️ Performance may require optimization (mitigated by baseline + tuning)
- ⚠️ Integration tests may be flaky (mitigated by retries + isolation)

---

## Next Steps After Day 7

### **Option 1: Advanced Features (Days 8-9)**

**Scope**:
- Rego policy integration (BR-GATEWAY-014)
- Namespace label reading (BR-GATEWAY-011-012)
- Remediation path decision matrix (BR-GATEWAY-022)
- Storm aggregation implementation

**Estimated Effort**: 2-3 days
**Business Value**: Enhanced customization

### **Option 2: Production Deployment**

**Scope**:
- Deploy to staging environment
- Monitor real-world performance
- Iterate on operational runbooks
- Establish SLAs

**Estimated Effort**: 1-2 days
**Business Value**: Production readiness

### **Option 3: E2E Testing**

**Scope**:
- Complete alert-to-remediation workflow
- Multi-cluster scenarios
- Load testing (1000+ req/s)
- Chaos testing

**Estimated Effort**: 2-3 days
**Business Value**: Production confidence

---

## Summary

**Day 7 Focus**: Integration Testing + Production Readiness

**Key Deliverables**:
1. ✅ K8s API failure integration tests (4-5 tests)
2. ✅ End-to-end webhook flow tests (4 tests)
3. ✅ Performance baseline established
4. ✅ Operational runbooks created (5 documents)

**Success Metrics**:
- Integration tests: 14-15 total (6 existing + 8-9 new)
- Performance: p95 < 500ms, 50+ req/s
- Documentation: 5 operational runbooks

**Production Readiness**: Target 85% (up from 70%)

---

**Status**: 🎯 **READY TO START** - All prerequisites met, clear implementation path

**Recommendation**: Proceed with Day 7 implementation, focusing on integration testing first, then production readiness.

