# Appendix B: Error Handling Philosophy - AI Analysis Service

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Last Updated**: 2025-12-04
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0
**Status**: ✅ Authoritative Guide

---

## 🎯 **Core Principles**

### 1. **Error Classification**

All AIAnalysis errors fall into three categories:

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**: HolmesGPT-API timeout, Data Storage API 503, K8s API connection errors
- **Strategy**: Exponential backoff with jitter
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**: 401 Unauthorized from HolmesGPT-API, 404 Not Found, Rego policy syntax errors, malformed EnrichmentResults
- **Strategy**: Fail immediately, log error, update status to `Failed`
- **Max Retries**: 0 (no retry)

#### **User Errors** (Input Validation)
- **Definition**: Invalid user input or configuration in `AIAnalysis.Spec`
- **Examples**: Missing required fields, invalid `FailedDetections` values, unknown workflow ID
- **Strategy**: Return validation error immediately in `Validating` phase, do not proceed
- **Max Retries**: 0 (no retry)

---

## 🏷️ **Service-Specific Error Categories** ⭐ V3.0

> **MANDATORY**: These 5 error categories (A-E) are specific to AIAnalysis. They map to the generic classification above but provide service-specific context.

### **Category A: AIAnalysis CRD Not Found**
- **When**: The `AIAnalysis` CRD is deleted during reconciliation (e.g., user deletes while investigating)
- **Action**: Log deletion, stop reconciliation, remove from work queue
- **Recovery**: Normal (no action needed - cascading deletion handled by K8s)
- **Example**: `AIAnalysis "aia-12345" deleted during HolmesGPT-API call`
- **Status Update**: None (CRD no longer exists)
- **Metric**: `aianalysis_crd_deleted_during_reconciliation_total`

```go
// Category A: CRD deletion handling
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var analysis aianalysisv1.AIAnalysis
    if err := r.Get(ctx, req.NamespacedName, &analysis); err != nil {
        if apierrors.IsNotFound(err) {
            // Category A: CRD deleted - nothing to do
            r.Log.Info("AIAnalysis deleted during reconciliation", "name", req.Name)
            r.Metrics.IncrementCRDDeleted(req.Namespace)
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }
    // Continue reconciliation...
}
```

---

### **Category B: HolmesGPT-API Errors** (Retry with Backoff)
- **When**: HolmesGPT-API timeout, rate limiting (429), 5xx errors, network failures
- **Action**: Exponential backoff (30s → 60s → 120s → 240s → 480s)
- **Recovery**: Automatic retry up to 5 attempts, then mark as `Failed` with reason
- **Example**: `HolmesGPT-API returned 503 Service Unavailable`
- **Status Update**: `status.phase = "Investigating"`, `status.retryCount++`
- **Metric**: `aianalysis_holmesgpt_api_errors_total{error_type="timeout|rate_limit|5xx"}`

```go
// Category B: HolmesGPT-API error handling with retry
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    response, err := h.holmesGPTClient.Investigate(ctx, analysis.Spec.AnalysisRequest)
    if err != nil {
        // Check if transient error (retry-able)
        if isTransientError(err) {
            retryCount := analysis.Status.RetryCount
            if retryCount >= MaxRetries {
                // Max retries exceeded - permanent failure
                return h.handlePermanentFailure(ctx, analysis, err, "HolmesGPT-API max retries exceeded")
            }

            // Calculate backoff delay
            delay := CalculateBackoff(retryCount)
            h.Log.Info("HolmesGPT-API transient error, scheduling retry",
                "error", err.Error(),
                "retryCount", retryCount,
                "nextRetryIn", delay.String(),
            )
            h.Metrics.IncrementHolmesGPTError("transient")

            // Update status with retry info
            analysis.Status.RetryCount = retryCount + 1
            analysis.Status.LastRetryTime = metav1.Now()
            return ctrl.Result{RequeueAfter: delay}, nil
        }

        // Permanent error - fail immediately
        return h.handlePermanentFailure(ctx, analysis, err, "HolmesGPT-API permanent error")
    }
    // Success - continue to next phase
    return h.processResponse(ctx, analysis, response)
}

func isTransientError(err error) bool {
    var apiErr *holmesgpt.APIError
    if errors.As(err, &apiErr) {
        switch apiErr.StatusCode {
        case 429, 502, 503, 504:
            return true
        }
    }
    // Network errors are transient
    var netErr net.Error
    if errors.As(err, &netErr) && netErr.Timeout() {
        return true
    }
    return false
}
```

---

### **Category C: Authentication/Authorization Errors** (Permanent Error)
- **When**: 401/403 auth errors from HolmesGPT-API, invalid credentials, K8s RBAC failures
- **Action**: Mark as `Failed` immediately, create Kubernetes Event
- **Recovery**: Manual (fix configuration, update secrets, fix RBAC)
- **Example**: `Invalid API key for HolmesGPT-API`, `Controller ServiceAccount lacks permissions`
- **Status Update**: `status.phase = "Failed"`, `status.message = "Authentication failed"`
- **Metric**: `aianalysis_auth_errors_total{error_type="api_key|rbac"}`

```go
// Category C: Authentication error handling
func (h *InvestigatingHandler) handleAuthError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
    h.Log.Error(err, "Authentication error - manual intervention required",
        "name", analysis.Name,
        "namespace", analysis.Namespace,
    )
    h.Metrics.IncrementAuthError("holmesgpt_api")

    // Create Event for visibility
    h.Recorder.Event(analysis, corev1.EventTypeWarning, "AuthenticationFailed",
        fmt.Sprintf("HolmesGPT-API authentication failed: %v", err))

    // Update status to Failed
    analysis.Status.Phase = aianalysisv1.PhaseFailed
    analysis.Status.Message = fmt.Sprintf("Authentication failed: %v. Check HolmesGPT-API credentials.", err)
    analysis.Status.FailedAt = metav1.Now()

    return ctrl.Result{}, nil // Don't requeue - manual fix required
}
```

---

### **Category D: Status/State Update Conflicts**
- **When**: Multiple processes updating same `AIAnalysis` CRD simultaneously (rare with single controller)
- **Action**: Retry with optimistic locking using `resourceVersion`
- **Recovery**: Automatic (retry status update after re-fetching)
- **Example**: `AIAnalysis status update conflict - version mismatch`
- **Status Update**: Re-fetch and retry
- **Metric**: `aianalysis_status_update_conflicts_total`

```go
// Category D: Status update conflict handling
func (r *AIAnalysisReconciler) updateStatusWithRetry(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Re-fetch the latest version
        latest := &aianalysisv1.AIAnalysis{}
        if err := r.Get(ctx, client.ObjectKeyFromObject(analysis), latest); err != nil {
            return err
        }

        // Copy our status changes to the latest version
        latest.Status = analysis.Status

        // Attempt update
        if err := r.Status().Update(ctx, latest); err != nil {
            if apierrors.IsConflict(err) {
                r.Log.Info("Status update conflict, retrying",
                    "name", analysis.Name,
                    "resourceVersion", latest.ResourceVersion,
                )
                r.Metrics.IncrementStatusConflict()
            }
            return err
        }
        return nil
    })
}
```

---

### **Category E: Rego Policy Evaluation Failures** (Graceful Degradation)
- **When**: Rego policy syntax error, policy timeout, unexpected input, missing ConfigMap
- **Action**: Log error, apply graceful degradation (default to manual approval)
- **Recovery**: Automatic (degraded operation), Manual (fix Rego policy)
- **Example**: `Rego policy "approval.rego" failed to evaluate - syntax error on line 15`
- **Status Update**: `status.phase = "Analyzing"`, `status.approvalRequired = true` (safe default)
- **Metric**: `aianalysis_rego_policy_failures_total{failure_type="syntax|timeout|missing"}`

```go
// Category E: Rego policy failure with graceful degradation
func (h *AnalyzingHandler) evaluateRegoPolicy(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*PolicyResult, error) {
    result, err := h.regoEngine.Evaluate(ctx, analysis)
    if err != nil {
        h.Log.Error(err, "Rego policy evaluation failed - defaulting to manual approval",
            "name", analysis.Name,
            "policyName", "approval.rego",
        )
        h.Metrics.IncrementRegoPolicyFailure(classifyRegoError(err))

        // Graceful degradation: default to manual approval (safe)
        h.Recorder.Event(analysis, corev1.EventTypeWarning, "RegoPolicyFailed",
            fmt.Sprintf("Rego policy evaluation failed: %v. Defaulting to manual approval.", err))

        return &PolicyResult{
            ApprovalRequired: true,  // Safe default
            Reason:           "Rego policy evaluation failed - manual review required",
            Degraded:         true,
        }, nil
    }
    return result, nil
}

func classifyRegoError(err error) string {
    switch {
    case strings.Contains(err.Error(), "syntax error"):
        return "syntax"
    case strings.Contains(err.Error(), "timeout"):
        return "timeout"
    case strings.Contains(err.Error(), "not found"):
        return "missing"
    default:
        return "unknown"
    }
}
```

---

## 🔄 **Retry Strategy**

### Exponential Backoff Implementation

```go
package retry

import (
    "math"
    "math/rand"
    "time"
)

const (
    BaseDelay = 30 * time.Second
    MaxDelay  = 480 * time.Second  // 8 minutes
    MaxRetries = 5
)

// CalculateBackoff returns exponential backoff duration with jitter
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func CalculateBackoff(attemptCount int) time.Duration {
    // Calculate exponential backoff: baseDelay * 2^attemptCount
    delay := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attemptCount)))

    // Cap at maximum delay
    if delay > MaxDelay {
        delay = MaxDelay
    }

    // Add jitter (±10%) to prevent thundering herd
    jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

    return jitter
}

// ShouldRetry determines if an error is retry-able
func ShouldRetry(err error, attemptCount int) bool {
    if attemptCount >= MaxRetries {
        return false
    }
    return isTransientError(err)
}
```

### Retry Decision Matrix

| Error Type | Retry? | Max Attempts | Backoff |
|------------|--------|--------------|---------|
| HolmesGPT-API 5xx | ✅ Yes | 5 | Exponential |
| HolmesGPT-API 429 | ✅ Yes | 5 | Exponential |
| HolmesGPT-API timeout | ✅ Yes | 5 | Exponential |
| HolmesGPT-API 401/403 | ❌ No | 0 | — |
| HolmesGPT-API 400 | ❌ No | 0 | — |
| K8s API 503 | ✅ Yes | 5 | Exponential |
| K8s API 409 Conflict | ✅ Yes | 3 | Fixed (1s) |
| Rego syntax error | ❌ No | 0 | — |
| Rego timeout | ✅ Yes | 3 | Fixed (5s) |
| Validation error | ❌ No | 0 | — |

---

## 🔐 **Circuit Breaker Pattern**

### HolmesGPT-API Circuit Breaker

```go
package circuit

import (
    "sync"
    "time"
)

type State int

const (
    StateClosed State = iota  // Normal operation
    StateOpen                  // Failing, reject requests
    StateHalfOpen             // Testing recovery
)

type CircuitBreaker struct {
    mu              sync.Mutex
    state           State
    failureCount    int
    successCount    int
    lastFailureTime time.Time

    // Configuration
    failureThreshold int           // Failures before opening
    successThreshold int           // Successes to close
    timeout          time.Duration // Time before half-open
}

func NewCircuitBreaker() *CircuitBreaker {
    return &CircuitBreaker{
        state:            StateClosed,
        failureThreshold: 5,
        successThreshold: 3,
        timeout:          60 * time.Second,
    }
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case StateOpen:
        // Check if timeout elapsed
        if time.Since(cb.lastFailureTime) > cb.timeout {
            cb.state = StateHalfOpen
            cb.successCount = 0
        } else {
            return ErrCircuitOpen
        }
    }

    // Execute the function
    err := fn()

    if err != nil {
        cb.recordFailure()
        return err
    }

    cb.recordSuccess()
    return nil
}

func (cb *CircuitBreaker) recordFailure() {
    cb.failureCount++
    cb.lastFailureTime = time.Now()

    if cb.failureCount >= cb.failureThreshold {
        cb.state = StateOpen
    }
}

func (cb *CircuitBreaker) recordSuccess() {
    if cb.state == StateHalfOpen {
        cb.successCount++
        if cb.successCount >= cb.successThreshold {
            cb.state = StateClosed
            cb.failureCount = 0
        }
    } else {
        cb.failureCount = 0
    }
}
```

### Circuit Breaker States

```
┌─────────────────────────────────────────────────────────────┐
│                    CIRCUIT BREAKER STATES                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────┐   5 failures   ┌──────────┐                   │
│  │  CLOSED  │ ─────────────► │   OPEN   │                   │
│  │ (Normal) │                │ (Failing)│                   │
│  └──────────┘                └──────────┘                   │
│       ▲                            │                         │
│       │ 3 successes       60s timeout                       │
│       │                            ▼                         │
│       │                    ┌───────────┐                    │
│       └─────────────────── │ HALF-OPEN │                    │
│                            │ (Testing) │                    │
│                            └───────────┘                    │
│                                   │                          │
│                            failure ▼                         │
│                            ┌──────────┐                     │
│                            │   OPEN   │ (reset timeout)     │
│                            └──────────┘                     │
└─────────────────────────────────────────────────────────────┘
```

---

## 📝 **Error Wrapping & Context**

### Standard Error Wrapping Pattern

```go
package errors

import (
    "fmt"

    pkgerrors "github.com/pkg/errors"
)

// Wrap adds context to an error while preserving the original
func Wrap(err error, message string) error {
    if err == nil {
        return nil
    }
    return pkgerrors.Wrap(err, message)
}

// Wrapf adds formatted context to an error
func Wrapf(err error, format string, args ...interface{}) error {
    if err == nil {
        return nil
    }
    return pkgerrors.Wrapf(err, format, args...)
}

// Usage example
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    response, err := h.holmesGPTClient.Investigate(ctx, analysis.Spec.AnalysisRequest)
    if err != nil {
        return errors.Wrapf(err, "failed to investigate AIAnalysis %s/%s",
            analysis.Namespace, analysis.Name)
    }

    if err := h.processResponse(ctx, analysis, response); err != nil {
        return errors.Wrap(err, "failed to process HolmesGPT response")
    }

    return nil
}
```

### Context Propagation

```go
// Always propagate context for cancellation and timeout support
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    // Create child context with timeout
    ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
    defer cancel()

    // Pass context to all downstream calls
    response, err := h.holmesGPTClient.Investigate(ctx, analysis.Spec.AnalysisRequest)
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return errors.Wrap(err, "HolmesGPT-API call timed out after 60s")
        }
        if ctx.Err() == context.Canceled {
            return errors.Wrap(err, "HolmesGPT-API call canceled")
        }
        return err
    }
    return h.processResponse(ctx, analysis, response)
}
```

---

## 📊 **Logging Best Practices**

### Structured Logging Pattern

```go
// Use logr.Logger (per DD-005)
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues(
        "aianalysis", req.NamespacedName,
        "reconcileID", uuid.New().String(),
    )

    // Info level: Normal operations
    log.Info("Starting reconciliation")

    // Error level: Failures (always include error)
    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", analysis.Status.Phase,
            "retryCount", analysis.Status.RetryCount,
        )
    }

    // Debug level: Detailed debugging (use V(1) for debug)
    log.V(1).Info("Processing phase",
        "currentPhase", analysis.Status.Phase,
        "targetPhase", targetPhase,
    )
}
```

### Log Levels

| Level | Usage | Example |
|-------|-------|---------|
| **Error** | Failures requiring attention | `log.Error(err, "HolmesGPT-API call failed")` |
| **Info** | Normal operations | `log.Info("Reconciliation complete", "phase", "Completed")` |
| **Debug** | Detailed debugging | `log.V(1).Info("Rego policy evaluation result", "decision", result)` |

---

## 🚨 **Error Recovery Strategies**

### Graceful Degradation

| Component | Failure Mode | Degraded Behavior |
|-----------|--------------|-------------------|
| **HolmesGPT-API** | Unavailable | Retry with backoff, then fail with clear message |
| **Rego Policy** | Syntax error | Default to manual approval (safe) |
| **Data Storage** | Audit write fails | Log locally, continue reconciliation |
| **ConfigMap** | Missing | Use embedded default policy |

### Recovery Actions by Error Category

| Category | Automatic Recovery | Manual Recovery |
|----------|-------------------|-----------------|
| **A: CRD Deleted** | Log and stop | None needed |
| **B: HolmesGPT-API** | Retry with backoff | Check HolmesGPT-API health |
| **C: Auth Errors** | None | Fix credentials/RBAC |
| **D: Conflicts** | Retry with re-fetch | None needed |
| **E: Rego Failures** | Default to manual approval | Fix Rego policy |

---

## ✅ **Error Handling Checklist**

Before marking implementation complete, verify:

- [ ] All error categories (A-E) have handlers
- [ ] Transient errors use exponential backoff
- [ ] Permanent errors fail immediately with clear message
- [ ] Auth errors create Kubernetes Events
- [ ] Rego failures have graceful degradation
- [ ] All errors are logged with structured context
- [ ] Error metrics exported (`aianalysis_errors_total{category="..."}`)
- [ ] Circuit breaker implemented for HolmesGPT-API
- [ ] Context propagated for timeout/cancellation support

---

## 📚 **Related Documents**

- [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md) - Main implementation plan
- [APPENDIX_A_EOD_TEMPLATES.md](./APPENDIX_A_EOD_TEMPLATES.md) - EOD documentation templates
- [APPENDIX_C_CONFIDENCE_METHODOLOGY.md](./APPENDIX_C_CONFIDENCE_METHODOLOGY.md) - Confidence calculation
- [DD-005: Observability Standards](../../../../../architecture/decisions/DD-005-observability-standards.md) - Logging standards

