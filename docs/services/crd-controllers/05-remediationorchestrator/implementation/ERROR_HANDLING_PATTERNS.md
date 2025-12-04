# Error Handling Patterns - Remediation Orchestrator

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md)
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0 §Error Handling Philosophy
**Last Updated**: 2025-12-04

---

## 📑 Table of Contents

| Section | Purpose |
|---------|---------|
| [Core Principles](#core-principles) | Foundational error handling approach |
| [Error Categories](#error-categories) | Category A-F classification |
| [Retry Strategy](#retry-strategy) | Exponential backoff patterns |
| [Circuit Breaker](#circuit-breaker-pattern) | Failure isolation |
| [Error Wrapping](#error-wrapping--context) | Context preservation |
| [Logging Best Practices](#logging-best-practices) | Structured logging patterns |
| [Recovery Strategies](#recovery-strategies) | Error recovery approaches |

---

## Core Principles

### 1. Fail Fast, Recover Gracefully

```go
// ✅ GOOD: Validate early, handle gracefully
func (r *Reconciler) handleProcessing(ctx context.Context, rr *remediationv1.RemediationRequest, status *AggregatedStatus) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. Validate preconditions early
    if rr.Status.SignalProcessingRef == "" {
        log.Info("SignalProcessing not yet created, creating now")
        return r.createSignalProcessing(ctx, rr)
    }

    // 2. Check for terminal conditions
    if status.SignalProcessingPhase == "Failed" {
        return r.handleSignalProcessingFailure(ctx, rr, status)
    }

    // 3. Normal processing
    if !status.SignalProcessingReady {
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    return r.transitionToAnalyzing(ctx, rr, status)
}
```

### 2. Never Swallow Errors

```go
// ❌ BAD: Silent failure
func (r *Reconciler) badExample(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
    _, _ = r.ChildCreator.CreateSignalProcessing(ctx, rr) // Error ignored!
    return ctrl.Result{}, nil
}

// ✅ GOOD: Always handle or propagate errors
func (r *Reconciler) goodExample(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    name, err := r.ChildCreator.CreateSignalProcessing(ctx, rr)
    if err != nil {
        log.Error(err, "Failed to create SignalProcessing")
        r.Metrics.ReconciliationErrors.WithLabelValues(rr.Namespace, "child_creation_error").Inc()

        // Categorize and handle appropriately
        if isRetryable(err) {
            return ctrl.Result{RequeueAfter: calculateBackoff(rr.Status.AttemptCount)}, nil
        }
        return ctrl.Result{}, err
    }

    log.Info("Created SignalProcessing", "name", name)
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```

### 3. Idempotency is Essential

```go
// ✅ GOOD: Idempotent child CRD creation
func (c *SignalProcessingCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
    log := log.FromContext(ctx)
    name := fmt.Sprintf("sp-%s", rr.Name)

    // Check if already exists (idempotency)
    existing := &signalprocessingv1.SignalProcessing{}
    err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
    if err == nil {
        // Already exists - verify owner reference
        if metav1.IsControlledBy(existing, rr) {
            log.Info("SignalProcessing already exists and owned by this RR", "name", name)
            return name, nil
        }
        return "", fmt.Errorf("SignalProcessing %s exists but not owned by this RR", name)
    }
    if !apierrors.IsNotFound(err) {
        return "", fmt.Errorf("failed to check existing SignalProcessing: %w", err)
    }

    // Create new SignalProcessing
    // ... creation logic ...
}
```

---

## Error Categories

### Category A: Transient/Retryable Errors

**Characteristics**: Temporary failures that may succeed on retry

```go
// Category A: Network/API transient errors
func isTransientError(err error) bool {
    // Kubernetes API server errors
    if apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) {
        return true
    }

    // Connection errors
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }

    // Rate limiting
    if apierrors.IsServiceUnavailable(err) {
        return true
    }

    return false
}

// Handling: Exponential backoff retry
if isTransientError(err) {
    backoff := calculateBackoff(rr.Status.AttemptCount)
    rr.Status.AttemptCount++
    log.Info("Transient error, retrying", "backoff", backoff, "attempt", rr.Status.AttemptCount)
    return ctrl.Result{RequeueAfter: backoff}, nil
}
```

### Category B: Resource State Errors

**Characteristics**: Resource in unexpected state

```go
// Category B: Resource state validation
func validateResourceState(rr *remediationv1.RemediationRequest, status *AggregatedStatus) error {
    // Check for inconsistent state
    if rr.Status.OverallPhase == "Executing" && rr.Status.WorkflowExecutionRef == "" {
        return &ResourceStateError{
            Resource: "RemediationRequest",
            Expected: "WorkflowExecutionRef should be set",
            Actual:   "WorkflowExecutionRef is empty",
        }
    }

    // Check for child CRD failure
    if status.SignalProcessingPhase == "Failed" {
        return &ChildCRDFailedError{
            ChildType: "SignalProcessing",
            Phase:     status.SignalProcessingPhase,
            Reason:    status.Error.Error(),
        }
    }

    return nil
}

// Handling: Log, update status, potentially escalate
if err := validateResourceState(rr, status); err != nil {
    var stateErr *ResourceStateError
    if errors.As(err, &stateErr) {
        log.Error(err, "Resource state validation failed")
        r.EventRecorder.Event(rr, "Warning", "StateValidationFailed", err.Error())
        // Don't retry - needs investigation
        return ctrl.Result{}, nil
    }
}
```

### Category C: Business Logic Errors

**Characteristics**: Validation failures, policy violations

```go
// Category C: Business rule validation
func validateBusinessRules(rr *remediationv1.RemediationRequest) error {
    // Validate required fields
    if rr.Spec.SignalFingerprint == "" {
        return &BusinessRuleError{
            Rule:   "SignalFingerprint required",
            Field:  "spec.signalFingerprint",
            Reason: "Cannot process remediation without signal fingerprint",
        }
    }

    // Validate target resource for execution
    if rr.Spec.TargetResource == nil {
        return &BusinessRuleError{
            Rule:   "TargetResource required",
            Field:  "spec.targetResource",
            Reason: "Cannot execute workflow without target resource",
        }
    }

    return nil
}

// Handling: Fail permanently, notify user
if err := validateBusinessRules(rr); err != nil {
    var bizErr *BusinessRuleError
    if errors.As(err, &bizErr) {
        log.Error(err, "Business rule validation failed")
        rr.Status.OverallPhase = "Failed"
        rr.Status.FailureReason = bizErr.Error()
        r.Status().Update(ctx, rr)
        r.EventRecorder.Event(rr, "Warning", "ValidationFailed", bizErr.Error())
        return ctrl.Result{}, nil // Don't retry
    }
}
```

### Category D: External Service Errors

**Characteristics**: External dependency failures

```go
// Category D: External service communication errors
type ExternalServiceError struct {
    Service    string
    Operation  string
    StatusCode int
    Message    string
}

func (e *ExternalServiceError) Error() string {
    return fmt.Sprintf("external service error: %s %s returned %d: %s",
        e.Service, e.Operation, e.StatusCode, e.Message)
}

// Handling: Circuit breaker + retry with backoff
func (r *Reconciler) callExternalService(ctx context.Context, rr *remediationv1.RemediationRequest) error {
    // Check circuit breaker
    if r.circuitBreaker.IsOpen("notification-service") {
        return &CircuitOpenError{Service: "notification-service"}
    }

    err := r.notificationClient.Send(ctx, notification)
    if err != nil {
        r.circuitBreaker.RecordFailure("notification-service")
        return &ExternalServiceError{
            Service:   "notification-service",
            Operation: "Send",
            Message:   err.Error(),
        }
    }

    r.circuitBreaker.RecordSuccess("notification-service")
    return nil
}
```

### Category E: Conflict Errors (Optimistic Locking)

**Characteristics**: Concurrent modification conflicts

```go
// Category E: Status update conflicts
func (r *Reconciler) updateStatusWithRetry(ctx context.Context, rr *remediationv1.RemediationRequest, maxRetries int) error {
    log := log.FromContext(ctx)

    for attempt := 0; attempt < maxRetries; attempt++ {
        err := r.Status().Update(ctx, rr)
        if err == nil {
            return nil
        }

        if apierrors.IsConflict(err) {
            log.Info("Status update conflict, refetching", "attempt", attempt+1)
            r.Metrics.StatusUpdateConflicts.WithLabelValues(rr.Namespace).Inc()

            // Refetch the latest version
            latest := &remediationv1.RemediationRequest{}
            if err := r.Get(ctx, client.ObjectKeyFromObject(rr), latest); err != nil {
                return fmt.Errorf("failed to refetch after conflict: %w", err)
            }

            // Reapply status changes to latest version
            latest.Status = rr.Status
            rr = latest

            continue
        }

        return err
    }

    return fmt.Errorf("max retries (%d) exceeded for status update", maxRetries)
}

// Usage
if err := r.updateStatusWithRetry(ctx, rr, 3); err != nil {
    log.Error(err, "Failed to update status after retries")
    return ctrl.Result{RequeueAfter: time.Second}, nil
}
```

### Category F: Fatal/Unrecoverable Errors

**Characteristics**: Errors that cannot be recovered through retry

```go
// Category F: Fatal errors requiring manual intervention
func isFatalError(err error) bool {
    // Schema validation failures
    var validationErr *ValidationError
    if errors.As(err, &validationErr) {
        return true
    }

    // Permission denied (RBAC)
    if apierrors.IsForbidden(err) {
        return true
    }

    // Resource not found (parent deleted)
    if apierrors.IsNotFound(err) {
        return true
    }

    return false
}

// Handling: Fail permanently, escalate for manual review
if isFatalError(err) {
    log.Error(err, "Fatal error, failing remediation")
    rr.Status.OverallPhase = "Failed"
    rr.Status.FailureReason = fmt.Sprintf("Fatal error: %v", err)
    r.Status().Update(ctx, rr)

    // Escalate
    r.EscalationMgr.Escalate(ctx, rr, fmt.Sprintf("Fatal error requiring manual intervention: %v", err))

    return ctrl.Result{}, nil // Don't retry
}
```

---

## Retry Strategy

### Exponential Backoff with Jitter

```go
package retry

import (
    "math"
    "math/rand"
    "time"
)

// Config holds retry configuration
type Config struct {
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    MaxAttempts int
    JitterRatio float64 // 0.0-1.0
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
    return Config{
        BaseDelay:   30 * time.Second,
        MaxDelay:    8 * time.Minute,
        MaxAttempts: 10,
        JitterRatio: 0.1, // ±10% jitter
    }
}

// CalculateBackoff returns exponential backoff with jitter
// Formula: min(maxDelay, baseDelay * 2^attempt) * (1 ± jitter)
func CalculateBackoff(attempt int, config Config) time.Duration {
    if attempt < 0 {
        attempt = 0
    }

    // Calculate base exponential delay
    delay := time.Duration(float64(config.BaseDelay) * math.Pow(2, float64(attempt)))

    // Cap at maximum
    if delay > config.MaxDelay {
        delay = config.MaxDelay
    }

    // Add jitter
    if config.JitterRatio > 0 {
        jitterRange := float64(delay) * config.JitterRatio
        jitter := (rand.Float64()*2 - 1) * jitterRange // -jitter to +jitter
        delay = time.Duration(float64(delay) + jitter)
    }

    return delay
}

// ShouldRetry determines if retry should be attempted
func ShouldRetry(attempt int, err error, config Config) bool {
    if attempt >= config.MaxAttempts {
        return false
    }

    // Only retry transient errors
    return isTransientError(err)
}
```

### Retry Decision Matrix

| Error Type | Retry? | Backoff | Max Attempts | Notes |
|------------|--------|---------|--------------|-------|
| Category A (Transient) | ✅ Yes | Exponential | 10 | Network, rate limiting |
| Category B (State) | ⚠️ Once | Fixed 30s | 1 | Then fail |
| Category C (Business) | ❌ No | N/A | 0 | Fail immediately |
| Category D (External) | ✅ Yes | Exponential | 5 | With circuit breaker |
| Category E (Conflict) | ✅ Yes | Immediate | 3 | Refetch and retry |
| Category F (Fatal) | ❌ No | N/A | 0 | Fail + escalate |

---

## Circuit Breaker Pattern

```go
package circuitbreaker

import (
    "sync"
    "time"
)

// State represents circuit breaker state
type State int

const (
    StateClosed State = iota   // Normal operation
    StateOpen                   // Failing, reject requests
    StateHalfOpen              // Testing if recovered
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
    mu              sync.RWMutex
    state           State
    failures        int
    successes       int
    lastFailure     time.Time

    // Configuration
    failureThreshold int           // Failures before opening
    successThreshold int           // Successes to close from half-open
    timeout          time.Duration // Time before half-open
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

// IsOpen returns true if circuit is open (should reject requests)
func (cb *CircuitBreaker) IsOpen() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()

    if cb.state == StateOpen {
        // Check if timeout has passed
        if time.Since(cb.lastFailure) > cb.timeout {
            return false // Will transition to half-open
        }
        return true
    }
    return false
}

// RecordSuccess records a successful call
func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.failures = 0

    if cb.state == StateHalfOpen {
        cb.successes++
        if cb.successes >= cb.successThreshold {
            cb.state = StateClosed
            cb.successes = 0
        }
    }
}

// RecordFailure records a failed call
func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.failures++
    cb.lastFailure = time.Now()
    cb.successes = 0

    if cb.failures >= cb.failureThreshold {
        cb.state = StateOpen
    }
}
```

---

## Error Wrapping & Context

```go
package errors

import (
    "fmt"
)

// OrchestratorError is the base error type for orchestrator errors
type OrchestratorError struct {
    Op      string // Operation that failed
    Kind    string // Error category (transient, business, fatal)
    Err     error  // Underlying error
    Context map[string]interface{} // Additional context
}

func (e *OrchestratorError) Error() string {
    return fmt.Sprintf("%s: %s: %v", e.Kind, e.Op, e.Err)
}

func (e *OrchestratorError) Unwrap() error {
    return e.Err
}

// Wrap wraps an error with operation context
func Wrap(err error, op string, kind string) *OrchestratorError {
    if err == nil {
        return nil
    }
    return &OrchestratorError{
        Op:      op,
        Kind:    kind,
        Err:     err,
        Context: make(map[string]interface{}),
    }
}

// WithContext adds context to an error
func (e *OrchestratorError) WithContext(key string, value interface{}) *OrchestratorError {
    if e.Context == nil {
        e.Context = make(map[string]interface{})
    }
    e.Context[key] = value
    return e
}

// Usage example
func (r *Reconciler) handleProcessing(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
    name, err := r.ChildCreator.CreateSignalProcessing(ctx, rr)
    if err != nil {
        return ctrl.Result{}, Wrap(err, "CreateSignalProcessing", "transient").
            WithContext("remediation", rr.Name).
            WithContext("namespace", rr.Namespace)
    }
    // ...
}
```

---

## Logging Best Practices

### Structured Logging with logr

```go
// ✅ GOOD: Structured logging with context
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx).WithValues(
        "remediation", req.Name,
        "namespace", req.Namespace,
    )

    log.Info("Starting reconciliation")

    rr := &remediationv1.RemediationRequest{}
    if err := r.Get(ctx, req.NamespacedName, rr); err != nil {
        log.Error(err, "Failed to get RemediationRequest")
        return ctrl.Result{}, err
    }

    log = log.WithValues(
        "phase", rr.Status.OverallPhase,
        "generation", rr.Generation,
    )

    log.Info("Processing remediation", "signalFingerprint", rr.Spec.SignalFingerprint)

    // ...
}

// ❌ BAD: Unstructured logging
func (r *Reconciler) badLogging(ctx context.Context, rr *remediationv1.RemediationRequest) {
    log := log.FromContext(ctx)
    log.Info(fmt.Sprintf("Processing remediation %s in phase %s", rr.Name, rr.Status.OverallPhase))
    // Hard to parse, filter, and query
}
```

### Log Levels

| Level | When to Use | Example |
|-------|-------------|---------|
| `Error` | Unexpected failures requiring attention | `log.Error(err, "Failed to create child CRD")` |
| `Info` | Normal operations, state changes | `log.Info("Phase transition", "from", "Processing", "to", "Analyzing")` |
| `V(1)` | Detailed operations | `log.V(1).Info("Checking child CRD status")` |
| `V(2)` | Debug information | `log.V(2).Info("Aggregated status", "status", status)` |

---

## Recovery Strategies

### 1. Automatic Recovery (Transient Errors)

```go
func (r *Reconciler) handleTransientError(ctx context.Context, rr *remediationv1.RemediationRequest, err error) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Increment attempt counter
    rr.Status.AttemptCount++

    // Calculate backoff
    backoff := retry.CalculateBackoff(rr.Status.AttemptCount, retry.DefaultConfig())

    // Update status
    rr.Status.LastError = err.Error()
    rr.Status.LastErrorTime = metav1.Now()
    r.Status().Update(ctx, rr)

    log.Info("Transient error, scheduling retry",
        "attempt", rr.Status.AttemptCount,
        "backoff", backoff,
        "error", err.Error(),
    )

    return ctrl.Result{RequeueAfter: backoff}, nil
}
```

### 2. Manual Recovery (Fatal Errors)

```go
func (r *Reconciler) handleFatalError(ctx context.Context, rr *remediationv1.RemediationRequest, err error) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Mark as failed
    rr.Status.OverallPhase = "Failed"
    rr.Status.FailureReason = err.Error()
    rr.Status.FailureTime = &metav1.Now()

    // Update status
    if updateErr := r.Status().Update(ctx, rr); updateErr != nil {
        log.Error(updateErr, "Failed to update failure status")
    }

    // Create escalation notification
    if escalateErr := r.EscalationMgr.Escalate(ctx, rr, err.Error()); escalateErr != nil {
        log.Error(escalateErr, "Failed to create escalation")
    }

    // Emit event
    r.EventRecorder.Event(rr, "Warning", "RemediationFailed", err.Error())

    // Update metrics
    r.Metrics.RemediationsFailed.WithLabelValues(rr.Namespace).Inc()

    log.Error(err, "Fatal error, remediation failed")

    // Don't requeue - needs manual intervention
    return ctrl.Result{}, nil
}
```

---

## Implementation Checklist

- [ ] Error category classification implemented
- [ ] Retry strategy with exponential backoff
- [ ] Circuit breaker for external services
- [ ] Error wrapping with context
- [ ] Structured logging throughout
- [ ] Status update with conflict handling
- [ ] Escalation for fatal errors
- [ ] Metrics for error tracking

---

## References

- [WORKFLOWEXECUTION_PATTERN_ENHANCEMENTS.md](./WORKFLOWEXECUTION_PATTERN_ENHANCEMENTS.md) - Error handling patterns from WE v1.2
- [DD-005: Observability Standards](../../../../architecture/decisions/DD-005-observability-standards.md)
- SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0 §Error Handling Philosophy

