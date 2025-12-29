# AI Analysis Service - Error Handling Philosophy

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Version**: 1.0
**Status**: Template (to be completed Day 5 EOD)

---

## 🎯 **Core Principles**

### 1. **Error Classification**

All errors fall into three categories:

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**: Network timeouts, 503 from HolmesGPT-API, K8s API connection errors
- **Strategy**: Exponential backoff with jitter
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**: 401 Unauthorized, 404 Not Found, validation failures, Rego syntax errors
- **Strategy**: Fail immediately, log error, update status
- **Max Retries**: 0 (no retry)

#### **User Errors** (Input Validation)
- **Definition**: Invalid user input or configuration in `AIAnalysis.Spec`
- **Examples**: Missing required fields, invalid formats, unknown FailedDetections fields
- **Strategy**: Return validation error immediately, do not retry
- **Max Retries**: 0 (no retry)

---

## 🏷️ **Service-Specific Error Categories**

### **Category A: AIAnalysis CRD Not Found**
- **When**: The `AIAnalysis` CRD is deleted during reconciliation
- **Action**: Log deletion, remove from retry queue
- **Recovery**: Normal (no action needed)
- **Example**: User deletes AIAnalysis before investigation completes

```go
if apierrors.IsNotFound(err) {
    log.Info("AIAnalysis resource not found, likely deleted")
    return ctrl.Result{}, nil
}
```

### **Category B: HolmesGPT-API Errors** (Retry with Backoff)
- **When**: HolmesGPT-API timeout, rate limiting, 5xx errors
- **Action**: Exponential backoff (30s → 60s → 120s → 240s → 480s)
- **Recovery**: Automatic retry up to 5 attempts, then mark as failed
- **Example**: HolmesGPT-API service temporarily unavailable

```go
if isRetriableError(err) {
    delay := calculateBackoff(attempt)
    log.Info("Retrying HolmesGPT-API call", "delay", delay, "attempt", attempt)
    return ctrl.Result{RequeueAfter: delay}, nil
}
```

### **Category C: Authentication/Authorization Errors** (Permanent Error)
- **When**: 401/403 auth errors from HolmesGPT-API, invalid K8s RBAC
- **Action**: Mark as failed immediately, create event
- **Recovery**: Manual (fix configuration/RBAC)
- **Example**: Invalid API key for HolmesGPT-API

```go
if isAuthError(err) {
    analysis.Status.Phase = aianalysisv1.PhaseFailed
    analysis.Status.Message = "Authentication failed: " + err.Error()
    recorder.Event(analysis, "Warning", "AuthError", err.Error())
    return ctrl.Result{}, nil
}
```

### **Category D: Status/State Update Conflicts**
- **When**: Multiple processes updating same AIAnalysis CRD simultaneously
- **Action**: Retry with optimistic locking
- **Recovery**: Automatic (retry status update)
- **Example**: Status update conflict during phase transition

```go
if apierrors.IsConflict(err) {
    log.Info("Status update conflict, requeuing")
    return ctrl.Result{Requeue: true}, nil
}
```

### **Category E: Rego Policy Evaluation Failures** (Graceful Degradation)
- **When**: Rego policy syntax error, policy timeout, unexpected input
- **Action**: Log error, apply graceful degradation (default to manual approval)
- **Recovery**: Automatic (degraded operation), manual (fix Rego policy)
- **Example**: `approval.rego` policy fails to evaluate

```go
decision, err := regoEngine.Evaluate(ctx, input)
if err != nil {
    log.Error(err, "Policy evaluation failed, defaulting to manual approval")
    decision = &PolicyDecision{
        Outcome: "manual_approval",
        Reason:  "Policy evaluation error (graceful degradation)",
    }
}
```

---

## 🔄 **Retry Strategy**

### **Exponential Backoff Implementation**

```go
package retry

import (
    "math/rand"
    "time"
)

// BackoffConfig holds retry configuration
type BackoffConfig struct {
    BaseDelay  time.Duration
    MaxDelay   time.Duration
    MaxRetries int
    Jitter     float64 // 0.0-1.0
}

// DefaultBackoffConfig returns default configuration
func DefaultBackoffConfig() BackoffConfig {
    return BackoffConfig{
        BaseDelay:  30 * time.Second,
        MaxDelay:   8 * time.Minute,
        MaxRetries: 5,
        Jitter:     0.1,
    }
}

// CalculateDelay calculates the delay for a given attempt
func (c BackoffConfig) CalculateDelay(attempt int) time.Duration {
    if attempt >= c.MaxRetries {
        return 0 // No more retries
    }

    delay := c.BaseDelay * time.Duration(1<<attempt)
    if delay > c.MaxDelay {
        delay = c.MaxDelay
    }

    // Add jitter
    jitter := time.Duration(float64(delay) * c.Jitter * (rand.Float64()*2 - 1))
    return delay + jitter
}
```

### **Retry Decision Matrix**

| Error Type | Retry? | Max Attempts | Backoff |
|------------|--------|--------------|---------|
| Network timeout | ✅ Yes | 5 | Exponential |
| 5xx Server Error | ✅ Yes | 5 | Exponential |
| 429 Rate Limit | ✅ Yes | 5 | Exponential + Retry-After |
| 4xx Client Error | ❌ No | 0 | — |
| Validation Error | ❌ No | 0 | — |
| K8s Conflict | ✅ Yes | 3 | Fixed (1s) |
| Rego Syntax Error | ❌ No | 0 | — |

---

## 🔐 **Circuit Breaker Pattern**

### **Circuit Breaker for HolmesGPT-API**

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
    StateOpen
    StateHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
    mu               sync.RWMutex
    state            State
    failures         int
    successes        int
    lastFailure      time.Time
    failureThreshold int
    successThreshold int
    timeout          time.Duration
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        state:            StateClosed,
        failureThreshold: failureThreshold,
        successThreshold: successThreshold,
        timeout:          timeout,
    }
}

// Allow checks if the request should be allowed
func (cb *CircuitBreaker) Allow() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()

    switch cb.state {
    case StateClosed:
        return true
    case StateOpen:
        if time.Since(cb.lastFailure) > cb.timeout {
            // Transition to half-open
            return true
        }
        return false
    case StateHalfOpen:
        return true
    }
    return false
}

// RecordSuccess records a successful call
func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.successes++
    if cb.state == StateHalfOpen && cb.successes >= cb.successThreshold {
        cb.state = StateClosed
        cb.failures = 0
        cb.successes = 0
    }
}

// RecordFailure records a failed call
func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.failures++
    cb.lastFailure = time.Now()

    if cb.failures >= cb.failureThreshold {
        cb.state = StateOpen
    }
}
```

### **Circuit Breaker States**

| State | Behavior | Transition Condition |
|-------|----------|---------------------|
| **Closed** | All requests allowed | 5 consecutive failures → Open |
| **Open** | All requests rejected | 60s timeout → Half-Open |
| **Half-Open** | Limited requests allowed | 2 successes → Closed |

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
    Phase   string
    Op      string
    Err     error
    Retry   bool
}

func (e *AIAnalysisError) Error() string {
    return fmt.Sprintf("phase=%s op=%s: %v", e.Phase, e.Op, e.Err)
}

func (e *AIAnalysisError) Unwrap() error {
    return e.Err
}

// NewValidationError creates a validation error
func NewValidationError(field, message string) error {
    return &AIAnalysisError{
        Phase: "validating",
        Op:    "validate",
        Err:   fmt.Errorf("field %s: %s", field, message),
        Retry: false,
    }
}

// NewHolmesGPTError creates a HolmesGPT-API error
func NewHolmesGPTError(op string, err error, retry bool) error {
    return &AIAnalysisError{
        Phase: "investigating",
        Op:    op,
        Err:   err,
        Retry: retry,
    }
}

// NewRegoError creates a Rego policy error
func NewRegoError(op string, err error) error {
    return &AIAnalysisError{
        Phase: "analyzing",
        Op:    op,
        Err:   err,
        Retry: false,
    }
}
```

---

## 📊 **Logging Best Practices**

### **Structured Logging Pattern**

```go
// ✅ GOOD: Structured logging with context
log.Error(err, "HolmesGPT-API call failed",
    "phase", "investigating",
    "fingerprint", analysis.Spec.AnalysisRequest.SignalContext.Fingerprint,
    "attempt", attempt,
    "nextRetry", delay,
)

// ❌ BAD: Unstructured logging
log.Error(err, fmt.Sprintf("HolmesGPT call failed for %s, retry in %v", name, delay))
```

### **Log Levels**

| Level | When to Use | Example |
|-------|-------------|---------|
| **Error** | Unrecoverable failures | HolmesGPT-API permanent failure |
| **Info** | Normal operations | Phase transition, reconciliation start |
| **V(1)** | Detailed operations | API request/response details |
| **V(2)** | Debug information | Input/output data structures |

---

## 🚨 **Error Recovery Strategies**

### **Graceful Degradation**

| Component | Degradation Strategy | Impact |
|-----------|---------------------|--------|
| HolmesGPT-API unavailable | Requeue with backoff | Delayed analysis |
| Rego policy failure | Default to manual approval | Increased manual reviews |
| Data Storage unavailable | Skip audit logging | Missing audit trail |
| K8s API conflict | Retry with fresh resource | Brief delay |

### **Recovery Actions by Phase**

| Phase | Error | Recovery |
|-------|-------|----------|
| Validating | Missing field | Fail immediately |
| Validating | Invalid FailedDetections | Fail immediately |
| Investigating | HolmesGPT timeout | Retry with backoff |
| Investigating | HolmesGPT 401 | Fail, alert ops |
| Analyzing | Rego syntax error | Default to manual |
| Analyzing | Rego timeout | Default to manual |
| Recommending | Status conflict | Retry |

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [DD-004: RFC 7807 Error Responses](../../../../architecture/decisions/DD-004-rfc7807-error-responses.md) | Error format |
| [DD-005: Observability Standards](../../../../architecture/decisions/DD-005-observability-standards.md) | Logging patterns |
| [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) | Error testing |

