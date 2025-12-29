# Error Handling Philosophy - AI Analysis Service

**Date**: 2025-12-04
**Status**: 📋 Template - Complete at Day 5 EOD
**Version**: 1.0
**Parent**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)

---

## 🎯 **Core Principles**

### 1. **Error Classification**

All errors fall into three categories:

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**:
  - HolmesGPT-API network timeouts
  - 503 Service Unavailable from HolmesGPT-API
  - Kubernetes API connection errors
  - Data Storage service temporarily unavailable
- **Strategy**: Exponential backoff with jitter
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**:
  - 401 Unauthorized from HolmesGPT-API (invalid API key)
  - 404 Not Found (workflow ID doesn't exist in catalog)
  - Validation failures (malformed SignalContext)
  - Rego policy syntax errors
- **Strategy**: Fail immediately, log error, update status
- **Max Retries**: 0 (no retry)

#### **User Errors** (Input Validation)
- **Definition**: Invalid user input or configuration in `AIAnalysis.Spec`
- **Examples**:
  - Missing required fields (`fingerprint`, `targetResource`)
  - Invalid formats (`businessPriority` not in expected values)
  - Unknown fields in `FailedDetections` array
  - Invalid Rego policy in ConfigMap
- **Strategy**: Return validation error immediately, do not retry
- **Max Retries**: 0 (no retry)

---

## 🏷️ **Service-Specific Error Categories**

> **AIAnalysis has 5 error categories (A-E)** that map to the generic classification above.

### **Category A: AIAnalysis CRD Not Found**
- **When**: The `AIAnalysis` CRD is deleted during reconciliation
- **Classification**: Permanent (expected)
- **Action**: Log deletion, remove from retry queue
- **Recovery**: Normal operation (no action needed)
- **Example Error**: `AIAnalysis "aia-test-123" not found`

```go
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var analysis aianalysisv1.AIAnalysis
    if err := r.Get(ctx, req.NamespacedName, &analysis); err != nil {
        if apierrors.IsNotFound(err) {
            // Category A: CRD deleted - normal operation
            r.Log.Info("AIAnalysis deleted, skipping reconciliation",
                "name", req.Name, "namespace", req.Namespace)
            return ctrl.Result{}, nil
        }
        // Unexpected error - retry
        return ctrl.Result{}, err
    }
    // ... continue reconciliation
}
```

### **Category B: HolmesGPT-API Errors** (Retry with Backoff)
- **When**: HolmesGPT-API timeout, rate limiting (429), 5xx errors
- **Classification**: Transient
- **Action**: Exponential backoff (30s → 60s → 120s → 240s → 480s)
- **Recovery**: Automatic retry up to 5 attempts, then mark as failed
- **Example Errors**:
  - `connection timeout to HolmesGPT-API`
  - `429 Too Many Requests`
  - `503 Service Unavailable`

```go
// Category B: HolmesGPT-API transient errors
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        resp, err := c.doRequest(ctx, req)
        if err == nil {
            return resp, nil
        }

        // Check if error is transient (retry-able)
        if isTransientError(err) {
            lastErr = err
            delay := calculateBackoff(attempt) // 30s, 60s, 120s, 240s, 480s
            c.log.Info("HolmesGPT-API transient error, retrying",
                "attempt", attempt+1,
                "maxRetries", maxRetries,
                "delay", delay,
                "error", err.Error())

            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
                continue
            }
        }

        // Permanent error - don't retry
        return nil, err
    }
    return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func isTransientError(err error) bool {
    // Network errors
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    // HTTP status codes
    var httpErr *HTTPError
    if errors.As(err, &httpErr) {
        switch httpErr.StatusCode {
        case 429, 500, 502, 503, 504:
            return true
        }
    }
    return false
}
```

### **Category C: Authentication/Authorization Errors** (Permanent)
- **When**: 401/403 auth errors from HolmesGPT-API, invalid K8s RBAC
- **Classification**: Permanent
- **Action**: Mark as failed immediately, create Kubernetes event
- **Recovery**: Manual intervention required (fix configuration/RBAC)
- **Example Errors**:
  - `401 Unauthorized: Invalid API key`
  - `403 Forbidden: ServiceAccount lacks permission`

```go
// Category C: Auth errors - permanent, no retry
func (c *HolmesGPTClient) handleAuthError(err error, analysis *aianalysisv1.AIAnalysis) error {
    c.log.Error(err, "Authentication error - manual intervention required",
        "name", analysis.Name,
        "namespace", analysis.Namespace,
        "error_type", "auth_failure")

    // Record event for visibility
    c.recorder.Event(analysis, corev1.EventTypeWarning, "AuthenticationFailed",
        fmt.Sprintf("HolmesGPT-API authentication failed: %v", err))

    // Update status to failed
    analysis.Status.Phase = "Failed"
    analysis.Status.Message = fmt.Sprintf("Authentication error: %v", err)

    return nil // Don't retry
}
```

### **Category D: Status/State Update Conflicts** (Retry with Optimistic Locking)
- **When**: Multiple processes updating same `AIAnalysis` CRD simultaneously
- **Classification**: Transient
- **Action**: Retry with optimistic locking (re-read, update, retry)
- **Recovery**: Automatic (retry status update)
- **Example Error**: `the object has been modified; please apply your changes to the latest version`

```go
// Category D: Conflict handling with optimistic locking
func (r *AIAnalysisReconciler) updateStatus(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Re-fetch the latest version
        var latest aianalysisv1.AIAnalysis
        if err := r.Get(ctx, client.ObjectKeyFromObject(analysis), &latest); err != nil {
            return err
        }

        // Apply our changes to the latest version
        latest.Status = analysis.Status

        // Attempt update
        if err := r.Status().Update(ctx, &latest); err != nil {
            if apierrors.IsConflict(err) {
                r.Log.Info("Status update conflict, retrying",
                    "name", analysis.Name,
                    "resourceVersion", latest.ResourceVersion)
            }
            return err
        }
        return nil
    })
}
```

### **Category E: Rego Policy Evaluation Failures** (Graceful Degradation)
- **When**: Rego policy syntax error, policy timeout, unexpected Rego input
- **Classification**: Permanent (but with fallback)
- **Action**: Log error, apply graceful degradation (default to manual approval)
- **Recovery**: Automatic (degraded operation), manual (fix Rego policy)
- **Example Errors**:
  - `rego: parse error in approval.rego`
  - `rego: evaluation timeout after 5s`

```go
// Category E: Rego policy failure with graceful degradation
func (r *RegoEngine) Evaluate(input *PolicyInput) (*PolicyResult, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    result, err := r.query.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        r.log.Error(err, "Rego policy evaluation failed - defaulting to manual approval",
            "policy", r.policyName,
            "input", input)

        // Record metric for degraded operation
        regoPolicyFailuresTotal.WithLabelValues(r.policyName, "evaluation_error").Inc()

        // Graceful degradation: default to manual approval (safe fallback)
        return &PolicyResult{
            Decision:     "manual_review",
            Reason:       fmt.Sprintf("Policy evaluation failed: %v", err),
            IsDegraded:   true,
            PolicyHash:   r.policyHash,
        }, nil // Return result, not error - allow workflow to continue
    }

    return r.parseResult(result)
}
```

---

## 🏷️ **Error Category Summary Table**

| Category | Error Type | Action | Recovery | Metrics Label |
|----------|------------|--------|----------|---------------|
| **A** | CRD Not Found | Log, skip | Normal | `crd_deleted` |
| **B** | HolmesGPT-API Transient | Retry with backoff | Automatic | `holmesgpt_transient` |
| **C** | Auth/RBAC Failure | Fail immediately | Manual | `auth_failure` |
| **D** | Status Conflict | Optimistic retry | Automatic | `status_conflict` |
| **E** | Rego Policy Failure | Graceful degradation | Degraded | `rego_failure` |

---

## 🔄 **Retry Strategy**

### **Exponential Backoff Implementation**

```go
package retry

import (
    "math"
    "math/rand"
    "time"
)

// Config for exponential backoff
type Config struct {
    InitialDelay time.Duration // 30s
    MaxDelay     time.Duration // 480s (8 minutes)
    Multiplier   float64       // 2.0
    Jitter       float64       // 0.1 (10%)
    MaxRetries   int           // 5
}

// DefaultConfig for AIAnalysis HolmesGPT-API calls
var DefaultConfig = Config{
    InitialDelay: 30 * time.Second,
    MaxDelay:     480 * time.Second,
    Multiplier:   2.0,
    Jitter:       0.1,
    MaxRetries:   5,
}

// CalculateDelay returns delay for given attempt (0-indexed)
func (c *Config) CalculateDelay(attempt int) time.Duration {
    if attempt >= c.MaxRetries {
        return c.MaxDelay
    }

    // Exponential: InitialDelay * Multiplier^attempt
    delay := float64(c.InitialDelay) * math.Pow(c.Multiplier, float64(attempt))

    // Cap at MaxDelay
    if delay > float64(c.MaxDelay) {
        delay = float64(c.MaxDelay)
    }

    // Add jitter (±10%)
    jitter := delay * c.Jitter * (rand.Float64()*2 - 1)
    delay += jitter

    return time.Duration(delay)
}
```

### **Retry Decision Matrix**

| Error Type | Retry? | Max Attempts | Backoff |
|------------|--------|--------------|---------|
| Network timeout | ✅ Yes | 5 | Exponential |
| 429 Rate Limited | ✅ Yes | 5 | Exponential |
| 500 Internal Server Error | ✅ Yes | 5 | Exponential |
| 502 Bad Gateway | ✅ Yes | 5 | Exponential |
| 503 Service Unavailable | ✅ Yes | 5 | Exponential |
| 504 Gateway Timeout | ✅ Yes | 5 | Exponential |
| 400 Bad Request | ❌ No | 0 | N/A |
| 401 Unauthorized | ❌ No | 0 | N/A |
| 403 Forbidden | ❌ No | 0 | N/A |
| 404 Not Found | ❌ No | 0 | N/A |
| Context Cancelled | ❌ No | 0 | N/A |
| Validation Error | ❌ No | 0 | N/A |

---

## 🔐 **Circuit Breaker Pattern**

### **Circuit Breaker States**

```
┌─────────────────────────────────────────────────────────────┐
│                    CIRCUIT BREAKER                           │
├─────────────┬─────────────┬─────────────────────────────────┤
│   CLOSED    │  HALF-OPEN  │            OPEN                 │
│  (normal)   │  (testing)  │         (failing)               │
├─────────────┼─────────────┼─────────────────────────────────┤
│ All calls   │ 1 test call │ All calls fail immediately      │
│ go through  │ allowed     │ (return cached/default)         │
├─────────────┼─────────────┼─────────────────────────────────┤
│ 5 failures  │ Success →   │ After 60s → HALF-OPEN           │
│ → OPEN      │ CLOSED      │                                 │
│             │ Failure →   │                                 │
│             │ OPEN        │                                 │
└─────────────┴─────────────┴─────────────────────────────────┘
```

### **AIAnalysis Circuit Breaker Configuration**

```go
package circuitbreaker

import (
    "sync"
    "time"
)

// State represents circuit breaker state
type State int

const (
    StateClosed State = iota
    StateHalfOpen
    StateOpen
)

// Breaker implements circuit breaker pattern for HolmesGPT-API
type Breaker struct {
    mu            sync.RWMutex
    state         State
    failures      int
    lastFailure   time.Time

    // Configuration
    failureThreshold int           // 5 failures to open
    resetTimeout     time.Duration // 60s before half-open
    halfOpenMax      int           // 1 test call in half-open
}

// NewBreaker creates circuit breaker for HolmesGPT-API
func NewBreaker() *Breaker {
    return &Breaker{
        state:            StateClosed,
        failureThreshold: 5,
        resetTimeout:     60 * time.Second,
        halfOpenMax:      1,
    }
}

// Allow checks if request should be allowed
func (b *Breaker) Allow() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    switch b.state {
    case StateClosed:
        return true
    case StateHalfOpen:
        return true // Allow test request
    case StateOpen:
        // Check if reset timeout elapsed
        if time.Since(b.lastFailure) > b.resetTimeout {
            return true // Will transition to half-open
        }
        return false
    }
    return false
}

// RecordSuccess records successful call
func (b *Breaker) RecordSuccess() {
    b.mu.Lock()
    defer b.mu.Unlock()

    if b.state == StateHalfOpen {
        b.state = StateClosed
        b.failures = 0
    }
}

// RecordFailure records failed call
func (b *Breaker) RecordFailure() {
    b.mu.Lock()
    defer b.mu.Unlock()

    b.failures++
    b.lastFailure = time.Now()

    if b.state == StateHalfOpen || b.failures >= b.failureThreshold {
        b.state = StateOpen
    }
}
```

---

## 📝 **Error Wrapping & Context**

### **Standard Error Wrapping Pattern**

```go
package errors

import (
    "fmt"
)

// AIAnalysisError wraps errors with context
type AIAnalysisError struct {
    Op       string // Operation: "investigate", "evaluate_policy", "update_status"
    Phase    string // Phase: "validating", "investigating", "analyzing", "recommending"
    Resource string // Resource name
    Err      error  // Underlying error
}

func (e *AIAnalysisError) Error() string {
    return fmt.Sprintf("aianalysis: %s failed in phase %s for %s: %v",
        e.Op, e.Phase, e.Resource, e.Err)
}

func (e *AIAnalysisError) Unwrap() error {
    return e.Err
}

// Wrap creates AIAnalysisError with context
func Wrap(op, phase, resource string, err error) error {
    if err == nil {
        return nil
    }
    return &AIAnalysisError{
        Op:       op,
        Phase:    phase,
        Resource: resource,
        Err:      err,
    }
}

// Usage example
func (r *AIAnalysisReconciler) investigate(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    result, err := r.holmesGPT.Investigate(ctx, buildRequest(analysis))
    if err != nil {
        return errors.Wrap("investigate", "investigating", analysis.Name, err)
    }
    // ...
}
```

---

## 📊 **Logging Best Practices**

### **Structured Logging Pattern (logr)**

```go
// ✅ CORRECT: Structured logging with context
r.Log.Info("Starting investigation",
    "name", analysis.Name,
    "namespace", analysis.Namespace,
    "phase", analysis.Status.Phase,
    "fingerprint", analysis.Spec.AnalysisRequest.SignalContext.Fingerprint)

r.Log.Error(err, "HolmesGPT-API call failed",
    "name", analysis.Name,
    "namespace", analysis.Namespace,
    "endpoint", "/api/v1/incident/analyze",
    "attempt", attempt,
    "maxRetries", maxRetries)

// ❌ WRONG: Unstructured logging
log.Printf("Starting investigation for %s", analysis.Name)
fmt.Printf("Error: %v\n", err)
```

### **Log Levels**

| Level | When to Use | AIAnalysis Examples |
|-------|-------------|---------------------|
| **Error** | Permanent failures requiring attention | Auth failure, Rego syntax error |
| **Info** | Normal operations, state changes | Phase transitions, reconciliation start/end |
| **Debug** | Detailed debugging (disabled in prod) | Request/response details, retry attempts |

---

## 🚨 **Graceful Degradation Matrix**

| Dependency | Failure Mode | Degraded Behavior | User Impact |
|------------|--------------|-------------------|-------------|
| **HolmesGPT-API** | Timeout/5xx | Retry with backoff, then fail | Analysis delayed |
| **HolmesGPT-API** | Auth failure | Fail immediately with event | Analysis blocked |
| **Rego Policy** | Parse error | Default to manual approval | Approval requires human |
| **Rego Policy** | Timeout | Default to manual approval | Approval requires human |
| **Data Storage** | Unavailable | Skip audit, continue analysis | Audit gap (logged) |
| **K8s API** | Unavailable | Controller restarts | All operations blocked |

---

## 📋 **Error Handling Checklist**

### **Implementation Checklist**
- [ ] All errors are classified (A-E categories)
- [ ] Transient errors use exponential backoff
- [ ] Permanent errors fail immediately with clear message
- [ ] Circuit breaker protects HolmesGPT-API calls
- [ ] All errors wrapped with context (op, phase, resource)
- [ ] Structured logging with logr
- [ ] Error metrics exported (`aianalysis_errors_total{category="..."}`)
- [ ] Kubernetes events created for user-visible errors
- [ ] Graceful degradation for Rego policy failures

### **Testing Checklist**
- [ ] Unit tests for each error category
- [ ] Integration test for retry with backoff
- [ ] Integration test for circuit breaker
- [ ] Integration test for graceful degradation
- [ ] E2E test for error recovery

---

## 📚 **References**

- [DD-004: RFC 7807 Error Responses](../../../../architecture/decisions/DD-004-rfc7807-error-responses.md)
- [DD-005: Observability Standards](../../../../architecture/decisions/DD-005-observability-standards.md)
- [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md) - Parent implementation plan

