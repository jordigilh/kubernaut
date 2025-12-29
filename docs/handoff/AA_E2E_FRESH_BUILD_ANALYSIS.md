# AIAnalysis E2E Tests - Fresh Build Analysis (Dec 15, 2025, 13:45)

## 🎯 Executive Summary

**Results**: **19/25 passing (76%)** - ❌ **REGRESSION** from 20/25 (80%)

**Status**: ❌ **FIXES DID NOT WORK** + 1 NEW FAILURE

**Root Cause**: Prometheus counter metrics don't appear until first increment + HAPI mock not configured for failure scenarios

---

## 📊 Test Results Breakdown

### Failures (6 total, vs 5 before)

| Test | Status | Category | Root Cause |
|------|--------|----------|-----------|
| **1. Data Storage health check** | ❌ FAIL | Pre-existing | Infrastructure issue |
| **2. HolmesGPT-API health check** | ❌ FAIL | Pre-existing | Infrastructure issue |
| **3. aianalysis_failures_total metric** | ❌ FAIL | My fix didn't work | Metric never incremented |
| **4. Data quality warnings approval** | ❌ FAIL | My fix didn't work | HAPI mock config issue |
| **5. Recovery status metrics** | ❌ **NEW FAIL** | Regression | Unknown (was passing) |
| **6. Full 4-phase reconciliation** | ❌ FAIL | Pre-existing | Timeout (3 min) |

---

## 🔍 Root Cause Analysis

### Issue 1: aianalysis_failures_total Metric Never Appears

**Expected**: Metric shows in `/metrics` output  
**Actual**: Metric missing entirely

**Root Cause Discovery**:
1. ✅ Metric IS defined in `pkg/aianalysis/metrics/metrics.go` (line 117)
2. ✅ Metric IS registered in `init()` (line 191)
3. ✅ Code IS in fresh build (controller pod created at 18:42:44 UTC)
4. ❌ **Metric never incremented** - Prometheus counters don't appear until first `.Inc()` call

**Evidence**:
```bash
# Metrics that DO appear (have been incremented):
aianalysis_approval_decisions_total
aianalysis_confidence_score_distribution
aianalysis_reconciler_reconciliations_total
aianalysis_recovery_status_populated_total  # ← This appears!
aianalysis_rego_evaluations_total

# Metric that DOESN'T appear (never incremented):
aianalysis_failures_total  # ← Missing!
```

**Why It's Missing**:
- Test `seedMetricsWithAnalysis()` creates analysis with fingerprint `"TRIGGER_WORKFLOW_RESOLUTION_FAILURE"`
- HAPI mock doesn't recognize this fingerprint as special
- Analysis completes successfully (no failure triggered)
- `metrics.FailuresTotal.WithLabelValues(...).Inc()` never called
- Prometheus counter never initialized → doesn't appear in `/metrics`

---

### Issue 2: Data Quality Warnings Test Failing

**Test**: "should require approval for data quality issues in production"

**Expected**: Rego policy checks `input.warnings` and `input.failed_detections`  
**Actual**: Test still failing

**Root Cause**: HAPI mock response doesn't include `warnings` field

**Evidence from test** (line 96):
```go
Fingerprint: "TRIGGER_WORKFLOW_RESOLUTION_FAILURE",
```

**Problem**: HAPI mock returns success response, not warnings/failures

---

### Issue 3: Recovery Status Metrics Regression (NEW)

**Test**: "should include recovery status metrics"

**Status**: Was passing before, now failing

**Possible Causes**:
1. Fresh build changed something in recovery status population
2. HAPI mock response structure changed
3. Metric seeding didn't trigger recovery flow

**Needs Investigation**: Check what changed in recovery status logic

---

## 🛠️ Required Fixes

### Fix 1: Configure HAPI Mock for Failure Scenarios

**File**: `test/infrastructure/aianalysis.go` (HAPI mock setup)

**Add Handler for Special Fingerprints**:
```go
// In deployHolmesGPTAPI function, add:
case "TRIGGER_WORKFLOW_RESOLUTION_FAILURE":
    // Return response that triggers approval logic
    json.NewEncoder(w).Encode(hapiResponse{
        IncidentID: req.IncidentID,
        Analysis: hapiAnalysis{
            WorkflowID: "",  // Empty = workflow resolution failure
            Confidence: 0.0,
            Warnings: []string{
                "No workflow could be resolved",
                "Data quality issue: missing labels",
            },
        },
        RecoveryAnalysis: nil,
    })
case "DATA_QUALITY_WARNINGS_PRODUCTION":
    // Return response with data quality warnings
    json.NewEncoder(w).Encode(hapiResponse{
        IncidentID: req.IncidentID,
        Analysis: hapiAnalysis{
            WorkflowID: "mock-generic-restart-v1",
            Confidence: 0.75,
            Warnings: []string{
                "Label detection failed for: environment",
                "Label detection failed for: priority",
            },
        },
        FailedDetections: []string{
            "environment_classification",
            "business_priority",
        },
        RecoveryAnalysis: nil,
    })
```

**Impact**: Enables tests to trigger specific failure scenarios

---

### Fix 2: Initialize aianalysis_failures_total Metric

**Option A: Eagerly Initialize in init()** (RECOMMENDED)
```go
// In pkg/aianalysis/metrics/metrics.go init()
func init() {
    metrics.Registry.MustRegister(
        // ... existing registrations ...
    )
    
    // Initialize failure metric with zero values so it appears in /metrics
    FailuresTotal.WithLabelValues("NoWorkflowSelected", "InvestigationFailed").Add(0)
    FailuresTotal.WithLabelValues("APIError", "HolmesGPTAPICallFailed").Add(0)
    FailuresTotal.WithLabelValues("RegoEvaluationError", "PolicyEvaluationFailed").Add(0)
    // ... other known failure types ...
}
```

**Option B: Use prometheus.NewCounterFunc** (Alternative)
```go
// Automatically shows counter even if zero
FailuresTotal = prometheus.NewCounterFunc(
    prometheus.CounterOpts{
        Name: "aianalysis_failures_total",
        Help: "Total number of AIAnalysis failures",
    },
    func() float64 { return 0 }, // Placeholder
)
```

**Recommendation**: Option A - matches existing patterns, explicit initialization

---

### Fix 3: Update seedMetricsWithAnalysis

**File**: `test/e2e/aianalysis/02_metrics_test.go`

**Update Failed Analysis Creation**:
```go
failedAnalysis := &aianalysisv1alpha1.AIAnalysis{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "metrics-seed-failed-" + randomSuffix(),
        Namespace: "kubernaut-system",
    },
    Spec: aianalysisv1alpha1.AIAnalysisSpec{
        RemediationRequestRef: corev1.ObjectReference{
            Name:      "metrics-seed-rem-fail",
            Namespace: "kubernaut-system",
        },
        RemediationID: "metrics-seed-fail-001",
        AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
            SignalContext: aianalysisv1alpha1.SignalContextInput{
                Fingerprint:      "TRIGGER_WORKFLOW_RESOLUTION_FAILURE", // ✅ Keep this
                Severity:         "critical",
                SignalType:       "TestFailureScenario",
                Environment:      "production",  // ← Change to production for data quality test
                BusinessPriority: "P0",
                TargetResource: aianalysisv1alpha1.TargetResource{
                    Kind:      "Pod",
                    Namespace: "default",
                    Name:      "test-pod-fail",
                },
            },
            AnalysisTypes: []string{"investigation"},
        },
    },
}
```

**Add second failed analysis for data quality warnings**:
```go
dataQualityAnalysis := &aianalysisv1alpha1.AIAnalysis{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "metrics-seed-dataquality-" + randomSuffix(),
        Namespace: "kubernaut-system",
    },
    Spec: aianalysisv1alpha1.AIAnalysisSpec{
        // ... similar to above but with:
        Fingerprint: "DATA_QUALITY_WARNINGS_PRODUCTION",
        Environment: "production",
    },
}
```

---

## 📊 Expected Outcomes After Fixes

### Immediate Impact
- ✅ `aianalysis_failures_total` metric appears in `/metrics` (initialized to 0)
- ✅ Failed analyses actually trigger failure logic
- ✅ Data quality warnings test passes (Rego sees warnings)
- ✅ 22-23/25 tests passing (88-92%)

### Remaining Failures (Pre-existing)
1. ❌ Data Storage health check (2 tests) - Infrastructure issue
2. ❌ Full 4-phase reconciliation (1 test) - Timeout issue

---

## 🎯 Next Steps

### Immediate (Priority 1)
1. ✅ Fix HAPI mock to handle special fingerprints
2. ✅ Initialize `aianalysis_failures_total` metric in init()
3. ✅ Update `seedMetricsWithAnalysis` to trigger both failure types
4. ✅ Run E2E tests again

### Short-term (Priority 2)
1. ⏸️ Investigate recovery status metrics regression
2. ⏸️ Fix Data Storage health check
3. ⏸️ Fix HolmesGPT-API health check

### Medium-term (Priority 3)
1. ⏸️ Fix full 4-phase reconciliation timeout
2. ⏸️ Document E2E mock configuration patterns
3. ⏸️ Add E2E mock validation tests

---

## 📚 Lessons Learned

### 1. Prometheus Counter Behavior
**Lesson**: Counters don't appear in `/metrics` until first increment

**Solution**: Eagerly initialize with `.Add(0)` in `init()` or use test seeding

### 2. E2E Mock Configuration
**Lesson**: Mocks need special handling for failure scenarios

**Solution**: Document special fingerprints/headers for triggering test scenarios

### 3. Test Dependencies
**Lesson**: Tests assume metrics are always present (not just when incremented)

**Solution**: Initialize metrics in `init()` or seed with actual failures

---

## 🔍 Investigation Notes

### Why Fresh Build Didn't Fix Issues

**Expected**: Fresh build with `--no-cache` would include fixes  
**Actual**: Build succeeded, but fixes didn't work as designed

**Reason**: Fixes were correct for *production* code, but E2E tests have additional requirements:
1. HAPI mock must be configured for test scenarios
2. Prometheus metrics must be initialized (not just defined)
3. Test seeding must actually trigger failure code paths

**Conclusion**: Need both production code fixes AND E2E infrastructure updates

---

**Analysis Date**: December 15, 2025, 13:45 EST  
**Build**: Fresh (controller pod 18:42:44 UTC)  
**Image**: `localhost/kubernaut-aianalysis:latest`  
**Status**: ❌ **BLOCKED** - Need HAPI mock updates and metric initialization
