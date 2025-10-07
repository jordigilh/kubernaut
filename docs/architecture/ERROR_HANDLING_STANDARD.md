# Cross-Service Error Handling Standard

**Document Type**: Architecture Standard
**Status**: ✅ **APPROVED**
**Version**: 1.0
**Last Updated**: October 6, 2025
**Scope**: All Kubernaut V1 services

---

## 🎯 Purpose

This document defines **mandatory** error handling patterns for all Kubernaut services to ensure:
1. **Consistent error semantics** across HTTP and CRD-based services
2. **Clear error propagation** through the remediation workflow
3. **Actionable error information** for operators and automation
4. **Observability** through structured logging and metrics
5. **Resilience** through retry, timeout, and circuit breaker patterns

---

## 📊 Error Handling Principles

### 1. Fail Fast, Fail Clearly
- Detect errors as early as possible
- Provide actionable error messages with context
- Include error codes for programmatic handling

### 2. Graceful Degradation
- Use circuit breakers to prevent cascade failures
- Implement retries with exponential backoff
- Fall back to safe defaults when possible

### 3. Observability First
- Log all errors with structured context
- Emit error metrics for monitoring
- Include trace IDs for distributed debugging

### 4. Idempotency
- All operations should be safe to retry
- Use request IDs to detect duplicates
- Store operation state to enable recovery

---

## 🔧 HTTP Service Error Standards

### HTTP Status Code Mapping

| Status | Meaning | When to Use | Retry? |
|--------|---------|-------------|--------|
| **400** | Bad Request | Invalid request format, missing required fields | No |
| **401** | Unauthorized | Missing or invalid authentication | No |
| **403** | Forbidden | Valid auth, insufficient permissions | No |
| **404** | Not Found | Resource doesn't exist | No |
| **409** | Conflict | Resource already exists, state conflict | No |
| **422** | Unprocessable Entity | Validation failed (semantic errors) | No |
| **429** | Too Many Requests | Rate limit exceeded | Yes (with backoff) |
| **500** | Internal Server Error | Unexpected error | Maybe |
| **502** | Bad Gateway | Upstream service returned error | Yes |
| **503** | Service Unavailable | Service temporarily down | Yes |
| **504** | Gateway Timeout | Upstream service timeout | Yes |

### Standard HTTP Error Response

```go
// pkg/shared/errors/http.go
package errors

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// HTTPError provides structured HTTP error response
type HTTPError struct {
    Code       string        `json:"code"`                 // Machine-readable error code
    Message    string        `json:"message"`              // Human-readable message
    Details    *ErrorDetails `json:"details,omitempty"`    // Structured error context
    Timestamp  time.Time     `json:"timestamp"`            // When error occurred
    RequestID  string        `json:"requestId"`            // Request trace ID
    RetryAfter *int          `json:"retryAfter,omitempty"` // Seconds to wait before retry
}

// ErrorDetails provides structured context for HTTP errors
// Use specific fields instead of map[string]interface{} for type safety
type ErrorDetails struct {
    // Validation errors (for 422 responses)
    ValidationErrors []ValidationError `json:"validationErrors,omitempty"`

    // Field-level errors (for 400 responses)
    FieldErrors map[string]string `json:"fieldErrors,omitempty"`

    // Upstream error context (for 502, 504 responses)
    UpstreamService string `json:"upstreamService,omitempty"`
    UpstreamError   string `json:"upstreamError,omitempty"`
    UpstreamCode    string `json:"upstreamCode,omitempty"`

    // Resource context (for 404, 409 responses)
    ResourceType string `json:"resourceType,omitempty"`
    ResourceID   string `json:"resourceId,omitempty"`
    ResourceName string `json:"resourceName,omitempty"`

    // Operation context (general)
    Operation    string `json:"operation,omitempty"`
    AttemptCount int    `json:"attemptCount,omitempty"`

    // Rate limiting context (for 429 responses)
    RateLimit struct {
        Limit     int       `json:"limit,omitempty"`     // Requests per window
        Remaining int       `json:"remaining,omitempty"` // Requests remaining
        Reset     time.Time `json:"reset,omitempty"`     // When limit resets
    } `json:"rateLimit,omitempty"`
}

// ValidationError represents a single validation failure
type ValidationError struct {
    Field   string `json:"field"`             // Field name that failed validation
    Value   string `json:"value,omitempty"`   // Value that was provided (sanitized)
    Message string `json:"message"`           // Why validation failed
    Code    string `json:"code,omitempty"`    // Machine-readable validation code
}

// RespondWithError writes standard JSON error response
func RespondWithError(w http.ResponseWriter, statusCode int, err HTTPError) {
    w.Header().Set("Content-Type", "application/json")

    // Add Retry-After header if applicable
    if err.RetryAfter != nil {
        w.Header().Set("Retry-After", fmt.Sprintf("%d", *err.RetryAfter))
    }

    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(err)
}

// Helper function for pointer to int
func ptr(i int) *int {
    return &i
}
```

### Example: Gateway Service Error Handling

```go
// cmd/gateway/handlers.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/shared/errors"
    "github.com/jordigilh/kubernaut/pkg/shared/middleware"
)

func (h *WebhookHandler) HandlePrometheusWebhook(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    requestID := middleware.GetRequestID(ctx)
    logger := h.logger.WithValues("requestId", requestID)

    // Parse request
    var payload PrometheusAlert
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        httpErr := errors.HTTPError{
            Code:    "INVALID_REQUEST",
            Message: "Failed to parse Prometheus alert payload",
            Details: &errors.ErrorDetails{
                Operation: "parse_webhook_payload",
                FieldErrors: map[string]string{
                    "body": err.Error(),
                },
            },
            Timestamp: time.Now(),
            RequestID: requestID,
        }
        errors.RespondWithError(w, http.StatusBadRequest, httpErr)
        logger.Error(err, "Invalid request payload")
        return
    }

    // Validate payload
    if err := h.validator.Validate(payload); err != nil {
        // Convert validation errors to structured format
        validationErrs := h.validator.GetValidationErrors(err)

        httpErr := errors.HTTPError{
            Code:    "VALIDATION_FAILED",
            Message: "Alert payload validation failed",
            Details: &errors.ErrorDetails{
                ValidationErrors: validationErrs,
                Operation:        "validate_alert_payload",
            },
            Timestamp: time.Now(),
            RequestID: requestID,
        }
        errors.RespondWithError(w, http.StatusUnprocessableEntity, httpErr)
        logger.Error(err, "Validation failed", "payload", payload)
        return
    }

    // Check rate limit
    if err := h.rateLimiter.Allow(ctx, payload.Source); err != nil {
        retryAfter := 60 // seconds
        limit, remaining, reset := h.rateLimiter.GetLimitInfo(payload.Source)

        httpErr := errors.HTTPError{
            Code:       "RATE_LIMIT_EXCEEDED",
            Message:    fmt.Sprintf("Rate limit exceeded for source: %s", payload.Source),
            Details: &errors.ErrorDetails{
                Operation: "rate_limit_check",
                RateLimit: struct {
                    Limit     int       `json:"limit,omitempty"`
                    Remaining int       `json:"remaining,omitempty"`
                    Reset     time.Time `json:"reset,omitempty"`
                }{
                    Limit:     limit,
                    Remaining: remaining,
                    Reset:     reset,
                },
            },
            Timestamp:  time.Now(),
            RequestID:  requestID,
            RetryAfter: &retryAfter,
        }
        errors.RespondWithError(w, http.StatusTooManyRequests, httpErr)
        logger.Info("Rate limit exceeded", "source", payload.Source)
        return
    }

    // Create RemediationRequest CRD
    if err := h.createRemediationRequest(ctx, payload); err != nil {
        // Check if retryable
        var svcErr *errors.ServiceError
        if errors.As(err, &svcErr) && svcErr.Retryable {
            httpErr := errors.HTTPError{
                Code:    "SERVICE_UNAVAILABLE",
                Message: "Failed to create remediation request (retryable)",
                Details: &errors.ErrorDetails{
                    Operation:       "create_remediation_request",
                    UpstreamService: "kubernetes-api",
                    UpstreamError:   err.Error(),
                    AttemptCount:    1,
                },
                Timestamp:  time.Now(),
                RequestID:  requestID,
                RetryAfter: ptr(30), // seconds
            }
            errors.RespondWithError(w, http.StatusServiceUnavailable, httpErr)
        } else {
            httpErr := errors.HTTPError{
                Code:    "INTERNAL_ERROR",
                Message: "Failed to create remediation request",
                Details: &errors.ErrorDetails{
                    Operation:    "create_remediation_request",
                    ResourceType: "RemediationRequest",
                },
                Timestamp: time.Now(),
                RequestID: requestID,
            }
            errors.RespondWithError(w, http.StatusInternalServerError, httpErr)
        }
        logger.Error(err, "Failed to create RemediationRequest")
        return
    }

    // Success
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{
        "status":    "accepted",
        "requestId": requestID,
    })
}
```

---

## 🎛️ CRD Status Error Propagation

### Status Phase Standards

All CRD controllers follow this phase progression:

```go
// Standard phase values
const (
    PhasePending    = "Pending"      // Initial state
    PhaseProcessing = "Processing"   // Work in progress
    PhaseCompleted  = "Completed"    // Successfully finished
    PhaseFailed     = "Failed"       // Unrecoverable error
)
```

### Error Information Structure

```go
// pkg/apis/shared/v1/common_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrorInfo provides structured error information in CRD status
type ErrorInfo struct {
    Code      string      `json:"code"`                // Error code (e.g., "ENRICHMENT_FAILED")
    Message   string      `json:"message"`             // Human-readable error message
    Phase     string      `json:"phase,omitempty"`     // Which phase failed
    Service   string      `json:"service"`             // Service that reported error
    Timestamp metav1.Time `json:"timestamp"`           // When error occurred
    Retryable bool        `json:"retryable"`           // Can this error be retried?
    Cause     string      `json:"cause,omitempty"`     // Root cause (if different from message)
}

// Condition follows Kubernetes standard condition pattern
type Condition struct {
    Type               string      `json:"type"`
    Status             metav1.ConditionStatus `json:"status"` // True, False, Unknown
    LastTransitionTime metav1.Time `json:"lastTransitionTime"`
    Reason             string      `json:"reason"`
    Message            string      `json:"message"`
}
```

### Example: Child Controller Error Reporting

```go
// pkg/remediationprocessing/reconciler.go
package remediationprocessing

import (
    "context"
    "fmt"
    "time"

    processingv1 "github.com/jordigilh/kubernaut/apis/processing/v1"
    sharedv1 "github.com/jordigilh/kubernaut/apis/shared/v1"
    "github.com/jordigilh/kubernaut/pkg/shared/errors"

    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *RemediationProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Get RemediationProcessing CR
    processing := &processingv1.RemediationProcessing{}
    if err := r.Get(ctx, req.NamespacedName, processing); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Perform enrichment
    if err := r.enrichAlert(ctx, processing); err != nil {
        // Update status with error
        processing.Status.Phase = processingv1.PhaseFailed
        processing.Status.Error = &sharedv1.ErrorInfo{
            Code:      "ENRICHMENT_FAILED",
            Message:   err.Error(),
            Phase:     "enrichment",
            Service:   "remediation-processing",
            Timestamp: metav1.Now(),
            Retryable: errors.IsRetryable(err),
            Cause:     errors.GetRootCause(err),
        }

        // Add failure condition
        setCondition(processing, sharedv1.Condition{
            Type:               "Ready",
            Status:             metav1.ConditionFalse,
            LastTransitionTime: metav1.Now(),
            Reason:             "EnrichmentFailed",
            Message:            err.Error(),
        })

        // Update status
        if statusErr := r.Status().Update(ctx, processing); statusErr != nil {
            logger.Error(statusErr, "Failed to update status", "name", processing.Name)
            return ctrl.Result{}, statusErr
        }

        // Emit event
        r.recorder.Event(processing, corev1.EventTypeWarning, "EnrichmentFailed", err.Error())

        // Log error
        errors.LogError(logger, err, "Enrichment failed",
            "name", processing.Name,
            "namespace", processing.Namespace,
        )

        // Decide on requeue
        if errors.IsRetryable(err) {
            // Retry with exponential backoff
            return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
        }

        // Non-retryable - don't requeue
        return ctrl.Result{}, nil
    }

    // Success - update status
    processing.Status.Phase = processingv1.PhaseCompleted
    processing.Status.Error = nil

    setCondition(processing, sharedv1.Condition{
        Type:               "Ready",
        Status:             metav1.ConditionTrue,
        LastTransitionTime: metav1.Now(),
        Reason:             "EnrichmentSucceeded",
        Message:            "Alert enrichment completed successfully",
    })

    return ctrl.Result{}, r.Status().Update(ctx, processing)
}

// Helper function to set condition
func setCondition(processing *processingv1.RemediationProcessing, condition sharedv1.Condition) {
    found := false
    for i, c := range processing.Status.Conditions {
        if c.Type == condition.Type {
            processing.Status.Conditions[i] = condition
            found = true
            break
        }
    }
    if !found {
        processing.Status.Conditions = append(processing.Status.Conditions, condition)
    }
}
```

---

## 🔧 Structured Error Types - Complete Implementation

### ServiceError Type

The `ServiceError` type provides rich, structured error context for all services.

```go
// pkg/shared/errors/types.go
package errors

import (
    "errors"
    "fmt"
    "time"
)

// Base error sentinels for error classification
var (
    ErrNotFound       = errors.New("resource not found")
    ErrAlreadyExists  = errors.New("resource already exists")
    ErrValidation     = errors.New("validation failed")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrForbidden      = errors.New("forbidden")
    ErrTimeout        = errors.New("operation timeout")
    ErrUpstreamFailed = errors.New("upstream service failed")
    ErrRetryable      = errors.New("retryable error")
    ErrConflict       = errors.New("resource conflict")
    ErrRateLimited    = errors.New("rate limit exceeded")
)

// ServiceError provides rich context for errors across all services
type ServiceError struct {
    Code      string                 // Machine-readable error code
    Message   string                 // Human-readable message
    Service   string                 // Service that originated the error
    Timestamp time.Time              // When the error occurred
    Retryable bool                   // Whether this error can be retried
    Cause     error                  // Wrapped underlying error
    Context   map[string]interface{} // Additional context (TODO: make type-safe in future)
}

// Error implements the error interface
func (e *ServiceError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("[%s] %s: %s (caused by: %v)", e.Service, e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("[%s] %s: %s", e.Service, e.Code, e.Message)
}

// Unwrap implements the error unwrapping interface (Go 1.13+)
func (e *ServiceError) Unwrap() error {
    return e.Cause
}

// Is implements error matching for errors.Is (Go 1.13+)
func (e *ServiceError) Is(target error) bool {
    t, ok := target.(*ServiceError)
    if !ok {
        return errors.Is(e.Cause, target)
    }
    return e.Code == t.Code && e.Service == t.Service
}

// WithContext adds context information to the error
func (e *ServiceError) WithContext(key string, value interface{}) *ServiceError {
    if e.Context == nil {
        e.Context = make(map[string]interface{})
    }
    e.Context[key] = value
    return e
}
```

### Error Constructor Helpers

```go
// NewNotFoundError creates a "not found" error
func NewNotFoundError(service, resource, id string) *ServiceError {
    return &ServiceError{
        Code:      "NOT_FOUND",
        Message:   fmt.Sprintf("%s not found: %s", resource, id),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: false,
        Cause:     ErrNotFound,
        Context: map[string]interface{}{
            "resource": resource,
            "id":       id,
        },
    }
}

// NewAlreadyExistsError creates a "resource already exists" error
func NewAlreadyExistsError(service, resource, id string) *ServiceError {
    return &ServiceError{
        Code:      "ALREADY_EXISTS",
        Message:   fmt.Sprintf("%s already exists: %s", resource, id),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: false,
        Cause:     ErrAlreadyExists,
        Context: map[string]interface{}{
            "resource": resource,
            "id":       id,
        },
    }
}

// NewValidationError creates a validation failure error
func NewValidationError(service, message string, validationErrors []ValidationError) *ServiceError {
    return &ServiceError{
        Code:      "VALIDATION_FAILED",
        Message:   message,
        Service:   service,
        Timestamp: time.Now(),
        Retryable: false,
        Cause:     ErrValidation,
        Context: map[string]interface{}{
            "validationErrors": validationErrors,
        },
    }
}

// NewUpstreamError creates an upstream service failure error
func NewUpstreamError(service, upstream string, cause error) *ServiceError {
    return &ServiceError{
        Code:      "UPSTREAM_ERROR",
        Message:   fmt.Sprintf("upstream service %s failed", upstream),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: true,
        Cause:     cause,
        Context: map[string]interface{}{
            "upstream": upstream,
        },
    }
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(service, operation string, duration time.Duration) *ServiceError {
    return &ServiceError{
        Code:      "TIMEOUT",
        Message:   fmt.Sprintf("operation %s timed out after %s", operation, duration),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: true,
        Cause:     ErrTimeout,
        Context: map[string]interface{}{
            "operation": operation,
            "duration":  duration.String(),
        },
    }
}

// NewUnauthorizedError creates an authentication error
func NewUnauthorizedError(service, reason string) *ServiceError {
    return &ServiceError{
        Code:      "UNAUTHORIZED",
        Message:   reason,
        Service:   service,
        Timestamp: time.Now(),
        Retryable: false,
        Cause:     ErrUnauthorized,
    }
}

// NewForbiddenError creates an authorization error
func NewForbiddenError(service, resource, action string) *ServiceError {
    return &ServiceError{
        Code:      "FORBIDDEN",
        Message:   fmt.Sprintf("insufficient permissions to %s %s", action, resource),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: false,
        Cause:     ErrForbidden,
        Context: map[string]interface{}{
            "resource": resource,
            "action":   action,
        },
    }
}

// NewConflictError creates a resource conflict error
func NewConflictError(service, resource, reason string) *ServiceError {
    return &ServiceError{
        Code:      "CONFLICT",
        Message:   fmt.Sprintf("conflict on %s: %s", resource, reason),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: false,
        Cause:     ErrConflict,
        Context: map[string]interface{}{
            "resource": resource,
            "reason":   reason,
        },
    }
}

// NewRateLimitError creates a rate limit exceeded error
func NewRateLimitError(service string, limit int, resetTime time.Time) *ServiceError {
    return &ServiceError{
        Code:      "RATE_LIMIT_EXCEEDED",
        Message:   fmt.Sprintf("rate limit of %d requests exceeded", limit),
        Service:   service,
        Timestamp: time.Now(),
        Retryable: true,
        Cause:     ErrRateLimited,
        Context: map[string]interface{}{
            "limit": limit,
            "reset": resetTime,
        },
    }
}
```

### Error Classification Helpers

```go
// IsRetryable checks if an error can be retried
func IsRetryable(err error) bool {
    if err == nil {
        return false
    }

    var svcErr *ServiceError
    if errors.As(err, &svcErr) {
        return svcErr.Retryable
    }

    // Check for known retryable sentinel errors
    return errors.Is(err, ErrTimeout) ||
           errors.Is(err, ErrUpstreamFailed) ||
           errors.Is(err, ErrRateLimited)
}

// GetRootCause extracts the root cause from an error chain
func GetRootCause(err error) string {
    if err == nil {
        return ""
    }

    // Unwrap error chain to find root cause
    for {
        unwrapped := errors.Unwrap(err)
        if unwrapped == nil {
            return err.Error()
        }
        err = unwrapped
    }
}

// GetErrorCode extracts the error code from a ServiceError
func GetErrorCode(err error) string {
    if err == nil {
        return ""
    }

    var svcErr *ServiceError
    if errors.As(err, &svcErr) {
        return svcErr.Code
    }

    return "UNKNOWN"
}

// GetServiceName extracts the service name from a ServiceError
func GetServiceName(err error) string {
    if err == nil {
        return ""
    }

    var svcErr *ServiceError
    if errors.As(err, &svcErr) {
        return svcErr.Service
    }

    return "unknown"
}
```

### Usage Example

```go
// pkg/datastorage/service.go
package datastorage

import (
    "context"
    "database/sql"

    "github.com/jordigilh/kubernaut/pkg/shared/errors"
)

func (s *DataStorageService) GetActionTrace(ctx context.Context, id string) (*ActionTrace, error) {
    trace, err := s.db.QueryActionTrace(ctx, id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, errors.NewNotFoundError("data-storage", "ActionTrace", id)
        }
        return nil, errors.NewUpstreamError("data-storage", "postgres", err)
    }
    return trace, nil
}

func (s *DataStorageService) CreateAlert(ctx context.Context, alert *Alert) error {
    if err := s.validator.Validate(alert); err != nil {
        validationErrs := s.validator.GetErrors(err)
        return errors.NewValidationError("data-storage", "alert validation failed", validationErrs)
    }

    if err := s.db.Insert(ctx, alert); err != nil {
        if s.isDuplicateKeyError(err) {
            return errors.NewAlreadyExistsError("data-storage", "Alert", alert.ID)
        }
        return errors.NewUpstreamError("data-storage", "postgres", err)
    }

    return nil
}
```

---

## 📦 Error Wrapping Standards (Go 1.13+)

### Error Wrapping with %w

Go 1.13 introduced error wrapping with `%w` verb. Always use `%w` to preserve error chains.

```go
// ✅ CORRECT: Use %w to wrap errors
func (s *AIAnalysisService) analyzeAlert(ctx context.Context, alert *Alert) error {
    result, err := s.holmesClient.Analyze(ctx, alert)
    if err != nil {
        return fmt.Errorf("HolmesGPT analysis failed: %w", err)
    }

    if err := s.storeResult(ctx, result); err != nil {
        return fmt.Errorf("failed to store analysis result: %w", err)
    }

    return nil
}

// ❌ WRONG: Don't use %v (loses error chain)
func badExample(ctx context.Context) error {
    err := doSomething()
    if err != nil {
        return fmt.Errorf("operation failed: %v", err) // ❌ Error chain lost!
    }
    return nil
}
```

### Error Chain Inspection

Use `errors.Is` and `errors.As` to inspect error chains.

```go
import (
    "errors"
    "database/sql"

    sharedErrors "github.com/jordigilh/kubernaut/pkg/shared/errors"
)

func handleError(err error) {
    // Check for specific sentinel error anywhere in chain
    if errors.Is(err, sql.ErrNoRows) {
        log.Info("Resource not found")
        return
    }

    if errors.Is(err, sharedErrors.ErrTimeout) {
        log.Warn("Operation timed out - will retry")
        return
    }

    // Extract ServiceError from chain
    var svcErr *sharedErrors.ServiceError
    if errors.As(err, &svcErr) {
        log.Error("Service error occurred",
            "code", svcErr.Code,
            "service", svcErr.Service,
            "retryable", svcErr.Retryable,
        )
        return
    }

    // Unknown error
    log.Error("Unexpected error", "error", err)
}
```

### Multi-Level Error Wrapping

Error wrapping preserves the entire error chain.

```go
// Layer 3: Application logic
func (s *WorkflowService) executeStep(ctx context.Context, step *Step) error {
    if err := s.executor.Execute(ctx, step); err != nil {
        return fmt.Errorf("step execution failed: %w", err)
    }
    return nil
}

// Layer 2: Executor
func (e *Executor) Execute(ctx context.Context, step *Step) error {
    if err := e.k8sClient.Apply(ctx, step.Resource); err != nil {
        return fmt.Errorf("failed to apply resource: %w", err)
    }
    return nil
}

// Layer 1: Kubernetes client
func (c *K8sClient) Apply(ctx context.Context, resource interface{}) error {
    if err := c.client.Create(ctx, resource); err != nil {
        return errors.NewUpstreamError("k8s-client", "kubernetes-api", err)
    }
    return nil
}

// Error chain inspection at top level
func handleWorkflowError(err error) {
    // Can still find the original ServiceError
    var svcErr *errors.ServiceError
    if errors.As(err, &svcErr) {
        fmt.Printf("Original service: %s\n", svcErr.Service)     // "k8s-client"
        fmt.Printf("Error code: %s\n", svcErr.Code)              // "UPSTREAM_ERROR"
        fmt.Printf("Retryable: %v\n", svcErr.Retryable)          // true
    }
}
```

### Error Annotation Pattern

Add context while preserving error chain.

```go
func (s *DataStorageService) processQuery(ctx context.Context, query string) error {
    results, err := s.db.Query(ctx, query)
    if err != nil {
        // Annotate with context
        return fmt.Errorf("query failed [query=%s, database=%s]: %w",
            sanitizeQuery(query),
            s.dbName,
            err,
        )
    }

    if len(results) == 0 {
        // Create specific error with context
        return errors.NewNotFoundError("data-storage", "QueryResult", query).
            WithContext("query", sanitizeQuery(query)).
            WithContext("database", s.dbName)
    }

    return nil
}
```

### Don't Wrap Sentinel Errors

Sentinel errors should be returned directly or checked with `errors.Is`.

```go
var (
    ErrInvalidInput = errors.New("invalid input")
    ErrNotReady     = errors.New("service not ready")
)

// ✅ CORRECT: Return sentinel directly
func validate(input string) error {
    if input == "" {
        return ErrInvalidInput
    }
    return nil
}

// ✅ CORRECT: Check with errors.Is
func process(input string) error {
    if err := validate(input); err != nil {
        if errors.Is(err, ErrInvalidInput) {
            return err // Return directly, don't wrap
        }
        return fmt.Errorf("validation failed: %w", err)
    }
    return nil
}

// ❌ WRONG: Don't wrap sentinel errors
func badExample(input string) error {
    if input == "" {
        return fmt.Errorf("validation failed: %w", ErrInvalidInput) // ❌ Unnecessary wrap
    }
    return nil
}
```

---

## 🔄 Retry and Timeout Standards

### Retry Configuration Presets

```go
// pkg/shared/retry/config.go
package retry

import "time"

type BackoffConfig struct {
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   float64
    MaxRetries   int
    Jitter       bool
}

var (
    // FastRetry: For internal service calls (low latency)
    FastRetry = BackoffConfig{
        InitialDelay: 100 * time.Millisecond,
        MaxDelay:     2 * time.Second,
        Multiplier:   2.0,
        MaxRetries:   3,
        Jitter:       true,
    }

    // NormalRetry: For most operations
    NormalRetry = BackoffConfig{
        InitialDelay: 500 * time.Millisecond,
        MaxDelay:     10 * time.Second,
        Multiplier:   2.0,
        MaxRetries:   5,
        Jitter:       true,
    }

    // SlowRetry: For external services or heavy operations
    SlowRetry = BackoffConfig{
        InitialDelay: 1 * time.Second,
        MaxDelay:     30 * time.Second,
        Multiplier:   2.0,
        MaxRetries:   7,
        Jitter:       true,
    }
)
```

### Per-Service Timeout Budgets

```go
// pkg/shared/timeouts/config.go
package timeouts

import "time"

// ServiceTimeouts defines timeout budgets for all operations
var ServiceTimeouts = map[string]time.Duration{
    // CRD Controller Phase Timeouts
    "remediation-processing": 5 * time.Minute,
    "ai-analysis":            10 * time.Minute,
    "workflow-execution":     30 * time.Minute,
    "kubernetes-execution":   20 * time.Minute,

    // HTTP Service Operation Timeouts
    "data-storage-query":     5 * time.Second,
    "data-storage-write":     10 * time.Second,
    "gateway-webhook":        30 * time.Second,
    "context-api-query":      15 * time.Second,
    "holmesgpt-analysis":     60 * time.Second,
    "notification-send":      30 * time.Second,
    "effectiveness-assess":   45 * time.Second,

    // External Service Timeouts
    "holmesgpt-external":     120 * time.Second,
    "prometheus-query":       30 * time.Second,
    "kubernetes-api":         15 * time.Second,
    "redis-operation":        5 * time.Second,
    "database-query":         10 * time.Second,
}

func GetTimeout(operation string) time.Duration {
    if timeout, ok := ServiceTimeouts[operation]; ok {
        return timeout
    }
    return 30 * time.Second // Safe default
}
```

### Complete Retry Implementation

```go
// pkg/shared/retry/backoff.go
package retry

import (
    "context"
    "errors"
    "math"
    "math/rand"
    "time"
)

// RetryWithBackoff executes a function with exponential backoff retry logic
func RetryWithBackoff(ctx context.Context, config BackoffConfig, fn func() error) error {
    var lastErr error
    delay := config.InitialDelay

    for attempt := 0; attempt <= config.MaxRetries; attempt++ {
        // Skip delay on first attempt
        if attempt > 0 {
            // Calculate delay with jitter
            actualDelay := delay
            if config.Jitter {
                actualDelay = addJitter(delay)
            }

            // Wait with context cancellation support
            timer := time.NewTimer(actualDelay)
            select {
            case <-ctx.Done():
                timer.Stop()
                return ctx.Err()
            case <-timer.C:
            }

            // Calculate next delay (exponential backoff)
            delay = time.Duration(math.Min(
                float64(delay)*config.Multiplier,
                float64(config.MaxDelay),
            ))
        }

        // Attempt operation
        lastErr = fn()
        if lastErr == nil {
            return nil // Success
        }

        // Check if error is retryable
        if !isRetryableError(lastErr) {
            return lastErr // Non-retryable, fail fast
        }
    }

    // All retries exhausted
    return &RetryExhaustedError{
        Attempts: config.MaxRetries + 1,
        LastErr:  lastErr,
    }
}

// addJitter adds random jitter to delay to prevent thundering herd
func addJitter(delay time.Duration) time.Duration {
    // Add up to 25% random jitter
    jitter := time.Duration(rand.Int63n(int64(delay / 4)))
    return delay + jitter
}

// isRetryableError checks if an error should be retried
func isRetryableError(err error) bool {
    if err == nil {
        return false
    }

    // Check for retryable service errors
    var svcErr *ServiceError
    if errors.As(err, &svcErr) {
        return svcErr.Retryable
    }

    // Check for context errors (not retryable)
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return false
    }

    // Check for known retryable errors
    return errors.Is(err, ErrTimeout) ||
           errors.Is(err, ErrUpstreamFailed) ||
           errors.Is(err, ErrRateLimited)
}

// RetryExhaustedError is returned when all retry attempts are exhausted
type RetryExhaustedError struct {
    Attempts int
    LastErr  error
}

func (e *RetryExhaustedError) Error() string {
    return fmt.Sprintf("operation failed after %d attempts: %v", e.Attempts, e.LastErr)
}

func (e *RetryExhaustedError) Unwrap() error {
    return e.LastErr
}
```

### Retry Budget Tracking

```go
// pkg/shared/retry/budget.go
package retry

import (
    "sync"
    "time"
)

// RetryBudget tracks retry attempts over a time window
type RetryBudget struct {
    maxRetries   int
    windowSize   time.Duration
    attempts     []time.Time
    mu           sync.Mutex
}

// NewRetryBudget creates a new retry budget
func NewRetryBudget(maxRetries int, windowSize time.Duration) *RetryBudget {
    return &RetryBudget{
        maxRetries: maxRetries,
        windowSize: windowSize,
        attempts:   make([]time.Time, 0, maxRetries),
    }
}

// CanRetry checks if a retry is allowed within the budget
func (rb *RetryBudget) CanRetry() bool {
    rb.mu.Lock()
    defer rb.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-rb.windowSize)

    // Remove attempts outside the window
    validAttempts := make([]time.Time, 0, len(rb.attempts))
    for _, attempt := range rb.attempts {
        if attempt.After(cutoff) {
            validAttempts = append(validAttempts, attempt)
        }
    }
    rb.attempts = validAttempts

    return len(rb.attempts) < rb.maxRetries
}

// RecordAttempt records a retry attempt
func (rb *RetryBudget) RecordAttempt() {
    rb.mu.Lock()
    defer rb.mu.Unlock()

    rb.attempts = append(rb.attempts, time.Now())
}

// Remaining returns the number of retries remaining in the budget
func (rb *RetryBudget) Remaining() int {
    rb.mu.Lock()
    defer rb.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-rb.windowSize)

    count := 0
    for _, attempt := range rb.attempts {
        if attempt.After(cutoff) {
            count++
        }
    }

    return rb.maxRetries - count
}
```

### Retry Usage Example

```go
// Example: Retry with backoff for upstream service call
func (s *AIAnalysisService) callHolmesGPT(ctx context.Context, req *AnalysisRequest) (*AnalysisResponse, error) {
    var resp *AnalysisResponse

    err := retry.RetryWithBackoff(ctx, retry.NormalRetry, func() error {
        var err error
        resp, err = s.holmesClient.Analyze(ctx, req)
        if err != nil {
            // Classify error to determine if retryable
            if isNetworkError(err) {
                return errors.NewUpstreamError("ai-analysis", "holmesgpt", err)
            }
            return err // Non-retryable
        }
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("HolmesGPT analysis failed: %w", err)
    }

    return resp, nil
}

// Example: Retry with custom config and budget
func (s *DataStorageService) storeWithBudget(ctx context.Context, data *Data) error {
    budget := retry.NewRetryBudget(10, 1*time.Minute)

    if !budget.CanRetry() {
        return fmt.Errorf("retry budget exhausted")
    }

    customRetry := retry.BackoffConfig{
        InitialDelay: 200 * time.Millisecond,
        MaxDelay:     5 * time.Second,
        Multiplier:   1.5,
        MaxRetries:   3,
        Jitter:       true,
    }

    err := retry.RetryWithBackoff(ctx, customRetry, func() error {
        budget.RecordAttempt()
        return s.db.Insert(ctx, data)
    })

    return err
}
```

---

## 🛡️ Circuit Breaker Pattern

### When to Use Circuit Breakers

Use circuit breakers for:
- ✅ External service calls (HolmesGPT, Prometheus)
- ✅ Inter-service HTTP calls
- ✅ Database connections
- ❌ CRD operations (use native Kubernetes backoff)
- ❌ File I/O (use timeouts instead)

### Standard Circuit Breaker Configuration

```go
// pkg/shared/circuitbreaker/config.go
package circuitbreaker

import "time"

type Config struct {
    MaxFailures  int           // Open circuit after N failures
    Timeout      time.Duration // How long to keep circuit open
    HalfOpenMax  int           // Max requests in half-open state
}

var Configurations = map[string]Config{
    "holmesgpt-external": {
        MaxFailures: 5,
        Timeout:     5 * time.Minute, // Longer timeout for external AI
        HalfOpenMax: 1,
    },
    "prometheus-query": {
        MaxFailures: 3,
        Timeout:     30 * time.Second,
        HalfOpenMax: 1,
    },
    "data-storage": {
        MaxFailures: 10,
        Timeout:     15 * time.Second,
        HalfOpenMax: 3,
    },
}
```

### Complete Circuit Breaker Implementation

```go
// pkg/shared/circuitbreaker/breaker.go
package circuitbreaker

import (
    "errors"
    "fmt"
    "sync"
    "time"
)

// State represents the circuit breaker state
type State int

const (
    StateClosed   State = iota // Normal operation, requests pass through
    StateOpen                   // Failing, requests rejected immediately
    StateHalfOpen               // Testing if service recovered
)

func (s State) String() string {
    switch s {
    case StateClosed:
        return "closed"
    case StateOpen:
        return "open"
    case StateHalfOpen:
        return "half-open"
    default:
        return "unknown"
    }
}

var (
    ErrCircuitOpen = errors.New("circuit breaker is open")
    ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
    config          Config
    state           State
    failures        int
    lastFailTime    time.Time
    halfOpenCount   int
    mu              sync.RWMutex
    onStateChange   func(from, to State)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config Config) *CircuitBreaker {
    return &CircuitBreaker{
        config: config,
        state:  StateClosed,
    }
}

// OnStateChange registers a callback for state changes
func (cb *CircuitBreaker) OnStateChange(fn func(from, to State)) {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.onStateChange = fn
}

// Call executes the function if the circuit breaker allows it
func (cb *CircuitBreaker) Call(fn func() error) error {
    // Check if request is allowed
    if err := cb.beforeRequest(); err != nil {
        return err
    }

    // Execute function
    err := fn()

    // Update circuit breaker state
    cb.afterRequest(err)

    return err
}

// beforeRequest checks if request should be allowed
func (cb *CircuitBreaker) beforeRequest() error {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    now := time.Now()

    switch cb.state {
    case StateClosed:
        // Allow request
        return nil

    case StateOpen:
        // Check if timeout elapsed
        if now.Sub(cb.lastFailTime) > cb.config.Timeout {
            // Transition to half-open
            cb.setState(StateHalfOpen)
            cb.halfOpenCount = 0
            return nil
        }
        // Circuit still open
        return ErrCircuitOpen

    case StateHalfOpen:
        // Limit concurrent requests in half-open state
        if cb.halfOpenCount >= cb.config.HalfOpenMax {
            return ErrTooManyRequests
        }
        cb.halfOpenCount++
        return nil

    default:
        return fmt.Errorf("unknown circuit breaker state: %v", cb.state)
    }
}

// afterRequest updates circuit breaker based on result
func (cb *CircuitBreaker) afterRequest(err error) {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if err != nil {
        // Request failed
        cb.onFailure()
    } else {
        // Request succeeded
        cb.onSuccess()
    }
}

// onFailure handles a failed request
func (cb *CircuitBreaker) onFailure() {
    cb.failures++
    cb.lastFailTime = time.Now()

    switch cb.state {
    case StateClosed:
        if cb.failures >= cb.config.MaxFailures {
            cb.setState(StateOpen)
        }

    case StateHalfOpen:
        // Failure in half-open, go back to open
        cb.setState(StateOpen)
    }
}

// onSuccess handles a successful request
func (cb *CircuitBreaker) onSuccess() {
    switch cb.state {
    case StateClosed:
        // Already closed, just reset failure count
        cb.failures = 0

    case StateHalfOpen:
        // Success in half-open, close the circuit
        cb.failures = 0
        cb.halfOpenCount = 0
        cb.setState(StateClosed)
    }
}

// setState transitions to a new state and calls callback
func (cb *CircuitBreaker) setState(newState State) {
    oldState := cb.state
    if oldState == newState {
        return
    }

    cb.state = newState

    // Call state change callback (without holding lock)
    if cb.onStateChange != nil {
        go cb.onStateChange(oldState, newState)
    }
}

// GetState returns the current state (thread-safe)
func (cb *CircuitBreaker) GetState() State {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return cb.state
}

// GetFailures returns the current failure count
func (cb *CircuitBreaker) GetFailures() int {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return cb.failures
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.setState(StateClosed)
    cb.failures = 0
    cb.halfOpenCount = 0
}
```

### Circuit Breaker with Metrics

```go
// pkg/shared/circuitbreaker/metrics.go
package circuitbreaker

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    circuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kubernaut_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"service", "upstream"},
    )

    circuitBreakerFailures = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_circuit_breaker_failures_total",
            Help: "Total number of failures tracked by circuit breaker",
        },
        []string{"service", "upstream"},
    )

    circuitBreakerRejections = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_circuit_breaker_rejections_total",
            Help: "Total number of requests rejected by circuit breaker",
        },
        []string{"service", "upstream"},
    )
)

// MetricsCircuitBreaker wraps a circuit breaker with Prometheus metrics
type MetricsCircuitBreaker struct {
    *CircuitBreaker
    service  string
    upstream string
}

// NewMetricsCircuitBreaker creates a circuit breaker with metrics
func NewMetricsCircuitBreaker(config Config, service, upstream string) *MetricsCircuitBreaker {
    cb := NewCircuitBreaker(config)

    mcb := &MetricsCircuitBreaker{
        CircuitBreaker: cb,
        service:        service,
        upstream:       upstream,
    }

    // Register state change callback
    cb.OnStateChange(func(from, to State) {
        circuitBreakerState.WithLabelValues(service, upstream).Set(float64(to))
    })

    // Initialize state metric
    circuitBreakerState.WithLabelValues(service, upstream).Set(0) // Closed

    return mcb
}

// Call wraps the circuit breaker call with metrics
func (mcb *MetricsCircuitBreaker) Call(fn func() error) error {
    err := mcb.CircuitBreaker.Call(fn)

    if err != nil {
        if err == ErrCircuitOpen || err == ErrTooManyRequests {
            circuitBreakerRejections.WithLabelValues(mcb.service, mcb.upstream).Inc()
        } else {
            circuitBreakerFailures.WithLabelValues(mcb.service, mcb.upstream).Inc()
        }
    }

    return err
}
```

### Circuit Breaker Usage Example

```go
// Example: Using circuit breaker with upstream service
type HolmesGPTClient struct {
    breaker *circuitbreaker.MetricsCircuitBreaker
    client  *http.Client
}

func NewHolmesGPTClient() *HolmesGPTClient {
    config := circuitbreaker.Configurations["holmesgpt-external"]
    breaker := circuitbreaker.NewMetricsCircuitBreaker(config, "ai-analysis", "holmesgpt")

    return &HolmesGPTClient{
        breaker: breaker,
        client:  &http.Client{Timeout: 120 * time.Second},
    }
}

func (c *HolmesGPTClient) Analyze(ctx context.Context, req *AnalysisRequest) (*AnalysisResponse, error) {
    var resp *AnalysisResponse

    err := c.breaker.Call(func() error {
        var err error
        resp, err = c.doAnalyzeRequest(ctx, req)
        return err
    })

    if err != nil {
        if err == circuitbreaker.ErrCircuitOpen {
            return nil, errors.NewUpstreamError("ai-analysis", "holmesgpt", err).
                WithContext("circuit_breaker", "open")
        }
        return nil, err
    }

    return resp, nil
}

// Example: Circuit breaker with retry
func (c *DataStorageClient) QueryWithResilience(ctx context.Context, query string) (*Result, error) {
    var result *Result

    err := retry.RetryWithBackoff(ctx, retry.NormalRetry, func() error {
        return c.breaker.Call(func() error {
            var err error
            result, err = c.db.Query(ctx, query)
            return err
        })
    })

    return result, err
}
```

---

## 📊 Error Observability

### Error Logging Standards

```go
// pkg/shared/logging/error.go
package logging

import (
    "github.com/go-logr/logr"
    "github.com/jordigilh/kubernaut/pkg/shared/errors"
)

// LogError logs error with structured context
func LogError(logger logr.Logger, err error, operation string, fields ...interface{}) {
    // Extract structured error if available
    var svcErr *errors.ServiceError
    if errors.As(err, &svcErr) {
        logger.Error(err, operation,
            "errorCode", svcErr.Code,
            "service", svcErr.Service,
            "retryable", svcErr.Retryable,
            "timestamp", svcErr.Timestamp.Format(time.RFC3339),
        )

        // Add context fields
        for k, v := range svcErr.Context {
            logger = logger.WithValues(k, v)
        }
    } else {
        // Generic error
        logger.Error(err, operation, fields...)
    }
}
```

### Error Metrics

```go
// pkg/shared/metrics/errors.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Error counter by service, operation, and error code
    ErrorCounter = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_errors_total",
            Help: "Total number of errors by service, operation, and error code",
        },
        []string{"service", "operation", "error_code", "retryable"},
    )

    // Error rate gauge (calculated periodically)
    ErrorRate = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kubernaut_error_rate",
            Help: "Error rate by service (errors per minute)",
        },
        []string{"service"},
    )

    // Circuit breaker state
    CircuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kubernaut_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"service", "upstream"},
    )
)

// RecordError increments error counter
func RecordError(service, operation, errorCode string, retryable bool) {
    ErrorCounter.WithLabelValues(
        service,
        operation,
        errorCode,
        fmt.Sprintf("%t", retryable),
    ).Inc()
}
```

---

## 🎯 Error Handling Decision Matrix

| Scenario | HTTP Status | CRD Phase | Retry? | Circuit Breaker? | Timeout |
|----------|-------------|-----------|--------|------------------|---------|
| **Invalid request data** | 400 | N/A | No | No | N/A |
| **Authentication failed** | 401 | N/A | No | No | N/A |
| **Insufficient permissions** | 403 | N/A | No | No | N/A |
| **Resource not found** | 404 | N/A | No | No | N/A |
| **Validation failed** | 422 | Failed | No | No | N/A |
| **Rate limit exceeded** | 429 | N/A | Yes (60s) | No | N/A |
| **Internal error** | 500 | Failed | Maybe | No | 30s |
| **Upstream service error** | 502 | Failed | Yes | Yes | Service-specific |
| **Service unavailable** | 503 | N/A | Yes | Yes | 30s |
| **Upstream timeout** | 504 | Failed | Yes | Yes | Service-specific |
| **CRD phase timeout** | N/A | Failed | No (escalate) | No | Phase-specific |
| **Database connection** | 503 | Failed | Yes | Yes | 10s |
| **External API error** | 502 | Failed | Yes | Yes | 60-120s |

---

## ✅ Implementation Checklist

### For All Services

- [ ] Implement standard HTTP error responses
- [ ] Use structured error types (ServiceError)
- [ ] Configure timeouts for all operations
- [ ] Add retry logic with exponential backoff
- [ ] Implement circuit breakers for external calls
- [ ] Log all errors with structured context
- [ ] Emit error metrics (Prometheus)
- [ ] Add distributed tracing (OpenTelemetry)

### For CRD Controllers

- [ ] Use standard status phases
- [ ] Populate ErrorInfo on failures
- [ ] Set Kubernetes Conditions
- [ ] Emit Events for significant errors
- [ ] Update RemediationRequest status (Central Controller only)
- [ ] Handle phase timeouts with escalation

### For HTTP Services

- [ ] Return standard HTTPError responses
- [ ] Include request IDs for tracing
- [ ] Set Retry-After headers when appropriate
- [ ] Validate requests early (fail fast)
- [ ] Use circuit breakers for upstream calls

---

## 📚 Related Documents

- [03-testing-strategy.mdc](../.cursor/rules/03-testing-strategy.mdc) - Testing error scenarios
- [APPROVED_MICROSERVICES_ARCHITECTURE.md](./APPROVED_MICROSERVICES_ARCHITECTURE.md) - Service architecture
- [SERVICE_CONNECTIVITY_SPECIFICATION.md](./SERVICE_CONNECTIVITY_SPECIFICATION.md) - Service dependencies
- [ADR-001-crd-microservices-architecture.md](./decisions/ADR-001-crd-microservices-architecture.md) - Architecture decisions

---

**Document Status**: ✅ **APPROVED**
**Authority**: Architecture Standard (applies to all services)
**Enforcement**: Mandatory for V1 implementation
**Last Updated**: October 6, 2025

