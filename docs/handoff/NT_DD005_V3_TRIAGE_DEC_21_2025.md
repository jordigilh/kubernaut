# Notification Service DD-005 V3.0 Compliance Triage

**Date**: December 21, 2025
**Service**: Notification (NT)
**Mandate**: DD-005 V3.0 Metric Name Constants
**Status**: ⚠️  **PARTIALLY COMPLIANT** - Requires Cleanup

---

## 🎯 **Executive Summary**

The Notification service has **TWO METRIC SYSTEMS** with different metric names:

1. ✅ **NEW System** (`pkg/notification/metrics/`) - DD-005 V3.0 COMPLIANT
   - Uses Pattern B (full metric names in constants)
   - Properly namespaced: `kubernaut_notification_*`
   - Used by E2E tests

2. ❌ **OLD System** (`internal/controller/notification/metrics.go`) - NON-COMPLIANT
   - Uses hardcoded strings (no constants)
   - Missing `kubernaut` namespace prefix
   - Still used by controller and unit tests

**Risk**: Metric name mismatches between production and tests.

---

## 📊 **Current State Analysis**

### **✅ COMPLIANT: New Metrics System**

**Location**: `pkg/notification/metrics/metrics.go`

**Status**: ✅ DD-005 V3.0 Compliant (Pattern B)

**Metric Constants** (10 total):
```go
const (
    MetricNameReconcilerRequestsTotal        = "kubernaut_notification_reconciler_requests_total"
    MetricNameReconcilerDuration             = "kubernaut_notification_reconciler_duration_seconds"
    MetricNameReconcilerErrorsTotal          = "kubernaut_notification_reconciler_errors_total"
    MetricNameReconcilerActive               = "kubernaut_notification_reconciler_active"
    MetricNameDeliveryAttemptsTotal          = "kubernaut_notification_delivery_attempts_total"
    MetricNameDeliveryDuration               = "kubernaut_notification_delivery_duration_seconds"
    MetricNameDeliveryRetriesTotal           = "kubernaut_notification_delivery_retries_total"
    MetricNameChannelCircuitBreakerState     = "kubernaut_notification_channel_circuit_breaker_state"
    MetricNameChannelHealthScore             = "kubernaut_notification_channel_health_score"
    MetricNameSanitizationRedactions         = "kubernaut_notification_sanitization_redactions_total"
)
```

**Used By**:
- ✅ `test/e2e/notification/04_metrics_validation_test.go` (uses constants directly)
- ✅ Production metric definitions (Pattern B)

---

### **❌ NON-COMPLIANT: Old Metrics System**

**Location**: `internal/controller/notification/metrics.go`

**Status**: ❌ NOT DD-005 V3.0 Compliant (hardcoded strings)

**Hardcoded Metric Names** (8 total):
```go
var (
    notificationDeliveryFailureRatio = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "notification_delivery_failure_ratio",  // ❌ No constant, no kubernaut prefix
            // ...
        },
        []string{"namespace"},
    )

    notificationDeliveryStuckDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "notification_delivery_stuck_duration_seconds",  // ❌ No constant
            // ...
        },
        []string{"namespace"},
    )

    // ... 6 more metrics with hardcoded names
)
```

**Used By**:
- ❌ `internal/controller/notification/notificationrequest_controller.go` (production code)
- ❌ `test/unit/notification/metrics_test.go` (unit tests)

**Helper Functions** (still in use):
- `RecordDeliveryAttempt(namespace, channel, status)`
- `RecordDeliveryDuration(namespace, channel, durationSeconds)`
- `UpdateFailureRatio(namespace, ratio)`
- `RecordStuckDuration(namespace, durationSeconds)`
- `UpdatePhaseCount(namespace, phase, count)`
- `RecordDeliveryRetries(namespace, retries)`
- `RecordSlackRetry(namespace, reason)`
- `RecordSlackBackoff(namespace, durationSeconds)`

---

## 🚨 **Problems Identified**

### **Problem 1: Metric Name Inconsistency**

**OLD System**:
```go
"notification_delivery_requests_total"  // Missing kubernaut prefix
```

**NEW System**:
```go
"kubernaut_notification_delivery_attempts_total"  // Has kubernaut prefix, different name
```

**Impact**: E2E tests validate different metrics than production emits.

---

### **Problem 2: Duplicate Registration Conflict**

**File Comment** (lines 100-123):
```go
// ========================================
// METRICS REGISTRATION DISABLED (DD-METRICS-001 Migration)
// 🔧 Refactoring Note: This file's init() registration is disabled
// ========================================
//
// REASON: Conflicting metrics registration with pkg/notification/metrics/metrics.go
//
// The controller now uses DD-METRICS-001 compliant PrometheusRecorder which
// references metrics from pkg/notification/metrics/metrics.go.
//
// This file's metrics have different label configurations than the pkg/ metrics,
// causing "different label names" panic in parallel tests.
//
// SOLUTION: Keep helper functions for backward compatibility, but remove init()
// registration. The pkg/ metrics are registered by promauto in their own init().
//
// FUTURE: This file should be deleted once all references are migrated to
// use the PrometheusRecorder interface (DD-METRICS-001 pattern).
// ========================================
```

**Status**: ⚠️  Migration incomplete - controller still uses old helpers, not PrometheusRecorder.

---

### **Problem 3: Label Mismatch**

**OLD System Labels**:
```go
[]string{"namespace", "status", "channel"}  // Has 'namespace' label
```

**NEW System Labels**:
```go
[]string{"channel", "status"}  // No 'namespace' label
```

**Impact**: Metric label schemas don't match between old and new systems.

---

## 🎯 **Recommended Solution**

### **Option A: Quick Fix - Use Constants in Old System** ⏱️ ~30 minutes

**Approach**: Update `internal/controller/notification/metrics.go` to import and use constants from `pkg/notification/metrics`.

**Changes**:
1. Import `ntmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"`
2. Replace hardcoded strings with `ntmetrics.MetricNameXXX`
3. Update helper functions if needed
4. Add deprecation warnings

**Pros**:
- ✅ Quick DD-005 V3.0 compliance
- ✅ No behavior changes
- ✅ Prevents test/production mismatches

**Cons**:
- ⚠️  Still have two metric systems
- ⚠️  Label mismatches remain
- ⚠️  Doesn't complete DD-METRICS-001 migration

---

### **Option B: Complete Migration - Use PrometheusRecorder** ⏱️ ~2-3 hours

**Approach**: Complete DD-METRICS-001 migration by replacing old helper functions with PrometheusRecorder.

**Changes**:
1. Update controller to inject `PrometheusRecorder`
2. Replace all helper function calls with recorder methods
3. Update tests to use new metrics
4. Delete `internal/controller/notification/metrics.go`
5. Verify label schemas match business needs

**Pros**:
- ✅ Completes DD-METRICS-001 migration
- ✅ Single source of truth for metrics
- ✅ Clean architecture
- ✅ DD-005 V3.0 compliant

**Cons**:
- ⚠️  Requires controller refactoring
- ⚠️  Breaking change if external systems depend on old metric names
- ⚠️  Requires label schema decision

---

### **Option C: Parallel Migration** ⏱️ ~1 hour

**Approach**: Keep both systems temporarily, but make old system DD-005 V3.0 compliant.

**Changes**:
1. Apply Option A fixes (use constants)
2. Add deprecation warnings to old system
3. Plan DD-METRICS-001 migration for next sprint
4. Document migration path

**Pros**:
- ✅ DD-005 V3.0 compliant immediately
- ✅ No immediate breaking changes
- ✅ Clear migration path documented

**Cons**:
- ⚠️  Technical debt remains
- ⚠️  Two metric systems coexist

---

## 📋 **Implementation Checklist (Option A - Recommended)**

### **Phase 1: Update Old Metrics System**

- [ ] Import metric constants from `pkg/notification/metrics`
- [ ] Replace 8 hardcoded metric names with constants
- [ ] Add DD-005 V3.0 compliance comments
- [ ] Add deprecation warnings

### **Phase 2: Verify Tests**

- [ ] Run unit tests: `go test ./test/unit/notification/metrics_test.go`
- [ ] Verify helper functions still work
- [ ] Check for compilation errors

### **Phase 3: Documentation**

- [ ] Update `internal/controller/notification/metrics.go` file comments
- [ ] Add migration path documentation
- [ ] Reference DD-METRICS-001 completion plan

### **Phase 4: Validation**

- [ ] Build controller: `go build ./cmd/notification/...`
- [ ] Run E2E metrics test: `ginkgo test/e2e/notification/04_metrics_validation_test.go`
- [ ] Verify no metric registration conflicts

---

## 🔍 **Affected Files**

### **Files Requiring Changes** (Option A):
1. `internal/controller/notification/metrics.go` - Add constants import
2. `internal/controller/notification/notificationrequest_controller.go` - Verify usage
3. `test/unit/notification/metrics_test.go` - Verify tests still pass

### **Files Already Compliant**:
1. ✅ `pkg/notification/metrics/metrics.go` - Pattern B constants
2. ✅ `test/e2e/notification/04_metrics_validation_test.go` - Uses constants

---

## 📊 **Compliance Status**

| Component | DD-005 V3.0 | DD-METRICS-001 | Status |
|-----------|-------------|----------------|--------|
| **pkg/notification/metrics/** | ✅ Compliant | ✅ Compliant | Production Ready |
| **E2E Tests** | ✅ Compliant | ✅ Compliant | Validated |
| **internal/controller/notification/metrics.go** | ❌ Non-Compliant | ⚠️  Incomplete | Needs Update |
| **Unit Tests** | ⚠️  Uses Old System | ⚠️  Uses Old System | Needs Update |
| **Controller** | ⚠️  Uses Old System | ⚠️  Incomplete | Needs Update |

---

## 🎯 **Next Steps**

### **Immediate (This Session)**:
1. Decide: Option A, B, or C?
2. If Option A: Apply quick fix (30 minutes)
3. Verify tests pass
4. Document migration path

### **Short Term (Next Sprint)**:
1. Complete DD-METRICS-001 migration
2. Delete old metrics system
3. Unify label schemas
4. Update documentation

### **Long Term**:
1. Ensure all services follow Pattern B
2. Standardize PrometheusRecorder usage
3. Document metric naming patterns

---

## 💡 **Recommendation**

**RECOMMENDED: Option A (Quick Fix)**

**Rationale**:
1. ✅ Achieves DD-005 V3.0 compliance immediately
2. ✅ Minimal risk (no behavior changes)
3. ✅ Prevents test/production metric mismatches
4. ✅ Allows time to plan proper DD-METRICS-001 migration
5. ✅ Documents technical debt clearly

**Time Investment**: ~30 minutes
**Risk Level**: LOW
**Business Value**: HIGH (test reliability)

---

## 📚 **References**

- **DD-005 V3.0 Mandate**: `docs/handoff/DD005_V3_METRIC_CONSTANTS_MANDATE_DEC_21_2025.md`
- **DD-005 Observability Standards**: `docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md`
- **DD-METRICS-001 Pattern**: `pkg/notification/metrics/recorder.go`
- **Pattern B Reference**: `pkg/workflowexecution/metrics/metrics.go`

---

**Triage Completed**: December 21, 2025
**Next Action**: Await user decision on Option A/B/C

