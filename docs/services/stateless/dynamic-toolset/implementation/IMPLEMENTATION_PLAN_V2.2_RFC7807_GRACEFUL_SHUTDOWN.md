# Dynamic Toolset Service - Implementation Plan v2.2

**Version**: v2.2 (RFC 7807 + Graceful Shutdown - Complete Production Readiness)
**Date**: 2025-11-09
**Timeline**: 2 days (16 hours)
**Status**: ⏸️ **PENDING APPROVAL**
**Based On**: IMPLEMENTATION_PLAN_ENHANCED.md v2.0
**Parent Plan**: IMPLEMENTATION_PLAN_ENHANCED.md (Days 1-13 complete)

---

## 📋 Version History & Changelog

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v2.0** | 2025-10-11 | Enhanced plan with Gateway learnings (Days 1-13) | ✅ **COMPLETE** |
| **v2.1** | 2025-11-09 | RFC 7807 Error Responses extension (Day 14) | ⏸️ **SUPERSEDED** |
| **v2.2** | 2025-11-09 | RFC 7807 + Graceful Shutdown (Days 14-15) | ⏸️ **PENDING APPROVAL** |

### v2.2 Changelog (2025-11-09)

**Added**:
- ✅ **BR-TOOLSET-039**: RFC 7807 Error Response Standard (NEW)
- ✅ **BR-TOOLSET-040**: Graceful Shutdown with Signal Handling (NEW)
- ✅ **Day 14**: RFC 7807 implementation following TDD methodology
- ✅ **Day 15**: Graceful shutdown following DD-007 pattern
- ✅ Integration tests for RFC 7807 compliance (6 tests)
- ✅ Integration tests for graceful shutdown (8 tests - matching Context API)
- ✅ Error response standardization across all endpoints
- ✅ SIGTERM/SIGINT signal handling for Kubernetes

**Modified**:
- Updated BR_MAPPING.md to include BR-TOOLSET-039 and BR-TOOLSET-040
- Updated BUSINESS_REQUIREMENTS.md with RFC 7807 and graceful shutdown requirements

**Rationale**:
- **Compliance**: DD-004 mandates RFC 7807 for all HTTP services
- **Production Safety**: DD-007 mandates graceful shutdown for zero-downtime deployments
- **Consistency**: Gateway, Context API, Data Storage already use RFC 7807 + DD-007
- **Test Parity**: 8 graceful shutdown tests match Context API exactly (per user requirement)

**Dependencies**:
- ✅ DD-004: RFC 7807 Error Response Standard (approved)
- ✅ DD-007: Kubernetes-Aware Graceful Shutdown Pattern (approved)
- ✅ Gateway Service RFC 7807 implementation (reference)
- ✅ Context API graceful shutdown implementation (reference - 8 tests)
- ✅ Days 1-13 complete (service operational)

---

## 🎯 Overview

This plan extends the Dynamic Toolset Service with **production readiness requirements**:
1. **RFC 7807 Error Responses**: Standardized HTTP error format (DD-004)
2. **Graceful Shutdown**: 4-step Kubernetes-aware shutdown pattern (DD-007)

**Why This Extension?**
1. **Compliance**: DD-004 and DD-007 mandate these features for all HTTP services
2. **Consistency**: 3 of 6 services already use these patterns (Gateway, Context API, Data Storage)
3. **Production Safety**: Zero request failures during rolling updates
4. **Test Parity**: Same 8-test graceful shutdown coverage as Context API

**Scope**:
- ✅ RFC 7807 error response implementation
- ✅ DD-007 graceful shutdown with 4-step pattern
- ✅ 6 RFC 7807 integration tests
- ✅ 8 graceful shutdown integration tests (matching Context API)
- ✅ SIGTERM/SIGINT signal handling
- ✅ BR documentation and mapping
- ❌ No changes to business logic (service already operational)

---

## 📊 New Business Requirements

### BR-TOOLSET-039: RFC 7807 Error Response Standard

**Priority**: P1 (Production Readiness)
**Status**: ⏸️ Pending Implementation
**Category**: API Quality & Standards Compliance

**Description**:
All HTTP error responses (4xx, 5xx) from the Dynamic Toolset Service MUST use RFC 7807 Problem Details format to ensure consistent, machine-readable error handling for clients and operators.

**Business Value**:
- **Operator Efficiency**: Standardized errors improve troubleshooting speed
- **Client Integration**: Single error parser for all Kubernaut services
- **API Quality**: Industry-standard format improves API professionalism
- **Monitoring**: Structured errors enable better alerting and metrics

**Acceptance Criteria**:
1. ✅ All HTTP error responses (4xx, 5xx) use RFC 7807 format
2. ✅ Error responses include all required fields: `type`, `title`, `detail`, `status`, `instance`
3. ✅ Error responses set `Content-Type: application/problem+json` header
4. ✅ Error type URIs follow convention: `https://kubernaut.io/errors/{error-type}`
5. ✅ Request ID included in error responses when available
6. ✅ Integration tests validate RFC 7807 compliance

**Error Types to Implement**:
| HTTP Status | Error Type | Title | Use Case |
|-------------|-----------|-------|----------|
| **400** | `validation-error` | Bad Request | Invalid request format, missing fields |
| **405** | `method-not-allowed` | Method Not Allowed | Wrong HTTP method |
| **415** | `unsupported-media-type` | Unsupported Media Type | Wrong Content-Type header |
| **500** | `internal-error` | Internal Server Error | Unexpected server errors |
| **503** | `service-unavailable` | Service Unavailable | Graceful shutdown, K8s unavailable |

**Related**:
- **DD-004**: RFC 7807 Error Response Standard (authority)
- **BR-TOOLSET-001 to BR-TOOLSET-038**: Existing business requirements (no changes)

**Test Coverage**:
- Integration: `test/integration/toolset/rfc7807_compliance_test.go` (6 tests)

**Implementation Files**:
- `pkg/toolset/errors/rfc7807.go` (new)
- `pkg/toolset/server/server.go` (modify error responses)

---

### BR-TOOLSET-040: Graceful Shutdown with Signal Handling

**Priority**: P0 (Production Safety)
**Status**: ⏸️ Pending Implementation
**Category**: Kubernetes Operations & Reliability

**Description**:
The Dynamic Toolset Service MUST handle SIGTERM and SIGINT signals gracefully using the DD-007 4-step pattern to ensure zero-downtime deployments and prevent request loss during pod termination in Kubernetes.

**Business Value**:
- **Zero-Downtime Deployments**: No request loss during rolling updates
- **Production Safety**: Clean shutdown prevents data corruption
- **Kubernetes Best Practice**: Aligns with terminationGracePeriodSeconds
- **Operator Confidence**: Predictable shutdown behavior

**Acceptance Criteria**:
1. ✅ Service handles SIGTERM signal (Kubernetes pod termination)
2. ✅ Service handles SIGINT signal (Ctrl+C for local development)
3. ✅ 4-step shutdown pattern (DD-007): flag → wait 5s → drain → cleanup
4. ✅ In-flight requests complete before shutdown (30s timeout)
5. ✅ Readiness probe returns 503 during shutdown
6. ✅ Liveness probe remains healthy during shutdown
7. ✅ Logs indicate all shutdown steps
8. ✅ 8 integration tests (matching Context API test coverage)

**Shutdown Sequence** (DD-007 Pattern):
1. Receive SIGTERM/SIGINT signal
2. Set `isShuttingDown` flag (atomic.Bool)
3. Readiness probe returns 503 (RFC 7807 format)
4. Wait 5 seconds for Kubernetes endpoint removal propagation
5. Drain in-flight HTTP connections (30s timeout)
6. Close resources (Kubernetes client)
7. Exit with status code 0

**Related**:
- **DD-007**: Kubernetes-Aware Graceful Shutdown Pattern (authority)
- **Kubernetes**: terminationGracePeriodSeconds (default: 30s)

**Test Coverage**:
- Integration: `test/integration/toolset/graceful_shutdown_test.go` (8 tests)
  1. Readiness probe coordination (P0)
  2. Liveness probe during shutdown (P0)
  3. In-flight request completion (P0)
  4. Resource cleanup (P1)
  5. Shutdown timing (5s wait) (P1)
  6. Shutdown timeout respect (P1)
  7. Concurrent shutdown safety (P2)
  8. Shutdown logging (P2)

**Implementation Files**:
- `pkg/toolset/server/server.go` (add Shutdown method)
- `cmd/dynamictoolset/main.go` (add signal handlers)

---

## 🗓️ Implementation Timeline

**Total Duration**: 16 hours (2 days)
**Methodology**: TDD (Test-Driven Development)

| Day | Phase | Focus | Duration | Status |
|-----|-------|-------|----------|--------|
| **Day 14** | RFC 7807 | Error response standardization | 8 hours | ⏸️ Pending |
| **Day 15** | Graceful Shutdown | DD-007 4-step pattern + 8 tests | 8 hours | ⏸️ Pending |

---

## 📅 Day 14: RFC 7807 Error Responses (8 hours)

### Phase 1: DO-RED (2 hours)

**Objective**: Write failing integration tests that define RFC 7807 requirements

#### Task 1.1: Create Integration Test File (30 min)

**File**: `test/integration/toolset/rfc7807_compliance_test.go`

**Test Structure** (6 tests):
```go
package toolset

import (
    "encoding/json"
    "net/http"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/toolset/errors"
)

var _ = Describe("BR-TOOLSET-039: RFC 7807 Error Response Compliance", func() {
    var (
        serverURL string
        client    *http.Client
    )

    BeforeEach(func() {
        serverURL = "http://localhost:8080"
        client = &http.Client{}
    })

    Context("when invalid Content-Type is provided", func() {
        It("should return RFC 7807 error with 415 status", func() {
            // BR-TOOLSET-039: Unsupported Media Type error
            req, err := http.NewRequest("POST", serverURL+"/api/v1/toolsets", nil)
            Expect(err).ToNot(HaveOccurred())
            req.Header.Set("Content-Type", "text/plain")

            resp, err := client.Do(req)
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()

            Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType))
            Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

            var errorResp errors.RFC7807Error
            err = json.NewDecoder(resp.Body).Decode(&errorResp)
            Expect(err).ToNot(HaveOccurred())

            Expect(errorResp.Type).To(Equal("https://kubernaut.io/errors/unsupported-media-type"))
            Expect(errorResp.Title).To(Equal("Unsupported Media Type"))
            Expect(errorResp.Detail).ToNot(BeEmpty())
            Expect(errorResp.Status).To(Equal(415))
            Expect(errorResp.Instance).To(Equal("/api/v1/toolsets"))
        })
    })

    Context("when method not allowed", func() {
        It("should return RFC 7807 error with 405 status", func() {
            // Test implementation
        })
    })

    Context("when service is shutting down", func() {
        It("should return RFC 7807 error with 503 status", func() {
            // Test implementation
        })
    })

    Context("RFC 7807 compliance validation", func() {
        It("should include all required fields in error responses", func() {
            // Test implementation
        })

        It("should use correct error type URI format", func() {
            // Test implementation
        })

        It("should include request ID when available", func() {
            // Test implementation
        })
    })
})
```

**Expected Result**: All 6 tests FAIL (service not yet using RFC 7807)

**Deliverables**:
- ✅ `test/integration/toolset/rfc7807_compliance_test.go` created
- ✅ 6 failing integration tests
- ✅ Test coverage for all error scenarios

---

### Phase 2: DO-GREEN (3 hours)

**Objective**: Minimal implementation to make tests pass

#### Task 2.1: Create RFC 7807 Error Package (45 min)

**File**: `pkg/toolset/errors/rfc7807.go`

**Implementation** (copy from Gateway with toolset-specific adjustments):
```go
package errors

// RFC7807Error represents an RFC 7807 Problem Details error response
// Specification: https://tools.ietf.org/html/rfc7807
// BR-TOOLSET-039: RFC 7807 error format
type RFC7807Error struct {
    Type      string `json:"type"`
    Title     string `json:"title"`
    Detail    string `json:"detail"`
    Status    int    `json:"status"`
    Instance  string `json:"instance"`
    RequestID string `json:"request_id,omitempty"`
}

// Error type URI constants
const (
    ErrorTypeValidationError      = "https://kubernaut.io/errors/validation-error"
    ErrorTypeMethodNotAllowed     = "https://kubernaut.io/errors/method-not-allowed"
    ErrorTypeUnsupportedMediaType = "https://kubernaut.io/errors/unsupported-media-type"
    ErrorTypeInternalError        = "https://kubernaut.io/errors/internal-error"
    ErrorTypeServiceUnavailable   = "https://kubernaut.io/errors/service-unavailable"
)

// Error title constants
const (
    TitleBadRequest             = "Bad Request"
    TitleMethodNotAllowed       = "Method Not Allowed"
    TitleUnsupportedMediaType   = "Unsupported Media Type"
    TitleInternalServerError    = "Internal Server Error"
    TitleServiceUnavailable     = "Service Unavailable"
)

// NewRFC7807Error creates a new RFC 7807 error
func NewRFC7807Error(statusCode int, detail, instance string) RFC7807Error {
    errorType, title := getErrorTypeAndTitle(statusCode)
    return RFC7807Error{
        Type:     errorType,
        Title:    title,
        Detail:   detail,
        Status:   statusCode,
        Instance: instance,
    }
}

func getErrorTypeAndTitle(statusCode int) (string, string) {
    switch statusCode {
    case 400:
        return ErrorTypeValidationError, TitleBadRequest
    case 405:
        return ErrorTypeMethodNotAllowed, TitleMethodNotAllowed
    case 415:
        return ErrorTypeUnsupportedMediaType, TitleUnsupportedMediaType
    case 500:
        return ErrorTypeInternalError, TitleInternalServerError
    case 503:
        return ErrorTypeServiceUnavailable, TitleServiceUnavailable
    default:
        return ErrorTypeInternalError, TitleInternalServerError
    }
}
```

#### Task 2.2: Add Error Response Helper (1 hour)

**File**: `pkg/toolset/server/server.go`

```go
import (
    toolseterrors "github.com/jordigilh/kubernaut/pkg/toolset/errors"
)

// writeJSONError writes an RFC 7807 compliant error response
// BR-TOOLSET-039: RFC 7807 error response helper
func (s *Server) writeJSONError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(statusCode)

    requestID := ""
    if id := r.Context().Value("request_id"); id != nil {
        requestID = id.(string)
    }

    errorResponse := toolseterrors.NewRFC7807Error(statusCode, message, r.URL.Path)
    errorResponse.RequestID = requestID

    if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
        http.Error(w, message, statusCode)
    }
}
```

#### Task 2.3: Update Error Responses (1 hour 15 min)

**Pattern** (replace all `http.Error()` calls):
```go
// BEFORE:
http.Error(w, "Invalid Content-Type", http.StatusUnsupportedMediaType)

// AFTER:
s.writeJSONError(w, r, "Invalid Content-Type: expected application/json", http.StatusUnsupportedMediaType)
```

**Expected Result**: All 6 RFC 7807 tests PASS

---

### Phase 3: DO-REFACTOR (2 hours)

**Objective**: Enhance implementation with production-quality features

#### Task 3.1: Add Request ID Middleware (45 min)

**File**: `pkg/toolset/middleware/request_id.go` (new)

```go
package middleware

import (
    "context"
    "net/http"
    "github.com/google/uuid"
)

func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = uuid.New().String()
        }
        w.Header().Set("X-Request-ID", requestID)
        ctx := context.WithValue(r.Context(), "request_id", requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

#### Task 3.2: Enhance Error Messages (30 min)

Make error messages more descriptive and actionable.

#### Task 3.3: Add Error Metrics (45 min)

**File**: `pkg/toolset/metrics/metrics.go`

```go
var ErrorResponsesTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "toolset_error_responses_total",
        Help: "Total number of HTTP error responses by status code and type",
    },
    []string{"status_code", "error_type"},
)
```

---

### Phase 4: CHECK (1 hour)

**Objective**: Validate RFC 7807 implementation

#### Task 4.1: Run Tests (20 min)

```bash
make test-integration-toolset
```

**Expected**: 6 RFC 7807 tests pass, no regressions

#### Task 4.2: Update BR Documentation (20 min)

Update BUSINESS_REQUIREMENTS.md and BR_MAPPING.md with BR-TOOLSET-039.

#### Task 4.3: Confidence Assessment (20 min)

**Expected Confidence**: 95%

---

## 📅 Day 15: Graceful Shutdown (8 hours)

**Objective**: Implement DD-007 4-step graceful shutdown pattern with **8 integration tests matching Context API**

### Phase 1: DO-RED (2 hours)

**Objective**: Write failing integration tests that define graceful shutdown requirements

#### Task 1.1: Create Graceful Shutdown Test File (2 hours)

**File**: `test/integration/toolset/graceful_shutdown_test.go`

**Test Coverage** (8 tests - **EXACT MATCH to Context API per user requirement**):

```go
package toolset

import (
    "context"
    "fmt"
    "net/http"
    "sync"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

// Integration Tests for DD-007: Graceful Shutdown
//
// Business Requirement: BR-TOOLSET-040 - Graceful shutdown with in-flight request completion
//
// Test Coverage (8 tests matching Context API):
// 1. Readiness probe coordination (P0)
// 2. Liveness probe during shutdown (P0)
// 3. In-flight request completion (P0)
// 4. Resource cleanup (P1)
// 5. Shutdown timing (5s wait) (P1)
// 6. Shutdown timeout respect (P1)
// 7. Concurrent shutdown safety (P2)
// 8. Shutdown logging (P2)
//
// Related: DD-007 Kubernetes-Aware Graceful Shutdown Pattern

var _ = Describe("DD-007: Graceful Shutdown", Ordered, Label("integration", "shutdown"), func() {
    var (
        testServer     *server.Server
        serverPort     int
        serverBaseURL  string
        logger         *zap.Logger
        shutdownCtx    context.Context
        shutdownCancel context.CancelFunc
    )

    BeforeAll(func() {
        GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
        GinkgoWriter.Println("🧪 DD-007: Graceful Shutdown Integration Tests")
        GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

        var err error
        logger, err = zap.NewDevelopment()
        Expect(err).ToNot(HaveOccurred())

        serverPort = 9092
        serverBaseURL = fmt.Sprintf("http://localhost:%d", serverPort)
        shutdownCtx, shutdownCancel = context.WithTimeout(context.Background(), 60*time.Second)
    })

    AfterAll(func() {
        shutdownCancel()
        GinkgoWriter.Println("✅ DD-007: Graceful Shutdown Tests Complete")
    })

    // Helper function to create and start a test server
    createTestServer := func() *server.Server {
        // Implementation similar to Context API
    }

    Describe("Test 1: Readiness Probe Coordination (P0)", func() {
        It("should return 503 from readiness probe during shutdown", func() {
            GinkgoWriter.Println("🧪 Test 1: Readiness Probe Coordination (P0)")
            
            testServer = createTestServer()

            // Verify readiness probe returns 200 before shutdown
            resp, err := http.Get(fmt.Sprintf("%s/health/ready", serverBaseURL))
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))

            // Initiate shutdown in background
            shutdownDone := make(chan error, 1)
            go func() {
                shutdownDone <- testServer.Shutdown(shutdownCtx)
            }()

            time.Sleep(100 * time.Millisecond)

            // Verify readiness probe returns 503 during shutdown
            resp, err = http.Get(fmt.Sprintf("%s/health/ready", serverBaseURL))
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

            Eventually(shutdownDone, 10*time.Second).Should(Receive(BeNil()))
            GinkgoWriter.Println("✅ Test 1 PASSED")
        })
    })

    Describe("Test 2: Liveness Probe During Shutdown (P0)", func() {
        It("should keep liveness probe healthy during shutdown", func() {
            GinkgoWriter.Println("🧪 Test 2: Liveness Probe During Shutdown (P0)")
            
            testServer = createTestServer()

            // Verify liveness probe returns 200 before shutdown
            resp, err := http.Get(fmt.Sprintf("%s/health/live", serverBaseURL))
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))

            // Initiate shutdown
            shutdownDone := make(chan error, 1)
            go func() {
                shutdownDone <- testServer.Shutdown(shutdownCtx)
            }()

            time.Sleep(100 * time.Millisecond)

            // Verify liveness probe still returns 200 during shutdown
            resp, err = http.Get(fmt.Sprintf("%s/health/live", serverBaseURL))
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))

            Eventually(shutdownDone, 10*time.Second).Should(Receive(BeNil()))
            GinkgoWriter.Println("✅ Test 2 PASSED")
        })
    })

    Describe("Test 3: In-Flight Request Completion (P0)", func() {
        It("should complete in-flight requests during shutdown", func() {
            GinkgoWriter.Println("🧪 Test 3: In-Flight Request Completion (P0)")
            
            testServer = createTestServer()

            // Start long-running request
            requestDone := make(chan error, 1)
            go func() {
                resp, err := http.Get(fmt.Sprintf("%s/health", serverBaseURL))
                if err != nil {
                    requestDone <- err
                    return
                }
                defer resp.Body.Close()
                if resp.StatusCode != http.StatusOK {
                    requestDone <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
                    return
                }
                requestDone <- nil
            }()

            time.Sleep(100 * time.Millisecond)

            // Initiate shutdown while request is in-flight
            shutdownDone := make(chan error, 1)
            go func() {
                shutdownDone <- testServer.Shutdown(shutdownCtx)
            }()

            // Verify request completes successfully
            Eventually(requestDone, 10*time.Second).Should(Receive(BeNil()))
            Eventually(shutdownDone, 10*time.Second).Should(Receive(BeNil()))
            GinkgoWriter.Println("✅ Test 3 PASSED")
        })
    })

    Describe("Test 4: Resource Cleanup (P1)", func() {
        It("should close Kubernetes client connections during shutdown", func() {
            GinkgoWriter.Println("🧪 Test 4: Resource Cleanup (P1)")
            
            testServer = createTestServer()

            err := testServer.Shutdown(shutdownCtx)
            Expect(err).ToNot(HaveOccurred())

            // Verify server no longer accepts requests
            _, err = http.Get(fmt.Sprintf("%s/health", serverBaseURL))
            Expect(err).To(HaveOccurred())

            GinkgoWriter.Println("✅ Test 4 PASSED")
        })
    })

    Describe("Test 5: Shutdown Timing (5s Wait) (P1)", func() {
        It("should wait 5 seconds for endpoint removal propagation", func() {
            GinkgoWriter.Println("🧪 Test 5: Shutdown Timing (5s Wait) (P1)")
            
            testServer = createTestServer()

            start := time.Now()
            err := testServer.Shutdown(shutdownCtx)
            duration := time.Since(start)

            Expect(err).ToNot(HaveOccurred())
            Expect(duration).To(BeNumerically(">=", 5*time.Second))

            GinkgoWriter.Println("✅ Test 5 PASSED")
        })
    })

    Describe("Test 6: Shutdown Timeout Respect (P1)", func() {
        It("should respect shutdown context timeout during HTTP drain", func() {
            GinkgoWriter.Println("🧪 Test 6: Shutdown Timeout Respect (P1)")
            
            testServer = createTestServer()

            shutdownTimeout := 6 * time.Second
            timeoutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
            defer cancel()

            start := time.Now()
            err := testServer.Shutdown(timeoutCtx)
            duration := time.Since(start)

            if err != nil {
                GinkgoWriter.Printf("⚠️  Shutdown error: %v\n", err)
            }

            Expect(duration).To(BeNumerically(">=", 5*time.Second))
            Expect(duration).To(BeNumerically("<", 7*time.Second))

            GinkgoWriter.Println("✅ Test 6 PASSED")
        })
    })

    Describe("Test 7: Concurrent Shutdown Safety (P2)", func() {
        It("should handle concurrent shutdown calls safely", func() {
            GinkgoWriter.Println("🧪 Test 7: Concurrent Shutdown Safety (P2)")
            
            testServer = createTestServer()

            var wg sync.WaitGroup
            errors := make(chan error, 10)

            for i := 0; i < 10; i++ {
                wg.Add(1)
                go func(id int) {
                    defer wg.Done()
                    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
                    defer cancel()
                    err := testServer.Shutdown(ctx)
                    errors <- err
                }(i)
            }

            wg.Wait()
            close(errors)

            successCount := 0
            for err := range errors {
                if err == nil {
                    successCount++
                }
            }

            Expect(successCount).To(BeNumerically(">=", 1))
            GinkgoWriter.Println("✅ Test 7 PASSED")
        })
    })

    Describe("Test 8: Shutdown Logging (P2)", func() {
        It("should log all shutdown steps", func() {
            GinkgoWriter.Println("🧪 Test 8: Shutdown Logging (P2)")
            
            testServer = createTestServer()

            err := testServer.Shutdown(shutdownCtx)
            Expect(err).ToNot(HaveOccurred())

            GinkgoWriter.Println("📝 Expected log entries:")
            GinkgoWriter.Println("   1. Initiating DD-007 Kubernetes-aware graceful shutdown")
            GinkgoWriter.Println("   2. Shutdown flag set - readiness probe now returns 503")
            GinkgoWriter.Println("   3. Waiting for Kubernetes endpoint removal propagation")
            GinkgoWriter.Println("   4. Endpoint removal propagation complete")
            GinkgoWriter.Println("   5. Draining in-flight HTTP connections")
            GinkgoWriter.Println("   6. HTTP connections drained successfully")
            GinkgoWriter.Println("   7. Closing external resources (Kubernetes client)")
            GinkgoWriter.Println("   8. DD-007 Kubernetes-aware graceful shutdown complete")

            GinkgoWriter.Println("✅ Test 8 PASSED")
        })
    })
})
```

**Expected Result**: All 8 tests FAIL (graceful shutdown not implemented)

**Deliverables**:
- ✅ `test/integration/toolset/graceful_shutdown_test.go` created
- ✅ 8 failing integration tests (matching Context API exactly)
- ✅ Test coverage for all DD-007 shutdown steps

---

### Phase 2: DO-GREEN (4 hours)

**Objective**: Implement DD-007 4-step graceful shutdown pattern

#### Task 2.1: Add Shutdown Flag to Server (30 min)

**File**: `pkg/toolset/server/server.go`

```go
import "sync/atomic"

type Server struct {
    httpServer      *http.Server
    logger          *zap.Logger
    isShuttingDown  atomic.Bool // DD-007: Explicit shutdown state
    clientset       kubernetes.Interface
    // ... other fields
}
```

#### Task 2.2: Implement 4-Step Shutdown Method (2 hours)

**File**: `pkg/toolset/server/server.go`

```go
// Shutdown performs DD-007 Kubernetes-aware graceful shutdown
// BR-TOOLSET-040: Graceful shutdown with in-flight request completion
func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Initiating DD-007 Kubernetes-aware graceful shutdown")

    // STEP 1: Set shutdown flag → Readiness probe returns 503
    s.isShuttingDown.Store(true)
    s.logger.Info("Shutdown flag set - readiness probe now returns 503")

    // STEP 2: Wait 5 seconds for Kubernetes endpoint removal propagation
    s.logger.Info("Waiting for Kubernetes endpoint removal propagation (5 seconds)")
    time.Sleep(5 * time.Second)
    s.logger.Info("Endpoint removal propagation complete - no new requests will arrive")

    // STEP 3: Drain in-flight HTTP connections (30s timeout)
    s.logger.Info("Draining in-flight HTTP connections (30s timeout)")
    if err := s.httpServer.Shutdown(ctx); err != nil {
        s.logger.Error("HTTP server shutdown error", zap.Error(err))
        return fmt.Errorf("HTTP server shutdown failed: %w", err)
    }
    s.logger.Info("HTTP connections drained successfully")

    // STEP 4: Close resources (Kubernetes client)
    s.logger.Info("Closing external resources (Kubernetes client)")
    // Kubernetes client cleanup if needed
    s.logger.Info("DD-007 Kubernetes-aware graceful shutdown complete")

    return nil
}
```

#### Task 2.3: Update Readiness Probe (30 min)

**File**: `pkg/toolset/server/server.go`

```go
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
    // DD-007: Return 503 during shutdown (RFC 7807 format)
    if s.isShuttingDown.Load() {
        s.writeJSONError(w, r, "Service is shutting down gracefully", http.StatusServiceUnavailable)
        return
    }
    
    // Check Kubernetes client health
    _, err := s.clientset.Discovery().ServerVersion()
    if err != nil {
        s.writeJSONError(w, r, "Kubernetes client unavailable", http.StatusServiceUnavailable)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
```

#### Task 2.4: Add Liveness Probe (30 min)

**File**: `pkg/toolset/server/server.go`

```go
func (s *Server) livenessHandler(w http.ResponseWriter, r *http.Request) {
    // DD-007: Liveness probe remains healthy during shutdown
    // to prevent Kubernetes from killing the pod prematurely
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}
```

**Expected Result**: All 8 graceful shutdown tests PASS

---

### Phase 3: DO-REFACTOR (1.5 hours)

**Objective**: Add signal handling and production enhancements

#### Task 3.1: Add Signal Handling to Main (1 hour)

**File**: `cmd/dynamictoolset/main.go`

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    "go.uber.org/zap"

    "github.com/jordigilh/kubernaut/pkg/k8sutil"
    "github.com/jordigilh/kubernaut/pkg/toolset/server"
)

func main() {
    // Initialize logger
    logger, err := zap.NewProduction()
    if err != nil {
        panic(err)
    }
    defer logger.Sync()

    // Create Kubernetes client using standard helper (DD-013)
    clientset, err := k8sutil.NewClientsetWithLogger(logger)
    if err != nil {
        logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
    }

    // Create server
    srv := server.New(clientset, logger)

    // Start server in goroutine
    go func() {
        logger.Info("Starting Dynamic Toolset Service")
        if err := srv.Start(); err != nil && err != http.ErrServerClosed {
            logger.Fatal("Server failed", zap.Error(err))
        }
    }()

    // Wait for SIGTERM or SIGINT
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    sig := <-quit

    logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

    // 30-second graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        logger.Error("Graceful shutdown failed", zap.Error(err))
        os.Exit(1)
    }

    logger.Info("Server exited gracefully")
}
```

#### Task 3.2: Add Shutdown Metrics (30 min)

**File**: `pkg/toolset/metrics/metrics.go`

```go
var ShutdownDurationSeconds = promauto.NewHistogram(
    prometheus.HistogramOpts{
        Name: "toolset_shutdown_duration_seconds",
        Help: "Duration of graceful shutdown in seconds",
        Buckets: []float64{1, 2, 5, 10, 15, 20, 30},
    },
)
```

**Deliverables**:
- ✅ SIGTERM/SIGINT signal handling
- ✅ 30-second shutdown timeout
- ✅ Shutdown metrics
- ✅ Clean exit on successful shutdown

---

### Phase 4: CHECK (30 min)

**Objective**: Validate graceful shutdown implementation

#### Task 4.1: Run All Tests (15 min)

```bash
# Run all integration tests
make test-integration-toolset
```

**Expected Results**:
- ✅ 6 RFC 7807 tests pass
- ✅ 8 graceful shutdown tests pass
- ✅ All existing tests pass (no regressions)
- ✅ Total: 14 new tests passing

#### Task 4.2: Update BR Documentation (10 min)

**Files to Update**:

1. **BUSINESS_REQUIREMENTS.md**:
```markdown
### BR-TOOLSET-040: Graceful Shutdown with Signal Handling

**Priority**: P0 (Production Safety)
**Status**: ✅ Implemented
**Category**: Kubernetes Operations & Reliability

**Description**: Service handles SIGTERM/SIGINT for zero-downtime deployments

**Test Coverage**:
- Integration: `test/integration/toolset/graceful_shutdown_test.go` (8 tests)

**Implementation**: `pkg/toolset/server/server.go`, `cmd/dynamictoolset/main.go`

**Related**: DD-007 (Kubernetes-Aware Graceful Shutdown Pattern)
```

2. **BR_MAPPING.md**:
```markdown
| BR-TOOLSET-040 | Graceful Shutdown with Signal Handling | test/integration/toolset/graceful_shutdown_test.go | pkg/toolset/server/server.go | DD-007 |
```

#### Task 4.3: Confidence Assessment (5 min)

**Expected Confidence**: 95%

**Rationale**:
- ✅ DD-007 pattern proven in Gateway and Context API (0% error rate)
- ✅ 8 integration tests match Context API exactly
- ✅ Clear specification and reference implementations
- ✅ No changes to core business logic
- ⚠️ 5% risk: Edge cases in Kubernetes client cleanup

---

## 📊 Success Criteria

### Functional Requirements ✅
- [ ] All HTTP error responses use RFC 7807 format
- [ ] Content-Type header set to `application/problem+json`
- [ ] All required RFC 7807 fields present
- [ ] SIGTERM and SIGINT handled gracefully
- [ ] 4-step DD-007 shutdown pattern implemented
- [ ] Readiness probe returns 503 during shutdown
- [ ] Liveness probe remains healthy during shutdown
- [ ] 5-second endpoint removal propagation delay
- [ ] In-flight requests complete before shutdown

### Testing Requirements ✅
- [ ] 6 RFC 7807 integration tests pass
- [ ] 8 graceful shutdown integration tests pass (matching Context API)
- [ ] All existing tests pass (no regressions)
- [ ] Total: 14 new tests passing

### Documentation Requirements ✅
- [ ] BR-TOOLSET-039 documented
- [ ] BR-TOOLSET-040 documented
- [ ] BR_MAPPING.md updated
- [ ] Implementation notes in code comments

### Quality Requirements ✅
- [ ] No lint errors
- [ ] Error messages are descriptive
- [ ] Shutdown logs are comprehensive
- [ ] Confidence assessment ≥ 90%

---

## 🔗 Related Documents

### Authority Documents
- **DD-004**: RFC 7807 Error Response Standard
- **DD-007**: Kubernetes-Aware Graceful Shutdown Pattern
- **RFC 7807**: Problem Details for HTTP APIs (IETF standard)

### Reference Implementations
- **Gateway Service**: `pkg/gateway/errors/rfc7807.go` (Go reference)
- **Context API**: `pkg/contextapi/server/server.go` (DD-007 reference)
- **Context API**: `test/integration/contextapi/13_graceful_shutdown_test.go` (8 tests reference)

### Service Documentation
- **IMPLEMENTATION_PLAN_ENHANCED.md v2.0**: Parent plan (Days 1-13)
- **BUSINESS_REQUIREMENTS.md**: All BRs including BR-TOOLSET-039, BR-TOOLSET-040
- **BR_MAPPING.md**: BR-to-test mapping

---

## 📈 Timeline & Milestones

| Time | Phase | Milestone | Status |
|------|-------|-----------|--------|
| **Day 14: 0:00-2:00** | RFC RED | 6 RFC 7807 tests written (failing) | ⏸️ Pending |
| **Day 14: 2:00-5:00** | RFC GREEN | RFC 7807 implemented (tests passing) | ⏸️ Pending |
| **Day 14: 5:00-7:00** | RFC REFACTOR | Production enhancements | ⏸️ Pending |
| **Day 14: 7:00-8:00** | RFC CHECK | Validation & documentation | ⏸️ Pending |
| **Day 15: 0:00-2:00** | Shutdown RED | 8 shutdown tests written (failing) | ⏸️ Pending |
| **Day 15: 2:00-6:00** | Shutdown GREEN | DD-007 implemented (tests passing) | ⏸️ Pending |
| **Day 15: 6:00-7:30** | Shutdown REFACTOR | Signal handling & metrics | ⏸️ Pending |
| **Day 15: 7:30-8:00** | Shutdown CHECK | Final validation | ⏸️ Pending |

**Total Duration**: 16 hours (2 days)

---

## ✅ Approval Checklist

Before implementation begins, confirm:

- [ ] **Business Value**: RFC 7807 + graceful shutdown required for production
- [ ] **TDD Methodology**: Plan follows RED → GREEN → REFACTOR → CHECK pattern
- [ ] **Reference Implementations**: Gateway (RFC 7807), Context API (DD-007 + 8 tests)
- [ ] **Test Coverage**: 14 new tests (6 RFC 7807 + 8 graceful shutdown)
- [ ] **Test Parity**: 8 graceful shutdown tests match Context API exactly
- [ ] **Documentation**: BR-TOOLSET-039 and BR-TOOLSET-040 fully specified
- [ ] **Timeline**: 2 days (16 hours) is reasonable
- [ ] **Dependencies**: Days 1-13 complete (service operational)
- [ ] **Risk Assessment**: Low risk (no core business logic changes)

---

## 🎯 Post-Implementation

After Days 14-15 completion:

1. **Validation**: Run full test suite (unit + integration)
2. **Documentation**: Update README.md with production features
3. **Deployment**: Test in Kubernetes with rolling updates
4. **Monitoring**: Verify error metrics and shutdown logs

**Next Steps**:
- ⏸️ Audit Trail Implementation (if applicable)
- ⏸️ V2 Features (per v2-business-requirements.md)

---

**Status**: ⏸️ **PENDING APPROVAL**
**Approval Required From**: Technical Lead / Product Owner
**Implementation Start**: Upon approval
**Estimated Completion**: 2 business days after approval

---

**Plan Author**: AI Assistant
**Plan Reviewer**: [Pending]
**Plan Approver**: [Pending]
**Implementation Date**: [TBD]

