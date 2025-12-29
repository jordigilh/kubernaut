# 📢 Shared Error Classification Library - Service Migration Guide

**Date**: 2025-12-20
**Status**: 📋 **APPROVED** - Ready for Implementation
**Decision**: [DD-SHARED-002](../architecture/decisions/DD-SHARED-002-shared-error-classification.md)
**Priority**: P1 for AA/SP, P2 for NT
**Pattern**: Following [DD-SHARED-001](../architecture/decisions/DD-SHARED-001-shared-backoff-library.md) success

---

## 🎯 **TL;DR - What You Need to Know**

### **The Problem**
We discovered **code duplication** across 3 services during AIAnalysis refactoring:
- 🔴 **250+ lines** of duplicated error classification logic
- 🔴 **Inconsistent** error handling (same errors, different classifications)
- 🔴 **Incomplete** coverage (no service handles ALL error types)

### **The Solution**
Create **`pkg/shared/errors/`** - A comprehensive error classification library following DD-SHARED-001 pattern.

### **Why This Matters**
- ✅ **Single source of truth**: Fix once, benefit everywhere
- ✅ **Comprehensive**: HTTP + K8s API + Network + Context errors
- ✅ **Observable**: Error categories for metrics and dashboards
- ✅ **Consistent**: Same classification across all services

---

## 👥 **Who Needs to Act?**

| Team | Action | Priority | Effort | Week |
|------|--------|----------|--------|------|
| **AIAnalysis (AA)** | Create library + Migrate | **P1** | 4h + 1h | Week 1 |
| **SignalProcessing (SP)** | Migrate existing logic | **P1** | 2h | Week 2 |
| **Notification (NT)** | Migrate HTTP status map | **P2** | 1h | Week 2 |
| **RemediationOrchestrator** | FYI - Use for future features | Info | N/A | Future |
| **WorkflowExecution** | FYI - Use for future features | Info | N/A | Future |
| **DataStorage** | FYI - May benefit from DB errors | Info | N/A | Future |

---

## 📊 **Current State Analysis**

### **Code Duplication Found**

| Service | Location | Lines | Error Types | Coverage |
|---------|----------|-------|-------------|----------|
| **AIAnalysis** | `pkg/aianalysis/handlers/error_classifier.go` | ~84 | String-based HTTP parsing | HTTP + Network ✅ |
| **SignalProcessing** | `internal/controller/signalprocessing/*.go` | ~20 | K8s API helpers | K8s API ✅ |
| **Notification** | `pkg/notification/retry/policy.go` | ~30 | HTTP status code map | HTTP only ✅ |
| **TOTAL** | 3 files | **~134 lines** | Inconsistent | **Incomplete** |

### **Gaps Identified**

| Service | Missing Coverage |
|---------|------------------|
| **AIAnalysis** | ❌ No K8s API error handling |
| **SignalProcessing** | ❌ No HTTP error classification |
| **Notification** | ❌ No network error detection (treats all non-HTTP as transient) |
| **All Services** | ❌ No unified error categories for metrics |

---

## 🏗️ **The Shared Library**

### **Package Structure**
```
pkg/shared/errors/
├── classifier.go      # Core implementation (IsTransient, Classify)
├── http.go            # HTTP status code classification
├── k8s.go             # K8s API error helpers (wraps client-go)
├── network.go         # Network error detection
├── context.go         # Context error helpers
└── classifier_test.go # Comprehensive unit tests (30+)
```

### **Core API**

```go
package errors

// ErrorCategory for observability
type ErrorCategory string

const (
    CategoryTransient      ErrorCategory = "transient"       // Retriable
    CategoryAuthentication ErrorCategory = "authentication"  // 401, 403
    CategoryValidation     ErrorCategory = "validation"      // 400, 422
    CategoryRateLimit      ErrorCategory = "rate_limit"      // 429
    CategoryNotFound       ErrorCategory = "not_found"       // 404
    CategoryConflict       ErrorCategory = "conflict"        // 409
    CategoryPermanent      ErrorCategory = "permanent"       // Non-retriable
)

// Classifier provides unified error classification
type Classifier struct {
    // Optional: Service-specific customization
}

// Core methods
func NewClassifier(opts ...Option) *Classifier
func (c *Classifier) IsTransient(err error) bool
func (c *Classifier) Classify(err error) ErrorCategory

// HTTP helpers
func IsRetryableHTTPStatus(statusCode int) bool
func GetRetryableStatusCodes() []int  // Returns: [408, 429, 500, 502, 503, 504]

// K8s helpers (wraps client-go apierrors)
func IsK8sTransient(err error) bool
func IsK8sConflict(err error) bool

// Network helpers
func IsNetworkError(err error) bool

// Context helpers
func IsContextCanceled(err error) bool
func IsContextTimeout(err error) bool
```

---

## 🚀 **Migration Guide**

### **AIAnalysis (AA) - P1, Week 1**

**Step 1: Create Shared Library (4 hours)**

Owner: AIAnalysis Team
Task: Extract and consolidate logic from all 3 services

```bash
# 1. Create package structure
mkdir -p pkg/shared/errors
touch pkg/shared/errors/{classifier.go,http.go,k8s.go,network.go,context.go,classifier_test.go}

# 2. Extract logic from:
#    - pkg/aianalysis/handlers/error_classifier.go (HTTP + Network)
#    - internal/controller/signalprocessing/*.go (K8s API)
#    - pkg/notification/retry/policy.go (HTTP status map)

# 3. Create 30+ unit tests for comprehensive coverage

# 4. Validate tests pass
go test ./pkg/shared/errors/... -v
```

**Step 2: Migrate AIAnalysis (1 hour)**

```go
// Before (pkg/aianalysis/handlers/error_classifier.go):
func isTransientError(err error) bool {
    if errors.Is(err, context.Canceled) {
        return false
    }
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    errMsg := err.Error()
    if containsTransientHTTPError(errMsg) {
        return true
    }
    return false
}

// After (using shared library):
import sharedErrors "github.com/jordigilh/kubernaut/pkg/shared/errors"

var classifier = sharedErrors.NewClassifier()

func isTransientError(err error) bool {
    return classifier.IsTransient(err)
}

// Optional: Add error categories to metrics
func handleError(ctx context.Context, err error) {
    if classifier.IsTransient(err) {
        category := classifier.Classify(err)
        metrics.RecordErrorCategory(string(category)) // "transient", "rate_limit", etc.
        // retry logic
    } else {
        // permanent failure
    }
}
```

**Validation**:
```bash
# Run integration tests
go test -v ./test/integration/aianalysis/... -timeout 10m

# Expected: 53/53 tests passing (no regressions)
```

---

### **SignalProcessing (SP) - P1, Week 2**

**Owner**: SignalProcessing Team
**Effort**: 2 hours
**Enhancement**: Adds HTTP error classification for future external API calls

```go
// Before (internal/controller/signalprocessing/*.go):
func isTransientError(err error) bool {
    if err == nil {
        return false
    }
    // K8s API transient errors
    if apierrors.IsTimeout(err) ||
        apierrors.IsServerTimeout(err) ||
        apierrors.IsTooManyRequests(err) ||
        apierrors.IsServiceUnavailable(err) {
        return true
    }
    // Context deadline/cancellation
    if err == context.DeadlineExceeded || err == context.Canceled {
        return true
    }
    return false
}

// After (using shared library - ENHANCED):
import sharedErrors "github.com/jordigilh/kubernaut/pkg/shared/errors"

var classifier = sharedErrors.NewClassifier()

func isTransientError(err error) bool {
    // Now handles: K8s API + HTTP + Network + Context
    // Future-proof for external API calls!
    return classifier.IsTransient(err)
}
```

**Benefits**:
- ✅ Maintains existing K8s API error detection
- ✅ Adds HTTP error classification (ready for future external API calls)
- ✅ Adds network error detection (connection refused, DNS, etc.)
- ✅ Error categories available for metrics

**Validation**:
```bash
# Run integration tests
go test -v ./test/integration/signalprocessing/... -timeout 10m

# Expected: All tests passing (no behavior change)
```

---

### **Notification (NT) - P2, Week 2**

**Owner**: Notification Team
**Effort**: 1 hour
**Enhancement**: Adds network error detection (currently treats all non-HTTP as transient)

```go
// Before (pkg/notification/retry/policy.go):
func isRetryableHTTPStatus(statusCode int) bool {
    retryableCodes := map[int]bool{
        408: true, // Request Timeout
        429: true, // Too Many Requests
        500: true, // Internal Server Error
        502: true, // Bad Gateway
        503: true, // Service Unavailable
        504: true, // Gateway Timeout
    }
    return retryableCodes[statusCode]
}

func (p *Policy) IsRetryable(err error) bool {
    if httpErr, ok := err.(*HTTPError); ok {
        return isRetryableHTTPStatus(httpErr.StatusCode)
    }
    // Network errors are typically transient
    return true // ← Too broad!
}

// After (using shared library - ENHANCED):
import sharedErrors "github.com/jordigilh/kubernaut/pkg/shared/errors"

var classifier = sharedErrors.NewClassifier()

func (p *Policy) IsRetryable(err error) bool {
    // Now properly detects network vs permanent errors
    return classifier.IsTransient(err)
}

// HTTP status helper still available:
func validateHTTPStatus(statusCode int) bool {
    return sharedErrors.IsRetryableHTTPStatus(statusCode)
}
```

**Benefits**:
- ✅ Replaces hardcoded HTTP status map with shared implementation
- ✅ Adds proper network error detection (not just "assume all non-HTTP is transient")
- ✅ Error categories available for metrics

**Validation**:
```bash
# Run integration tests
go test -v ./test/integration/notification/... -timeout 10m

# Expected: All tests passing
```

---

## 📈 **Benefits by Service**

| Service | Current Issues | After Migration |
|---------|----------------|-----------------|
| **AIAnalysis** | ❌ No K8s error handling<br>❌ String-only HTTP parsing | ✅ Comprehensive coverage<br>✅ Error categories for metrics |
| **SignalProcessing** | ❌ No HTTP error classification<br>❌ No network detection | ✅ Ready for external APIs<br>✅ Enhanced error detection |
| **Notification** | ❌ Hardcoded HTTP map<br>❌ Overly broad "all non-HTTP = transient" | ✅ Proper network detection<br>✅ Accurate classification |

---

## 🧪 **Testing Strategy**

### **Shared Library Tests (AA Team - Week 1)**
```bash
# Unit tests (30+ tests)
go test ./pkg/shared/errors/... -v -cover

# Target coverage: 100%
# Test categories:
# - HTTP status codes (retriable vs permanent)
# - K8s API errors (all apierrors types)
# - Network errors (connection refused, DNS, etc.)
# - Context errors (canceled vs timeout)
# - Error categorization (transient, auth, validation, etc.)
# - Custom patterns (service-specific errors)
```

### **Service Migration Tests**
```bash
# For each service after migration:
go test -v ./test/integration/[service]/... -timeout 10m

# Expected results:
# - AIAnalysis: 53/53 integration tests passing
# - SignalProcessing: All integration tests passing
# - Notification: All integration tests passing

# Regression test: Behavior must remain identical
```

---

## 📊 **Metrics Enhancement (Optional)**

### **Error Category Tracking**

```go
// Optional enhancement: Track error categories in metrics
func (h *Handler) handleError(ctx context.Context, err error) {
    category := classifier.Classify(err)

    // Track in metrics for observability
    h.metrics.ErrorsTotal.WithLabelValues(
        string(category),  // "transient", "rate_limit", "authentication"
        h.serviceName,
    ).Inc()

    // Log for debugging
    h.log.Info("Error classified",
        "category", category,
        "error", err,
        "transient", classifier.IsTransient(err),
    )
}
```

### **Dashboard Queries (Prometheus)**

```promql
# Transient error rate by service
sum(rate(errors_total{category="transient"}[5m])) by (service)

# Rate limiting issues
sum(rate(errors_total{category="rate_limit"}[5m])) by (service)

# Authentication failures (security monitoring)
sum(rate(errors_total{category="authentication"}[5m])) by (service)
```

---

## 🔗 **Resources**

### **Documentation**
- **Design Decision**: [DD-SHARED-002](../architecture/decisions/DD-SHARED-002-shared-error-classification.md)
- **Pattern Reference**: [DD-SHARED-001](../architecture/decisions/DD-SHARED-001-shared-backoff-library.md)
- **K8s API Errors**: https://pkg.go.dev/k8s.io/apimachinery/pkg/api/errors
- **HTTP Status Codes**: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status

### **Code Locations**
- **AIAnalysis**: `pkg/aianalysis/handlers/error_classifier.go` (to be replaced)
- **SignalProcessing**: `internal/controller/signalprocessing/*.go` (`isTransientError` function)
- **Notification**: `pkg/notification/retry/policy.go` (`isRetryableHTTPStatus` function)

---

## 📞 **Questions & Support**

### **Contact**
- **Implementation Lead**: AIAnalysis Team
- **Technical Questions**: Post in #kubernaut-dev Slack channel
- **Migration Support**: Pair with AA team during Week 1-2

### **Common Questions**

**Q: Will this break existing behavior?**
A: No. Migration is backward compatible. Same logic, just consolidated.

**Q: Do I need to update all error handling immediately?**
A: No. Migrate incrementally. Start with `IsTransient()`, add error categories later.

**Q: What about service-specific errors?**
A: Classifier supports custom patterns. Example:
```go
classifier := errors.NewClassifier(
    errors.WithCustomPattern("my-service-error", errors.CategoryTransient),
)
```

**Q: How does this relate to DD-SHARED-001 (Backoff)?**
A: They work together! Error classification determines IF to retry, backoff determines WHEN to retry.

---

## ✅ **Success Criteria**

### **Week 1 (AA Team)**
- ✅ `pkg/shared/errors/` created with 30+ tests passing
- ✅ AIAnalysis migrated and integration tests passing (53/53)
- ✅ Documentation complete (API docs in package comments)

### **Week 2 (SP + NT Teams)**
- ✅ SignalProcessing migrated, integration tests passing
- ✅ Notification migrated, integration tests passing
- ✅ 3/3 services using shared library

### **1 Month Post-Implementation**
- ✅ ~134 lines of duplicated code eliminated
- ✅ Zero error classification bugs
- ✅ Error categories tracked in metrics (if implemented)

---

## 🎯 **Call to Action**

### **AIAnalysis Team (Week 1)**
1. Create `pkg/shared/errors/` package (4 hours)
2. Migrate AIAnalysis service (1 hour)
3. Notify SP and NT teams when ready

### **SignalProcessing Team (Week 2)**
1. Review DD-SHARED-002 and this migration guide
2. Schedule 2-hour migration window
3. Run integration tests to validate

### **Notification Team (Week 2)**
1. Review DD-SHARED-002 and this migration guide
2. Schedule 1-hour migration window
3. Run integration tests to validate

---

**Last Updated**: 2025-12-20
**Document Owner**: AIAnalysis Team
**Review Date**: 2026-01-20 (1 month post-implementation)

