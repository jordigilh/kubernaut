# AIAnalysis Integration Test Coverage Gaps & Opportunities

**Date**: December 24, 2025
**Service**: AIAnalysis
**Current Integration Coverage**: 54.6% (53/53 tests passing)
**Analysis Scope**: Identify missing test scenarios for improved coverage

---

## 📊 COVERAGE SUMMARY

### Current State
- **Overall Integration Coverage**: 54.6%
- **Unit Test Coverage**: 70.0% (meets 70%+ requirement ✅)
- **Integration Tests**: 53 passing
- **Status**: Production Ready ✅

### Coverage Breakdown by Component

| Component | Coverage | Status | Priority |
|-----------|----------|--------|----------|
| **Controller** (`aianalysis_controller.go`) | 45-82% | 🟡 Moderate | **HIGH** |
| **Audit** (`audit/audit.go`) | 90-94% | ✅ Excellent | LOW |
| **Error Classifier** (`handlers/error_classifier.go`) | **0%** | ❌ Critical | **CRITICAL** |
| **Error Handling** (`handlers/investigating.go`) | **0-77%** | 🟡 Partial | **HIGH** |
| **Conditions** (`conditions.go`) | **0-75%** | 🟡 Partial | MEDIUM |
| **Error Types** (`handler.go`) | **0%** | ❌ Critical | MEDIUM |
| **Generated Helpers** (`handlers/generated_helpers.go`) | 0-83% | 🟡 Partial | LOW |

---

## 🚨 CRITICAL GAPS (0% Coverage)

### 1. Error Classification Logic (**CRITICAL PRIORITY**)

**File**: `pkg/aianalysis/handlers/error_classifier.go`
**Coverage**: **0%**
**Business Impact**: Error handling, retry logic, metrics accuracy

#### Missing Test Scenarios:

##### A. HTTP Status Code Classification
```go
// MISSING: Test 401/403 Authentication Errors
// Expected: ErrorCategoryAuthentication
// Scenarios: "401 Unauthorized", "403 Forbidden", "access denied"
```

```go
// MISSING: Test 400/422 Validation Errors
// Expected: ErrorCategoryValidation
// Scenarios: "400 Bad Request", "invalid parameter", "missing required field"
```

```go
// MISSING: Test 404 Not Found Errors
// Expected: ErrorCategoryNotFound
// Scenarios: "404 Not Found", "resource does not exist"
```

```go
// MISSING: Test 409 Conflict Errors
// Expected: ErrorCategoryConflict
// Scenarios: "409 Conflict", "already exists", "duplicate entry"
```

```go
// MISSING: Test 429 Rate Limiting
// Expected: ErrorCategoryRateLimit
// Scenarios: "429 Too Many Requests", "rate limit exceeded", "throttled"
```

```go
// MISSING: Test 5xx Server Errors
// Expected: ErrorCategoryTransient
// Scenarios: "500 Internal Server Error", "502 Bad Gateway", "503 Service Unavailable", "504 Gateway Timeout"
```

##### B. Network & Context Errors
```go
// MISSING: Test context.DeadlineExceeded (should be transient)
// MISSING: Test context.Canceled (should NOT be transient)
// MISSING: Test "connection refused" (transient)
// MISSING: Test "no such host" (permanent)
// MISSING: Test "TLS handshake timeout" (transient)
```

##### C. Edge Cases
```go
// MISSING: Test unknown/unclassified errors (default to permanent)
// MISSING: Test mixed error messages (multiple patterns)
// MISSING: Test case-insensitive pattern matching
```

#### Recommended Test Structure:
```go
// test/integration/aianalysis/error_classification_test.go
var _ = Describe("Error Classification - BR-AI-009", Label("integration", "error_handling"), func() {
    Context("HTTP Status Code Classification", func() {
        It("should classify 401/403 as authentication errors", func() {
            // Create AIAnalysis with mock HAPI returning 401
            // Verify Status.Reason = "APIError"
            // Verify Status.SubReason = "AuthenticationError"
            // Verify metric: aianalysis_failures_total{reason="APIError",sub_reason="AuthenticationError"}
        })

        It("should classify 400/422 as validation errors", func() {
            // Similar pattern for validation errors
        })

        It("should classify 429 as rate limit errors and retry", func() {
            // Verify exponential backoff is applied
            // Verify Status.ConsecutiveFailures incremented
        })

        It("should classify 5xx as transient errors and retry", func() {
            // Test 500, 502, 503, 504
            // Verify retry with backoff
        })
    })

    Context("Network Errors", func() {
        It("should classify deadline exceeded as transient", func() {
            // Verify retry behavior
        })

        It("should classify context canceled as permanent", func() {
            // Verify immediate failure (no retry)
        })
    })

    Context("Edge Cases", func() {
        It("should default unknown errors to permanent", func() {
            // Test with unrecognized error message
        })
    })
})
```

---

### 2. Retry & Backoff Logic (**HIGH PRIORITY**)

**File**: `pkg/aianalysis/handlers/investigating.go` (handleError)
**Coverage**: **0%**
**Business Impact**: Resilience, cost efficiency, user experience

#### Missing Test Scenarios:

##### A. Max Retries Exceeded
```go
// MISSING: Test transient error exceeding MaxRetries (3)
// Expected Behavior:
// - Status.ConsecutiveFailures = 4 (after 4th attempt)
// - Status.Phase = "Failed"
// - Status.Reason = "APIError"
// - Status.SubReason = "MaxRetriesExceeded"
// - Metric: aianalysis_failures_total{reason="APIError",sub_reason="MaxRetriesExceeded"}
```

##### B. Exponential Backoff
```go
// MISSING: Test backoff duration calculation
// Attempt 1: ~5s base delay + jitter
// Attempt 2: ~10s (2x) + jitter
// Attempt 3: ~20s (4x) + jitter
// Attempt 4: Max 60s cap + jitter
```

##### C. Retry Count Annotations
```go
// MISSING: Test retry count persistence in annotations
// annotation: aianalysis.kubernaut.ai/retry-count = "2"
// Verify count incremented across reconciliations
```

##### D. Permanent vs Transient Decision
```go
// MISSING: Test immediate failure on permanent errors (no retry)
// MISSING: Test retry on transient errors
```

#### Recommended Test Structure:
```go
// test/integration/aianalysis/retry_logic_test.go
var _ = Describe("Retry Logic - BR-AI-009", Label("integration", "retry"), func() {
    Context("Max Retries Exceeded", func() {
        It("should fail permanently after 3 retries", func() {
            // Create AIAnalysis with mock HAPI returning 503 (transient)
            // Wait for 4 reconciliation attempts
            // Verify Status.ConsecutiveFailures = 4
            // Verify Status.Phase = "Failed"
            // Verify Status.SubReason = "MaxRetriesExceeded"
        })

        It("should record max retries exceeded metric", func() {
            // Verify metric: aianalysis_failures_total{sub_reason="MaxRetriesExceeded"}
        })
    })

    Context("Exponential Backoff", func() {
        It("should apply exponential backoff with jitter", func() {
            // Measure RequeueAfter durations
            // Verify Attempt 1: ~5s ±10%
            // Verify Attempt 2: ~10s ±10%
            // Verify Attempt 3: ~20s ±10%
        })

        It("should cap backoff at max delay (60s)", func() {
            // Test with high failure count (e.g., 10)
            // Verify backoff never exceeds 60s
        })
    })

    Context("Retry Annotations", func() {
        It("should persist retry count in annotations", func() {
            // Verify annotation: aianalysis.kubernaut.ai/retry-count
            // Verify count increments across reconciliations
        })
    })

    Context("Permanent Errors", func() {
        It("should fail immediately on permanent errors", func() {
            // Create AIAnalysis with mock HAPI returning 400 (validation)
            // Verify NO retry (ConsecutiveFailures = 0)
            // Verify immediate Phase = "Failed"
        })
    })
})
```

---

## 🟡 MODERATE GAPS (Low Coverage)

### 3. Controller State Transitions (**HIGH PRIORITY**)

**File**: `internal/controller/aianalysis/aianalysis_controller.go`
**Coverage**: 45-82%
**Business Impact**: Core orchestration logic

#### Missing Test Scenarios:

##### A. Deletion Handling (Line 179: 45% coverage)
```go
// MISSING: Test finalizer cleanup
// MISSING: Test graceful shutdown during deletion
// MISSING: Test audit event generation on deletion
```

##### B. Status Update Failures
```go
// MISSING: Test status update conflict (409)
// MISSING: Test status update retry logic
```

##### C. Phase Transition Edge Cases
```go
// MISSING: Test transition from Investigating → Analyzing → Complete
// MISSING: Test transition from Investigating → Failed (error)
// MISSING: Test transition from Analyzing → Approved (production)
// MISSING: Test transition from Analyzing → Complete (non-production)
```

#### Recommended Test Structure:
```go
// test/integration/aianalysis/controller_edge_cases_test.go
var _ = Describe("Controller Edge Cases", Label("integration", "controller"), func() {
    Context("Deletion Handling", func() {
        It("should cleanup finalizers before deletion", func() {
            // Create AIAnalysis → Delete → Verify finalizer removed
        })

        It("should generate deletion audit event", func() {
            // Verify audit event: reason="Deleted"
        })
    })

    Context("Status Update Conflicts", func() {
        It("should retry on status update conflict", func() {
            // Simulate 409 conflict
            // Verify retry logic
        })
    })

    Context("Phase Transitions", func() {
        It("should transition all phases in order", func() {
            // New → Investigating → Analyzing → Complete
            // Verify each phase transition audit event
        })
    })
})
```

---

### 4. Condition Helpers (**MEDIUM PRIORITY**)

**File**: `pkg/aianalysis/conditions.go`
**Coverage**: 0-75%
**Business Impact**: Observability, debugging

#### Missing Test Scenarios:

##### A. GetCondition Function (0% coverage)
```go
// MISSING: Test GetCondition with existing condition
// MISSING: Test GetCondition with non-existent condition (return nil)
```

##### B. Condition State Transitions (66-75% coverage)
```go
// MISSING: Test SetInvestigationComplete edge cases
// MISSING: Test SetAnalysisComplete edge cases
// MISSING: Test SetWorkflowResolved edge cases
// MISSING: Test condition timestamp updates
```

#### Recommended Test Structure:
```go
// test/integration/aianalysis/conditions_test.go
var _ = Describe("Conditions - Observability", Label("integration", "conditions"), func() {
    Context("GetCondition", func() {
        It("should return existing condition", func() {
            // Create AIAnalysis with condition
            // Verify GetCondition returns correct condition
        })

        It("should return nil for non-existent condition", func() {
            // Verify GetCondition returns nil safely
        })
    })

    Context("Condition Transitions", func() {
        It("should update condition timestamps", func() {
            // Verify lastTransitionTime updates
        })
    })
})
```

---

### 5. Error Type Constructors (**MEDIUM PRIORITY**)

**File**: `pkg/aianalysis/handler.go`
**Coverage**: **0%**
**Business Impact**: Error handling consistency

#### Missing Test Scenarios:

```go
// MISSING: Test NewTransientError constructor
// MISSING: Test NewPermanentError constructor
// MISSING: Test NewValidationError constructor
// MISSING: Test Error.Unwrap() for error chains
```

#### Recommended Test Structure:
```go
// Add to test/integration/aianalysis/error_classification_test.go
Context("Error Type Constructors", func() {
    It("should create transient error with correct type", func() {
        // Verify error type can be unwrapped
    })

    It("should create permanent error with correct type", func() {
        // Verify error type differentiation
    })
})
```

---

## 📈 ENHANCEMENT OPPORTUNITIES

### 6. Additional Scenarios for Complete Coverage

#### A. Recovery Context Scenarios
```go
// OPPORTUNITY: Test recovery with different PreviousExecution contexts
// - Multiple failed attempts (attempt 3, 4, 5)
// - Different failure reasons (timeout, OOM, config error)
// - Different workflow types (restart, scale, config-update)
```

#### B. Enrichment Edge Cases
```go
// OPPORTUNITY: Test various enrichment data combinations
// - DetectedLabels present vs absent
// - KubernetesContext present vs absent
// - CustomLabels edge cases
```

#### C. Approval Workflow
```go
// OPPORTUNITY: Test approval required scenarios
// - Production environment with manual approval
// - Non-production auto-approval
// - Approval timeout scenarios
```

#### D. Metrics Edge Cases
```go
// OPPORTUNITY: Test metric collection for all failure modes
// - Each ErrorCategory should have metric test
// - Each SubReason should have metric test
// - Verify metric cardinality limits
```

---

## 🎯 RECOMMENDED PRIORITY ORDER

### Phase 1: Critical Gaps (Week 1)
1. **Error Classification Tests** (`error_classification_test.go`)
   - All HTTP status codes (401, 403, 404, 409, 429, 5xx)
   - Context errors (deadline, canceled)
   - Network errors (connection refused, timeout)
   - **Impact**: Fixes 0% coverage, enables reliable retry logic

2. **Retry Logic Tests** (`retry_logic_test.go`)
   - Max retries exceeded
   - Exponential backoff verification
   - Retry annotations
   - **Impact**: Validates resilience mechanisms

### Phase 2: High-Value Gaps (Week 2)
3. **Controller Edge Cases** (`controller_edge_cases_test.go`)
   - Deletion handling
   - Status update conflicts
   - Phase transitions
   - **Impact**: Improves core orchestration coverage to 90%+

### Phase 3: Comprehensive Coverage (Week 3)
4. **Condition Helpers** (`conditions_test.go`)
   - GetCondition edge cases
   - Condition state transitions
   - **Impact**: Completes observability testing

5. **Error Type Constructors** (add to `error_classification_test.go`)
   - Test all error type constructors
   - **Impact**: Ensures error handling consistency

---

## 📊 EXPECTED COVERAGE IMPROVEMENTS

| Phase | New Tests | Expected Coverage | Improvement |
|-------|-----------|-------------------|-------------|
| **Current** | 53 | 54.6% | - |
| **Phase 1** | +15-20 | **70-75%** | +15-20% |
| **Phase 2** | +10-15 | **80-85%** | +10-15% |
| **Phase 3** | +5-10 | **85-90%** | +5-10% |

### Target Coverage by Component (After All Phases)

| Component | Current | Target | Achievable |
|-----------|---------|--------|------------|
| Controller | 45-82% | **90%+** | ✅ Yes |
| Error Classifier | **0%** | **95%+** | ✅ Yes |
| Error Handling | 0-77% | **90%+** | ✅ Yes |
| Conditions | 0-75% | **85%+** | ✅ Yes |
| Error Types | **0%** | **90%+** | ✅ Yes |
| **Overall** | **54.6%** | **85-90%** | ✅ Yes |

---

## 🔍 TEST IMPLEMENTATION GUIDELINES

### Integration Test Patterns (from existing tests)

#### Pattern 1: End-to-End Reconciliation
```go
// From: test/integration/aianalysis/reconciliation_test.go
It("should transition through all phases successfully", func() {
    // 1. Create AIAnalysis CRD
    // 2. Wait for phase transitions (Eventually)
    // 3. Verify status fields at each phase
    // 4. Check audit events in DataStorage
    // 5. Verify metrics in Prometheus
})
```

#### Pattern 2: HAPI Mock Response Testing
```go
// From: test/integration/aianalysis/holmesgpt_integration_test.go
It("should handle needs_human_review=true", func() {
    // 1. Configure HAPI mock to return needs_human_review=true
    // 2. Create AIAnalysis
    // 3. Verify Status.Reason and Status.SubReason
    // 4. Check that NO WorkflowExecution is created
    // 5. Verify audit trail
})
```

#### Pattern 3: Audit Trail Verification
```go
// From: test/integration/aianalysis/audit_integration_test.go
It("should persist analysis completion audit event", func() {
    // 1. Create AIAnalysis
    // 2. Wait for completion
    // 3. Query DataStorage audit API
    // 4. Verify all required fields in audit event
    // 5. Verify event timestamp and correlation ID
})
```

### Testing Infrastructure Available

#### Mock Services (via podman-compose)
- **PostgreSQL** (port 15434): Real database with pgvector
- **Redis** (port 16380): Real cache
- **DataStorage API** (port 18091): Real API with audit endpoints
- **HolmesGPT API** (port 18120): Mock LLM with deterministic responses

#### Test Utilities
- `testutil.NewMockLLMClient()`: Mock HAPI client for unit tests
- `infrastructure.StartAIAnalysisIntegrationInfrastructure()`: Podman setup
- envtest: In-memory K8s API server for CRD testing

---

## 💼 BUSINESS VALUE BY PRIORITY

### Critical Priority Tests (Phase 1)
**Business Value**: **CRITICAL**
- **Prevents**: Production incidents from unhandled error scenarios
- **Enables**: Reliable retry logic for transient failures
- **Improves**: Operator confidence in error handling
- **ROI**: High - Directly impacts system reliability

### High Priority Tests (Phase 2)
**Business Value**: **HIGH**
- **Prevents**: Edge case failures in core orchestration
- **Enables**: Smooth upgrades and migration
- **Improves**: System resilience and recovery
- **ROI**: Medium-High - Improves operational stability

### Medium Priority Tests (Phase 3)
**Business Value**: **MEDIUM**
- **Prevents**: Observability gaps and debugging difficulty
- **Enables**: Faster incident resolution
- **Improves**: Developer experience
- **ROI**: Medium - Supports maintenance and debugging

---

## ✅ ACCEPTANCE CRITERIA

### Phase 1 Complete When:
- [ ] All error classification scenarios tested (15+ tests)
- [ ] Retry logic fully validated (10+ tests)
- [ ] Integration coverage ≥ 70%
- [ ] Zero 0% coverage in error handling code

### Phase 2 Complete When:
- [ ] Controller edge cases tested (10+ tests)
- [ ] Integration coverage ≥ 80%
- [ ] All phase transitions validated

### Phase 3 Complete When:
- [ ] Condition helpers tested (5+ tests)
- [ ] Error type constructors validated (5+ tests)
- [ ] Integration coverage ≥ 85%
- [ ] No component below 80% coverage

---

## 📚 REFERENCES

- **Current Coverage Report**: `coverage-integration-aianalysis.out`
- **Business Requirements**: `docs/requirements/BR-AI-*.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`
- **Integration Test Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **DD-TEST-002**: Sequential infrastructure startup pattern
- **DD-AUDIT-003**: AIAnalysis audit trace requirements

---

**Generated**: December 24, 2025
**Next Review**: After Phase 1 implementation
**Owner**: AIAnalysis Service Team









