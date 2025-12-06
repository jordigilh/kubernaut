# Error Handling Philosophy - Signal Processing Service

**Date**: 2025-12-06
**Status**: ✅ Authoritative Guide
**Version**: 1.0
**Author**: Signal Processing Team

---

## 🎯 Core Principles

### 1. Error Classification

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**: K8s API timeouts, Data Storage Service 503, network errors
- **Strategy**: Exponential backoff with jitter (requeue with delay)
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**: Invalid CRD spec, missing required fields, Rego policy syntax error
- **Strategy**: Fail immediately, update status.phase = Failed, log error
- **Max Retries**: 0 (no retry)

#### **Partial Errors** (Graceful Degradation)
- **Definition**: Some operations succeed while others fail
- **Examples**: K8s API returns partial namespace data, one classifier fails
- **Strategy**: Continue with available data, set confidence = 0.5 for failed components
- **Max Retries**: N/A (proceed with degraded results)

---

### 2. Signal Processing-Specific Error Categories (A-E)

> **Source**: Adapted from Notification Controller v3.2 patterns

#### **Category A: SignalProcessing CR Not Found**
- **When**: CRD deleted during reconciliation (race condition)
- **Action**: Log deletion, return without error (normal cleanup)
- **Recovery**: Automatic (Kubernetes garbage collection)
- **Metric**: `signalprocessing_reconciliation_total{result="not_found"}`

```go
// Category A: Handle CR deleted during reconciliation
if apierrors.IsNotFound(err) {
    log.Info("SignalProcessing CR deleted during reconciliation, skipping")
    return ctrl.Result{}, nil // No requeue, normal cleanup
}
```

#### **Category B: K8s API Errors** (Retry with Backoff)
- **When**: K8s API timeouts, 503 errors, rate limiting (429)
- **Action**: Exponential backoff (30s → 60s → 120s → 240s → 480s)
- **Recovery**: Automatic retry up to 5 attempts, then mark as failed
- **Metric**: `signalprocessing_k8s_api_errors_total{error_type="..."}`

```go
// Category B: K8s API transient error
if isTransientK8sError(err) {
    log.Error(err, "K8s API transient error, will retry",
        "attempt", attemptCount, "backoff", CalculateBackoff(attemptCount))
    return HandleTransientError(attemptCount), nil
}
```

#### **Category C: Rego Policy Errors** (User Configuration Error)
- **When**: Rego syntax error, invalid policy output, policy not found
- **Action**: Mark as failed immediately, create Kubernetes Event
- **Recovery**: Manual (fix Rego policy in ConfigMap, controller will re-evaluate)
- **Metric**: `signalprocessing_rego_policy_errors_total{policy="...",error_type="..."}`

```go
// Category C: Rego policy user error (permanent)
if isRegoPolicyError(err) {
    log.Error(err, "Rego policy configuration error - manual intervention required",
        "policy", policyName, "error", err.Error())
    r.recorder.Event(sp, corev1.EventTypeWarning, "RegoPolicyError",
        fmt.Sprintf("Rego policy %s has configuration error: %v", policyName, err))
    return HandlePermanentError(), r.updateStatusFailed(ctx, sp, err)
}
```

#### **Category D: Status Update Conflicts** (Optimistic Locking)
- **When**: Multiple reconcile attempts updating status simultaneously
- **Action**: Retry status update with fresh resource version (3 attempts)
- **Recovery**: Automatic (retry with latest version)
- **Metric**: `signalprocessing_status_update_conflicts_total`

```go
// Category D: Status update with retry for conflicts
func (r *SignalProcessingReconciler) updateStatusWithRetry(ctx context.Context, sp *v1alpha1.SignalProcessing) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Get fresh version
        fresh := &v1alpha1.SignalProcessing{}
        if err := r.Get(ctx, client.ObjectKeyFromObject(sp), fresh); err != nil {
            return err
        }
        fresh.Status = sp.Status
        return r.Status().Update(ctx, fresh)
    })
}
```

#### **Category E: Enrichment/Classification Failures** (Partial Data)
- **When**: K8s enrichment partially succeeds, one classifier fails while others succeed
- **Action**: Continue with available data, set component confidence to 0.0
- **Recovery**: Automatic (degraded results are acceptable for non-critical data)
- **Metric**: `signalprocessing_partial_success_total{component="..."}`

```go
// Category E: Partial enrichment (graceful degradation)
k8sContext, err := r.enricher.EnrichSignal(ctx, signal)
if err != nil {
    if isPartialEnrichmentError(err) {
        log.Info("Partial K8s enrichment - proceeding with degraded context",
            "missing_fields", err.(*PartialEnrichmentError).MissingFields)
        k8sContext = err.(*PartialEnrichmentError).PartialContext
        k8sContext.Confidence = 0.5 // Reduced confidence
    } else {
        return HandleTransientError(attemptCount), err
    }
}
```

---

## 🔄 Retry Strategy for CRD Controller

### Requeue with Backoff

```go
package controller

import (
    "math"
    "math/rand"
    "time"

    ctrl "sigs.k8s.io/controller-runtime"
)

// CalculateBackoff returns exponential backoff duration for controller requeue.
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func CalculateBackoff(attemptCount int) time.Duration {
    baseDelay := 30 * time.Second
    maxDelay := 8 * time.Minute

    delay := time.Duration(math.Pow(2, float64(attemptCount))) * baseDelay
    if delay > maxDelay {
        delay = maxDelay
    }

    // Add jitter (±10%)
    jitter := time.Duration(rand.Float64()*0.2-0.1) * delay
    return delay + jitter
}

// HandleTransientError returns ctrl.Result for requeue with backoff.
func HandleTransientError(attemptCount int) ctrl.Result {
    return ctrl.Result{
        RequeueAfter: CalculateBackoff(attemptCount),
    }
}

// HandlePermanentError returns ctrl.Result for no requeue (failed state).
func HandlePermanentError() ctrl.Result {
    return ctrl.Result{Requeue: false}
}
```

---

## 📊 Error Handling Decision Matrix

| Error Category | Example | Retry | Backoff | Max Attempts | Final State |
|----------------|---------|-------|---------|--------------|-------------|
| **A: CR Not Found** | CRD deleted | ❌ | N/A | N/A | Normal (cleanup) |
| **B: K8s API** | 503, timeout | ✅ | Exponential | 5 | Failed |
| **C: Rego Policy** | Syntax error | ❌ | N/A | 0 | Failed (Event) |
| **D: Conflict** | Stale version | ✅ | None | 3 | Retry or Failed |
| **E: Partial** | Missing Pod | ⚠️ | N/A | N/A | Degraded (0.5 conf) |

---

## 🛡️ Graceful Degradation Strategy

### Per-Component Confidence Scoring

When a component fails but others succeed, the controller continues with reduced confidence:

| Component | Success | Failure | Notes |
|-----------|---------|---------|-------|
| **K8s Enricher** | 1.0 | 0.5 | Falls back to signal labels |
| **Environment Classifier** | 0.95 | 0.0 | Returns "unknown" |
| **Priority Engine** | 0.95 | 0.6 | Falls back to severity-based |
| **Business Classifier** | Varies | 0.4 | Returns defaults |

### Overall Confidence Calculation

```go
// CalculateOverallConfidence computes weighted confidence from components.
func CalculateOverallConfidence(components []ComponentResult) float64 {
    var sum, weightSum float64
    weights := map[string]float64{
        "k8s_enricher":          0.3,
        "environment_classifier": 0.25,
        "priority_engine":        0.25,
        "business_classifier":    0.2,
    }

    for _, c := range components {
        if w, ok := weights[c.Name]; ok {
            sum += c.Confidence * w
            weightSum += w
        }
    }

    if weightSum == 0 {
        return 0.0
    }
    return sum / weightSum
}
```

---

## 📈 Observability Requirements

### Required Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `signalprocessing_reconciliation_total` | Counter | `result` | Total reconciliations |
| `signalprocessing_k8s_api_errors_total` | Counter | `error_type` | K8s API failures |
| `signalprocessing_rego_policy_errors_total` | Counter | `policy`, `error_type` | Rego failures |
| `signalprocessing_status_update_conflicts_total` | Counter | - | Status conflicts |
| `signalprocessing_partial_success_total` | Counter | `component` | Degraded results |
| `signalprocessing_component_confidence` | Gauge | `component` | Component confidence |

### Required Logging

All errors MUST be logged with:
- Error message
- Component that failed
- Correlation ID (reconciliation request)
- Attempt count (for retries)
- Backoff duration (if applicable)

```go
log.Error(err, "Component operation failed",
    "component", "k8s_enricher",
    "reconcile_id", req.NamespacedName.String(),
    "attempt", attemptCount,
    "backoff", CalculateBackoff(attemptCount),
)
```

---

## 🔗 Related Documents

| Document | Description |
|----------|-------------|
| [BR-SP-070-072](../BUSINESS_REQUIREMENTS.md) | Priority assignment with fallback |
| [DD-INFRA-001](../../../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md) | Hot-reload pattern |
| [IMPLEMENTATION_PLAN_V1.25.md](../IMPLEMENTATION_PLAN_V1.25.md) | Day 6 implementation |

---

## 📝 Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial release - Day 6 EOD document |
