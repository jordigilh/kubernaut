# RO Post-Atomic Updates Test Triage (December 26, 2025)

**Session Date**: December 26, 2025
**Context**: NT team implemented DD-PERF-001 atomic status updates for RemediationOrchestrator service
**Test Run**: All 3 tiers (unit, integration, E2E) after atomic updates implementation

---

## 📊 **Test Results Summary**

### ✅ **Unit Tests: 44/51 (86% Pass Rate)**

**Status**: 7 failures related to atomic status updates refactoring
**Command**: `make test-unit-remediationorchestrator`
**Duration**: ~1.1 seconds

#### **Failures Breakdown**

| Test | Category | Issue | Root Cause |
|------|----------|-------|------------|
| **1.4: Pending→Processing** | Phase Transition | Gateway metadata (`dedup_group`) not preserved | Status update may not preserve `spec` metadata |
| **3.5: Analyzing→Failed** | Phase Transition | Reason text mismatch ("AnalysisFailed" vs "AIAnalysis failed") | Reason string format changed in atomic update |
| **AE-7.5: Approval Requested** | Audit Events | Missing `orchestrator.approval.requested` event | Event not emitted in atomic update flow |
| **AE-7.6: Approval Approved** | Audit Events | Missing `orchestrator.approval.approved` event | Approval decision events not emitted |
| **AE-7.7: Approval Rejected** | Audit Events | Wrong event type (`lifecycle.completed` vs `approval.rejected`) | Event type mapping incorrect |
| **AE-7.8: Global Timeout** | Audit Events | No timeout event emitted | Timeout handler not calling audit emission |
| **AE-7.10: Routing Blocked** | Audit Events | Missing `lifecycle.started` event | Event emission order changed |

#### **Expected Test Locations**
```bash
# Phase transition tests
test/unit/remediationorchestrator/controller/reconcile_phases_test.go:205  # 1.4
test/unit/remediationorchestrator/controller/reconcile_phases_test.go:417  # 3.5

# Audit event tests
test/unit/remediationorchestrator/controller/audit_events_test.go:225      # AE-7.5
test/unit/remediationorchestrator/controller/audit_events_test.go:252      # AE-7.6
test/unit/remediationorchestrator/controller/audit_events_test.go:282      # AE-7.7
test/unit/remediationorchestrator/controller/audit_events_test.go:312      # AE-7.8
test/unit/remediationorchestrator/controller/audit_events_test.go:368      # AE-7.10
```

---

### ✅ **Integration Tests: 56/57 (98% Pass Rate)**

**Status**: 1 approval audit event timeout
**Command**: `make test-integration-remediationorchestrator`
**Duration**: ~190 seconds (3 minutes)

#### **Failure Details**

**AE-INT-5: Approval Requested Audit**
- **Test**: `test/integration/remediationorchestrator/audit_emission_integration_test.go:414`
- **Issue**: Event not emitted within 15s timeout
- **Expected**: `orchestrator.approval.requested` event when low confidence triggers approval
- **Actual**: Event never appears in DataStorage audit_events table
- **Root Cause**: Likely same as unit test AE-7.5 - event not emitted during atomic status update

---

### ❌ **E2E Tests: 0/28 (Infrastructure Setup Failed)**

**Status**: DataStorage pod not ready within 120s timeout
**Command**: `make test-e2e-remediationorchestrator`
**Duration**: ~309 seconds (5 minutes)

#### **Infrastructure Failure Details**

**Symptom**: DataStorage pod stuck in non-ready state
**Location**: `test/infrastructure/remediationorchestrator_e2e_hybrid.go:600`
**Timeout**: 120 seconds (2 minutes)

**Services Status**:
- ✅ PostgreSQL pod: Ready
- ✅ Redis pod: Ready
- ✅ RemediationOrchestrator controller: Ready
- ✅ Migrations: Applied successfully (all 11 migrations)
- ❌ **DataStorage pod: NOT ready** (blocking E2E tests)

**Evidence from Test Log**:
```
✅ RemediationOrchestrator ready
✅ Applied 001_initial_schema.sql
✅ Applied 002_fix_partitioning.sql
... (9 more migrations) ...
✅ Applied 1000_create_audit_events_partitions.sql
✅ Migrations applied successfully
✅ All manifests applied! (Kubernetes reconciling...)
⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...
   ⏳ Waiting for DataStorage pod to be ready...
❌ FAILED: Timed out after 120.000s.
```

#### **DataStorage Pod Configuration**

**Image**: `localhost/kubernaut-datastorage:e2e-test-remediationorchestrator`
**Built with**: `--no-cache` + `GOFLAGS=-cover` (if E2E_COVERAGE=true)
**Namespace**: `kubernaut-system`
**Label Selector**: `app=datastorage`

**Readiness Probe**:
- Path: `/health`
- Port: 8080
- Initial Delay: 5s
- Period: 5s
- Timeout: 3s

**Liveness Probe**:
- Path: `/health`
- Port: 8080
- Initial Delay: 30s
- Period: 10s
- Timeout: 5s

**Config Dependencies**:
- PostgreSQL: `postgresql.kubernaut-system.svc.cluster.local:5432`
- Redis: `redis.kubernaut-system.svc.cluster.local:6379`
- ConfigMap: `datastorage-config`
- Secret: `datastorage-secret`

---

## 🔍 **Root Cause Analysis**

### **Atomic Status Updates Impact**

The NT team implemented `DD-PERF-001: Atomic Status Updates` for the RO service. This refactoring:

1. **Consolidated Status Updates**: Multiple `Status().Update()` calls → single atomic update
2. **Changed Update Flow**: Status fields now batched and updated together
3. **Affected Event Emission**: Audit events may be emitted at different points in reconciliation

### **Expected Side Effects**

✅ **Expected** (per DD-PERF-001):
- E2E tests pass unchanged (observable behavior identical)
- Unit tests may need updates for new status manager API
- 75-90% reduction in K8s API calls

❌ **Unexpected** (requires investigation):
- Audit events not emitted (7 unit test failures, 1 integration failure)
- Gateway metadata not preserved during phase transitions
- DataStorage pod stuck in non-ready state (E2E blocker)

---

## 🚨 **Critical Issues Requiring Investigation**

### **Issue 1: Audit Event Emission (HIGH PRIORITY)**

**Affected Tests**: 4 unit tests + 1 integration test

**Hypothesis**: Audit events previously emitted during inline `Status().Update()` calls are now missing because they're called after status update batching.

**Investigation Steps**:
1. Check if `emitLifecycleStartedAudit` is called before atomic status update
2. Verify `emitApprovalRequestedAudit` is called when `AwaitingApproval` phase is set
3. Confirm `emitApprovalDecisionAudit` is called for `Approved`/`Rejected` decisions
4. Validate timeout handler calls `emitTimeoutAudit`

**Files to Check**:
- `internal/controller/remediationorchestrator/reconciler.go`
- `pkg/remediationorchestrator/status/manager.go` (if created by NT team)

---

### **Issue 2: Gateway Metadata Preservation (MEDIUM PRIORITY)**

**Affected Tests**: 1 unit test (1.4: Pending→Processing)

**Hypothesis**: The atomic status update may be using `Get()` to refetch the RR, which loses in-memory changes to `spec.gatewayMetadata`.

**Investigation Steps**:
1. Check if `AtomicStatusUpdate` calls `Get()` before `updateFunc()`
2. Verify `spec.gatewayMetadata` is preserved across refetch
3. Confirm test expectation: `dedup_group: "test-group"` should exist after phase transition

**Files to Check**:
- `pkg/remediationorchestrator/status/manager.go`
- `internal/controller/remediationorchestrator/reconciler.go` (phase transition logic)

---

### **Issue 3: DataStorage Pod Not Ready (CRITICAL - E2E BLOCKER)**

**Symptom**: Pod exists, manifests applied, but pod never becomes ready

**Hypothesis**:
1. **Health endpoint failing**: `/health` returning non-200 status
2. **Dependency issue**: DataStorage unable to connect to PostgreSQL/Redis
3. **Metrics issue**: If atomic updates affected DataStorage metrics, service may panic on startup
4. **Coverage issue**: DD-TEST-007 coverage instrumentation may have issue

**Investigation Steps** (requires rerunning E2E with cluster preserved):

```bash
# Set cluster preservation
export PRESERVE_E2E_CLUSTER=true

# Run E2E tests (will fail but keep cluster)
make test-e2e-remediationorchestrator

# Access cluster
export KUBECONFIG=~/.kube/ro-e2e-config

# Check DataStorage pod status
kubectl get pods -n kubernaut-system -l app=datastorage

# Get pod details
kubectl describe pod -n kubernaut-system -l app=datastorage

# Check pod logs
kubectl logs -n kubernaut-system -l app=datastorage --tail=100

# Check readiness probe results
kubectl get events -n kubernaut-system --sort-by='.lastTimestamp' | grep datastorage

# Test health endpoint directly (if pod is running but failing probe)
kubectl port-forward -n kubernaut-system svc/datastorage 8080:8080 &
curl http://localhost:8080/health

# Check if pod can reach PostgreSQL
kubectl exec -n kubernaut-system -l app=datastorage -- nc -zv postgresql 5432

# Check if pod can reach Redis
kubectl exec -n kubernaut-system -l app=datastorage -- nc -zv redis 6379

# Cleanup when done
kind delete cluster --name ro-e2e
```

**Files to Check**:
- `pkg/datastorage/server/server.go` (health endpoint implementation)
- `pkg/datastorage/metrics/metrics.go` (if affected by atomic updates)
- `docker/data-storage.Dockerfile` (build issues?)
- `test/infrastructure/datastorage.go:708-1004` (deployment manifest)

---

## 🔧 **Recommended Next Steps**

### **Step 1: Triage DataStorage Pod Issue (PRIORITY 1)**

**Goal**: Unblock E2E tests

1. Rerun E2E with `PRESERVE_E2E_CLUSTER=true`
2. Inspect DataStorage pod logs for errors
3. Verify `/health` endpoint responds correctly
4. Check PostgreSQL/Redis connectivity from pod
5. Fix root cause and verify E2E infrastructure setup completes

**Expected Outcome**: DataStorage pod becomes ready, E2E tests can run

---

### **Step 2: Fix Audit Event Emission (PRIORITY 2)**

**Goal**: Restore audit event emission in atomic update flow

1. Review NT team's atomic status update implementation
2. Identify where audit events were previously emitted
3. Ensure audit emission happens BEFORE or AFTER atomic update (not during)
4. Update tests if event emission timing changed (but events still occur)

**Expected Outcome**: 7 unit test failures → 0, 1 integration failure → 0

---

### **Step 3: Fix Gateway Metadata Preservation (PRIORITY 3)**

**Goal**: Ensure spec fields survive atomic status updates

1. Check if atomic update refetches the object (optimistic locking)
2. If refetch loses in-memory spec changes, modify approach:
   - Option A: Don't refetch if only status is changing
   - Option B: Preserve spec changes during refetch
   - Option C: Update test to not rely on in-memory spec changes

**Expected Outcome**: 1 unit test failure → 0

---

### **Step 4: Apply Unique Namespace Pattern (PRIORITY 4)**

**Goal**: Enable parallel E2E tests (after infrastructure fixed)

1. Use `test/e2e/remediationorchestrator/helpers.go::GenerateUniqueNamespace()`
2. Update all RO E2E tests to create unique namespaces
3. Pattern: `ro-e2e-p{process}-{uuid-8chars}`
4. Ensure cleanup deletes test namespaces

**Expected Outcome**: E2E tests can run in parallel without namespace collisions

---

## 📋 **Test Commands**

### **Rerun All 3 Tiers**
```bash
# Unit tests
make test-unit-remediationorchestrator

# Integration tests
make test-integration-remediationorchestrator

# E2E tests (preserve cluster for debugging)
PRESERVE_E2E_CLUSTER=true make test-e2e-remediationorchestrator
```

### **Debug DataStorage Pod**
```bash
# Set kubeconfig
export KUBECONFIG=~/.kube/ro-e2e-config

# Check pod
kubectl get pods -n kubernaut-system
kubectl describe pod -n kubernaut-system -l app=datastorage
kubectl logs -n kubernaut-system -l app=datastorage

# Cleanup
kind delete cluster --name ro-e2e
```

---

## 📊 **Success Criteria**

### **Tier 1: Unit Tests**
- ✅ Target: 51/51 passing (100%)
- 🔄 Current: 44/51 passing (86%)
- 📌 Gap: 7 failures (audit events + metadata)

### **Tier 2: Integration Tests**
- ✅ Target: 57/57 passing (100%)
- 🔄 Current: 56/57 passing (98%)
- 📌 Gap: 1 failure (approval audit timeout)

### **Tier 3: E2E Tests**
- ✅ Target: 28/28 passing (100%)
- 🔄 Current: 0/28 (infrastructure failure)
- 📌 Gap: DataStorage pod not ready (critical blocker)

---

## 🔗 **Related Documentation**

- **DD-PERF-001**: `docs/architecture/decisions/DD-PERF-001-atomic-status-updates-mandate.md`
- **DD-AUDIT-003**: Audit event emission standards
- **DD-TEST-007**: E2E coverage capture standard
- **ADR-034**: Audit event specification
- **Notification Reference**: `pkg/notification/status/manager.go` (completed atomic updates)

---

## 📝 **Session Log**

**Test Execution**:
- Unit tests: `/tmp/ro-unit-tests.log`
- Integration tests: `/tmp/ro-integration-tests.log`
- E2E tests: `/tmp/ro-e2e-tests.log`

**Cluster Status**: Deleted after E2E failure (no logs available)

**Next Session**: Rerun with `PRESERVE_E2E_CLUSTER=true` to inspect DataStorage pod

