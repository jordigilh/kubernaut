# Context API Implementation Plan v2.12

## Version History

| Version | Date | Author | Changes | Status |
|---------|------|--------|---------|--------|
| v2.0 | 2024-10-15 | Initial | Initial implementation plan | ✅ Complete |
| v2.1 | 2024-10-20 | Update | Added graceful shutdown (DD-007) | ✅ Complete |
| v2.2 | 2024-10-25 | Update | Enhanced observability metrics | ✅ Complete |
| v2.3 | 2024-11-01 | Update | Added RFC 7807 error handling | ✅ Complete |
| v2.4 | 2024-11-02 | Update | Added circuit breaker patterns | ✅ Complete |
| v2.5 | 2024-11-03 | Update | Enhanced cache stampede prevention | ✅ Complete |
| v2.6 | 2024-11-04 | Update | Added Data Storage Service integration | ✅ Complete |
| v2.7 | 2024-11-05 | Update | Added graceful shutdown tests | ✅ Complete |
| v2.8 | 2024-11-06 | Update | Added aggregation API endpoints | ✅ Complete |
| v2.9 | 2024-11-06 | Update | Added edge case testing documentation | ✅ Complete |
| v2.10 | 2024-11-06 | Update | Added E2E tests and production docs | ✅ Complete |
| v2.11 | 2024-11-06 | Update | Added BR documentation and mapping | ✅ Complete |
| **v2.12** | **2024-11-07** | **Update** | **Day 15: P0 Unit Test Gap Closure (100% 2x Coverage)** | **⏳ In Progress** |

---

## 📋 v2.12 Overview

**Purpose**: Close P0 unit test coverage gaps to achieve 100% P0 2x coverage (50% → 100%)

**Key Changes**:
- ✅ **Day 15 Added**: P0 Unit Test Gap Closure (9 hours)
- ✅ **Iterative TDD Enforced**: Explicit one-test-at-a-time methodology
- ✅ **BR_MAPPING.md Integration**: All tests reference BR mapping document
- ✅ **P0 Priority**: Maximum coverage to prevent E2E surprises

**Coverage Impact**:
- P0 2x Coverage: 50% → **100%** ✅
- Total Tests: 133 → 147 (+14 unit tests)
- Confidence: 98% → **99%** ✅

**New Files**:
- `test/unit/contextapi/graceful_shutdown_test.go` (5 tests, ~250 lines)
- `test/unit/contextapi/incident_type_api_test.go` (3 tests, ~150 lines)
- `test/unit/contextapi/playbook_api_test.go` (3 tests, ~150 lines)
- `test/unit/contextapi/multi_dimensional_api_test.go` (3 tests, ~150 lines)

**See**: [V2.12_CHANGELOG.md](./V2.12_CHANGELOG.md) for detailed changes

---

## 🚨 CRITICAL: Iterative TDD Methodology

**MANDATORY FOR ALL DAY 15 IMPLEMENTATION**:

### **✅ CORRECT: Iterative TDD (One Test at a Time)**

```
1. Write ONE test (RED phase)
2. Make that ONE test pass (GREEN phase)
3. Refactor if needed (REFACTOR phase)
4. Commit/validate
5. Move to NEXT test
6. Repeat steps 1-5 for each test
```

**Example Flow**:
```
Test 1: "should close HTTP server gracefully"
  → RED: Write test, verify it fails
  → GREEN: Implement minimal code to pass
  → REFACTOR: Clean up implementation
  → ✅ Test passes

Test 2: "should drain in-flight requests"
  → RED: Write test, verify it fails
  → GREEN: Implement minimal code to pass
  → REFACTOR: Clean up implementation
  → ✅ Test passes

... continue for all 14 tests
```

### **❌ FORBIDDEN: Batch TDD (All Tests at Once)**

```
❌ Write all 14 tests first
❌ Then implement all business logic
❌ Then run all tests together
```

**Why Forbidden**: Violates TDD principles, increases debugging complexity, harder to track progress

---

## 📚 Reference to v2.11

**For Days 1-14 details, see**: [IMPLEMENTATION_PLAN_V2.11.md](./IMPLEMENTATION_PLAN_V2.11.md)

This document (v2.12) focuses exclusively on **Day 15: P0 Unit Test Gap Closure**. All previous implementation details (Days 1-14) remain unchanged from v2.11.

---

## 🎯 Day 15: P0 Unit Test Gap Closure (9 hours)

### **Objective**

Close P0 unit test coverage gaps to achieve 100% P0 2x coverage for Context API service.

**Current State**:
- P0 2x Coverage: 50% (4 of 8 P0 BRs have 2x coverage)
- Missing Coverage: 4 P0 BRs need unit tests

**Target State**:
- P0 2x Coverage: **100%** (8 of 8 P0 BRs have 2x coverage)
- Additional Tests: 14 new unit tests
- Confidence: **99%** (maximum coverage to prevent E2E surprises)

**Business Requirements**:
- **BR-CONTEXT-012**: Graceful Shutdown (5 unit tests)
- **BR-INTEGRATION-008**: Incident-Type Success Rate API (3 unit tests)
- **BR-INTEGRATION-009**: Playbook Success Rate API (3 unit tests)
- **BR-INTEGRATION-010**: Multi-Dimensional Success Rate API (3 unit tests)

**See**: [BR_MAPPING.md](../BR_MAPPING.md) for BR hierarchy and test file mapping

---

### **Phase 1: BR-CONTEXT-012 Graceful Shutdown Unit Tests** (2-3 hours)

**Objective**: Achieve 2x coverage for graceful shutdown behavior (currently only integration tests exist)

**Current Coverage**:
- ✅ Integration: `test/integration/contextapi/11_graceful_shutdown_test.go` (1x coverage)
- ❌ Unit: None (0x coverage)

**Target Coverage**:
- ✅ Integration: Existing tests (1x coverage)
- ✅ Unit: New tests (1x coverage)
- **Total**: 2x coverage ✅

---

#### **🚨 ITERATIVE TDD METHODOLOGY - PHASE 1**

**RULE**: Write ONE test at a time, make it pass, refactor, then move to next test.

**Test Sequence** (5 tests, one at a time):

1. **Test 1**: "should close HTTP server gracefully"
   - RED → GREEN → REFACTOR
   - Verify test passes before moving to Test 2

2. **Test 2**: "should drain in-flight requests before shutdown"
   - RED → GREEN → REFACTOR
   - Verify test passes before moving to Test 3

3. **Test 3**: "should stop accepting new requests after shutdown signal"
   - RED → GREEN → REFACTOR
   - Verify test passes before moving to Test 4

4. **Test 4**: "should respect shutdown timeout"
   - RED → GREEN → REFACTOR
   - Verify test passes before moving to Test 5

5. **Test 5**: "should force close connections after timeout"
   - RED → GREEN → REFACTOR
   - Verify test passes, complete Phase 1

---

#### **RED Phase: Write Failing Tests (One at a Time)**

**File**: `test/unit/contextapi/graceful_shutdown_test.go` (~250 lines)

**Test 1: HTTP Server Graceful Close**

```go
package contextapi_test

import (
    "context"
    "net/http"
    "testing"
    "time"

    "github.com/jordigilh/kubernaut/pkg/contextapi/server"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestGracefulShutdown(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Context API Graceful Shutdown Unit Test Suite")
}

var _ = Describe("Graceful Shutdown - Unit Tests", func() {
    // BR-CONTEXT-012: Graceful Shutdown
    // See: docs/services/stateless/context-api/BR_MAPPING.md

    Context("HTTP Server Shutdown", func() {
        It("should close HTTP server gracefully", func() {
            // BEHAVIOR: Server closes without errors
            // CORRECTNESS: All resources released properly

            // Create test server
            srv := &server.Server{
                HTTPServer: &http.Server{
                    Addr: ":0", // Random port
                },
            }

            // Start server in goroutine
            go func() {
                _ = srv.Start()
            }()

            // Wait for server to start
            time.Sleep(100 * time.Millisecond)

            // Trigger graceful shutdown
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()

            err := srv.Shutdown(ctx)

            // BEHAVIOR: Shutdown completes without error
            Expect(err).ToNot(HaveOccurred(), "Server should shutdown gracefully")

            // CORRECTNESS: Server is no longer accepting connections
            _, err = http.Get("http://localhost" + srv.HTTPServer.Addr)
            Expect(err).To(HaveOccurred(), "Server should not accept new connections after shutdown")
        })
    })
})
```

**Expected**: ❌ Test fails (implementation incomplete)

---

**Test 2: In-Flight Request Draining** (Write AFTER Test 1 passes)

```go
It("should drain in-flight requests before shutdown", func() {
    // BEHAVIOR: In-flight requests complete before shutdown
    // CORRECTNESS: No requests are dropped

    srv := createTestServer()
    go func() { _ = srv.Start() }()
    time.Sleep(100 * time.Millisecond)

    // Start a long-running request
    requestCompleted := make(chan bool)
    go func() {
        resp, err := http.Get("http://localhost" + srv.HTTPServer.Addr + "/slow-endpoint")
        if err == nil {
            resp.Body.Close()
            requestCompleted <- true
        } else {
            requestCompleted <- false
        }
    }()

    // Wait for request to start processing
    time.Sleep(50 * time.Millisecond)

    // Trigger shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    shutdownComplete := make(chan bool)
    go func() {
        err := srv.Shutdown(ctx)
        Expect(err).ToNot(HaveOccurred())
        shutdownComplete <- true
    }()

    // BEHAVIOR: Request completes before shutdown
    Eventually(requestCompleted, 5*time.Second).Should(Receive(Equal(true)))

    // CORRECTNESS: Shutdown completes after request drains
    Eventually(shutdownComplete, 6*time.Second).Should(Receive(Equal(true)))
})
```

**Expected**: ❌ Test fails (implementation incomplete)

---

**Test 3: New Request Rejection** (Write AFTER Test 2 passes)

```go
It("should stop accepting new requests after shutdown signal", func() {
    // BEHAVIOR: New requests rejected after shutdown starts
    // CORRECTNESS: HTTP 503 Service Unavailable returned

    srv := createTestServer()
    go func() { _ = srv.Start() }()
    time.Sleep(100 * time.Millisecond)

    // Trigger shutdown (non-blocking)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    go func() {
        _ = srv.Shutdown(ctx)
    }()

    // Wait for shutdown to start
    time.Sleep(100 * time.Millisecond)

    // Attempt new request
    resp, err := http.Get("http://localhost" + srv.HTTPServer.Addr + "/health")

    // BEHAVIOR: Request fails or returns 503
    if err == nil {
        defer resp.Body.Close()
        // CORRECTNESS: Returns 503 Service Unavailable
        Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
    } else {
        // CORRECTNESS: Connection refused (server stopped accepting)
        Expect(err).To(HaveOccurred())
    }
})
```

**Expected**: ❌ Test fails (implementation incomplete)

---

**Test 4: Shutdown Timeout Respect** (Write AFTER Test 3 passes)

```go
It("should respect shutdown timeout", func() {
    // BEHAVIOR: Shutdown waits for timeout before forcing close
    // CORRECTNESS: Timeout duration is honored

    srv := createTestServer()
    go func() { _ = srv.Start() }()
    time.Sleep(100 * time.Millisecond)

    // Start a request that takes longer than shutdown timeout
    go func() {
        _, _ = http.Get("http://localhost" + srv.HTTPServer.Addr + "/very-slow-endpoint")
    }()

    time.Sleep(50 * time.Millisecond)

    // Trigger shutdown with 2-second timeout
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    startTime := time.Now()
    err := srv.Shutdown(ctx)
    duration := time.Since(startTime)

    // BEHAVIOR: Shutdown completes within timeout window
    Expect(duration).To(BeNumerically(">=", 2*time.Second))
    Expect(duration).To(BeNumerically("<", 3*time.Second))

    // CORRECTNESS: Context deadline exceeded (timeout enforced)
    if err != nil {
        Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
    }
})
```

**Expected**: ❌ Test fails (implementation incomplete)

---

**Test 5: Force Close After Timeout** (Write AFTER Test 4 passes)

```go
It("should force close connections after timeout", func() {
    // BEHAVIOR: Connections forcibly closed after timeout
    // CORRECTNESS: Server stops regardless of in-flight requests

    srv := createTestServer()
    go func() { _ = srv.Start() }()
    time.Sleep(100 * time.Millisecond)

    // Start multiple long-running requests
    for i := 0; i < 5; i++ {
        go func() {
            _, _ = http.Get("http://localhost" + srv.HTTPServer.Addr + "/infinite-endpoint")
        }()
    }

    time.Sleep(100 * time.Millisecond)

    // Trigger shutdown with 1-second timeout
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    startTime := time.Now()
    _ = srv.Shutdown(ctx) // Ignore error (expected timeout)
    duration := time.Since(startTime)

    // BEHAVIOR: Shutdown completes after timeout (force close)
    Expect(duration).To(BeNumerically(">=", 1*time.Second))
    Expect(duration).To(BeNumerically("<", 2*time.Second))

    // CORRECTNESS: Server is stopped (no longer accepting connections)
    _, err := http.Get("http://localhost" + srv.HTTPServer.Addr + "/health")
    Expect(err).To(HaveOccurred(), "Server should be stopped after force close")
})
```

**Expected**: ❌ Test fails (implementation incomplete)

---

#### **GREEN Phase: Minimal Implementation (One Test at a Time)**

**Implementation Strategy**: Enhance existing `pkg/contextapi/server/server.go` graceful shutdown logic

**For Test 1** (HTTP Server Graceful Close):

```go
// pkg/contextapi/server/server.go

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Starting graceful shutdown")

    // Close HTTP server gracefully
    if err := s.HTTPServer.Shutdown(ctx); err != nil {
        s.logger.Error("HTTP server shutdown failed", zap.Error(err))
        return fmt.Errorf("failed to shutdown HTTP server: %w", err)
    }

    s.logger.Info("Graceful shutdown complete")
    return nil
}
```

**Expected**: ✅ Test 1 passes

---

**For Test 2** (In-Flight Request Draining):

```go
// pkg/contextapi/server/server.go

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Starting graceful shutdown")

    // Signal shutdown to middleware (stop accepting new requests)
    s.shutdownSignal.Store(true)

    // Wait for in-flight requests to complete
    s.logger.Info("Draining in-flight requests")

    // HTTP server Shutdown already handles draining
    if err := s.HTTPServer.Shutdown(ctx); err != nil {
        s.logger.Error("HTTP server shutdown failed", zap.Error(err))
        return fmt.Errorf("failed to shutdown HTTP server: %w", err)
    }

    s.logger.Info("All in-flight requests drained")
    s.logger.Info("Graceful shutdown complete")
    return nil
}
```

**Expected**: ✅ Tests 1-2 pass

---

**For Test 3** (New Request Rejection):

```go
// pkg/contextapi/server/middleware.go

func (s *Server) shutdownMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check if shutdown has been signaled
        if s.shutdownSignal.Load() {
            s.logger.Warn("Rejecting new request during shutdown",
                zap.String("path", r.URL.Path))
            http.Error(w, "Service shutting down", http.StatusServiceUnavailable)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

**Expected**: ✅ Tests 1-3 pass

---

**For Test 4** (Shutdown Timeout Respect):

```go
// pkg/contextapi/server/server.go

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Starting graceful shutdown",
        zap.Duration("timeout", s.config.ShutdownTimeout))

    s.shutdownSignal.Store(true)

    // Create context with configured timeout
    shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
    defer cancel()

    // Wait for in-flight requests with timeout
    if err := s.HTTPServer.Shutdown(shutdownCtx); err != nil {
        if err == context.DeadlineExceeded {
            s.logger.Warn("Shutdown timeout exceeded, forcing close")
            return s.HTTPServer.Close() // Force close
        }
        return fmt.Errorf("failed to shutdown HTTP server: %w", err)
    }

    s.logger.Info("Graceful shutdown complete")
    return nil
}
```

**Expected**: ✅ Tests 1-4 pass

---

**For Test 5** (Force Close After Timeout):

```go
// pkg/contextapi/server/server.go

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Starting graceful shutdown",
        zap.Duration("timeout", s.config.ShutdownTimeout))

    s.shutdownSignal.Store(true)

    shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
    defer cancel()

    if err := s.HTTPServer.Shutdown(shutdownCtx); err != nil {
        if err == context.DeadlineExceeded {
            s.logger.Warn("Shutdown timeout exceeded, forcing close",
                zap.Int("in_flight_requests", s.getInFlightRequestCount()))

            // Force close all connections
            if closeErr := s.HTTPServer.Close(); closeErr != nil {
                s.logger.Error("Force close failed", zap.Error(closeErr))
                return fmt.Errorf("failed to force close server: %w", closeErr)
            }

            s.logger.Info("Server force closed after timeout")
            return nil // Force close is success
        }
        return fmt.Errorf("failed to shutdown HTTP server: %w", err)
    }

    s.logger.Info("Graceful shutdown complete")
    return nil
}
```

**Expected**: ✅ All 5 tests pass

---

#### **REFACTOR Phase: Enhance Implementation**

**Enhancements**:
1. **Metrics**: Add `context_api_shutdown_duration_seconds` metric
2. **Logging**: Structured logging with shutdown phases
3. **Error Handling**: Wrap errors with context
4. **Resource Cleanup**: Close database connections, Redis clients

**Enhanced Implementation**:

```go
// pkg/contextapi/server/server.go

func (s *Server) Shutdown(ctx context.Context) error {
    startTime := time.Now()
    s.logger.Info("Starting graceful shutdown",
        zap.Duration("timeout", s.config.ShutdownTimeout))

    // Phase 1: Signal shutdown
    s.shutdownSignal.Store(true)
    s.logger.Info("Shutdown signal sent")

    // Phase 2: Stop accepting new requests (middleware handles this)
    s.logger.Info("Stopped accepting new requests")

    // Phase 3: Drain in-flight requests
    shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
    defer cancel()

    s.logger.Info("Draining in-flight requests",
        zap.Int("count", s.getInFlightRequestCount()))

    if err := s.HTTPServer.Shutdown(shutdownCtx); err != nil {
        if err == context.DeadlineExceeded {
            s.logger.Warn("Shutdown timeout exceeded, forcing close",
                zap.Int("remaining_requests", s.getInFlightRequestCount()))

            if closeErr := s.HTTPServer.Close(); closeErr != nil {
                s.logger.Error("Force close failed", zap.Error(closeErr))
                return fmt.Errorf("failed to force close server: %w", closeErr)
            }

            s.logger.Info("Server force closed after timeout")
        } else {
            s.logger.Error("HTTP server shutdown failed", zap.Error(err))
            return fmt.Errorf("failed to shutdown HTTP server: %w", err)
        }
    }

    // Phase 4: Close external connections
    s.logger.Info("Closing external connections")

    if s.dsClient != nil {
        // Data Storage client cleanup (if needed)
        s.logger.Info("Data Storage client closed")
    }

    if s.redisClient != nil {
        if err := s.redisClient.Close(); err != nil {
            s.logger.Error("Redis client close failed", zap.Error(err))
        } else {
            s.logger.Info("Redis client closed")
        }
    }

    // Record shutdown duration
    duration := time.Since(startTime)
    s.metrics.RecordShutdownDuration(duration)

    s.logger.Info("Graceful shutdown complete",
        zap.Duration("duration", duration))

    return nil
}
```

**Expected**: ✅ All 5 tests pass with enhanced implementation

---

#### **Phase 1 Deliverables**

**Files Created**:
- ✅ `test/unit/contextapi/graceful_shutdown_test.go` (5 tests, ~250 lines)

**Coverage Impact**:
- BR-CONTEXT-012: 1x → 2x coverage ✅

**Confidence**: 95% (Graceful shutdown is well-understood pattern)

---

### **Phase 2: BR-INTEGRATION-008 Incident-Type API Unit Tests** (1-2 hours)

**Objective**: Achieve 2x coverage for Incident-Type Success Rate API

**Current Coverage**:
- ✅ Integration: `test/integration/contextapi/11_aggregation_api_test.go` (1x coverage)
- ❌ Unit: None (0x coverage)

**Target Coverage**:
- ✅ Integration: Existing tests (1x coverage)
- ✅ Unit: New tests (1x coverage)
- **Total**: 2x coverage ✅

---

#### **🚨 ITERATIVE TDD METHODOLOGY - PHASE 2**

**Test Sequence** (3 tests, one at a time):

1. **Test 1**: "should validate incident_type parameter"
   - RED → GREEN → REFACTOR
   - Verify test passes before moving to Test 2

2. **Test 2**: "should parse time_range parameter correctly"
   - RED → GREEN → REFACTOR
   - Verify test passes before moving to Test 3

3. **Test 3**: "should call Data Storage Service with correct parameters"
   - RED → GREEN → REFACTOR
   - Verify test passes, complete Phase 2

---

#### **RED Phase: Write Failing Tests (One at a Time)**

**File**: `test/unit/contextapi/incident_type_api_test.go` (~150 lines)

**Test 1: Parameter Validation**

```go
package contextapi_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/jordigilh/kubernaut/pkg/contextapi/server"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestIncidentTypeAPI(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Context API Incident-Type API Unit Test Suite")
}

var _ = Describe("Incident-Type Success Rate API - Unit Tests", func() {
    // BR-INTEGRATION-008: Incident-Type Success Rate API
    // See: docs/services/stateless/context-api/BR_MAPPING.md

    var (
        srv *server.Server
    )

    BeforeEach(func() {
        srv = createTestServer()
    })

    Context("Parameter Validation", func() {
        It("should validate incident_type parameter", func() {
            // BEHAVIOR: Empty incident_type returns 400 Bad Request
            // CORRECTNESS: RFC 7807 error response with validation message

            // Test 1: Missing incident_type
            req := httptest.NewRequest("GET", "/api/v1/aggregation/success-rate/incident-type", nil)
            w := httptest.NewRecorder()

            srv.HandleGetSuccessRateByIncidentType(w, req)

            // BEHAVIOR: Returns 400 Bad Request
            Expect(w.Code).To(Equal(http.StatusBadRequest))

            // CORRECTNESS: RFC 7807 error response
            var problem map[string]interface{}
            err := json.NewDecoder(w.Body).Decode(&problem)
            Expect(err).ToNot(HaveOccurred())
            Expect(problem["type"]).To(ContainSubstring("validation-error"))
            Expect(problem["detail"]).To(ContainSubstring("incident_type"))

            // Test 2: Empty incident_type
            req = httptest.NewRequest("GET", "/api/v1/aggregation/success-rate/incident-type?incident_type=", nil)
            w = httptest.NewRecorder()

            srv.HandleGetSuccessRateByIncidentType(w, req)

            Expect(w.Code).To(Equal(http.StatusBadRequest))
        })
    })
})
```

**Expected**: ❌ Test fails (validation not implemented)

---

**Test 2: Time Range Parsing** (Write AFTER Test 1 passes)

```go
It("should parse time_range parameter correctly", func() {
    // BEHAVIOR: Valid time ranges parsed correctly
    // CORRECTNESS: Default time range applied when missing

    // Test 1: Valid time range
    req := httptest.NewRequest("GET",
        "/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d", nil)
    w := httptest.NewRecorder()

    srv.HandleGetSuccessRateByIncidentType(w, req)

    // BEHAVIOR: Returns 200 OK (or appropriate status)
    Expect(w.Code).To(BeNumerically(">=", 200))
    Expect(w.Code).To(BeNumerically("<", 300))

    // Test 2: Missing time range (should use default)
    req = httptest.NewRequest("GET",
        "/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom", nil)
    w = httptest.NewRecorder()

    srv.HandleGetSuccessRateByIncidentType(w, req)

    Expect(w.Code).To(BeNumerically(">=", 200))

    // Test 3: Invalid time range
    req = httptest.NewRequest("GET",
        "/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=invalid", nil)
    w = httptest.NewRecorder()

    srv.HandleGetSuccessRateByIncidentType(w, req)

    // BEHAVIOR: Returns 400 Bad Request
    Expect(w.Code).To(Equal(http.StatusBadRequest))
})
```

**Expected**: ❌ Test fails (parsing not implemented)

---

**Test 3: Data Storage Service Call** (Write AFTER Test 2 passes)

```go
It("should call Data Storage Service with correct parameters", func() {
    // BEHAVIOR: Correct parameters passed to Data Storage Service
    // CORRECTNESS: Response transformed correctly

    // Mock Data Storage Service client
    mockDSClient := &MockDataStorageClient{
        GetIncidentTypeSuccessRateFunc: func(ctx context.Context, params *dsmodels.IncidentTypeParams) (*dsmodels.IncidentTypeSuccessRateResponse, error) {
            // CORRECTNESS: Verify parameters
            Expect(params.IncidentType).To(Equal("pod-oom"))
            Expect(params.TimeRange).To(Equal("7d"))
            Expect(params.MinSamples).To(Equal(10))

            // Return mock response
            return &dsmodels.IncidentTypeSuccessRateResponse{
                IncidentType:         "pod-oom",
                SuccessRate:          75.5,
                TotalExecutions:      100,
                SuccessfulExecutions: 75,
                FailedExecutions:     25,
            }, nil
        },
    }

    srv.dsClient = mockDSClient

    req := httptest.NewRequest("GET",
        "/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=10", nil)
    w := httptest.NewRecorder()

    srv.HandleGetSuccessRateByIncidentType(w, req)

    // BEHAVIOR: Returns 200 OK
    Expect(w.Code).To(Equal(http.StatusOK))

    // CORRECTNESS: Response matches Data Storage response
    var response dsmodels.IncidentTypeSuccessRateResponse
    err := json.NewDecoder(w.Body).Decode(&response)
    Expect(err).ToNot(HaveOccurred())
    Expect(response.IncidentType).To(Equal("pod-oom"))
    Expect(response.SuccessRate).To(Equal(75.5))
    Expect(response.TotalExecutions).To(Equal(100))
})
```

**Expected**: ❌ Test fails (Data Storage call not implemented)

---

#### **GREEN Phase: Minimal Implementation (One Test at a Time)**

**For Test 1** (Parameter Validation):

```go
// pkg/contextapi/server/aggregation_handlers.go

func (s *Server) HandleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")

    // Validate incident_type
    if incidentType == "" {
        s.respondRFC7807Error(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    // TODO: Implement rest of handler
    w.WriteHeader(http.StatusOK)
}
```

**Expected**: ✅ Test 1 passes

---

**For Test 2** (Time Range Parsing):

```go
func (s *Server) HandleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondRFC7807Error(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    // Parse time_range (default: 7d)
    timeRange := r.URL.Query().Get("time_range")
    if timeRange == "" {
        timeRange = "7d"
    }

    // Validate time_range format
    if !isValidTimeRange(timeRange) {
        s.respondRFC7807Error(w, http.StatusBadRequest, "invalid time_range format")
        return
    }

    // TODO: Call Data Storage Service
    w.WriteHeader(http.StatusOK)
}

func isValidTimeRange(timeRange string) bool {
    // Simple validation: ends with d, h, m
    if len(timeRange) < 2 {
        return false
    }
    unit := timeRange[len(timeRange)-1]
    return unit == 'd' || unit == 'h' || unit == 'm'
}
```

**Expected**: ✅ Tests 1-2 pass

---

**For Test 3** (Data Storage Service Call):

```go
func (s *Server) HandleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondRFC7807Error(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    timeRange := r.URL.Query().Get("time_range")
    if timeRange == "" {
        timeRange = "7d"
    }

    if !isValidTimeRange(timeRange) {
        s.respondRFC7807Error(w, http.StatusBadRequest, "invalid time_range format")
        return
    }

    minSamples := parseIntParam(r, "min_samples", 10) // Default: 10

    // Call Data Storage Service
    result, err := s.dsClient.GetIncidentTypeSuccessRate(r.Context(), &dsmodels.IncidentTypeParams{
        IncidentType: incidentType,
        TimeRange:    timeRange,
        MinSamples:   minSamples,
    })
    if err != nil {
        s.logger.Error("failed to get incident type success rate", zap.Error(err))
        s.respondRFC7807Error(w, http.StatusInternalServerError, "failed to retrieve success rate data")
        return
    }

    // Return JSON response
    s.respondJSON(w, http.StatusOK, result)
}
```

**Expected**: ✅ All 3 tests pass

---

#### **REFACTOR Phase: Enhance Implementation**

**Enhancements**:
1. **Caching**: Add Redis caching for responses
2. **Metrics**: Record request duration and cache hits
3. **Logging**: Structured logging with request context
4. **Error Handling**: Circuit breaker for Data Storage failures

**Enhanced Implementation** (same as integration tests, no changes needed for unit tests)

---

#### **Phase 2 Deliverables**

**Files Created**:
- ✅ `test/unit/contextapi/incident_type_api_test.go` (3 tests, ~150 lines)

**Coverage Impact**:
- BR-INTEGRATION-008: 1x → 2x coverage ✅

**Confidence**: 90% (API handler patterns established)

---

### **Phase 3: BR-INTEGRATION-009 Playbook API Unit Tests** (1-2 hours)

**Objective**: Achieve 2x coverage for Playbook Success Rate API

**Current Coverage**:
- ✅ Integration: `test/integration/contextapi/11_aggregation_api_test.go` (1x coverage)
- ❌ Unit: None (0x coverage)

**Target Coverage**:
- ✅ Integration: Existing tests (1x coverage)
- ✅ Unit: New tests (1x coverage)
- **Total**: 2x coverage ✅

---

#### **🚨 ITERATIVE TDD METHODOLOGY - PHASE 3**

**Test Sequence** (3 tests, one at a time):

1. **Test 1**: "should validate playbook_id parameter"
2. **Test 2**: "should handle optional playbook_version parameter"
3. **Test 3**: "should call Data Storage Service with correct playbook parameters"

---

#### **RED/GREEN/REFACTOR Phases**

**Implementation follows same pattern as Phase 2**:
- Test 1: Parameter validation (playbook_id required)
- Test 2: Optional parameter handling (playbook_version)
- Test 3: Data Storage Service call verification

**File**: `test/unit/contextapi/playbook_api_test.go` (~150 lines)

**Code structure mirrors Phase 2 with playbook-specific parameters**

---

#### **Phase 3 Deliverables**

**Files Created**:
- ✅ `test/unit/contextapi/playbook_api_test.go` (3 tests, ~150 lines)

**Coverage Impact**:
- BR-INTEGRATION-009: 1x → 2x coverage ✅

**Confidence**: 90% (Same pattern as Phase 2)

---

### **Phase 4: BR-INTEGRATION-010 Multi-Dimensional API Unit Tests** (1-2 hours)

**Objective**: Achieve 2x coverage for Multi-Dimensional Success Rate API

**Current Coverage**:
- ✅ Integration: `test/integration/contextapi/11_aggregation_api_test.go` (1x coverage)
- ❌ Unit: None (0x coverage)

**Target Coverage**:
- ✅ Integration: Existing tests (1x coverage)
- ✅ Unit: New tests (1x coverage)
- **Total**: 2x coverage ✅

---

#### **🚨 ITERATIVE TDD METHODOLOGY - PHASE 4**

**Test Sequence** (3 tests, one at a time):

1. **Test 1**: "should validate at least one dimension is provided"
2. **Test 2**: "should handle multiple dimensions correctly"
3. **Test 3**: "should call Data Storage Service with multi-dimensional parameters"

---

#### **RED/GREEN/REFACTOR Phases**

**Implementation follows same pattern as Phases 2-3**:
- Test 1: Multi-dimensional validation (at least one dimension required)
- Test 2: Multiple dimension handling
- Test 3: Data Storage Service call verification

**File**: `test/unit/contextapi/multi_dimensional_api_test.go` (~150 lines)

**Code structure mirrors Phases 2-3 with multi-dimensional parameters**

---

#### **Phase 4 Deliverables**

**Files Created**:
- ✅ `test/unit/contextapi/multi_dimensional_api_test.go` (3 tests, ~150 lines)

**Coverage Impact**:
- BR-INTEGRATION-010: 1x → 2x coverage ✅

**Confidence**: 90% (Same pattern as Phases 2-3)

---

## 📊 Day 15 Summary

### **Total Effort**: 9 hours

| Phase | BR | Tests | Lines | Hours |
|-------|-----|-------|-------|-------|
| Phase 1 | BR-CONTEXT-012 | 5 | ~250 | 2-3 |
| Phase 2 | BR-INTEGRATION-008 | 3 | ~150 | 1-2 |
| Phase 3 | BR-INTEGRATION-009 | 3 | ~150 | 1-2 |
| Phase 4 | BR-INTEGRATION-010 | 3 | ~150 | 1-2 |
| **Total** | **4 BRs** | **14** | **~700** | **9** |

### **Coverage Impact**

**Before Day 15**:
- P0 BRs: 8 total
- P0 2x Coverage: 50% (4 of 8)
- Total Tests: 133

**After Day 15**:
- P0 BRs: 8 total
- P0 2x Coverage: **100%** (8 of 8) ✅
- Total Tests: 147 (+14 unit tests)

### **Files Created**

1. ✅ `test/unit/contextapi/graceful_shutdown_test.go` (5 tests, ~250 lines)
2. ✅ `test/unit/contextapi/incident_type_api_test.go` (3 tests, ~150 lines)
3. ✅ `test/unit/contextapi/playbook_api_test.go` (3 tests, ~150 lines)
4. ✅ `test/unit/contextapi/multi_dimensional_api_test.go` (3 tests, ~150 lines)

**Total**: 4 files, 14 tests, ~700 lines

---

## ✅ Success Criteria

**Day 15 is complete when**:

1. ✅ All 14 unit tests pass
2. ✅ P0 2x coverage reaches 100% (8 of 8 BRs)
3. ✅ No build or lint errors
4. ✅ All tests follow iterative TDD (one at a time)
5. ✅ All BRs reference BR_MAPPING.md
6. ✅ Confidence assessment ≥95%

---

## 🎯 Confidence Assessment

**99% Confidence** that Day 15 implementation will succeed:

**Evidence**:
- ✅ Iterative TDD methodology explicitly enforced (no batch TDD)
- ✅ Test patterns established and proven (Phases 2-4 mirror existing patterns)
- ✅ Graceful shutdown is well-understood pattern (Phase 1)
- ✅ All BRs have existing integration tests (1x coverage already proven)
- ✅ BR_MAPPING.md provides clear traceability
- ✅ P0 priority ensures maximum coverage to prevent E2E surprises

**Risk**: Very Low
- Unit tests are isolated and fast
- No infrastructure dependencies
- Patterns already validated in integration tests

**Mitigation**:
- Iterative TDD ensures early failure detection
- One test at a time prevents cascade failures
- Existing integration tests provide reference implementation

---

## 📚 References

- **Business Requirements**: [BUSINESS_REQUIREMENTS.md](../BUSINESS_REQUIREMENTS.md)
- **BR Mapping**: [BR_MAPPING.md](../BR_MAPPING.md)
- **Changelog**: [V2.12_CHANGELOG.md](./V2.12_CHANGELOG.md)
- **Previous Plan**: [IMPLEMENTATION_PLAN_V2.11.md](./IMPLEMENTATION_PLAN_V2.11.md)
- **Testing Strategy**: [docs/services/crd-controllers/03-workflowexecution/testing-strategy.md](../../../../crd-controllers/03-workflowexecution/testing-strategy.md)
- **TDD Methodology**: [.cursor/rules/00-core-development-methodology.mdc](../../../../../.cursor/rules/00-core-development-methodology.mdc)

---

**End of Implementation Plan v2.12**

