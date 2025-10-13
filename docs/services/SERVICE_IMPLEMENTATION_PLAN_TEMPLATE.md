# [Service Name] - Implementation Plan Template

**Version**: v2.0 - COMPREHENSIVE PRODUCTION-READY STANDARD
**Last Updated**: 2025-10-12
**Timeline**: [X] days (11-12 days typical)
**Status**: ✅ Production-Ready Template (98% Confidence Standard)
**Quality Level**: Matches Data Storage v4.1 and Notification V3.0 standards

**Change Log**:
- **v2.0** (2025-10-12): 🎯 **MAJOR UPDATE** - Comprehensive production-ready enhancements
  - ✅ **60+ complete code examples** (zero TODO placeholders, V3.0 standard)
  - ✅ **Error Handling Philosophy Template** (280 lines, complete methodology)
  - ✅ **Enhanced BR Coverage Matrix** (calculation methodology, 97%+ target)
  - ✅ **3 Complete EOD Templates** (Days 1, 4, 7 - ~450 lines total)
  - ✅ **CRD Controller Variant Section** (~400 lines, reconciliation patterns)
  - ✅ **Enhanced Prometheus Metrics** (10+ metrics with recording patterns)
  - ✅ **Complete Integration Test Examples** (2-3 tests, ~400 lines)
  - ✅ **Phase 4 Documentation Templates** (Handoff, Production, Confidence)
  - ✅ **Confidence Assessment Methodology** (evidence-based calculation)
  - ✅ **Production-ready code quality** (error handling, logging, metrics in all examples)
  - 📏 **Template size**: ~4,500 lines (3x growth from v1.3 for comprehensive guidance)
- v1.3: Added Integration Test Environment Decision Tree (KIND/envtest/Podman/Mocks)
- v1.2: Added Kind Cluster Test Template for integration tests
- v1.1: Added table-driven testing patterns

---

## 🎯 Quick Reference

**Use this template for**: All Kubernaut stateless services and CRD controllers
**Based on**: Gateway Service + Dynamic Toolset + Notification Controller (proven success)
**Methodology**: APDC-TDD with Integration-First Testing
**Success Rate**:
- Gateway: 95% test coverage, 100% BR coverage, 98% confidence
- Notification: 97.2% BR coverage, 95% test coverage, 98% confidence
**Quality Standard**: V3.0 - Production-ready with comprehensive examples

---

## Document Purpose

This template incorporates lessons learned from:
1. **Gateway Service**: Production-ready (21/22 tests, 95% coverage, 98% confidence)
2. **Dynamic Toolset Service**: Enhanced with additional best practices
3. **Notification Controller**: CRD controller standard (97.2% BR coverage, 98% confidence)
4. **Data Storage Service**: Comprehensive v4.1 standard (complete error handling, metrics)

**🎯 V2.0 Enhancement Highlights** (Major Update):
- **60+ Complete Code Examples**: Zero TODO placeholders, production-ready quality
- **Error Handling Philosophy**: 280-line methodology template included
- **Enhanced BR Coverage Matrix**: Calculation methodology, 97%+ target
- **3 Complete EOD Templates**: Days 1, 4, 7 with checklists and confidence assessments
- **CRD Controller Variant**: 400-line section with reconciliation patterns
- **Enhanced Prometheus Metrics**: 10+ metrics with recording patterns and testing
- **Complete Integration Tests**: 2-3 full examples (~400 lines) from proven implementations
- **Phase 4 Templates**: Handoff Summary, Production Readiness, Confidence Assessment
- **Evidence-Based Confidence**: Calculation methodology with formula

**Key Improvements Over Ad-Hoc Planning** (Retained from v1.x):
- Integration-first testing (catches issues 2 days earlier)
- Schema validation before testing (prevents test failures)
- Daily progress tracking (EOD documentation templates) ⭐ **v2.0 ENHANCED**
- BR coverage matrix (calculation methodology included) ⭐ **v2.0 ENHANCED**
- Production readiness checklist (comprehensive templates) ⭐ **v2.0 ENHANCED**
- File organization strategy (cleaner git history)
- Table-driven testing pattern (25-40% less test code)
- Kind cluster test template (15 lines vs 80+)
- Integration test decision tree (KIND/envtest/Podman/Mocks)
- Error handling philosophy (complete template) ⭐ **v2.0 NEW**
- CRD controller patterns (reconciliation, status updates) ⭐ **v2.0 NEW**

---

## Prerequisites Checklist

Before starting Day 1, ensure:
- [ ] Service specifications complete (overview, API spec, implementation docs)
- [ ] Business requirements documented (BR-[CATEGORY]-XXX format)
- [ ] Architecture decisions approved
- [ ] Dependencies identified
- [ ] Success criteria defined
- [ ] **Integration test environment determined** (see decision tree below)
- [ ] **Required test infrastructure available** (KIND/envtest/Podman/none)
- [ ] **V2.0 Template sections reviewed**: ⭐ **NEW**
  - [ ] Error Handling Philosophy Template (Section after Day 6)
  - [ ] BR Coverage Matrix Methodology (Day 9 Enhanced)
  - [ ] EOD Documentation Templates (Appendix A)
  - [ ] CRD Controller Variant (Appendix B, if applicable)
  - [ ] Complete Integration Test Examples (Day 8 Enhanced)
  - [ ] Phase 4 Documentation Templates (Days 10-12 Enhanced)
  - [ ] Confidence Assessment Methodology (Day 12 Enhanced)

---

## 🔍 Integration Test Environment Decision (v1.3) ⭐ NEW

**CRITICAL**: Determine your integration test environment **before Day 1** using this decision tree.

### Decision Tree

```
Does your service WRITE to Kubernetes (create/modify CRDs or resources)?
├─ YES → Does it need RBAC or TokenReview API?
│        ├─ YES → Use KIND (full K8s cluster)
│        └─ NO → Use ENVTEST (API server only)
│
└─ NO → Does it READ from Kubernetes?
         ├─ YES → Need field selectors or CRDs?
         │        ├─ YES → Use ENVTEST
         │        └─ NO → Use FAKE CLIENT
         │
         └─ NO → Use PODMAN (external services only)
                 or HTTP MOCKS (if no external deps)
```

### Classification Guide

#### 🔴 KIND Required
**Use When**:
- Writes CRDs or Kubernetes resources
- Needs RBAC enforcement
- Uses TokenReview API for authentication
- Requires ServiceAccount permissions testing

**Examples**: Gateway Service, Dynamic Toolset Service (V2)

**Prerequisites**:
- [ ] KIND cluster available (`make bootstrap-dev`)
- [ ] Kind template documentation reviewed ([KIND_CLUSTER_TEST_TEMPLATE.md](../testing/KIND_CLUSTER_TEST_TEMPLATE.md))

---

#### 🟡 ENVTEST Required
**Use When**:
- Reads from Kubernetes (logs, events, resources)
- Needs field selectors (e.g., `.spec.nodeName=worker`)
- Writes ConfigMaps/Services (but no RBAC needed)
- Testing with CRDs (no RBAC validation)

**Examples**: Dynamic Toolset Service (V1), HolmesGPT API Service

**Prerequisites**:
- [ ] `setup-envtest` installed (`go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`)
- [ ] Binaries downloaded (`setup-envtest use 1.31.0`)

---

#### 🟢 PODMAN Required
**Use When**:
- No Kubernetes operations
- Needs PostgreSQL, Redis, or other databases
- External service dependencies

**Examples**: Data Storage Service, Context API Service

**Prerequisites**:
- [ ] Docker/Podman available
- [ ] testcontainers-go configured

---

#### ⚪ HTTP MOCKS Only
**Use When**:
- No Kubernetes operations
- No database dependencies
- Only HTTP API calls to other services

**Examples**: Effectiveness Monitor Service, Notification Service

**Prerequisites**:
- [ ] None (uses Go stdlib `net/http/httptest`)

---

### Quick Classification Examples

| Service Type | Kubernetes Ops | Databases | Test Env |
|--------------|---------------|-----------|----------|
| Writes CRDs + RBAC | ✅ Write + RBAC | ❌ | 🔴 KIND |
| Writes ConfigMaps only | ✅ Write (no RBAC) | ❌ | 🟡 ENVTEST |
| Reads K8s (field selectors) | ✅ Read (complex) | ❌ | 🟡 ENVTEST |
| Reads K8s (simple) | ✅ Read (simple) | ❌ | Fake Client |
| HTTP API + PostgreSQL | ❌ | ✅ | 🟢 PODMAN |
| HTTP API only | ❌ | ❌ | ⚪ HTTP MOCKS |

### Update Your Plan

Once determined, update all instances of `[TEST_ENVIRONMENT]` in this plan with your choice:
- Replace `[TEST_ENVIRONMENT]` with: **KIND** | **ENVTEST** | **PODMAN** | **HTTP_MOCKS**
- Update prerequisites checklist above
- Review setup requirements in Day 8 (Integration Test Setup)

**Reference Documentation**:
- [Integration Test Environment Decision Tree](../../testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md)
- [Stateless Services Integration Test Strategy](../stateless/INTEGRATION_TEST_STRATEGY.md)
- [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)

---

## Timeline Overview

| Phase | Days | Focus | Key Deliverables |
|-------|------|-------|------------------|
| **Foundation** | 1 | Types, interfaces, K8s client | Package structure, interfaces |
| **Core Logic** | 2-6 | Business logic components | All components implemented |
| **Integration** | 7 | Server, API, metrics | Complete service |
| **Testing** | 8-10 | Integration + Unit tests | 70%+ coverage |
| **Finalization** | 11-12 | E2E, docs, production readiness | Ready for deployment |

**Total**: 11-12 days (with buffer)

---

## Day-by-Day Breakdown

### Day 1: Foundation (8h)

#### ANALYSIS Phase (1h)
**Search existing patterns:**
```bash
codebase_search "Kubernetes client initialization in-cluster config"
codebase_search "[Service functionality] implementations"
grep -r "relevant patterns" pkg/ cmd/ --include="*.go"
```

**Map business requirements:**
- List all BR-[CATEGORY]-XXX requirements
- Identify critical path requirements
- Note any missing specifications

#### PLAN Phase (1h)
**TDD Strategy:**
- Unit tests: [Component list] (70%+ coverage target)
- Integration tests: [Scenario list] (>50% coverage target)
- E2E tests: [Workflow list] (<10% coverage target)

**Integration points:**
- Main app: `cmd/[service]/main.go`
- Business logic: `pkg/[service]/`
- Tests: `test/unit/[service]/`, `test/integration/[service]/`

**Success criteria:**
- [Performance metric 1] (target: X)
- [Performance metric 2] (target: Y)
- [Functional requirement] verified

#### DO-DISCOVERY (6h)
**Create package structure:**
```bash
mkdir -p cmd/[service]
mkdir -p pkg/[service]/{component1,component2,component3}
mkdir -p internal/[service]/{helpers}
mkdir -p test/unit/[service]
mkdir -p test/integration/[service]
mkdir -p test/e2e/[service]
```

**Create foundational files:**
- `pkg/[service]/types.go` - Core type definitions
- `pkg/[service]/[interface1].go` - Primary interface
- `pkg/[service]/[interface2].go` - Secondary interface
- `internal/[service]/k8s/client.go` - Kubernetes client wrapper (if needed)
- `cmd/[service]/main.go` - Basic skeleton

**Validation:**
- [ ] All packages created
- [ ] Types defined
- [ ] Interfaces defined
- [ ] Main.go compiles
- [ ] Zero lint errors

**EOD Documentation:**
- [ ] Create `implementation/phase0/01-day1-complete.md`
- [ ] Document architecture decisions
- [ ] Note any deviations from plan

---

### Days 2-6: Core Implementation (5 days, 8h each)

**Pattern for Each Component:**

#### DO-RED: Write Tests First (1.5-2h per component)
**File**: `test/unit/[service]/[component]_test.go`

**⭐ RECOMMENDED: Use Table-Driven Tests (DescribeTable) whenever possible**

**Pattern 1: Table-Driven Tests for Multiple Similar Scenarios** (Preferred)
```go
var _ = Describe("BR-[CATEGORY]-XXX: [Component Name]", func() {
    // Use DescribeTable for multiple test cases with same logic
    DescribeTable("should handle various input scenarios",
        func(input InputType, expectedOutput OutputType, expectError bool) {
            result, err := component.Method(input)

            if expectError {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).ToNot(HaveOccurred())
                Expect(result).To(Equal(expectedOutput))
            }
        },
        Entry("scenario 1 description", input1, output1, false),
        Entry("scenario 2 description", input2, output2, false),
        Entry("scenario 3 with error", input3, nil, true),
        // Easy to add more scenarios - just add Entry lines!
    )
})
```

**Pattern 2: Traditional Tests for Unique Logic** (When needed)
```go
var _ = Describe("BR-[CATEGORY]-XXX: [Component Name]", func() {
    Context("when [unique condition]", func() {
        It("should [behavior]", func() {
            // Test implementation for unique scenario
        })
    })
})
```

**When to Use Table-Driven Tests**:
- ✅ Testing same logic with different inputs/outputs
- ✅ Testing multiple detection/validation scenarios
- ✅ Testing various error conditions
- ✅ Testing different configuration permutations
- ✅ Testing boundary conditions and edge cases

**When to Use Traditional Tests**:
- Complex setup that varies significantly per test
- Unique test logic that doesn't fit table pattern
- One-off tests with complex assertions

**Benefits**:
- 25-40% less code through elimination of duplication
- Easier to add new test cases (just add Entry)
- Better test organization and readability
- Consistent assertion patterns

**Reference**: See Dynamic Toolset detector tests for examples:
- `test/unit/toolset/prometheus_detector_test.go`
- `test/unit/toolset/grafana_detector_test.go`

**Validation:**
- [ ] Tests written (prefer table-driven where applicable)
- [ ] Tests fail (expected)
- [ ] Business requirements referenced (BR-XXX-XXX)
- [ ] Entry names clearly describe scenarios

#### DO-GREEN: Minimal Implementation (1.5-2h per component)
**File**: `pkg/[service]/[component].go`

**Validation:**
- [ ] Tests pass
- [ ] No extra features
- [ ] Integration point identified

#### DO-REFACTOR: Extract Common Patterns (2-3h per day)
**Common Refactorings:**
- Extract shared utilities
- Standardize error handling
- Extract validation logic
- Create helper functions

**Validation:**
- [ ] Code DRY (Don't Repeat Yourself)
- [ ] Patterns consistent
- [ ] Tests still pass

**Day-Specific Focus:**
- **Day 2**: [Component set 1]
- **Day 3**: [Component set 2]
- **Day 4**: [Component set 3] + **EOD: Create 02-day4-midpoint.md** ⭐
- **Day 5**: [Component set 4]
- **Day 6**: [Component set 5] + **DO-REFACTOR: Error handling philosophy doc** ⭐

---

## 📖 Error Handling Philosophy Template ⭐ V2.0

**⚠️ MANDATORY**: Create this document at end of Day 6 to establish consistent error handling across all components.

**File**: `docs/services/[service-type]/[service-name]/implementation/design/ERROR_HANDLING_PHILOSOPHY.md`

**Purpose**: Define authoritative error handling patterns for this service, ensuring consistency and reliability.

---

### Complete Template (Copy This)

```markdown
# Error Handling Philosophy - [Service Name]

**Date**: YYYY-MM-DD
**Status**: ✅ Authoritative Guide
**Version**: 1.0

---

## 🎯 **Core Principles**

### 1. **Error Classification**
All errors fall into three categories:

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**: Network timeouts, 503 Service Unavailable, database connection errors
- **Strategy**: Exponential backoff with jitter
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**: 401 Unauthorized, 404 Not Found, validation failures, malformed input
- **Strategy**: Fail immediately, log error, update status
- **Max Retries**: 0 (no retry)

#### **User Errors** (Input Validation)
- **Definition**: Invalid user input or configuration
- **Examples**: Missing required fields, invalid formats, out-of-range values
- **Strategy**: Return validation error immediately, do not retry
- **Max Retries**: 0 (no retry)

---

## 🔄 **Retry Strategy**

### Exponential Backoff Implementation

\`\`\`go
// CalculateBackoff returns exponential backoff duration
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func CalculateBackoff(attemptCount int) time.Duration {
	baseDelay := 30 * time.Second
	maxDelay := 480 * time.Second

	// Calculate exponential backoff: baseDelay * 2^attemptCount
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))

	// Cap at maximum delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10%) to prevent thundering herd
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

	return jitter
}
\`\`\`

### Retry Decision Matrix

| Error Type | HTTP Status | Retry? | Backoff | Max Attempts | Example |
|-----------|-------------|--------|---------|--------------|---------|
| Transient | 500, 502, 503, 504 | ✅ Yes | Exponential | 5 | Service temporarily unavailable |
| Transient | Timeout | ✅ Yes | Exponential | 5 | Network timeout |
| Transient | Connection refused | ✅ Yes | Exponential | 3 | Service restarting |
| Permanent | 401, 403 | ❌ No | N/A | 0 | Authentication failure |
| Permanent | 404 | ❌ No | N/A | 0 | Resource not found |
| Permanent | 400 | ❌ No | N/A | 0 | Bad request format |
| User Error | Validation | ❌ No | N/A | 0 | Missing required field |

---

## 🔐 **Circuit Breaker Pattern**

### Circuit Breaker States

\`\`\`
CLOSED (Normal Operation) → OPEN (Failing) → HALF_OPEN (Testing) → CLOSED
     ↓                           ↓                    ↓
   Normal                   Fast-fail          Limited retries
\`\`\`

### Implementation Guidance

\`\`\`go
// CircuitBreaker tracks failure rates and manages state transitions
type CircuitBreaker struct {
	State              CircuitState
	FailureCount       int
	FailureThreshold   int // e.g., 5 failures
	SuccessCount       int
	SuccessThreshold   int // e.g., 2 successes in HALF_OPEN
	Timeout            time.Duration // e.g., 60 seconds
	LastFailureTime    time.Time
}

// ShouldAllowRequest determines if request should proceed
func (cb *CircuitBreaker) ShouldAllowRequest() bool {
	switch cb.State {
	case CircuitStateClosed:
		return true // Normal operation
	case CircuitStateOpen:
		// Check if timeout elapsed
		if time.Since(cb.LastFailureTime) > cb.Timeout {
			cb.State = CircuitStateHalfOpen
			cb.SuccessCount = 0
			return true // Try one request
		}
		return false // Fast-fail
	case CircuitStateHalfOpen:
		return true // Allow limited requests
	default:
		return false
	}
}
\`\`\`

**When to Use**:
- External API calls (Slack, email providers)
- Database connections
- Network-dependent operations

**Benefits**:
- Prevents cascade failures
- Reduces load on failing services
- Graceful degradation

---

## 📝 **Error Wrapping & Context**

### Standard Error Wrapping Pattern

\`\`\`go
// Good: Error wrapping with context
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
	data, err := s.fetchData(ctx, req.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch data for request %s: %w", req.ID, err)
	}

	if err := s.validate(data); err != nil {
		return fmt.Errorf("validation failed for request %s: %w", req.ID, err)
	}

	return nil
}

// Bad: Error swallowing
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
	data, _ := s.fetchData(ctx, req.ID) // ❌ Error ignored!
	s.validate(data)
	return nil
}
\`\`\`

### Context Propagation

Always include:
- **Request ID**: Unique identifier for tracing
- **Resource ID**: Affected resource (pod name, deployment name)
- **Operation**: What was being attempted
- **Timestamp**: When error occurred

---

## 📊 **Logging Best Practices**

### Structured Logging Pattern

\`\`\`go
// Production-ready error logging
func (s *Service) handleError(ctx context.Context, operation string, err error) {
	log := log.FromContext(ctx)

	// Classify error
	errorType := classifyError(err)

	// Structured logging with context
	log.Error(err, "Operation failed",
		"operation", operation,
		"error_type", errorType,
		"retry_able", isRetryable(errorType),
		"request_id", getRequestID(ctx),
		"resource", getResourceName(ctx),
		"timestamp", time.Now().Format(time.RFC3339),
	)

	// Emit metric
	s.metrics.ErrorsTotal.With(prometheus.Labels{
		"operation":  operation,
		"error_type": string(errorType),
	}).Inc()
}
\`\`\`

### Log Levels

| Level | Use When | Example |
|-------|----------|---------|
| ERROR | Permanent failures, requires intervention | Authentication failure, CRD validation error |
| WARN | Transient failures, will retry | Network timeout on attempt 1/5 |
| INFO | Normal operation events | Request processed successfully |
| DEBUG | Detailed troubleshooting | Retry attempt 3/5 with 120s backoff |

---

## 🚨 **Error Recovery Strategies**

### Graceful Degradation

\`\`\`go
// Example: Notification service with graceful degradation
func (s *Service) SendNotifications(ctx context.Context, notif *Notification) error {
	errors := make([]error, 0)

	// Try console (always succeeds)
	if err := s.sendToConsole(ctx, notif); err != nil {
		errors = append(errors, fmt.Errorf("console delivery failed: %w", err))
	}

	// Try Slack (may fail - graceful degradation)
	if err := s.sendToSlack(ctx, notif); err != nil {
		log.Warn("Slack delivery failed, continuing", "error", err)
		errors = append(errors, fmt.Errorf("slack delivery failed: %w", err))
	}

	// Partial success handling
	if len(errors) == 0 {
		return nil // All succeeded
	} else if len(errors) < 2 {
		return nil // At least one succeeded (graceful degradation)
	} else {
		return fmt.Errorf("all deliveries failed: %v", errors)
	}
}
\`\`\`

### Rollback Strategies

For CRD controllers and stateful operations:

\`\`\`go
// Example: Kubernetes action with rollback
func (e *Executor) ExecuteAction(ctx context.Context, action *Action) error {
	// Save original state
	originalState, err := e.captureState(ctx, action.Resource)
	if err != nil {
		return fmt.Errorf("failed to capture original state: %w", err)
	}

	// Attempt action
	if err := e.applyAction(ctx, action); err != nil {
		log.Error(err, "Action failed, attempting rollback")

		// Rollback to original state
		if rollbackErr := e.restoreState(ctx, originalState); rollbackErr != nil {
			return fmt.Errorf("action failed and rollback failed: %w (rollback: %v)", err, rollbackErr)
		}

		return fmt.Errorf("action failed, rolled back successfully: %w", err)
	}

	return nil
}
\`\`\`

---

## ✅ **Implementation Checklist**

Use this checklist when implementing error handling for each component:

- [ ] **Error Classification**: All errors classified as transient/permanent/user
- [ ] **Retry Logic**: Exponential backoff implemented for transient errors
- [ ] **Circuit Breaker**: Implemented for external dependencies
- [ ] **Error Wrapping**: All errors wrapped with context using `fmt.Errorf("%w")`
- [ ] **Structured Logging**: All errors logged with structured fields
- [ ] **Metrics**: Error counters emitted for all error types
- [ ] **Graceful Degradation**: Partial failure handling implemented
- [ ] **Rollback**: State recovery implemented for stateful operations
- [ ] **Testing**: Error scenarios tested (transient, permanent, user)
- [ ] **Documentation**: Error handling patterns documented in code

---

## 📚 **References**

- [Error Handling Standard](../../../architecture/ERROR_HANDLING_STANDARD.md)
- [Logging Standard](../../../architecture/LOGGING_STANDARD.md)
- [Testing Strategy](../../../testing/README.md)

---

**Status**: ✅ Complete Error Handling Philosophy
**Confidence**: 98% - Production-ready error handling patterns
**Next**: Apply this philosophy to all service components
```

---

**Validation After Creation**:
- [ ] Error classification guide complete
- [ ] Retry strategy with exponential backoff defined
- [ ] Circuit breaker pattern documented
- [ ] Error wrapping patterns established
- [ ] Logging best practices defined
- [ ] Graceful degradation examples provided
- [ ] Rollback strategies documented
- [ ] Implementation checklist included

**Impact**: Ensures consistent, production-ready error handling across all components, preventing common pitfalls like error swallowing, infinite retries, and cascade failures.

---

### Day 7: Server + API + Metrics (8h)

#### HTTP Server Implementation (3h)
- Server struct with router
- Route registration
- Middleware stack
- Health/readiness endpoints

#### Metrics Implementation (2h) ⭐ V2.0 ENHANCED

**Target**: 10+ production-ready Prometheus metrics with complete recording patterns

---

**Complete Metrics Definition**:

```go
// pkg/[service]/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics contains all Prometheus metrics for the service
type Metrics struct {
	// Operations metrics
	OperationsTotal   *prometheus.CounterVec
	OperationDuration *prometheus.HistogramVec

	// Error metrics
	ErrorsTotal       *prometheus.CounterVec

	// Resource metrics (for CRD controllers)
	ResourcesProcessed *prometheus.CounterVec
	ReconciliationDuration *prometheus.HistogramVec
	ReconciliationErrors *prometheus.CounterVec

	// Queue metrics (if applicable)
	QueueDepth *prometheus.GaugeVec
	QueueLatency *prometheus.HistogramVec

	// Business-specific metrics
	[CustomMetric1] *prometheus.CounterVec
	[CustomMetric2] *prometheus.GaugeVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics(namespace, subsystem string) *Metrics {
	return &Metrics{
		// 1. Operations Counter (tracks all operations)
		OperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "operations_total",
				Help:      "Total number of operations processed",
			},
			[]string{"operation", "status"}, // labels: operation type, success/failure
		),

		// 2. Operation Duration Histogram (latency tracking)
		OperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "operation_duration_seconds",
				Help:      "Duration of operations in seconds",
				Buckets:   prometheus.DefBuckets, // or custom: []float64{.001, .01, .1, .5, 1, 2.5, 5, 10}
			},
			[]string{"operation"},
		),

		// 3. Errors Counter (detailed error tracking)
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "errors_total",
				Help:      "Total number of errors by type",
			},
			[]string{"error_type", "operation"}, // transient/permanent/user
		),

		// 4. Resources Processed (CRD controllers)
		ResourcesProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "resources_processed_total",
				Help:      "Total number of resources processed",
			},
			[]string{"phase", "result"}, // phase: pending/processing/complete, result: success/failure
		),

		// 5. Reconciliation Duration (CRD controllers)
		ReconciliationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_duration_seconds",
				Help:      "Duration of reconciliation loops",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30},
			},
			[]string{"controller"},
		),

		// 6. Reconciliation Errors (CRD controllers)
		ReconciliationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_errors_total",
				Help:      "Total number of reconciliation errors",
			},
			[]string{"controller", "error_type"},
		),

		// 7. Queue Depth (if applicable)
		QueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queue_depth",
				Help:      "Current depth of processing queue",
			},
			[]string{"queue_name"},
		),

		// 8. Queue Latency (if applicable)
		QueueLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queue_latency_seconds",
				Help:      "Time items spend in queue before processing",
				Buckets:   []float64{.5, 1, 5, 10, 30, 60, 300},
			},
			[]string{"queue_name"},
		),

		// 9-10+. Business-specific metrics (customize per service)
		// Example: Notification delivery metrics
		// [CustomMetric1]: promauto.NewCounterVec(...)
		// [CustomMetric2]: promauto.NewGaugeVec(...)
	}
}
```

---

**Metric Recording Patterns in Business Logic**:

```go
// Example 1: Recording operation metrics
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
	// Start timer for duration
	timer := prometheus.NewTimer(s.metrics.OperationDuration.WithLabelValues("process_request"))
	defer timer.ObserveDuration()

	// Process request
	err := s.doProcess(ctx, req)

	// Record result
	if err != nil {
		s.metrics.OperationsTotal.WithLabelValues("process_request", "failure").Inc()
		s.metrics.ErrorsTotal.WithLabelValues(classifyError(err), "process_request").Inc()
		return err
	}

	s.metrics.OperationsTotal.WithLabelValues("process_request", "success").Inc()
	return nil
}

// Example 2: Recording CRD reconciliation metrics
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Start timer
	timer := prometheus.NewTimer(r.metrics.ReconciliationDuration.WithLabelValues("my-controller"))
	defer timer.ObserveDuration()

	// Fetch resource
	resource := &Resource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.metrics.ReconciliationErrors.WithLabelValues("my-controller", "fetch_error").Inc()
		return ctrl.Result{}, err
	}

	// Process
	if err := r.process(ctx, resource); err != nil {
		r.metrics.ResourcesProcessed.WithLabelValues(string(resource.Status.Phase), "failure").Inc()
		r.metrics.ReconciliationErrors.WithLabelValues("my-controller", "processing_error").Inc()
		return ctrl.Result{}, err
	}

	r.metrics.ResourcesProcessed.WithLabelValues(string(resource.Status.Phase), "success").Inc()
	return ctrl.Result{}, nil
}

// Example 3: Recording gauge metrics (queue depth)
func (s *Service) EnqueueItem(item *Item) error {
	s.queue.Add(item)
	s.metrics.QueueDepth.WithLabelValues("main").Set(float64(s.queue.Len()))
	return nil
}
```

---

**Metrics Endpoint Exposure**:

**For HTTP Services**:
```go
// cmd/[service]/main.go
import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

func main() {
	// Initialize metrics
	metrics := metrics.NewMetrics("kubernaut", "[service]")

	// Create service with metrics
	service := NewService(metrics)

	// Expose metrics endpoint
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":9090", nil)

	// Start main service on port 8080
	// ...
}
```

**For CRD Controllers (controller-runtime)**:
```go
// cmd/[service]/main.go
import (
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func main() {
	// Metrics automatically exposed by controller-runtime
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		MetricsBindAddress: ":9090", // Metrics endpoint
		// ...
	})

	// Initialize custom metrics
	customMetrics := metrics.NewMetrics("kubernaut", "[service]")

	// Pass to reconciler
	if err = (&Reconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Metrics: customMetrics,
	}).SetupWithManager(mgr); err != nil {
		// ...
	}
}
```

---

**Testing Metrics**:

```go
// test/unit/[service]/metrics_test.go
package [service]_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut/pkg/[service]"
	"github.com/jordigilh/kubernaut/pkg/[service]/metrics"
)

var _ = Describe("BR-XXX-XXX: Metrics Recording", func() {
	var (
		ctx     context.Context
		svc     *[service].Service
		m       *metrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create custom registry for testing (isolated)
		registry := prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(registry)
		svc = [service].NewService(m)
	})

	It("should record successful operations", func() {
		// Execute operation
		err := svc.ProcessRequest(ctx, &Request{})
		Expect(err).ToNot(HaveOccurred())

		// Verify metric recorded
		count := testutil.ToFloat64(m.OperationsTotal.WithLabelValues("process_request", "success"))
		Expect(count).To(Equal(1.0))
	})

	It("should record operation duration", func() {
		// Execute operation
		err := svc.ProcessRequest(ctx, &Request{})
		Expect(err).ToNot(HaveOccurred())

		// Verify histogram recorded (count > 0)
		count := testutil.ToFloat64(m.OperationDuration.WithLabelValues("process_request"))
		Expect(count).To(BeNumerically(">", 0))
	})

	It("should record error metrics", func() {
		// Execute operation that fails
		err := svc.ProcessRequest(ctx, &Request{Invalid: true})
		Expect(err).To(HaveOccurred())

		// Verify error metric recorded
		errorCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("user_error", "process_request"))
		Expect(errorCount).To(Equal(1.0))
	})
})
```

---

**Validation Checklist**:
- [ ] 10+ metrics defined
- [ ] Metrics registered with promauto (automatic registration)
- [ ] Labels used for dimension breakdown (operation, status, error_type)
- [ ] Histograms for duration/latency (with appropriate buckets)
- [ ] Counters for operations/errors
- [ ] Gauges for current state (queue depth, active connections)
- [ ] Metrics recorded in all business logic paths
- [ ] Metrics endpoint exposed (`:9090/metrics`)
- [ ] Metrics tested with prometheus/testutil
- [ ] ServiceMonitor created (Kubernetes Prometheus Operator)

#### Main Application Integration (2h)
- Component wiring in main.go
- Configuration loading
- Graceful shutdown

#### Critical EOD Checkpoints (1h) ⭐
- [ ] **Schema Validation**: Create `design/01-[schema]-validation.md`
- [ ] **Test Infrastructure Setup**: Create test suite skeleton
- [ ] **Status Documentation**: Create `03-day7-complete.md`
- [ ] **Testing Strategy**: Create `testing/01-integration-first-rationale.md`

**Why These Matter**: Gateway found these prevented 2+ days of debugging

---

### Day 8: Integration-First Testing ⭐ (8h)

**CRITICAL CHANGE FROM TRADITIONAL TDD**: Integration tests BEFORE unit tests

#### Morning: 5 Critical Integration Tests (4h) ⭐

**Test Infrastructure Setup**: Choose based on your `[TEST_ENVIRONMENT]` decision ⭐ **v1.3**

<details>
<summary><strong>🔴 KIND Setup (if [TEST_ENVIRONMENT] = KIND)</strong></summary>

Use **Kind Cluster Test Template** for standardized integration tests:

**Documentation**: [Kind Cluster Test Template Guide](../testing/KIND_CLUSTER_TEST_TEMPLATE.md)

```go
package myservice

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (Kind)")
}

var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
	// Use Kind template for standardized test setup
	// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
	suite = kind.Setup("[service]-test", "kubernaut-system")

	// Additional setup if needed (PostgreSQL, Redis, etc.)
	// suite.WaitForPostgreSQLReady(60 * time.Second)
})

var _ = AfterSuite(func() {
	suite.Cleanup()
})

// Integration Test Pattern
Describe("Integration Test [N]: [Scenario]", func() {
	var component *[service].Component

	BeforeEach(func() {
		// Setup real components using Kind cluster resources
		// Example: Deploy test services
		// svc, err := suite.DeployPrometheusService("[service]-test")

		// Initialize component with real dependencies
		component = [service].NewComponent(suite.Client, logger)
	})

	It("should [end-to-end behavior]", func() {
		// Complete workflow test using real Kind cluster resources
		// Example test assertion
		result, err := component.Process(suite.Context, input)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal("success"))
	})
})
```

**Key Benefits of Kind Template**:
- ✅ **15 lines setup** vs 80+ custom (81% reduction)
- ✅ **Complete imports** (copy-pasteable)
- ✅ **Kind cluster DNS** (no port-forwarding)
- ✅ **Automatic cleanup** (`suite.Cleanup()`)
- ✅ **Consistent pattern** (aligned with Gateway, Dynamic Toolset V2)
- ✅ **30+ helper methods** (services, ConfigMaps, database, wait utilities)

</details>

---

<details>
<summary><strong>🟡 ENVTEST Setup (if [TEST_ENVIRONMENT] = ENVTEST)</strong></summary>

Use **envtest** for Kubernetes API server testing without full cluster:

**Prerequisites**:
```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use 1.31.0
```

**Documentation**: [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)

```go
package myservice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (envtest)")
}

var (
	cfg       *rest.Config
	k8sClient kubernetes.Interface
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Start envtest with CRDs if needed
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd")},
		ErrorIfCRDPathMissing: false, // Set true if CRDs required
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test K8s API operations]", func() {
		// Create test resources
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}
		_, err := k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Test your service logic
		component := [service].NewComponent(k8sClient)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of envtest**:
- ✅ **Real API server** validation (schema, field selectors)
- ✅ **CRD support** (register definitions + use controller-runtime client)
- ✅ **Fast setup** (~3 seconds vs ~60 seconds for KIND)
- ✅ **Standard K8s client** (same as production)
- ⚠️ **No RBAC/TokenReview** (use KIND if needed)

</details>

---

<details>
<summary><strong>🟢 PODMAN Setup (if [TEST_ENVIRONMENT] = PODMAN)</strong></summary>

Use **testcontainers-go** for PostgreSQL/Redis/database testing:

**Prerequisites**: Docker or Podman installed

**Documentation**: [Podman Integration Test Template](../../testing/PODMAN_INTEGRATION_TEST_TEMPLATE.md)

```go
package myservice

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (Podman)")
}

var (
	postgresContainer testcontainers.Container
	redisContainer    testcontainers.Container
	dbURL             string
	redisAddr         string
)

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresReq := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections"),
	}
	var err error
	postgresContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred())

	// Get database URL
	host, _ := postgresContainer.Host(ctx)
	port, _ := postgresContainer.MappedPort(ctx, "5432")
	dbURL = fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	// Start Redis container (if needed)
	// ... similar pattern
})

var _ = AfterSuite(func() {
	ctx := context.Background()
	if postgresContainer != nil {
		postgresContainer.Terminate(ctx)
	}
	if redisContainer != nil {
		redisContainer.Terminate(ctx)
	}
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test database operations]", func() {
		// Test your service with real database
		component := [service].NewComponent(dbURL, redisAddr)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of Podman**:
- ✅ **Real databases** (PostgreSQL, Redis, etc.)
- ✅ **Fast startup** (~1-2 seconds)
- ✅ **Automatic cleanup** (testcontainers-go)
- ✅ **No Kubernetes** (simpler for pure HTTP APIs)

</details>

---

<details>
<summary><strong>⚪ HTTP MOCKS Setup (if [TEST_ENVIRONMENT] = HTTP_MOCKS)</strong></summary>

Use **net/http/httptest** for mocking external HTTP APIs:

**Prerequisites**: None (Go stdlib)

```go
package myservice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (HTTP Mocks)")
}

var (
	mockDataStorageAPI    *httptest.Server
	mockMonitoringAPI     *httptest.Server
)

var _ = BeforeSuite(func() {
	// Mock Data Storage API
	mockDataStorageAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/audit/actions" {
			json.NewEncoder(w).Encode(mockActions)
		}
	}))

	// Mock Monitoring API
	mockMonitoringAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/metrics" {
			json.NewEncoder(w).Encode(mockMetrics)
		}
	}))
})

var _ = AfterSuite(func() {
	if mockDataStorageAPI != nil {
		mockDataStorageAPI.Close()
	}
	if mockMonitoringAPI != nil {
		mockMonitoringAPI.Close()
	}
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test HTTP API interactions]", func() {
		// Test your service with mocked APIs
		component := [service].NewComponent(mockDataStorageAPI.URL, mockMonitoringAPI.URL)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of HTTP Mocks**:
- ✅ **Zero infrastructure** (no KIND, databases, containers)
- ✅ **Instant startup** (milliseconds)
- ✅ **Easy failure simulation** (return errors, timeouts)
- ✅ **Perfect for pure HTTP API services**

</details>

---

**Required Tests**:
1. **Test 1**: Basic flow (input → processing → output) - 90 min
2. **Test 2**: Deduplication/Caching logic - 45 min
3. **Test 3**: Error recovery scenario - 60 min
4. **Test 4**: Data persistence/state management - 45 min
5. **Test 5**: Authentication/Authorization - 30 min

---

### 📋 Complete Integration Test Examples ⭐ V2.0

**Purpose**: Provide production-ready integration test templates to accelerate Day 8 implementation

---

#### **Integration Test Example 1: Complete Workflow (CRD Controller)**

**File**: `test/integration/[service]/workflow_test.go`

**BR Coverage**: BR-XXX-001 (Complete workflow), BR-XXX-002 (Status tracking)

```go
package [service]_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	[service]v1alpha1 "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

var _ = Describe("Integration Test 1: Complete Workflow (Pending → Complete)", func() {
	var (
		ctx          context.Context
		resourceName string
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-workflow-" + randomString(5)
		namespace = "kubernaut-test"
	})

	AfterEach(func() {
		// Cleanup: Delete resource
		resource := &[service]v1alpha1.[Resource]{}
		if err := suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, resource); err == nil {
			suite.Client.Delete(ctx, resource)
		}
	})

	It("should complete full workflow from creation to completion", func() {
		By("Creating resource in Pending state")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "[type]",
				Priority: "high",
				Data:     map[string]string{"key": "value"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for status to transition to Processing")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal([service]v1alpha1.PhaseProcessing))

		By("Verifying processing timestamps are set")
		processing := &[service]v1alpha1.[Resource]{}
		err = suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, processing)
		Expect(err).ToNot(HaveOccurred())
		Expect(processing.Status.QueuedAt).ToNot(BeNil())
		Expect(processing.Status.ProcessingStartedAt).ToNot(BeNil())

		By("Waiting for status to transition to Complete")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal([service]v1alpha1.PhaseComplete))

		By("Verifying completion timestamp and results")
		completed := &[service]v1alpha1.[Resource]{}
		err = suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, completed)
		Expect(err).ToNot(HaveOccurred())
		Expect(completed.Status.CompletionTime).ToNot(BeNil())
		Expect(completed.Status.SuccessCount).To(BeNumerically(">", 0))
		Expect(completed.Status.ObservedGeneration).To(Equal(completed.Generation))

		By("Verifying conditions are set correctly")
		readyCondition := meta.FindStatusCondition(completed.Status.Conditions, "Ready")
		Expect(readyCondition).ToNot(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCondition.Reason).To(Equal("ProcessingComplete"))
	})
})
```

---

#### **Integration Test Example 2: Failure Recovery with Retry**

**File**: `test/integration/[service]/failure_recovery_test.go`

**BR Coverage**: BR-XXX-003 (Error recovery), BR-XXX-004 (Exponential backoff)

```go
package [service]_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	[service]v1alpha1 "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

var _ = Describe("Integration Test 2: Failure Recovery with Automatic Retry", func() {
	var (
		ctx          context.Context
		resourceName string
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-retry-" + randomString(5)
		namespace = "kubernaut-test"
	})

	AfterEach(func() {
		// Cleanup
		resource := &[service]v1alpha1.[Resource]{}
		if err := suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, resource); err == nil {
			suite.Client.Delete(ctx, resource)
		}
	})

	It("should retry on transient failures with exponential backoff", func() {
		By("Creating resource that will initially fail")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				Annotations: map[string]string{
					"test.kubernaut.ai/simulate-transient-error": "true",
					"test.kubernaut.ai/fail-attempts":            "2", // Fail first 2 attempts
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "[type]",
				Priority: "medium",
				Data:     map[string]string{"test": "retry"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for first failure attempt")
		Eventually(func() int {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.AttemptCount
		}, 15*time.Second, 1*time.Second).Should(BeNumerically(">=", 1))

		By("Verifying transient error is recorded")
		firstAttempt := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, firstAttempt)
		Expect(firstAttempt.Status.LastError).To(ContainSubstring("transient"))
		Expect(firstAttempt.Status.AttemptCount).To(BeNumerically(">=", 1))

		By("Waiting for automatic retry and eventual success")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 90*time.Second, 2*time.Second).Should(Equal([service]v1alpha1.PhaseComplete))

		By("Verifying retry attempts were made with exponential backoff")
		completed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, completed)

		// Should have 3 total attempts (2 failures + 1 success)
		Expect(completed.Status.AttemptCount).To(Equal(3))

		// Verify backoff timing between attempts
		if len(completed.Status.AttemptHistory) >= 2 {
			attempt1Time := completed.Status.AttemptHistory[0].Timestamp.Time
			attempt2Time := completed.Status.AttemptHistory[1].Timestamp.Time
			backoffDuration := attempt2Time.Sub(attempt1Time)

			// First retry should be ~30s (with jitter: 27s-33s)
			Expect(backoffDuration.Seconds()).To(BeNumerically(">=", 25))
			Expect(backoffDuration.Seconds()).To(BeNumerically("<=", 35))
		}
	})

	It("should fail permanently after max retry attempts exceeded", func() {
		By("Creating resource that always fails")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-permanent",
				Namespace: namespace,
				Annotations: map[string]string{
					"test.kubernaut.ai/simulate-permanent-error": "true",
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "[type]",
				Priority: "low",
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for status to reach Failed after max retries")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-permanent",
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 180*time.Second, 5*time.Second).Should(Equal([service]v1alpha1.PhaseFailed))

		By("Verifying max retry attempts reached")
		failed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName + "-permanent",
			Namespace: namespace,
		}, failed)
		Expect(failed.Status.AttemptCount).To(Equal(5)) // Max retry attempts
		Expect(failed.Status.LastError).To(ContainSubstring("max retry attempts exceeded"))
	})
})
```

---

#### **Integration Test Example 3: Graceful Degradation (Multi-Channel)**

**File**: `test/integration/[service]/graceful_degradation_test.go`

**BR Coverage**: BR-XXX-005 (Graceful degradation), BR-XXX-006 (Partial success)

```go
package [service]_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	[service]v1alpha1 "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

var _ = Describe("Integration Test 3: Graceful Degradation (Partial Failure)", func() {
	var (
		ctx          context.Context
		resourceName string
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-degradation-" + randomString(5)
		namespace = "kubernaut-test"
	})

	AfterEach(func() {
		// Cleanup
		resource := &[service]v1alpha1.[Resource]{}
		if err := suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, resource); err == nil {
			suite.Client.Delete(ctx, resource)
		}
	})

	It("should complete successfully when at least one channel succeeds", func() {
		By("Creating resource with multiple delivery channels")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				Annotations: map[string]string{
					// Simulate Slack failure, Console success
					"test.kubernaut.ai/fail-slack": "true",
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "notification",
				Priority: "high",
				Channels: []string{"console", "slack"},
				Data:     map[string]string{"message": "Test notification"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for resource to complete (graceful degradation)")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal([service]v1alpha1.PhaseComplete))

		By("Verifying partial success status")
		completed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, completed)

		// At least one channel succeeded
		Expect(completed.Status.SuccessfulDeliveries).To(BeNumerically(">=", 1))

		// Some channels failed
		Expect(completed.Status.FailedDeliveries).To(BeNumerically(">=", 1))

		// Total attempts = success + failures
		totalExpected := completed.Status.SuccessfulDeliveries + completed.Status.FailedDeliveries
		Expect(completed.Status.TotalAttempts).To(Equal(totalExpected))

		By("Verifying delivery attempts are recorded")
		Expect(len(completed.Status.DeliveryAttempts)).To(BeNumerically(">=", 2))

		// Find console delivery (should succeed)
		consoleDelivery := findDeliveryAttempt(completed.Status.DeliveryAttempts, "console")
		Expect(consoleDelivery).ToNot(BeNil())
		Expect(consoleDelivery.Status).To(Equal("success"))

		// Find Slack delivery (should fail)
		slackDelivery := findDeliveryAttempt(completed.Status.DeliveryAttempts, "slack")
		Expect(slackDelivery).ToNot(BeNil())
		Expect(slackDelivery.Status).To(Equal("failed"))
		Expect(slackDelivery.Error).To(ContainSubstring("slack"))
	})

	It("should fail when all channels fail", func() {
		By("Creating resource where all channels fail")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-all-fail",
				Namespace: namespace,
				Annotations: map[string]string{
					"test.kubernaut.ai/fail-all-channels": "true",
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "notification",
				Priority: "medium",
				Channels: []string{"console", "slack"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for resource to fail (all channels failed)")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-all-fail",
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 60*time.Second, 2*time.Second).Should(Equal([service]v1alpha1.PhaseFailed))

		By("Verifying all deliveries failed")
		failed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName + "-all-fail",
			Namespace: namespace,
		}, failed)

		Expect(failed.Status.SuccessfulDeliveries).To(Equal(0))
		Expect(failed.Status.FailedDeliveries).To(BeNumerically(">=", 2))
		Expect(failed.Status.LastError).To(ContainSubstring("all channels failed"))
	})
})

// Helper function
func findDeliveryAttempt(attempts []v1alpha1.DeliveryAttempt, channel string) *v1alpha1.DeliveryAttempt {
	for i := range attempts {
		if attempts[i].Channel == channel {
			return &attempts[i]
		}
	}
	return nil
}

func randomString(length int) string {
	// Simple random string generator for test names
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
```

---

**Integration Test Best Practices Applied**:

✅ **Complete Examples**: All 3 tests are production-ready, copy-pasteable code
✅ **Real Infrastructure**: Tests use actual Kind cluster, not mocks
✅ **Comprehensive Assertions**: Multiple verification points per test
✅ **Proper Cleanup**: AfterEach ensures no resource leaks
✅ **Timing Considerations**: Eventually() with appropriate timeouts
✅ **BR Coverage**: Each test maps to specific business requirements
✅ **Failure Scenarios**: Tests cover success, transient failure, permanent failure, partial success
✅ **Production-Ready**: Error handling, contexts, proper resource management

**Total Integration Test Code**: ~400 lines of complete, production-ready examples

---

**Validation After Integration Tests**:
- [ ] Architecture validated
- [ ] Integration issues found early
- [ ] Timing/concurrency issues identified
- [ ] Ready for unit test details

#### Afternoon: Unit Tests Part 1 (4h)
- Focus on components tested in integration tests
- Fill in edge cases
- Add negative test cases

**Metrics Validation Checkpoint**:
```bash
curl http://localhost:9090/metrics | grep [service]_
```

---

### Day 9: Unit Tests Part 2 (8h)

#### Morning: Unit Tests - [Component Group 1] (4h)
- Edge cases
- Error conditions
- Boundary values

#### Afternoon: Unit Tests - [Component Group 2] (4h)
- Mock dependencies
- Timeout scenarios
- Concurrent access

**EOD: Create BR Coverage Matrix** ⭐ **V2.0 ENHANCED**
**File**: `implementation/testing/BR-COVERAGE-MATRIX.md`

---

### Enhanced BR Coverage Matrix Template (Complete)

```markdown
# BR Coverage Matrix - [Service Name]

**Date**: YYYY-MM-DD
**Status**: Complete coverage validation
**Target Coverage**: ≥97% (V3.0 standard)

---

## 📊 **Coverage Summary**

### Overall Coverage Calculation

**Formula**: `(BRs with tests / Total BRs) * 100 = Coverage %`

**Example**:
- Total BRs: 9
- BRs with unit tests: 9 (100%)
- BRs with integration tests: 8 (89%)
- BRs with E2E tests: 4 (44%)
- **Overall BR Coverage**: 9/9 = **100%** ✅

### Coverage By Test Type

| Test Type | BR Coverage | Test Count | Code Coverage | Status |
|-----------|-------------|------------|---------------|--------|
| **Unit Tests** | 100% (9/9 BRs) | 50+ tests | ~75% | ✅ **Target: >70%** |
| **Integration Tests** | 89% (8/9 BRs) | 5 critical tests | ~60% | ✅ **Target: >50%** |
| **E2E Tests** | 44% (4/9 BRs) | 1 comprehensive test | ~15% | ✅ **Target: <10%** |

**Overall Test Quality**: **97.2% BR coverage** ✅

---

## 🔍 **Per-BR Coverage Breakdown**

### **BR-[CATEGORY]-001: [Requirement Name]**

**Requirement**: [Full requirement description]

#### Unit Tests
- **File**: `test/unit/[service]/[component]_test.go`
- **Tests**:
  - `It("should [behavior 1]")` - Lines 45-62
  - `It("should [behavior 2]")` - Lines 64-78
  - `DescribeTable("should handle multiple scenarios")` - Lines 80-95 (5 scenarios)
- **Coverage**: 3 tests + 5 table entries = **8 test cases** ✅

#### Integration Tests
- **File**: `test/integration/[service]/[scenario]_test.go`
- **Tests**:
  - `It("should [integration scenario]")` - Lines 120-155
- **Coverage**: 1 integration test ✅

#### E2E Tests
- **File**: `test/e2e/[service]/[workflow]_test.go`
- **Tests**:
  - Covered in comprehensive E2E workflow
- **Coverage**: Implicit ✅

**Status**: ✅ **100% Coverage** (unit + integration + E2E)

---

### **BR-[CATEGORY]-002: [Requirement Name]**

**Requirement**: [Full requirement description]

#### Unit Tests
- **File**: `test/unit/[service]/[component]_test.go`
- **Tests**:
  - `DescribeTable("should handle [scenarios]")` - Lines 100-125 (7 entries)
- **Coverage**: 7 table-driven test cases ✅

#### Integration Tests
- **File**: `test/integration/[service]/[scenario]_test.go`
- **Tests**:
  - `It("should [integration scenario]")` - Lines 200-240
- **Coverage**: 1 integration test ✅

**Status**: ✅ **100% Coverage** (unit + integration)

---

### **BR-[CATEGORY]-003: [Requirement Name]**

[Repeat pattern for each BR...]

---

## 📈 **Coverage Gap Analysis**

### ✅ **Fully Covered BRs** (100% coverage)

| BR | Requirement | Unit | Integration | E2E | Status |
|----|-------------|------|-------------|-----|--------|
| BR-XXX-001 | [Req 1] | 8 tests | 1 test | ✅ | ✅ Complete |
| BR-XXX-002 | [Req 2] | 7 tests | 1 test | ❌ | ✅ Complete |
| BR-XXX-003 | [Req 3] | 5 tests | 1 test | ✅ | ✅ Complete |
| ...

**Count**: 9/9 BRs (100%) ✅

### ⚠️ **Partially Covered BRs** (50-99% coverage)

**None** - All BRs fully covered ✅

### ❌ **Uncovered BRs** (0-49% coverage)

**None** - All BRs fully covered ✅

---

## 🎯 **Testing Strategy Validation**

### Unit Test Coverage (Target: >70%)

**Achieved**: ~75% code coverage ✅

**Coverage By Component**:
- [Component 1]: 80% coverage (50 tests)
- [Component 2]: 75% coverage (35 tests)
- [Component 3]: 70% coverage (25 tests)

**Status**: ✅ **Exceeds target**

### Integration Test Coverage (Target: >50%)

**Achieved**: ~60% scenario coverage ✅

**Critical Scenarios Covered**:
1. Complete workflow (Pending → Sent)
2. Failure recovery with retry
3. Graceful degradation (partial failure)
4. Status tracking (multiple attempts)
5. Priority handling (critical vs low)

**Status**: ✅ **Exceeds target**

### E2E Test Coverage (Target: <10%)

**Achieved**: ~15% production scenarios ✅

**Critical Paths Covered**:
- End-to-end workflow with real external dependencies

**Status**: ✅ **Within acceptable range**

---

## 📊 **Test Distribution Analysis**

### Test Count By Type

| Type | Count | Percentage | Target | Status |
|------|-------|------------|--------|--------|
| Unit Tests | 50+ | ~70% | 70%+ | ✅ Met |
| Integration Tests | 5 | ~20% | >50% coverage | ✅ Met |
| E2E Tests | 1 | ~10% | <10% | ✅ Met |

**Total Tests**: 56+ tests covering 9 business requirements

---

## ✅ **Validation Checklist**

Before releasing:
- [ ] All BRs mapped to tests ✅
- [ ] Unit test coverage >70% ✅
- [ ] Integration test coverage >50% ✅
- [ ] E2E test coverage >10% (but <20%) ✅
- [ ] No BRs with 0% coverage ✅
- [ ] Critical paths tested ✅
- [ ] Failure scenarios tested ✅
- [ ] Table-driven tests used where applicable ✅
- [ ] All test files documented in this matrix ✅

**Status**: ✅ **Ready for Production** (97.2% BR coverage)

---

## 📝 **Test File Reference Index**

### Unit Tests
- `test/unit/[service]/[component1]_test.go` - BR-XXX-001, BR-XXX-002
- `test/unit/[service]/[component2]_test.go` - BR-XXX-003, BR-XXX-004
- `test/unit/[service]/[component3]_test.go` - BR-XXX-005, BR-XXX-006

### Integration Tests
- `test/integration/[service]/suite_test.go` - Setup and teardown
- `test/integration/[service]/workflow_test.go` - BR-XXX-001 integration
- `test/integration/[service]/failure_test.go` - BR-XXX-002, BR-XXX-003 integration
- `test/integration/[service]/degradation_test.go` - BR-XXX-004 integration

### E2E Tests
- `test/e2e/[service]/end_to_end_test.go` - BR-XXX-001, BR-XXX-003, BR-XXX-005, BR-XXX-007

---

## 🔄 **Coverage Maintenance**

### When to Update This Matrix
- After adding new business requirements
- After implementing new tests
- Before release (validation checkpoint)
- During code reviews

### Coverage Targets
- **Unit**: Maintain >70% (increase to 75%+ for complex services)
- **Integration**: Maintain >50% (increase to 60%+ for CRD controllers)
- **E2E**: Keep <10% (only critical user journeys)

### Quality Indicators
- ✅ **Excellent**: >95% BR coverage (V3.0 standard)
- ✅ **Good**: 90-95% BR coverage
- ⚠️ **Acceptable**: 85-90% BR coverage
- ❌ **Insufficient**: <85% BR coverage

**Current Status**: **97.2%** = ✅ **Excellent** (V3.0 standard achieved)

---

**Confidence Assessment**: 98%
- **Evidence**: Complete BR-to-test mapping, all coverage targets exceeded
- **Risk**: Minimal - comprehensive test coverage validated
- **Next Steps**: Monitor coverage during future development, update matrix when adding new BRs
```

---

**Validation**:
- [ ] BR Coverage Matrix complete with calculation methodology
- [ ] All BRs mapped to specific test files and line numbers
- [ ] Coverage calculation formula documented
- [ ] Per-BR breakdown included
- [ ] Coverage gap analysis completed
- [ ] Test distribution analysis validated
- [ ] Coverage maintenance plan documented
- [ ] Unit test coverage >70% validated
- [ ] Integration test coverage >50% validated
- [ ] Overall BR coverage ≥97% achieved

---

### Day 10: Advanced Integration + E2E Tests (8h)

#### Advanced Integration Tests (4h)
- Concurrent request scenarios
- Resource exhaustion
- Long-running operations
- Failure recovery

#### E2E Test Setup (2h)
- Kind cluster setup
- Service deployment
- Dependencies deployment

#### E2E Test Execution (2h)
- Complete workflow tests
- Real environment validation

---

### Day 11: Comprehensive Documentation (8h) ⭐ V2.0 ENHANCED

**Purpose**: Create production-ready documentation that enables smooth handoffs and operational excellence

---

#### Implementation Documentation (4h)

**File 1**: `docs/services/[service]/README.md` (Complete Service Overview)

```markdown
# [Service Name] Service

## Overview
**Service Type**: [CRD Controller | Stateless Service]
**Purpose**: [One-sentence business purpose]
**Dependencies**: [List critical dependencies]
**Status**: ✅ Production-Ready | 🚧 In Development

## Quick Start
\`\`\`bash
# Build
make build-[service]

# Run locally
./_bin/[service] --config config/development.yaml

# Run tests
make test-[service]
\`\`\`

## Architecture

### Component Diagram
\`\`\`
[External Input] → [Main Handler] → [Business Logic] → [External Output]
                       ↓                    ↓
                  [Metrics]          [Database/Cache]
\`\`\`

### Key Components
- **[Component1]**: [Purpose and responsibility]
- **[Component2]**: [Purpose and responsibility]
- **[Component3]**: [Purpose and responsibility]

### Data Flow
1. [Step 1 description]
2. [Step 2 description]
3. [Step 3 description]

## Configuration

### Required Environment Variables
\`\`\`yaml
# config/development.yaml example
[service]:
  setting1: "value1"  # [Description of what this controls]
  setting2: 30        # [Default: 30, range: 10-300]
  setting3: true      # [Enable/disable specific feature]
\`\`\`

### Configuration Options
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `setting1` | string | required | [What it does] |
| `setting2` | int | 30 | [Valid range and impact] |
| `setting3` | bool | true | [When to enable/disable] |

## API Reference (for HTTP services)

### Endpoints

#### POST /api/v1/[resource]
**Purpose**: [What this endpoint does]

**Request**:
\`\`\`json
{
  "field1": "value1",
  "field2": 123
}
\`\`\`

**Response**:
\`\`\`json
{
  "id": "generated-id",
  "status": "processing"
}
\`\`\`

**Error Codes**:
- `400`: [When this happens]
- `500`: [When this happens]

## CRD Reference (for CRD controllers)

### Resource Spec
\`\`\`yaml
apiVersion: [group]/v1alpha1
kind: [ResourceKind]
metadata:
  name: example
  namespace: kubernaut
spec:
  field1: "value1"     # [Description]
  field2: 123          # [Valid range]
  field3:              # [Complex field description]
    nestedField: "value"
\`\`\`

### Status Fields
| Field | Type | Description |
|-------|------|-------------|
| `phase` | Phase | Current processing phase |
| `conditions` | []Condition | Detailed status conditions |
| `observedGeneration` | int64 | Last processed generation |

### Phase Lifecycle
\`\`\`
Pending → Processing → Complete
            ↓
          Failed (with retry)
\`\`\`

## Integration Guide

### Integrating with [Service]

**Step 1**: [First integration step]
\`\`\`go
// Example code
import "[service]"

service := [service].New(config)
\`\`\`

**Step 2**: [Second integration step]
\`\`\`go
// Example usage
result, err := service.Process(ctx, request)
\`\`\`

## Observability

### Metrics
Service exposes Prometheus metrics at `:9090/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `[service]_operations_total` | Counter | Total operations processed |
| `[service]_operation_duration_seconds` | Histogram | Operation latency |
| `[service]_errors_total` | Counter | Total errors by type |

### Health Checks
- **Liveness**: `GET /healthz` - Always returns 200 if process is running
- **Readiness**: `GET /readyz` - Returns 200 when ready to serve traffic

### Logging
Structured logging using `logr`:
\`\`\`json
{
  "level": "info",
  "ts": "2025-10-12T10:30:00Z",
  "msg": "Processing request",
  "request_id": "uuid",
  "operation": "process"
}
\`\`\`

## Troubleshooting

### Common Issues

**Issue**: [Problem description]
- **Symptom**: [What you see]
- **Cause**: [Why it happens]
- **Fix**: [How to resolve]

**Issue**: [Another problem]
- **Symptom**: [What you see]
- **Cause**: [Why it happens]
- **Fix**: [How to resolve]

## Development

### Running Tests
\`\`\`bash
# Unit tests
make test-unit-[service]

# Integration tests (requires Kind cluster)
make test-integration-[service]

# E2E tests
make test-e2e-[service]
\`\`\`

### Local Development Setup
1. [Setup step 1]
2. [Setup step 2]
3. [Setup step 3]

## References
- [Business Requirements](../../requirements/BR-[SERVICE]-REQUIREMENTS.md)
- [Design Decisions](./implementation/DESIGN_DECISIONS.md)
- [Implementation Plan](./implementation/IMPLEMENTATION_PLAN_V3.0.md)
- [Production Readiness](./implementation/PRODUCTION_READINESS_REPORT.md)
\`\`\`

---

#### Design Decision Documentation (2h) ⭐ V2.0 ENHANCED

**File 2**: `docs/services/[service]/implementation/DESIGN_DECISIONS.md`

```markdown
# [Service] Design Decisions

## DD-[SERVICE]-001: [Decision Title]

**Date**: 2025-10-12
**Status**: ✅ Accepted | 🚧 Proposed | ❌ Rejected
**Deciders**: [Names]

### Context
[Describe the problem or situation requiring a decision]

### Decision
[State the decision clearly]

### Alternatives Considered

#### Option A: [Alternative 1]
**Pros**:
- [Advantage 1]
- [Advantage 2]

**Cons**:
- [Disadvantage 1]
- [Disadvantage 2]

#### Option B: [Alternative 2]
**Pros**:
- [Advantage 1]

**Cons**:
- [Disadvantage 1]

#### Option C: Selected - [Chosen Option]
**Pros**:
- [Why this is best]
- [Specific advantage]

**Cons**:
- [Acknowledged tradeoffs]

### Rationale
[Explain why Option C was chosen despite tradeoffs]

### Consequences
**Positive**:
- [Expected benefit 1]
- [Expected benefit 2]

**Negative**:
- [Accepted limitation 1]
- [Mitigation strategy]

### Implementation Notes
- [Key implementation detail 1]
- [Key implementation detail 2]

### References
- [Related BR-XXX-XXX]
- [Related code: pkg/[service]/[file].go]
```

---

#### Testing Documentation (2h) ⭐ V2.0 ENHANCED

**File 3**: `docs/services/[service]/implementation/TESTING_STRATEGY.md`

```markdown
# [Service] Testing Strategy

## Test Coverage Overview

### Coverage by Test Type
| Test Type | Count | Coverage | Confidence |
|-----------|-------|----------|------------|
| Unit Tests | [N] | 70-75% | 85-90% |
| Integration Tests | [M] | 15-20% | 80-85% |
| E2E Tests | [K] | 5-10% | 90-95% |

### Coverage by Component
| Component | Unit | Integration | E2E | Total Coverage |
|-----------|------|-------------|-----|----------------|
| [Component1] | 80% | 15% | 5% | 100% |
| [Component2] | 75% | 20% | 5% | 100% |
| [Component3] | 70% | 25% | 5% | 100% |

## Test Infrastructure

### Unit Tests
**Framework**: Ginkgo/Gomega
**Mocks**: Real business logic, mock external dependencies only
**Location**: `test/unit/[service]/`

### Integration Tests
**Framework**: Ginkgo/Gomega + Kind cluster
**Infrastructure**: Real Kubernetes API (Kind), real database (Testcontainers)
**Location**: `test/integration/[service]/`

### E2E Tests
**Framework**: Ginkgo/Gomega + full deployment
**Infrastructure**: Complete Kind cluster with all services
**Location**: `test/e2e/[service]/`

## Test Scenarios

### Critical Paths (Must Test)
1. [Happy path scenario]
   - **Coverage**: Unit + Integration + E2E
   - **Files**: `test/*/[service]/happy_path_test.go`

2. [Error recovery scenario]
   - **Coverage**: Unit + Integration
   - **Files**: `test/*/[service]/error_recovery_test.go`

3. [Concurrent operations scenario]
   - **Coverage**: Integration + E2E
   - **Files**: `test/*/[service]/concurrent_test.go`

### Edge Cases (Unit Tests)
- [Edge case 1]
- [Edge case 2]
- [Edge case 3]

## Known Limitations

### Test Coverage Gaps
- **Gap 1**: [Description of what's not tested]
  - **Reason**: [Why it's not tested]
  - **Mitigation**: [How risk is managed]
  - **Future Work**: [Plan to address]

- **Gap 2**: [Another untested scenario]
  - **Reason**: [Justification]
  - **Mitigation**: [Risk management]

### Test Infrastructure Limitations
- **Limitation 1**: [Infrastructure constraint]
  - **Impact**: [What this affects]
  - **Workaround**: [How we handle it]

## Running Tests

### Quick Test Commands
\`\`\`bash
# All tests
make test-[service]

# By type
make test-unit-[service]
make test-integration-[service]
make test-e2e-[service]

# Specific test
go test -v ./test/unit/[service]/ -ginkgo.focus="specific test"
\`\`\`

### CI/CD Integration
\`\`\`yaml
# .github/workflows/[service]-tests.yml
- name: Unit Tests
  run: make test-unit-[service]

- name: Integration Tests
  run: make test-integration-[service]
  # Requires Kind cluster setup
\`\`\`

## Test Maintenance

### Adding New Tests
1. Identify BR-XXX-XXX requirement
2. Determine appropriate test type (unit/integration/E2E)
3. Follow table-driven pattern if applicable
4. Update BR Coverage Matrix

### Updating Existing Tests
1. Maintain BR-XXX-XXX mapping in test description
2. Keep test names descriptive
3. Update documentation if behavior changes

## References
- [BR Coverage Matrix](./BR_COVERAGE_MATRIX.md)
- [Integration Test Examples](../../../test/integration/[service]/)
```

---

**Documentation Validation Checklist**:
- [ ] README.md complete with all sections
- [ ] Configuration examples tested and accurate
- [ ] API/CRD reference matches implementation
- [ ] Design decisions documented with DD-XXX format
- [ ] Testing strategy reflects actual test coverage
- [ ] Known limitations documented with mitigation
- [ ] Troubleshooting guide includes common issues
- [ ] All code examples are production-ready
- [ ] References to other docs are valid links

---

### Day 12: CHECK Phase + Production Readiness ⭐ V2.0 COMPREHENSIVE (8h)

#### CHECK Phase Validation (2h)
**Checklist**:
- [ ] All business requirements met
- [ ] Build passes without errors
- [ ] All tests passing
- [ ] Metrics exposed and validated
- [ ] Health checks functional
- [ ] Authentication working
- [ ] Documentation complete
- [ ] No lint errors

#### Production Readiness Checklist (2h) ⭐ V2.0 COMPREHENSIVE
**File**: `docs/services/[service]/implementation/PRODUCTION_READINESS_REPORT.md`

```markdown
# [Service] Production Readiness Assessment

**Assessment Date**: 2025-10-12
**Assessment Status**: ✅ Production-Ready | 🚧 Partially Ready | ❌ Not Ready
**Overall Score**: XX/100 (target 95+)

---

## 1. Functional Validation (Weight: 30%)

### 1.1 Critical Path Testing
- [ ] **Happy path** - Complete workflow from input to success output
  - **Test**: `test/integration/[service]/workflow_test.go`
  - **Evidence**: All phases transition correctly (Pending → Processing → Complete)
  - **Score**: X/10

- [ ] **Error recovery** - Transient failure with automatic retry
  - **Test**: `test/integration/[service]/failure_recovery_test.go`
  - **Evidence**: Exponential backoff working, retries succeed after transient errors
  - **Score**: X/10

- [ ] **Permanent failure** - Failure after max retries
  - **Test**: `test/integration/[service]/failure_recovery_test.go`
  - **Evidence**: Fails gracefully after 5 retry attempts, status reflects failure
  - **Score**: X/10

### 1.2 Edge Cases and Boundary Conditions
- [ ] **Empty/nil inputs** - Handles missing or invalid data
  - **Test**: `test/unit/[service]/validation_test.go`
  - **Evidence**: Proper validation errors, no panics
  - **Score**: X/5

- [ ] **Large payloads** - Handles maximum expected data size
  - **Test**: `test/unit/[service]/large_payload_test.go`
  - **Evidence**: Processes 10MB payloads without memory issues
  - **Score**: X/5

- [ ] **Concurrent operations** - Thread-safe under concurrent load
  - **Test**: `test/integration/[service]/concurrent_test.go`
  - **Evidence**: 100 concurrent operations complete successfully
  - **Score**: X/5

### 1.3 Graceful Degradation
- [ ] **Partial success** - System continues with partial functionality
  - **Test**: `test/integration/[service]/graceful_degradation_test.go`
  - **Evidence**: Completes when at least 1 channel succeeds, records failures
  - **Score**: X/5

### Functional Validation Score: XX/35 (Target: 32+)

---

## 2. Operational Validation (Weight: 25%)

### 2.1 Observability - Metrics
- [ ] **10+ Prometheus metrics** defined and exported
  - **File**: `pkg/[service]/metrics/metrics.go`
  - **Endpoint**: `:9090/metrics`
  - **Evidence**: `curl localhost:9090/metrics | grep [service]_ | wc -l` returns 10+
  - **Score**: X/5

- [ ] **Metrics recorded** in all business logic paths
  - **Test**: `test/unit/[service]/metrics_test.go`
  - **Evidence**: All operations increment counters, duration histograms populated
  - **Score**: X/5

- [ ] **Metric labels** provide useful dimension breakdown
  - **Evidence**: Labels include operation, status, error_type for debugging
  - **Score**: X/3

### 2.2 Observability - Logging
- [ ] **Structured logging** using logr throughout
  - **Evidence**: All log entries include context (request_id, resource, operation)
  - **Score**: X/4

- [ ] **Log levels** appropriate (Info for normal, Error for failures)
  - **Evidence**: No Debug logs in production code, errors always logged
  - **Score**: X/3

### 2.3 Observability - Health Checks
- [ ] **Liveness probe** - Returns 200 when process is alive
  - **Endpoint**: `GET /healthz`
  - **Test**: `curl localhost:8080/healthz` returns 200
  - **Score**: X/3

- [ ] **Readiness probe** - Returns 200 when ready to serve traffic
  - **Endpoint**: `GET /readyz`
  - **Test**: Checks database connectivity, returns 503 if unhealthy
  - **Score**: X/3

### 2.4 Graceful Shutdown
- [ ] **Signal handling** - SIGTERM/SIGINT handled gracefully
  - **Evidence**: In-flight requests complete before shutdown (30s grace period)
  - **Test**: Manual test with `kill -TERM <pid>`
  - **Score**: X/3

### Operational Validation Score: XX/29 (Target: 27+)

---

## 3. Security Validation (Weight: 15%)

### 3.1 RBAC Permissions
- [ ] **Minimal permissions** - Service has only required Kubernetes permissions
  - **File**: `config/rbac/role.yaml`
  - **Evidence**: No wildcard permissions (`*`), no cluster-admin
  - **Score**: X/5

- [ ] **ServiceAccount** properly configured with RBAC
  - **File**: `deploy/manifests/[service]-deployment.yaml`
  - **Evidence**: Custom ServiceAccount with role binding
  - **Score**: X/3

### 3.2 Secret Management
- [ ] **No hardcoded secrets** in code or configuration
  - **Evidence**: Code review confirms all secrets from Kubernetes Secrets
  - **Score**: X/4

- [ ] **Secrets documented** with examples (not actual values)
  - **File**: `deploy/manifests/[service]-secret.yaml.example`
  - **Score**: X/3

### Security Validation Score: XX/15 (Target: 14+)

---

## 4. Performance Validation (Weight: 15%)

### 4.1 Latency
- [ ] **P50 latency** < 100ms for normal operations
  - **Test**: `go test -bench=BenchmarkProcess -benchmem`
  - **Evidence**: Benchmark results show 50ms P50
  - **Score**: X/5

- [ ] **P99 latency** < 500ms for normal operations
  - **Evidence**: Benchmark results show 300ms P99
  - **Score**: X/5

### 4.2 Throughput
- [ ] **Throughput** meets requirements (e.g., 100 ops/sec)
  - **Test**: Load test with 100 concurrent requests
  - **Evidence**: Sustains 150 ops/sec without errors
  - **Score**: X/5

### Performance Validation Score: XX/15 (Target: 13+)

---

## 5. Deployment Validation (Weight: 15%)

### 5.1 Kubernetes Manifests
- [ ] **Deployment manifest** complete with resource limits
  - **File**: `deploy/manifests/[service]-deployment.yaml`
  - **Evidence**: CPU (100m-500m), Memory (128Mi-512Mi) limits set
  - **Score**: X/4

- [ ] **ConfigMap** for configuration management
  - **File**: `deploy/manifests/[service]-configmap.yaml`
  - **Evidence**: All runtime config externalized
  - **Score**: X/3

- [ ] **Service manifest** (if applicable)
  - **File**: `deploy/manifests/[service]-service.yaml`
  - **Evidence**: Service exposes ports for HTTP (8080) and metrics (9090)
  - **Score**: X/3

### 5.2 Probes Configuration
- [ ] **Liveness probe** configured with appropriate thresholds
  - **Evidence**: `periodSeconds: 10, failureThreshold: 3`
  - **Score**: X/3

- [ ] **Readiness probe** configured with appropriate thresholds
  - **Evidence**: `periodSeconds: 5, failureThreshold: 3`
  - **Score**: X/2

### Deployment Validation Score: XX/15 (Target: 14+)

---

## 6. Documentation Quality (Weight: 10% bonus, not in total)

- [ ] **README.md** comprehensive with all sections
  - **Score**: X/3

- [ ] **Design Decisions** documented with DD-XXX format
  - **Score**: X/2

- [ ] **Testing Strategy** reflects actual implementation
  - **Score**: X/2

- [ ] **Troubleshooting Guide** includes common issues
  - **Score**: X/3

### Documentation Score: XX/10 (Bonus: adds to overall score)

---

## Overall Production Readiness Assessment

**Total Score**: XX/109 (Functional:35 + Operational:29 + Security:15 + Performance:15 + Deployment:15)
**With Documentation Bonus**: XX/119

**Production Readiness Level**:
- **95-100%** (113+): ✅ **Production-Ready** - Deploy to production immediately
- **85-94%** (101-112): 🚧 **Mostly Ready** - Minor improvements needed
- **75-84%** (89-100): ⚠️ **Needs Work** - Address gaps before production
- **<75%** (<89): ❌ **Not Ready** - Significant work required

**Current Level**: [✅ Production-Ready | 🚧 Mostly Ready | ⚠️ Needs Work | ❌ Not Ready]

---

## Critical Gaps (Score < Target)

### Gap 1: [Area where score is below target]
- **Current Score**: X/Y (Target: Z)
- **Missing**: [What's missing]
- **Impact**: [Risk if not addressed]
- **Mitigation**: [Plan to address]

### Gap 2: [Another gap]
- **Current Score**: X/Y
- **Missing**: [Description]
- **Impact**: [Risk assessment]
- **Mitigation**: [Action plan]

---

## Risks and Mitigations

### Risk 1: [Identified risk]
- **Probability**: Low | Medium | High
- **Impact**: Low | Medium | High
- **Mitigation**: [Specific mitigation strategy]
- **Owner**: [Responsible person/team]

### Risk 2: [Another risk]
- **Probability**: [Level]
- **Impact**: [Level]
- **Mitigation**: [Strategy]

---

## Production Deployment Recommendation

### Go/No-Go Decision
**Recommendation**: ✅ GO | 🚧 GO with caveats | ❌ NO-GO

**Justification**:
[Explain the recommendation based on scores, gaps, and risks]

### Pre-Deployment Checklist
- [ ] All critical gaps addressed
- [ ] High-priority risks mitigated
- [ ] Deployment manifests reviewed
- [ ] Rollback plan documented
- [ ] Monitoring dashboards configured
- [ ] On-call team briefed

### Post-Deployment Monitoring
- Monitor metrics dashboard for 24 hours
- Watch for error rate spikes in `[service]_errors_total`
- Track latency in `[service]_operation_duration_seconds`
- Review logs for unexpected ERROR entries

**Monitoring Dashboard**: [Link to Grafana/Prometheus dashboard]
```

#### File Organization (1h) ⭐
**File**: `implementation/FILE_ORGANIZATION_PLAN.md`

Categorize all files:
- Production implementation (pkg/, cmd/)
- Unit tests (test/unit/)
- Integration tests (test/integration/)
- E2E tests (test/e2e/)
- Configuration (deploy/)
- Documentation (docs/)

**Git commit strategy**:
```
Commit 1: Foundation (types, interfaces)
Commit 2: Component 1
Commit 3: Component 2
...
Commit N: Tests
Commit N+1: Documentation
Commit N+2: Deployment manifests
```

#### Performance Benchmarking (1h) ⭐
**File**: `implementation/PERFORMANCE_REPORT.md`

```bash
go test -bench=. -benchmem ./pkg/[service]/...
```

Validate:
- [ ] Latency targets met
- [ ] Throughput targets met
- [ ] Memory usage acceptable
- [ ] CPU usage acceptable

#### Troubleshooting Guide (1h) ⭐
**File**: `implementation/TROUBLESHOOTING_GUIDE.md`

For each common issue:
- **Symptoms**: What the user sees
- **Diagnosis**: How to investigate
- **Resolution**: How to fix

#### Confidence Assessment (1h) ⭐
**File**: `implementation/CONFIDENCE_ASSESSMENT.md`

```markdown
## Confidence Assessment

**Implementation Accuracy**: X% (target 90%+)
**Evidence**: [spec compliance, code review results]

**Test Coverage**:
- Unit: X% (target 70%+)
- Integration: X% (target 50%+)
- E2E: X% (target <10%)

**Business Requirement Coverage**: X% (target 100%)
**Mapped BRs**: [count]
**Untested BRs**: [count with justification]

**Production Readiness**: X% (target 95%+)
**Risks**: [list with mitigations]
```

#### Handoff Summary (Last Step) ⭐ V2.0 COMPREHENSIVE
**File**: `docs/services/[service]/implementation/00-HANDOFF-SUMMARY.md`

```markdown
# [Service] Implementation Handoff Summary

**Service Name**: [Service Full Name]
**Implementation Dates**: [Start Date] - [End Date]
**Implementation Team**: [Team Members]
**Handoff Date**: 2025-10-12
**Document Status**: ✅ Complete

---

## Executive Summary

**What Was Built**:
[2-3 sentence summary of what was accomplished and why it matters]

**Current Status**: ✅ Production-Ready | 🚧 Beta | 🔬 Experimental

**Production Readiness Score**: XX/119 ([Percentage]%)

**Key Achievement**: [One sentence highlighting the main accomplishment]

---

## Implementation Overview

### Scope Accomplished
✅ **Phase 1 (Days 1-3)**: Foundation and types
- [X] packages created with [Y] interfaces
- CRD schema defined and generated
- Configuration structure established

✅ **Phase 2 (Days 4-7)**: Business logic implementation
- [N] core components implemented
- Error handling with exponential backoff retry
- 10+ Prometheus metrics integrated
- Health checks and graceful shutdown

✅ **Phase 3 (Days 8-10)**: Testing
- [N] integration tests (covering happy path, error recovery, graceful degradation)
- [M] unit tests (70-75% coverage)
- [K] E2E tests (<10% coverage)

✅ **Phase 4 (Days 11-12)**: Documentation + Production Readiness
- Complete README with API reference
- Design decisions documented (DD-XXX format)
- Production readiness assessment completed
- Deployment manifests finalized

### Scope Deferred (If Any)
- [Feature 1]: Deferred to [Version/Date] - [Reason]
- [Feature 2]: Deferred to [Version/Date] - [Reason]

---

## Architecture Summary

### Component Diagram
\`\`\`
[High-level component diagram showing key components and interactions]

Example:
┌──────────────┐
│  Controller  │ (watches CRs)
└──────┬───────┘
       │
       ▼
┌──────────────┐         ┌──────────────┐
│   Manager    │────────▶│  Processor   │
└──────────────┘         └──────┬───────┘
                                │
                                ▼
                         ┌──────────────┐
                         │   Delivery   │ (external API calls)
                         └──────────────┘
\`\`\`

### Key Components
| Component | Purpose | Key Files |
|-----------|---------|-----------|
| **Controller** | Reconciles CRs | `pkg/[service]/controller/reconciler.go` |
| **Manager** | Orchestrates processing | `pkg/[service]/manager/manager.go` |
| **Processor** | Business logic | `pkg/[service]/processor/processor.go` |
| **Delivery** | External integrations | `pkg/[service]/delivery/*.go` |

### Data Flow
1. **Trigger**: User creates CR / HTTP request arrives
2. **Validation**: Input validated, defaults applied
3. **Processing**: Business logic executes with retry on transient errors
4. **Delivery**: Results delivered to external systems
5. **Status Update**: CR status updated with results

---

## Business Requirements Coverage

### Implemented Requirements
- ✅ **BR-[SERVICE]-001**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-002**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-003**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-004**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-005**: [Requirement description] - Fully implemented

**Total**: [N]/[M] business requirements implemented (100%)

### Deferred/Out-of-Scope Requirements
- ⏸️ **BR-[SERVICE]-XXX**: [Requirement] - Deferred to v2.0 ([Justification])

---

## Key Design Decisions

### DD-[SERVICE]-001: [Major Decision Title]
**Decision**: [What was decided]
**Rationale**: [Why this approach was chosen]
**Alternatives**: [What other options were considered]
**Impact**: [How this affects the system]

### DD-[SERVICE]-002**: [Another Decision]
**Decision**: [What was decided]
**Rationale**: [Why]
**Impact**: [Effects]

[Refer to full document: `DESIGN_DECISIONS.md`]

---

## Key Files and Locations

### Production Code
- **Main Entry Point**: `cmd/[service]/main.go`
- **Core Business Logic**: `pkg/[service]/processor/processor.go`
- **CRD Controller**: `pkg/[service]/controller/reconciler.go`
- **API Types**: `api/[service]/v1alpha1/*_types.go`
- **Metrics**: `pkg/[service]/metrics/metrics.go`

### Tests
- **Integration Tests**: `test/integration/[service]/` ([N] tests)
- **Unit Tests**: `test/unit/[service]/` ([M] tests)
- **E2E Tests**: `test/e2e/[service]/` ([K] tests)

### Configuration
- **CRD Schema**: `config/crd/bases/[group]_[resources].yaml`
- **RBAC**: `config/rbac/role.yaml`
- **Deployment**: `deploy/manifests/[service]-deployment.yaml`
- **ConfigMap**: `deploy/manifests/[service]-configmap.yaml`

### Documentation
- **Service README**: `docs/services/[service]/README.md`
- **Implementation Plan**: `docs/services/[service]/implementation/IMPLEMENTATION_PLAN_V3.0.md`
- **Design Decisions**: `docs/services/[service]/implementation/DESIGN_DECISIONS.md`
- **Production Readiness**: `docs/services/[service]/implementation/PRODUCTION_READINESS_REPORT.md`

---

## Testing Summary

### Test Coverage Breakdown
| Test Type | Count | Coverage | Confidence | Files |
|-----------|-------|----------|------------|-------|
| **Integration** | [N] | 15-20% | 80-85% | `test/integration/[service]/` |
| **Unit** | [M] | 70-75% | 85-90% | `test/unit/[service]/` |
| **E2E** | [K] | 5-10% | 90-95% | `test/e2e/[service]/` |

### Key Test Scenarios Covered
✅ Happy path (Pending → Processing → Complete)
✅ Error recovery with exponential backoff retry
✅ Permanent failure after max retries
✅ Graceful degradation (partial success)
✅ Concurrent operations (100 concurrent requests)
✅ Edge cases (nil inputs, large payloads, invalid data)

### Known Test Gaps
- [Gap 1]: [Description] - Mitigated by [Strategy]
- [Gap 2]: [Description] - Accepted risk because [Justification]

---

## Deployment Guide

### Quick Deployment (Development)
\`\`\`bash
# Build
make build-[service]

# Deploy to Kind cluster
kubectl apply -f deploy/manifests/[service]-deployment.yaml

# Verify deployment
kubectl get pods -n kubernaut | grep [service]
kubectl logs -f deployment/[service] -n kubernaut
\`\`\`

### Production Deployment
\`\`\`bash
# Apply CRDs (first-time only)
kubectl apply -f config/crd/bases/

# Apply RBAC
kubectl apply -f config/rbac/

# Create ConfigMap and Secrets
kubectl apply -f deploy/manifests/[service]-configmap.yaml
kubectl create secret generic [service]-secrets --from-literal=api-key=xxx

# Deploy service
kubectl apply -f deploy/manifests/[service]-deployment.yaml

# Verify
kubectl rollout status deployment/[service] -n kubernaut
curl http://[service]:8080/healthz  # Should return 200
curl http://[service]:9090/metrics  # Should show Prometheus metrics
\`\`\`

### Configuration
**Required Environment Variables**:
- `CONFIG_FILE`: Path to configuration YAML (default: `/etc/[service]/config.yaml`)
- `LOG_LEVEL`: Logging level (default: `info`, options: `debug`, `info`, `warn`, `error`)

**Optional Environment Variables**:
- `METRICS_PORT`: Prometheus metrics port (default: `9090`)
- `SERVER_PORT`: HTTP server port (default: `8080`)

---

## Operational Considerations

### Monitoring
**Prometheus Metrics Endpoint**: `:9090/metrics`

**Key Metrics to Watch**:
- `[service]_operations_total{status="failure"}` - Error rate
- `[service]_operation_duration_seconds` - Latency (P50, P99)
- `[service]_resources_processed_total{result="failure"}` - Processing failures
- `[service]_queue_depth` - Queue backlog (if applicable)

**Alert Recommendations**:
- Error rate > 5% for 5 minutes → Page on-call
- P99 latency > 5s for 10 minutes → Ticket
- Queue depth > 1000 for 15 minutes → Investigate

### Logging
**Log Level**: Configured via `LOG_LEVEL` environment variable

**Key Log Patterns**:
- `"Processing request"` - Normal operation
- `"Transient error, retrying"` - Retry logic triggered
- `"Max retry attempts exceeded"` - Permanent failure
- `"Graceful shutdown initiated"` - Shutdown in progress

**Log Location**: stdout (captured by Kubernetes)

### Health Checks
- **Liveness**: `GET :8080/healthz` - Returns 200 if process is alive
- **Readiness**: `GET :8080/readyz` - Returns 200 if ready to serve traffic, 503 if unhealthy

---

## Troubleshooting Guide

### Common Issues

#### Issue 1: Service fails to start
**Symptoms**: Pod in CrashLoopBackOff, logs show configuration error
**Diagnosis**: Check ConfigMap exists and is mounted correctly
**Resolution**:
\`\`\`bash
kubectl describe pod [service]-xxx
kubectl get configmap [service]-config -o yaml
\`\`\`

#### Issue 2: High error rate in metrics
**Symptoms**: `[service]_operations_total{status="failure"}` increasing rapidly
**Diagnosis**: Check logs for error messages, verify external service availability
**Resolution**:
\`\`\`bash
kubectl logs -f deployment/[service] | grep ERROR
# Check external service connectivity
kubectl exec -it [service]-xxx -- curl http://external-service/health
\`\`\`

#### Issue 3: Processing stuck in queue
**Symptoms**: `[service]_queue_depth` growing, resources stuck in Processing phase
**Diagnosis**: Check for resource exhaustion or deadlock
**Resolution**:
\`\`\`bash
# Check resource usage
kubectl top pod [service]-xxx
# Restart if needed
kubectl rollout restart deployment/[service]
\`\`\`

[Full troubleshooting guide: `TROUBLESHOOTING_GUIDE.md`]

---

## Lessons Learned

### What Went Well ✅
1. **Integration-first testing**: Caught architectural issues early (Day 8), saved 2-3 days of rework
2. **Table-driven tests**: Reduced test code by 25-40%, easier to add new scenarios
3. **Error handling philosophy**: Consistent retry/backoff logic across all components
4. **Daily EOD docs**: Smooth handoffs, clear progress tracking

### Challenges Encountered ⚠️
1. **[Challenge 1]**: [Description]
   - **Resolution**: [How it was solved]
   - **Lesson**: [What to do differently next time]

2. **[Challenge 2]**: [Description]
   - **Resolution**: [Solution]
   - **Lesson**: [Takeaway]

### Recommendations for Future Services
1. [Recommendation 1]: [Why this matters]
2. [Recommendation 2]: [Impact]
3. [Recommendation 3]: [Benefit]

---

## Next Steps and Future Work

### Immediate Next Steps (Week 1-2)
- [ ] Monitor production deployment for 72 hours
- [ ] Create Grafana dashboard for key metrics
- [ ] Set up alerts in Prometheus AlertManager
- [ ] Brief on-call team on troubleshooting procedures

### Short-Term Enhancements (Month 1-3)
- [ ] [Enhancement 1]: [Description and priority]
- [ ] [Enhancement 2]: [Description and priority]
- [ ] Performance optimization: [Specific area]

### Long-Term Roadmap (Quarter 2-4)
- [ ] v2.0: [Major feature addition]
- [ ] [Scalability improvement]
- [ ] [Additional integration]

---

## Team and Contacts

**Implementation Team**:
- Lead: [Name] - [Role]
- Developer: [Name] - [Role]
- Reviewer: [Name] - [Role]

**Operational Contacts**:
- On-Call: [Team/Rotation]
- SME: [Name/Team] for [specific area]
- Documentation: [Link to wiki/runbook]

---

## References

- **Business Requirements**: `docs/requirements/BR-[SERVICE]-REQUIREMENTS.md`
- **Implementation Plan**: `docs/services/[service]/implementation/IMPLEMENTATION_PLAN_V3.0.md`
- **Design Decisions**: `docs/services/[service]/implementation/DESIGN_DECISIONS.md`
- **Production Readiness**: `docs/services/[service]/implementation/PRODUCTION_READINESS_REPORT.md`
- **API Documentation**: `docs/services/[service]/README.md#api-reference`
- **Testing Strategy**: `docs/services/[service]/implementation/TESTING_STRATEGY.md`

---

**Handoff Complete**: ✅ [Date]
**Next Review**: [Date for follow-up review]
```

---

## Critical Checkpoints (From Gateway Learnings)

### ✅ Checkpoint 1: Integration-First Testing (Day 8)
**Why**: Catches architectural issues 2 days earlier
**Action**: Write 5 integration tests before unit tests
**Evidence**: Gateway caught function signature mismatches early

### ✅ Checkpoint 2: Schema Validation (Day 7 EOD)
**Why**: Prevents test failures from schema mismatches
**Action**: Validate 100% field alignment before testing
**Evidence**: Gateway added missing CRD fields, avoided test failures

### ✅ Checkpoint 3: BR Coverage Matrix (Day 9 EOD)
**Why**: Ensures all requirements have test coverage
**Action**: Map every BR to tests, justify any skipped
**Evidence**: Gateway achieved 100% BR coverage

### ✅ Checkpoint 4: Production Readiness (Day 12)
**Why**: Reduces production deployment issues
**Action**: Complete comprehensive readiness checklist
**Evidence**: Gateway deployment went smoothly

### ✅ Checkpoint 5: Daily Status Docs (Days 1, 4, 7, 12)
**Why**: Better progress tracking and handoffs
**Action**: Create progress documentation at key milestones
**Evidence**: Gateway handoff was smooth

---

## Documentation Standards

### Daily Status Documents

**Day 1**: `01-day1-complete.md`
- Package structure created
- Types and interfaces defined
- Build successful
- Confidence assessment

**Day 4**: `02-day4-midpoint.md`
- Components completed so far
- Integration status
- Any blockers
- Confidence assessment

**Day 7**: `03-day7-complete.md`
- Core implementation complete
- Server and metrics implemented
- Schema validation complete
- Test infrastructure ready
- Confidence assessment

**Day 12**: `00-HANDOFF-SUMMARY.md`
- Executive summary
- Complete file inventory
- Key decisions
- Lessons learned
- Next steps

### Design Decision Documents

**Pattern**: Create DD-XXX entries for significant decisions

**Template**:
```markdown
## DD-XXX: [Decision Title]

### Status
**[Status Emoji] [Status]** (YYYY-MM-DD)

### Context & Problem
[What problem are we solving?]

### Alternatives Considered
1. **Alternative A**: [Pros/Cons]
2. **Alternative B**: [Pros/Cons]
3. **Alternative C**: [Pros/Cons]

### Decision
**APPROVED: Alternative X**

**Rationale**:
1. [Reason 1]
2. [Reason 2]

### Consequences
**Positive**: [Benefits]
**Negative**: [Trade-offs + Mitigations]
```

---

## Testing Strategy

### Test Distribution (From Gateway Success)

| Type | Coverage | Purpose |
|------|----------|---------|
| **Unit** | 70%+ | Component logic, edge cases |
| **Integration** | >50% | Component interactions, real dependencies |
| **E2E** | <10% | Complete workflows, production-like |

### Integration-First Order ⭐

**Traditional (DON'T DO THIS)**:
```
Days 7-8: Unit tests (40+ tests)
Days 9-10: Integration tests (12+ tests)
```

**Integration-First (DO THIS)** ✅:
```
Day 8 Morning: 5 critical integration tests
Day 8 Afternoon: Unit tests - Component Group 1
Day 9 Morning: Unit tests - Component Group 2
Day 9 Afternoon: Unit tests - Component Group 3
Day 10: Advanced integration + E2E tests
```

**Why This Works Better**:
- Validates architecture before details
- Catches integration issues early (cheaper to fix)
- Provides confidence for unit test details
- Follows TDD spirit (prove it works, then refine)

### Table-Driven Testing Pattern ⭐ (RECOMMENDED)

**Why Table-Driven Tests?**
Based on Dynamic Toolset Service implementation:
- **38% code reduction** in test files (1,612 lines → 1,001 lines)
- **25-40% faster** to add new test cases
- **Better maintainability**: Change logic once, all entries benefit
- **Clearer coverage**: Easy to see all scenarios at a glance

**Implementation Pattern**:

#### Pattern 1: Success Scenarios
```go
DescribeTable("should detect [Service] services",
    func(name, namespace string, labels map[string]string, ports []corev1.ServicePort, expectedEndpoint string) {
        service := &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:      name,
                Namespace: namespace,
                Labels:    labels,
            },
            Spec: corev1.ServiceSpec{Ports: ports},
        }

        result, err := detector.Detect(ctx, service)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).ToNot(BeNil())
        Expect(result.Endpoint).To(Equal(expectedEndpoint))
    },
    Entry("with standard label", "svc-1", "ns-1",
        map[string]string{"app": "myapp"},
        []corev1.ServicePort{{Port: 8080}},
        "http://svc-1.ns-1.svc.cluster.local:8080"),
    Entry("with name-based detection", "myapp-server", "ns-2",
        nil,
        []corev1.ServicePort{{Port: 8080}},
        "http://myapp-server.ns-2.svc.cluster.local:8080"),
    // Easy to add more - just add Entry!
)
```

#### Pattern 2: Negative Scenarios
```go
DescribeTable("should NOT detect non-matching services",
    func(name string, labels map[string]string, ports []corev1.ServicePort) {
        service := &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:   name,
                Labels: labels,
            },
            Spec: corev1.ServiceSpec{Ports: ports},
        }

        result, err := detector.Detect(ctx, service)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).To(BeNil())
    },
    Entry("for different service type", "other-svc",
        map[string]string{"app": "other"},
        []corev1.ServicePort{{Port: 9090}}),
    Entry("for service without ports", "no-ports",
        map[string]string{"app": "myapp"},
        []corev1.ServicePort{}),
)
```

#### Pattern 3: Health Check Scenarios
```go
DescribeTable("should validate health status",
    func(statusCode int, body string, expectSuccess bool) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(statusCode)
            w.Write([]byte(body))
        }))
        defer server.Close()

        err := checker.HealthCheck(ctx, server.URL)
        if expectSuccess {
            Expect(err).ToNot(HaveOccurred())
        } else {
            Expect(err).To(HaveOccurred())
        }
    },
    Entry("with 200 OK", http.StatusOK, "", true),
    Entry("with 204 No Content", http.StatusNoContent, "", true),
    Entry("with 503 Unavailable", http.StatusServiceUnavailable, "", false),
)
```

#### Pattern 4: Setup Functions for Complex Cases
```go
DescribeTable("should handle error conditions",
    func(setupServer func() string) {
        endpoint := setupServer()
        err := component.Process(endpoint)
        Expect(err).To(HaveOccurred())
    },
    Entry("for connection refused", func() string {
        return "http://localhost:9999"
    }),
    Entry("for timeout", func() string {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            time.Sleep(10 * time.Second)
        }))
        DeferCleanup(server.Close)
        return server.URL
    }),
    Entry("for invalid URL", func() string {
        return "not-a-valid-url"
    }),
)
```

**Best Practices**:
1. Use descriptive Entry names that document the scenario
2. Keep table logic simple and consistent
3. Use traditional It() for truly unique scenarios
4. Group related scenarios in same DescribeTable
5. Use DeferCleanup for resource cleanup in Entry setup functions

**Reference Examples**:
- Excellent examples in `test/unit/toolset/*_detector_test.go`
- 73 tests consolidated from 77, 38% less code
- All tests passing with 100% coverage maintained

### Test Naming Convention

```go
// Business requirement reference in test description
Describe("BR-[CATEGORY]-XXX: [Requirement]", func() {
    // Prefer table-driven tests for multiple scenarios
    DescribeTable("should [behavior]",
        func(params...) {
            // Test logic
        },
        Entry("scenario 1", ...),
        Entry("scenario 2", ...),
    )

    // Use traditional It() for unique scenarios
    Context("when [unique condition]", func() {
        It("should [expected behavior]", func() {
            // Test implementation
        })
    })
})
```

---

## Performance Targets

Define service-specific targets:

| Metric | Target | Measurement |
|--------|--------|-------------|
| API Latency (p95) | < Xms | HTTP request duration |
| API Latency (p99) | < Yms | HTTP request duration |
| Throughput | > Z req/s | Requests per second |
| Memory Usage | < XMB | Per replica |
| CPU Usage | < X cores | Average |
| [Service-specific] | [Target] | [How measured] |

---

## Common Pitfalls to Avoid

### ❌ Don't Do This:
1. **Skip integration tests until end**: Costs 2+ days in debugging
2. **Write all unit tests first**: Wastes time on wrong details
3. **Skip schema validation**: Causes test failures later
4. **No daily status docs**: Makes handoffs difficult
5. **Skip BR coverage matrix**: Results in untested requirements
6. **No production readiness check**: Causes deployment issues
7. **Repetitive test code**: Copy-paste It blocks for similar scenarios
8. **No table-driven tests**: Results in 25-40% more code

### ✅ Do This Instead:
1. **Integration-first testing**: Validates architecture early
2. **5 critical integration tests Day 8**: Proves core functionality
3. **Schema validation Day 7**: Prevents test failures
4. **Daily progress docs**: Smooth handoffs and communication
5. **BR coverage matrix Day 9**: Ensures 100% requirement coverage
6. **Production checklist Day 12**: Smooth deployment
7. **Table-driven tests**: Use DescribeTable for multiple similar scenarios ⭐
8. **DRY test code**: Extract common test logic, parameterize with Entry

---

## Success Criteria

### Implementation Complete When:
- [ ] All business requirements implemented
- [ ] Build passes without errors
- [ ] Zero lint errors
- [ ] Unit test coverage > 70%
- [ ] Integration test coverage > 50%
- [ ] E2E tests passing
- [ ] All metrics exposed
- [ ] Health checks functional
- [ ] Documentation complete
- [ ] Production readiness validated

### Quality Indicators:
- **Code Quality**: No lint errors, follows Go idioms
- **Test Quality**: BDD style, clear assertions, business requirement references
- **Test Organization**: Table-driven tests for similar scenarios, 25-40% less test code
- **Test Maintainability**: Easy to add new cases (just add Entry), consistent patterns
- **Documentation Quality**: Complete, accurate, helpful
- **Production Readiness**: Deployment manifests complete, observability comprehensive

---

## Makefile Targets

Create consistent development commands:

```makefile
# Testing
.PHONY: test-unit-[service]
test-unit-[service]:
	go test -v ./test/unit/[service]/...

.PHONY: test-integration-[service]
test-integration-[service]:
	go test -v ./test/integration/[service]/...

.PHONY: test-e2e-[service]
test-e2e-[service]:
	go test -v ./test/e2e/[service]/...

# Coverage
.PHONY: test-coverage-[service]
test-coverage-[service]:
	go test -cover -coverprofile=coverage.out ./pkg/[service]/...
	go tool cover -html=coverage.out

# Build
.PHONY: build-[service]
build-[service]:
	go build -o bin/[service] ./cmd/[service]

# Linting
.PHONY: lint-[service]
lint-[service]:
	golangci-lint run ./pkg/[service]/... ./cmd/[service]/...

# Deployment
.PHONY: deploy-kind-[service]
deploy-kind-[service]:
	kubectl apply -f deploy/[service]/
```

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-10-11 | Initial template based on Gateway + Dynamic Toolset learnings | AI Assistant |

---

## 📚 Appendix A: Complete EOD Documentation Templates ⭐ V2.0

### Day 1 Complete Template

**File**: `docs/services/[service-type]/[service-name]/implementation/phase0/01-day1-complete.md`

```markdown
# Day 1 Complete: Foundation Setup

**Date**: YYYY-MM-DD
**Phase**: Foundation
**Status**: ✅ Complete
**Confidence**: 90%

---

## ✅ Completed Tasks

### Package Structure
- [x] Controller/Service skeleton created
- [x] Business logic packages created (`pkg/[service]/`)
- [x] Test directories established (`test/unit/`, `test/integration/`, `test/e2e/`)
- [x] Deployment directory created (`deploy/[service]/`)

### Foundational Files
- [x] Main application entry point (`cmd/[service]/main.go`)
- [x] Core type definitions (`pkg/[service]/types.go`)
- [x] Primary interfaces defined
- [x] [List specific files created]

### Build Validation
- [x] Code compiles successfully (`go build ./cmd/[service]/`)
- [x] Zero lint errors (`golangci-lint run`)
- [x] Imports resolve correctly
- [x] CRD manifests generated (if applicable)

---

## 🏗️ Architecture Decisions

### Decision 1: [Decision Name]
- **Chosen Approach**: [What was decided]
- **Rationale**: [Why this approach]
- **Alternatives Considered**: [What other options were evaluated]
- **Impact**: [How this affects implementation]

### Decision 2: [Decision Name]
[Repeat pattern]

---

## 📊 Progress Metrics

- **Hours Spent**: 8h
- **Files Created**: [X] files
- **Lines of Code**: ~[Y] lines (skeleton)
- **Tests Written**: 0 (foundation only)

---

## 🚧 Known Issues / Blockers

### Issue 1: [Issue Description]
- **Status**: [Resolved / In Progress / Blocked]
- **Impact**: [High / Medium / Low]
- **Resolution Plan**: [How it will be addressed]

**Current Status**: No blockers ✅

---

## 📝 Next Steps (Day 2)

### Immediate Priorities
1. [Priority 1 - specific task]
2. [Priority 2 - specific task]
3. [Priority 3 - specific task]

### Success Criteria for Day 2
- [ ] [Specific deliverable 1]
- [ ] [Specific deliverable 2]
- [ ] [Specific deliverable 3]

---

## 🎯 Confidence Assessment

**Overall Confidence**: 90%

**Evidence**:
- Foundation is solid and follows established patterns
- All skeleton code compiles without errors
- Package structure aligns with project standards
- Architecture decisions validated against business requirements

**Risks**:
- None identified at foundation stage

**Mitigation**:
- N/A

---

**Prepared By**: [Name]
**Reviewed By**: [Name] (if applicable)
**Next Review**: End of Day 2
```

---

### Day 4 Midpoint Template

**File**: `docs/services/[service-type]/[service-name]/implementation/phase0/02-day4-midpoint.md`

```markdown
# Day 4 Complete: Midpoint Review

**Date**: YYYY-MM-DD
**Phase**: Core Implementation (Midpoint)
**Status**: ✅ 50% Complete
**Confidence**: 85%

---

## ✅ Completed Components (Days 2-4)

### Component 1: [Component Name]
- [x] Unit tests written (RED phase) - BR-XXX-001, BR-XXX-002
- [x] Implementation complete (GREEN phase)
- [x] Refactored for production quality
- **Test Coverage**: [X]% (target >70%)
- **BR Coverage**: BR-XXX-001 ✅, BR-XXX-002 ✅

### Component 2: [Component Name]
[Repeat pattern for each completed component]

---

## 🧪 Testing Summary

### Unit Tests
- **Total Tests Written**: [X] tests
- **Tests Passing**: [Y]/[X] (Z% passing)
- **Coverage**: [Coverage %] (target >70%)
- **Table-Driven Tests**: [N] DescribeTable blocks

### Business Requirement Coverage
- **Total BRs**: [N] requirements
- **BRs Tested**: [M]/[N] ([%] coverage)
- **Remaining BRs**: [N-M] (to be covered Days 5-6)

---

## 🏗️ Architecture Refinements

### Refinement 1: [Refinement Description]
- **Reason**: [Why refinement was needed]
- **Change**: [What was changed]
- **Impact**: [How it improves design]

**Total Refinements**: [N] (expected at midpoint)

---

## 🚧 Current Blockers

### Blocker 1: [Blocker Description]
- **Discovered**: Day [X]
- **Impact**: [High / Medium / Low]
- **Resolution Plan**: [Specific plan]
- **Expected Resolution**: Day [Y]

**Current Status**: [No blockers / X blockers identified]

---

## 📊 Progress Metrics

### Velocity
- **Days Elapsed**: 4/12 (33%)
- **Components Complete**: [X]/[Y] ([Z]%)
- **On Track**: [Yes / No / At Risk]

### Code Quality
- **Lint Errors**: 0 ✅
- **Build Errors**: 0 ✅
- **Test Failures**: [N] (should be 0)

---

## 📝 Remaining Work (Days 5-7)

### Day 5 Priorities
1. [Component X] - BR-XXX-XXX
2. [Component Y] - BR-XXX-XXX
3. [Component Z] - BR-XXX-XXX

### Day 6 Priorities
1. [Error handling philosophy doc]
2. [Final component]
3. [Code refactoring]

### Day 7 Priorities
1. [Server integration]
2. [Metrics implementation]
3. [Health checks]

---

## 🎯 Confidence Assessment

**Midpoint Confidence**: 85%

**Evidence**:
- [X] components complete with passing tests
- Test coverage at [Y]% (target >70%)
- Architecture decisions validated through implementation
- No critical blockers identified

**Risks**:
- [Risk 1]: [Description]
  - **Mitigation**: [How it will be addressed]
- [Risk 2]: [Description]
  - **Mitigation**: [How it will be addressed]

**Adjustment Plan**:
- [Any timeline or scope adjustments needed]

---

**Status**: ✅ On Track for Day 12 Completion
**Next Checkpoint**: Day 7 EOD (Integration Ready)
```

---

### Day 7 Complete Template

**File**: `docs/services/[service-type]/[service-name]/implementation/phase0/03-day7-complete.md`

```markdown
# Day 7 Complete: Integration Ready

**Date**: YYYY-MM-DD
**Phase**: Core Implementation Complete + Integration
**Status**: ✅ Integration Ready
**Confidence**: 92%

---

## ✅ Core Implementation Complete

### All Components Implemented
- [x] Component 1: [Name] - BR-XXX-001, BR-XXX-002
- [x] Component 2: [Name] - BR-XXX-003, BR-XXX-004
- [x] Component 3: [Name] - BR-XXX-005, BR-XXX-006
- [x] Component 4: [Name] - BR-XXX-007
- [x] Component 5: [Name] - BR-XXX-008, BR-XXX-009

**Total Components**: [X]/[X] (100% complete) ✅

---

## 🔗 Integration Complete

### Server Implementation
- [x] HTTP server struct created
- [x] Route registration complete
- [x] Middleware stack implemented
- [x] Health endpoints functional (`/health`, `/ready`)
- **Port**: 8080 (API/health), 9090 (metrics)

### Main Application Wiring
- [x] All components wired in `main.go`
- [x] Configuration loading implemented
- [x] Graceful shutdown handling
- [x] Manager setup (for CRD controllers)

### Metrics Implementation
- [x] 10+ Prometheus metrics defined
- [x] Metric recording in business logic
- [x] Metrics endpoint exposed (`:9090/metrics`)
- **Metrics Count**: [X] metrics (target: 10+) ✅

**Example Metrics**:
```
[service]_operations_total{operation="create",status="success"} 142
[service]_operations_duration_seconds{operation="create",quantile="0.95"} 0.234
[service]_errors_total{error_type="transient"} 5
```

---

## 🧪 Test Infrastructure Ready

### Schema Validation Complete
- [x] All CRD fields validated (if applicable)
- [x] API schemas validated
- [x] No field mismatches identified
- **Validation Document**: `design/01-[schema]-validation.md` ✅

### Test Suite Skeleton
- [x] Integration test suite created
- [x] Kind cluster setup validated (if applicable)
- [x] Test infrastructure imports verified
- [x] BeforeSuite/AfterSuite scaffolding complete

**Integration Test Files**:
- `test/integration/[service]/suite_test.go` ✅
- `test/integration/[service]/[scenario]_test.go` (placeholder) ✅

---

## 📋 Documentation Complete

### Error Handling Philosophy
- [x] Complete error handling document created
- [x] Error classification defined (transient/permanent/user)
- [x] Retry strategy documented
- [x] Circuit breaker patterns included
- **Document**: `design/ERROR_HANDLING_PHILOSOPHY.md` ✅

### Testing Strategy
- [x] Testing approach documented
- [x] Integration-first rationale explained
- [x] Test environment decision documented
- **Document**: `testing/01-integration-first-rationale.md` ✅

---

## 📊 Progress Metrics

### Implementation Progress
- **Days Elapsed**: 7/12 (58%)
- **Components Complete**: [X]/[X] (100%) ✅
- **Integration**: Complete ✅
- **Metrics**: [Y] metrics implemented (target 10+) ✅

### Code Quality
- **Build Status**: ✅ Passing
- **Lint Errors**: 0 ✅
- **Test Coverage**: [Z]% (preliminary, full validation Day 9)
- **BR Coverage**: [N]/[M] BRs implemented ([%]%)

---

## 🚧 Remaining Work (Days 8-12)

### Day 8: Integration-First Testing
- [ ] 5 critical integration tests
- [ ] Unit tests Part 1

### Day 9: Unit Tests Part 2 + BR Coverage
- [ ] Unit tests completion
- [ ] BR Coverage Matrix

### Day 10: E2E Tests
- [ ] E2E test scenarios
- [ ] Production environment setup

### Days 11-12: Documentation + Production Readiness
- [ ] Complete documentation
- [ ] Production readiness checklist
- [ ] Handoff summary

---

## 🎯 Confidence Assessment

**Day 7 Confidence**: 92%

**Evidence**:
- All components implemented with passing unit tests
- Server integration complete and functional
- 10+ Prometheus metrics implemented and tested
- Health checks operational
- Error handling philosophy documented
- Test infrastructure validated

**Risks**:
- Integration test complexity (Day 8) - **Medium**
  - **Mitigation**: Use Kind cluster template, start with 5 critical tests
- BR coverage validation (Day 9) - **Low**
  - **Mitigation**: BR coverage matrix template prepared

**Status**: ✅ **Ready for Integration Testing Phase**

---

**Next Milestone**: Day 8 EOD - Integration Tests Complete
**Expected Confidence After Day 9**: 95%+
```

---

## 📚 Appendix B: CRD Controller Variant ⭐ V2.0

**When to Use**: If your service is a CRD controller (5 out of 12 V1 services are CRD controllers)

**CRD Controllers in V1**:
- RemediationProcessor
- AIAnalysis
- WorkflowExecution
- KubernetesExecutor
- RemediationOrchestrator

---

### CRD Controller-Specific Patterns

#### 1. Reconciliation Loop Pattern

**Standard Controller Structure**:
```go
package controller

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	[servicev1alpha1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

// Reconciler reconciles a [Resource] object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=[service].kubernaut.ai,resources=[resources],verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=[service].kubernaut.ai,resources=[resources]/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=[service].kubernaut.ai,resources=[resources]/finalizers,verbs=update

// Reconcile implements the reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. FETCH RESOURCE
	resource := &[servicev1alpha1].[Resource]{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted, nothing to do
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// 2. CHECK TERMINAL STATES
	if resource.Status.Phase == [servicev1alpha1].[PhaseComplete] {
		log.Info("Resource already complete", "name", resource.Name)
		return ctrl.Result{}, nil
	}

	// 3. INITIALIZE STATUS
	if resource.Status.Phase == "" || resource.Status.Phase == [servicev1alpha1].[PhasePending] {
		resource.Status.Phase = [servicev1alpha1].[PhaseProcessing]
		now := metav1.Now()
		resource.Status.StartTime = &now

		if err := r.Status().Update(ctx, resource); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
	}

	// 4. BUSINESS LOGIC
	result, err := r.processResource(ctx, resource)
	if err != nil {
		// Update status with error
		resource.Status.Phase = [servicev1alpha1].[PhaseFailed]
		resource.Status.Error = err.Error()
		r.Status().Update(ctx, resource)

		// Determine if error is retryable
		if isRetryable(err) {
			backoff := calculateBackoff(resource.Status.AttemptCount)
			return ctrl.Result{RequeueAfter: backoff}, nil
		}

		return ctrl.Result{}, err
	}

	// 5. UPDATE STATUS ON SUCCESS
	return r.updateStatusAndRequeue(ctx, resource, result)
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&[servicev1alpha1].[Resource]{}).
		Complete(r)
}
```

---

#### 2. Status Update Patterns

**Complete Status Update with Conditions**:
```go
// updateStatusAndRequeue updates resource status and determines requeue strategy
func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, resource *Resource, result *ProcessResult) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Update phase
	resource.Status.Phase = PhaseComplete
	now := metav1.Now()
	resource.Status.CompletionTime = &now
	resource.Status.ObservedGeneration = resource.Generation

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: resource.Generation,
		LastTransitionTime: now,
		Reason:             "ProcessingComplete",
		Message:            "Resource processed successfully",
	}
	meta.SetStatusCondition(&resource.Status.Conditions, condition)

	// Update status subresource
	if err := r.Status().Update(ctx, resource); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Resource processing complete", "name", resource.Name)
	return ctrl.Result{}, nil
}
```

---

#### 3. Finalizer Pattern

**Finalizer Implementation for Cleanup**:
```go
const finalizerName = "[service].kubernaut.ai/finalizer"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	resource := &Resource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if resource is being deleted
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(resource, finalizerName) {
			// Perform cleanup
			if err := r.cleanupExternalResources(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(resource, finalizerName)
			if err := r.Update(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(resource, finalizerName) {
		controllerutil.AddFinalizer(resource, finalizerName)
		if err := r.Update(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Continue with reconciliation...
	return ctrl.Result{}, nil
}
```

---

#### 4. Exponential Backoff Requeue

**Production-Ready Backoff Implementation**:
```go
// calculateBackoff returns exponential backoff duration
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func calculateBackoff(attemptCount int) time.Duration {
	baseDelay := 30 * time.Second
	maxDelay := 480 * time.Second

	// Calculate exponential backoff: baseDelay * 2^attemptCount
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))

	// Cap at maximum delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10%) to prevent thundering herd
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

	return jitter
}

// Usage in Reconcile
if isRetryable(err) {
	resource.Status.AttemptCount++
	backoff := calculateBackoff(resource.Status.AttemptCount)
	log.Info("Transient error, requeueing", "attempt", resource.Status.AttemptCount, "backoff", backoff)
	return ctrl.Result{RequeueAfter: backoff}, nil
}
```

---

#### 5. Phase State Machine Pattern

**Common CRD Phase Transitions**:
```go
// Phase definitions
const (
	PhasePending    Phase = "Pending"
	PhaseProcessing Phase = "Processing"
	PhaseComplete   Phase = "Complete"
	PhaseFailed     Phase = "Failed"
)

// Phase transition validation
func (r *Reconciler) validatePhaseTransition(current, next Phase) error {
	validTransitions := map[Phase][]Phase{
		PhasePending:    {PhaseProcessing},
		PhaseProcessing: {PhaseComplete, PhaseFailed},
		PhaseComplete:   {}, // Terminal state
		PhaseFailed:     {}, // Terminal state
	}

	allowed, ok := validTransitions[current]
	if !ok {
		return fmt.Errorf("unknown current phase: %s", current)
	}

	for _, validNext := range allowed {
		if validNext == next {
			return nil
		}
	}

	return fmt.Errorf("invalid phase transition: %s → %s", current, next)
}
```

---

#### 6. CRD Testing Patterns

**Fake Client Testing for Controllers**:
```go
var _ = Describe("Controller Tests", func() {
	var (
		ctx        context.Context
		reconciler *Reconciler
		scheme     *runtime.Scheme
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = [service]v1alpha1.AddToScheme(scheme)

		// Create fake client with test resources
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "default",
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				// Test spec
			},
		}

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(resource).
			WithStatusSubresource(&[service]v1alpha1.[Resource]{}).
			Build()

		reconciler = &Reconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
	})

	It("should transition from Pending to Processing", func() {
		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-resource",
				Namespace: "default",
			},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		// Verify status updated
		updated := &[service]v1alpha1.[Resource]{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-resource", Namespace: "default"}, updated)
		Expect(err).ToNot(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal([service]v1alpha1.PhaseProcessing))
	})
})
```

---

### CRD Controller Checklist

Use this checklist when implementing CRD controllers:

- [ ] **Reconciliation Loop**: Standard pattern with fetch → validate → process → update
- [ ] **Status Updates**: Status subresource updates with conditions
- [ ] **Finalizers**: Cleanup logic implemented for resource deletion
- [ ] **Exponential Backoff**: Transient error retry with jitter
- [ ] **Phase State Machine**: Valid phase transitions enforced
- [ ] **RBAC Annotations**: Kubebuilder RBAC markers complete
- [ ] **Fake Client Tests**: Unit tests use fake.NewClientBuilder()
- [ ] **Integration Tests**: Kind cluster tests with real CRDs
- [ ] **Manager Setup**: SetupWithManager() implemented
- [ ] **Metrics**: Controller-specific metrics (reconciliations, errors, duration)

---

## 📚 Appendix C: Confidence Assessment Methodology ⭐ V2.0

**Purpose**: Evidence-based methodology for calculating implementation plan confidence

---

### Confidence Assessment Framework

**Confidence Range**: 60% (minimum viable) to 100% (perfect certainty)

**Target Confidence**: 90-95% for production-ready implementations

---

### Calculation Formula

```
Confidence = (Implementation Accuracy × 0.30) +
             (Test Coverage × 0.25) +
             (BR Coverage × 0.20) +
             (Production Readiness × 0.15) +
             (Documentation Quality × 0.10)
```

**Component Breakdown**:

1. **Implementation Accuracy (30% weight)**: How well does the implementation match specifications?
2. **Test Coverage (25% weight)**: How comprehensive is the test suite?
3. **BR Coverage (20% weight)**: Percentage of business requirements implemented
4. **Production Readiness (15% weight)**: Deployment and operational readiness
5. **Documentation Quality (10% weight)**: Completeness and accuracy of documentation

---

### Component Scoring Methodology

#### 1. Implementation Accuracy (30% weight)

**Score Calculation**:
```
Implementation Accuracy = (Spec Compliance + Code Quality + Error Handling) / 3
```

**Spec Compliance** (0-100):
- 100: All specified features implemented exactly as designed
- 80-99: Minor deviations from spec, documented with justification
- 60-79: Some features simplified or deferred
- <60: Significant gaps between spec and implementation

**Code Quality** (0-100):
- 100: Zero lint errors, all code reviewed, follows patterns
- 80-99: Minor lint warnings, mostly follows patterns
- 60-79: Some code quality issues, inconsistent patterns
- <60: Significant code quality problems

**Error Handling** (0-100):
- 100: Comprehensive error handling, retry logic, circuit breakers
- 80-99: Good error handling, some edge cases missed
- 60-79: Basic error handling, missing retry/recovery
- <60: Inadequate error handling

**Example**:
```
Implementation Accuracy = (95 + 90 + 92) / 3 = 92.3%
Weighted Contribution = 92.3% × 0.30 = 27.7 points
```

---

#### 2. Test Coverage (25% weight)

**Score Calculation**:
```
Test Coverage = (Unit Test Quality × 0.40) +
                (Integration Test Quality × 0.40) +
                (E2E Test Quality × 0.20)
```

**Unit Test Quality** (0-100):
- **Coverage Target**: 70-75% code coverage
- **Quality Factors**:
  - Tests use real business logic (not just mocks)
  - Table-driven tests for similar scenarios
  - Edge cases covered (nil, empty, invalid inputs)
  - Error paths tested

**Scoring**:
- 100: 75%+ coverage, comprehensive edge cases, production-ready tests
- 85: 70-75% coverage, most edge cases covered
- 70: 60-70% coverage, basic edge cases
- <70: <60% coverage or poor test quality

**Integration Test Quality** (0-100):
- **Coverage Target**: 15-20% of overall coverage
- **Quality Factors**:
  - Tests use real infrastructure (Kind/envtest)
  - Critical paths tested (happy path + error recovery)
  - Timing/concurrency issues validated
  - External dependency integration tested

**Scoring**:
- 100: 5+ comprehensive integration tests, all critical paths
- 85: 3-5 integration tests, most critical paths
- 70: 1-2 integration tests, basic coverage
- <70: No integration tests or poor quality

**E2E Test Quality** (0-100):
- **Coverage Target**: 5-10% of overall coverage
- **Quality Factors**:
  - Complete workflow scenarios
  - Production-like environment
  - Deployment validation

**Scoring**:
- 100: 2+ E2E tests covering complete workflows
- 85: 1 E2E test covering main workflow
- 70: E2E test planned but not implemented
- <70: No E2E tests

**Example**:
```
Test Coverage = (90 × 0.40) + (85 × 0.40) + (80 × 0.20)
              = 36 + 34 + 16 = 86%
Weighted Contribution = 86% × 0.25 = 21.5 points
```

---

#### 3. BR Coverage (20% weight)

**Score Calculation**:
```
BR Coverage = (Implemented BRs / Total BRs) × 100
```

**Scoring**:
- 100: All business requirements implemented and tested
- 90-99: 1-2 non-critical BRs deferred with justification
- 80-89: 3-5 BRs deferred, clear roadmap for implementation
- <80: Significant BR gaps affecting core functionality

**BR Mapping Requirement**:
- Each implemented BR must have:
  - At least one test validating the requirement
  - Documentation explaining the implementation
  - Evidence of successful validation

**Example**:
```
Total BRs: 15
Implemented BRs: 14
Deferred BRs: 1 (BR-SERVICE-015: Advanced analytics - v2.0 feature)

BR Coverage = (14 / 15) × 100 = 93.3%
Weighted Contribution = 93.3% × 0.20 = 18.7 points
```

---

#### 4. Production Readiness (15% weight)

**Score Calculation**: Based on Production Readiness Assessment scoring (from Appendix)

**Scoring Components**:
- Functional Validation (35 points)
- Operational Validation (29 points)
- Security Validation (15 points)
- Performance Validation (15 points)
- Deployment Validation (15 points)

**Total Possible**: 109 points (+ 10 bonus for documentation)

**Conversion to Percentage**:
```
Production Readiness = (Total Score / 109) × 100
```

**Scoring**:
- 95-100: Production-ready, deploy immediately
- 85-94: Mostly ready, minor improvements needed
- 75-84: Needs work before production
- <75: Not ready for production

**Example**:
```
Production Readiness Score: 103/109 = 94.5%
Weighted Contribution = 94.5% × 0.15 = 14.2 points
```

---

#### 5. Documentation Quality (10% weight)

**Score Calculation**:
```
Documentation Quality = (README + Design Decisions + Testing Docs + Troubleshooting) / 4
```

**README Quality** (0-100):
- 100: Complete with all sections, tested examples, accurate references
- 85: All sections present, minor gaps in examples
- 70: Basic README, missing integration guide or troubleshooting
- <70: Incomplete or inaccurate

**Design Decisions** (0-100):
- 100: All major decisions documented with DD-XXX format, alternatives considered
- 85: Most decisions documented, some missing rationale
- 70: Basic decisions documented, missing alternatives
- <70: Minimal or no design decision documentation

**Testing Documentation** (0-100):
- 100: Complete testing strategy, coverage matrix, known limitations documented
- 85: Good testing docs, minor gaps in coverage breakdown
- 70: Basic test documentation
- <70: Minimal test documentation

**Troubleshooting Guide** (0-100):
- 100: Comprehensive guide with common issues, symptoms, diagnosis, resolution
- 85: Good coverage of common issues
- 70: Basic troubleshooting info
- <70: Minimal or no troubleshooting guide

**Example**:
```
Documentation Quality = (95 + 90 + 88 + 85) / 4 = 89.5%
Weighted Contribution = 89.5% × 0.10 = 9.0 points
```

---

### Overall Confidence Calculation Example

**Component Scores**:
1. Implementation Accuracy: 92.3% → 27.7 points (30% weight)
2. Test Coverage: 86.0% → 21.5 points (25% weight)
3. BR Coverage: 93.3% → 18.7 points (20% weight)
4. Production Readiness: 94.5% → 14.2 points (15% weight)
5. Documentation Quality: 89.5% → 9.0 points (10% weight)

**Total Confidence Score**: 27.7 + 21.5 + 18.7 + 14.2 + 9.0 = **91.1%**

---

### Confidence Level Interpretation

| Score | Level | Interpretation | Action |
|-------|-------|----------------|--------|
| **95-100%** | ✅ **Exceptional** | Production-ready, comprehensive implementation | Deploy immediately |
| **90-94%** | ✅ **Excellent** | Production-ready with minor gaps | Deploy with confidence |
| **85-89%** | 🚧 **Good** | Mostly ready, some improvements needed | Address gaps, then deploy |
| **80-84%** | 🚧 **Acceptable** | Functional but needs work | Improve before production |
| **75-79%** | ⚠️ **Needs Improvement** | Significant gaps | Address critical issues |
| **<75%** | ❌ **Insufficient** | Not production-ready | Major work required |

---

### Confidence Assessment Template

**File**: `docs/services/[service]/implementation/CONFIDENCE_ASSESSMENT.md`

```markdown
# [Service] Confidence Assessment

**Assessment Date**: 2025-10-12
**Assessor**: [Name]
**Overall Confidence**: XX.X% ⭐

---

## Component Scores

### 1. Implementation Accuracy (30% weight)
- **Spec Compliance**: XX/100
- **Code Quality**: XX/100
- **Error Handling**: XX/100
- **Average**: XX.X%
- **Weighted Score**: XX.X points

**Evidence**:
- [Specific evidence for scores]

### 2. Test Coverage (25% weight)
- **Unit Tests**: XX/100
- **Integration Tests**: XX/100
- **E2E Tests**: XX/100
- **Weighted Average**: XX.X%
- **Weighted Score**: XX.X points

**Evidence**:
- Unit test coverage: XX% (`go test -cover`)
- [N] integration tests covering [list critical paths]
- [M] E2E tests validating [workflows]

### 3. BR Coverage (20% weight)
- **Implemented**: [N]/[M] BRs
- **Deferred**: [K] BRs with justification
- **Coverage**: XX.X%
- **Weighted Score**: XX.X points

**Deferred BRs**:
- BR-XXX-XXX: [Reason for deferral]

### 4. Production Readiness (15% weight)
- **Score**: [X]/109 points
- **Percentage**: XX.X%
- **Weighted Score**: XX.X points

**Reference**: [Production Readiness Report](./PRODUCTION_READINESS_REPORT.md)

### 5. Documentation Quality (10% weight)
- **README**: XX/100
- **Design Decisions**: XX/100
- **Testing Docs**: XX/100
- **Troubleshooting**: XX/100
- **Average**: XX.X%
- **Weighted Score**: XX.X points

---

## Overall Confidence Score

**Total**: XX.X / 100 points = **XX.X% Confidence**

**Confidence Level**: ✅ Exceptional | ✅ Excellent | 🚧 Good | ⚠️ Needs Improvement | ❌ Insufficient

---

## Strengths

1. [Specific strength with evidence]
2. [Another strength]
3. [Third strength]

---

## Areas for Improvement

1. **[Area]**: Current XX%, target YY%
   - **Gap**: [Description]
   - **Plan**: [Improvement strategy]

2. **[Another area]**: Current XX%, target YY%
   - **Gap**: [Description]
   - **Plan**: [Strategy]

---

## Recommendations

### Before Production Deployment
- [ ] [Critical recommendation]
- [ ] [Important recommendation]

### Post-Deployment
- [ ] [Monitoring recommendation]
- [ ] [Future improvement]

---

**Assessment Valid Until**: [Date]
**Next Assessment**: [Date for re-evaluation]
```

---

### Using Confidence Assessments

**When to Perform Assessments**:
1. **Day 7 (Mid-Implementation)**: Initial assessment to identify risks early
2. **Day 12 (Pre-Production)**: Final assessment before deployment
3. **Post-Production**: Periodic reassessments based on operational experience

**Confidence-Driven Decisions**:
- **<80%**: Do not deploy to production, address gaps first
- **80-89%**: Consider beta/staging deployment, plan improvements
- **90%+**: Production-ready, proceed with deployment

---

## Related Documents

- [00-core-development-methodology.mdc](.cursor/rules/00-core-development-methodology.mdc) - APDC-TDD methodology
- [Gateway Implementation](docs/services/stateless/gateway-service/implementation/) - Reference implementation
- [Dynamic Toolset Implementation](docs/services/stateless/dynamic-toolset/implementation/) - Enhanced patterns
- [Notification Controller Implementation](docs/services/crd-controllers/06-notification/implementation/IMPLEMENTATION_PLAN_V3.0.md) - CRD controller standard (V3.0, 98% confidence)
- [Data Storage Implementation](docs/services/stateless/data-storage/implementation/) - v4.1 standard

---

**Template Status**: ✅ **Production-Ready** (V2.0)
**Quality Standard**: Matches Notification V3.0 and Data Storage v4.1 standards
**Success Rate**:
- Gateway: 95% test coverage, 100% BR coverage, 98% confidence
- Notification: 97.2% BR coverage, 95% test coverage, 98% confidence
**Estimated Effort Savings**: 3-5 days per service (comprehensive guidance prevents rework)

---

## Version History

### v2.0 (2025-10-12)
**Added: Table-Driven Testing Pattern** ⭐

**Changes**:
- Added comprehensive table-driven testing guidance in DO-RED section
- Added "Table-Driven Testing Pattern" subsection in Testing Strategy
- Provided 4 complete code pattern examples (success, negative, health checks, setup functions)
- Updated Common Pitfalls section with table-driven testing guidance
- Updated Success Criteria with test organization quality indicators
- Added references to Dynamic Toolset detector test examples

**Impact**:
- 25-40% less test code expected
- Better test maintainability
- Easier to extend test coverage

**Reference**: [TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md](./TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md)

---

### v1.0 (Initial Release)
**Base Template from Gateway + Dynamic Toolset Learnings**

**Included**:
- APDC-TDD methodology integration
- Integration-first testing strategy
- 12-day implementation timeline
- 5 critical checkpoints
- Daily progress documentation
- BR coverage matrix
- Production readiness checklist
- Performance benchmarking guidance

**Based On**:
- Gateway Service (proven success: 95% test coverage)
- Dynamic Toolset enhancements
- Gateway post-implementation triage

