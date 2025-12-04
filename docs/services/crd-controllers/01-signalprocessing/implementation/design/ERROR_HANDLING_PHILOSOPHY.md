# Error Handling Philosophy - Signal Processing Service

**Date**: 2025-12-04
**Status**: ✅ Authoritative Guide
**Version**: 1.0
**Part of**: Signal Processing Implementation Plan V1.23

---

## 🎯 **Core Principles**

### 1. **Error Classification**

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

### 2. **Signal Processing-Specific Error Categories (A-E)**

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
        // Fetch latest version before update
        latest := &v1alpha1.SignalProcessing{}
        if err := r.Client.Get(ctx, client.ObjectKeyFromObject(sp), latest); err != nil {
            return err
        }
        // Copy status to latest version and update
        latest.Status = sp.Status
        return r.Client.Status().Update(ctx, latest)
    })
}
```

#### **Category E: Audit Write Failures** (Fire-and-Forget)
- **When**: Data Storage Service unavailable, buffer overflow
- **Action**: Log warning, continue processing (audit is non-blocking per ADR-038)
- **Recovery**: Automatic (buffered events flushed later)
- **Metric**: `signalprocessing_audit_write_failures_total`

```go
// Category E: Audit failure (non-blocking per ADR-038)
if err := r.auditStore.WriteAsync(ctx, auditEvent); err != nil {
    log.Error(err, "Audit write failed - continuing (fire-and-forget)",
        "signalprocessing", sp.Name)
    r.metrics.AuditWriteFailures.Inc()
    // Do NOT return error - audit is non-blocking
}
```

---

## 🔄 **Retry Strategy for CRD Controller**

### **Exponential Backoff Configuration**

| Attempt | Backoff | With Jitter (±10%) | Cumulative |
|---------|---------|-------------------|------------|
| 1 | 30s | 27-33s | ~30s |
| 2 | 60s | 54-66s | ~1.5m |
| 3 | 120s | 108-132s | ~3.5m |
| 4 | 240s | 216-264s | ~7.5m |
| 5 | 480s | 432-528s | ~15.5m |

### **Implementation**

```go
const (
    baseBackoff    = 30 * time.Second
    maxBackoff     = 8 * time.Minute
    jitterFraction = 0.1 // ±10%
    maxAttempts    = 5
)

// CalculateBackoff returns exponential backoff duration with jitter
func CalculateBackoff(attemptCount int) time.Duration {
    backoff := baseBackoff * time.Duration(math.Pow(2, float64(attemptCount)))
    if backoff > maxBackoff {
        backoff = maxBackoff
    }
    
    // Add ±10% jitter to prevent thundering herd
    jitter := time.Duration(float64(backoff) * jitterFraction * (rand.Float64()*2 - 1))
    return backoff + jitter
}

// ShouldRetry determines if error should be retried based on attempt count and error type
func ShouldRetry(attemptCount int, err error) bool {
    if attemptCount >= maxAttempts {
        return false
    }
    return IsRetryableError(err)
}

// IsRetryableError checks if error is transient and should be retried
func IsRetryableError(err error) bool {
    // Context timeouts are retryable
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    
    // K8s API transient errors
    if apierrors.IsServerTimeout(err) ||
       apierrors.IsTooManyRequests(err) ||
       apierrors.IsServiceUnavailable(err) ||
       apierrors.IsInternalError(err) {
        return true
    }
    
    // Permanent errors should NOT retry
    if apierrors.IsNotFound(err) ||
       apierrors.IsBadRequest(err) ||
       apierrors.IsUnauthorized(err) ||
       apierrors.IsForbidden(err) {
        return false
    }
    
    return false
}
```

---

## 📦 **Error Wrapping Pattern**

### **Standard Error Wrapper**

```go
package errors

import (
    "fmt"
    
    pkgerrors "github.com/pkg/errors"
)

// SignalProcessingError wraps errors with category and context
type SignalProcessingError struct {
    Category    ErrorCategory
    Operation   string
    Resource    string
    Cause       error
    Retryable   bool
}

type ErrorCategory string

const (
    CategoryCRNotFound      ErrorCategory = "CR_NOT_FOUND"
    CategoryK8sAPIError     ErrorCategory = "K8S_API_ERROR"
    CategoryRegoPolicyError ErrorCategory = "REGO_POLICY_ERROR"
    CategoryStatusConflict  ErrorCategory = "STATUS_CONFLICT"
    CategoryAuditFailure    ErrorCategory = "AUDIT_FAILURE"
)

func (e *SignalProcessingError) Error() string {
    return fmt.Sprintf("[%s] %s on %s: %v", e.Category, e.Operation, e.Resource, e.Cause)
}

func (e *SignalProcessingError) Unwrap() error {
    return e.Cause
}

// NewK8sAPIError creates a Category B error
func NewK8sAPIError(operation, resource string, cause error) *SignalProcessingError {
    return &SignalProcessingError{
        Category:  CategoryK8sAPIError,
        Operation: operation,
        Resource:  resource,
        Cause:     cause,
        Retryable: IsRetryableError(cause),
    }
}

// NewRegoPolicyError creates a Category C error (permanent)
func NewRegoPolicyError(policyName string, cause error) *SignalProcessingError {
    return &SignalProcessingError{
        Category:  CategoryRegoPolicyError,
        Operation: "evaluate_policy",
        Resource:  policyName,
        Cause:     cause,
        Retryable: false, // Policy errors are permanent until user fixes config
    }
}
```

---

## 📝 **Logging Standards**

### **Error Logging Levels**

| Level | When to Use | Example |
|-------|-------------|---------|
| **Error** | Operation failed, needs attention | K8s API 500, Rego syntax error |
| **Warn** | Degraded mode, non-critical failure | Partial enrichment, audit buffer full |
| **Info** | Normal operations, state changes | Phase transitions, reconcile start/end |
| **Debug** | Detailed troubleshooting | Individual API calls, policy evaluation |

### **Structured Logging Fields**

```go
// Standard fields for all error logs
log.Error(err, "Enrichment failed",
    "phase", sp.Status.Phase,
    "signalprocessing", sp.Name,
    "namespace", sp.Namespace,
    "attempt", attemptCount,
    "backoff", CalculateBackoff(attemptCount),
    "errorCategory", errorCategory,
    "retryable", isRetryable,
)
```

---

## 📊 **Metrics for Error Tracking**

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `signalprocessing_reconciliation_total` | Counter | `phase`, `result` | Track all reconciliation outcomes |
| `signalprocessing_errors_total` | Counter | `category`, `retryable` | Error classification |
| `signalprocessing_k8s_api_errors_total` | Counter | `error_type`, `resource` | K8s API failures |
| `signalprocessing_rego_policy_errors_total` | Counter | `policy`, `error_type` | Policy evaluation failures |
| `signalprocessing_audit_write_failures_total` | Counter | - | Audit system failures |
| `signalprocessing_status_update_conflicts_total` | Counter | - | Optimistic locking conflicts |

---

## ✅ **Error Handling Checklist**

Before implementing any new operation, verify:

- [ ] Error is categorized (A-E)
- [ ] Appropriate retry strategy selected
- [ ] Error is wrapped with context
- [ ] Structured log entry created
- [ ] Metric incremented
- [ ] Status updated appropriately
- [ ] Kubernetes Event created (if user-actionable)

---

## 📚 **References**

- [ADR-038: Async Buffered Audit Ingestion](../../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md)
- [DD-005: Observability Standards](../../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [DD-007: Graceful Shutdown](../../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)
- [Implementation Plan V1.23](../IMPLEMENTATION_PLAN.md)

