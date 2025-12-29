# DD-SHARED-002: Shared Error Classification Library

**Status**: 📋 **APPROVED** (Implementation Pending)
**Date**: 2025-12-20
**Decision Maker**: AIAnalysis Team (AA), Cross-Service Triage
**Implementation**: `/pkg/shared/errors/` (to be created)
**Priority**: P1 - Cross-service consistency and maintainability

---

## 📋 **Context**

### Problem Statement
Multiple Kubernaut services implement error classification independently, leading to:
- **Code duplication**: Each service reimplements HTTP status code checking, network error detection, and retry logic
- **Inconsistency**: Different services classify the same errors differently (e.g., 429 handling)
- **Incomplete coverage**: No single service handles ALL error types (HTTP + K8s API + Network + Context)
- **Maintenance burden**: Bug fixes or enhancements require changes across multiple services
- **Testing gaps**: Each implementation needs independent validation

### Discovery Context
**Source**: AIAnalysis P2.1 refactoring triage (2025-12-20)
**Evidence**: Code duplication found across 3 services:

| Service | Location | Error Types | HTTP | K8s API | Network |
|---------|----------|-------------|------|---------|---------|
| **AIAnalysis** | `pkg/aianalysis/handlers/error_classifier.go` | String-based | ✅ | ❌ | ✅ |
| **SignalProcessing** | `internal/controller/signalprocessing/*.go` | K8s API helpers | ❌ | ✅ | ✅ |
| **Notification** | `pkg/notification/retry/policy.go` | HTTP status map | ✅ | ❌ | ✅ |

**Key Insight**: No service has comprehensive coverage. Each implements only what it needs.

### Scope
This decision applies to ALL Kubernaut services that make external calls or handle transient errors:
- ✅ **AIAnalysis (AA)**: HolmesGPT-API calls (HTTP errors)
- ✅ **SignalProcessing (SP)**: K8s API operations
- ✅ **Notification (NT)**: Slack webhook calls (HTTP errors)
- 🔜 **RemediationOrchestrator (RO)**: Cross-service orchestration
- 🔜 **WorkflowExecution (WE)**: Argo workflow API calls
- 🔜 **DataStorage (DS)**: Database client errors (if exposed)

---

## 🎯 **Decision**

### Adopt Shared Error Classification Library (`pkg/shared/errors`)
**Pattern**: Following DD-SHARED-001 (Shared Backoff Library) success pattern

**Key Features**:
1. **Comprehensive coverage**: HTTP + K8s API + Network + Context errors
2. **Error categorization**: For metrics and observability (transient, auth, validation, rate limit, etc.)
3. **Type-safe**: Uses Go error types when available (e.g., `apierrors` from client-go)
4. **String-based fallback**: For wrapped errors (e.g., ogen-generated clients)
5. **Configurable**: Services can customize classification for domain-specific errors

---

## 🏗️ **Architecture**

### Core Type: `ErrorCategory`

```go
package errors

// ErrorCategory for observability and metrics
type ErrorCategory string

const (
    CategoryTransient      ErrorCategory = "transient"       // Retriable (5xx, network, K8s timeouts)
    CategoryAuthentication ErrorCategory = "authentication"  // 401, 403, K8s Unauthorized
    CategoryValidation     ErrorCategory = "validation"      // 400, 422, invalid input
    CategoryRateLimit      ErrorCategory = "rate_limit"      // 429, K8s TooManyRequests
    CategoryNotFound       ErrorCategory = "not_found"       // 404, K8s NotFound
    CategoryConflict       ErrorCategory = "conflict"        // 409, K8s AlreadyExists
    CategoryPermanent      ErrorCategory = "permanent"       // Non-retriable
)
```

### Core Type: `Classifier`

```go
// Classifier provides unified error classification
type Classifier struct {
    // Optional: Service-specific customization
    CustomPatterns map[string]ErrorCategory
}

// NewClassifier creates a classifier with optional customization
func NewClassifier(opts ...Option) *Classifier

// IsTransient checks if error should be retried (ALL error types)
func (c *Classifier) IsTransient(err error) bool

// Classify categorizes error for metrics/observability
func (c *Classifier) Classify(err error) ErrorCategory

// HTTP status code helpers
func IsRetryableHTTPStatus(statusCode int) bool
func GetRetryableStatusCodes() []int
func GetPermanentStatusCodes() []int

// K8s API helpers (wraps client-go apierrors)
func IsK8sTransient(err error) bool
func IsK8sConflict(err error) bool
func IsK8sNotFound(err error) bool

// Network error detection
func IsNetworkError(err error) bool

// Context error helpers
func IsContextCanceled(err error) bool
func IsContextTimeout(err error) bool
```

---

## ✅ **Benefits**

### 1. Single Source of Truth
**Before**: 3 services with 3 different implementations (~250 lines total)
**After**: Shared library with 100% test coverage (~200 lines, comprehensive)

### 2. Comprehensive Coverage
**Before**: No service handles ALL error types
**After**: One library handles HTTP + K8s API + Network + Context

### 3. Consistent Observability
**Error categories** enable unified metrics across services:
- `error_category="transient"` → Service behavior comparison
- `error_category="rate_limit"` → API quota monitoring
- `error_category="authentication"` → Security issue detection

### 4. Maintainability
**Single fix propagates**: Update once, benefit everywhere

### 5. Flexibility
**Customizable**: Services can extend with domain-specific patterns

---

## 🚧 **Trade-offs**

### Complexity vs Coverage
**Decision**: Accept comprehensive library over service-specific simplicity
**Rationale**: Long-term maintenance > short-term simplicity
**Mitigation**: Provide simple `IsTransient(err)` for common case

### String Parsing vs Type Checking
**Decision**: Support BOTH type-safe (K8s API) and string-based (HTTP wrapped errors)
**Rationale**: ogen-generated clients wrap errors as strings, need string parsing
**Implementation**: Try type assertions first, fall back to string parsing

### Backward Compatibility
**Decision**: No breaking changes required for migration
**Implementation**: Each service can migrate incrementally

---

## 📚 **Implementation Details**

### Package Location
```
pkg/shared/errors/
├── classifier.go      # Core implementation
├── http.go            # HTTP status code classification
├── k8s.go             # K8s API error helpers (wraps client-go)
├── network.go         # Network error detection
├── context.go         # Context error helpers
└── classifier_test.go # Comprehensive unit tests
```

### HTTP Status Code Coverage

**Retriable (Transient)**:
- 408 Request Timeout
- 429 Too Many Requests (rate limit)
- 500 Internal Server Error
- 502 Bad Gateway
- 503 Service Unavailable
- 504 Gateway Timeout

**Non-Retriable (Permanent)**:
- 400 Bad Request (validation)
- 401 Unauthorized (authentication)
- 403 Forbidden (authorization)
- 404 Not Found
- 409 Conflict
- 422 Unprocessable Entity (validation)

### K8s API Error Coverage

**Transient** (from `client-go/apierrors`):
- `IsTimeout`
- `IsServerTimeout`
- `IsTooManyRequests`
- `IsServiceUnavailable`
- `IsConflict` (with retry on conflict pattern)

**Permanent**:
- `IsNotFound`
- `IsUnauthorized`
- `IsForbidden`
- `IsInvalid`
- `IsAlreadyExists`

### Network Error Patterns

```go
// Network errors (transient)
- "connection refused"
- "connection reset"
- "connection closed"
- "network unreachable"
- "host unreachable"
- "dns"
- "name resolution"
- "temporary failure"
- "temporarily unavailable"
```

---

## 🔄 **Migration Plan**

### Phase 1: 🔜 Create Shared Library (Week 1)
**Owner**: AIAnalysis Team
**Estimated Effort**: 4 hours
**Steps**:
1. Create `pkg/shared/errors/` package structure
2. Extract and consolidate logic from 3 services
3. Create comprehensive unit tests (target: 30+ tests)
4. Document API with examples

**Deliverables**:
- ✅ `pkg/shared/errors/` with 100% test coverage
- ✅ API documentation in package comments
- ✅ Migration guide in this DD

### Phase 2: 🔜 Migrate AIAnalysis (Week 1 - P1)
**Owner**: AIAnalysis Team
**Estimated Effort**: 1 hour
**Steps**:
1. Replace `pkg/aianalysis/handlers/error_classifier.go` with `pkg/shared/errors`
2. Update `isTransientError()` to use `classifier.IsTransient()`
3. Add error category to metrics (optional enhancement)
4. Run integration tests to validate

**Migration Pattern**:
```go
// Before (AIAnalysis):
if isTransientError(err) {
    // retry logic
}

// After (using shared library):
classifier := errors.NewClassifier()
if classifier.IsTransient(err) {
    // retry logic
    // Optional: Track category for metrics
    category := classifier.Classify(err)
    metrics.RecordErrorCategory(category)
}
```

### Phase 3: 🔜 Migrate SignalProcessing (Week 2 - P1)
**Owner**: SignalProcessing Team
**Estimated Effort**: 2 hours
**Steps**:
1. Replace local `isTransientError()` with `pkg/shared/errors`
2. Preserve K8s API error detection (already comprehensive)
3. **Enhancement**: Add HTTP error classification for future external API calls
4. Run integration tests to validate

**Migration Pattern**:
```go
// Before (SignalProcessing):
func isTransientError(err error) bool {
    if apierrors.IsTimeout(err) || apierrors.IsTooManyRequests(err) {
        return true
    }
    return false
}

// After (using shared library - enhanced coverage):
classifier := errors.NewClassifier()
if classifier.IsTransient(err) {
    // Now handles K8s API + HTTP + Network errors
}
```

### Phase 4: 🔜 Migrate Notification (Week 2 - P2)
**Owner**: Notification Team
**Estimated Effort**: 1 hour
**Steps**:
1. Replace `pkg/notification/retry/policy.go` HTTP status map
2. Update `IsRetryable()` method to use `pkg/shared/errors`
3. **Enhancement**: Add network error detection (currently treats all non-HTTP as transient)
4. Run integration tests to validate

**Migration Pattern**:
```go
// Before (Notification):
func isRetryableHTTPStatus(statusCode int) bool {
    retryableCodes := map[int]bool{
        408: true, 429: true, 500: true, 502: true, 503: true, 504: true,
    }
    return retryableCodes[statusCode]
}

// After (using shared library):
if errors.IsRetryableHTTPStatus(statusCode) {
    return true
}
```

### Phase 5: 🔜 Other Services (Opportunistic)
**When**: During implementation of error-handling BRs
**Services**: RemediationOrchestrator, WorkflowExecution, DataStorage
**Pattern**: Adopt during new feature development

---

## 🎓 **Teaching Guide**

### For New Team Members

**Key Concepts**:
1. **Transient vs Permanent**: Transient errors may succeed on retry, permanent errors won't
2. **Error Categories**: For metrics (transient, auth, validation, rate limit, etc.)
3. **Multi-source**: Handles HTTP, K8s API, Network, Context errors
4. **Type-safe when possible**: Uses client-go types for K8s, falls back to string parsing

**When to Use**:
- ✅ **External API calls**: HolmesGPT, Slack, any HTTP service
- ✅ **K8s API operations**: CRD create/update/delete
- ✅ **Retry logic**: Determining if error is transient
- ✅ **Metrics/observability**: Categorizing errors for monitoring

### For Code Reviewers

**What to Look For**:
- ✅ Using `classifier.IsTransient(err)` instead of custom string parsing
- ✅ Using `errors.IsRetryableHTTPStatus(code)` instead of map lookups
- ✅ Tracking error categories in metrics for observability
- ❌ Custom error classification logic that duplicates shared library
- ❌ Hardcoded HTTP status code checks that shared library already handles

---

## 🔗 **Related Decisions**

### Dependencies
- **DD-SHARED-001**: Shared Backoff Library (works hand-in-hand with error classification)
- **DD-METRICS-001**: Controller Metrics Wiring (error categories feed into metrics)

### Influences
- **BR-AI-009**: Retry transient errors (AIAnalysis)
- **BR-AI-010**: Fail immediately on permanent errors (AIAnalysis)
- **BR-NOT-052**: Automatic retry with custom policies (Notification)

---

## 📞 **Communication Plan**

### Service Migration Announcement

**Document**: `docs/handoff/SHARED_ERROR_CLASSIFICATION_MIGRATION_DEC_20_2025.md`

**Recipients**:
- [x] **AIAnalysis (AA)**: P1 - Create library and migrate (Week 1)
- [ ] **SignalProcessing (SP)**: P1 - Migrate (Week 2)
- [ ] **Notification (NT)**: P2 - Migrate (Week 2)
- [ ] **RemediationOrchestrator (RO)**: FYI - Available for future use
- [ ] **WorkflowExecution (WE)**: FYI - Available for future use
- [ ] **DataStorage (DS)**: FYI - May benefit from DB error classification

**Key Message**:
```
📢 NEW SHARED UTILITY: Error Classification Library

Location: pkg/shared/errors/ (to be created)
Status: 📋 Approved (DD-SHARED-002)
Pattern: Following DD-SHARED-001 success

PROBLEM SOLVED:
- Code duplication across 3 services (~250 lines)
- Inconsistent error handling (same errors, different classifications)
- Incomplete coverage (no service handles ALL error types)

WHO NEEDS TO MIGRATE:
- AA Team: P1 - Create library + migrate (Week 1, 4+1 hours)
- SP Team: P1 - Migrate (Week 2, 2 hours)
- NT Team: P2 - Migrate (Week 2, 1 hour)

BENEFITS:
- Comprehensive: HTTP + K8s API + Network + Context errors
- Consistent: Same classification across all services
- Observable: Error categories for metrics
- Maintainable: Fix once, benefit everywhere

DOCUMENTATION:
- Design Decision: DD-SHARED-002
- Migration Guide: SHARED_ERROR_CLASSIFICATION_MIGRATION_DEC_20_2025.md
```

---

## ⚖️ **Decision Log**

### Key Decision Points

#### 1. Extract New vs Enhance Existing?
**Decision**: Extract new comprehensive library
**Rationale**: No single service has complete coverage; consolidation is more valuable than enhancement
**Alternative Rejected**: Pick one service's implementation (would leave gaps)

#### 2. String Parsing vs Type-Only?
**Decision**: ✅ Support BOTH
**Rationale**: ogen-generated clients wrap errors as strings (can't avoid string parsing)
**Implementation**: Try type assertions first (K8s API), fall back to string parsing (HTTP)

#### 3. Include Error Categories?
**Decision**: ✅ YES
**Rationale**: Enables rich observability and metrics across services
**Example**: Track `error_category="rate_limit"` to monitor API quotas

#### 4. Customization Support?
**Decision**: ✅ YES (optional custom patterns)
**Rationale**: Services may have domain-specific errors to classify
**Trade-off**: Slightly more complex API, but flexible

---

## 🎯 **Success Metrics**

### Implementation Success (Week 1)
- ✅ Shared library created (`pkg/shared/errors/`)
- ✅ 30+ unit tests passing (100% coverage)
- ✅ AA migrated successfully (integration tests pass)

### Adoption Success (1 month)
- **Target**: 3/3 services migrated (AA, SP, NT)
- **Metric**: ~250 lines of error classification code eliminated
- **Quality**: Zero error classification bugs in services using shared library

### Observability Success (3 months)
- **Metrics**: Error categories tracked across all services
- **Dashboards**: Unified error category dashboard for SOC2 compliance
- **Insights**: Cross-service error patterns visible

### Long-term Impact (6 months)
- **Consistency**: All services classify errors identically
- **Maintainability**: Single source of truth for error logic
- **Coverage**: All error types (HTTP + K8s + Network + Context) handled uniformly

---

## 📚 **References**

- **DD-SHARED-001**: Shared Backoff Library (successful pattern to follow)
- **AIAnalysis P2.1 Triage**: Discovery of code duplication (2025-12-20)
- **client-go apierrors**: https://pkg.go.dev/k8s.io/apimachinery/pkg/api/errors
- **HTTP Status Codes**: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status

---

## ✅ **Approval**

**Approved By**: User (2025-12-20)
**Implementation Owner**: AIAnalysis Team
**Review Date**: 2026-01-20 (1 month post-implementation)

