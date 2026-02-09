# AIAnalysis Integration Test Plan V1.0

**Service**: AIAnalysis
**Test Type**: Integration
**Version**: 1.1 (Updated for Guidelines Compliance)
**Created**: December 24, 2025
**Last Updated**: December 24, 2025 (Triage vs TESTING_GUIDELINES.md)
**Status**: 🟡 In Progress (53/114 scenarios implemented)
**Current Coverage**: 54.6% → Target 60-70% (Guidelines: 50% ✅ EXCEEDS)

**Guidelines Compliance**: ✅ Aligned with [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 📋 TEST PLAN OVERVIEW

### Purpose
Comprehensive integration test coverage for AIAnalysis service focusing on error handling, retry logic, controller orchestration, and **V1.0 maturity compliance** (metrics, audit, graceful shutdown).

### Scope
- Error classification and handling
- Retry logic with exponential backoff
- Controller state transitions
- Condition management
- Recovery scenarios
- Audit trail validation
- **🆕 V1.0 Maturity**: Metrics, audit traces, EventRecorder, graceful shutdown (Phase 4)

### Test ID Format
`AA-INT-[CATEGORY]-[NUMBER]`

**Categories**:
- `ERR`: Error classification and handling
- `RETRY`: Retry logic and backoff
- `CTRL`: Controller orchestration
- `COND`: Condition management
- `RCV`: Recovery scenarios
- `AUDIT`: Audit trail validation
- **🆕 `METRICS`**: Metrics testing (V1.0 maturity)
- **🆕 `SHUTDOWN`**: Graceful shutdown testing (V1.0 maturity)

### Guidelines Compliance (TESTING_GUIDELINES.md)

**Coverage Targets** (Lines 47-81):
- **Unit**: 70%+ (current: 70.0% ✅)
- **Integration**: 50% target (current: 54.6% ✅ **EXCEEDS**)
- **E2E**: 50% target (not yet measured)

**Key Insight**: AIAnalysis integration tests **already exceed** the 50% guideline target. This plan focuses on **critical path coverage** (error handling, retry logic) and **V1.0 maturity compliance** (metrics, audit validation).

**Infrastructure** (Lines 1030-1199):
- ✅ Uses DD-TEST-002 sequential startup pattern
- ✅ No `podman-compose` race conditions
- ✅ Auto-started in `SynchronizedBeforeSuite`

**Test Patterns** (Lines 573-985):
- ✅ `Eventually()` required for async operations (NEVER `time.Sleep()`)
- ✅ `Fail()` required for missing services (NEVER `Skip()`)
- ✅ Mock LLM policy (cost constraint)

---

## 🏗️ TEST INFRASTRUCTURE (DD-TEST-002 Sequential Startup)

**Authority**: [DD-TEST-002: Integration Test Container Orchestration](../../architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md)

### Sequential Startup Pattern (Per TESTING_GUIDELINES.md Lines 1030-1199)

AIAnalysis integration tests use **DD-TEST-002 compliant sequential startup** to avoid race conditions:

```go
// test/integration/aianalysis/suite_test.go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Sequential startup: PostgreSQL → Redis → DataStorage → HAPI
    // Each service waits for previous service's health check
    err := infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())

    // All services healthy before tests start
    return []byte{}
}, func(data []byte) {
    // Parallel workers reuse infrastructure
})
```

**Why NOT `podman-compose`?**

❌ `podman-compose up -d` starts services **simultaneously** → race conditions
✅ Sequential startup with health checks → **NO race conditions**

**Services Started**:
1. **PostgreSQL** (port 15438) - Wait for `pg_isready`
2. **Redis** (port 16384) - Wait for `redis-cli ping`
3. **DataStorage** (port 18095) - Wait for `/health` endpoint
4. **HAPI** (port 18120) - Wait for `/health` endpoint with `MOCK_LLM_MODE=true`

**Reference**: `test/infrastructure/datastorage_bootstrap.go` - `StartAIAnalysisIntegrationInfrastructure()`

### Mock LLM Policy (Cost Constraint)

**Authority**: TESTING_GUIDELINES.md Lines 437-469

**All test tiers use mock LLM** to prevent runaway API costs:

| Test Tier | HAPI | LLM | Rationale |
|-----------|------|-----|-----------|
| Unit | Mock | Mock | No real services |
| Integration | **Real** | **Mock** | Cost constraint |
| E2E | **Real** | **Mock** | Cost constraint |

**HAPI Configuration**:
```go
// test/integration/aianalysis/suite_test.go
hapiConfig := infrastructure.GenericContainerConfig{
    Env: map[string]string{
        "MOCK_LLM_MODE": "true", // Enables deterministic mock responses
        "LOG_LEVEL":     "INFO",
    },
}
```

**Mock Response Behavior**:
- HAPI returns deterministic responses based on signal type
- Example: `SignalType="OOMKilled"` → returns memory-related workflow
- Allows repeatable, predictable test scenarios
- No actual LLM API costs

**Reference**: `holmesgpt-api/src/mock_responses.py` - Mock response generator

---

## 🔴 PHASE 1: CRITICAL ERROR HANDLING (Priority: CRITICAL)

**Target Coverage**: 70-75%
**Estimated Tests**: 15-20
**Effort**: Week 1
**File**: `test/integration/aianalysis/error_classification_test.go`

### Error Classification Tests

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-ERR-001** | Classify 401 Unauthorized as Authentication Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-002** | Classify 403 Forbidden as Authentication Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-003** | Classify 400 Bad Request as Validation Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-004** | Classify 422 Unprocessable Entity as Validation Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-005** | Classify 404 Not Found as NotFound Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-006** | Classify 409 Conflict as Conflict Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-007** | Classify 429 Too Many Requests as RateLimit Error | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-008** | Classify 500 Internal Server Error as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-009** | Classify 502 Bad Gateway as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-010** | Classify 503 Service Unavailable as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-011** | Classify 504 Gateway Timeout as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-012** | Classify context.DeadlineExceeded as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-013** | Classify context.Canceled as Permanent (NOT transient) | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-014** | Classify "connection refused" as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-015** | Classify "no such host" as Permanent | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-016** | Classify "TLS handshake timeout" as Transient | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-017** | Default unknown errors to Permanent classification | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-ERR-018** | Verify error classification metrics recorded correctly | BR-AI-009, DD-METRIC-001 | 🔴 Critical | ⏸️ Pending |

### Test Details

#### AA-INT-ERR-001: Classify 401 Unauthorized as Authentication Error
**Description**: When HAPI returns 401 Unauthorized, system should classify as authentication error and fail permanently (no retry).

**Test Steps**:
1. Configure mock HAPI to return 401 with body: `{"error": "401 Unauthorized"}`
2. Create AIAnalysis CRD with valid spec
3. Wait for reconciliation

**Expected Results**:
- `Status.Phase` = `"Failed"`
- `Status.Reason` = `"APIError"`
- `Status.SubReason` = `"AuthenticationError"`
- `Status.ConsecutiveFailures` = `0` (no retry for permanent errors)
- Metric recorded: `aianalysis_failures_total{reason="APIError",sub_reason="AuthenticationError"}` = 1
- NO requeue (permanent failure)

**Related Code**:
- `pkg/aianalysis/handlers/error_classifier.go:76` - ClassifyError()
- `pkg/aianalysis/handlers/investigating.go:184` - handleError()

---

#### AA-INT-ERR-007: Classify 429 Too Many Requests as RateLimit Error
**Description**: When HAPI returns 429 rate limit, system should classify as rate limit error and retry with backoff.

**Test Steps**:
1. Configure mock HAPI to return 429 with body: `{"error": "429 Too Many Requests"}`
2. Create AIAnalysis CRD with valid spec
3. Wait for first retry
4. Configure mock HAPI to return 200 (success)
5. Wait for successful completion

**Expected Results**:
- First attempt: `Status.ConsecutiveFailures` = 1
- First attempt: `Status.Reason` = `"TransientError"`
- First attempt: `Status.SubReason` = `"RateLimitExceeded"` or `"TransientError"`
- Requeue duration: ~5s base delay + jitter
- Second attempt: `Status.Phase` = `"Analyzing"` (success)
- Metric recorded: `aianalysis_failures_total{reason="TransientError",sub_reason="RateLimitExceeded"}` = 1

**Related Code**:
- `pkg/aianalysis/handlers/error_classifier.go:96` - Rate limit detection
- `pkg/aianalysis/handlers/investigating.go:186` - Retry with backoff

---

#### AA-INT-ERR-012: Classify context.DeadlineExceeded as Transient
**Description**: When HAPI call times out (context deadline exceeded), system should retry with backoff.

**Test Steps**:
1. Configure mock HAPI with 10s delay (exceeds 5s timeout)
2. Create AIAnalysis CRD with valid spec
3. Wait for timeout and first retry
4. Configure mock HAPI to respond within 1s
5. Wait for successful completion

**Expected Results**:
- First attempt: `Status.ConsecutiveFailures` = 1
- Error message contains: "context deadline exceeded"
- Retry with backoff (~5s)
- Second attempt: Success
- Metric recorded: `aianalysis_failures_total{reason="TransientError"}` = 1

**Related Code**:
- `pkg/aianalysis/handlers/error_classifier.go:63` - DeadlineExceeded check
- `pkg/aianalysis/handlers/investigating.go:213` - Backoff calculation

---

#### AA-INT-ERR-013: Classify context.Canceled as Permanent (NOT transient)
**Description**: When context is canceled (user/system initiated), should NOT retry.

**Test Steps**:
1. Create AIAnalysis CRD
2. Delete AIAnalysis during HAPI call (triggers context cancellation)
3. Verify no retry occurs

**Expected Results**:
- Error: "context canceled"
- NO retry (isTransientError returns false for Canceled)
- Phase transitions to terminal state
- `Status.ConsecutiveFailures` = 0

**Related Code**:
- `pkg/aianalysis/handlers/error_classifier.go:58` - Context canceled check

---

### Retry Logic Tests

**File**: `test/integration/aianalysis/retry_logic_test.go`

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-RETRY-001** | Max retries exceeded transitions to Failed | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-002** | Exponential backoff: Attempt 1 = ~5s base delay | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-003** | Exponential backoff: Attempt 2 = ~10s (2x) | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-004** | Exponential backoff: Attempt 3 = ~20s (4x) | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-005** | Exponential backoff: Max cap at 60s | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-006** | Backoff includes jitter (±10%) | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-007** | Retry count persisted in annotations | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-008** | Retry count increments across reconciliations | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-009** | Permanent errors do NOT retry (immediate fail) | BR-AI-010 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-010** | Transient errors DO retry with backoff | BR-AI-009 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-011** | MaxRetriesExceeded metric recorded correctly | DD-METRIC-001 | 🔴 Critical | ⏸️ Pending |
| **AA-INT-RETRY-012** | ConsecutiveFailures resets on success | BR-AI-009 | 🔴 Critical | ⏸️ Pending |

### Test Details

#### AA-INT-RETRY-001: Max retries exceeded transitions to Failed
**Description**: After 3 failed retry attempts for transient errors, system should transition to permanent failure.

**Test Steps**:
1. Configure mock HAPI to always return 503 (transient error)
2. Create AIAnalysis CRD
3. Wait for 4 reconciliation attempts (initial + 3 retries)
4. Verify final state

**Expected Results**:
- Attempt 1: `Status.ConsecutiveFailures` = 1, Requeue = ~5s
- Attempt 2: `Status.ConsecutiveFailures` = 2, Requeue = ~10s
- Attempt 3: `Status.ConsecutiveFailures` = 3, Requeue = ~20s
- Attempt 4: `Status.ConsecutiveFailures` = 4 (exceeds MaxRetries=3)
- Final: `Status.Phase` = `"Failed"`
- Final: `Status.Reason` = `"APIError"`
- Final: `Status.SubReason` = `"MaxRetriesExceeded"`
- Final: `Status.Message` contains "exceeded max retries (4 attempts)"
- Metric: `aianalysis_failures_total{sub_reason="MaxRetriesExceeded"}` = 1
- NO further requeue

**Related Code**:
- `pkg/aianalysis/handlers/investigating.go:191` - Max retries check
- `pkg/aianalysis/handlers/investigating.go:208` - MaxRetriesExceeded metric

**MaxRetries Constant**: 3 (defined in investigating.go)

---

#### AA-INT-RETRY-002: Exponential backoff: Attempt 1 = ~5s base delay
**Description**: First retry should use base delay (~5s) with jitter.

**Test Steps**:
1. Configure mock HAPI to return 503 on first call, 200 on second
2. Create AIAnalysis CRD
3. Record timestamp of first failure
4. Record timestamp of first retry
5. Calculate actual backoff duration

**Expected Results**:
- `ctrl.Result.RequeueAfter` ≈ 5s ± 10% (4.5s - 5.5s range due to jitter)
- Actual retry occurs within expected window
- `Status.ConsecutiveFailures` = 1

**Related Code**:
- `pkg/aianalysis/handlers/investigating.go:213` - CalculateWithDefaults()
- `internal/backoff/backoff.go` - Backoff calculation

**BaseDelay Constant**: 5 seconds

---

#### AA-INT-RETRY-007: Retry count persisted in annotations
**Description**: Retry count should be stored in AIAnalysis annotations for visibility and state preservation.

**Test Steps**:
1. Configure mock HAPI to return 503 (transient)
2. Create AIAnalysis CRD
3. Wait for first retry
4. Read AIAnalysis annotations

**Expected Results**:
- Annotation exists: `aianalysis.kubernaut.ai/retry-count`
- After attempt 1: value = `"1"`
- After attempt 2: value = `"2"`
- After attempt 3: value = `"3"`
- Annotation persists across reconciliations

**Related Code**:
- `pkg/aianalysis/handlers/investigating.go:260` - getRetryCount()
- `pkg/aianalysis/handlers/investigating.go:276` - setRetryCount()

**Annotation Key**: `aianalysis.kubernaut.ai/retry-count`

---

#### AA-INT-RETRY-009: Permanent errors do NOT retry (immediate fail)
**Description**: Validation errors (400), authentication errors (401), etc. should fail immediately without retry.

**Test Steps**:
1. Configure mock HAPI to return 400 with validation error
2. Create AIAnalysis CRD
3. Wait for reconciliation
4. Verify no retry occurs

**Expected Results**:
- Single reconciliation attempt
- `Status.ConsecutiveFailures` = 0 (no increment for permanent errors)
- `Status.Phase` = `"Failed"` immediately
- `Status.Reason` = `"APIError"`
- `Status.SubReason` = `"ValidationError"` or `"PermanentError"`
- NO requeue (ctrl.Result.RequeueAfter = 0)
- Metric: `aianalysis_failures_total{sub_reason="PermanentError"}` = 1

**Related Code**:
- `pkg/aianalysis/handlers/investigating.go:234` - Permanent error handling

---

---

## 🟡 PHASE 2: CONTROLLER ORCHESTRATION (Priority: HIGH)

**Target Coverage**: 80-85%
**Estimated Tests**: 10-15
**Effort**: Week 2
**File**: `test/integration/aianalysis/controller_edge_cases_test.go`

### Controller Edge Cases

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-CTRL-001** | Finalizer cleanup before deletion | BR-AI-001 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-002** | Deletion audit event generation | DD-AUDIT-003 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-003** | Status update conflict retry (409) | BR-AI-001 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-004** | Phase transition: New → Investigating | BR-AI-001 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-005** | Phase transition: Investigating → Analyzing | BR-AI-001 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-006** | Phase transition: Analyzing → Approved (production) | BR-AI-013 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-007** | Phase transition: Analyzing → Complete (non-production) | BR-AI-001 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-008** | Phase transition: Investigating → Failed (error) | BR-AI-009 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-009** | Audit event at each phase transition | DD-AUDIT-003 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-010** | CompletedAt timestamp set on terminal states | CRD Schema | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-011** | Graceful shutdown during deletion | BR-AI-001 | 🟡 High | ⏸️ Pending |
| **AA-INT-CTRL-012** | Concurrent reconciliation handling | BR-AI-001 | 🟡 High | ⏸️ Pending |

### Test Details

#### AA-INT-CTRL-001: Finalizer cleanup before deletion
**Description**: When AIAnalysis is deleted, finalizer should be removed before resource deletion completes.

**Test Steps**:
1. Create AIAnalysis CRD
2. Verify finalizer exists: `aianalysis.kubernaut.ai/finalizer`
3. Delete AIAnalysis
4. Verify finalizer is removed
5. Verify resource is fully deleted

**Expected Results**:
- Initial state: `.metadata.finalizers` contains `"aianalysis.kubernaut.ai/finalizer"`
- During deletion: Reconciler processes deletion
- Finalizer removed: `.metadata.finalizers` = `[]`
- Resource deleted: GET returns NotFound

**Related Code**:
- `internal/controller/aianalysis/aianalysis_controller.go:179` - Deletion handling

**Finalizer Name**: `aianalysis.kubernaut.ai/finalizer`

---

#### AA-INT-CTRL-003: Status update conflict retry (409)
**Description**: When status update conflicts with another update, controller should retry.

**Test Steps**:
1. Create AIAnalysis CRD
2. Simulate concurrent status update (trigger 409 conflict)
3. Verify controller retries status update
4. Verify final status is correct

**Expected Results**:
- First update: Returns 409 Conflict
- Controller logs: "Status update conflict, retrying"
- Second update: Success (200 OK)
- Final status reflects latest changes

**Related Code**:
- `internal/controller/aianalysis/aianalysis_controller.go:260` - Status update handling

---

#### AA-INT-CTRL-004: Phase transition: New → Investigating
**Description**: New AIAnalysis should transition to Investigating phase when reconciliation starts.

**Test Steps**:
1. Create AIAnalysis CRD (Phase defaults to empty/"")
2. Wait for reconciliation
3. Verify phase transition

**Expected Results**:
- Initial: `Status.Phase` = `""` or `"New"`
- After reconciliation: `Status.Phase` = `"Investigating"`
- `Status.Message` = "Starting analysis investigation"
- Audit event recorded: `event_type="PhaseTransition"`, `phase="Investigating"`

**Related Code**:
- `internal/controller/aianalysis/aianalysis_controller.go:92` - Reconcile()
- `pkg/aianalysis/handlers/investigating.go:74` - Handle()

---

---

## 🟢 PHASE 3: COMPREHENSIVE COVERAGE (Priority: MEDIUM)

**Target Coverage**: 85-90%
**Estimated Tests**: 5-10
**Effort**: Week 3

### Condition Management

**File**: `test/integration/aianalysis/conditions_test.go`

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-COND-001** | GetCondition returns existing condition | CRD Schema | 🟢 Medium | ⏸️ Pending |
| **AA-INT-COND-002** | GetCondition returns nil for non-existent condition | CRD Schema | 🟢 Medium | ⏸️ Pending |
| **AA-INT-COND-003** | SetInvestigationComplete updates condition | BR-AI-001 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-COND-004** | SetAnalysisComplete updates condition | BR-AI-001 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-COND-005** | SetWorkflowResolved updates condition | BR-AI-001 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-COND-006** | Condition timestamps update on transition | CRD Schema | 🟢 Medium | ⏸️ Pending |

### Test Details

#### AA-INT-COND-001: GetCondition returns existing condition
**Description**: GetCondition should return the condition when it exists in status.

**Test Steps**:
1. Create AIAnalysis with condition: `InvestigationComplete=True`
2. Call `GetCondition("InvestigationComplete")`
3. Verify returned condition

**Expected Results**:
- Function returns non-nil condition
- `condition.Type` = `"InvestigationComplete"`
- `condition.Status` = `"True"`
- `condition.LastTransitionTime` is set

**Related Code**:
- `pkg/aianalysis/conditions.go:84` - GetCondition()

---

### Error Type Constructors

**File**: `test/integration/aianalysis/error_types_test.go`

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-ERR-TYPE-001** | NewTransientError creates correct error type | BR-AI-009 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-ERR-TYPE-002** | NewPermanentError creates correct error type | BR-AI-010 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-ERR-TYPE-003** | NewValidationError creates correct error type | BR-AI-009 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-ERR-TYPE-004** | Error.Unwrap() returns wrapped error | BR-AI-009 | 🟢 Medium | ⏸️ Pending |
| **AA-INT-ERR-TYPE-005** | Error chain preserved through wrapping | BR-AI-009 | 🟢 Medium | ⏸️ Pending |

### Test Details

#### AA-INT-ERR-TYPE-001: NewTransientError creates correct error type
**Description**: NewTransientError should create a typed error that can be detected by isTransientError().

**Test Steps**:
1. Create error: `err := NewTransientError("service unavailable")`
2. Check type: `isTransientError(err)`
3. Verify error message

**Expected Results**:
- `isTransientError(err)` returns `true`
- `err.Error()` contains "service unavailable"
- Can be unwrapped with `errors.Unwrap()`

**Related Code**:
- `pkg/aianalysis/handler.go:168` - NewTransientError()
- `pkg/aianalysis/handlers/error_classifier.go:56` - isTransientError()

---

---

## 🟢 PHASE 4: V1.0 MATURITY COMPLIANCE (Priority: MANDATORY)

**Target**: V1.0 Production Readiness
**Estimated Tests**: 8-12
**Effort**: 2-3 days
**File**: `test/integration/aianalysis/v1_maturity_test.go`
**Authority**: TESTING_GUIDELINES.md Lines 1368-1806

### V1.0 Maturity Requirements

**MANDATORY**: All services must have tests that verify V1.0 maturity features. A service without these tests is **NOT** considered production-ready.

### Metrics Testing (DD-METRICS-001)

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-METRICS-001** | Record reconciliation metrics via registry inspection | DD-METRICS-001 | 🟢 Mandatory | ⏸️ Pending |
| **AA-INT-METRICS-002** | Record failure metrics with correct labels | DD-METRICS-001 | 🟢 Mandatory | ⏸️ Pending |
| **AA-INT-METRICS-003** | Record phase transition duration metrics | DD-METRICS-001 | 🟢 Mandatory | ⏸️ Pending |

### Audit Trace Validation (DD-AUDIT-003)

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-AUDIT-001** | Validate analysis_completed audit trace ALL fields | DD-AUDIT-003 | 🟢 Mandatory | ⏸️ Pending |
| **AA-INT-AUDIT-002** | Validate phase_transition audit trace ALL fields | DD-AUDIT-003 | 🟢 Mandatory | ⏸️ Pending |
| **AA-INT-AUDIT-003** | Validate analysis_failed audit trace ALL fields | DD-AUDIT-003 | 🟢 Mandatory | ⏸️ Pending |

### Graceful Shutdown (DD-007)

| Test ID | Test Scenario | Related BRs | Priority | Status |
|---------|--------------|-------------|----------|--------|
| **AA-INT-SHUTDOWN-001** | Verify audit store flush on SIGTERM | DD-007 | 🟢 Mandatory | ⏸️ Pending |

### Test Details

#### AA-INT-METRICS-001: Record reconciliation metrics via registry inspection

**Description**: When AIAnalysis reconciles, metrics MUST be recorded and verifiable via Prometheus registry inspection (NOT HTTP endpoint - integration tests use envtest with NO HTTP server).

**Authority**: TESTING_GUIDELINES.md Lines 473-528, 1490-1537

**Test Steps**:
1. Create test-specific Prometheus registry (DD-METRICS-001)
2. Initialize metrics with test registry: `testMetrics := metrics.NewMetricsWithRegistry(testRegistry)`
3. Create AIAnalysis CRD
4. Wait for reconciliation completion with `Eventually()`
5. Verify metrics via registry inspection (NOT HTTP)

**Expected Results**:
- Metric exists: `aianalysis_reconciler_reconciliations_total{phase="Investigating", result="success"}` > 0
- Metric exists: `aianalysis_reconciler_duration_seconds{phase="Investigating"}` > 0
- All metrics have correct label dimensions
- NO HTTP server started (integration tests use envtest)

**Pattern** (MANDATORY):
```go
It("AA-INT-METRICS-001: should record reconciliation metrics via registry inspection", func() {
    By("Creating test-specific registry (DD-METRICS-001)")
    testRegistry := prometheus.NewRegistry()
    testMetrics := metrics.NewMetricsWithRegistry(testRegistry)

    By("Creating AIAnalysis CRD")
    analysis := createTestAIAnalysis("test-metrics", namespace)
    Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

    By("Waiting for reconciliation with Eventually()")
    Eventually(func() string {
        var updated aianalysisv1.AIAnalysis
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
        return updated.Status.Phase
    }, 30*time.Second, 1*time.Second).Should(Equal("Analyzing"))

    By("Verifying metrics via registry inspection (NOT HTTP)")
    families, err := testRegistry.Gather()
    Expect(err).ToNot(HaveOccurred())

    found := false
    for _, family := range families {
        if family.GetName() == "aianalysis_reconciler_reconciliations_total" {
            found = true
            // Verify label values
            for _, metric := range family.GetMetric() {
                labels := make(map[string]string)
                for _, label := range metric.GetLabel() {
                    labels[label.GetName()] = label.GetValue()
                }
                if labels["phase"] == "Investigating" && labels["result"] == "success" {
                    Expect(metric.GetCounter().GetValue()).To(BeNumerically(">", 0))
                }
            }
        }
    }
    Expect(found).To(BeTrue(), "Metric aianalysis_reconciler_reconciliations_total not found in registry")
})
```

**Related Code**:
- `pkg/aianalysis/metrics/metrics.go` - Metrics definitions
- `internal/controller/aianalysis/aianalysis_controller.go` - Metric recording

**Critical**: Integration tests use **registry inspection** (envtest has NO HTTP server). E2E tests will use `/metrics` HTTP endpoint.

---

#### AA-INT-AUDIT-001: Validate analysis_completed audit trace ALL fields

**Description**: When AIAnalysis completes successfully, audit trace MUST have ALL required fields validated via **OpenAPI client** (MANDATORY per TESTING_GUIDELINES.md).

**Authority**: TESTING_GUIDELINES.md Lines 1593-1678

**Test Steps**:
1. Setup OpenAPI audit client (MANDATORY): `dsgen.NewAPIClient(cfg)`
2. Create AIAnalysis CRD with valid spec
3. Wait for Phase = "Complete" with `Eventually()`
4. Query DataStorage audit API via OpenAPI client
5. Validate **ALL fields** (no fields skipped)

**Expected Results** (ALL FIELDS REQUIRED):
- `event.Service` = `"aianalysis"`
- `event.EventType` = `"analysis_completed"`
- `event.EventCategory` = `dsgen.AuditEventRequestEventCategory("aianalysis")` (enum type, NOT string)
- `event.CorrelationId` = `string(analysis.UID)`
- `event.Severity` = `"info"`
- `eventData["analysis_id"]` = `analysis.Name`
- `eventData["phase"]` = `"Complete"`
- `eventData["selected_workflow"]` = workflow ID (if applicable)
- `eventData["duration_seconds"]` > 0
- ALL other fields per DD-AUDIT-003

**Pattern** (MANDATORY):
```go
It("AA-INT-AUDIT-001: should emit audit trace with ALL required fields", func() {
    By("Setting up OpenAPI audit client (MANDATORY)")
    cfg := dsgen.NewConfiguration()
    cfg.Servers = []dsgen.ServerConfiguration{{URL: dataStorageURL}}
    auditClient := dsgen.NewAPIClient(cfg)

    By("Creating AIAnalysis CRD")
    analysis := createTestAIAnalysis("test-audit", namespace)
    analysis.Spec.Signal.Severity = "critical"
    Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

    By("Waiting for completion with Eventually()")
    Eventually(func() string {
        var updated aianalysisv1.AIAnalysis
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
        return updated.Status.Phase
    }, 30*time.Second, 1*time.Second).Should(Equal("Complete"))

    By("Querying audit events via OpenAPI client with Eventually()")
    var event dsgen.AuditEventResponse
    Eventually(func() bool {
        events, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
            Service("aianalysis").
            CorrelationId(string(analysis.UID)).
            Execute()

        if err != nil || len(events.Events) == 0 {
            return false
        }

        event = events.Events[0]
        return true
    }, 30*time.Second, 2*time.Second).Should(BeTrue())

    By("Validating ALL audit fields (NO FIELDS SKIPPED)")
    // ✅ REQUIRED: Validate ALL fields
    Expect(event.Service).To(Equal("aianalysis"))
    Expect(event.EventType).To(Equal("analysis_completed"))
    Expect(event.EventCategory).To(Equal(dsgen.AuditEventRequestEventCategory("aianalysis")))
    Expect(event.CorrelationId).To(Equal(string(analysis.UID)))
    Expect(event.Severity).To(Equal("info"))

    // Validate event data fields
    eventData, ok := event.EventData.(map[string]interface{})
    Expect(ok).To(BeTrue())
    Expect(eventData["analysis_id"]).To(Equal(analysis.Name))
    Expect(eventData["phase"]).To(Equal("Complete"))
    Expect(eventData["signal_name"]).To(Equal(analysis.Spec.Signal.Name))
    Expect(eventData["severity"]).To(Equal("critical"))
    Expect(eventData["duration_seconds"]).To(BeNumerically(">", 0))
    // ... ALL other fields per DD-AUDIT-003
})
```

**Related Code**:
- `pkg/aianalysis/audit/audit.go:120` - RecordAnalysisComplete()
- OpenAPI client: `pkg/datastorage/client/generated/` (dsgen)

**Validation Checklist** (Lines 1665-1678):
- [ ] OpenAPI client used (MANDATORY)
- [ ] ALL fields validated (no fields skipped)
- [ ] EventCategory uses enum type (not string)
- [ ] eventData structure matches schema
- [ ] Error scenarios tested (analysis_failed audit trace)

---

#### AA-INT-SHUTDOWN-001: Verify audit store flush on SIGTERM

**Description**: When controller receives SIGTERM, audit store MUST flush pending events before shutdown (DD-007).

**Authority**: TESTING_GUIDELINES.md Lines 1733-1755

**Test Steps**:
1. Mock audit store with flush tracking
2. Mock manager with SIGTERM simulation
3. Run main function logic with mocks
4. Verify `Close()` called on audit store

**Expected Results**:
- `auditStore.Close()` called before process exit
- Audit buffer flushed to DataStorage
- No events lost during shutdown

**Pattern**:
```go
It("AA-INT-SHUTDOWN-001: should flush audit store on SIGTERM", func() {
    By("Setting up mocks")
    mockAuditStore := &mockAuditStore{}
    mockManager := &mockManager{
        startFunc: func(ctx context.Context) error {
            // Simulate receiving SIGTERM
            <-ctx.Done()
            return nil
        },
    }

    By("Running main function logic with mocks")
    runMainWithMocks(mockManager, mockAuditStore)

    By("Verifying audit store Close() was called")
    Expect(mockAuditStore.closeCalled).To(BeTrue(),
        "Audit store MUST be closed on SIGTERM per DD-007")
})
```

**Related Code**:
- `cmd/aianalysis/main.go` - Graceful shutdown logic
- `pkg/aianalysis/audit/audit.go` - Audit store interface

---

### V1.0 Maturity Compliance Checklist

Use this checklist to verify V1.0 production readiness:

- [ ] **AA-INT-METRICS-001**: Metrics recorded via registry inspection ✅/❌
- [ ] **AA-INT-METRICS-002**: Failure metrics with correct labels ✅/❌
- [ ] **AA-INT-METRICS-003**: Duration metrics recorded ✅/❌
- [ ] **AA-INT-AUDIT-001**: analysis_completed audit trace ALL fields validated ✅/❌
- [ ] **AA-INT-AUDIT-002**: phase_transition audit trace ALL fields validated ✅/❌
- [ ] **AA-INT-AUDIT-003**: analysis_failed audit trace ALL fields validated ✅/❌
- [ ] **AA-INT-SHUTDOWN-001**: Graceful shutdown flushes audit ✅/❌

**Status**: ALL checkboxes MUST be ✅ before V1.0 release

---

## 📊 COVERAGE TRACKING

### Coverage Targets (Per TESTING_GUIDELINES.md)

**Code Coverage** (Cumulative Defense-in-Depth):
- **Unit Tests**: 70%+ (current: 70.0% ✅)
- **Integration Tests**: 50% target (current: 54.6% ✅ **EXCEEDS TARGET**)
- **E2E Tests**: 50% target (not yet measured)

**Interpretation**: AIAnalysis integration tests achieve 54.6% coverage, **EXCEEDING** the 50% guideline target. The new tests focus on **uncovered critical paths** (error handling, retry logic, V1.0 maturity) rather than increasing overall coverage percentage.

**BR Coverage** (Overlapping - Same BRs tested at multiple tiers):
- **Unit Tests**: 70%+ of BRs (validated via test-to-BR mapping)
- **Integration Tests**: >50% of BRs (same BRs tested at multiple tiers)
- **E2E Tests**: <10% of BRs (critical user journeys)

### Overall Progress

| Phase | Tests Planned | Tests Implemented | Coverage Target | Current Coverage | Focus |
|-------|--------------|-------------------|-----------------|------------------|-------|
| **Baseline** | 53 | 53 | 54.6% | **54.6%** ✅ | Existing tests |
| **Phase 1** | 30 | 0 | 60-65% | 54.6% ⏸️ | Error handling critical paths |
| **Phase 2** | 12 | 0 | 65-70% | 54.6% ⏸️ | Controller edge cases |
| **Phase 3** | 11 | 0 | 70%+ | 54.6% ⏸️ | Comprehensive coverage |
| **Phase 4** | 8 | 0 | 70%+ | 54.6% ⏸️ | **V1.0 Maturity** (metrics, audit, shutdown) |
| **Total** | **114** | **53** | **70%+** | **54.6%** | Critical paths + V1.0 compliance |

**Key Change**: Phase targets adjusted to reflect that **50% integration coverage is sufficient** per guidelines. Additional tests focus on **critical paths** (error handling, retry logic) and **V1.0 maturity compliance** (metrics, audit, shutdown), not just code coverage percentage.

### Coverage by Component

| Component | Current | Phase 1 Target | Phase 2 Target | Phase 3 Target | Phase 4 Target |
|-----------|---------|---------------|----------------|----------------|----------------|
| Error Classifier | 0% | **90%** 🔴 | 95% | 95% | 95% |
| Retry Logic | 0% | **85%** 🔴 | 90% | 90% | 90% |
| Controller | 45-82% | 60-85% | **90%** 🟡 | 90% | 90% |
| Conditions | 0-75% | 0-75% | 40-80% | **85%** 🟢 | 85% |
| Error Types | 0% | 0% | 50% | **90%** 🟢 | 90% |
| Audit | 90-94% | 90-94% | 92-95% | 95% | **95%** ✅ (OpenAPI validation) |
| **Metrics** | **0%** | **0%** | **0%** | **0%** | **90%** 🆕 (V1.0 maturity) |
| **Graceful Shutdown** | **0%** | **0%** | **0%** | **0%** | **90%** 🆕 (V1.0 maturity) |

### Test Status Legend
- ✅ **Implemented**: Test exists and passing
- 🚧 **In Progress**: Test being developed
- ⏸️ **Pending**: Not yet started
- ❌ **Failed**: Test exists but failing
- 🔄 **Refactoring**: Test exists but needs updates

---

## 🎯 TEST IMPLEMENTATION ORDER

### Week 1 (Phase 1 - Critical Error Handling)

**Days 1-2: Error Classification (AA-INT-ERR-001 to AA-INT-ERR-018)**
```bash
# Create test file
touch test/integration/aianalysis/error_classification_test.go

# Implement in order:
# - HTTP status codes (001-011): 2 days
# - Context errors (012-013): 0.5 day
# - Network errors (014-016): 0.5 day
# - Edge cases (017-018): 0.5 day
```

**Days 3-5: Retry Logic (AA-INT-RETRY-001 to AA-INT-RETRY-012)**
```bash
# Create test file
touch test/integration/aianalysis/retry_logic_test.go

# Implement in order:
# - Max retries (001): 0.5 day
# - Backoff calculation (002-006): 1.5 days
# - Annotations (007-008): 0.5 day
# - Retry behavior (009-012): 1 day
```

### Week 2 (Phase 2 - Controller Orchestration)

**Days 1-3: Controller Edge Cases (AA-INT-CTRL-001 to AA-INT-CTRL-012)**
```bash
# Create test file
touch test/integration/aianalysis/controller_edge_cases_test.go

# Implement in order:
# - Deletion handling (001-002): 1 day
# - Status conflicts (003): 0.5 day
# - Phase transitions (004-010): 1.5 days
# - Concurrency (011-012): 0.5 day
```

### Week 3 (Phase 3 - Comprehensive Coverage)

**Days 1-2: Condition Management (AA-INT-COND-001 to AA-INT-COND-006)**
```bash
# Create test file
touch test/integration/aianalysis/conditions_test.go

# Implement: 2 days
```

**Day 3: Error Type Constructors (AA-INT-ERR-TYPE-001 to AA-INT-ERR-TYPE-005)**
```bash
# Create test file
touch test/integration/aianalysis/error_types_test.go

# Implement: 1 day
```

### 🆕 Week 4 (Phase 4 - V1.0 Maturity Compliance) ⭐ MANDATORY

**Days 1-2: Metrics Testing (AA-INT-METRICS-001 to AA-INT-METRICS-003)**
```bash
# Create test file
touch test/integration/aianalysis/v1_maturity_test.go

# Implement:
# - AA-INT-METRICS-001: Registry inspection pattern: 0.5 day
# - AA-INT-METRICS-002: Failure metrics with labels: 0.5 day
# - AA-INT-METRICS-003: Duration metrics: 0.5 day
```

**Days 2-3: Audit Trace Validation (AA-INT-AUDIT-001 to AA-INT-AUDIT-003)**
```bash
# Continue in test/integration/aianalysis/v1_maturity_test.go

# Implement:
# - AA-INT-AUDIT-001: analysis_completed ALL fields: 0.5 day
# - AA-INT-AUDIT-002: phase_transition ALL fields: 0.5 day
# - AA-INT-AUDIT-003: analysis_failed ALL fields: 0.5 day
```

**Day 4: Graceful Shutdown (AA-INT-SHUTDOWN-001)**
```bash
# Continue in test/integration/aianalysis/v1_maturity_test.go

# Implement:
# - AA-INT-SHUTDOWN-001: Audit flush on SIGTERM: 0.5 day
```

**Day 4 (Afternoon): V1.0 Sign-Off**
- Run full integration test suite
- Verify all V1.0 maturity checklist items
- Generate coverage report
- Document compliance status

---

## 📝 TEST WRITING GUIDELINES

### MANDATORY Anti-Patterns (TESTING_GUIDELINES.md)

#### ❌ Skip() is ABSOLUTELY FORBIDDEN (Lines 855-985)

**NEVER use Skip()** when services are unavailable:

```go
// ❌ FORBIDDEN: Skipping when HAPI unavailable
BeforeEach(func() {
    if !hapiAvailable {
        Skip("HAPI not running")  // ← ABSOLUTELY FORBIDDEN
    }
})

// ✅ REQUIRED: Fail with clear error message
BeforeEach(func() {
    resp, err := http.Get(hapiURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf(
            "REQUIRED: HAPI not available at %s\n"+
            "  Per DD-TEST-002: Infrastructure is auto-started in SynchronizedBeforeSuite\n"+
            "  If HAPI failed to start, check test/infrastructure/datastorage_bootstrap.go logs\n"+
            "  Error: %v",
            hapiURL, err))
    }
})
```

**Rationale**: If a service can run without HAPI, then HAPI is optional. Integration tests MUST fail when required services are unavailable.

---

#### ❌ time.Sleep() is ABSOLUTELY FORBIDDEN (Lines 573-852)

**NEVER use time.Sleep()** for waiting on asynchronous operations:

```go
// ❌ FORBIDDEN: time.Sleep() before assertions
time.Sleep(5 * time.Second)
Expect(analysis.Status.Phase).To(Equal("Failed"))

// ✅ REQUIRED: Eventually() with timeout and interval
Eventually(func() string {
    var updated aianalysisv1.AIAnalysis
    _ = k8sClient.Get(ctx, key, &updated)
    return updated.Status.Phase
}, 30*time.Second, 1*time.Second).Should(Equal("Failed"))
```

**Integration Test Timeouts** (Lines 696-719):
- **Timeout**: 30-60 seconds (real K8s API is slower)
- **Interval**: 1-2 seconds (reasonable polling frequency)

**Acceptable time.Sleep() Usage** (ONLY for timing behavior tests):
```go
// ✅ Acceptable: Testing backoff duration
start := time.Now()
// trigger retry logic
duration := time.Since(start)
Expect(duration).To(BeNumerically("~", 5*time.Second, 500*time.Millisecond))
```

**Rule**: `time.Sleep()` is ONLY acceptable when testing timing behavior itself, NEVER for waiting on asynchronous operations.

---

### Test Structure Template

```go
var _ = Describe("Error Classification - AA-INT-ERR", Label("integration", "error_handling"), func() {
    Context("HTTP Status Code Classification", func() {
        It("AA-INT-ERR-001: should classify 401 as authentication error", func() {
            By("Configuring mock HAPI to return 401")
            // Setup code

            By("Creating AIAnalysis CRD")
            analysis := createTestAIAnalysis("test-auth-error", namespace)
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Waiting for reconciliation with Eventually()")
            // ✅ REQUIRED: Use Eventually(), NEVER time.Sleep()
            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
                return updated.Status.Phase
            }, 30*time.Second, 1*time.Second).Should(Equal("Failed"))

            By("Verifying error classification")
            var final aianalysisv1.AIAnalysis
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &final)).To(Succeed())
            Expect(final.Status.Reason).To(Equal("APIError"))
            Expect(final.Status.SubReason).To(Equal("AuthenticationError"))

            By("Verifying metrics recorded with Eventually()")
            Eventually(func() float64 {
                return testutil.GetMetricValue(testMetrics, "aianalysis_failures_total",
                    prometheus.Labels{"sub_reason": "AuthenticationError"})
            }, 10*time.Second, 1*time.Second).Should(Equal(1.0))
        })
    })
})
```

### Assertion Patterns (Per TESTING_GUIDELINES.md)

```go
// ✅ Pattern 1: Simple status field assertions
Expect(analysis.Status.Phase).To(Equal("Failed"))
Expect(analysis.Status.Reason).To(Equal("APIError"))
Expect(analysis.Status.SubReason).To(Equal("MaxRetriesExceeded"))

// ✅ Pattern 2: Async operations - ALWAYS use Eventually()
// Timeout: 30-60s for integration tests (real K8s API)
// Interval: 1-2s (reasonable polling frequency)
Eventually(func() string {
    var updated aianalysisv1.AIAnalysis
    _ = k8sClient.Get(ctx, key, &updated)  // Ignore error, Eventually will retry
    return updated.Status.Phase
}, 30*time.Second, 1*time.Second).Should(Equal("Failed"))

// ✅ Pattern 3: Metric validation via registry inspection
// (Integration tests use registry, NOT HTTP endpoint)
Eventually(func() float64 {
    families, _ := testRegistry.Gather()
    for _, family := range families {
        if family.GetName() == "aianalysis_failures_total" {
            for _, metric := range family.GetMetric() {
                // Find metric with matching labels
                labels := getLabels(metric)
                if labels["sub_reason"] == "MaxRetriesExceeded" {
                    return metric.GetCounter().GetValue()
                }
            }
        }
    }
    return 0
}, 10*time.Second, 1*time.Second).Should(Equal(1.0))

// ✅ Pattern 4: Audit event validation via OpenAPI client
Eventually(func() bool {
    events, _, err := auditClient.AuditAPI.QueryAuditEvents(ctx).
        Service("aianalysis").
        CorrelationId(string(analysis.UID)).
        Execute()

    if err != nil || len(events.Events) == 0 {
        return false
    }

    // Validate ALL fields (MANDATORY per TESTING_GUIDELINES.md)
    event := events.Events[0]
    return event.Service == "aianalysis" &&
           event.EventType == "analysis_completed" &&
           event.CorrelationId == string(analysis.UID)
}, 30*time.Second, 2*time.Second).Should(BeTrue())

// ✅ Pattern 5: Complex object search with Eventually()
Eventually(func() *aianalysisv1.AIAnalysis {
    var list aianalysisv1.AIAnalysisList
    if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
        return nil  // Return nil on error, Eventually will retry
    }

    for i := range list.Items {
        if list.Items[i].Status.Phase == "Failed" {
            return &list.Items[i]
        }
    }
    return nil
}, 30*time.Second, 1*time.Second).ShouldNot(BeNil())
```

### Mock HAPI Configuration

```go
// Return specific error
mockHAPI.SetResponse(503, `{"error": "Service Unavailable"}`)

// Return success after N failures
mockHAPI.SetResponseSequence([]Response{
    {StatusCode: 503, Body: `{"error": "Service Unavailable"}`},
    {StatusCode: 503, Body: `{"error": "Service Unavailable"}`},
    {StatusCode: 200, Body: validIncidentResponse},
})

// Simulate timeout
mockHAPI.SetDelay(10 * time.Second) // exceeds 5s timeout
```

---

## 🔍 VALIDATION CHECKLIST

### Before Marking Test as Complete

- [ ] Test ID matches format: `AA-INT-[CATEGORY]-[NUMBER]`
- [ ] Test description includes test ID
- [ ] Related BRs documented in test plan
- [ ] Test follows Given-When-Then or By() pattern
- [ ] Assertions use Eventually() for async operations
- [ ] Metrics validated if applicable
- [ ] Audit events validated if applicable
- [ ] Test passes consistently (no flakes)
- [ ] Coverage report shows improvement
- [ ] Code reviewed and approved

### Coverage Verification

```bash
# Run integration tests with coverage
make test-integration-aianalysis-coverage

# Check coverage report
go tool cover -func=coverage-integration-aianalysis.out | grep "error_classifier.go"
go tool cover -func=coverage-integration-aianalysis.out | grep "investigating.go"

# Generate HTML report
go tool cover -html=coverage-integration-aianalysis.out -o coverage-integration.html
```

---

## 📚 REFERENCES

### Related Documents
- **🆕 Guidelines Triage**: `docs/handoff/AA_TEST_PLAN_GUIDELINES_TRIAGE_DEC_24_2025.md` (Gap analysis vs TESTING_GUIDELINES.md)
- **Coverage Analysis**: `docs/handoff/AA_INTEGRATION_TEST_COVERAGE_GAPS_DEC_24_2025.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md` ⭐ AUTHORITATIVE
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`
- **Business Requirements**: `docs/requirements/BR-AI-*.md`

### Architecture Decisions
- **DD-TEST-002**: Integration Test Container Orchestration (Sequential startup)
- **DD-AUDIT-003**: AIAnalysis audit trace requirements
- **DD-METRICS-001**: Controller Metrics Wiring Pattern
- **DD-007**: Graceful Shutdown Requirements

### Code References
- **Error Classifier**: `pkg/aianalysis/handlers/error_classifier.go`
- **Retry Logic**: `pkg/aianalysis/handlers/investigating.go:184`
- **Controller**: `internal/controller/aianalysis/aianalysis_controller.go`
- **Conditions**: `pkg/aianalysis/conditions.go`
- **Error Types**: `pkg/aianalysis/handler.go`
- **Metrics**: `pkg/aianalysis/metrics/metrics.go`
- **Audit**: `pkg/aianalysis/audit/audit.go`

### Infrastructure References
- **Sequential Startup**: `test/infrastructure/datastorage_bootstrap.go`
- **Suite Setup**: `test/integration/aianalysis/suite_test.go`
- **Mock HAPI**: `holmesgpt-api/src/mock_responses.py`

---

## 📊 SUCCESS METRICS

### Phase 1 Success Criteria (Critical Error Handling)
- [ ] 18 error classification tests passing
- [ ] 12 retry logic tests passing
- [ ] Error classifier coverage ≥ 90%
- [ ] Retry logic coverage ≥ 85%
- [ ] Overall integration coverage: 60-65%
- [ ] Zero test flakes
- [ ] All metrics validated via registry inspection

### Phase 2 Success Criteria (Controller Orchestration)
- [ ] 12 controller edge case tests passing
- [ ] Controller coverage ≥ 90%
- [ ] Overall integration coverage: 65-70%
- [ ] All phase transitions validated
- [ ] Audit events validated at each transition

### Phase 3 Success Criteria (Comprehensive Coverage)
- [ ] 6 condition tests passing
- [ ] 5 error type tests passing
- [ ] Conditions coverage ≥ 85%
- [ ] Error types coverage ≥ 90%
- [ ] Overall integration coverage: 70%+
- [ ] All components ≥ 80% coverage

### 🆕 Phase 4 Success Criteria (V1.0 Maturity) ⭐ MANDATORY

**Production Readiness Checklist** (Per TESTING_GUIDELINES.md Lines 1763-1770):

#### Metrics Testing (DD-METRICS-001)
- [ ] **AA-INT-METRICS-001** passing: Reconciliation metrics via registry ✅/❌
- [ ] **AA-INT-METRICS-002** passing: Failure metrics with labels ✅/❌
- [ ] **AA-INT-METRICS-003** passing: Duration metrics ✅/❌
- [ ] Metrics coverage ≥ 90% ✅/❌
- [ ] All metrics use test-specific registry (NO HTTP in integration) ✅/❌

#### Audit Trace Validation (DD-AUDIT-003)
- [ ] **AA-INT-AUDIT-001** passing: analysis_completed ALL fields via OpenAPI ✅/❌
- [ ] **AA-INT-AUDIT-002** passing: phase_transition ALL fields via OpenAPI ✅/❌
- [ ] **AA-INT-AUDIT-003** passing: analysis_failed ALL fields via OpenAPI ✅/❌
- [ ] Audit coverage ≥ 95% ✅/❌
- [ ] OpenAPI client used for ALL audit validation (MANDATORY) ✅/❌
- [ ] EventCategory uses enum type (not string) ✅/❌

#### Graceful Shutdown (DD-007)
- [ ] **AA-INT-SHUTDOWN-001** passing: Audit flush on SIGTERM ✅/❌
- [ ] Graceful shutdown coverage ≥ 90% ✅/❌

#### V1.0 Production Readiness Sign-Off
- [ ] **ALL Phase 4 tests passing** (8 tests) ✅/❌
- [ ] **Zero V1.0 maturity gaps** ✅/❌
- [ ] **Integration test suite stable** (zero flakes) ✅/❌
- [ ] **Overall integration coverage**: 70%+ ✅/❌

**Status**: ALL checkboxes MUST be ✅ before V1.0 release

### Final Success Criteria (V1.0 Production-Ready)

#### Test Coverage
- [ ] **114 total tests passing** (53 baseline + 30 Phase 1 + 12 Phase 2 + 11 Phase 3 + 8 Phase 4)
- [ ] **70%+ integration coverage** (exceeds 50% guideline target)
- [ ] **Zero 0% coverage components**
- [ ] **All critical paths tested**

#### Guidelines Compliance (TESTING_GUIDELINES.md)
- [ ] **Eventually() used** for ALL async operations (no time.Sleep())
- [ ] **Fail() used** for missing services (no Skip())
- [ ] **DD-TEST-002** sequential startup pattern followed
- [ ] **Mock LLM policy** implemented (cost constraint)
- [ ] **Metrics via registry** inspection (NOT HTTP for integration)
- [ ] **Audit via OpenAPI** client (MANDATORY)

#### Production Readiness
- [ ] **V1.0 maturity compliance**: 100% (Phase 4 complete)
- [ ] **Production-ready confidence**: 95%+
- [ ] **Zero technical debt** in test infrastructure
- [ ] **Documentation complete**: Test plan, coverage analysis, handoff

**Final Sign-Off**: Service Owner + Tech Lead approval required

---

## 📝 VERSION HISTORY

### Version 1.1 (December 24, 2025) - Guidelines Compliance Update

**Changes Made**:
1. ✅ **Added Phase 4**: V1.0 Maturity Compliance (8 tests: metrics, audit, graceful shutdown)
2. ✅ **Updated Coverage Targets**: Aligned with 70%/50%/50% defense-in-depth guideline (was targeting 85-90%)
3. ✅ **Added Infrastructure Section**: DD-TEST-002 sequential startup + mock LLM policy documentation
4. ✅ **Added Anti-Patterns**: Skip() forbidden + time.Sleep() forbidden with mandatory Eventually() usage
5. ✅ **Added Metrics Testing**: Registry inspection pattern (NOT HTTP) for integration tests
6. ✅ **Added Audit Validation**: OpenAPI client MANDATORY, ALL fields validation required
7. ✅ **Updated Test Patterns**: Eventually() examples with correct timeouts (30-60s for integration)
8. ✅ **Updated Success Metrics**: Phase 4 V1.0 maturity checklist added

**Authority**: [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

**Triage Document**: [AA_TEST_PLAN_GUIDELINES_TRIAGE_DEC_24_2025.md](../../handoff/AA_TEST_PLAN_GUIDELINES_TRIAGE_DEC_24_2025.md)

**Critical Gaps Addressed**:
- Gap #1: Missing V1.0 maturity tests (metrics, audit, shutdown) → **Phase 4 added**
- Gap #2: Skip() not documented as forbidden → **Anti-patterns section added**
- Gap #3: Eventually() requirements missing → **Test patterns updated**
- Gap #4: Coverage target mismatch → **Aligned with 50% guideline target**
- Gap #5: DD-TEST-002 not referenced → **Infrastructure section added**
- Gap #6: Metrics testing tier mismatch → **Registry inspection pattern documented**
- Gap #7: Audit validation pattern missing → **OpenAPI client requirements added**
- Gap #8: Mock LLM policy missing → **Infrastructure section documents MOCK_LLM_MODE**

**Test Count Update**: 106 → 114 tests (added 8 V1.0 maturity tests)

**Compliance Score**: 65% → 100% (all critical gaps addressed)

### Version 1.0 (December 24, 2025) - Initial Release

**Initial Plan**:
- Phase 1: Error classification and retry logic (30 tests)
- Phase 2: Controller orchestration (12 tests)
- Phase 3: Comprehensive coverage (11 tests)
- Baseline: 53 existing tests
- Total: 106 tests targeting 85-90% coverage

---

**Test Plan Owner**: AIAnalysis Service Team
**Last Updated**: December 24, 2025 (V1.1 - Guidelines Compliance)
**Next Review**: After Phase 1 completion
**Status**: ✅ Ready for implementation (Guidelines-compliant) 🚀

**Implementation Readiness**:
- ✅ Guidelines compliance verified
- ✅ Anti-patterns documented (Skip(), time.Sleep())
- ✅ Infrastructure patterns documented (DD-TEST-002)
- ✅ V1.0 maturity requirements defined (Phase 4)
- ✅ Test patterns updated (Eventually(), OpenAPI audit client)
- ⏸️ Awaiting Phase 1 implementation start

