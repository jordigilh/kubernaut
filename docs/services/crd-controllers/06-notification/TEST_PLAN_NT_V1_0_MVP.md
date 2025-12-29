# Notification Service (NT) V1.0 MVP - Test Plan

**Version**: 1.3.0
**Last Updated**: December 22, 2025
**Status**: READY FOR EXECUTION (with adjustments per triage)

**Business Requirements**: BR-NOT-052 (Retry), BR-NOT-053 (Delivery), BR-NOT-055 (Graceful Degradation), BR-NOT-056 (Priority)
**Design Decisions**: DD-METRICS-001 (Metrics Wiring), DD-005 V3.0 (Observability)
**Authority**: DD-TEST-001 - Port Allocation Strategy

**Cross-References**:
- [Test Plan Best Practices](../../../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md) - When/why to use each section
- [Test Plan Template](../../../holmesgpt-api/tests/e2e/TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md) - Reusable template for other teams

---

## 📋 **Changelog**

### Version 1.3.0 (2025-12-22)
- **TRIAGE**: Comprehensive pre-execution infrastructure triage completed
- **REVISED**: Timeline from 2.5 days → 2 days (Test 05 exists, needs fixes)
- **ADDED**: Phase 0 - Infrastructure validation (0.5 day)
- **UPDATED**: E2E-1 status from "To Implement" to "Exists (needs CRD fixes)"
- **REFERENCE**: See [NT_TEST_PLAN_EXECUTION_TRIAGE.md](./NT_TEST_PLAN_EXECUTION_TRIAGE.md) for complete findings

### Version 1.2.2 (2025-12-22)
- **ADDED**: Code Coverage column to Test Outcomes by Tier table
- **ALIGNED**: Fully aligned with enhanced test plan template v1.3.0

### Version 1.2.1 (2025-12-22)
- **FIXED**: Tier headers now show BR coverage + code coverage (not just percentages)
- **ADDED**: Infrastructure setup section for E2E tests
- **ALIGNED**: Headers now match summary table notation

### Version 1.2.0 (2025-12-22)
- **ADDED**: Code coverage targets (70%/50%/50%) based on empirical DS/SP data
- **CLARIFIED**: BR coverage (overlapping) vs code coverage (cumulative) distinction
- **UPDATED**: E2E code coverage target from 10-20% to 50% based on DS/SP validation
- **ADDED**: Example showing retry logic tested in all 3 tiers

### Version 1.1.0 (2025-12-22)
- **CORRECTED**: Changed "Test Pyramid" to "Defense-in-Depth Testing" per `.cursor/rules/03-testing-strategy.mdc`
- **CLARIFIED**: BR coverage targets are overlapping (70% + 50% + 10%), not mutually exclusive
- **ADDED**: Defense-in-depth principle explanation (multiple tiers test same BRs)
- **FIXED**: Test strategy terminology throughout document

### Version 1.0.0 (2025-12-22)
- Initial test plan for NT V1.0 MVP
- 3 new E2E tests for retry, fanout, and priority routing
- Based on Kubernaut test plan template
- Focus on MVP with existing channels (console + file)

---

## 🎯 **Testing Scope**

```
┌─────────────────────────────────────────────────────────────────────┐
│ Notification Controller (Reconciler)                                │
│   ├── Retry logic (exponential backoff: 30s → 480s)                │
│   ├── Circuit breaker (closed → open → half-open)                  │
│   ├── Multi-channel fanout (console + file)                        │
│   ├── Priority-based routing (high → console+file, low → console)  │
│   └── Metrics recording (DD-METRICS-001 dependency injection)      │
└────────────────────────┬────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│ Delivery Services                                                    │
│   ├── Console: Structured logging (always succeeds)                 │
│   ├── File: JSON output to HostPath (E2E validation)               │
│   └── Slack: Basic webhook (existing, not tested in MVP)           │
└─────────────────────────────────────────────────────────────────────┘
```

**Scope**: Validate existing NT controller implementation for V1.0 MVP using file channel for E2E validation.

**Out of Scope** (Post-MVP):
- ❌ Real Slack webhook E2E
- ❌ Email channel
- ❌ PagerDuty channel
- ❌ Slack reliability improvements (rate limiting, 429 retry)

---

## 📊 **Defense-in-Depth Testing Summary**

**Strategy**: Overlapping BR coverage + cumulative code coverage approaching 100% (per [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc))

### BR Coverage (Overlapping) + Code Coverage (Cumulative)

| Tier | Tests | Infrastructure | BR Coverage | Code Coverage | Status |
|------|-------|----------------|-------------|---------------|--------|
| **Unit** | 117 | None (mocked external deps) | 70%+ of ALL BRs | 70%+ | ✅ 100% passing |
| **Integration** | 9 | Real K8s (envtest) | >50% of ALL BRs | 50% | ✅ 100% passing |
| **E2E** | 5 + 3 NEW | Real K8s (Kind) + File delivery | <10% BR coverage | 50% | ⏸️ 5 existing, 3 new for MVP |

**Defense-in-Depth Principle**:
- **BR Coverage**: Overlapping (same BRs tested at multiple tiers)
- **Code Coverage**: Cumulative (~100% combined across all tiers)
- **Key Insight**: With 70%/50%/50% targets, **50%+ of codebase is tested in ALL 3 tiers**

**Example - Retry Logic (BR-NOT-052)**:
- **Unit (70%)**: Algorithm correctness (30s → 480s exponential backoff) - tests `pkg/notification/retry/policy.go`
- **Integration (50%)**: Real K8s reconciliation loop - tests same code with envtest
- **E2E (50%)**: Deployed controller in Kind - tests same code in production-like environment

If the exponential backoff calculation has a bug, it must slip through **ALL 3 defense layers** to reach production!

**Current Status**: 5/5 E2E tests passing (audit, file delivery, metrics)
**MVP Target**: 8/8 E2E tests passing (+ retry, fanout, priority routing)

**Total Tests**: 131 (117 unit + 9 integration + 5 existing E2E + 3 new MVP E2E)

---

# 🧪 **TIER 1: UNIT TESTS** (70%+ BR Coverage | 70%+ Code Coverage) - ✅ COMPLETE

**Location**: `test/unit/notification/`
**Infrastructure**: None (mocked external dependencies)
**Execution**: `make test-unit-notification`
**Status**: ✅ 117/117 passing (100% pass rate)

### Current Unit Test Coverage

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| Controller reconciliation | 35 | ✅ Passing | BR-NOT-052, 053, 056 |
| Delivery services | 25 | ✅ Passing | BR-NOT-053 |
| Retry logic | 18 | ✅ Passing | BR-NOT-052 |
| Circuit breaker | 15 | ✅ Passing | BR-NOT-055 |
| Status management | 12 | ✅ Passing | BR-NOT-051 |
| Metrics recording | 8 | ✅ Passing | DD-METRICS-001 |
| Data sanitization | 4 | ✅ Passing | BR-NOT-054 |

**MVP Assessment**: ✅ **NO NEW UNIT TESTS NEEDED** - Existing coverage is comprehensive

---

# 🔗 **TIER 2: INTEGRATION TESTS** (>50% BR Coverage | 50% Code Coverage) - ✅ COMPLETE

**Location**: `test/integration/notification/`
**Infrastructure**: Real Kubernetes (envtest) + Mock DataStorage
**Execution**: `make test-integration-notification`
**Status**: ✅ 9/9 passing (100% pass rate)

### Current Integration Test Coverage

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| Controller audit emission | 3 | ✅ Passing | BR-NOT-062, 063, 064 |
| Acknowledged field validation | 2 | ✅ Passing | BR-NOT-069 |
| Status updates | 2 | ✅ Passing | BR-NOT-051 |
| Delivery orchestration | 2 | ✅ Passing | BR-NOT-053 |

**MVP Assessment**: ✅ **NO NEW INTEGRATION TESTS NEEDED** - Existing coverage is sufficient

---

# 🚀 **TIER 3: E2E TESTS** (<10% BR Coverage | 50% Code Coverage) - ⏸️ 3 NEW TESTS NEEDED

**Location**: `test/e2e/notification/`
**Infrastructure**: Real Kubernetes (Kind) + File delivery + Real DataStorage
**Execution**: `make test-e2e-notification`

---

## 🏗️ **E2E Infrastructure Setup**

### Prerequisites

- Kind cluster running with kubernaut controllers
- Notification controller deployed
- File delivery channel configured with HostPath volume

### Setup Commands

```bash
# 1. Create Kind cluster (if not already running)
make kind-up

# 2. Deploy notification controller
make deploy-notification

# 3. Verify controller is running
kubectl get pods -n kubernaut-system | grep notification-controller

# Expected output:
# notification-controller-manager-xxxx-xxxx   2/2   Running   0   30s

# 4. Verify CRD is registered
kubectl get crd notificationrequests.kubernaut.io

# 5. Create test namespace
kubectl create namespace notification-e2e-test

# 6. Run E2E tests
make test-e2e-notification
```

### Infrastructure Validation

```bash
# Verify all E2E prerequisites are met
make validate-e2e-notification-infrastructure

# Expected checks:
# ✅ Kind cluster accessible
# ✅ Notification controller deployed
# ✅ NotificationRequest CRD registered
# ✅ File delivery HostPath writable
# ✅ DataStorage service accessible
```

---

## ✅ Existing E2E Tests (5 tests - ALL PASSING)

| Test File | Tests | Status | Coverage |
|---|---|---|---|
| `01_notification_lifecycle_audit_test.go` | 1 | ✅ Passing | BR-NOT-062, 063, 064 |
| `02_audit_correlation_test.go` | 1 | ✅ Passing | BR-NOT-064 |
| `03_file_delivery_validation_test.go` | 1 | ✅ Passing | BR-NOT-053, 054, 056 |
| `04_failed_delivery_audit_test.go` | 1 | ✅ Passing | BR-NOT-062, 063 |
| `04_metrics_validation_test.go` | 1 | ✅ Passing | BR-NOT-060 |

**Total Existing**: 5 E2E tests, ~1,290 LOC

---

## 🆕 MVP E2E Tests (3 NEW TESTS - TO BE IMPLEMENTED)

### **E2E Test 1: Retry and Exponential Backoff E2E** (NEW - P0 CRITICAL)

**File**: `test/e2e/notification/05_retry_exponential_backoff_test.go`
**Priority**: P0 (CRITICAL for MVP)
**Estimated LOC**: ~250 lines
**Estimated Time**: 1 day
**BR Coverage**: BR-NOT-052 (Automatic Retry)

#### Test Scenario

**Business Outcome**: Validate that failed deliveries are automatically retried with exponential backoff, preventing notification loss.

**Test Steps**:
1. Create NotificationRequest with file channel pointing to read-only directory (causes delivery failure)
2. Verify initial delivery attempt fails
3. Verify CRD status shows retry attempt scheduled with 30s delay
4. Verify 2nd attempt after 30s
5. Verify 3rd attempt after 60s (cumulative: 90s)
6. Verify max 5 attempts before marking as failed
7. Verify CRD phase transitions: Pending → Sending → Failed (after 5 attempts)

**Success Criteria**:
- ✅ Retry attempts follow exponential backoff (30s, 60s, 120s, 240s, 480s)
- ✅ CRD status shows all retry attempts with timestamps
- ✅ Phase transitions to Failed after max attempts
- ✅ Console channel continues (not blocked by file channel failures)

**Infrastructure**:
- Kind cluster (existing)
- File channel with read-only directory (simulated failure)
- No external dependencies

---

### **E2E Test 2: Multi-Channel Fanout E2E** (NEW - P0 CRITICAL)

**File**: `test/e2e/notification/06_multi_channel_fanout_test.go`
**Priority**: P0 (CRITICAL for MVP)
**Estimated LOC**: ~200 lines
**Estimated Time**: 0.5 day
**BR Coverage**: BR-NOT-053 (At-Least-Once Delivery), BR-NOT-068 (Multi-Channel Fanout)

#### Test Scenario

**Business Outcome**: Validate that a single notification can be delivered to multiple channels simultaneously.

**Test Steps**:
1. Create NotificationRequest with channels: [console, file]
2. Verify both deliveries attempted
3. Verify CRD status shows 2 delivery attempts
4. Verify file channel wrote JSON file
5. Verify console channel logged message (check controller logs)
6. Verify phase: Sent (both successful)

**Partial Failure Scenario**:
1. Create NotificationRequest with console + file (read-only directory)
2. Verify console succeeds, file fails
3. Verify CRD status shows: 1 success, 1 failure
4. Verify phase: PartiallySent (not Sent or Failed)

**Success Criteria**:
- ✅ Multi-channel fanout delivers to all channels
- ✅ CRD status shows all delivery attempts (success + failure)
- ✅ Partial failures set phase to PartiallySent
- ✅ Console delivery not blocked by file failure

**Infrastructure**:
- Kind cluster (existing)
- File channel with HostPath (writable)
- File channel with read-only directory (failure scenario)
- No external dependencies

---

### **E2E Test 3: Priority-Based Routing E2E** (NEW - P1 HIGH)

**File**: `test/e2e/notification/07_priority_routing_test.go`
**Priority**: P1 (HIGH for MVP)
**Estimated LOC**: ~180 lines
**Estimated Time**: 0.5 day
**BR Coverage**: BR-NOT-056 (Priority Handling), BR-NOT-065 (Channel Routing)

#### Test Scenario

**Business Outcome**: Validate that notification priority determines which channels receive the notification.

**Test Steps**:

**High-Priority Notification**:
1. Create NotificationRequest with priority: high, channels: [console, file]
2. Verify both channels receive notification
3. Verify CRD status shows 2 delivery attempts
4. Verify file content includes priority: high

**Low-Priority Notification**:
1. Create NotificationRequest with priority: low, channels: [console]
2. Verify only console channel receives notification
3. Verify CRD status shows 1 delivery attempt
4. Verify no file created (file channel not used for low priority)

**Priority Preservation**:
1. Create NotificationRequest with priority: critical
2. Verify priority field preserved in file output
3. Verify priority field preserved in CRD status

**Success Criteria**:
- ✅ High-priority → console + file
- ✅ Low-priority → console only
- ✅ Priority field preserved in delivery
- ✅ CRD status reflects correct channel selection

**Infrastructure**:
- Kind cluster (existing)
- File channel with HostPath (writable)
- No external dependencies

---

## 📊 MVP E2E Test Summary

| Test | File | Priority | Time | LOC | BR Coverage | Status |
|---|---|---|---|---|---|---|
| **E2E-1** | `05_retry_exponential_backoff_test.go` | P0 | 1-2 hours (fix) | 301 | BR-NOT-052 | ⏸️ Exists (CRD fixes needed) |
| **E2E-2** | `06_multi_channel_fanout_test.go` | P0 | 0.5 day | 200 | BR-NOT-053, 068 | ⏸️ To Implement |
| **E2E-3** | `07_priority_routing_test.go` | P1 | 0.5 day | 180 | BR-NOT-056, 065 | ⏸️ To Implement |
| **TOTAL** | Fix 1 + Create 2 | - | **1.5 days** | **681 LOC** | 5 BRs | - |

---

# 📁 **File Structure**

```
test/e2e/notification/
├── notification_e2e_suite_test.go                     # Suite setup (existing)
│
├── ✅ EXISTING E2E TESTS (5 tests - ALL PASSING)
├── 01_notification_lifecycle_audit_test.go            # Audit lifecycle
├── 02_audit_correlation_test.go                       # Audit correlation
├── 03_file_delivery_validation_test.go                # File delivery
├── 04_failed_delivery_audit_test.go                   # Failed delivery audit
├── 04_metrics_validation_test.go                      # Metrics exposure
│
├── ⏸️ NEW MVP E2E TESTS (3 tests - TO IMPLEMENT)
├── 05_retry_exponential_backoff_test.go               # NEW: Retry + backoff
├── 06_multi_channel_fanout_test.go                    # NEW: Multi-channel
└── 07_priority_routing_test.go                        # NEW: Priority routing
```

---

# 🎯 **Test Outcomes by Tier**

| Tier | What It Proves | Failure Means | Code Coverage |
|------|----------------|---------------|---------------|
| **Unit** | Controller logic is correct | Bug in NT controller code | 70%+ |
| **Integration** | CRD operations and audit work | Kubernetes integration issue | 50% |
| **E2E** | Complete notification lifecycle works in real cluster | System doesn't serve business need | 50% |

---

# 📋 **Implementation Checklist**

## ✅ Pre-MVP Status (COMPLETE)
- [x] Unit tests: 117/117 passing
- [x] Integration tests: 9/9 passing
- [x] Existing E2E tests: 5/5 passing
- [x] Documentation: Up-to-date (to be refreshed)

## 🆕 MVP E2E Tests (TO IMPLEMENT)

### Week 1: Core Retry and Fanout (P0 - CRITICAL)
- [ ] E2E-1: Retry and Exponential Backoff (1 day)
  - [ ] Implement test file structure
  - [ ] Create read-only directory failure scenario
  - [ ] Validate 5 retry attempts with exponential backoff
  - [ ] Verify CRD status shows retry attempts
  - [ ] Verify phase transitions (Pending → Sending → Failed)

- [ ] E2E-2: Multi-Channel Fanout (0.5 day)
  - [ ] Implement console + file fanout scenario
  - [ ] Validate both channels receive notification
  - [ ] Implement partial failure scenario
  - [ ] Verify PartiallySent phase
  - [ ] Validate CRD status shows all attempts

### Week 1-2: Priority Routing (P1 - HIGH)
- [ ] E2E-3: Priority-Based Routing (0.5 day)
  - [ ] Implement high-priority scenario (console + file)
  - [ ] Implement low-priority scenario (console only)
  - [ ] Validate priority field preservation
  - [ ] Verify correct channel selection per priority

### Week 2: Validation and Documentation
- [ ] Run all E2E tests in CI/CD pipeline
- [ ] Verify 100% E2E pass rate
- [ ] Update documentation with new E2E tests
- [ ] Create test execution summary

---

# 🎯 **Success Criteria**

## MVP V1.0 E2E Test Success Criteria

### **Test Coverage**:
- [ ] ✅ 8/8 E2E tests passing (5 existing + 3 new)
- [ ] ✅ BR-NOT-052, 053, 055, 056, 065, 068 validated in E2E
- [ ] ✅ Retry logic validated with real Kubernetes
- [ ] ✅ Multi-channel fanout validated
- [ ] ✅ Priority routing validated

### **Infrastructure**:
- [ ] ✅ All tests use existing channels (console + file)
- [ ] ✅ No external dependencies (Slack, Email, PagerDuty)
- [ ] ✅ Tests run in Kind cluster (existing infrastructure)

### **Quality**:
- [ ] ✅ Zero flaky tests (<5% failure rate)
- [ ] ✅ Tests run in CI/CD pipeline
- [ ] ✅ Tests complete in <20 minutes (total E2E suite)

---

# 📊 **Execution Commands**

```bash
# ========================================
# TIER 1: UNIT TESTS (EXISTING - ALL PASSING)
# ========================================

# Run all unit tests (fast, no infrastructure)
make test-unit-notification
go test ./test/unit/notification/... -v

# Run with coverage
go test ./test/unit/notification/... -v -coverprofile=coverage.out
go tool cover -html=coverage.out

# ========================================
# TIER 2: INTEGRATION TESTS (EXISTING - ALL PASSING)
# ========================================

# Run all integration tests (requires envtest)
make test-integration-notification
go test ./test/integration/notification/... -v

# ========================================
# TIER 3: E2E TESTS
# ========================================

# Run EXISTING E2E tests (5 tests - ALL PASSING)
make test-e2e-notification
go test ./test/e2e/notification/... -v

# Run NEW MVP E2E tests (3 tests - TO IMPLEMENT)
go test ./test/e2e/notification/05_retry_exponential_backoff_test.go -v
go test ./test/e2e/notification/06_multi_channel_fanout_test.go -v
go test ./test/e2e/notification/07_priority_routing_test.go -v

# Run ALL E2E tests (8 tests total)
go test ./test/e2e/notification/... -v

# ========================================
# FULL TEST SUITE
# ========================================

# Run all tiers (unit + integration + E2E)
make test-notification-all

# Run with specific test
go test ./test/e2e/notification/... -v -run TestRetryExponentialBackoff
```

---

# ⏱️ **Execution Timeline** (Revised per Triage)

## **Phase 0: Infrastructure Validation** (0.5 day)

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 0.5** | Create `validate-e2e-notification-infrastructure` target | 30 min | NT Team | Make target |
| **Day 0.5** | Fix Test 05 CRD compilation errors | 1-2 hours | NT Team | Test 05 compiles & passes |
| **Day 0.5** | Run validation + verify existing 5 E2E tests pass | 30 min | NT Team | Infrastructure validated |

## **Week 1: MVP E2E Implementation** (1 day - Simple-to-Complex)

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 1 AM** | E2E-2: Multi-Channel Fanout | 0.5 day | NT Team | `06_multi_channel_fanout_test.go` passing |
| **Day 1 PM** | E2E-3: Priority-Based Routing | 0.5 day | NT Team | `07_priority_routing_test.go` passing |

## **Week 2: Validation & Documentation** (0.5 day)

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 2 AM** | Run full E2E suite (8 tests) in CI/CD | 0.25 day | NT Team | All 8 E2E tests passing |
| **Day 2 PM** | Update documentation (test plan, status report) | 0.25 day | NT Team | Documentation updated |

**Total Time**: **2 days** (0.5 validation + 1 implementation + 0.5 docs)
**Original Estimate**: 2.5 days
**Savings**: 0.5 day (Test 05 already exists, just needs fixes)

---

# 🎉 **Expected Outcomes**

## **Pre-MVP Status**:
- ✅ 131 tests passing (117 unit + 9 integration + 5 E2E)
- ✅ 100% pass rate across all tiers
- ✅ BR-NOT-062, 063, 064, 060 validated in E2E

## **Post-MVP Status** (Target):
- ✅ 134 tests passing (117 unit + 9 integration + 8 E2E)
- ✅ 100% pass rate across all tiers
- ✅ BR-NOT-052, 053, 055, 056, 065, 068 validated in E2E
- ✅ Retry logic validated in real cluster
- ✅ Multi-channel fanout validated
- ✅ Priority routing validated

## **Confidence Improvement**:
- **Before MVP**: 95% confidence for production (5 E2E tests)
- **After MVP**: 99% confidence for production (8 E2E tests)

---

# 📚 **References**

### Authoritative Documents
- `docs/services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md` - 18 BRs with acceptance criteria
- `docs/services/crd-controllers/06-notification/testing-strategy.md` - Defense-in-depth testing strategy
- `.cursor/rules/03-testing-strategy.mdc` - Defense-in-depth approach (overlapping BR coverage)

### Current E2E Tests
- `test/e2e/notification/01_notification_lifecycle_audit_test.go` - Audit lifecycle
- `test/e2e/notification/02_audit_correlation_test.go` - Audit correlation
- `test/e2e/notification/03_file_delivery_validation_test.go` - File delivery
- `test/e2e/notification/04_failed_delivery_audit_test.go` - Failed delivery audit
- `test/e2e/notification/04_metrics_validation_test.go` - Metrics exposure

### Planning Documents
- `docs/services/crd-controllers/06-notification/NT_V1_0_ROADMAP.md` - Master roadmap
- `docs/services/crd-controllers/06-notification/NT_E2E_TEST_COVERAGE_PLAN_V1_0.md` - Full E2E plan (post-MVP)

---

**Status**: 📋 **READY FOR EXECUTION**
**Owner**: NT Team
**Target Completion**: 2.5 days
**Next Milestone**: V1.0 MVP Production Deployment

