# DD-GATEWAY-010: Audit Trace Integration - Implementation Plan

**Version**: 1.0
**Status**: 📋 DRAFT
**Design Decision**: [audit-integration-analysis.md](./audit-integration-analysis.md)
**Service**: Gateway Service
**Confidence**: 95% (Evidence-Based - ADR-038 pattern, existing audit library)
**Estimated Effort**: 6 days (APDC cycle: 3 days implementation + 2 days testing + 1 day documentation)

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | 2025-11-19 | Initial implementation plan created | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-GATEWAY-017** | Audit trace for signal ingestion | All received signals logged to audit_events table with correlation_id |
| **BR-GATEWAY-018** | Audit trace for deduplication decisions | Duplicate signals logged with existing CRD correlation |
| **BR-GATEWAY-019** | Audit trace for state-based deduplication | K8s API queries logged with success/failure outcome |
| **BR-GATEWAY-020** | Audit trace for storm detection | Storm events logged with storm_type and alert_count |
| **BR-GATEWAY-021** | Audit trace for storm buffering | Buffered alerts logged with buffer utilization metrics |
| **BR-GATEWAY-022** | Audit trace for storm window extension | Sliding window extensions logged with resource_count |
| **BR-GATEWAY-023** | Audit trace for CRD creation | Individual and aggregated CRDs logged with environment and priority |

### **Success Metrics**

- **Audit Coverage**: 100% of 7 integration points emit audit events
- **Latency Impact**: <0.1ms overhead per signal (fire-and-forget pattern)
- **Audit Throughput**: 10,000 events/sec (batched writes)
- **Data Loss Rate**: <1% (buffer overflow monitoring)
- **Test Coverage**: Unit (70%+), Integration (>50%), E2E (<10%)

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 4 hours | Day 0 (pre-work) | Comprehensive context understanding | ✅ Analysis document (audit-integration-analysis.md), ADR-038 review |
| **PLAN** | 4 hours | Day 0 (pre-work) | Detailed implementation strategy | ✅ This document, TDD phase mapping, 7 integration points identified |
| **DO (Implementation)** | 3 days | Days 1-3 | Controlled TDD execution | Audit store integration, 7 audit calls, configuration |
| **CHECK (Testing)** | 2 days | Days 4-5 | Comprehensive result validation | Unit tests (70%+), integration tests, E2E tests |
| **PRODUCTION READINESS** | 1 day | Day 6 | Documentation & deployment prep | Updated docs, runbook, confidence report |

### **6-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 8h | ✅ Analysis complete (audit-integration-analysis.md), Plan approved (this document) |
| **Day 1** | DO-RED | Foundation + Tests | 8h | Audit store initialization, test framework, 2 audit points (RED) |
| **Day 2** | DO-GREEN | Core audit calls | 8h | 5 remaining audit points implemented (GREEN) |
| **Day 3** | DO-REFACTOR | Integration + Config | 8h | Configuration, graceful shutdown, metrics integration |
| **Day 4** | CHECK | Unit + Integration tests | 8h | 70%+ unit coverage, integration test scenarios |
| **Day 5** | CHECK | E2E tests + BR validation | 8h | E2E test scenarios, BR-GATEWAY-017 to BR-GATEWAY-023 validated |
| **Day 6** | PRODUCTION | Documentation + Readiness | 8h | Service docs updated, runbook created, confidence report |

### **Critical Path Dependencies**

```
Day 1 (Foundation) → Day 2 (Audit Calls) → Day 3 (Integration)
                                           ↓
Day 4 (Unit + Integration Tests) → Day 5 (E2E Tests) → Day 6 (Production)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 1 Complete**: Foundation checkpoint (audit store initialized, 2 audit points RED)
- **Day 2 Complete**: Implementation progress checkpoint (7 audit points GREEN)
- **Day 3 Complete**: Integration complete checkpoint (config, shutdown, metrics)
- **Day 4 Complete**: Testing checkpoint (unit + integration tests passing)
- **Day 5 Complete**: E2E validation checkpoint (all BRs validated)
- **Day 6 Complete**: Production ready checkpoint (docs, runbook, handoff)

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 8 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: [audit-integration-analysis.md](./audit-integration-analysis.md) (1077 lines)
- ✅ Implementation plan (this document v1.0): 6-day timeline, 7 integration points, test examples
- ✅ Risk assessment: 3 critical pitfalls identified with mitigation strategies
- ✅ Existing code review: `server.go`, `deduplication.go`, `storm_aggregator.go` analyzed
- ✅ BR coverage matrix: 7 primary BRs (BR-GATEWAY-017 to BR-GATEWAY-023) mapped to test scenarios
- ✅ ADR-038 review: Fire-and-forget pattern with buffering confirmed as mandatory approach

---

### **Day 1: Foundation + Test Framework (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 8 hours
**TDD Focus**: Write failing tests first, enhance existing code

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `pkg/gateway/server.go` (1449 LOC) - Main signal processing pipeline
- ✅ `pkg/gateway/processing/deduplication.go` (553 LOC) - Deduplication logic
- ✅ `pkg/gateway/processing/storm_aggregator.go` (854 LOC) - Storm aggregation logic
- ✅ `pkg/gateway/config/config.go` - Configuration management

**Morning (4 hours): Audit Store Setup + First 2 Audit Points**

1. **Initialize audit store** in `pkg/gateway/server.go` (1 hour)
   - Add `auditStore audit.BufferedStore` field to `Server` struct
   - Initialize in `NewServer()` with ADR-038 configuration:
     - BufferSize: 10,000 events
     - BatchSize: 1,000 events
     - FlushInterval: 1 second
     - MaxRetries: 3 attempts
   - Add graceful shutdown logic in `Shutdown()` method

2. **Create test file** `test/unit/gateway/audit_integration_test.go` (1 hour)
   - Set up Ginkgo/Gomega test suite
   - Create mock audit store for testing
   - Define test fixtures for audit events

3. **Implement AUDIT POINT 1: Signal Received** (1 hour)
   - Location: `pkg/gateway/server.go:ProcessSignal()` (line ~790)
   - Add audit call after signal ingestion
   - Write failing test for signal received audit
   - Event type: `gateway.signal.received`
   - Outcome: `success`

4. **Implement AUDIT POINT 2: Signal Deduplicated** (1 hour)
   - Location: `pkg/gateway/server.go:processDuplicateSignal()` (line ~1000)
   - Add audit call when duplicate detected
   - Write failing test for duplicate audit
   - Event type: `gateway.signal.deduplicated`
   - Outcome: `success`

**Afternoon (4 hours): Configuration + Integration Test Setup**

5. **Update configuration** `pkg/gateway/config/config.go` (1 hour)
   ```go
   type Config struct {
       // ... existing fields ...

       // Data Storage Service URL for audit writes (ADR-038)
       DataStorageServiceURL string `yaml:"data_storage_service_url" env:"DATA_STORAGE_SERVICE_URL"`
   }
   ```

6. **Update config file** `config/gateway.yaml` (30 min)
   ```yaml
   # Data Storage Service integration (ADR-038)
   data_storage_service_url: http://data-storage.kubernaut-system:8080
   ```

7. **Create integration test** `test/integration/gateway/audit_integration_test.go` (2 hours)
   - Set up test infrastructure (Data Storage client, Redis, K8s API)
   - Define integration test helpers for audit validation
   - Create helper to verify audit events in Data Storage

8. **Run tests** → Verify they FAIL (RED phase) (30 min)
   ```bash
   go test ./test/unit/gateway/audit_integration_test.go -v 2>&1 | grep "FAIL"
   ```

**EOD Deliverables**:
- ✅ Audit store initialized in `Server` struct
- ✅ Configuration updated (code + YAML)
- ✅ 2 audit points implemented (RED phase - tests failing)
- ✅ Test framework complete (unit + integration)
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify tests fail (RED phase)
go test ./test/unit/gateway/audit_integration_test.go -v 2>&1 | grep "FAIL"

# Expected: Tests should FAIL with "audit event not found" or similar
```

---

### **Day 2: Core Audit Calls (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Minimal implementation to pass tests

**Morning (4 hours): Audit Points 3-5**

1. **Implement AUDIT POINT 3: State-Based Dedup Check** (1.5 hours)
   - Location: `pkg/gateway/processing/deduplication.go:Check()` (line ~200)
   - Add audit call after K8s API query
   - Event type: `gateway.deduplication.state_checked`
   - Outcome: `success` (if K8s API succeeds), `failure` (if K8s API fails)
   - Labels: `query_result`, `fallback_to_redis`, `existing_crd_count`

2. **Implement AUDIT POINT 4: Storm Detected** (1.5 hours)
   - Location: `pkg/gateway/server.go:processStormAggregation()` (line ~1085)
   - Add audit call after storm detection
   - Event type: `gateway.storm.detected`
   - Outcome: `success`
   - Labels: `storm_type`, `alert_count`

3. **Implement AUDIT POINT 5: Storm Buffered** (1 hour)
   - Location: `pkg/gateway/server.go:processStormAggregation()` (line ~1137)
   - Add audit call when alert buffered (threshold not reached)
   - Event type: `gateway.storm.buffered`
   - Outcome: `success`
   - Labels: `buffer_utilization`, `buffer_threshold`

**Afternoon (4 hours): Audit Points 6-7**

4. **Implement AUDIT POINT 6: Storm Window Extended** (1.5 hours)
   - Location: `pkg/gateway/server.go:processStormAggregation()` (line ~1102)
   - Add audit call when alert added to existing window
   - Event type: `gateway.storm.window_extended`
   - Outcome: `success`
   - Labels: `window_id`, `resource_count`, `inactivity_timeout`

5. **Implement AUDIT POINT 7: CRD Created** (2 hours)
   - Location 7A: `pkg/gateway/server.go:createRemediationRequestCRD()` (line ~1190)
   - Location 7B: `pkg/gateway/server.go:createAggregatedCRDAfterWindow()` (line ~1304)
   - Add audit call after CRD creation (individual and aggregated)
   - Event type: `gateway.crd.created`
   - Outcome: `success` (if CRD created), `failure` (if K8s API error)
   - Labels: `crd_name`, `crd_type` (individual/aggregated), `environment`, `priority`, `remediation_path`

6. **Run tests** → Verify they PASS (GREEN phase) (30 min)
   ```bash
   go test ./test/unit/gateway/audit_integration_test.go -v
   ```

**EOD Deliverables**:
- ✅ All 7 audit points implemented (GREEN phase)
- ✅ All unit tests passing
- ✅ Basic functionality working
- ✅ Day 2 EOD report

**Validation Commands**:
```bash
# Verify tests pass (GREEN phase)
go test ./test/unit/gateway/audit_integration_test.go -v

# Expected: All tests should PASS
```

---

### **Day 3: Integration + Refactor (DO-REFACTOR Phase)**

**Phase**: DO-REFACTOR
**Duration**: 8 hours
**TDD Focus**: Complete integration + enhancement

**Morning (4 hours): Graceful Shutdown + Error Handling**

1. **Enhance graceful shutdown** in `pkg/gateway/server.go:Shutdown()` (1 hour)
   ```go
   func (s *Server) Shutdown(ctx context.Context) error {
       // ... existing shutdown logic ...

       // Flush remaining audit events (ADR-038)
       if s.auditStore != nil {
           if err := s.auditStore.Close(); err != nil {
               s.logger.Warn("Failed to flush audit events during shutdown",
                   zap.Error(err))
           }
       }

       return nil
   }
   ```

2. **Add error handling for audit failures** (1 hour)
   - Ensure audit failures don't block signal processing
   - Add logging for audit errors (WARN level)
   - Verify fire-and-forget pattern (no error propagation)

3. **Refactor audit event builders** (1 hour)
   - Extract common audit event building logic
   - Create helper functions for repeated patterns
   - Improve code readability

4. **Code review and cleanup** (1 hour)
   - Review all 7 audit integration points
   - Ensure consistent error handling
   - Add GoDoc comments

**Afternoon (4 hours): Metrics + Deployment Manifests**

5. **Add audit metrics** to `pkg/gateway/metrics/metrics.go` (1 hour)
   ```go
   // Audit metrics (ADR-038)
   AuditEventsBuffered  prometheus.Counter
   AuditEventsDropped   prometheus.Counter
   AuditEventsWritten   prometheus.Counter
   AuditBatchesFailed   prometheus.Counter
   AuditBufferSize      prometheus.Gauge
   AuditWriteDuration   prometheus.Histogram
   ```

6. **Update deployment manifests** `deploy/gateway/base/02-configmap.yaml` (1 hour)
   - Add `data_storage_service_url` configuration
   - Document audit configuration fields
   - Add inline comments for ADR-038 reference

7. **Update deployment README** `deploy/gateway/README.md` (1 hour)
   - Add audit integration section
   - Document new configuration fields
   - Add troubleshooting guide for audit issues

8. **Run all tests** → Verify integration complete (1 hour)
   ```bash
   go test ./pkg/gateway/... -v
   go test ./test/unit/gateway/... -v
   ```

**EOD Deliverables**:
- ✅ Graceful shutdown implemented
- ✅ Error handling complete
- ✅ Audit metrics added
- ✅ Deployment manifests updated
- ✅ All tests passing
- ✅ Day 3 EOD report

**Validation Commands**:
```bash
# Verify all tests pass
go test ./pkg/gateway/... -v
go test ./test/unit/gateway/... -v

# Verify no lint errors
golangci-lint run ./pkg/gateway/...
```

---

### **Day 4: Unit + Integration Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Comprehensive unit and integration test coverage

**Morning (4 hours): Unit Tests**

1. **Expand unit tests** to 70%+ coverage (3 hours)
   - Test each audit point with edge cases
   - Test error conditions (Data Storage unavailable)
   - Test boundary values (buffer full, rate limits)
   - Test concurrent audit writes

2. **Behavior & Correctness Validation** (1 hour)
   - Tests validate WHAT the system does (not HOW)
   - Clear business scenarios in test names
   - Specific assertions (not `ToNot(BeNil())`)
   - BR references in test comments

**Unit Test Examples**:

```go
// test/unit/gateway/audit_integration_test.go
var _ = Describe("Gateway Audit Integration", func() {
    var (
        ctx        context.Context
        server     *gateway.Server
        mockAudit  *mocks.MockAuditStore
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockAudit = mocks.NewMockAuditStore()
        server = gateway.NewServerWithAuditStore(cfg, mockAudit)
    })

    Context("when signal is received (BR-GATEWAY-017)", func() {
        It("should emit gateway.signal.received audit event", func() {
            // BUSINESS SCENARIO: Prometheus alert received
            signal := &types.NormalizedSignal{
                SourceType:   "prometheus",
                AlertName:    "HighMemoryUsage",
                Fingerprint:  "sha256:abc123",
                Namespace:    "production",
                ResourceType: "pod",
                ResourceName: "api-server-xyz-123",
                Severity:     "critical",
            }

            // BEHAVIOR: Signal processing emits audit event
            _, err := server.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred())

            // CORRECTNESS: Audit event created with correct fields
            events := mockAudit.GetStoredEvents()
            Expect(events).To(HaveLen(1))
            Expect(events[0].EventType).To(Equal("gateway.signal.received"))
            Expect(events[0].CorrelationID).To(Equal("sha256:abc123"))
            Expect(events[0].Outcome).To(Equal("success"))

            // BUSINESS OUTCOME: BR-GATEWAY-017 validated
        })
    })

    Context("when signal is deduplicated (BR-GATEWAY-018)", func() {
        It("should emit gateway.signal.deduplicated audit event", func() {
            // BUSINESS SCENARIO: Duplicate signal detected
            signal := &types.NormalizedSignal{
                Fingerprint: "sha256:duplicate",
            }

            // Pre-create CRD to trigger deduplication
            existingCRD := createTestCRD(ctx, signal.Fingerprint)

            // BEHAVIOR: Deduplication emits audit event
            _, err := server.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred())

            // CORRECTNESS: Audit event shows duplicate status
            events := mockAudit.GetStoredEvents()
            auditEvent := findEventByType(events, "gateway.signal.deduplicated")
            Expect(auditEvent).ToNot(BeNil())
            Expect(auditEvent.CorrelationID).To(Equal(existingCRD.Name))
            Expect(auditEvent.Outcome).To(Equal("success"))

            // BUSINESS OUTCOME: BR-GATEWAY-018 validated
        })
    })
})
```

**Afternoon (4 hours): Integration Tests**

3. **Create integration tests** (>50% coverage of integration points) (3 hours)
   - Test with real Data Storage Service
   - Test with real Redis
   - Test with real K8s API
   - Test failure scenarios (Data Storage down, Redis unavailable)

4. **Test realistic scenarios** (1 hour)
   - Multi-step workflows (signal → dedup → storm → CRD → audit)
   - Concurrent signal processing with audit writes
   - Graceful shutdown with pending audit events

**Integration Test Example**:

```go
// test/integration/gateway/audit_integration_test.go
var _ = Describe("Gateway Audit Integration Tests", func() {
    var (
        ctx             context.Context
        gatewayServer   *gateway.Server
        datastorageURL  string
        redisClient     *redis.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        datastorageURL = os.Getenv("DATASTORAGE_URL")
        redisClient = setupTestRedis()
        gatewayServer = setupTestGateway(datastorageURL, redisClient)
    })

    AfterEach(func() {
        cleanupTestData(ctx, redisClient)
    })

    Context("when signal processing completes (BR-GATEWAY-017 to BR-GATEWAY-023)", func() {
        It("should create audit trail in Data Storage", func() {
            // BUSINESS SCENARIO: Complete signal processing workflow
            signal := &types.NormalizedSignal{
                SourceType:   "prometheus",
                AlertName:    "PodOOMKilled",
                Fingerprint:  "sha256:integration-test",
                Namespace:    "production",
                ResourceType: "pod",
                ResourceName: "api-server-123",
                Severity:     "critical",
            }

            // BEHAVIOR: Process signal through Gateway
            response, err := gatewayServer.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred())
            Expect(response.Status).To(Equal("created"))

            // Wait for async audit write to complete (ADR-038 eventual consistency)
            time.Sleep(2 * time.Second)

            // CORRECTNESS: Verify audit event in Data Storage
            auditEvents := queryDataStorageAuditEvents(ctx, datastorageURL, signal.Fingerprint)
            Expect(auditEvents).To(HaveLen(1))
            Expect(auditEvents[0].EventType).To(Equal("gateway.signal.received"))
            Expect(auditEvents[0].Service).To(Equal("gateway"))
            Expect(auditEvents[0].Outcome).To(Equal("success"))

            // BUSINESS OUTCOME: Complete audit trail created
        })
    })
})
```

**EOD Deliverables**:
- ✅ 70%+ unit test coverage
- ✅ Integration tests passing (>50% coverage)
- ✅ Tests follow behavior/correctness protocol
- ✅ Day 4 EOD report

**Validation Commands**:
```bash
# Run unit tests with coverage
go test ./test/unit/gateway/audit_integration_test.go -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: total coverage ≥70%

# Run integration tests
make test-integration-gateway

# Expected: All integration tests pass
```

---

### **Day 5: E2E Tests + BR Validation (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: End-to-end feature validation

**Morning (4 hours): E2E Test Implementation**

1. **Create E2E tests** (<10% coverage, critical paths only) (3 hours)
   - Test complete signal processing with audit trail
   - Test storm aggregation with audit events
   - Test deduplication with audit correlation
   - Test graceful shutdown with pending audits

2. **Business requirement validation** (1 hour)
   - Map each E2E test to BR-GATEWAY-017 to BR-GATEWAY-023
   - Validate success criteria
   - Document business outcomes

**E2E Test Example**:

```go
// test/e2e/gateway/06_audit_integration_test.go
var _ = Describe("Gateway Audit Integration E2E", func() {
    var (
        ctx            context.Context
        gatewayURL     string
        datastorageURL string
    )

    BeforeEach(func() {
        ctx = context.Background()
        gatewayURL = os.Getenv("GATEWAY_URL")
        datastorageURL = os.Getenv("DATASTORAGE_URL")
    })

    Context("when complete signal workflow executes (BR-GATEWAY-017, BR-GATEWAY-023)", func() {
        It("should create complete audit trail from ingestion to CRD creation", func() {
            // BUSINESS SCENARIO: Prometheus alert → Gateway → CRD → Audit
            alertPayload := createPrometheusAlert("PodOOMKilled", "production", "api-server-123")

            // BEHAVIOR: Send alert to Gateway
            resp, err := http.Post(
                gatewayURL+"/api/v1/signals/prometheus",
                "application/json",
                bytes.NewBuffer(alertPayload),
            )
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode).To(Equal(http.StatusCreated))

            // Parse response for correlation ID
            var response map[string]interface{}
            json.NewDecoder(resp.Body).Decode(&response)
            crdName := response["crd_name"].(string)

            // Wait for async audit writes (ADR-038 eventual consistency)
            time.Sleep(3 * time.Second)

            // CORRECTNESS: Verify complete audit trail
            auditEvents := queryDataStorageAuditEvents(ctx, datastorageURL, crdName)
            Expect(auditEvents).To(HaveLen(2), "Should have signal.received + crd.created")

            // Verify signal received audit
            signalEvent := findEventByType(auditEvents, "gateway.signal.received")
            Expect(signalEvent).ToNot(BeNil())
            Expect(signalEvent.Outcome).To(Equal("success"))

            // Verify CRD created audit
            crdEvent := findEventByType(auditEvents, "gateway.crd.created")
            Expect(crdEvent).ToNot(BeNil())
            Expect(crdEvent.CorrelationID).To(Equal(crdName))
            Expect(crdEvent.Outcome).To(Equal("success"))

            // BUSINESS OUTCOME: BR-GATEWAY-017 + BR-GATEWAY-023 validated
        })
    })

    Context("when storm aggregation occurs (BR-GATEWAY-020, BR-GATEWAY-021, BR-GATEWAY-022)", func() {
        It("should create audit trail for storm detection and buffering", func() {
            // BUSINESS SCENARIO: Multiple alerts trigger storm aggregation
            alertName := "HighMemoryUsage"
            namespace := "production"

            // Send 10 alerts to trigger storm
            for i := 0; i < 10; i++ {
                alertPayload := createPrometheusAlert(alertName, namespace, fmt.Sprintf("pod-%d", i))
                resp, err := http.Post(
                    gatewayURL+"/api/v1/signals/prometheus",
                    "application/json",
                    bytes.NewBuffer(alertPayload),
                )
                Expect(err).ToNot(HaveOccurred())
                Expect(resp.StatusCode).To(BeOneOf(http.StatusCreated, http.StatusAccepted))
            }

            // Wait for async audit writes
            time.Sleep(3 * time.Second)

            // CORRECTNESS: Verify storm audit events
            auditEvents := queryDataStorageAuditEventsByType(ctx, datastorageURL, "gateway.storm.detected")
            Expect(auditEvents).To(HaveLen(1), "Should have one storm.detected event")

            stormEvent := auditEvents[0]
            Expect(stormEvent.Outcome).To(Equal("success"))
            Expect(stormEvent.EventData).To(HaveKeyWithValue("storm_type", "rate"))

            // BUSINESS OUTCOME: BR-GATEWAY-020, BR-GATEWAY-021, BR-GATEWAY-022 validated
        })
    })
})
```

**Afternoon (4 hours): E2E Edge Cases + BR Validation**

3. **Test edge cases** (2 hours)
   - Data Storage Service unavailable (graceful degradation)
   - High-volume signal processing (buffer overflow)
   - Graceful shutdown with pending audit events

4. **Complete BR validation matrix** (2 hours)
   - Verify all 7 BRs (BR-GATEWAY-017 to BR-GATEWAY-023) validated
   - Document test coverage per BR
   - Create BR validation report

**EOD Deliverables**:
- ✅ E2E tests passing (<10% coverage)
- ✅ All 7 BRs validated
- ✅ BR validation report complete
- ✅ Day 5 EOD report

**Validation Commands**:
```bash
# Run E2E tests
make test-e2e-gateway

# Expected: All E2E tests pass, BRs validated
```

---

### **Day 6: Documentation + Production Readiness (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 8 hours
**Focus**: Finalize documentation and knowledge transfer

**Morning (4 hours): Service Documentation**

1. **Update `overview.md`** (1 hour)
   - Add audit integration feature to "Features" section
   - Update architecture diagram with audit flow
   - Add feature to Table of Contents
   - Update version and changelog

2. **Update `BUSINESS_REQUIREMENTS.md`** (1 hour)
   - Add BR-GATEWAY-017 to BR-GATEWAY-023
   - Mark BRs as implemented
   - Link to implementation files
   - Link to test files

3. **Update `testing-strategy.md`** (1 hour)
   - Add test examples for audit integration
   - Document test coverage for feature
   - Add ADR-038 testing patterns

4. **Update `metrics-slos.md`** (1 hour)
   - Document 6 new audit metrics
   - Add Grafana dashboard panels
   - Update SLI/SLO targets

**Afternoon (4 hours): Operational Documentation**

5. **Create audit runbook** (2 hours)
   - **File**: `docs/services/stateless/gateway-service/operations/audit-runbook.md`
   - **Content**:
     - Audit integration overview
     - Configuration guide
     - Monitoring and alerting
     - Troubleshooting guide (buffer full, Data Storage down)
     - Common issues and solutions

6. **Update deployment README** (1 hour)
   - Document audit configuration fields
   - Add troubleshooting section
   - Add monitoring section

7. **Create handoff summary** (1 hour)
   - Executive summary
   - Key decisions (ADR-038 fire-and-forget)
   - Lessons learned
   - Known limitations (eventual consistency, 1% data loss)
   - Future work (buffer size tuning, metrics dashboard)

**EOD Deliverables**:
- ✅ Service documentation updated
- ✅ Audit runbook created
- ✅ Handoff summary complete
- ✅ Day 6 EOD report
- ✅ Production readiness report

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```go
   // ✅ CORRECT: TDD Cycle 1
   It("should emit audit event when signal received", func() {
       // Test for AUDIT POINT 1
   })
   // Run test → FAIL (RED)
   // Implement audit call → PASS (GREEN)
   // Refactor if needed

   // ✅ CORRECT: TDD Cycle 2 (after Cycle 1 complete)
   It("should emit audit event when signal deduplicated", func() {
       // Test for AUDIT POINT 2
   })
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```go
   // ✅ CORRECT: Behavior-focused
   It("should create audit trail for signal ingestion (BR-GATEWAY-017)", func() {
       _, err := server.ProcessSignal(ctx, signal)
       Expect(err).ToNot(HaveOccurred())

       events := mockAudit.GetStoredEvents()
       Expect(events).To(HaveLen(1))
       Expect(events[0].EventType).To(Equal("gateway.signal.received"))
   })
   ```

3. **Use specific assertions** (not weak checks)
   ```go
   // ✅ CORRECT: Specific business assertions
   Expect(auditEvent.EventType).To(Equal("gateway.signal.received"))
   Expect(auditEvent.CorrelationID).To(Equal("sha256:abc123"))
   Expect(auditEvent.Outcome).To(Equal("success"))
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```go
   // ❌ WRONG: Writing 7 tests before any implementation
   It("test audit point 1", func() { ... })
   It("test audit point 2", func() { ... })
   // ... 5 more tests
   // Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```go
   // ❌ WRONG: Testing internal audit store state
   Expect(server.auditStore.buffer).To(HaveLen(1))
   Expect(server.auditStore.batchSize).To(Equal(1000))
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```go
   // ❌ WRONG: Weak assertions
   Expect(auditEvent).ToNot(BeNil())
   Expect(events).ToNot(BeEmpty())
   Expect(len(events)).To(BeNumerically(">", 0))
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **Unit Test Example - AUDIT POINT 1**

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/gateway"
    "github.com/jordigilh/kubernaut/pkg/gateway/types"
    "github.com/jordigilh/kubernaut/pkg/audit"
)

var _ = Describe("Gateway Audit Integration - Signal Received", func() {
    var (
        ctx        context.Context
        server     *gateway.Server
        mockAudit  *MockAuditStore
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockAudit = NewMockAuditStore()
        server = gateway.NewServerWithAuditStore(cfg, mockAudit)
    })

    Context("when Prometheus alert is received (BR-GATEWAY-017)", func() {
        It("should emit gateway.signal.received audit event with correct fields", func() {
            // BUSINESS SCENARIO: Prometheus AlertManager sends high memory alert
            signal := &types.NormalizedSignal{
                SourceType:   "prometheus",
                AlertName:    "HighMemoryUsage",
                Fingerprint:  "sha256:abc123",
                Namespace:    "production",
                ResourceType: "pod",
                ResourceName: "api-server-xyz-123",
                Severity:     "critical",
            }

            // BEHAVIOR: Gateway processes signal and emits audit event
            response, err := server.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred(), "Signal processing should succeed")
            Expect(response.Status).To(Equal("created"), "CRD should be created")

            // CORRECTNESS: Audit event created with correct fields
            events := mockAudit.GetStoredEvents()
            Expect(events).To(HaveLen(1), "Should emit exactly one audit event")

            auditEvent := events[0]
            Expect(auditEvent.EventType).To(Equal("gateway.signal.received"))
            Expect(auditEvent.CorrelationID).To(Equal("sha256:abc123"))
            Expect(auditEvent.ResourceType).To(Equal("pod"))
            Expect(auditEvent.ResourceID).To(Equal("api-server-xyz-123"))
            Expect(auditEvent.ResourceNS).To(Equal("production"))
            Expect(auditEvent.Outcome).To(Equal("success"))
            Expect(auditEvent.Operation).To(Equal("signal_received"))
            Expect(auditEvent.Severity).To(Equal("critical"))

            // Verify event data structure
            Expect(auditEvent.EventData).To(HaveKey("gateway"))
            gatewayData := auditEvent.EventData["gateway"].(map[string]interface{})
            Expect(gatewayData["signal_type"]).To(Equal("prometheus"))
            Expect(gatewayData["alert_name"]).To(Equal("HighMemoryUsage"))

            // BUSINESS OUTCOME: BR-GATEWAY-017 validated
            // Audit trail created for signal ingestion
        })
    })

    Context("when Kubernetes event is received (BR-GATEWAY-017)", func() {
        It("should emit gateway.signal.received audit event with event_reason", func() {
            // BUSINESS SCENARIO: Kubernetes Event API reports OOMKilled
            signal := &types.NormalizedSignal{
                SourceType:   "kubernetes",
                EventReason:  "OOMKilled",
                Fingerprint:  "sha256:k8s-event",
                Namespace:    "production",
                ResourceType: "pod",
                ResourceName: "memory-hog-pod",
                Severity:     "critical",
            }

            // BEHAVIOR: Gateway processes K8s event and emits audit
            response, err := server.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred())

            // CORRECTNESS: Audit event includes event_reason
            events := mockAudit.GetStoredEvents()
            Expect(events).To(HaveLen(1))

            auditEvent := events[0]
            Expect(auditEvent.EventType).To(Equal("gateway.signal.received"))
            gatewayData := auditEvent.EventData["gateway"].(map[string]interface{})
            Expect(gatewayData["signal_type"]).To(Equal("kubernetes"))
            Expect(gatewayData["event_reason"]).To(Equal("OOMKilled"))

            // BUSINESS OUTCOME: BR-GATEWAY-017 validated for K8s events
        })
    })
})
```

### **Integration Test Example - Complete Audit Trail**

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Gateway Audit Integration Tests", func() {
    var (
        ctx             context.Context
        gatewayServer   *gateway.Server
        datastorageURL  string
        redisClient     *redis.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        datastorageURL = os.Getenv("DATASTORAGE_URL")
        redisClient = infrastructure.SetupTestRedis()
        gatewayServer = infrastructure.SetupTestGateway(datastorageURL, redisClient)
    })

    AfterEach(func() {
        infrastructure.CleanupTestData(ctx, redisClient)
    })

    Context("when signal processing completes (BR-GATEWAY-017, BR-GATEWAY-023)", func() {
        It("should create complete audit trail from ingestion to CRD creation", func() {
            // BUSINESS SCENARIO: Alert ingestion → CRD creation → Audit trail
            signal := &types.NormalizedSignal{
                SourceType:   "prometheus",
                AlertName:    "PodOOMKilled",
                Fingerprint:  "sha256:integration-test",
                Namespace:    "production",
                ResourceType: "pod",
                ResourceName: "api-server-123",
                Severity:     "critical",
            }

            // BEHAVIOR: Process signal through Gateway
            response, err := gatewayServer.ProcessSignal(ctx, signal)
            Expect(err).ToNot(HaveOccurred())
            Expect(response.Status).To(Equal("created"))
            crdName := response.CRDName

            // Wait for async audit write (ADR-038 eventual consistency)
            time.Sleep(2 * time.Second)

            // CORRECTNESS: Verify audit events in Data Storage
            auditEvents := infrastructure.QueryDataStorageAuditEvents(
                ctx,
                datastorageURL,
                crdName,
            )
            Expect(auditEvents).To(HaveLen(2), "Should have signal.received + crd.created")

            // Verify signal received audit
            signalEvent := infrastructure.FindEventByType(auditEvents, "gateway.signal.received")
            Expect(signalEvent).ToNot(BeNil())
            Expect(signalEvent.Service).To(Equal("gateway"))
            Expect(signalEvent.Outcome).To(Equal("success"))
            Expect(signalEvent.CorrelationID).To(Equal("sha256:integration-test"))

            // Verify CRD created audit
            crdEvent := infrastructure.FindEventByType(auditEvents, "gateway.crd.created")
            Expect(crdEvent).ToNot(BeNil())
            Expect(crdEvent.CorrelationID).To(Equal(crdName))
            Expect(crdEvent.Outcome).To(Equal("success"))
            Expect(crdEvent.EventData).To(HaveKeyWithValue("crd_type", "individual"))

            // BUSINESS OUTCOME: Complete audit trail created
            // BR-GATEWAY-017 + BR-GATEWAY-023 validated
        })
    })

    Context("when Data Storage Service is unavailable (ADR-038 graceful degradation)", func() {
        It("should continue processing signals without blocking", func() {
            // BUSINESS SCENARIO: Data Storage down, Gateway must remain operational

            // Stop Data Storage Service
            infrastructure.StopDataStorageService()

            // BEHAVIOR: Gateway continues processing (fire-and-forget)
            signal := &types.NormalizedSignal{
                SourceType:  "prometheus",
                AlertName:   "TestAlert",
                Fingerprint: "sha256:degradation-test",
                Namespace:   "production",
            }

            start := time.Now()
            response, err := gatewayServer.ProcessSignal(ctx, signal)
            duration := time.Since(start)

            // CORRECTNESS: Signal processing succeeds despite audit failure
            Expect(err).ToNot(HaveOccurred(), "Signal processing should not fail")
            Expect(response.Status).To(Equal("created"))
            Expect(duration).To(BeNumerically("<", 100*time.Millisecond),
                "Latency should remain <100ms (no blocking)")

            // BUSINESS OUTCOME: ADR-038 graceful degradation validated
            // Business operations never fail due to audit issues
        })
    })
})
```

### **E2E Test Example - Storm Aggregation with Audit**

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/test/e2e/infrastructure"
)

var _ = Describe("Gateway Audit E2E - Storm Aggregation", func() {
    var (
        ctx            context.Context
        gatewayURL     string
        datastorageURL string
    )

    BeforeEach(func() {
        ctx = context.Background()
        gatewayURL = os.Getenv("GATEWAY_URL")
        datastorageURL = os.Getenv("DATASTORAGE_URL")
    })

    Context("when storm aggregation occurs (BR-GATEWAY-020, BR-GATEWAY-021, BR-GATEWAY-022)", func() {
        It("should create audit trail for storm detection, buffering, and window extension", func() {
            // BUSINESS SCENARIO: Multiple alerts trigger storm aggregation
            alertName := "HighMemoryUsage"
            namespace := "production"
            testID := fmt.Sprintf("storm-test-%d", time.Now().UnixNano())

            // Send 10 alerts to trigger storm
            for i := 0; i < 10; i++ {
                alertPayload := infrastructure.CreatePrometheusAlert(
                    alertName,
                    namespace,
                    fmt.Sprintf("pod-%s-%d", testID, i),
                )

                resp, err := http.Post(
                    gatewayURL+"/api/v1/signals/prometheus",
                    "application/json",
                    bytes.NewBuffer(alertPayload),
                )
                Expect(err).ToNot(HaveOccurred())
                Expect(resp.StatusCode).To(BeOneOf(http.StatusCreated, http.StatusAccepted))
            }

            // Wait for async audit writes (ADR-038 eventual consistency)
            time.Sleep(3 * time.Second)

            // CORRECTNESS: Verify storm detection audit
            stormEvents := infrastructure.QueryDataStorageAuditEventsByType(
                ctx,
                datastorageURL,
                "gateway.storm.detected",
            )
            Expect(stormEvents).To(HaveLen(1), "Should detect one storm")

            stormEvent := stormEvents[0]
            Expect(stormEvent.Outcome).To(Equal("success"))
            Expect(stormEvent.EventData).To(HaveKeyWithValue("storm_type", "rate"))
            Expect(stormEvent.EventData).To(HaveKey("alert_count"))

            // Verify storm buffering audits (first 5 alerts)
            bufferEvents := infrastructure.QueryDataStorageAuditEventsByType(
                ctx,
                datastorageURL,
                "gateway.storm.buffered",
            )
            Expect(bufferEvents).To(HaveLen(5), "Should buffer first 5 alerts")

            // Verify storm window extension audits (remaining 5 alerts)
            windowEvents := infrastructure.QueryDataStorageAuditEventsByType(
                ctx,
                datastorageURL,
                "gateway.storm.window_extended",
            )
            Expect(windowEvents).To(HaveLen(5), "Should extend window for remaining alerts")

            // BUSINESS OUTCOME: Complete storm aggregation audit trail
            // BR-GATEWAY-020, BR-GATEWAY-021, BR-GATEWAY-022 validated
        })
    })
})
```

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-GATEWAY-017** | Audit trace for signal ingestion | `audit_integration_test.go` (signal received) | `audit_integration_test.go` (complete workflow) | `06_audit_integration_test.go` (E2E workflow) | ✅ |
| **BR-GATEWAY-018** | Audit trace for deduplication | `audit_integration_test.go` (duplicate detected) | `audit_integration_test.go` (dedup workflow) | `06_audit_integration_test.go` (dedup E2E) | ✅ |
| **BR-GATEWAY-019** | Audit trace for state-based dedup | `audit_integration_test.go` (K8s API query) | `audit_integration_test.go` (K8s integration) | `06_audit_integration_test.go` (state-based E2E) | ✅ |
| **BR-GATEWAY-020** | Audit trace for storm detection | `audit_integration_test.go` (storm detected) | `audit_integration_test.go` (storm workflow) | `06_audit_integration_test.go` (storm E2E) | ✅ |
| **BR-GATEWAY-021** | Audit trace for storm buffering | `audit_integration_test.go` (buffering) | `audit_integration_test.go` (buffer workflow) | `06_audit_integration_test.go` (buffer E2E) | ✅ |
| **BR-GATEWAY-022** | Audit trace for window extension | `audit_integration_test.go` (window extended) | `audit_integration_test.go` (window workflow) | `06_audit_integration_test.go` (window E2E) | ✅ |
| **BR-GATEWAY-023** | Audit trace for CRD creation | `audit_integration_test.go` (CRD created) | `audit_integration_test.go` (CRD workflow) | `06_audit_integration_test.go` (CRD E2E) | ✅ |

**Coverage Calculation**:
- **Unit**: 7/7 BRs covered (100%)
- **Integration**: 7/7 BRs covered (100%)
- **E2E**: 7/7 BRs covered (100%)
- **Total**: 7/7 BRs covered (100%)

---

## 🚨 **Critical Pitfalls to Avoid**

### **1. Blocking Signal Processing with Synchronous Audit Writes**
- ❌ **Problem**: Using synchronous audit writes would add 10-50ms latency to every signal, exceeding the 100ms target
- ✅ **Solution**: Use ADR-038 fire-and-forget pattern with `pkg/audit.BufferedStore`
- **Impact**: Business operations remain fast (<100ms), audit writes happen asynchronously

### **2. Not Handling Audit Failures Gracefully**
- ❌ **Problem**: Propagating audit errors to signal processing would cause business operations to fail
- ✅ **Solution**: Fire-and-forget pattern with error logging only (no error propagation)
- **Impact**: Business operations never fail due to audit issues (ADR-038 principle)

### **3. Forgetting Graceful Shutdown**
- ❌ **Problem**: Not flushing pending audit events during shutdown would cause data loss
- ✅ **Solution**: Call `auditStore.Close()` in `Server.Shutdown()` to flush remaining events
- **Impact**: Pending audit events are written before Gateway terminates

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit 70%+, Integration >50%, E2E <10%)
- ✅ No lint errors
- ✅ All 7 audit integration points implemented
- ✅ Graceful shutdown with audit flush
- ✅ Latency impact <0.1ms (fire-and-forget)
- ✅ Documentation complete

### **Business Success**
- ✅ BR-GATEWAY-017 validated (signal ingestion audit)
- ✅ BR-GATEWAY-018 validated (deduplication audit)
- ✅ BR-GATEWAY-019 validated (state-based dedup audit)
- ✅ BR-GATEWAY-020 validated (storm detection audit)
- ✅ BR-GATEWAY-021 validated (storm buffering audit)
- ✅ BR-GATEWAY-022 validated (window extension audit)
- ✅ BR-GATEWAY-023 validated (CRD creation audit)
- ✅ Audit throughput: 10,000 events/sec
- ✅ Data loss rate: <1%

### **Confidence Assessment**
- **Target**: ≥95% confidence
- **Calculation**: Evidence-based (ADR-038 pattern + existing audit library + comprehensive testing)
- **Justification**:
  - ✅ ADR-038 fire-and-forget pattern is industry-proven (9/10 platforms)
  - ✅ `pkg/audit.BufferedStore` already exists and tested
  - ✅ 7 integration points clearly identified with code locations
  - ✅ Comprehensive test plan (unit + integration + E2E)
  - ✅ Zero latency impact on signal processing

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Critical bug discovered in audit integration
- Performance degradation >5% in signal processing
- Data loss rate >5% (exceeds 1% threshold)
- Business requirement not met

### **Rollback Procedure**
1. **Disable audit integration** (feature flag or config)
   ```yaml
   # config/gateway.yaml
   data_storage_service_url: ""  # Empty string disables audit
   ```
2. **Deploy previous Gateway version** (if needed)
3. **Verify rollback success** (signal processing latency returns to baseline)
4. **Document rollback reason** (create incident report)

### **Rollback Validation**
```bash
# Verify Gateway is processing signals without audit
curl -X POST http://gateway:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d @test-alert.json

# Expected: 201 Created, no audit events in Data Storage
```

---

## 📚 **References**

### **Templates**
- [FEATURE_EXTENSION_PLAN_TEMPLATE.md](../../FEATURE_EXTENSION_PLAN_TEMPLATE.md) - Feature extension template
- [SERVICE_DOCUMENTATION_GUIDE.md](../../SERVICE_DOCUMENTATION_GUIDE.md) - Documentation standards

### **Standards**
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-go-coding-standards.mdc](../../../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns

### **Design Decisions**
- [ADR-038: Asynchronous Buffered Audit Trace Ingestion](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) - **PRIMARY AUTHORITY**
- [ADR-034: Unified Audit Table Design](../../../architecture/decisions/ADR-034-unified-audit-table.md) - Audit table schema
- [ADR-036: Authentication and Authorization Strategy](../../../architecture/decisions/ADR-036-auth-strategy.md) - Network-level security
- [DD-AUDIT-002: Audit Shared Library Design](../../../architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md) - Shared library implementation

### **Analysis**
- [audit-integration-analysis.md](./audit-integration-analysis.md) - Comprehensive audit integration analysis (1077 lines)

### **Examples**
- [DD-GATEWAY-008](./DD_GATEWAY_008_IMPLEMENTATION_PLAN.md) - Storm buffering (12 days)
- [DD-GATEWAY-009](./DD_GATEWAY_009_IMPLEMENTATION_PLAN.md) - State-based deduplication (5 days)

---

**Document Status**: 📋 **DRAFT**
**Last Updated**: 2025-11-19
**Version**: 1.0
**Maintained By**: Gateway Development Team

---

## 📝 **Implementation Checklist**

### **Day 1: Foundation**
- [ ] Initialize `auditStore` in `Server` struct
- [ ] Add configuration (`data_storage_service_url`)
- [ ] Implement AUDIT POINT 1 (signal received) - RED
- [ ] Implement AUDIT POINT 2 (signal deduplicated) - RED
- [ ] Create test framework (unit + integration)
- [ ] Day 1 EOD report

### **Day 2: Core Audit Calls**
- [ ] Implement AUDIT POINT 3 (state-based dedup) - GREEN
- [ ] Implement AUDIT POINT 4 (storm detected) - GREEN
- [ ] Implement AUDIT POINT 5 (storm buffered) - GREEN
- [ ] Implement AUDIT POINT 6 (storm window extended) - GREEN
- [ ] Implement AUDIT POINT 7 (CRD created) - GREEN
- [ ] All unit tests passing
- [ ] Day 2 EOD report

### **Day 3: Integration + Refactor**
- [ ] Graceful shutdown with audit flush
- [ ] Error handling for audit failures
- [ ] Refactor audit event builders
- [ ] Add audit metrics
- [ ] Update deployment manifests
- [ ] Update deployment README
- [ ] Day 3 EOD report

### **Day 4: Unit + Integration Tests**
- [ ] Expand unit tests to 70%+ coverage
- [ ] Behavior & correctness validation
- [ ] Create integration tests (>50% coverage)
- [ ] Test realistic scenarios
- [ ] All tests passing
- [ ] Day 4 EOD report

### **Day 5: E2E Tests + BR Validation**
- [ ] Create E2E tests (<10% coverage)
- [ ] Test complete signal workflow with audit
- [ ] Test storm aggregation with audit
- [ ] Test graceful degradation (Data Storage down)
- [ ] Validate all 7 BRs (BR-GATEWAY-017 to BR-GATEWAY-023)
- [ ] BR validation report
- [ ] Day 5 EOD report

### **Day 6: Documentation + Production Readiness**
- [ ] Update `overview.md`
- [ ] Update `BUSINESS_REQUIREMENTS.md`
- [ ] Update `testing-strategy.md`
- [ ] Update `metrics-slos.md`
- [ ] Create audit runbook
- [ ] Update deployment README
- [ ] Create handoff summary
- [ ] Production readiness report
- [ ] Day 6 EOD report

---

**Ready to implement?** Start with Day 1 (Foundation + Test Framework) and follow the APDC-TDD methodology!

