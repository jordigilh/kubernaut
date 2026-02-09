# Audit Gap Remediation - Complete Implementation Plan V2

**Document Type**: Implementation Plan
**Version**: 2.0
**Date**: December 8, 2025
**Authority**: DD-AUDIT-002, DD-AUDIT-003, DD-009, DD-005, ADR-032, ADR-034, ADR-038
**Methodology**: APDC-Enhanced TDD (Analysis → Plan → Do → Check)
**Status**: 📋 APPROVED FOR IMPLEMENTATION

---

## 📋 **Gap Summary**

| Gap | Severity | Component | Issue |
|-----|----------|-----------|-------|
| **GAP-1** | 🔴 P0 | Server | Batch endpoint expects single object, not array |
| **GAP-3** | 🔴 P0 | Tests | No unit tests for HTTPDataStorageClient |
| **GAP-4** | 🔴 P0 | Tests | No integration tests for batch path |
| **GAP-9** | 🔴 P0 | BufferedStore | Silent event drops when buffer full |
| **GAP-10** | 🔴 P0 | BufferedStore | No DLQ fallback on write failures |
| **GAP-11** | 🟡 P1 | BufferedStore | No error type differentiation |
| **GAP-12** | 🟡 P1 | BufferedStore | Close() doesn't report lost events |
| **GAP-8** | 🟡 P1 | DLQ | Consumer methods not implemented |
| **GAP-2** | 🟡 P1 | Worker | Async retry worker missing |
| **GAP-7** | 🟡 P1 | Tests | Missing service audit integration tests |
| **GAP-5** | 🟢 P2 | Observability | Log sanitization missing |
| **GAP-6** | 🟢 P2 | Observability | Path normalization missing |

---

## 🎯 **Implementation Schedule**

| Day | Phase | Gaps | Hours | Deliverables |
|-----|-------|------|-------|--------------|
| **Day 1** | Critical API Fix | GAP-1 | 4h | Batch endpoint handler |
| **Day 1** | BufferedStore Fix | GAP-9, GAP-12 | 2h | Error returns |
| **Day 2** | Defense-in-Depth | GAP-3, GAP-4 | 6h | Unit + integration tests |
| **Day 2** | DLQ Integration | GAP-10, GAP-11 | 3h | DLQ fallback in store |
| **Day 3** | DLQ Consumer | GAP-8 | 4h | ReadMessages, AckMessage |
| **Day 3** | Retry Worker | GAP-2 | 4h | cmd/audit-retry-worker |
| **Day 4** | Retry Worker | GAP-2 | 4h | Complete retry worker |
| **Day 4** | Service Tests | GAP-7 | 6h | Gateway, WE, EM tests |
| **Day 5** | Observability | GAP-5, GAP-6 | 4h | Log sanitization, path normalization |
| **Day 5** | CHECK Phase | All | 3h | Validation, documentation |

**Total**: 40 hours / 5 days

---

## 📅 **Day 1: Critical API Fix + BufferedStore Error Returns**

### Phase 1A: GAP-1 - Batch Endpoint (4 hours)

#### TDD Cycle 1.1: Batch Handler (RED → GREEN → REFACTOR)

**Business Requirement**: BR-AUDIT-001 (Complete audit trail with no data loss)
**Design Decision**: DD-AUDIT-002 (StoreBatch interface must accept arrays)

##### RED Phase (30 min)

**Create**: `test/integration/datastorage/audit_events_batch_write_api_test.go`

```go
var _ = Describe("Audit Events Batch Write API", func() {
    // BR-AUDIT-001: Complete audit trail with no data loss
    // DD-AUDIT-002: StoreBatch interface must accept arrays

    Context("POST /api/v1/audit/events/batch", func() {
        // BEHAVIOR: Handler accepts array of audit events
        // CORRECTNESS: All events in batch are persisted atomically
        It("should accept batch of audit events and return 201 with all event_ids", func() {
            events := []map[string]interface{}{
                createBatchTestEvent("batch-test-1"),
                createBatchTestEvent("batch-test-2"),
                createBatchTestEvent("batch-test-3"),
            }

            body, _ := json.Marshal(events)
            req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events/batch", bytes.NewBuffer(body))
            req.Header.Set("Content-Type", "application/json")

            resp, err := httpClient.Do(req)
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()

            // CORRECTNESS: 201 Created
            Expect(resp.StatusCode).To(Equal(http.StatusCreated))

            // CORRECTNESS: Response contains event_ids array
            var response struct {
                EventIDs []string `json:"event_ids"`
            }
            json.NewDecoder(resp.Body).Decode(&response)
            Expect(response.EventIDs).To(HaveLen(3))

            // CORRECTNESS: All events persisted in DB
            for _, eventID := range response.EventIDs {
                var count int
                db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE event_id = $1`, eventID).Scan(&count)
                Expect(count).To(Equal(1))
            }
        })

        // BEHAVIOR: Atomic batch - all succeed or all fail
        It("should reject entire batch if any event is invalid", func() {
            events := []map[string]interface{}{
                createBatchTestEvent("atomic-test-1"),
                {"invalid": "missing required fields"},  // Invalid
            }

            body, _ := json.Marshal(events)
            req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events/batch", bytes.NewBuffer(body))
            req.Header.Set("Content-Type", "application/json")

            resp, _ := httpClient.Do(req)
            defer resp.Body.Close()

            Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

            // No events persisted
            var count int
            db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE correlation_id LIKE 'atomic-test%'`).Scan(&count)
            Expect(count).To(Equal(0))
        })
    })
})
```

**Run**: `go test ./test/integration/datastorage/... -v -run "Batch"`
**Expected**: ❌ FAIL (endpoint doesn't exist)

##### GREEN Phase (1.5 hours)

**Create**: `pkg/datastorage/server/audit_events_batch_handler.go`

```go
// ========================================
// AUDIT EVENTS BATCH HANDLER
// 📋 Design Decision: DD-AUDIT-002 | BR-AUDIT-001
// Authority: DD-AUDIT-002 "DataStorageClient.StoreBatch"
// ========================================

package server

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/google/uuid"
)

// handleCreateAuditEventsBatch handles POST /api/v1/audit/events/batch
// DD-AUDIT-002: StoreBatch interface accepts arrays
func (s *Server) handleCreateAuditEventsBatch(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    // 1. Parse JSON array
    var payloads []map[string]interface{}
    if err := json.NewDecoder(r.Body).Decode(&payloads); err != nil {
        s.logger.Info("Invalid JSON array", "error", err)
        writeRFC7807Error(w, &RFC7807Problem{
            Type:   "https://kubernaut.io/problems/invalid-request",
            Title:  "Invalid Request",
            Status: http.StatusBadRequest,
            Detail: "Request body must be a JSON array: " + err.Error(),
        })
        return
    }

    if len(payloads) == 0 {
        writeRFC7807Error(w, &RFC7807Problem{
            Type:   "https://kubernaut.io/problems/invalid-request",
            Title:  "Invalid Request",
            Status: http.StatusBadRequest,
            Detail: "Batch cannot be empty",
        })
        return
    }

    // 2. Validate ALL events BEFORE persisting (atomic)
    events := make([]*repository.AuditEvent, 0, len(payloads))
    for i, payload := range payloads {
        event, err := s.parseAuditEventPayload(payload)
        if err != nil {
            writeRFC7807Error(w, &RFC7807Problem{
                Type:   "https://kubernaut.io/problems/validation-error",
                Title:  "Validation Error",
                Status: http.StatusBadRequest,
                Detail: fmt.Sprintf("Event at index %d: %s", i, err.Error()),
            })
            return
        }
        events = append(events, event)
    }

    // 3. Write batch atomically (transaction)
    createdEvents, err := s.auditEventsRepo.CreateBatch(ctx, events)
    if err != nil {
        s.logger.Error(err, "Batch write failed", "count", len(events))

        // DLQ fallback
        s.enqueueBatchToDLQ(ctx, events, err)

        writeRFC7807Error(w, &RFC7807Problem{
            Type:   "https://kubernaut.io/problems/database-error",
            Title:  "Database Error",
            Status: http.StatusInternalServerError,
            Detail: "Failed to write audit events batch",
        })
        return
    }

    // 4. Build response
    eventIDs := make([]string, len(createdEvents))
    for i, e := range createdEvents {
        eventIDs[i] = e.EventID.String()
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "event_ids": eventIDs,
        "message":   fmt.Sprintf("%d audit events created", len(eventIDs)),
    })
}
```

**Update**: `pkg/datastorage/server/server.go` (register route)

```go
// In Handler() method:
r.Post("/audit/events/batch", s.handleCreateAuditEventsBatch)
```

**Add Repository Method**: `pkg/datastorage/repository/audit_events_repository.go`

```go
// CreateBatch creates multiple audit events atomically
// DD-AUDIT-002: Batch write for efficiency
func (r *AuditEventsRepository) CreateBatch(ctx context.Context, events []*AuditEvent) ([]*AuditEvent, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    created := make([]*AuditEvent, 0, len(events))
    for _, event := range events {
        event.EventID = uuid.New()
        event.EventDate = event.EventTimestamp.Truncate(24 * time.Hour)

        eventDataJSON, _ := json.Marshal(event.EventData)

        _, err := tx.ExecContext(ctx, `
            INSERT INTO audit_events (
                event_id, event_version, event_timestamp, event_date,
                event_type, event_category, event_action, event_outcome,
                actor_type, actor_id, resource_type, resource_id,
                correlation_id, namespace, cluster_name, event_data,
                severity, retention_days, is_sensitive
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
        `,
            event.EventID, "1.0", event.EventTimestamp, event.EventDate,
            event.EventType, event.EventCategory, event.EventAction, event.EventOutcome,
            event.ActorType, event.ActorID, event.ResourceType, event.ResourceID,
            event.CorrelationID, event.ResourceNamespace, event.ClusterID, eventDataJSON,
            event.Severity, 2555, false,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to insert event: %w", err)
        }
        created = append(created, event)
    }

    if err := tx.Commit(); err != nil {
        return nil, fmt.Errorf("failed to commit: %w", err)
    }

    return created, nil
}
```

**Run**: `go test ./test/integration/datastorage/... -v -run "Batch"`
**Expected**: ✅ PASS

##### REFACTOR Phase (30 min)

1. Extract `parseAuditEventPayload()` to shared function
2. Add comprehensive logging
3. Run tests → Verify still PASS

---

### Phase 1B: GAP-9 + GAP-12 - BufferedStore Error Returns (2 hours)

#### TDD Cycle 1.2: Buffer Full Returns Error

**Business Requirement**: BR-AUDIT-001
**Design Decision**: ADR-032 (No Audit Loss)

##### RED Phase (20 min)

**Update**: `test/unit/audit/store_test.go`

```go
// EXISTING TEST - MUST BE CHANGED
It("should return error when buffer is full", func() {
    // Fill buffer
    for i := 0; i < 100; i++ {
        event := createTestEvent()
        _ = store.StoreAudit(ctx, event)
    }

    // Next event should return error (not nil)
    event := createTestEvent()
    err := store.StoreAudit(ctx, event)

    // ADR-032: Caller must know event was dropped
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("buffer full"))
})
```

**Run**: `go test ./test/unit/audit/... -v -run "buffer is full"`
**Expected**: ❌ FAIL (currently returns nil)

##### GREEN Phase (30 min)

**Update**: `pkg/audit/store.go` (Lines 160-174)

```go
default:
    // ⚠️ Buffer full - MUST return error per ADR-032
    atomic.AddInt64(&s.droppedCount, 1)
    s.metrics.RecordDropped()

    s.logger.Info("Audit buffer full, event dropped",
        "event_type", event.EventType,
        "correlation_id", event.CorrelationID,
    )

    // ADR-032: Caller must know event was dropped to implement fallback
    return fmt.Errorf("audit buffer full: event dropped (correlation_id=%s)", event.CorrelationID)
```

**Run**: Test should pass

#### TDD Cycle 1.3: Close() Reports Lost Events

##### RED Phase (15 min)

**Add Test**: `test/unit/audit/store_test.go`

```go
It("should return error on close if events were dropped", func() {
    // Configure store to fail all writes
    mockClient.SetShouldFail(true)

    // Store events (will fail after retries)
    for i := 0; i < 10; i++ {
        event := createTestEvent()
        store.StoreAudit(ctx, event)
    }

    // Wait for retries to complete
    time.Sleep(15 * time.Second)

    // Close should report failure
    err := store.Close()

    // ADR-032: Caller must know events were lost
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("failed batches"))
})
```

##### GREEN Phase (30 min)

**Update**: `pkg/audit/store.go` (Close method)

```go
func (s *BufferedAuditStore) Close() error {
    // ... existing close logic ...

    // ADR-032: Report if events were lost
    dropped := atomic.LoadInt64(&s.droppedCount)
    failed := atomic.LoadInt64(&s.failedBatchCount)

    if dropped > 0 || failed > 0 {
        return fmt.Errorf("audit store closed with data loss: %d events dropped, %d batches failed", dropped, failed)
    }

    return nil
}
```

---

## 📅 **Day 2: Defense-in-Depth Tests + DLQ Integration**

### Phase 2A: GAP-3 - HTTPDataStorageClient Unit Tests (2 hours)

#### TDD Cycle 2.1: Unit Tests for http_client.go

**Create**: `test/unit/audit/http_client_test.go`

```go
package audit_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/audit"
)

var _ = Describe("HTTPDataStorageClient", func() {
    // DD-AUDIT-002: HTTPDataStorageClient must implement DataStorageClient interface

    Describe("StoreBatch", func() {
        var (
            client     audit.DataStorageClient
            mockServer *httptest.Server
        )

        AfterEach(func() {
            if mockServer != nil {
                mockServer.Close()
            }
        })

        // BEHAVIOR: StoreBatch sends events as JSON array to /batch endpoint
        // CORRECTNESS: Server receives array at correct endpoint
        It("should send events as JSON array to /api/v1/audit/events/batch", func() {
            var receivedPath string
            var receivedPayload interface{}

            mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                receivedPath = r.URL.Path
                json.NewDecoder(r.Body).Decode(&receivedPayload)
                w.WriteHeader(http.StatusCreated)
                json.NewEncoder(w).Encode(map[string]interface{}{"event_ids": []string{"id-1"}})
            }))

            client = audit.NewHTTPDataStorageClient(mockServer.URL, &http.Client{Timeout: 5 * time.Second})
            events := []*audit.AuditEvent{createTestAuditEvent("test-1")}

            err := client.StoreBatch(context.Background(), events)

            Expect(err).ToNot(HaveOccurred())
            Expect(receivedPath).To(Equal("/api/v1/audit/events/batch"))

            // Verify array was sent
            _, isArray := receivedPayload.([]interface{})
            Expect(isArray).To(BeTrue(), "Payload must be JSON array")
        })

        // BEHAVIOR: StoreBatch returns nil for empty batch
        It("should return nil for empty batch without HTTP call", func() {
            httpCalled := false
            mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                httpCalled = true
            }))

            client = audit.NewHTTPDataStorageClient(mockServer.URL, &http.Client{})

            err := client.StoreBatch(context.Background(), []*audit.AuditEvent{})

            Expect(err).ToNot(HaveOccurred())
            Expect(httpCalled).To(BeFalse())
        })

        // BEHAVIOR: StoreBatch returns error with status code on HTTP failure
        It("should return error with status code on 4xx/5xx", func() {
            mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusBadRequest)
            }))

            client = audit.NewHTTPDataStorageClient(mockServer.URL, &http.Client{})
            events := []*audit.AuditEvent{createTestAuditEvent("test")}

            err := client.StoreBatch(context.Background(), events)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("400"))
        })
    })
})
```

---

### Phase 2B: GAP-4 - Integration Tests (4 hours)

**Create**: `test/integration/datastorage/http_client_integration_test.go`

```go
var _ = Describe("HTTPDataStorageClient Integration", func() {
    // DD-AUDIT-002: Integration test for client → server path

    It("should persist batch via HTTPDataStorageClient.StoreBatch", func() {
        client := audit.NewHTTPDataStorageClient(datastorageURL, &http.Client{Timeout: 10 * time.Second})

        correlationID := "http-client-test-" + time.Now().Format("20060102150405")
        events := []*audit.AuditEvent{
            createIntegrationEvent("event-1", correlationID),
            createIntegrationEvent("event-2", correlationID),
        }

        err := client.StoreBatch(context.Background(), events)
        Expect(err).ToNot(HaveOccurred())

        // Verify in database
        var count int
        db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`, correlationID).Scan(&count)
        Expect(count).To(Equal(2))
    })
})
```

---

### Phase 2C: GAP-10 + GAP-11 - DLQ Fallback in BufferedStore (3 hours)

#### TDD Cycle 2.2: DLQ Fallback on Write Failure

##### RED Phase (30 min)

**Add Test**: `test/unit/audit/store_test.go`

```go
It("should enqueue to DLQ after max retries", func() {
    mockDLQ := NewMockDLQClient()
    mockClient.SetShouldFail(true)

    config := audit.Config{
        BufferSize:    100,
        BatchSize:     10,
        FlushInterval: 100 * time.Millisecond,
        MaxRetries:    3,
    }

    store, _ = audit.NewBufferedStoreWithDLQ(mockClient, mockDLQ, config, "test-service", logger)

    // Store events
    for i := 0; i < 10; i++ {
        store.StoreAudit(ctx, createTestEvent())
    }

    // Wait for retries
    Eventually(func() int {
        return mockDLQ.EnqueueCount()
    }, "20s").Should(Equal(10))

    Expect(mockClient.BatchCount()).To(Equal(0))  // Not written
})
```

##### GREEN Phase (1.5 hours)

**Update**: `pkg/audit/store.go`

```go
// Add DLQ client field
type BufferedAuditStore struct {
    // ... existing fields ...
    dlqClient DLQClient  // Optional DLQ for failed writes
}

// Add constructor
func NewBufferedStoreWithDLQ(client DataStorageClient, dlqClient DLQClient, config Config, serviceName string, logger logr.Logger) (AuditStore, error) {
    store, err := NewBufferedStore(client, config, serviceName, logger)
    if err != nil {
        return nil, err
    }
    store.(*BufferedAuditStore).dlqClient = dlqClient
    return store, nil
}

// Update writeBatchWithRetry
func (s *BufferedAuditStore) writeBatchWithRetry(batch []*AuditEvent) {
    ctx := context.Background()
    var lastErr error

    for attempt := 1; attempt <= s.config.MaxRetries; attempt++ {
        lastErr = s.client.StoreBatch(ctx, batch)
        if lastErr == nil {
            // Success
            atomic.AddInt64(&s.writtenCount, int64(len(batch)))
            s.metrics.RecordWritten(len(batch))
            return
        }

        // GAP-11: Check if error is non-retryable
        if isNonRetryableError(lastErr) {
            s.logger.Info("Non-retryable error, skipping retries",
                "error", lastErr,
                "batch_size", len(batch))
            break  // Go directly to DLQ
        }

        if attempt < s.config.MaxRetries {
            backoff := time.Duration(attempt*attempt) * time.Second
            time.Sleep(backoff)
        }
    }

    // Max retries exceeded or non-retryable error
    atomic.AddInt64(&s.failedBatchCount, 1)
    s.metrics.RecordBatchFailed()

    // GAP-10: DLQ fallback
    if s.dlqClient != nil {
        for _, event := range batch {
            if err := s.dlqClient.EnqueueAuditEvent(ctx, event, lastErr); err != nil {
                s.logger.Error(err, "DLQ fallback failed", "event_type", event.EventType)
            }
        }
        s.logger.Info("Batch enqueued to DLQ", "batch_size", len(batch))
    } else {
        s.logger.Error(nil, "AUDIT DATA LOSS: Batch dropped, no DLQ configured",
            "batch_size", len(batch))
    }
}

// isNonRetryableError checks if error is a client error (4xx)
func isNonRetryableError(err error) bool {
    if err == nil {
        return false
    }
    errStr := err.Error()
    // HTTP 4xx errors are non-retryable
    return strings.Contains(errStr, "status 400") ||
           strings.Contains(errStr, "status 401") ||
           strings.Contains(errStr, "status 403") ||
           strings.Contains(errStr, "status 404")
}
```

---

## 📅 **Day 3: DLQ Consumer + Retry Worker**

### Phase 3A: GAP-8 - DLQ Consumer Methods (4 hours)

**Create**: `test/unit/datastorage/dlq/consumer_test.go`

See detailed implementation in original plan.

**Implement in** `pkg/datastorage/dlq/client.go`:
- `ReadMessages(ctx, consumerGroup, consumerName, timeout) ([]*DLQMessage, error)`
- `AckMessage(ctx, consumerGroup, messageID) error`
- `MoveToDeadLetter(ctx, msg) error`

---

### Phase 3B: GAP-2 - Async Retry Worker (4 hours today, 4 hours Day 4)

**Create**: `cmd/audit-retry-worker/main.go`

See detailed implementation in original plan.

Key components:
- Exponential backoff: 1m, 5m, 15m, 1h, 4h, 24h
- Consumer group: `audit-retry-workers`
- Dead letter after 6 retries
- Prometheus metrics for monitoring

---

## 📅 **Day 4: Complete Retry Worker + Service Tests**

### Phase 4A: GAP-2 - Complete Retry Worker (4 hours)

- Add health endpoints (`/health/live`, `/health/ready`)
- Add Prometheus metrics endpoint
- Create Kubernetes deployment manifest
- Create unit tests for retry logic

### Phase 4B: GAP-7 - Service Audit Integration Tests (6 hours)

**Create**:
- `test/integration/gateway/audit_integration_test.go`
- `test/integration/workflowexecution/audit_integration_test.go`
- `test/integration/effectivenessmonitor/audit_integration_test.go`

---

## 📅 **Day 5: Observability + CHECK Phase**

### Phase 5A: GAP-5 - Log Sanitization (2 hours)

**Create**: `pkg/datastorage/middleware/log_sanitization.go`

```go
var sensitivePatterns = []struct {
    pattern     *regexp.Regexp
    replacement string
}{
    {regexp.MustCompile(`"password"\s*:\s*"[^"]*"`), `"password":"[REDACTED]"`},
    {regexp.MustCompile(`"token"\s*:\s*"[^"]*"`), `"token":"[REDACTED]"`},
    {regexp.MustCompile(`"api_key"\s*:\s*"[^"]*"`), `"api_key":"[REDACTED]"`},
    {regexp.MustCompile(`"secret"\s*:\s*"[^"]*"`), `"secret":"[REDACTED]"`},
    {regexp.MustCompile(`"authorization"\s*:\s*"[^"]*"`), `"authorization":"[REDACTED]"`},
}

func SanitizeForLog(data string) string {
    for _, sp := range sensitivePatterns {
        data = sp.pattern.ReplaceAllString(data, sp.replacement)
    }
    return data
}
```

### Phase 5B: GAP-6 - Path Normalization (2 hours)

**Create**: `pkg/datastorage/metrics/path_normalizer.go`

```go
func NormalizePath(path string) string {
    segments := strings.Split(path, "/")
    for i, segment := range segments {
        if isUUID(segment) || isNumericID(segment) {
            segments[i] = ":id"
        }
    }
    return strings.Join(segments, "/")
}
```

### Phase 5C: CHECK Phase (3 hours)

1. Run all tests: `make test-all`
2. Verify coverage: `make coverage`
3. Run linter: `golangci-lint run`
4. Update documentation
5. Create handoff document

---

## ✅ **Acceptance Criteria by Gap**

| Gap | Acceptance Criteria | Test Evidence |
|-----|---------------------|---------------|
| **GAP-1** | Batch endpoint accepts arrays | Integration test passes |
| **GAP-3** | http_client.go has >70% coverage | `go test -cover` |
| **GAP-4** | Client→Server batch path tested | Integration test passes |
| **GAP-9** | Buffer full returns error | Unit test passes |
| **GAP-10** | Failed batches go to DLQ | Unit test with mock DLQ |
| **GAP-11** | 4xx errors skip retry | Unit test verifies no retry |
| **GAP-12** | Close() reports lost events | Unit test passes |
| **GAP-8** | DLQ consumer methods work | Unit test with miniredis |
| **GAP-2** | Retry worker processes DLQ | Integration test |
| **GAP-7** | 3 service audit tests exist | Test files exist |
| **GAP-5** | Sensitive data redacted | Unit test verifies redaction |
| **GAP-6** | Paths normalized in metrics | Unit test verifies normalization |

---

## 🎯 **Success Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| All gaps addressed | 12/12 | Code review |
| Unit test coverage | >70% | `go test -cover` |
| Integration tests passing | 100% | CI pipeline |
| No silent event drops | 0 | ADR-032 audit |
| DLQ fallback working | Yes | Manual test |

---

## 📜 **Document History**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | Dec 8, 2025 | DS Team | Initial plan (GAP-1 to GAP-8) |
| 2.0 | Dec 8, 2025 | DS Team | Added GAP-9 to GAP-12 from BufferedStore audit |






