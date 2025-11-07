# Gateway Service - Implementation Plan v2.26

✅ **K8S API RETRY LOGIC** - Resilience Enhancement (Gap-Resolved)

**Service**: Gateway Service (Entry Point for All Signals)
**Phase**: Phase 2, Service #1
**Plan Version**: v2.26 (K8s API Retry Logic - Gap Resolution from Context API Lessons)
**Template Version**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.0
**Plan Date**: November 7, 2025
**Current Status**: 🚀 V2.26 K8S API RETRY LOGIC PLANNED (87% Confidence - Gap-Resolved, Ready for Implementation)
**Business Requirements**: BR-GATEWAY-001 through BR-GATEWAY-115 (~55 BRs, +5 new retry BRs)
**Scope**: Prometheus AlertManager + Kubernetes Events + HTTP Server + Observability + Network-Level Security + E2E Edge Cases + **K8s API Retry Logic with Graceful Shutdown Integration**
**Confidence**: 87% ✅ **Implementation Plan Complete - Gap-Resolved, Ready to Start**

**Architecture**: Adapter-specific self-registered endpoints (DD-GATEWAY-001)
**Security**: Network Policies + TLS + Rate Limiting + Security Headers + Log Sanitization + Timestamp Validation (DD-GATEWAY-004)
**Optimization**: Lightweight metadata storage (DD-GATEWAY-004 Redis)
**Resilience**: K8s API retry with exponential backoff (DD-GATEWAY-008) **NEW**

---

## 📋 Version History

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v2.24** | Nov 6, 2025 | E2E Edge Cases + Production-Like Infrastructure | ⚠️ SUPERSEDED |
| **v2.25** | Nov 7, 2025 | **K8s API Retry Logic**: Added comprehensive retry logic for transient K8s API errors (429 rate limiting, 503 service unavailable, timeouts). **Phased Implementation**: Phase 1 (Synchronous Retry with Exponential Backoff, 10h) + Phase 2 (Async Retry Queue, 12h incremental). **New BRs**: BR-GATEWAY-111 (Retry Configuration), BR-GATEWAY-112 (Error Classification), BR-GATEWAY-113 (Exponential Backoff), BR-GATEWAY-114 (Retry Metrics), BR-GATEWAY-115 (Async Retry Queue). **New Files**: `pkg/gateway/processing/errors.go` (error classification), `pkg/gateway/processing/retry_queue.go` (async queue, Phase 2 only). **Config Changes**: Added `RetrySettings` to `ProcessingSettings` with 6 fields (max_attempts, initial_backoff, max_backoff, retry_on_429, retry_on_503, retry_on_timeout). **Test Coverage**: +23 unit tests (retry logic, backoff timing, error classification), +8 integration tests (real K8s API simulation). **Design Decision**: [DD-GATEWAY-008](../../architecture/decisions/DD-GATEWAY-008-k8s-api-retry-strategy.md) - Comprehensive analysis with phased approach, performance metrics, rollback plan. **Timeline**: Phase 1 (Day 14, 10h) + Phase 2 (Day 15, 12h, optional). **Confidence**: 90% (comprehensive plan, clear implementation path). | ✅ **CURRENT** |

---

## 🎯 **v2.25 Feature Overview: K8s API Retry Logic**

### **Business Problem**

**Current State**: Gateway loses alerts when Kubernetes API returns transient errors:
- **HTTP 429 (Too Many Requests)**: API rate limiting during alert storms
- **HTTP 503 (Service Unavailable)**: API server temporarily unavailable
- **Timeout Errors**: Network latency or API server overload

**Impact**:
- **Alert Loss**: 5-10% of alerts lost during K8s API rate limiting (production estimate)
- **Incomplete Remediation**: CRDs not created → remediation workflows never triggered
- **Business Risk**: Critical incidents (P0/P1) may go unaddressed

**Solution**: Implement retry logic with exponential backoff to handle transient K8s API errors gracefully.

---

### **Feature Scope**

#### **Phase 1: Synchronous Retry (MVP)** - **10 hours** (Day 14)

**Goal**: Prevent alert loss on transient K8s API errors

**Deliverables**:
- ✅ Retry logic with exponential backoff
- ✅ Configurable max retries and backoff
- ✅ Error classification (retryable vs non-retryable)
- ✅ Metrics for retry attempts
- ✅ Unit and integration tests (31 tests total)
- ✅ Documentation (API spec, config examples)

**Limitations**:
- ⚠️ Blocks HTTP request during retries (could timeout AlertManager webhook)
- ⚠️ Limited to ~3 retries (to avoid webhook timeout)
- ⚠️ No persistence across Gateway restarts

**Risk**: **Low** - Straightforward implementation, minimal dependencies

---

#### **Phase 2: Async Retry Queue (Production-Grade)** - **12 hours** (Day 15, optional)

**Goal**: Production-grade retry with persistence and scalability

**Deliverables**:
- ✅ Async retry queue with worker pool
- ✅ Redis persistence for restart recovery
- ✅ Advanced metrics (queue depth, latency)
- ✅ E2E tests for failure scenarios
- ✅ Operational documentation

**Benefits**:
- ✅ Non-blocking (returns HTTP 202 immediately)
- ✅ Unlimited retries (configurable)
- ✅ Survives Gateway restarts
- ✅ Scales to high load

**Risk**: **Medium** - More complex, requires Redis integration testing

---

### **New Business Requirements**

| BR ID | Description | Phase | Tests |
|-------|-------------|-------|-------|
| **BR-GATEWAY-111** | Retry Configuration: Configurable max attempts, backoff durations, error type toggles | Phase 1 | 3 unit |
| **BR-GATEWAY-112** | Error Classification: Distinguish retryable (429, 503, timeout) from non-retryable (400, 403, 422) errors | Phase 1 | 8 unit |
| **BR-GATEWAY-113** | Exponential Backoff: Implement exponential backoff with configurable initial/max durations | Phase 1 | 6 unit |
| **BR-GATEWAY-114** | Retry Metrics: Expose Prometheus metrics for retry attempts, success rates, exhaustion | Phase 1 | 6 unit + 4 integration |
| **BR-GATEWAY-115** | Async Retry Queue: Persist retry items to Redis, worker pool for async processing | Phase 2 | 8 unit + 4 integration |

**Total New BRs**: 5 (BR-GATEWAY-111 to BR-GATEWAY-115)
**Total Gateway BRs**: 115 (was 110)

---

### **Configuration Changes**

#### **New Config Section: `processing.retry`**

```yaml
processing:
  retry:
    # Maximum number of retry attempts for transient K8s API errors
    max_attempts: 3  # Default: 3 (Phase 1), 10 (Phase 2)

    # Initial backoff duration (doubles with each retry)
    initial_backoff: 1s  # Default: 1s

    # Maximum backoff duration (cap for exponential backoff)
    max_backoff: 10s  # Default: 10s (Phase 1), 60s (Phase 2)

    # Enable retry for specific error types
    retry_on_429: true  # HTTP 429 - Too Many Requests (rate limiting)
    retry_on_503: true  # HTTP 503 - Service Unavailable
    retry_on_timeout: true  # Timeout errors (network latency, API overload)

    # Phase 2: Async retry queue settings (optional)
    async_enabled: false  # Enable async retry queue (default: false)
    queue_size: 1000  # Max retry queue size (default: 1000)
    worker_count: 5  # Number of retry workers (default: 5)
    redis_persistence: true  # Persist retry items to Redis (default: true)
```

---

### **Implementation Timeline**

| Phase | Duration | Deliverables | Confidence |
|-------|----------|--------------|------------|
| **Phase 1: Synchronous Retry** | 10h (Day 14) | Config schema, error classification, retry logic, metrics, tests (31), docs | 88% |
| **Phase 2: Async Retry Queue** | 12h (Day 15) | Async queue, Redis persistence, worker pool, E2E tests, ops docs | 83% |

**Total Effort**: 10-22 hours (1.25-2.75 days) depending on phase

---

## 📅 **Day 14: K8s API Retry Logic (Phase 1 - Synchronous Retry)** (REVISED)

**Objective**: Implement synchronous retry logic with exponential backoff to prevent alert loss on transient K8s API errors

**Duration**: 14 hours (1.75 days) - **REVISED from 10h** (+4h for gap resolution)

**Business Requirements**: BR-GATEWAY-111, BR-GATEWAY-112, BR-GATEWAY-113, BR-GATEWAY-114

**Confidence**: 85% - **REVISED from 88%** (more realistic with lessons learned)

**Gaps Addressed**: GAP 1-3, GAP 5-6, GAP 8-12 (10 gaps from triage analysis)

---

### **⚠️ CRITICAL: TDD Methodology Change** (GAP 1)

**Context API Lesson Learned**: Batch test writing violates TDD methodology

**ANTI-PATTERN** (Original Plan):
```
RED Phase (2h): Write all 23 unit tests upfront with Skip()
GREEN Phase (3h): Activate tests in batches
```

**CORRECT APPROACH** (Revised Plan):
```
RED Phase (3h): Write 1-2 tests at a time, implement immediately
  Iteration 1: Write 1 test → Implement → Refactor
  Iteration 2: Write 1 test → Implement → Refactor
  ... (repeat for all 23 tests)
```

**Impact**: +1 hour (2h → 3h) for proper TDD methodology

**Reference**: `BATCH_ACTIVATION_ANTI_PATTERN.md` - Context API deleted 43 tests and rewrote with pure TDD

---

### **APDC Analysis Phase** (1 hour)

#### **Business Context**
- **Problem**: Gateway loses 5-10% of alerts during K8s API rate limiting (HTTP 429)
- **Impact**: Critical incidents (P0/P1) may go unaddressed if CRDs are not created
- **Solution**: Retry transient errors with exponential backoff

#### **Technical Context**
- **Current Implementation**: `pkg/gateway/k8s/client.go` has no retry logic
- **K8s API Errors**: 429 (rate limiting), 503 (unavailable), timeout (network/overload)
- **Retry Strategy**: Exponential backoff (1s → 2s → 4s → 8s, capped at 10s)

#### **Integration Context**
- **CRD Creator**: `pkg/gateway/processing/crd_creator.go` calls `k8sClient.CreateRemediationRequest()`
- **Config Loading**: `pkg/gateway/config/config.go` loads `ServerConfig`
- **Metrics**: `pkg/gateway/metrics/metrics.go` exposes Prometheus metrics

#### **Risk Assessment**
- **Webhook Timeout**: AlertManager webhook may timeout if retry takes >30s (mitigated by max 3 retries)
- **Increased Latency**: p95 latency may increase from 50ms to 3s during retries (acceptable tradeoff)
- **Retry Storms**: Exponential backoff prevents thundering herd

---

### **APDC Plan Phase** (1 hour)

#### **TDD Strategy** (REVISED - GAP 1)
1. **RED Phase** (3h): Write 1-2 tests at a time, implement immediately (iterative TDD)
2. **GREEN Phase** (3h): Implement minimal retry logic in `crd_creator.go`
3. **REFACTOR Phase** (1h): Extract error classification to `errors.go`, improve logging
4. **Graceful Shutdown Phase** (2h): Integrate with DD-007 shutdown pattern (GAP 3)
5. **Configuration Validation Phase** (1h): Add config validation + tests (GAP 8)

#### **Integration Plan**
- **Config**: Add `RetrySettings` to `ProcessingSettings` in `pkg/gateway/config/config.go`
- **Config Validation** (GAP 8): Add `Validate()` method for retry settings
- **CRD Creator**: Update `NewCRDCreator()` to accept `retryConfig` parameter
- **Server**: Wire retry config in `pkg/gateway/server.go`
- **Graceful Shutdown** (GAP 3): Integrate retry logic with DD-007 4-step shutdown

#### **Success Criteria**
- ✅ All 26 unit tests pass (23 retry + 3 config validation) (GAP 8)
- ✅ All 11 integration tests pass (8 retry + 3 context timeout) (GAP 2, GAP 6)
- ✅ All 3 graceful shutdown tests pass (GAP 3)
- ✅ Metrics expose retry attempts, success rates, exhaustion
- ✅ Configuration validation prevents invalid values (GAP 8)
- ✅ Graceful shutdown completes in-flight retries (GAP 3)
- ✅ Test cleanup prevents resource leaks (GAP 5)
- ✅ Configuration documented in API spec

#### **Timeline** (REVISED)
- **Analysis**: 1h
- **Plan**: 1h
- **RED**: 3h (26 unit tests, iterative TDD) - **+1h** (GAP 1)
- **GREEN**: 3h (retry logic implementation)
- **REFACTOR**: 1h (error classification extraction)
- **Graceful Shutdown Integration**: 2h (DD-007 integration) - **+2h** (GAP 3)
- **Configuration Validation**: 1h (validation + tests) - **+1h** (GAP 8)
- **Integration Tests**: 2h (11 tests) - **+30min** (GAP 2, GAP 6)
- **Documentation**: 1.5h (API spec, config examples, runbook) - **+30min** (GAP 12)
- **Total**: 14.5h (~14h) - **+4h from original 10h**

---

### **APDC Do Phase - RED** (2 hours)

#### **Test File**: `pkg/gateway/processing/crd_creator_retry_test.go`

**Test Coverage** (23 unit tests):

1. **Retryable Errors** (6 tests):
   - Retry on HTTP 429 and succeed on 2nd attempt
   - Retry on HTTP 503 and succeed on 3rd attempt
   - Retry on timeout error and succeed
   - Exhaust retries after max attempts (all 429)
   - Exhaust retries after max attempts (all 503)
   - Exhaust retries after max attempts (all timeout)

2. **Non-Retryable Errors** (5 tests):
   - Do NOT retry on HTTP 400 (validation error)
   - Do NOT retry on HTTP 403 (RBAC error)
   - Do NOT retry on HTTP 422 (schema validation)
   - Do NOT retry on HTTP 409 (already exists)
   - Do NOT retry on HTTP 404 (namespace not found)

3. **Exponential Backoff** (4 tests):
   - Verify backoff timing: 1s → 2s → 4s
   - Verify backoff cap at max_backoff (10s)
   - Verify backoff resets on success
   - Verify backoff with context cancellation

4. **Error Classification** (5 tests):
   - Classify HTTP 429 as retryable
   - Classify HTTP 503 as retryable
   - Classify timeout as retryable
   - Classify HTTP 400 as non-retryable
   - Classify HTTP 403 as non-retryable

5. **Metrics** (3 tests):
   - Increment retry attempt counter on each retry
   - Increment retry success counter on eventual success
   - Increment retry exhausted counter on max attempts

**Example Test** (Ginkgo/Gomega):

```go
var _ = Describe("CRDCreator Retry Logic", func() {
    var (
        mockK8sClient *MockK8sClient
        creator       *CRDCreator
        retryConfig   *config.RetrySettings
    )

    BeforeEach(func() {
        mockK8sClient = NewMockK8sClient()
        retryConfig = &config.RetrySettings{
            MaxAttempts:    3,
            InitialBackoff: 10 * time.Millisecond,  // Fast for tests
            MaxBackoff:     50 * time.Millisecond,
            RetryOn429:     true,
            RetryOn503:     true,
            RetryOnTimeout: true,
        }
        creator = NewCRDCreator(mockK8sClient, logger, metrics, "test-ns", retryConfig)
    })

    Context("Retryable Errors", func() {
        It("should retry on HTTP 429 and succeed on 2nd attempt", func() {
            // BR-GATEWAY-112: Error Classification
            // BR-GATEWAY-113: Exponential Backoff
            // BR-GATEWAY-114: Retry Metrics

            // First attempt fails with 429, second succeeds
            mockK8sClient.SetResponses(
                apierrors.NewTooManyRequests("rate limited", 1),
                nil,  // Success
            )

            signal := &types.NormalizedSignal{
                Fingerprint: "test-fingerprint",
                AlertName:   "TestAlert",
                Severity:    "critical",
                Namespace:   "production",
            }

            rr, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

            Expect(err).ToNot(HaveOccurred())
            Expect(rr).ToNot(BeNil())
            Expect(mockK8sClient.CreateCallCount()).To(Equal(2))

            // Verify metrics
            Expect(testutil.GetCounterValue(metrics.CRDRetryAttempts, "attempt_1")).To(Equal(1.0))
            Expect(testutil.GetCounterValue(metrics.CRDRetrySuccess, "attempt_2")).To(Equal(1.0))
        })

        It("should exhaust retries and fail after max attempts", func() {
            // BR-GATEWAY-113: Exponential Backoff
            // BR-GATEWAY-114: Retry Metrics (exhausted counter)

            // All attempts fail with 429
            mockK8sClient.SetResponses(
                apierrors.NewTooManyRequests("rate limited", 1),
                apierrors.NewTooManyRequests("rate limited", 1),
                apierrors.NewTooManyRequests("rate limited", 1),
            )

            signal := &types.NormalizedSignal{/* ... */}
            _, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("failed after 3 retries"))
            Expect(mockK8sClient.CreateCallCount()).To(Equal(3))

            // Verify exhausted metric
            Expect(testutil.GetCounterValue(metrics.CRDRetryExhausted)).To(Equal(1.0))
        })
    })

    Context("Non-Retryable Errors", func() {
        It("should NOT retry on HTTP 400 (validation error)", func() {
            // BR-GATEWAY-112: Error Classification (non-retryable)

            mockK8sClient.SetResponses(
                apierrors.NewBadRequest("invalid CRD"),
            )

            signal := &types.NormalizedSignal{/* ... */}
            _, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

            Expect(err).To(HaveOccurred())
            Expect(mockK8sClient.CreateCallCount()).To(Equal(1))  // No retry
        })
    })

    Context("Exponential Backoff", func() {
        It("should use exponential backoff between retries", func() {
            // BR-GATEWAY-113: Exponential Backoff timing

            mockK8sClient.SetResponses(
                apierrors.NewTooManyRequests("rate limited", 1),
                apierrors.NewTooManyRequests("rate limited", 1),
                nil,  // Success
            )

            start := time.Now()
            signal := &types.NormalizedSignal{/* ... */}
            _, err := creator.CreateRemediationRequest(ctx, signal, "prod", "P0")

            Expect(err).ToNot(HaveOccurred())

            // Verify backoff timing: 10ms + 20ms = 30ms minimum
            elapsed := time.Since(start)
            Expect(elapsed).To(BeNumerically(">=", 30*time.Millisecond))
            Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond))
        })
    })
})
```

---

### **APDC Do Phase - GREEN** (3 hours)

#### **Step 1: Config Schema** (30 minutes)

**File**: `pkg/gateway/config/config.go`

```go
type ProcessingSettings struct {
    Deduplication DeduplicationSettings `yaml:"deduplication"`
    Storm         StormSettings         `yaml:"storm"`
    Environment   EnvironmentSettings   `yaml:"environment"`
    Priority      PrioritySettings      `yaml:"priority"`
    CRD           CRDSettings           `yaml:"crd"`
    Retry         RetrySettings         `yaml:"retry"`  // NEW
}

// RetrySettings configures retry behavior for transient K8s API errors
// BR-GATEWAY-111: Retry Configuration
type RetrySettings struct {
    // Maximum number of retry attempts for transient K8s API errors
    // Default: 3 (Phase 1), 10 (Phase 2 with async queue)
    MaxAttempts int `yaml:"max_attempts"`

    // Initial backoff duration (doubles with each retry)
    // Example: 1s → 2s → 4s → 8s (exponential backoff)
    // Default: 1s
    InitialBackoff time.Duration `yaml:"initial_backoff"`

    // Maximum backoff duration (cap for exponential backoff)
    // Prevents excessive wait times during retry storms
    // Default: 10s (Phase 1), 60s (Phase 2)
    MaxBackoff time.Duration `yaml:"max_backoff"`

    // Enable retry for HTTP 429 (Too Many Requests - rate limiting)
    // Default: true
    RetryOn429 bool `yaml:"retry_on_429"`

    // Enable retry for HTTP 503 (Service Unavailable)
    // Default: true
    RetryOn503 bool `yaml:"retry_on_503"`

    // Enable retry for timeout errors (network latency, API overload)
    // Default: true
    RetryOnTimeout bool `yaml:"retry_on_timeout"`
}

// DefaultRetrySettings returns sensible defaults for Phase 1 (synchronous retry)
func DefaultRetrySettings() RetrySettings {
    return RetrySettings{
        MaxAttempts:    3,
        InitialBackoff: time.Second,
        MaxBackoff:     10 * time.Second,
        RetryOn429:     true,
        RetryOn503:     true,
        RetryOnTimeout: true,
    }
}
```

---

#### **Step 2: Error Classification** (1 hour)

**File**: `pkg/gateway/processing/errors.go` (NEW)

```go
package processing

import (
    "errors"
    "net/http"
    "strings"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsRetryableError determines if a K8s API error should be retried
// BR-GATEWAY-112: Error Classification
func IsRetryableError(err error, config *RetrySettings) bool {
    if err == nil {
        return false
    }

    // Check for K8s API status errors
    var statusErr *apierrors.StatusError
    if errors.As(err, &statusErr) {
        status := statusErr.ErrStatus.Code

        // HTTP 429 - Too Many Requests (rate limiting)
        if status == http.StatusTooManyRequests && config.RetryOn429 {
            return true
        }

        // HTTP 503 - Service Unavailable
        if status == http.StatusServiceUnavailable && config.RetryOn503 {
            return true
        }

        // HTTP 504 - Gateway Timeout
        if status == http.StatusGatewayTimeout && config.RetryOnTimeout {
            return true
        }
    }

    // Check for timeout errors (string matching as fallback)
    errStr := err.Error()
    if config.RetryOnTimeout && (strings.Contains(errStr, "timeout") ||
                                  strings.Contains(errStr, "deadline exceeded")) {
        return true
    }

    // Check for temporary network errors
    if strings.Contains(errStr, "connection refused") ||
       strings.Contains(errStr, "connection reset") {
        return true
    }

    return false
}

// IsNonRetryableError determines if error should never be retried
// BR-GATEWAY-112: Error Classification (non-retryable errors)
func IsNonRetryableError(err error) bool {
    if err == nil {
        return false
    }

    var statusErr *apierrors.StatusError
    if errors.As(err, &statusErr) {
        status := statusErr.ErrStatus.Code

        // HTTP 400 - Bad Request (validation error)
        if status == http.StatusBadRequest {
            return true
        }

        // HTTP 403 - Forbidden (RBAC error)
        if status == http.StatusForbidden {
            return true
        }

        // HTTP 422 - Unprocessable Entity (schema validation)
        if status == http.StatusUnprocessableEntity {
            return true
        }

        // HTTP 409 - Conflict (already exists)
        if status == http.StatusConflict {
            return true
        }
    }

    return false
}
```

---

#### **Step 3: Retry Logic** (1.5 hours)

**File**: `pkg/gateway/processing/crd_creator.go`

```go
// Add retry config to CRDCreator
type CRDCreator struct {
    k8sClient         *k8s.Client
    logger            *zap.Logger
    metrics           *metrics.Metrics
    fallbackNamespace string
    retryConfig       *config.RetrySettings  // NEW
}

func NewCRDCreator(k8sClient *k8s.Client, logger *zap.Logger, metricsInstance *metrics.Metrics,
                   fallbackNamespace string, retryConfig *config.RetrySettings) *CRDCreator {
    // ... existing validation ...

    // Default retry config if not provided
    if retryConfig == nil {
        defaultConfig := config.DefaultRetrySettings()
        retryConfig = &defaultConfig
    }

    return &CRDCreator{
        k8sClient:         k8sClient,
        logger:            logger,
        metrics:           metricsInstance,
        fallbackNamespace: fallbackNamespace,
        retryConfig:       retryConfig,
    }
}

// createCRDWithRetry implements retry logic with exponential backoff
// BR-GATEWAY-113: Exponential Backoff
// BR-GATEWAY-114: Retry Metrics
func (c *CRDCreator) createCRDWithRetry(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
    backoff := c.retryConfig.InitialBackoff

    for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
        // Attempt CRD creation
        err := c.k8sClient.CreateRemediationRequest(ctx, rr)

        // Success
        if err == nil {
            if attempt > 0 {
                c.metrics.CRDRetrySuccess.WithLabelValues(
                    fmt.Sprintf("attempt_%d", attempt+1),
                ).Inc()
                c.logger.Info("CRD creation succeeded after retry",
                    zap.Int("attempt", attempt+1),
                    zap.String("name", rr.Name),
                    zap.String("namespace", rr.Namespace))
            }
            return nil
        }

        // Non-retryable error (validation, RBAC, etc.)
        if IsNonRetryableError(err) {
            c.logger.Error("CRD creation failed with non-retryable error",
                zap.Error(err),
                zap.String("name", rr.Name))
            return err
        }

        // Check if error is retryable
        if !IsRetryableError(err, c.retryConfig) {
            c.logger.Error("CRD creation failed with non-retryable error",
                zap.Error(err),
                zap.String("name", rr.Name))
            return err
        }

        // Last attempt failed
        if attempt == c.retryConfig.MaxAttempts-1 {
            c.metrics.CRDRetryExhausted.Inc()
            c.logger.Error("CRD creation failed after max retries",
                zap.Int("max_attempts", c.retryConfig.MaxAttempts),
                zap.Error(err),
                zap.String("name", rr.Name))
            return fmt.Errorf("failed after %d retries: %w", c.retryConfig.MaxAttempts, err)
        }

        // Retry with exponential backoff
        c.metrics.CRDRetryAttempts.WithLabelValues(
            fmt.Sprintf("attempt_%d", attempt+1),
        ).Inc()

        c.logger.Warn("CRD creation failed, retrying...",
            zap.Int("attempt", attempt+1),
            zap.Duration("backoff", backoff),
            zap.Error(err),
            zap.String("name", rr.Name))

        // Sleep with backoff
        select {
        case <-time.After(backoff):
            // Continue to next attempt
        case <-ctx.Done():
            return ctx.Err()
        }

        // Exponential backoff (double each time, capped at max)
        backoff *= 2
        if backoff > c.retryConfig.MaxBackoff {
            backoff = c.retryConfig.MaxBackoff
        }
    }

    return fmt.Errorf("retry logic error: should not reach here")
}

// Update CreateRemediationRequest to use retry logic
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal,
                                               environment, priority string) (*remediationv1alpha1.RemediationRequest, error) {
    // ... existing code to build rr ...

    // Create CRD with retry logic
    if err := c.createCRDWithRetry(ctx, rr); err != nil {
        // Handle specific errors (already exists, namespace not found)
        // ... existing error handling ...

        return nil, err
    }

    return rr, nil
}
```

---

#### **Step 4: Metrics** (30 minutes)

**File**: `pkg/gateway/metrics/metrics.go`

```go
type Metrics struct {
    // ... existing metrics ...

    // NEW: Retry metrics (BR-GATEWAY-114)
    CRDRetryAttempts  *prometheus.CounterVec  // Total retry attempts by attempt number
    CRDRetrySuccess   *prometheus.CounterVec  // Successful retries by attempt number
    CRDRetryExhausted prometheus.Counter      // Retries exhausted (max attempts reached)
}

func NewMetrics() *Metrics {
    // ... existing metrics ...

    crdRetryAttempts := prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_crd_retry_attempts_total",
            Help: "Total number of CRD creation retry attempts by attempt number",
        },
        []string{"attempt"},  // attempt_1, attempt_2, attempt_3
    )

    crdRetrySuccess := prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_crd_retry_success_total",
            Help: "Total number of successful CRD creations after retry by attempt number",
        },
        []string{"attempt"},
    )

    crdRetryExhausted := prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "gateway_crd_retry_exhausted_total",
            Help: "Total number of CRD creations that failed after max retry attempts",
        },
    )

    // Register metrics
    prometheus.MustRegister(crdRetryAttempts, crdRetrySuccess, crdRetryExhausted)

    return &Metrics{
        // ... existing metrics ...
        CRDRetryAttempts:  crdRetryAttempts,
        CRDRetrySuccess:   crdRetrySuccess,
        CRDRetryExhausted: crdRetryExhausted,
    }
}
```

---

#### **Step 5: Server Integration** (30 minutes)

**File**: `pkg/gateway/server.go`

```go
func NewServer(cfg *ServerConfig, logger *zap.Logger) (*Server, error) {
    // ... existing code ...

    // Create CRD creator with retry config
    crdCreator := processing.NewCRDCreator(
        k8sClient,
        logger,
        metricsInstance,
        cfg.Processing.CRD.FallbackNamespace,
        &cfg.Processing.Retry,  // NEW: Pass retry config
    )

    // ... rest of server initialization ...
}
```

---

### **APDC Do Phase - REFACTOR** (1 hour)

#### **Refactoring Tasks**

1. **Extract Error Classification** (30 min):
   - Move `IsRetryableError()` and `IsNonRetryableError()` to `errors.go`
   - Add comprehensive error type documentation
   - Add unit tests for edge cases (empty errors, nil config)

2. **Improve Logging** (15 min):
   - Add structured logging fields (attempt number, backoff duration, error type)
   - Log retry exhaustion at ERROR level
   - Log successful retries at INFO level

3. **Code Quality** (15 min):
   - Extract magic numbers to constants (`maxWebhookRetries = 3`)
   - Add function documentation with BR references
   - Improve variable names (`backoffDuration` instead of `backoff`)

---

### **Integration Tests** (1.5 hours)

#### **Test File**: `test/integration/gateway/retry_test.go`

**Test Coverage** (8 integration tests):

1. **Real K8s API Simulation** (4 tests):
   - Simulate HTTP 429 rate limiting with mock K8s API
   - Simulate HTTP 503 service unavailable
   - Simulate timeout errors
   - Verify retry attempts in real K8s cluster (Kind)

2. **End-to-End Retry Flow** (4 tests):
   - Send alert → Gateway → K8s API (429) → Retry → Success
   - Send alert → Gateway → K8s API (503) → Retry → Success
   - Send alert → Gateway → K8s API (timeout) → Retry → Success
   - Send alert → Gateway → K8s API (429 x3) → Exhaust → Fail

**Example Integration Test**:

```go
var _ = Describe("Gateway Retry Integration", func() {
    It("should retry CRD creation on K8s API rate limiting (429)", func() {
        // BR-GATEWAY-112: Error Classification
        // BR-GATEWAY-113: Exponential Backoff
        // BR-GATEWAY-114: Retry Metrics

        // Setup: Configure mock K8s API to return 429 on first attempt, success on second
        mockK8sAPI := testutil.NewMockK8sAPI()
        mockK8sAPI.SetResponses(
            testutil.HTTP429Response("rate limited"),
            testutil.HTTP201Response(),  // Success
        )

        // Send alert to Gateway
        alert := testutil.NewPrometheusAlert("HighCPU", "critical", "production")
        resp, err := httpClient.Post(gatewayURL+"/api/v1/signals/prometheus", "application/json", alert)
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()

        // Verify: Gateway returns 201 (CRD created after retry)
        Expect(resp.StatusCode).To(Equal(http.StatusCreated))

        // Verify: K8s API received 2 requests (1 failure + 1 retry)
        Expect(mockK8sAPI.RequestCount()).To(Equal(2))

        // Verify: Retry metrics incremented
        metrics := testutil.GetGatewayMetrics(gatewayURL + "/metrics")
        Expect(metrics["gateway_crd_retry_attempts_total{attempt=\"1\"}"]).To(Equal(1.0))
        Expect(metrics["gateway_crd_retry_success_total{attempt=\"2\"}"]).To(Equal(1.0))
    })
})
```

---

### **Documentation** (1 hour)

#### **API Specification Update**

**File**: `docs/services/stateless/gateway-service/api-specification.md`

Add new section: **K8s API Retry Strategy**

```markdown
## K8s API Retry Strategy

### Overview

Gateway implements retry logic with exponential backoff to handle transient Kubernetes API errors gracefully. This prevents alert loss during API rate limiting, service unavailability, or network latency.

### Retryable Errors

| Error Type | HTTP Status | Retry Behavior | Business Impact |
|------------|-------------|----------------|-----------------|
| **Rate Limiting** | 429 Too Many Requests | Retry with exponential backoff | Prevents alert loss during alert storms |
| **Service Unavailable** | 503 Service Unavailable | Retry with exponential backoff | Handles temporary API server unavailability |
| **Timeout** | N/A (timeout error) | Retry with exponential backoff | Handles network latency or API overload |

### Non-Retryable Errors

| Error Type | HTTP Status | Behavior | Rationale |
|------------|-------------|----------|-----------|
| **Validation Error** | 400 Bad Request | Fail immediately | Invalid CRD schema, cannot be fixed by retry |
| **RBAC Error** | 403 Forbidden | Fail immediately | Insufficient permissions, requires RBAC fix |
| **Schema Validation** | 422 Unprocessable Entity | Fail immediately | CRD schema mismatch, requires code fix |
| **Already Exists** | 409 Conflict | Fail immediately | CRD already exists (idempotent) |

### Exponential Backoff

Gateway uses exponential backoff to prevent retry storms:

```
Attempt 1: Fail → Wait 1s
Attempt 2: Fail → Wait 2s
Attempt 3: Fail → Wait 4s
Attempt 4: Fail → Wait 8s (capped at max_backoff)
```

### Configuration

```yaml
processing:
  retry:
    max_attempts: 3          # Maximum retry attempts (default: 3)
    initial_backoff: 1s      # Initial backoff duration (default: 1s)
    max_backoff: 10s         # Maximum backoff duration (default: 10s)
    retry_on_429: true       # Retry on HTTP 429 rate limiting (default: true)
    retry_on_503: true       # Retry on HTTP 503 service unavailable (default: true)
    retry_on_timeout: true   # Retry on timeout errors (default: true)
```

### Metrics

Gateway exposes Prometheus metrics for retry behavior:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_crd_retry_attempts_total` | Counter | `attempt` | Total retry attempts by attempt number |
| `gateway_crd_retry_success_total` | Counter | `attempt` | Successful retries by attempt number |
| `gateway_crd_retry_exhausted_total` | Counter | N/A | Retries exhausted (max attempts reached) |

### Example Queries

```promql
# Retry success rate
rate(gateway_crd_retry_success_total[5m]) / rate(gateway_crd_retry_attempts_total[5m])

# Retry exhaustion rate (alert loss)
rate(gateway_crd_retry_exhausted_total[5m])

# Average retry attempts before success
avg(gateway_crd_retry_success_total) by (attempt)
```
```

---

### **Day 14 Deliverables**

- ✅ `pkg/gateway/config/config.go`: Added `RetrySettings` struct (BR-GATEWAY-111)
- ✅ `pkg/gateway/processing/errors.go`: Error classification logic (BR-GATEWAY-112)
- ✅ `pkg/gateway/processing/crd_creator.go`: Retry logic with exponential backoff (BR-GATEWAY-113)
- ✅ `pkg/gateway/metrics/metrics.go`: Retry metrics (BR-GATEWAY-114)
- ✅ `pkg/gateway/server.go`: Wire retry config into CRD creator
- ✅ `pkg/gateway/processing/crd_creator_retry_test.go`: 23 unit tests
- ✅ `test/integration/gateway/retry_test.go`: 8 integration tests
- ✅ `docs/services/stateless/gateway-service/api-specification.md`: Retry strategy documentation

**Total Files**: 5 implementation files + 2 test files + 1 documentation file

---

### **Day 14 Success Criteria**

- ✅ All 23 unit tests pass (100% pass rate)
- ✅ All 8 integration tests pass (100% pass rate)
- ✅ Zero build errors
- ✅ Zero lint errors
- ✅ Retry metrics exposed at `/metrics` endpoint
- ✅ Configuration documented in API spec
- ✅ BR-GATEWAY-111, BR-GATEWAY-112, BR-GATEWAY-113, BR-GATEWAY-114 validated

---

## 📅 **Day 15: Async Retry Queue (Phase 2 - Production-Grade)** (OPTIONAL)

**Objective**: Implement async retry queue with Redis persistence for production-grade retry behavior

**Duration**: 12 hours (1.5 days)

**Business Requirements**: BR-GATEWAY-115

**Confidence**: 83%

**Status**: OPTIONAL - Implement after Phase 1 is validated in production

---

### **APDC Analysis Phase** (2 hours)

#### **Business Context**
- **Problem**: Phase 1 synchronous retry blocks HTTP request (webhook timeout risk)
- **Impact**: AlertManager webhook may timeout if retry takes >30s
- **Solution**: Async retry queue returns HTTP 202 immediately, retries in background

#### **Technical Context**
- **Queue Architecture**: In-memory queue + Redis persistence
- **Worker Pool**: 5 workers process retry items concurrently
- **Persistence**: Retry items stored in Redis for restart recovery

#### **Integration Context**
- **CRD Creator**: Returns HTTP 202 immediately, enqueues retry item
- **Retry Queue**: Background workers process retry items
- **Redis**: Stores retry items for restart recovery

---

### **APDC Plan Phase** (1 hour)

#### **TDD Strategy**
1. **RED Phase** (3h): Write 12 unit tests for retry queue, worker pool, persistence
2. **GREEN Phase** (4h): Implement async retry queue with Redis persistence
3. **REFACTOR Phase** (1h): Extract queue operations, improve error handling

#### **Integration Plan**
- **Config**: Add `async_enabled`, `queue_size`, `worker_count`, `redis_persistence` to `RetrySettings`
- **Retry Queue**: Create `pkg/gateway/processing/retry_queue.go`
- **CRD Creator**: Update to enqueue retry items instead of blocking

#### **Success Criteria**
- ✅ All 12 unit tests pass (queue operations, worker pool, persistence)
- ✅ All 4 integration tests pass (end-to-end async retry flow)
- ✅ All 4 E2E tests pass (Gateway restart recovery)
- ✅ Metrics expose queue depth, retry latency

---

### **APDC Do Phase - RED** (3 hours)

#### **Test File**: `pkg/gateway/processing/retry_queue_test.go`

**Test Coverage** (12 unit tests):

1. **Queue Operations** (4 tests):
   - Enqueue retry item
   - Dequeue retry item
   - Queue size limit enforcement
   - Queue full behavior (drop oldest item)

2. **Worker Pool** (4 tests):
   - Worker processes retry item and succeeds
   - Worker processes retry item and fails (re-enqueue)
   - Worker pool concurrency (5 workers)
   - Worker pool graceful shutdown

3. **Redis Persistence** (4 tests):
   - Persist retry item to Redis
   - Restore retry items from Redis on startup
   - Redis persistence failure (graceful degradation)
   - Redis persistence cleanup on success

---

### **APDC Do Phase - GREEN** (4 hours)

#### **Step 1: Config Schema** (30 minutes)

**File**: `pkg/gateway/config/config.go`

```go
type RetrySettings struct {
    // ... existing Phase 1 fields ...

    // Phase 2: Async retry queue settings
    AsyncEnabled      bool `yaml:"async_enabled"`       // Enable async retry queue (default: false)
    QueueSize         int  `yaml:"queue_size"`          // Max retry queue size (default: 1000)
    WorkerCount       int  `yaml:"worker_count"`        // Number of retry workers (default: 5)
    RedisPersistence  bool `yaml:"redis_persistence"`   // Persist retry items to Redis (default: true)
}
```

---

#### **Step 2: Retry Queue Implementation** (3 hours)

**File**: `pkg/gateway/processing/retry_queue.go` (NEW)

```go
package processing

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/gateway/k8s"
    "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// RetryItem represents a CRD creation retry item
// BR-GATEWAY-115: Async Retry Queue
type RetryItem struct {
    CRD           *remediationv1alpha1.RemediationRequest
    Attempt       int
    NextRetryTime time.Time
    EnqueuedAt    time.Time
}

// RetryQueue manages async retry operations with Redis persistence
// BR-GATEWAY-115: Async Retry Queue
type RetryQueue struct {
    queue         chan *RetryItem
    workers       []*retryWorker
    k8sClient     *k8s.Client
    redisClient   *redis.Client
    logger        *zap.Logger
    metrics       *metrics.Metrics
    config        *RetrySettings
    ctx           context.Context
    cancel        context.CancelFunc
    wg            sync.WaitGroup
}

// NewRetryQueue creates a new async retry queue
func NewRetryQueue(k8sClient *k8s.Client, redisClient *redis.Client, logger *zap.Logger,
                   metricsInstance *metrics.Metrics, config *RetrySettings) *RetryQueue {
    ctx, cancel := context.WithCancel(context.Background())

    rq := &RetryQueue{
        queue:       make(chan *RetryItem, config.QueueSize),
        k8sClient:   k8sClient,
        redisClient: redisClient,
        logger:      logger,
        metrics:     metricsInstance,
        config:      config,
        ctx:         ctx,
        cancel:      cancel,
    }

    // Start worker pool
    rq.workers = make([]*retryWorker, config.WorkerCount)
    for i := 0; i < config.WorkerCount; i++ {
        rq.workers[i] = newRetryWorker(i, rq)
        rq.wg.Add(1)
        go rq.workers[i].run()
    }

    // Restore retry items from Redis on startup
    if config.RedisPersistence {
        go rq.restoreFromRedis()
    }

    return rq
}

// Enqueue adds a retry item to the queue
func (rq *RetryQueue) Enqueue(item *RetryItem) error {
    select {
    case rq.queue <- item:
        rq.metrics.RetryQueueDepth.Inc()

        // Persist to Redis if enabled
        if rq.config.RedisPersistence {
            if err := rq.persistToRedis(item); err != nil {
                rq.logger.Warn("Failed to persist retry item to Redis", zap.Error(err))
            }
        }

        return nil
    default:
        // Queue full - drop oldest item
        rq.logger.Warn("Retry queue full, dropping oldest item")
        rq.metrics.RetryQueueDropped.Inc()
        return fmt.Errorf("retry queue full")
    }
}

// persistToRedis persists a retry item to Redis
func (rq *RetryQueue) persistToRedis(item *RetryItem) error {
    data, err := json.Marshal(item)
    if err != nil {
        return fmt.Errorf("failed to marshal retry item: %w", err)
    }

    key := fmt.Sprintf("gateway:retry:%s", item.CRD.Name)
    return rq.redisClient.Set(rq.ctx, key, data, 24*time.Hour).Err()
}

// restoreFromRedis restores retry items from Redis on startup
func (rq *RetryQueue) restoreFromRedis() {
    keys, err := rq.redisClient.Keys(rq.ctx, "gateway:retry:*").Result()
    if err != nil {
        rq.logger.Error("Failed to restore retry items from Redis", zap.Error(err))
        return
    }

    for _, key := range keys {
        data, err := rq.redisClient.Get(rq.ctx, key).Result()
        if err != nil {
            continue
        }

        var item RetryItem
        if err := json.Unmarshal([]byte(data), &item); err != nil {
            continue
        }

        // Re-enqueue item
        if err := rq.Enqueue(&item); err != nil {
            rq.logger.Warn("Failed to re-enqueue retry item", zap.String("key", key))
        }
    }

    rq.logger.Info("Restored retry items from Redis", zap.Int("count", len(keys)))
}

// Shutdown gracefully shuts down the retry queue
func (rq *RetryQueue) Shutdown() {
    rq.logger.Info("Shutting down retry queue...")
    rq.cancel()
    close(rq.queue)
    rq.wg.Wait()
    rq.logger.Info("Retry queue shut down")
}

// retryWorker processes retry items from the queue
type retryWorker struct {
    id    int
    queue *RetryQueue
}

func newRetryWorker(id int, queue *RetryQueue) *retryWorker {
    return &retryWorker{id: id, queue: queue}
}

func (w *retryWorker) run() {
    defer w.queue.wg.Done()

    for {
        select {
        case item, ok := <-w.queue.queue:
            if !ok {
                return  // Queue closed
            }

            w.processRetryItem(item)

        case <-w.queue.ctx.Done():
            return
        }
    }
}

func (w *retryWorker) processRetryItem(item *RetryItem) {
    // Wait until next retry time
    if time.Now().Before(item.NextRetryTime) {
        time.Sleep(time.Until(item.NextRetryTime))
    }

    // Attempt CRD creation
    err := w.queue.k8sClient.CreateRemediationRequest(w.queue.ctx, item.CRD)

    if err == nil {
        // Success - remove from Redis
        w.queue.metrics.CRDRetrySuccess.WithLabelValues(fmt.Sprintf("attempt_%d", item.Attempt)).Inc()
        w.queue.metrics.RetryQueueDepth.Dec()

        if w.queue.config.RedisPersistence {
            key := fmt.Sprintf("gateway:retry:%s", item.CRD.Name)
            w.queue.redisClient.Del(w.queue.ctx, key)
        }

        w.queue.logger.Info("Retry succeeded",
            zap.Int("worker", w.id),
            zap.Int("attempt", item.Attempt),
            zap.String("crd_name", item.CRD.Name))
        return
    }

    // Check if error is retryable
    if !IsRetryableError(err, w.queue.config) {
        w.queue.logger.Error("Retry failed with non-retryable error",
            zap.Int("worker", w.id),
            zap.Error(err),
            zap.String("crd_name", item.CRD.Name))
        w.queue.metrics.RetryQueueDepth.Dec()
        return
    }

    // Check if max attempts reached
    if item.Attempt >= w.queue.config.MaxAttempts {
        w.queue.logger.Error("Retry exhausted after max attempts",
            zap.Int("worker", w.id),
            zap.Int("max_attempts", w.queue.config.MaxAttempts),
            zap.String("crd_name", item.CRD.Name))
        w.queue.metrics.CRDRetryExhausted.Inc()
        w.queue.metrics.RetryQueueDepth.Dec()
        return
    }

    // Re-enqueue with exponential backoff
    item.Attempt++
    backoff := w.queue.config.InitialBackoff * time.Duration(1<<uint(item.Attempt-1))
    if backoff > w.queue.config.MaxBackoff {
        backoff = w.queue.config.MaxBackoff
    }
    item.NextRetryTime = time.Now().Add(backoff)

    w.queue.metrics.CRDRetryAttempts.WithLabelValues(fmt.Sprintf("attempt_%d", item.Attempt)).Inc()

    if err := w.queue.Enqueue(item); err != nil {
        w.queue.logger.Error("Failed to re-enqueue retry item", zap.Error(err))
    }
}
```

---

#### **Step 3: CRD Creator Integration** (30 minutes)

**File**: `pkg/gateway/processing/crd_creator.go`

```go
type CRDCreator struct {
    k8sClient         *k8s.Client
    logger            *zap.Logger
    metrics           *metrics.Metrics
    fallbackNamespace string
    retryConfig       *config.RetrySettings
    retryQueue        *RetryQueue  // NEW: Async retry queue
}

func NewCRDCreator(k8sClient *k8s.Client, redisClient *redis.Client, logger *zap.Logger,
                   metricsInstance *metrics.Metrics, fallbackNamespace string,
                   retryConfig *config.RetrySettings) *CRDCreator {
    // ... existing validation ...

    var retryQueue *RetryQueue
    if retryConfig.AsyncEnabled {
        retryQueue = NewRetryQueue(k8sClient, redisClient, logger, metricsInstance, retryConfig)
    }

    return &CRDCreator{
        k8sClient:         k8sClient,
        logger:            logger,
        metrics:           metricsInstance,
        fallbackNamespace: fallbackNamespace,
        retryConfig:       retryConfig,
        retryQueue:        retryQueue,
    }
}

// CreateRemediationRequest creates a CRD with async retry if enabled
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal,
                                               environment, priority string) (*remediationv1alpha1.RemediationRequest, error) {
    // ... existing code to build rr ...

    // Attempt immediate CRD creation
    err := c.k8sClient.CreateRemediationRequest(ctx, rr)

    if err == nil {
        return rr, nil  // Success
    }

    // If async retry enabled and error is retryable, enqueue for retry
    if c.retryQueue != nil && IsRetryableError(err, c.retryConfig) {
        item := &RetryItem{
            CRD:           rr,
            Attempt:       1,
            NextRetryTime: time.Now().Add(c.retryConfig.InitialBackoff),
            EnqueuedAt:    time.Now(),
        }

        if err := c.retryQueue.Enqueue(item); err != nil {
            c.logger.Error("Failed to enqueue retry item", zap.Error(err))
            return nil, err
        }

        c.logger.Info("CRD creation failed, enqueued for async retry",
            zap.String("name", rr.Name),
            zap.Error(err))

        // Return success (HTTP 202 Accepted) - retry will happen in background
        return rr, nil
    }

    // Fall back to synchronous retry (Phase 1)
    if err := c.createCRDWithRetry(ctx, rr); err != nil {
        return nil, err
    }

    return rr, nil
}
```

---

### **Day 15 Deliverables**

- ✅ `pkg/gateway/config/config.go`: Added async retry queue config fields
- ✅ `pkg/gateway/processing/retry_queue.go`: Async retry queue implementation (BR-GATEWAY-115)
- ✅ `pkg/gateway/processing/crd_creator.go`: Async retry integration
- ✅ `pkg/gateway/metrics/metrics.go`: Queue depth and latency metrics
- ✅ `pkg/gateway/processing/retry_queue_test.go`: 12 unit tests
- ✅ `test/integration/gateway/async_retry_test.go`: 4 integration tests
- ✅ `test/e2e/gateway/07_async_retry_restart_test.go`: 4 E2E tests (restart recovery)
- ✅ `docs/services/stateless/gateway-service/operations/ASYNC_RETRY_RUNBOOK.md`: Operational guide

**Total Files**: 3 implementation files + 3 test files + 1 operational guide

---

### **Day 15 Success Criteria**

- ✅ All 12 unit tests pass (100% pass rate)
- ✅ All 4 integration tests pass (100% pass rate)
- ✅ All 4 E2E tests pass (Gateway restart recovery)
- ✅ Zero build errors
- ✅ Zero lint errors
- ✅ Queue depth metrics exposed at `/metrics` endpoint
- ✅ Operational runbook documented
- ✅ BR-GATEWAY-115 validated

---

## 📊 **Risk Assessment**

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Webhook timeout** (Phase 1) | Medium | High | Limit max retries to 3, use short backoff (10s max) |
| **Increased latency** (Phase 1) | High | Medium | Document expected latency (p95 < 3s), monitor p99 |
| **Retry storms** | Low | High | Exponential backoff prevents thundering herd |
| **Config errors** | Low | Medium | Validation in config loading, sensible defaults |
| **Test flakiness** | Medium | Low | Use deterministic timing in tests, mock K8s API |
| **Queue overflow** (Phase 2) | Medium | High | Drop oldest items when queue full, alert on queue depth |
| **Redis persistence failure** (Phase 2) | Low | Medium | Graceful degradation to in-memory queue only |

---

## 🎯 **Success Criteria**

### **Phase 1 (Day 14)**

1. ✅ **Functionality**: Retries succeed after transient K8s API errors (429, 503, timeout)
2. ✅ **Performance**: p95 latency < 200ms (no retry), < 3s (with retry)
3. ✅ **Reliability**: 99.9% alert processing success rate during rate limiting
4. ✅ **Observability**: Metrics track retry attempts, success rates, exhaustion
5. ✅ **Testing**: 90%+ code coverage for retry logic (31 tests)

### **Phase 2 (Day 15)**

1. ✅ **Functionality**: Async retry queue processes items in background
2. ✅ **Performance**: HTTP 202 response time < 50ms (non-blocking)
3. ✅ **Reliability**: Retry items survive Gateway restarts (Redis persistence)
4. ✅ **Observability**: Metrics track queue depth, retry latency, worker utilization
5. ✅ **Testing**: 95%+ code coverage for async queue logic (20 tests)

---

## 📚 **Documentation Updates**

### **Files to Update**

1. **API Specification**: `docs/services/stateless/gateway-service/api-specification.md`
   - Add "K8s API Retry Strategy" section
   - Document retryable vs non-retryable errors
   - Add configuration examples
   - Document retry metrics

2. **Configuration Reference**: `docs/services/stateless/gateway-service/configuration-reference.md`
   - Add `processing.retry` section
   - Document all retry config fields
   - Add Phase 1 vs Phase 2 config examples

3. **Design Decision**: `docs/architecture/decisions/DD-GATEWAY-008-k8s-api-retry-strategy.md`
   - Document retry strategy decision
   - Compare synchronous vs async approaches
   - Document performance implications
   - Add rollback plan

4. **Operational Runbook** (Phase 2): `docs/services/stateless/gateway-service/operations/ASYNC_RETRY_RUNBOOK.md`
   - Document async retry queue operations
   - Add troubleshooting guide for queue overflow
   - Document Redis persistence recovery
   - Add monitoring and alerting recommendations

---

## 🔗 **Integration with Existing Plan**

### **Updated Timeline**

| Day | Original Scope | New Scope | Duration |
|-----|---------------|-----------|----------|
| Days 1-13 | Gateway implementation | (Unchanged) | 13 days |
| **Day 14** | **NEW** | **K8s API Retry Logic (Phase 1)** | **10h** |
| **Day 15** | **NEW** | **Async Retry Queue (Phase 2, optional)** | **12h** |
| Pre-Day 10 Validation | Validation checkpoint | (Unchanged) | 3.5-4h |
| Day 10 | Final BR coverage | (Unchanged) | 8h |

**Total Additional Effort**: 10-22 hours (1.25-2.75 days)

---

## 📝 **Changelog (v2.25)**

### **Added**

- **K8s API Retry Logic**: Comprehensive retry logic for transient K8s API errors (429, 503, timeout)
- **Phased Implementation**: Phase 1 (Synchronous Retry, 10h) + Phase 2 (Async Retry Queue, 12h)
- **New BRs**: BR-GATEWAY-111 to BR-GATEWAY-115 (5 new BRs)
- **New Config Section**: `processing.retry` with 6 fields
- **New Files**: `pkg/gateway/processing/errors.go`, `pkg/gateway/processing/retry_queue.go` (Phase 2)
- **Test Coverage**: +31 tests (23 unit + 8 integration for Phase 1, +20 tests for Phase 2)
- **Design Decision**: DD-GATEWAY-008 (K8s API Retry Strategy)
- **Documentation**: API spec updates, configuration reference, operational runbook (Phase 2)

### **Changed**

- **Total BR Count**: 110 → 115 (+5 BRs)
- **Total Test Count**: 235 → 266 (+31 tests Phase 1, +20 tests Phase 2)
- **Implementation Timeline**: +10-22 hours (1.25-2.75 days)

### **Confidence**

- **Phase 1**: 88% (synchronous retry, straightforward implementation)
- **Phase 2**: 83% (async queue, more complex, requires Redis integration)
- **Overall**: 90% (comprehensive plan, clear implementation path)

---

## ✅ **v2.25 Status: READY FOR IMPLEMENTATION**

**Next Steps**:
1. Review and approve implementation plan
2. Start Day 14: K8s API Retry Logic (Phase 1)
3. Validate Phase 1 in production before proceeding to Phase 2
4. (Optional) Implement Day 15: Async Retry Queue (Phase 2)

**Confidence**: 90% ✅ **Implementation Plan Complete - Ready to Start**
