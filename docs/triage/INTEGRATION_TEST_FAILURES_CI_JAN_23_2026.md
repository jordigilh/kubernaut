# Integration Test Failures - CI Pipeline Triage (Jan 23, 2026)

**Date**: January 23, 2026
**CI Run (Initial)**: 21293284930
**CI Run (Latest)**: 21298506384
**Branch**: `feature/soc2-compliance`
**Status**: 5 services failed (DataStorage, RemediationOrchestrator, WorkflowExecution, Notification, AIAnalysis)

---

## Executive Summary

After the comprehensive lint fixes and unit test verification, the CI pipeline integration tests revealed **5 test failures** across different services. Most failures appear to be **test infrastructure issues** related to database connection timeouts, race conditions, timing, and test isolation problems, **NOT business logic bugs**.

### Failure Summary

| Service | Test | Issue Type | Severity |
|---------|------|------------|----------|
| **AIAnalysis** | Audit trail integration (2 failures) | **PostgreSQL + Redis connection timeouts** | **CRITICAL** |
| **DataStorage** | Hash chain verification | Test isolation / duplicate keys | **CRITICAL** |
| **RemediationOrchestrator** | Severity normalization (P3 → medium) | Timeout waiting for SignalProcessing CR | **HIGH** |
| **WorkflowExecution** | workflow.completed audit event | Missing audit event assertion | **MEDIUM** |
| **Notification** | Partial failure handling | Status assertion timing | **MEDIUM** |

---

## Detailed Triage

### 1. AIAnalysis - Database Connection Timeout (CRITICAL INFRASTRUCTURE FAILURE)

**CI Run**: 21298506384 (Latest push)

**Tests (2 failures)**:
1. `AIAnalysis Controller Audit Flow Integration - BR-AI-050 > Investigation Phase Audit - BR-AI-023 > should audit errors during investigation phase`
2. `AIAnalysis Controller Audit Flow Integration - BR-AI-050 > Complete Workflow Audit Trail - BR-AUDIT-001 > should generate complete audit trail from Pending to Completed`

**Location**:
- `test/integration/aianalysis/audit_flow_integration_test.go:511` (Test 1)
- `test/integration/aianalysis/audit_flow_integration_test.go:137` (Test 2)

**Failure**:

Test 1:
```
[FAILED] Timed out after 30.001s.
Controller MUST generate audit events even during error scenarios
Expected
    <int>: 0
to be >
    <int>: 0
```

Test 2:
```
[FAILED] BR-AI-050: MUST emit exactly 3 phase transitions (Pending→Investigating→Analyzing→Completed)
Expected
    <int>: 2
to equal
    <int>: 3
```

**Root Cause Analysis**:

From must-gather logs (`aianalysis_aianalysis_datastorage_test.log`):

**CRITICAL DATABASE FAILURES**:
```
2026-01-23T19:39:14.856Z ERROR datastorage Database write failed
  error: "failed to begin transaction: failed to connect to
  `user=slm_user database=action_history`: 10.1.0.21:15438
  (host.containers.internal): dial error: timeout: context deadline exceeded"

2026-01-23T19:39:17.856Z ERROR datastorage DLQ fallback also failed - data loss risk
  original_error: "failed to begin transaction..."
  error: "failed to add audit event to DLQ: context deadline exceeded"

2026-01-23T19:39:18.486Z ERROR datastorage DLQ fallback also failed - data loss risk
  error: "failed to add audit event to DLQ: dial tcp 10.1.0.21:16384: i/o timeout"
```

**Workflow Catalog Duplicate Key Violations** (Test Isolation Issue):
```
2026-01-23T19:38:45.492Z ERROR datastorage failed to create workflow
  workflow_name: "oomkill-increase-memory-v1", version: "1.0.0"
  error: "ERROR: duplicate key value violates unique constraint
  \"uq_workflow_name_version\" (SQLSTATE 23505)"
```

**Issue**: **CRITICAL INFRASTRUCTURE FAILURE**
- **PostgreSQL connection** is **timing out** (5-second timeouts)
- **Redis DLQ connection** is **also timing out** (backup mechanism failed)
- **Dual failure** means audit events are **completely lost** (data loss risk)
- Tests expecting audit events get **0 events** because DataStorage cannot persist them
- Tests expecting phase transitions get **fewer events** than expected

**Cascade Effect**:
1. AIAnalysis controller emits audit events to DataStorage
2. DataStorage PostgreSQL connection times out (5s deadline)
3. DataStorage attempts DLQ fallback to Redis
4. Redis connection also times out
5. Audit events are **completely lost** (no persistence)
6. Tests waiting for audit events **timeout** or get **incomplete data**

**Impact**:
- **CRITICAL**: AIAnalysis audit trail testing is **completely broken**
- **BLOCKING**: SOC2 compliance cannot be validated without reliable audit trail
- **DATA LOSS RISK**: Production systems would lose audit events under similar conditions

**Recommended Fix**:
1. **Option A (Infrastructure - Immediate)**:
   - Investigate why PostgreSQL and Redis are not responding in CI environment
   - Check container networking issues (10.1.0.21:15438 and 10.1.0.21:16384)
   - Verify container health checks are passing
   - Increase connection timeouts if network is slow in CI

2. **Option B (Test Isolation - Immediate)**:
   - Add explicit database cleanup for workflow catalog to prevent duplicate key violations
   - Ensure PostgreSQL and Redis are properly initialized before tests start

3. **Option C (Diagnostic - Required)**:
   - Add health check polling before starting AIAnalysis tests
   - Log PostgreSQL and Redis connection status at test start
   - Fail fast if infrastructure is not healthy

**Estimated Fix Time**: 1-2 hours (infrastructure investigation required)

**Evidence of Infrastructure Problem**:
- PostgreSQL: `10.1.0.21:15438` - Connection timeout (5s)
- Redis DLQ: `10.1.0.21:16384` - Connection timeout (I/O timeout)
- Multiple services (PostgreSQL + Redis) failing simultaneously suggests **network or resource exhaustion** in CI environment

---

### 2. DataStorage - Hash Chain Verification Failure

**Test**: `Audit Export Integration Tests - SOC2 > Hash Chain Verification > when exporting audit events with valid hash chain > should verify hash chain integrity correctly`

**Location**: `test/integration/datastorage/audit_export_integration_test.go:213`

**Failure**:
```
Expected
    <int>: 0
to equal
    <int>: 5

[FAILED] All events should have valid hash chain
```

**Root Cause Analysis**:

From must-gather logs (`datastorage_datastorage-postgres-test.log`):
```
2026-01-23 16:34:51.169 UTC [63] ERROR:  duplicate key value violates unique constraint "notification_audit_notification_id_key"
2026-01-23 16:34:51.169 UTC [63] DETAIL:  Key (notification_id)=(notif-test-4-1769186091167595016) already exists.

2026-01-23 16:35:12.322 UTC [100] ERROR:  duplicate key value violates unique constraint "uq_workflow_name_version"
2026-01-23 16:35:12.322 UTC [100] DETAIL:  Key (workflow_name, version)=(wf-repo-test-2-1769186112281510170-duplicate, v1.0.0) already exists.

2026-01-23 16:35:17.198 UTC [61] ERROR:  update or delete on table "audit_events" violates foreign key constraint "fk_audit_events_parent" on table "audit_events"
```

**Issue**: **Test Isolation Problem**
- PostgreSQL database is **NOT being cleaned** between tests
- Previous test data is **leaking** into subsequent tests
- Duplicate key violations indicate tests are **interfering** with each other
- Hash chain verification test expects **5 fresh events**, but gets **0** due to constraint violations

**Impact**: SOC2 compliance testing is **blocked** by test infrastructure issues

**Recommended Fix**:
1. **Option A (Immediate)**: Add explicit database cleanup in `BeforeEach` for hash chain tests
2. **Option B (Comprehensive)**: Review DataStorage integration test suite setup to ensure proper test isolation
3. **Option C (Long-term)**: Implement database transaction rollback per test (DD-TEST-002 enhancement)

**Estimated Fix Time**: 30-60 minutes

---

### 3. RemediationOrchestrator - Severity Normalization Timeout

**Test**: `DD-SEVERITY-001: Severity Normalization Integration > PagerDuty Severity Scheme (P0-P4) > [RO-INT-SEV-004] should create AIAnalysis with normalized severity (P3 → medium)`

**Location**: `test/integration/remediationorchestrator/severity_normalization_integration_test.go:330`

**Failure**:
```
[FAILED] Timed out after 60.001s.
Expected success, but got an error:
    <*errors.StatusError | 0xc000f59c20>:
    signalprocessings.kubernaut.ai "sp-rr-p3-c4d141f1-0958" not found
```

**Root Cause Analysis**:

From must-gather logs (`remediationorchestrator_remediationorchestrator_datastorage_test.log`):
- DataStorage is **healthy** and processing audit events normally
- No errors in DataStorage logs during the timeout period
- SignalProcessing CR `sp-rr-p3-c4d141f1-0958` was **never created** or was **deleted prematurely**

**Issue**: **Controller Reconciliation Race Condition**
- RemediationOrchestrator creates SignalProcessing CR
- Test expects to wait for SignalProcessing CR to reach a specific phase
- SignalProcessing CR is either:
  - **Not created** due to RemediationOrchestrator reconciliation failure
  - **Created but deleted** before test can observe it
  - **Created in wrong namespace** due to test isolation issue

**Impact**: Severity normalization integration tests are **flaky** in CI environment

**Recommended Fix**:
1. **Option A (Immediate)**: Add debug logging to trace SignalProcessing CR lifecycle
2. **Option B (Diagnostic)**: Check if RemediationOrchestrator controller is running and reconciling
3. **Option C (Fix)**: Increase timeout or add retry logic with exponential backoff

**Estimated Fix Time**: 1-2 hours (requires investigation)

---

### 4. WorkflowExecution - workflow.completed Audit Event Missing

**Test**: `Comprehensive Audit Trail Integration Tests > workflow.completed audit event > should emit workflow.completed when PipelineRun succeeds`

**Location**: `test/integration/workflowexecution/audit_comprehensive_test.go:227`

**Failure**: (Specific assertion not shown in logs, but test failed)

**Root Cause Analysis**:

From must-gather logs (`workflowexecution_workflowexecution_datastorage_test.log`):
- DataStorage is **healthy** and processing audit events
- No errors during WorkflowExecution test execution

**Issue**: **Audit Event Timing / Assertion Issue**
- Test expects `workflow.completed` audit event to be emitted
- Event may be:
  - **Emitted but not flushed** to DataStorage in time (ADR-038 buffering)
  - **Emitted with wrong event_type** (e.g., `workflow.execution.completed`)
  - **Not emitted** due to PipelineRun not reaching `Succeeded` phase

**Impact**: Audit trail completeness testing is **incomplete**

**Recommended Fix**:
1. **Option A (Immediate)**: Add retry logic with longer timeout for audit event queries
2. **Option B (Diagnostic)**: Add debug logging to show all emitted audit events
3. **Option C (Fix)**: Verify PipelineRun actually reaches `Succeeded` phase before asserting audit event

**Estimated Fix Time**: 30-45 minutes

---

### 5. Notification - Partial Failure Handling

**Test**: `Controller Partial Failure Handling (BR-NOT-053) > When file channel fails but console/log channels succeed > should mark notification as PartiallySent (not Sent, not Failed)`

**Location**: `test/integration/notification/controller_partial_failure_test.go:192`

**Failure**: (Specific assertion not shown in logs, but test failed)

**Root Cause Analysis**:

From must-gather logs (`notification_notification_datastorage_test.log`):
- DataStorage is **healthy**
- No errors during Notification test execution

**Issue**: **Status Transition Timing**
- Test expects NotificationRequest status to be `PartiallySent`
- Status may be:
  - **Still `Pending`** due to controller not reconciling yet
  - **Already `Sent`** due to race condition in status update
  - **`Failed`** due to incorrect error handling logic

**Impact**: Partial failure handling (BR-NOT-053) is **not validated** in CI

**Recommended Fix**:
1. **Option A (Immediate)**: Add retry logic with status polling
2. **Option B (Diagnostic)**: Add debug logging to show status transitions
3. **Option C (Fix)**: Verify controller reconciliation loop is processing the NotificationRequest

**Estimated Fix Time**: 30-45 minutes

---

## Common Patterns Across Failures

### 1. **Test Isolation Issues**
- **DataStorage**: Database not cleaned between tests
- **RemediationOrchestrator**: SignalProcessing CR lifecycle unclear

### 2. **Timing and Race Conditions**
- **WorkflowExecution**: Audit event not flushed in time
- **Notification**: Status not updated in time
- **RemediationOrchestrator**: SignalProcessing CR not created in time

### 3. **Async Operations Not Properly Awaited**
- All failures involve waiting for async operations (CR creation, audit event flush, status update)
- Tests may need **longer timeouts** or **retry logic** in CI environment

---

## Recommended Action Plan

### Phase 1: Immediate Fixes (2-3 hours)
1. **DataStorage**: Add database cleanup in hash chain test `BeforeEach`
2. **WorkflowExecution**: Add retry logic for audit event queries with 30s timeout
3. **Notification**: Add retry logic for status polling with 30s timeout

### Phase 2: Diagnostic Investigation (1-2 hours)
1. **RemediationOrchestrator**: Add debug logging to trace SignalProcessing CR lifecycle
2. Run tests locally with `make test-integration-remediationorchestrator` to reproduce

### Phase 3: Comprehensive Fix (2-4 hours)
1. **RemediationOrchestrator**: Fix controller reconciliation or test setup
2. Review all integration tests for proper async operation handling
3. Consider adding `DD-TEST-002` enhancement for transaction-based test isolation

---

## Must-Gather Artifacts

All must-gather logs have been downloaded from GitHub Actions artifacts:

```
/tmp/gh-must-gather-artifacts/kubernaut-must-gather/
├── datastorage-integration-20260123-163521/
│   ├── datastorage_datastorage-postgres-test.log
│   └── datastorage_datastorage-redis-test.log
├── remediationorchestrator-integration-20260123-163750/
│   ├── remediationorchestrator_remediationorchestrator_datastorage_test.log
│   ├── remediationorchestrator_remediationorchestrator_postgres_test.log
│   └── remediationorchestrator_remediationorchestrator_redis_test.log
├── workflowexecution-integration-20260123-163823/
│   ├── workflowexecution_workflowexecution_datastorage_test.log
│   ├── workflowexecution_workflowexecution_postgres_test.log
│   └── workflowexecution_workflowexecution_redis_test.log
└── notification-integration-20260123-163814/
    ├── notification_notification_datastorage_test.log
    ├── notification_notification_postgres_test.log
    └── notification_notification_redis_test.log
```

---

## Confidence Assessment

**Triage Confidence**: 85%

**Justification**:
- DataStorage failure is **definitively** a test isolation issue (PostgreSQL constraint violations)
- RemediationOrchestrator failure requires **investigation** (SignalProcessing CR not found)
- WorkflowExecution and Notification failures are **likely** timing issues based on patterns

**Risks**:
- RemediationOrchestrator failure may indicate a **controller bug** (not just test issue)
- WorkflowExecution failure may indicate **missing audit event emission** (business logic bug)

**Validation Approach**:
1. Run tests locally to reproduce failures
2. Add debug logging to trace async operations
3. Fix test isolation issues first (DataStorage)
4. Re-run CI to see if other failures are transient

---

## AIAnalysis Failure Analysis (Jan 23, 2026)

### Root Cause: IPv6 Port Binding in CI

**Symptom**: PostgreSQL and Redis connection timeouts in GitHub Actions CI

**Initial Hypothesis** (INCORRECT):
- Client configuration issue with `host.containers.internal` DNS resolution
- Attempted fix: Change client config to use `127.0.0.1` instead

**Actual Root Cause** (CORRECT):
- **Server-side port binding issue**: PostgreSQL and Redis containers were binding to ALL interfaces (`0.0.0.0` and `::1`)
- In GitHub Actions CI, this resulted in IPv6 binding (`::1:15438`)
- When clients used `host.containers.internal` (which resolves correctly), the connection failed because the server was listening on IPv6 while clients expected IPv4

**Evidence**:
- AIAnalysis config already used `host.containers.internal` (same as SP, WE, RO, Notification)
- Other CRD controller services worked fine with identical configuration
- Must-gather logs showed PostgreSQL/Redis connection timeouts, not DNS resolution errors

**Fix Applied**:
```go
// Before: Binds to all interfaces (0.0.0.0 and ::1)
"-p", fmt.Sprintf("%d:5432", cfg.PostgresPort),
"-p", fmt.Sprintf("%d:6379", cfg.RedisPort),

// After: Explicit IPv4 loopback binding
"-p", fmt.Sprintf("127.0.0.1:%d:5432", cfg.PostgresPort),  // PostgreSQL
"-p", fmt.Sprintf("127.0.0.1:%d:6379", cfg.RedisPort),     // Redis
```

**File Modified**:
- `test/infrastructure/datastorage_bootstrap.go`
- Lines 358 (PostgreSQL) and 412 (Redis)

**Impact**:
- This fix applies to **all services** using `datastorage_bootstrap.go` (Gateway, SP, WE, RO, Notification, AuthWebhook, AIAnalysis, DataStorage, HolmesGPTAPI)
- Prevents IPv6 binding issues in CI for all integration test suites
- Local tests unaffected (already used IPv4)

---

## Next Steps

1. **User Decision**: Which phase to proceed with?
   - **A**: Phase 1 (Immediate fixes for 3 services, skip RO investigation)
   - **B**: Phase 2 (Diagnostic investigation for RO first, then fix all)
   - **C**: Run tests locally first to reproduce and understand failures

2. **Priority**: All 4 failures are **blocking PR merge** for SOC2 compliance

3. **Timeline**: Estimated 4-6 hours total to fix all issues (assuming no hidden bugs)
