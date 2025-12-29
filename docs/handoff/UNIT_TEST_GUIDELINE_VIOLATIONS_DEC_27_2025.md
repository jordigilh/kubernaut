# Unit Test Guideline Violations - System-Wide Triage

**Date**: 2025-12-27
**Status**: 🔴 **VIOLATIONS FOUND** - 4 major violations, 2 borderline cases
**Scope**: All services - Go and Python unit tests
**Impact**: HIGH - Tests validate dependencies instead of business logic
**Reference**: [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)

---

## Executive Summary

System-wide triage of unit tests found **3 major guideline violations** where unit tests validate external dependencies (HolmesGPT client, audit infrastructure, Redis client) instead of service business logic.

### Key Findings

| Violation | Service | Test File | Impact |
|-----------|---------|-----------|--------|
| **❌ CRITICAL** | AIAnalysis | `test/unit/aianalysis/holmesgpt_client_test.go` | Testing HolmesGPT client HTTP behavior |
| **❌ CRITICAL** | HolmesGPT-API | `holmesgpt-api/tests/unit/test_llm_audit_integration.py` | Testing audit infrastructure, not HAPI logic |
| **❌ CRITICAL** | Cache (shared) | `test/unit/cache/redis_client_test.go` | Testing Redis client connection management |
| **❌ CRITICAL** | DataStorage | `test/unit/datastorage/dlq/client_test.go` | Testing DLQ client with embedded Redis |
| **⚠️ BORDERLINE** | AIAnalysis | `test/unit/aianalysis/audit_client_test.go` | Testing audit adapter (acceptable, but low value) |
| **✅ ACCEPTABLE** | Audit (shared) | `test/unit/audit/openapi_client_adapter_test.go` | Testing adapter business logic |
| **✅ CORRECT** | Gateway | `test/unit/gateway/adapters/prometheus_adapter_test.go` | Testing adapter business logic |
| **✅ CORRECT** | Notification | `test/unit/notification/routing_integration_test.go` | Testing routing logic (unit test despite name) |

### Guideline Reference

From [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md#when-to-use-unit-tests):

> **✅ Use Unit Tests For:**
> 1. Function/Method Behavior
> 2. Error Handling & Edge Cases
> 3. Internal Logic Validation
> 4. Interface Compliance
>
> **❌ Don't Use Unit Tests For:**
> 1. Business Value Validation (use business requirement tests)
> 2. **End-to-End Workflows (use integration/E2E tests)**
> 3. **External Dependencies (mock them, don't test them)**

---

## Detailed Violations

### ❌ VIOLATION #1: Testing HolmesGPT Client Instead of AIAnalysis Logic

**File**: `test/unit/aianalysis/holmesgpt_client_test.go` (170 lines)
**Service**: AIAnalysis
**Severity**: 🔴 **CRITICAL**

#### What's Being Tested (WRONG)

```go
// ❌ WRONG: Testing HolmesGPT client HTTP behavior
var _ = Describe("HolmesGPTClient", func() {
    var (
        mockServer *httptest.Server
        hgClient   *client.HolmesGPTClient  // ← Testing external client
    )

    Context("with successful response", func() {
        BeforeEach(func() {
            mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                Expect(r.URL.Path).To(Equal("/api/v1/incident/analyze"))  // ← Testing HTTP path
                Expect(r.Method).To(Equal(http.MethodPost))                // ← Testing HTTP method
                // ... mock HTTP response
            }))
            hgClient = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
        })

        It("should return valid response", func() {
            resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{...})
            // ← Testing client.Investigate(), NOT AIAnalysis business logic
        })
    })
})
```

**Why This is Wrong**:
1. **Tests External Client**: Validates `pkg/holmesgpt/client.HolmesGPTClient` behavior, not AIAnalysis controller logic
2. **Tests HTTP Mechanics**: Validates HTTP status codes (503, 401, 400, 429), request paths, methods
3. **Wrong Ownership**: This test belongs in `pkg/holmesgpt/client/client_test.go`, not AIAnalysis unit tests
4. **Missing AIAnalysis Logic**: NO tests for how AIAnalysis USES the client (e.g., how it processes responses, handles retries, emits metrics)

#### What SHOULD Be Tested Instead

```go
// ✅ CORRECT: Test AIAnalysis business logic that USES HolmesGPT client
var _ = Describe("InvestigatingHandler", func() {
    var (
        mockHGClient  *MockHolmesGPTClient  // Mock the CLIENT, don't test it
        handler       *InvestigatingHandler
    )

    Context("when HolmesGPT returns high-confidence analysis", func() {
        BeforeEach(func() {
            mockHGClient = &MockHolmesGPTClient{
                Response: &client.IncidentResponse{
                    Analysis:   "Root cause: OOM",
                    Confidence: 0.92,
                },
            }
            handler = NewInvestigatingHandler(mockHGClient, ...)
        })

        It("should transition to Completed phase", func() {
            // Test AIAnalysis BUSINESS LOGIC: phase transitions
            analysis := &aianalysisv1.AIAnalysis{...}
            result, err := handler.Handle(ctx, analysis)

            Expect(err).NotTo(HaveOccurred())
            Expect(result.Phase).To(Equal("Completed"))  // ← Business logic
            Expect(result.ApprovalRequired).To(BeFalse()) // ← Business logic
        })

        It("should emit analysis_complete metric", func() {
            // Test AIAnalysis BUSINESS LOGIC: metrics emission
            analysis := &aianalysisv1.AIAnalysis{...}
            handler.Handle(ctx, analysis)

            // Verify metrics were called (business logic integration)
            Expect(mockMetrics.AnalysisCompleteCallCount()).To(Equal(1))
        })
    })

    Context("when HolmesGPT returns low-confidence analysis", func() {
        It("should require manual approval", func() {
            // Test AIAnalysis BUSINESS LOGIC: approval decision logic
            mockHGClient.Response.Confidence = 0.65

            analysis := &aianalysisv1.AIAnalysis{...}
            result, err := handler.Handle(ctx, analysis)

            Expect(result.ApprovalRequired).To(BeTrue())  // ← Business logic
        })
    })
})
```

**Key Difference**:
- ❌ **Wrong**: Test HolmesGPT client HTTP behavior
- ✅ **Correct**: Test AIAnalysis business logic (phase transitions, approval decisions, metrics) that USES HolmesGPT client

#### Recommendation

**Action**: Move existing tests to `pkg/holmesgpt/client/client_test.go` (where they belong)
**Priority**: P1 (High) - Currently no unit tests for AIAnalysis's core business logic
**Effort**: ~4 hours
- 2 hours: Move existing tests to correct location
- 2 hours: Write new tests for AIAnalysis business logic (using mocked client)

---

### ❌ VIOLATION #2: Testing Audit Infrastructure Instead of HAPI Logic

**File**: `holmesgpt-api/tests/unit/test_llm_audit_integration.py` (224 lines)
**Service**: HolmesGPT-API (Python)
**Severity**: 🔴 **CRITICAL**

#### What's Being Tested (WRONG)

```python
# ❌ WRONG: Testing BufferedAuditStore infrastructure
class TestLLMAuditIntegration:
    def test_buffered_audit_store_initialization(self):
        """BR-AUDIT-005: BufferedAuditStore can be initialized with config"""
        config = AuditConfig(buffer_size=1000, batch_size=10, ...)
        store = BufferedAuditStore(data_storage_url="...", config=config)

        # ← Testing audit store initialization (infrastructure)
        assert store is not None
        assert store._config.buffer_size == 1000  # ← Testing infrastructure config

    def test_store_audit_event_non_blocking(self):
        """ADR-038: store_audit() must be non-blocking (fire-and-forget)"""
        store = BufferedAuditStore(...)
        result = store.store_audit(event)

        # ← Testing buffering behavior (infrastructure)
        assert result is True
        assert store._queue.qsize() == 1  # ← Testing internal queue state

    def test_llm_request_audit_event_structure(self):
        """BR-AUDIT-005 + ADR-034: LLM request audit events have correct structure"""
        event = create_llm_request_audit_event(...)

        # ← Testing event structure validation (infrastructure)
        assert "version" in event
        assert event["version"] == "1.0"
        assert "event_category" in event
        # ... more structure validation
```

**Why This is Wrong**:
1. **Tests Audit Infrastructure**: Validates `BufferedAuditStore` behavior (buffering, batching, non-blocking), not HAPI business logic
2. **Tests Event Structure**: Validates ADR-034 compliance (audit event schema), which is infrastructure concern
3. **Wrong Ownership**: These tests belong in `src/audit/` as unit tests for BufferedAuditStore, not HAPI unit tests
4. **Missing HAPI Logic**: NO tests for how HAPI USES audit (e.g., when to emit audit events, what data to include)

#### What SHOULD Be Tested Instead

```python
# ✅ CORRECT: Test HAPI business logic that USES audit
class TestIncidentAnalysisAudit:
    """Test how HAPI emits audit events during incident analysis"""

    def test_incident_analysis_emits_audit_event(self, mock_audit_store):
        """BR-HAPI-XXX: Incident analysis should emit audit event with analysis results"""
        # Setup
        client = create_test_client(audit_store=mock_audit_store)

        # Act: Business operation (incident analysis)
        response = client.post("/api/v1/incident/analyze", json={
            "incident_id": "test-001",
            "signal_type": "OOMKilled",
            ...
        })

        # Assert: HAPI business logic - audit was emitted
        assert response.status_code == 200
        assert mock_audit_store.store_audit.call_count == 1  # ← Business logic

        # Verify HAPI included correct data in audit event
        audit_event = mock_audit_store.store_audit.call_args[0][0]
        assert audit_event["event_type"] == "llm_response"  # ← Business logic
        assert audit_event["correlation_id"] == "test-001"  # ← Business logic
        assert "analysis" in audit_event["event_data"]      # ← Business logic

    def test_failed_analysis_emits_error_audit(self, mock_audit_store):
        """BR-HAPI-XXX: Failed analysis should emit audit event with error details"""
        # Test HAPI business logic: error handling with audit
        client = create_test_client(audit_store=mock_audit_store)

        # Mock LLM failure
        with patch('src.llm.client.analyze', side_effect=TimeoutError("LLM timeout")):
            response = client.post("/api/v1/incident/analyze", json={...})

        # Verify HAPI emitted audit event for error (business logic)
        assert mock_audit_store.store_audit.call_count == 1
        audit_event = mock_audit_store.store_audit.call_args[0][0]
        assert audit_event["event_outcome"] == "failure"
        assert "timeout" in str(audit_event["event_data"]["error"]).lower()
```

**Key Difference**:
- ❌ **Wrong**: Test audit infrastructure (BufferedAuditStore buffering, event structure validation)
- ✅ **Correct**: Test HAPI business logic (when audit events are emitted, what data is included)

#### Recommendation

**Action**: Move infrastructure tests to `src/audit/test_buffered_store.py`, create HAPI business logic tests
**Priority**: P1 (High) - Currently no unit tests for HAPI's audit integration logic
**Effort**: ~3 hours
- 1 hour: Move existing tests to `src/audit/`
- 2 hours: Write new tests for HAPI business logic (using mocked audit store)

---

### ❌ VIOLATION #3: Testing Redis Client Instead of Cache Business Logic

**File**: `test/unit/cache/redis_client_test.go` (~200+ lines)
**Service**: Shared (pkg/cache/redis)
**Severity**: 🔴 **CRITICAL**

#### What's Being Tested (WRONG)

```go
// ❌ WRONG: Testing Redis client connection management
var _ = Describe("Redis Client", func() {
    var (
        miniRedis *miniredis.Miniredis  // ← Embedded Redis for testing
        client    *rediscache.Client
    )

    BeforeEach(func() {
        // Start embedded Redis server
        miniRedis, err = miniredis.Run()
        redisAddr = miniRedis.Addr()
    })

    Describe("EnsureConnection", func() {
        It("should establish connection on first call", func() {
            client = rediscache.NewClient(opts, logger)
            err := client.EnsureConnection(ctx)  // ← Testing connection logic
            Expect(err).ToNot(HaveOccurred())
        })

        It("should use fast path on subsequent calls", func() {
            // ← Testing connection pooling behavior
        })
    })
})
```

**Why This is Wrong**:
1. **Tests Redis Client Infrastructure**: Validates connection management, connection pooling, reconnection logic
2. **Uses Embedded Redis**: Spins up miniredis (embedded Redis) to test client behavior
3. **Wrong Scope**: Unit tests should mock Redis, not test the client wrapper
4. **Missing Service Logic**: NO tests for how SERVICES use the cache (e.g., Gateway deduplication logic)

#### What SHOULD Be Tested Instead

```go
// ✅ CORRECT: Test service business logic that USES Redis cache
var _ = Describe("Gateway Deduplication Logic", func() {
    var (
        mockCache     *MockRedisCache  // Mock the CACHE, don't test it
        deduplicator  *Deduplicator
    )

    Context("when alert fingerprint is cached", func() {
        BeforeEach(func() {
            mockCache = &MockRedisCache{
                Has: func(key string) bool { return true },  // Mock cache hit
            }
            deduplicator = NewDeduplicator(mockCache, ...)
        })

        It("should skip duplicate alert", func() {
            // Test Gateway BUSINESS LOGIC: deduplication decision
            signal := &Signal{Fingerprint: "oom-pod-123"}
            result := deduplicator.ShouldProcess(signal)

            Expect(result.IsDuplicate).To(BeTrue())     // ← Business logic
            Expect(result.Reason).To(Equal("cached"))   // ← Business logic
        })

        It("should emit duplicate_skipped metric", func() {
            // Test Gateway BUSINESS LOGIC: metrics emission
            signal := &Signal{Fingerprint: "oom-pod-123"}
            deduplicator.ShouldProcess(signal)

            // Verify metrics were called (business logic integration)
            Expect(mockMetrics.DuplicatesSkippedCount()).To(Equal(1))
        })
    })

    Context("when cache is unavailable", func() {
        It("should fail open and process alert", func() {
            // Test Gateway BUSINESS LOGIC: graceful degradation
            mockCache.Has = func(key string) bool { panic("cache down") }

            signal := &Signal{Fingerprint: "oom-pod-123"}
            result := deduplicator.ShouldProcess(signal)

            // Gateway should fail open (process alert despite cache error)
            Expect(result.IsDuplicate).To(BeFalse())  // ← Business logic
            Expect(result.Reason).To(Equal("cache_unavailable"))  // ← Business logic
        })
    })
})
```

**Key Difference**:
- ❌ **Wrong**: Test Redis client connection management (infrastructure)
- ✅ **Correct**: Test service business logic (Gateway deduplication) that USES Redis cache

#### Recommendation

**Action**: Keep existing tests in `pkg/cache/redis/` (they're testing the client wrapper), but they're NOT unit tests for services
**Note**: These tests are useful for validating the Redis client wrapper itself, but services should NOT have unit tests that test Redis client behavior
**Priority**: P2 (Medium) - Existing tests are useful for client library, just misclassified
**Effort**: ~1 hour - Document that these are client library tests, not service unit tests

---

### ❌ VIOLATION #4: Testing DLQ Client Instead of DataStorage Business Logic

**File**: `test/unit/datastorage/dlq/client_test.go` (~694 lines)
**Service**: DataStorage
**Severity**: 🔴 **CRITICAL**

#### What's Being Tested (WRONG)

```go
// ❌ WRONG: Testing DLQ client with embedded Redis
var _ = Describe("DD-009: Dead Letter Queue Client", func() {
    var (
        miniRedis   *miniredis.Miniredis  // ← Embedded Redis for testing
        redisClient *redis.Client
        dlqClient   *dlq.Client
    )

    BeforeEach(func() {
        // Start embedded Redis server
        miniRedis = miniredis.RunT(GinkgoT())

        // Create Redis client
        redisClient = redis.NewClient(&redis.Options{
            Addr: miniRedis.Addr(),
        })

        // Create DLQ client
        dlqClient, err = dlq.NewClient(redisClient, logger, 10000)
    })

    Context("EnqueueAuditEvent - Success Path", func() {
        It("should successfully enqueue audit event to Redis Stream", func() {
            // ← Testing DLQ client infrastructure behavior
            auditEvent := &audit.AuditEvent{...}
            err := dlqClient.EnqueueAuditEvent(ctx, auditEvent)

            Expect(err).ToNot(HaveOccurred())  // ← Testing client works
        })
    })
})
```

**Why This is Wrong**:
1. **Tests DLQ Client Infrastructure**: Validates how the DLQ client enqueues to Redis, not DataStorage business logic
2. **Uses Embedded Redis**: Spins up miniredis to test client behavior (infrastructure testing)
3. **Wrong Scope**: Unit tests should mock Redis/DLQ, not test the client wrapper
4. **Missing DataStorage Logic**: NO tests for how DataStorage USES the DLQ (e.g., when to enqueue, retry logic)

#### What SHOULD Be Tested Instead

```go
// ✅ CORRECT: Test DataStorage business logic that USES DLQ
var _ = Describe("DataStorage DLQ Fallback Logic", func() {
    var (
        mockDLQClient *MockDLQClient  // Mock the CLIENT, don't test it
        auditHandler  *AuditHandler
        mockDB        *MockDatabase
    )

    Context("when database is unavailable", func() {
        BeforeEach(func() {
            mockDB = &MockDatabase{
                WriteError: errors.New("database connection failed"),
            }
            mockDLQClient = &MockDLQClient{}
            auditHandler = NewAuditHandler(mockDB, mockDLQClient, ...)
        })

        It("should enqueue event to DLQ", func() {
            // Test DataStorage BUSINESS LOGIC: DLQ fallback decision
            event := &audit.AuditEvent{...}
            err := auditHandler.StoreAuditEvent(ctx, event)

            // Verify DataStorage used DLQ fallback (business logic)
            Expect(err).NotTo(HaveOccurred())  // ← Graceful degradation
            Expect(mockDLQClient.EnqueueCallCount()).To(Equal(1))  // ← Used DLQ
            Expect(mockDB.WriteCallCount()).To(Equal(1))  // ← Tried DB first
        })

        It("should emit dlq_enqueue metric", func() {
            // Test DataStorage BUSINESS LOGIC: metrics emission
            event := &audit.AuditEvent{...}
            auditHandler.StoreAuditEvent(ctx, event)

            // Verify metrics were emitted (business logic)
            Expect(mockMetrics.DLQEnqueueCount()).To(Equal(1))
        })
    })

    Context("when database recovers", func() {
        It("should stop using DLQ", func() {
            // Test DataStorage BUSINESS LOGIC: recovery behavior
            mockDB.WriteError = nil  // Database recovered

            event := &audit.AuditEvent{...}
            err := auditHandler.StoreAuditEvent(ctx, event)

            // Verify DataStorage stopped using DLQ (business logic)
            Expect(err).NotTo(HaveOccurred())
            Expect(mockDLQClient.EnqueueCallCount()).To(Equal(0))  // ← No DLQ
            Expect(mockDB.WriteCallCount()).To(Equal(1))  // ← Directly to DB
        })
    })
})
```

**Key Difference**:
- ❌ **Wrong**: Test DLQ client infrastructure (enqueue to Redis, stream operations)
- ✅ **Correct**: Test DataStorage business logic (when to use DLQ, fallback decisions, recovery)

#### Recommendation

**Action**: Keep existing tests in `pkg/datastorage/dlq/` (they're testing the DLQ client wrapper), create DataStorage business logic tests
**Priority**: P1 (High) - Currently no unit tests for DataStorage's DLQ fallback logic
**Effort**: ~3 hours
- 1 hour: Move existing tests to `pkg/datastorage/dlq/client_test.go` (correct location)
- 2 hours: Write new tests for DataStorage business logic (using mocked DLQ client)

---

## ⚠️ Borderline Cases (Acceptable but Low Value)

### ⚠️ BORDERLINE: Testing Audit Adapter (Low Value, Questionable)

**File**: `test/unit/aianalysis/audit_client_test.go` (415 lines)
**Service**: AIAnalysis
**Assessment**: ⚠️ **LOW VALUE** - Tests custom wrapper, but redundant with integration tests

#### What's Actually Being Tested

The tests validate `aiaudit.AuditClient`, which is **NOT** the OpenAPI generated client. It's a **custom wrapper** that AIAnalysis wrote:

```go
// pkg/aianalysis/audit/audit.go
type AuditClient struct {
    store audit.AuditStore  // Wraps audit store
    log   logr.Logger
}

// Custom method that creates AIAnalysis-specific audit events
func (c *AuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
    // 1. Extract fields from AIAnalysis CRD
    payload := AnalysisCompletePayload{
        Phase:            analysis.Status.Phase,
        ApprovalRequired: analysis.Status.ApprovalRequired,
        // ... more field mapping
    }

    // 2. Determine outcome (business logic)
    var apiOutcome dsgen.AuditEventRequestEventOutcome
    if analysis.Status.Phase == "Failed" {
        apiOutcome = audit.OutcomeFailure  // ← Business logic
    } else {
        apiOutcome = audit.OutcomeSuccess
    }

    // 3. Populate OpenAPI event (field mapping)
    event := audit.NewAuditEventRequest()
    audit.SetEventType(event, "aianalysis.analysis.completed")
    audit.SetEventOutcome(event, apiOutcome)
    audit.SetEventData(event, payload)

    // 4. Store event (delegates to audit store)
    c.store.StoreAudit(ctx, event)
}
```

#### What the Tests Validate

```go
// test/unit/aianalysis/audit_client_test.go
It("should record analysis completion with all required fields", func() {
    analysis := createTestAnalysis()
    auditClient.RecordAnalysisComplete(ctx, analysis)  // ← Call wrapper

    // Verify wrapper populated fields correctly
    Expect(mockStore.Events).To(HaveLen(1))
    event := mockStore.Events[0]
    Expect(event.EventType).To(Equal("aianalysis.analysis.completed"))  // ← Field mapping
    Expect(event.EventAction).To(Equal("analysis_complete"))            // ← Field mapping
    Expect(event.EventOutcome).To(Equal("success"))                      // ← Business logic
    // ... more field validation
})
```

#### Why This is Low Value

1. ⚠️ **Straightforward Logic**: Field mapping is simple (read CRD field → set audit field)
2. ⚠️ **Redundant Coverage**: Integration tests already validate audit events end-to-end
3. ⚠️ **Not Core Business Logic**: This tests the adapter, not phase transitions or approval decisions
4. ⚠️ **Time Investment**: 415 lines of tests for field mapping logic

**User's Valid Question**: "The audit adapter is generated with OpenAPI CLI, what's the value of testing that?"

**Clarification**:
- The **OpenAPI client** (`dsgen.AuditEventRequest`) is generated → **NO VALUE in testing generated code**
- The **custom wrapper** (`aiaudit.AuditClient`) is handwritten → **SOME VALUE in testing field mapping**
- But integration tests already prove it works → **LOW VALUE for unit tests**

#### Recommendation

**Verdict**: ⚠️ **LOW VALUE** - Tests are technically correct but provide minimal value

**Better Use of Testing Time**:
1. **Delete or reduce**: 415 lines → 50-100 lines (test edge cases only)
2. **Focus instead on**: Core business logic unit tests that DON'T exist yet:
   - Phase transition logic (when to move from Investigating → Completed)
   - Approval decision logic (confidence threshold calculations)
   - Rego policy evaluation logic
   - Error classification logic

**Why Integration Tests Are Sufficient**:
```go
// Integration test proves adapter works end-to-end
It("should emit audit event when analysis completes", func() {
    // Create AIAnalysis CRD
    aianalysis := &aianalysisv1.AIAnalysis{...}
    k8sClient.Create(ctx, aianalysis)

    // Wait for controller to process
    Eventually(func() string {
        k8sClient.Get(ctx, key, aianalysis)
        return aianalysis.Status.Phase
    }).Should(Equal("Completed"))

    // Verify audit event was emitted with correct fields
    events := queryDataStorageAudits(correlationID)
    Expect(events).To(HaveLen(1))
    Expect(events[0].EventType).To(Equal("aianalysis.analysis.completed"))
    // ← This proves the adapter works, making unit tests redundant
})
```

**Priority**: P3 (Low) - Consider reducing or removing these tests to focus on core business logic

---

## ✅ Acceptable Tests (Reference Implementation)

### ✅ CORRECT: Testing Adapter Business Logic

**File**: `test/unit/audit/openapi_client_adapter_test.go` (368 lines)
**Component**: Shared Audit Library
**Assessment**: ✅ **CORRECT** - Tests adapter business logic

#### Why This is Correct

```go
// ✅ CORRECT: Testing adapter's business logic
var _ = Describe("OpenAPIClientAdapter - DD-API-001 Compliance", func() {
    var (
        ctx    context.Context
        server *httptest.Server  // ← Mock DataStorage HTTP endpoint
    )

    Describe("StoreBatch - DD-API-001 Compliance", func() {
        Context("Success Cases", func() {
            It("should successfully write batch with 201 response", func() {
                server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    // Mock DataStorage API response
                    w.WriteHeader(http.StatusCreated)
                    w.Write([]byte(`{"message": "Batch created successfully"}`))
                }))

                client, _ = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)

                // Test adapter BUSINESS LOGIC: how it handles 201 response
                events := []*dsgen.AuditEventRequest{...}
                err := client.StoreBatch(ctx, events)

                Expect(err).ToNot(HaveOccurred())  // ← Adapter business logic
            })
        })

        Context("Error Handling", func() {
            It("should return error on 500 response", func() {
                // Test adapter BUSINESS LOGIC: error classification
                server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    w.WriteHeader(http.StatusInternalServerError)
                }))

                client, _ = audit.NewOpenAPIClientAdapter(server.URL, 5*time.Second)
                err := client.StoreBatch(ctx, events)

                Expect(err).To(HaveOccurred())  // ← Adapter business logic
                Expect(err.Error()).To(ContainSubstring("500"))  // ← Error handling
            })
        })
    })
})
```

**Why This is Correct**:
1. ✅ **Tests Adapter Logic**: Validates how the adapter handles HTTP responses (201, 500, etc.)
2. ✅ **Tests Error Classification**: Validates adapter's error handling business logic
3. ✅ **Appropriate Scope**: This IS the adapter's business logic (adapting OpenAPI client to audit interface)
4. ✅ **Correct Ownership**: Tests belong in `test/unit/audit/` (where the adapter lives)

---

## Summary Matrix

| File | Service | What It Tests | Should Test | Verdict | Priority |
|------|---------|---------------|-------------|---------|----------|
| `test/unit/aianalysis/holmesgpt_client_test.go` | AIAnalysis | HolmesGPT client HTTP behavior | AIAnalysis business logic | ❌ VIOLATION | P1 (High) |
| `holmesgpt-api/tests/unit/test_llm_audit_integration.py` | HAPI | Audit infrastructure (BufferedAuditStore) | HAPI audit integration logic | ❌ VIOLATION | P1 (High) |
| `test/unit/cache/redis_client_test.go` | Cache | Redis client connection management | Service cache usage logic | ❌ VIOLATION | P2 (Medium) |
| `test/unit/datastorage/dlq/client_test.go` | DataStorage | DLQ client with embedded Redis | DataStorage DLQ fallback logic | ❌ VIOLATION | P1 (High) |
| `test/unit/aianalysis/audit_client_test.go` | AIAnalysis | Audit adapter wrapper logic | Core business logic (acceptable) | ⚠️ BORDERLINE | P3 (Low) |
| `test/unit/audit/openapi_client_adapter_test.go` | Audit | Adapter business logic | N/A (correct as-is) | ✅ CORRECT | N/A |
| `test/unit/gateway/adapters/prometheus_adapter_test.go` | Gateway | Adapter business logic (fingerprinting) | N/A (correct as-is) | ✅ CORRECT | N/A |
| `test/unit/notification/routing_integration_test.go` | Notification | Routing logic (unit test) | N/A (correct as-is) | ✅ CORRECT | N/A |

---

## Recommendations

### Immediate Actions (P1 - High Priority)

1. **Move `holmesgpt_client_test.go` to `pkg/holmesgpt/client/`**
   - Rename to `client_test.go` (these ARE tests for the client library)
   - Create NEW unit tests for AIAnalysis business logic:
     - `analyzing_handler_test.go` - Test phase transition logic
     - `approval_decision_test.go` - Test approval decision logic
     - `metrics_integration_test.go` - Test metrics emission logic

2. **Move `test_llm_audit_integration.py` to `src/audit/`**
   - Rename to `test_buffered_store.py` (these ARE tests for BufferedAuditStore)
   - Create NEW unit tests for HAPI business logic:
     - `test_incident_analysis_audit.py` - Test when audit events are emitted
     - `test_recovery_analysis_audit.py` - Test audit event data content

3. **Move `dlq/client_test.go` to `pkg/datastorage/dlq/`**
   - These ARE tests for the DLQ client wrapper (correct location would be pkg/)
   - Create NEW unit tests for DataStorage business logic:
     - `dlq_fallback_test.go` - Test DLQ fallback decision logic
     - `dlq_recovery_test.go` - Test database recovery behavior

### Medium Priority Actions (P2)

4. **Document `redis_client_test.go` as Client Library Tests**
   - Keep existing tests (useful for Redis client wrapper validation)
   - Document that these are client library tests, NOT service unit tests
   - Services should mock Redis in their own unit tests

### Low Priority Actions (P3)

5. **Review `audit_client_test.go` for Redundancy**
   - Tests are acceptable but low-value (integration tests already cover this)
   - Consider reducing test count, focusing on edge cases only
   - Prioritize testing core business logic (phase transitions, approval decisions)

---

## Guideline Enforcement

### Detection Commands

```bash
# Find unit tests that might be testing external dependencies
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Find Go unit tests testing HTTP clients
grep -r "httptest.NewServer\|http.HandlerFunc" test/unit --include="*_test.go" -l

# Find Go unit tests using miniredis (embedded Redis)
grep -r "miniredis.Run\|miniredis.Miniredis" test/unit --include="*_test.go" -l

# Find Python unit tests testing audit infrastructure
grep -r "BufferedAuditStore\|AuditConfig" holmesgpt-api/tests/unit --include="test_*.py" -l

# Find unit tests with "integration" or "client" in filename (suspicious)
find test/unit -name "*integration*" -o -name "*client*_test.go"
```

### CI Check (Recommended)

```bash
#!/bin/bash
# CI check for unit test guideline violations

echo "🔍 Checking for unit test guideline violations..."

violations=0

# Check 1: Unit tests should not use httptest to test external clients
if grep -r "httptest.NewServer" test/unit/*/holmesgpt*_test.go 2>/dev/null; then
    echo "❌ VIOLATION: Unit tests testing HolmesGPT client (should test business logic)"
    violations=$((violations + 1))
fi

# Check 2: Unit tests should not test audit infrastructure
if grep -r "BufferedAuditStore" holmesgpt-api/tests/unit/test_llm_audit_integration.py 2>/dev/null; then
    echo "❌ VIOLATION: Unit tests testing audit infrastructure (should test HAPI logic)"
    violations=$((violations + 1))
fi

# Check 3: Unit tests should not use embedded services (miniredis, etc.)
if grep -r "miniredis.Run" test/unit --include="*_test.go" 2>/dev/null; then
    echo "⚠️  WARNING: Unit tests using embedded services (should mock dependencies)"
    violations=$((violations + 1))
fi

if [ $violations -gt 0 ]; then
    echo ""
    echo "❌ Found $violations guideline violation(s)"
    echo "   See: docs/handoff/UNIT_TEST_GUIDELINE_VIOLATIONS_DEC_27_2025.md"
    exit 1
fi

echo "✅ No unit test guideline violations found"
```

---

## Next Steps

1. **Immediate**: Address P1 violations (move misplaced tests, create business logic tests)
2. **Short-term**: Implement CI checks to prevent future violations
3. **Long-term**: Document testing best practices in team wiki

**Estimated Total Effort**: ~12 hours
- 4 hours: Move holmesgpt_client_test.go + create AIAnalysis business logic tests
- 3 hours: Move test_llm_audit_integration.py + create HAPI business logic tests
- 3 hours: Move dlq/client_test.go + create DataStorage DLQ fallback tests
- 1 hour: Document redis_client_test.go as client library tests
- 1 hour: Implement CI checks and update documentation

---

**Last Updated**: 2025-12-27
**Status**: 🔴 **VIOLATIONS FOUND** - Remediation plan defined
**Tracking**: UNIT-TEST-VIOLATIONS-001

