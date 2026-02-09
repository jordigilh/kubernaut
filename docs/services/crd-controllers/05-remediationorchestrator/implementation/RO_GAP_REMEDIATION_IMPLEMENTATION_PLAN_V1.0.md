# DD-RO-GAP-001: Remediation Orchestrator Gap Remediation - Implementation Plan

**Version**: 1.0
**Filename**: `RO_GAP_REMEDIATION_IMPLEMENTATION_PLAN_V1.0.md`
**Status**: ✅ APPROVED (December 8, 2025)
**Design Decision**: DD-RO-GAP-001 (Gap Remediation)
**Service**: Remediation Orchestrator
**Confidence**: 75% (Evidence-Based - pending testing infrastructure)
**Estimated Effort**: 8 days (APDC cycle: 5 days implementation + 2 days testing + 1 day documentation)

⚠️ **CRITICAL**: Filename version MUST match document version at all times.

---

## 🚨 **CRITICAL: Read This First**

**Before starting implementation, you MUST review these 5 critical pitfalls** (from FEATURE_EXTENSION_PLAN_TEMPLATE.md):

1. **Insufficient TDD Discipline** → Write ONE test at a time (not batched)
2. **Missing Integration Tests** → Integration tests BEFORE E2E tests
3. **Critical Infrastructure Without Unit Tests** → ≥70% coverage for critical components
4. **Late E2E Discovery** → Follow test pyramid (Unit → Integration → E2E)
5. **No Test Coverage Gates** → Automated CI/CD coverage gates

⚠️ **These pitfalls were discovered in Notification service audit (December 2025).**

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | 2025-12-08 | Initial gap remediation plan from comprehensive RO audit | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-ORCH-039** | Testing Tier Compliance | All RO tests follow TESTING_GUIDELINES.md (Unit→Integration→E2E pyramid) |
| **BR-ORCH-040** | Prometheus Metrics Correctness | All metrics emit without runtime panics, labels match definitions |
| **BR-ORCH-041** | Audit Trail Integration | RO emits audit events per DD-AUDIT-003 (BLOCKED on Data Storage) |

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

- **Unit Test Coverage**: ≥70% - *Justification: TESTING_GUIDELINES.md mandate for critical components*
- **Integration Test Coverage**: ≥50% - *Justification: TESTING_GUIDELINES.md mandate*
- **Metrics Runtime Stability**: 0 panics - *Justification: Label mismatch causes immediate crash*
- **Audit Event Persistence**: 100% (when unblocked) - *Justification: BR-ORCH-041 compliance*

---

## 📊 **Gap Analysis Summary**

### **Gaps Identified (December 8, 2025 Audit)**

| Gap ID | Category | Severity | Description | Status |
|--------|----------|----------|-------------|--------|
| **GAP-RO-001** | Testing | 🔴 CRITICAL | Integration tests use mocks (httptest.NewServer) | ✅ N/A (uses envtest) |
| **GAP-RO-002** | Testing | 🔴 CRITICAL | E2E tests are empty (suite only, no actual tests) | ✅ Complete |
| **GAP-RO-003** | Runtime | 🔴 CRITICAL | `ReconcileTotal` metric: 3 labels defined, 2 provided | ✅ Complete |
| **GAP-RO-004** | Runtime | 🔴 CRITICAL | `PhaseTransitionsTotal` metric: labels mismatch | ✅ Complete |
| **GAP-RO-005** | Feature | 🔴 CRITICAL | No audit client integration (DD-AUDIT-003) | ✅ Complete |

### **Dependencies**

| Gap | Dependency | Status |
|-----|------------|--------|
| **GAP-RO-005** | Data Storage batch endpoint (POST /api/v1/audit/events/batch) | ✅ Resolved (using existing single-event workaround) |
| **GAP-RO-001/002** | Cross-service testing infrastructure (podman-compose, Kind) | ⏳ Pending Initiative |

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 2 hours | Day 0 (pre-work) | Comprehensive context understanding | This document, gap mapping |
| **PLAN** | 2 hours | Day 0 (pre-work) | Detailed implementation strategy | Day-by-day breakdown, BR mapping |
| **DO (Phase 1)** | 2 days | Days 1-2 | Testing tier compliance | Unit tests, integration test infra |
| **DO (Phase 2)** | 2 days | Days 3-4 | Metrics bugs | Fix label mismatches, add metric tests |
| **DO (Phase 3)** | 1 day | Day 5 | Audit integration (if unblocked) | Audit client wiring |
| **CHECK** | 2 days | Days 6-7 | Comprehensive validation | Full test suite, coverage report |
| **PRODUCTION** | 1 day | Day 8 | Documentation & handoff | Updated BR_MAPPING.md, confidence report |

### **8-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 4h | ✅ Analysis complete, Plan approved (this document) |
| **Day 1** | DO-RED | Testing Infrastructure | 8h | podman-compose setup, failing integration tests |
| **Day 2** | DO-GREEN | Integration Tests | 8h | Real Data Storage/Redis in integration tests |
| **Day 3** | DO-RED | Metrics Bugs | 8h | Failing unit tests for metric label mismatches |
| **Day 4** | DO-GREEN | Metrics Fixes | 8h | All metrics emit correctly, unit tests pass |
| **Day 5** | DO-GREEN | Audit Integration (if unblocked) | 8h | Audit client wired, events emitted |
| **Day 6** | CHECK | Unit + Integration Tests | 8h | ≥70% unit, ≥50% integration coverage |
| **Day 7** | CHECK | E2E Tests (Kind) | 8h | Real Kind cluster, full lifecycle tests |
| **Day 8** | PRODUCTION | Documentation + Handoff | 8h | BR_MAPPING.md, confidence report |

### **Priority Order (Per User Direction)**

```
Day 1-2: Testing Tier Violations (Foundation) → D
Day 3-4: Metrics Bugs (Runtime Panics) → A
Day 5: Audit Integration (If Unblocked) → BLOCKED on Data Storage
```

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 4 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Gap analysis document: 5 gaps identified with severity and status
- ✅ Implementation plan (this document v1.0): 8-day timeline, test examples
- ✅ Risk assessment: 2 blocked dependencies identified
- ✅ Existing code review: reconciler.go, prometheus.go analyzed
- ✅ BR coverage matrix: 3 new BRs mapped (BR-ORCH-039, 040, 041)

---

### **Day 1: Testing Infrastructure Setup (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 8 hours
**TDD Focus**: Write failing tests first, establish infrastructure
**Gap Target**: GAP-RO-001, GAP-RO-002

**⚠️ CRITICAL**: We are fixing TESTING_GUIDELINES.md violations!

**Existing Code to Fix**:
- ❌ `test/integration/remediationorchestrator/suite_test.go` - Uses mocks
- ❌ `test/e2e/remediationorchestrator/suite_test.go` - Empty (no actual tests)

**Morning (4 hours): Infrastructure Setup**

1. **Create podman-compose.test.yml for RO** (if not exists)
   ```yaml
   # test/integration/remediationorchestrator/podman-compose.test.yml
   services:
     postgres:
       image: postgres:15
       environment:
         POSTGRES_DB: kubernaut_test
         POSTGRES_USER: test_user
         POSTGRES_PASSWORD: test_pass
     redis:
       image: redis:7
     datastorage:
       build: ../../../cmd/datastorage
       depends_on: [postgres, redis]
   ```

2. **Create integration test helpers** `test/integration/remediationorchestrator/helpers_test.go`
   - Real Data Storage client (not mock)
   - Real K8s client via envtest
   - Database verification helpers

**Afternoon (4 hours): Write Failing Integration Tests**

3. **Update** `test/integration/remediationorchestrator/lifecycle_test.go`
   - Replace `httptest.NewServer()` with real service connection
   - Add database verification (`SELECT FROM audit_events`)
   - Tests should FAIL initially (infrastructure not connected)

**EOD Deliverables**:
- ✅ podman-compose.test.yml created
- ✅ Integration test helpers created
- ❌ Integration tests FAILING (RED phase - infrastructure not running)
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify tests fail (infrastructure not connected)
go test ./test/integration/remediationorchestrator/... -v 2>&1 | grep "FAIL"

# Expected: Tests fail with "connection refused" or similar
```

---

### **Day 2: Integration Tests with Real Infrastructure (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Make failing tests pass with real infrastructure
**Gap Target**: GAP-RO-001

**Morning (4 hours): Connect Real Infrastructure**

1. **Start test infrastructure**
   ```bash
   podman-compose -f test/integration/remediationorchestrator/podman-compose.test.yml up -d
   ```

2. **Update integration tests to connect to real services**
   - Data Storage: `http://localhost:8082`
   - PostgreSQL: `localhost:5432`
   - Redis: `localhost:6379`

3. **Run tests → Verify they PASS**

**Afternoon (4 hours): Add Database Verification**

4. **Add database verification to integration tests**
   ```go
   // Verify audit events persisted
   var count int
   err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
   Expect(err).ToNot(HaveOccurred())
   Expect(count).To(BeNumerically(">", 0), "Audit events should be persisted")
   ```

**EOD Deliverables**:
- ✅ Integration tests connect to real infrastructure
- ✅ All integration tests passing (GREEN phase)
- ✅ Database verification added
- ✅ Day 2 EOD report

**Validation Commands**:
```bash
# Start infrastructure
podman-compose -f test/integration/remediationorchestrator/podman-compose.test.yml up -d

# Run integration tests
go test ./test/integration/remediationorchestrator/... -v

# Expected: All tests PASS
```

---

### **Day 3: Metrics Bugs - Failing Tests (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 8 hours
**TDD Focus**: Write failing tests for metric label mismatches
**Gap Target**: GAP-RO-003, GAP-RO-004

**Morning (4 hours): Analyze Metric Definitions**

1. **Read current metric definitions** `pkg/remediationorchestrator/metrics/prometheus.go`
   ```go
   // Current (BUGGY):
   ReconcileTotal = prometheus.NewCounterVec(
       prometheus.CounterOpts{...},
       []string{"namespace", "phase", "result"},  // 3 labels
   )

   // Usage (WRONG - only 2 labels):
   ReconcileTotal.WithLabelValues(namespace, phase).Inc()  // Missing "result"!
   ```

2. **Create metric test file** `test/unit/remediationorchestrator/metrics_test.go`
   ```go
   var _ = Describe("Prometheus Metrics", func() {
       It("should emit ReconcileTotal with correct labels", func() {
           // This test will PANIC with current code
           metrics.ReconcileTotal.WithLabelValues("default", "Pending", "success").Inc()
           // Verify metric was recorded
       })
   })
   ```

**Afternoon (4 hours): Write Tests for All Metrics**

3. **Add tests for each metric** (one at a time, TDD cycle)
   - ReconcileTotal (3 labels: namespace, phase, result)
   - PhaseTransitionsTotal (3 labels: namespace, old_phase, new_phase)
   - ManualReviewNotificationsTotal
   - All other metrics in prometheus.go

**EOD Deliverables**:
- ✅ Metric test file created
- ❌ Tests PANIC or FAIL (RED phase - label mismatch)
- ✅ All metrics tested
- ✅ Day 3 EOD report

**Validation Commands**:
```bash
# Verify tests fail/panic
go test ./test/unit/remediationorchestrator/metrics_test.go -v 2>&1 | grep -E "FAIL|panic"

# Expected: Tests fail with label mismatch errors
```

---

### **Day 4: Metrics Bugs - Fix Implementation (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Fix metric implementations to pass tests
**Gap Target**: GAP-RO-003, GAP-RO-004

**Morning (4 hours): Fix Metric Definitions**

1. **Option A: Fix usage to match definitions**
   ```go
   // Fix usage to provide all 3 labels
   ReconcileTotal.WithLabelValues(namespace, phase, "success").Inc()
   ```

2. **Option B: Fix definitions to match usage**
   ```go
   // Reduce to 2 labels if "result" is not needed
   ReconcileTotal = prometheus.NewCounterVec(
       prometheus.CounterOpts{...},
       []string{"namespace", "phase"},  // 2 labels
   )
   ```

3. **Run tests → Verify they PASS**

**Afternoon (4 hours): Fix All Metric Usages**

4. **Update all metric calls in reconciler.go**
   - Search for `.WithLabelValues(` calls
   - Ensure label count matches definition
   - Add missing labels or remove excess from definition

5. **Update METRICS.md documentation**

**EOD Deliverables**:
- ✅ All metrics emit correctly (no panics)
- ✅ All metric tests passing (GREEN phase)
- ✅ METRICS.md updated
- ✅ Day 4 EOD report

**Validation Commands**:
```bash
# Run metric tests
go test ./test/unit/remediationorchestrator/metrics_test.go -v

# Expected: All tests PASS

# Verify no lint errors
golangci-lint run ./pkg/remediationorchestrator/metrics/...
```

---

### **Day 5: Audit Integration (DO-GREEN Phase) - ✅ COMPLETE**

**Phase**: DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Integrate audit client into RO
**Gap Target**: GAP-RO-005
**Status**: ✅ **COMPLETE** (December 9, 2025)

**Resolution**: Used existing single-event workaround in `pkg/audit/http_client.go`. DD-AUDIT-003 P1 events implemented.

**If UNBLOCKED - Morning (4 hours): Audit Client Integration**

1. **Add audit client to RO reconciler**
   ```go
   // pkg/remediationorchestrator/controller/reconciler.go
   type Reconciler struct {
       client      client.Client
       scheme      *runtime.Scheme
       auditStore  audit.Store  // NEW
       auditHelper *audit.Helper  // NEW
   }
   ```

2. **Add audit event emission points**
   - `reconcile.started` - When reconciliation begins
   - `reconcile.phase_changed` - When phase transitions
   - `reconcile.completed` - When reconciliation completes
   - `reconcile.failed` - When reconciliation fails

**If UNBLOCKED - Afternoon (4 hours): Audit Tests**

3. **Create audit integration tests**
   - Verify events emitted at each point
   - Verify event format per DD-AUDIT-003
   - Verify database persistence

**If BLOCKED - Alternative Work**

- **Option A**: Move to Day 6 (CHECK phase) early
- **Option B**: Work on E2E test infrastructure (Kind setup)
- **Option C**: Document audit integration plan for when unblocked

**EOD Deliverables (If Unblocked)**:
- ✅ Audit client wired to RO
- ✅ Audit events emitted at key points
- ✅ Audit tests passing
- ✅ Day 5 EOD report

**EOD Deliverables (If Blocked)**:
- ✅ Audit integration documented for future
- ✅ Alternative work completed
- ✅ Day 5 EOD report

---

### **Day 6: Unit + Integration Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Comprehensive test coverage validation

**Morning (4 hours): Unit Test Coverage**

1. **Run unit tests with coverage**
   ```bash
   go test ./pkg/remediationorchestrator/... -coverprofile=coverage.out
   go tool cover -func=coverage.out | grep total
   ```

2. **Target: ≥70% unit test coverage**
   - Add tests for uncovered code paths
   - Focus on critical logic (reconciler, handlers, creators)

**Afternoon (4 hours): Integration Test Validation**

3. **Run integration tests with real infrastructure**
   ```bash
   podman-compose -f test/integration/remediationorchestrator/podman-compose.test.yml up -d
   go test ./test/integration/remediationorchestrator/... -v
   ```

4. **Target: ≥50% integration test coverage**
   - Verify all integration points tested
   - Verify database persistence verified

**EOD Deliverables**:
- ✅ ≥70% unit test coverage
- ✅ ≥50% integration test coverage
- ✅ All tests passing
- ✅ Coverage report generated
- ✅ Day 6 EOD report

**Validation Commands**:
```bash
# Unit test coverage
go test ./pkg/remediationorchestrator/... -coverprofile=unit.out
go tool cover -func=unit.out | grep total
# Expected: total >= 70%

# Integration test coverage
go test ./test/integration/remediationorchestrator/... -coverprofile=integration.out
go tool cover -func=integration.out | grep total
# Expected: total >= 50%
```

---

### **Day 7: E2E Tests with Kind (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: End-to-end validation with real Kubernetes
**Gap Target**: GAP-RO-002

**Morning (4 hours): Kind Cluster Setup**

1. **Update E2E suite to use Kind** `test/e2e/remediationorchestrator/suite_test.go`
   ```go
   // Replace envtest with Kind
   var _ = BeforeSuite(func() {
       // Create Kind cluster with isolated kubeconfig
       cmd := exec.Command("kind", "create", "cluster",
           "--name", "ro-e2e-test",
           "--kubeconfig", os.ExpandEnv("$HOME/.kube/ro-e2e-config"),
       )
       // ...
   })
   ```

2. **Deploy CRDs to Kind cluster**
   ```bash
   kubectl apply -f config/crd/bases/ --kubeconfig ~/.kube/ro-e2e-config
   ```

**Afternoon (4 hours): E2E Test Implementation**

3. **Create actual E2E tests** `test/e2e/remediationorchestrator/lifecycle_e2e_test.go`
   - Full remediation lifecycle (Pending → Analyzing → Executing → Completed)
   - Approval flow (Pending → AwaitingApproval → Approved → Executing)
   - Failure handling (→ Failed → ManualReview)

4. **Verify with real Kubernetes**

**EOD Deliverables**:
- ✅ Kind cluster setup automated
- ✅ E2E tests implemented (not empty)
- ✅ All E2E tests passing
- ✅ Kubeconfig isolation verified
- ✅ Day 7 EOD report

**Validation Commands**:
```bash
# Create Kind cluster with isolated kubeconfig
kind create cluster --name ro-e2e-test --kubeconfig ~/.kube/ro-e2e-config

# Deploy CRDs
kubectl apply -f config/crd/bases/ --kubeconfig ~/.kube/ro-e2e-config

# Run E2E tests
go test ./test/e2e/remediationorchestrator/... -v

# Cleanup
kind delete cluster --name ro-e2e-test

# Expected: All E2E tests PASS
```

---

### **Day 8: Documentation + Production Readiness (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 8 hours
**Focus**: Finalize documentation and handoff

**Morning (4 hours): Documentation Updates**

1. **Update BR_MAPPING.md**
   - Add BR-ORCH-039 (Testing Tier Compliance)
   - Add BR-ORCH-040 (Prometheus Metrics Correctness)
   - Add BR-ORCH-041 (Audit Trail Integration - BLOCKED status)

2. **Update METRICS.md**
   - Document fixed metric definitions
   - Add label descriptions

3. **Create/Update testing-strategy.md for RO**

**Afternoon (4 hours): Production Readiness**

4. **Confidence assessment**
   ```
   Confidence: 85% (Evidence-Based)
   - Testing Tier Compliance: 95% (real infrastructure verified)
   - Metrics Correctness: 100% (all tests pass, no panics)
   - Audit Integration: 0% (BLOCKED on Data Storage)

   Overall: 85% (excluding blocked audit integration)
   ```

5. **Create handoff summary**

**EOD Deliverables**:
- ✅ BR_MAPPING.md updated
- ✅ METRICS.md updated
- ✅ Confidence assessment documented
- ✅ Handoff summary created
- ✅ Day 8 EOD report

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-ORCH-039** | Testing Tier Compliance | N/A | lifecycle_test.go | lifecycle_e2e_test.go | ✅ Complete |
| **BR-ORCH-040** | Prometheus Metrics Correctness | prometheus.go | N/A | N/A | ✅ Complete |
| **BR-ORCH-041** | Audit Trail Integration | helpers_test.go (93.6%) | audit_integration_test.go | N/A | ✅ Complete |

---

## ✅ **Previously Blocked Items (Now Resolved)**

### **GAP-RO-005: Audit Trail Integration - ✅ RESOLVED**

**Previously Blocked On**: Data Storage batch endpoint (`POST /api/v1/audit/events/batch`)

**Resolution Date**: December 9, 2025

**Resolution Details**:
- Used existing single-event workaround in `pkg/audit/http_client.go` (`storeSingleEvent` method)
- Implemented all DD-AUDIT-003 P1 events:
  - `orchestrator.lifecycle.started`
  - `orchestrator.phase.transitioned`
  - `orchestrator.lifecycle.completed`
- P2 event (`orchestrator.crd.updated`) deferred to V1.1 as approved by user

**Implementation**:
- `pkg/remediationorchestrator/audit/helpers.go` (93.6% unit test coverage)
- `pkg/remediationorchestrator/audit/helpers_test.go` (37 tests)
- `test/integration/remediationorchestrator/audit_integration_test.go`
- Integration in `pkg/remediationorchestrator/controller/reconciler.go`

**Tracking Document**: `docs/handoff/NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md`

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All unit tests passing (≥70% coverage)
- ✅ All integration tests passing (≥50% coverage, real infrastructure)
- ✅ All E2E tests passing (Kind cluster)
- ✅ No metric runtime panics
- ✅ No lint errors
- ✅ Audit events persisted (using single-event workaround)

### **Business Success**
- ✅ BR-ORCH-039 validated (Testing compliance)
- ✅ BR-ORCH-040 validated (Metrics correctness)
- ✅ BR-ORCH-041 validated (December 9, 2025)

### **Confidence Assessment**
- **Target**: ≥85% confidence (excluding blocked items)
- **Calculation**: Evidence-based (test coverage + BR validation)

---

## 🔗 **Related Documents**

- **Cross-Service Initiative**: `docs/initiatives/TESTING_TIER_COMPLIANCE_INITIATIVE.md` (to be created)
- **Data Storage Blocker**: `docs/handoff/NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md`
- **Feature Template**: `docs/services/FEATURE_EXTENSION_PLAN_TEMPLATE.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`

---

**Document Status**: ✅ **APPROVED**
**Last Updated**: December 9, 2025 (GAP-RO-005 resolved)
**Maintained By**: RO Team / AI Assistant
**Next Action**: Continue with remaining gaps (GAP-RO-001 through GAP-RO-004)


