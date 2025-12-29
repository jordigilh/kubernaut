# AIAnalysis Mock HAPI Error Injection Assessment

**Date**: December 24, 2025
**Service**: AIAnalysis
**Status**: ✅ RECOMMENDATION - Move error classification tests to unit tier
**Authority**: User instruction "no test logic in production code"

---

## 📋 EXECUTIVE SUMMARY

**Question**: How to make real HAPI return specific HTTP error codes (401, 403, 503, timeout) for error classification tests?

**Answer**: ❌ **NOT RECOMMENDED** for integration tests

**Recommendation**: ✅ **Move error classification tests to UNIT TEST tier** where we can mock the HAPI client completely.

**Rationale**:
1. HAPI runs in `dev_mode=true` in integration tests (no production auth logic)
2. No error injection mechanism exists without modifying production code
3. Unit tests provide **BETTER isolation** for error classification logic testing
4. Follows user mandate: "no test logic in production code"

---

## 🔍 HAPI ERROR INJECTION INVESTIGATION

### Current HAPI Configuration in Integration Tests

**Environment** (`test/integration/aianalysis/podman-compose.yml:87-91`):
```yaml
environment:
  MOCK_LLM_MODE: "true"  # Mock LLM responses, not HTTP errors
  DATASTORAGE_URL: http://datastorage:8080
  PORT: 8080
```

**Default Configuration** (`holmesgpt-api/src/main.py:125-129`):
```python
config = {
    "dev_mode": True,      # ⚠️ Development mode enabled by default
    "auth_enabled": False, # ⚠️ Authentication disabled by default
    ...
}
```

**Impact**:
- Authentication middleware runs in **dev mode** (accepts test tokens)
- Real production error paths **are not executed**
- Error injection would require **test-specific production code**

---

### Authentication Middleware Analysis

**File**: `holmesgpt-api/src/middleware/auth.py`

**Error Paths in Production**:

#### 1️⃣ 401 Unauthorized (`auth.py:213-216`)
```python
if not auth_header.startswith("Bearer "):
    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="No valid authentication credentials provided"
    )
```

**Triggers**:
- Missing `Authorization` header
- Invalid token format (not "Bearer ...")
- Token validation failure (`auth.py:250-253`)

**Integration Test Reality**:
- `dev_mode=True` → Accepts test tokens like `"test-token-user-role"` (`auth.py:232-242`)
- `auth_enabled=False` → Middleware may not even be active
- Would need to **modify production code** to reject test tokens

---

#### 2️⃣ 403 Forbidden (`auth.py:177-180`)
```python
if not self._check_permissions(user, request):
    raise HTTPException(
        status_code=status.HTTP_403_FORBIDDEN,
        detail="Insufficient permissions"
    )
```

**Production Implementation** (`auth.py:396-404`):
```python
def _check_permissions(self, user: User, request: Request) -> bool:
    # Minimal permission check for internal service
    # All authenticated users can call investigation endpoints
    return True  # K8s RBAC handles authorization at network level
```

**Integration Test Reality**:
- Permission check **always returns True** in current implementation
- Triggering 403 would require **modifying production permission logic**
- NOT a realistic test scenario (K8s RBAC is network-level)

---

#### 3️⃣ 503 Service Unavailable (`auth.py:371-373`)
```python
raise HTTPException(
    status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
    detail="Cannot reach Kubernetes API: {str(e)}"
)
```

**Triggers**:
- Kubernetes TokenReviewer API unreachable
- Network connectivity issues

**Integration Test Reality**:
- `dev_mode=True` → Doesn't call K8s API at all
- Would require **breaking K8s API** in integration environment
- NOT practical or safe for integration tests

---

#### 4️⃣ 500 Internal Server Error (`auth.py:194-200`)
```python
except Exception as e:
    logger.error({"event": "auth_error", "error": str(e)})
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={"detail": "Internal server error"}
    )
```

**Triggers**:
- Unexpected exceptions in middleware

**Integration Test Reality**:
- Would require **injecting bugs** into production code
- NOT a valid test approach

---

#### 5️⃣ Timeout / Context Deadline Exceeded
**Triggers**:
- Client-side timeout configuration
- Network latency

**Integration Test Reality**:
- ✅ **CAN be tested** by configuring short timeout in Go client
- Example: `Timeout: 1 * time.Nanosecond` (effectively instant timeout)
- **Already demonstrated** in `test/integration/aianalysis/recovery_integration_test.go:380`

---

## 🎯 RECOMMENDATION: Move to Unit Tests

### Why Unit Tests Are Better for Error Classification

| Aspect | Integration Tests | Unit Tests |
|--------|-------------------|------------|
| **Error Injection** | ❌ Requires production code changes | ✅ Simple mock configuration |
| **Isolation** | ⚠️ Tests real HAPI (dev mode) | ✅ Tests AIAnalysis error classification logic |
| **Speed** | ❌ Slow (infra startup ~133s) | ✅ Fast (no infrastructure) |
| **Reliability** | ⚠️ Depends on HAPI configuration | ✅ Deterministic mock responses |
| **Coverage** | ❌ Limited error scenarios | ✅ Comprehensive error coverage |
| **Maintainability** | ❌ Coupled to HAPI implementation | ✅ Independent of HAPI changes |

---

### Unit Test Pattern for Error Classification

**File**: `test/unit/aianalysis/error_classifier_test.go` (to be created)

**Pattern**:
```go
var _ = Describe("ErrorClassifier", func() {
    var (
        mockHAPI       *testutil.MockHolmesGPTClient
        errorClassifier *handlers.ErrorClassifier
        ctx            context.Context
    )

    BeforeEach(func() {
        mockHAPI = testutil.NewMockHolmesGPTClient()
        errorClassifier = handlers.NewErrorClassifier(mockHAPI, logger)
        ctx = context.Background()
    })

    Context("HTTP Error Classification - BR-AI-009", func() {
        It("should classify 401 as Authentication Error", func() {
            // Configure mock to return 401 error
            mockHAPI.WithError(&client.APIError{
                StatusCode: 401,
                Message:    "Unauthorized",
            })

            // Call business logic
            result, err := errorClassifier.ClassifyError(ctx, testRequest)

            // Verify classification
            Expect(err).ToNot(HaveOccurred())
            Expect(result.ErrorType).To(Equal("Authentication"))
            Expect(result.IsRetryable).To(BeFalse())
            Expect(result.ShouldAlert).To(BeTrue())
        })

        It("should classify 503 as Transient Error", func() {
            mockHAPI.WithError(&client.APIError{
                StatusCode: 503,
                Message:    "Service Unavailable",
            })

            result, err := errorClassifier.ClassifyError(ctx, testRequest)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.ErrorType).To(Equal("Transient"))
            Expect(result.IsRetryable).To(BeTrue())
            Expect(result.ShouldAlert).To(BeFalse())
        })

        It("should classify context.DeadlineExceeded as Timeout", func() {
            mockHAPI.WithError(context.DeadlineExceeded)

            result, err := errorClassifier.ClassifyError(ctx, testRequest)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.ErrorType).To(Equal("Timeout"))
            Expect(result.IsRetryable).To(BeTrue())
            Expect(result.ShouldAlert).To(BeFalse())
        })
    })
})
```

**Benefits**:
- ✅ **Full control** over error scenarios
- ✅ **Fast execution** (no infrastructure)
- ✅ **Deterministic** results
- ✅ **No production code modification** required
- ✅ **Follows user mandate**: "no test logic in production code"

---

## 📊 REVISED TEST PLAN ALLOCATION

### Phase 1: Error Classification (16 tests) → **UNIT TESTS**

| Test ID | Original Tier | New Tier | Rationale |
|---------|---------------|----------|-----------|
| AA-INT-ERR-001 | Integration | **Unit** | Mock 401 error |
| AA-INT-ERR-002 to 007 | Integration | **Unit** | Mock 4xx errors |
| AA-INT-ERR-008 to 011 | Integration | **Unit** | Mock 5xx errors |
| AA-INT-ERR-012 | Integration | **Unit** | Mock timeout error |
| AA-INT-ERR-013 to 016 | Integration | **Unit** | Mock network errors |

**Result**: **ALL 16 error classification tests move to unit tier**.

---

### Phase 2: Retry/Backoff Logic (13 tests) → **INTEGRATION TESTS**

| Test Category | Test Count | Tier | Rationale |
|---------------|------------|------|-----------|
| **Exponential Backoff** | 5 tests | Integration | Tests real backoff implementation |
| **Max Retries** | 4 tests | Integration | Tests real retry exhaustion |
| **Success After Retry** | 2 tests | Integration | Tests real recovery |
| **Non-Retryable** | 2 tests | Integration | Tests real no-retry logic |

**Justification for Integration**:
- Tests **real backoff timing** (not just classification)
- Verifies **controller reconciliation behavior** under transient failures
- Uses real HAPI with **simulated transient failures** (timeout via short client timeout)
- **Does NOT require production code modification**

---

### Phase 3: Controller Edge Cases (8 tests) → **INTEGRATION TESTS**

| Test Category | Tier | Rationale |
|---------------|------|-----------|
| **Nil Pointer Safety** | Integration | Tests real CRD handling |
| **Empty Field Handling** | Integration | Tests real validation |
| **Concurrent Reconciliation** | Integration | Tests real K8s controller behavior |

**Justification for Integration**:
- Tests **real controller reconciliation loop**
- Verifies **K8s API interactions**
- Uses **envtest** for realistic K8s behavior

---

### Phase 4: V1.0 Maturity (8 tests) → **INTEGRATION TESTS**

| Test Category | Tier | Rationale |
|---------------|------|-----------|
| **Metrics Verification** | Integration | Tests real metrics infrastructure |
| **Audit Verification** | Integration | Tests real DataStorage integration |
| **Graceful Shutdown** | Integration | Tests real lifecycle management |

**Justification for Integration**:
- Tests **real infrastructure integration**
- Verifies **cross-service behavior**

---

## 📋 UPDATED TEST METRICS

### Test Count by Tier

| Tier | Original Plan | Revised Plan | Change |
|------|--------------|--------------|--------|
| **Unit Tests** | 0 | **+16** | +16 error classification tests |
| **Integration Tests** | 45 | **29** | -16 error classification tests |
| **E2E Tests** | 0 | 0 | No change |
| **TOTAL** | 45 | 45 | Same total count |

---

### Coverage Targets

| Tier | Target | Expected with Revised Plan |
|------|--------|---------------------------|
| **Unit** | 70%+ | **75-80%** (adds error_classifier.go coverage) |
| **Integration** | >50% | **55-60%** (focused on real integration scenarios) |
| **E2E** | 10-15% | N/A (no E2E tests planned) |

**Impact**: ✅ **Improves unit coverage** while maintaining integration coverage focus on real infrastructure interactions.

---

## 🚀 IMPLEMENTATION PLAN

### Step 1: Implement `error_classifier.go` (Production Code)

**File**: `pkg/aianalysis/handlers/error_classifier.go`

**Business Requirements**:
- BR-AI-009: Error classification and handling
- BR-AI-010: Retry logic for transient failures

**Functions**:
```go
// ClassifyError determines error type and retry strategy
func (c *ErrorClassifier) ClassifyError(err error) ErrorClassification

// IsRetryable checks if error should be retried
func (c *ErrorClassifier) IsRetryable(classification ErrorClassification) bool

// GetRetryDelay calculates exponential backoff delay
func (c *ErrorClassifier) GetRetryDelay(attemptCount int) time.Duration
```

---

### Step 2: Implement Unit Tests

**File**: `test/unit/aianalysis/error_classifier_test.go`

**Test Count**: 16 tests (Phase 1 error classification)

**Test Structure**:
```
ErrorClassifier
├── HTTP Error Classification (12 tests)
│   ├── 401 Unauthorized → Authentication Error
│   ├── 403 Forbidden → Authorization Error
│   ├── 404 Not Found → Configuration Error
│   ├── 429 Too Many Requests → Rate Limit Error
│   ├── 500 Internal Server Error → Transient Error
│   ├── 502 Bad Gateway → Transient Error
│   ├── 503 Service Unavailable → Transient Error
│   └── 504 Gateway Timeout → Transient Error
├── Network Error Classification (3 tests)
│   ├── context.DeadlineExceeded → Timeout
│   ├── connection refused → Network Error
│   └── DNS resolution failure → Network Error
└── Retry Strategy (1 test)
    └── GetRetryDelay exponential backoff calculation
```

---

### Step 3: Update Integration Tests (Phase 2-4)

**Files**:
- `test/integration/aianalysis/retry_logic_test.go` (13 tests)
- `test/integration/aianalysis/controller_edge_cases_test.go` (8 tests)
- `test/integration/aianalysis/v1_maturity_test.go` (8 tests)

**Timeout Test Pattern** (already working):
```go
// Create client with very short timeout
shortClient := client.NewHolmesGPTClient(client.Config{
    BaseURL: hapiURL,
    Timeout: 1 * time.Nanosecond, // Instant timeout
})

_, err := shortClient.Investigate(ctx, request)
Expect(err).To(MatchError(context.DeadlineExceeded))
```

---

## ✅ APPROVAL CHECKLIST

**User Requirements**:
- [x] No test logic in production code ✅
- [x] Error classification tests at appropriate tier ✅
- [x] Integration tests focus on real infrastructure ✅
- [x] Maintains test count (45 tests total) ✅

**Technical Requirements**:
- [x] Unit tests provide full error scenario coverage ✅
- [x] Integration tests verify real retry/backoff behavior ✅
- [x] No HAPI production code modification required ✅
- [x] Follows TDD methodology (tests define error_classifier.go interface) ✅

---

## 📚 REFERENCES

### Code Files Analyzed
- `holmesgpt-api/src/middleware/auth.py` (authentication error paths)
- `holmesgpt-api/src/main.py` (dev_mode configuration)
- `test/integration/aianalysis/podman-compose.yml` (HAPI integration config)
- `test/integration/aianalysis/recovery_integration_test.go` (timeout test pattern)
- `test/unit/aianalysis/holmesgpt_client_test.go` (mock client usage pattern)

### Design Decisions
- DD-TEST-002: Sequential startup pattern (not affected by this change)
- DD-HOLMESGPT-012: Minimal internal service architecture (explains auth simplicity)

### Business Requirements
- BR-AI-009: Error classification and handling
- BR-AI-010: Retry logic for transient failures
- BR-HAPI-212: Mock LLM mode for integration testing (already implemented)

---

## 🎯 NEXT STEPS

1. ✅ **User Approval**: Confirm revised test allocation (unit vs integration)
2. ⏭️ **Implement error_classifier.go**: TDD RED phase (define interfaces)
3. ⏭️ **Implement unit tests**: TDD GREEN phase (16 error classification tests)
4. ⏭️ **Implement integration tests**: Phase 2-4 (29 tests with real infrastructure)

---

**Assessment Complete**: December 24, 2025
**Recommendation**: ✅ Move error classification tests to unit tier
**Status**: Awaiting user approval to proceed with implementation

---

## 🔍 APPENDIX: Mock HAPI Client Pattern

For reference, here's how the existing mock HAPI client works:

**File**: `pkg/testutil/mock_holmesgpt_client.go`

```go
// MockHolmesGPTClient for unit tests
type MockHolmesGPTClient struct {
    investigateFunc func(context.Context, *client.IncidentRequest) (*client.IncidentResponse, error)
    reanAnalyzeFunc func(context.Context, *client.RecoveryRequest) (*client.IncidentResponse, error)

    // For error injection
    nextError error
}

// WithError configures the mock to return a specific error
func (m *MockHolmesGPTClient) WithError(err error) *MockHolmesGPTClient {
    m.nextError = err
    return m
}

// Investigate returns mock response or configured error
func (m *MockHolmesGPTClient) Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error) {
    if m.nextError != nil {
        err := m.nextError
        m.nextError = nil // Reset for next call
        return nil, err
    }

    if m.investigateFunc != nil {
        return m.investigateFunc(ctx, req)
    }

    // Default mock response
    return &client.IncidentResponse{...}, nil
}
```

**Usage in Unit Tests**:
```go
mockHAPI := testutil.NewMockHolmesGPTClient()

// Test 401 error
mockHAPI.WithError(&client.APIError{StatusCode: 401, Message: "Unauthorized"})
_, err := mockHAPI.Investigate(ctx, req)
// Error classification logic is tested here

// Test 503 error
mockHAPI.WithError(&client.APIError{StatusCode: 503, Message: "Service Unavailable"})
_, err := mockHAPI.Investigate(ctx, req)
// Error classification logic is tested here
```

This pattern provides **full control** over error scenarios without modifying production code.









