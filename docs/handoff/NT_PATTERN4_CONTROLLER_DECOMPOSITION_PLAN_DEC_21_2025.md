# NT Pattern 4: Controller Decomposition Plan

**Date**: December 21, 2025
**Service**: Notification (NT)
**Pattern**: Controller Decomposition (File Splitting)
**Status**: 📋 **PLANNING COMPLETE - READY TO EXECUTE**

---

## 🎯 **Executive Summary**

Pattern 4 decomposes the NT controller into smaller, more maintainable files following RO's proven architecture. This is the final refactoring pattern.

**Current State**: 1471-line monolithic controller
**Target State**: Main controller + 4-5 specialized files (~300-400 lines each)
**Reference**: RemediationOrchestrator (RO) service

---

## 📊 **Current State Analysis**

### **File Breakdown**

| File | Lines | Purpose |
|------|-------|---------|
| `notificationrequest_controller.go` | 1471 | Main reconciler + all logic |
| `audit.go` | 289 | Audit helper functions |
| `metrics.go` | 170 | Metrics definitions |
| **Total** | **1930** | Monolithic structure |

### **Controller Method Breakdown** (notificationrequest_controller.go)

| Method Category | Lines | Methods | Priority |
|-----------------|-------|---------|----------|
| **Reconcile Loop** | ~100 | `Reconcile()`, `SetupWithManager()` | Keep in main |
| **Initialization** | ~50 | `handleInitialization()` | Keep in main |
| **Delivery Loop** | ~200 | `handleDeliveryLoop()`, `attemptChannelDelivery()`, `recordDeliveryAttempt()` | ✅ **EXTRACTED (Pattern 3)** |
| **Status Updates** | ~100 | `updateStatusWithRetry()` | ✅ **EXTRACTED (Pattern 2)** |
| **Terminal State** | ~50 | `handleTerminalStateCheck()` | ✅ **EXTRACTED (Pattern 1)** |
| **Retry Logic** | ~150 | Retry policy handling | Move to `retry_handler.go` |
| **Circuit Breaker** | ~100 | Circuit breaker logic | Move to `circuit_breaker_handler.go` |
| **Routing** | ~80 | Channel routing logic | Move to `routing_handler.go` |
| **Sanitization** | ~50 | Data sanitization calls | Keep in main (thin wrapper) |
| **Audit Emission** | ~100 | Audit event creation | Keep in main (uses audit.go) |
| **Helper Methods** | ~491 | Various utilities | Distribute across files |

**Patterns 1-3 Already Extracted**: ~350 lines moved to packages
**Remaining to Decompose**: ~1121 lines in main controller

---

## 🏆 **RO Reference Architecture**

### **RO Controller Structure**

| File | Lines | Purpose |
|------|-------|---------|
| `remediationrequest_controller.go` | 1754 | Main reconciler |
| `blocking.go` | 298 | Blocking condition handling |
| `consecutive_failure.go` | 233 | Consecutive failure tracking |
| `notification_handler.go` | 233 | Notification creation logic |
| `notification_tracking.go` | 233 | Notification tracking state |
| **Total** | **2751** | Decomposed structure |

**Key Insight**: RO splits by **functional domain**, not by method type.

---

## 📋 **Proposed NT Decomposition**

### **Target File Structure**

```
internal/controller/notification/
├── notificationrequest_controller.go  (~600 lines)  # Main reconciler
├── audit.go                           (289 lines)   # Existing audit helpers
├── metrics.go                         (170 lines)   # Existing metrics
├── retry_handler.go                   (~300 lines)  # NEW: Retry logic
├── circuit_breaker_handler.go         (~250 lines)  # NEW: Circuit breaker
├── routing_handler.go                 (~200 lines)  # NEW: Channel routing
└── sanitization_handler.go            (~150 lines)  # NEW: Sanitization logic
```

**Total**: ~1959 lines (from 1930, slight increase due to file headers/comments)

---

## 🔍 **Detailed Decomposition Plan**

### **File 1: Main Controller** (notificationrequest_controller.go)

**Keep in Main** (~600 lines):
- `Reconcile()` - Main reconciliation loop
- `SetupWithManager()` - Controller setup
- `handleInitialization()` - CRD initialization
- Thin wrappers calling handler methods
- Struct definition with all dependencies

**Remove from Main**:
- Retry policy logic → `retry_handler.go`
- Circuit breaker logic → `circuit_breaker_handler.go`
- Channel routing logic → `routing_handler.go`
- Sanitization logic → `sanitization_handler.go`

---

### **File 2: retry_handler.go** (NEW - ~300 lines)

**Purpose**: Centralize retry logic and backoff calculations

**Methods to Extract**:
```go
// handleRetryPolicy determines if notification should be retried
func (r *NotificationRequestReconciler) handleRetryPolicy(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) (bool, error)

// calculateBackoff calculates exponential backoff duration
func (r *NotificationRequestReconciler) calculateBackoff(
    attempt int,
    policy *notificationv1alpha1.RetryPolicy,
) time.Duration

// shouldRetryError determines if error is retryable
func (r *NotificationRequestReconciler) shouldRetryError(err error) bool

// recordRetryAttempt records retry attempt in status
func (r *NotificationRequestReconciler) recordRetryAttempt(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
    attempt int,
    err error,
) error
```

**Benefits**:
- ✅ Isolates retry logic for independent testing
- ✅ Clear separation of concerns
- ✅ Easy to modify retry behavior without touching main controller

---

### **File 3: circuit_breaker_handler.go** (NEW - ~250 lines)

**Purpose**: Circuit breaker state management and failure tracking

**Methods to Extract**:
```go
// checkCircuitBreaker checks if circuit breaker allows delivery
func (r *NotificationRequestReconciler) checkCircuitBreaker(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
    channel notificationv1alpha1.Channel,
) (bool, error)

// recordCircuitBreakerFailure records failure for circuit breaker
func (r *NotificationRequestReconciler) recordCircuitBreakerFailure(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
    channel notificationv1alpha1.Channel,
) error

// resetCircuitBreaker resets circuit breaker after success
func (r *NotificationRequestReconciler) resetCircuitBreaker(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
    channel notificationv1alpha1.Channel,
) error

// getCircuitBreakerState gets current circuit breaker state
func (r *NotificationRequestReconciler) getCircuitBreakerState(
    notification *notificationv1alpha1.NotificationRequest,
    channel notificationv1alpha1.Channel,
) string
```

**Benefits**:
- ✅ Circuit breaker logic isolated
- ✅ Easy to test failure scenarios
- ✅ Clear state management

---

### **File 4: routing_handler.go** (NEW - ~200 lines)

**Purpose**: Channel routing and selection logic

**Methods to Extract**:
```go
// selectChannels determines which channels to use for delivery
func (r *NotificationRequestReconciler) selectChannels(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) ([]notificationv1alpha1.Channel, error)

// routeByPriority routes notification based on priority
func (r *NotificationRequestReconciler) routeByPriority(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) ([]notificationv1alpha1.Channel, error)

// routeBySkipReason routes notification based on skip-reason label
func (r *NotificationRequestReconciler) routeBySkipReason(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) ([]notificationv1alpha1.Channel, error)

// validateChannelAvailability checks if channels are available
func (r *NotificationRequestReconciler) validateChannelAvailability(
    channels []notificationv1alpha1.Channel,
) error
```

**Benefits**:
- ✅ Routing logic isolated
- ✅ Easy to add new routing strategies
- ✅ Clear channel selection logic

---

### **File 5: sanitization_handler.go** (NEW - ~150 lines)

**Purpose**: Data sanitization and secret redaction

**Methods to Extract**:
```go
// sanitizeNotification sanitizes notification before delivery
func (r *NotificationRequestReconciler) sanitizeNotification(
    notification *notificationv1alpha1.NotificationRequest,
) *notificationv1alpha1.NotificationRequest

// redactSecrets redacts secrets from notification content
func (r *NotificationRequestReconciler) redactSecrets(
    content string,
) string

// validateSanitization validates sanitization was successful
func (r *NotificationRequestReconciler) validateSanitization(
    original, sanitized *notificationv1alpha1.NotificationRequest,
) error

// sanitizeForAudit sanitizes notification for audit trail
func (r *NotificationRequestReconciler) sanitizeForAudit(
    notification *notificationv1alpha1.NotificationRequest,
) map[string]interface{}
```

**Benefits**:
- ✅ Sanitization logic isolated
- ✅ Easy to test secret redaction
- ✅ Clear data protection logic

---

## 🎯 **Implementation Strategy**

### **Phase 4A: Create Handler Files** (Week 1)

**Day 1-2**: Create `retry_handler.go`
- Extract retry methods from main controller
- Add file header with Pattern 4 documentation
- Maintain all method signatures
- Run unit tests to verify

**Day 3-4**: Create `circuit_breaker_handler.go`
- Extract circuit breaker methods
- Add file header with Pattern 4 documentation
- Run unit tests to verify

**Day 5**: Create `routing_handler.go`
- Extract routing methods
- Add file header with Pattern 4 documentation
- Run unit tests to verify

---

### **Phase 4B: Refine Main Controller** (Week 2)

**Day 6-7**: Create `sanitization_handler.go`
- Extract sanitization methods
- Add file header with Pattern 4 documentation
- Run unit tests to verify

**Day 8-9**: Clean up main controller
- Remove extracted methods
- Add comments referencing handler files
- Ensure all method calls route to correct files
- Run integration tests

**Day 10**: Final validation
- Run all test tiers (unit, integration, E2E)
- Verify 89% pass rate maintained
- Create completion documentation

---

## ✅ **Success Criteria**

| Criterion | Target | Validation |
|-----------|--------|------------|
| **Main Controller Size** | <700 lines | `wc -l notificationrequest_controller.go` |
| **Handler Files Created** | 4 files | `ls *_handler.go` |
| **Unit Tests Passing** | 100% (239/239) | `make test-unit-notification` |
| **Integration Tests Passing** | ≥89% (115/129) | `make test-integration-notification` |
| **E2E Tests Passing** | ≥90% | `make test-e2e-notification` |
| **No Regressions** | 0 new failures | Compare before/after |
| **Code Duplication** | <5% | Manual review |

---

## 📊 **Expected Benefits**

### **Maintainability**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Main Controller Lines** | 1471 | ~600 | -59% |
| **Largest File** | 1471 lines | ~300 lines | -80% |
| **Files** | 3 | 7 | +133% |
| **Avg File Size** | 643 lines | 280 lines | -56% |

### **Testability**

- ✅ Retry logic testable independently
- ✅ Circuit breaker testable independently
- ✅ Routing logic testable independently
- ✅ Sanitization testable independently

### **Extensibility**

- ✅ Easy to add new retry strategies
- ✅ Easy to add new routing rules
- ✅ Easy to add new sanitization rules
- ✅ Clear separation of concerns

---

## ⚠️ **Risks and Mitigation**

### **Risk 1: Breaking Changes**

**Probability**: Low
**Impact**: High
**Mitigation**:
- Run tests after each file extraction
- Maintain method signatures exactly
- Use git commits per file for easy rollback

### **Risk 2: Test Failures**

**Probability**: Medium
**Impact**: Medium
**Mitigation**:
- Run integration tests after each extraction
- Fix failures immediately before proceeding
- Maintain 89% pass rate as baseline

### **Risk 3: Import Cycles**

**Probability**: Low
**Impact**: Medium
**Mitigation**:
- All handler files in same package (no new imports)
- Methods remain on same reconciler struct
- No cross-file dependencies

---

## 📚 **References**

- **Pattern Library**: `docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md`
- **RO Reference**: `internal/controller/remediationorchestrator/`
- **Cross-Service Analysis**: `docs/handoff/CROSS_SERVICE_REFACTORING_PATTERNS_DEC_20_2025.md`
- **NT Refactoring Triage**: `docs/handoff/NT_REFACTORING_TRIAGE_DEC_19_2025.md`

---

## 🎯 **Decision Required**

**Question**: Proceed with Pattern 4 implementation?

**Options**:
- **A**: Proceed with full 2-week plan (recommended)
- **B**: Implement Phase 4A only (Week 1 - handler files)
- **C**: Defer Pattern 4 to next sprint

**Recommendation**: **Option A** - Full implementation. The service is stable (89% pass rate), and decomposition will improve long-term maintainability.

**Confidence**: 90% - Plan is detailed, RO provides proven reference, tests provide safety net.

---

**Document Status**: ✅ Complete
**Last Updated**: 2025-12-21 12:00 EST
**Author**: AI Assistant (Cursor)
**Next Step**: User approval to proceed


