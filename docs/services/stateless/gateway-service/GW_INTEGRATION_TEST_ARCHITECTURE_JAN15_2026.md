# Gateway Integration Test Architecture
**Date**: January 15, 2026  
**Status**: 🚨 **CRITICAL ARCHITECTURAL UPDATE**  
**Impact**: ALL 77 integration tests must use this pattern

---

## 🚨 Critical Constraint: No Mocks in Integration Tests

### **Integration Test Environment**
- ✅ **Real DataStorage**: Podman container with PostgreSQL backend
- ✅ **Real Kubernetes API**: Kind cluster or envtest
- ✅ **Real Gateway Service**: Business logic with actual dependencies
- ❌ **NO MOCKS**: Mocks only allowed in unit tests

### **Parallel Execution Requirement**
- ✅ Multiple integration tests run concurrently
- ✅ All tests share the SAME DataStorage instance
- ✅ Correlation ID filtering is MANDATORY for test isolation
- ❌ Cannot rely on "empty" DataStorage or sequential execution

---

## 🏗️ Architecture Comparison

### ❌ WRONG: Mock-Based (Unit Test Pattern)

```go
var _ = Describe("Gateway Integration Tests", func() {
    var auditStore *MockAuditStore  // ❌ Mock not allowed in integration tests
    
    BeforeEach(func() {
        auditStore = NewMockAuditStore()  // ❌ Wrong tier
    })
    
    It("should emit audit event", func() {
        // Process signal
        gateway.ProcessSignal(ctx, signal)
        
        // ❌ Query mock - NOT testing real DataStorage integration
        events := auditStore.Events
        Expect(events).To(HaveLen(1))
    })
})
```

**Problems**:
1. ❌ Not testing real DataStorage integration
2. ❌ Not testing PostgreSQL persistence
3. ❌ Not testing concurrent access patterns
4. ❌ Not testing audit event query API

---

### ✅ CORRECT: Real DataStorage with Correlation ID Filtering

```go
var _ = Describe("Gateway Integration Tests", func() {
    var (
        dsClient  *api.Client         // ✅ Real DataStorage client
        gateway   *gateway.Service    // ✅ Real Gateway service
        k8sClient client.Client       // ✅ Real Kubernetes client
        ctx       context.Context
    )
    
    BeforeEach(func() {
        // Connect to real DataStorage in Podman
        dsClient = connectToDataStorage()
        
        // Initialize Gateway with real dependencies
        gateway = gateway.NewService(dsClient, k8sClient, logger)
        
        ctx = context.Background()
    })
    
    // Test ID: GW-INT-AUD-001
    It("[GW-INT-AUD-001] should emit signal.received audit event", func() {
        // Given: Prometheus alert
        signal := createTestPrometheusAlert()
        
        // When: Gateway processes signal
        correlationID, err := gateway.ProcessSignal(ctx, signal)
        Expect(err).ToNot(HaveOccurred())
        
        // Then: Query REAL DataStorage by correlation ID for test isolation
        auditEvent := FindAuditEventByTypeAndCorrelationID(
            ctx,
            dsClient,
            api.GatewayAuditPayloadEventTypeGatewaySignalReceived, // OpenAPI constant
            correlationID, // ← CRITICAL for parallel execution
            30*time.Second,
        )
        
        Expect(auditEvent).ToNot(BeNil())
        Expect(auditEvent.CorrelationID).To(Equal(correlationID))
        
        // Validate audit payload
        gatewayPayload := ParseGatewayPayload(auditEvent)
        Expect(gatewayPayload.SignalType).To(Equal(api.GatewayAuditPayloadSignalTypePrometheusAlert))
    })
})
```

**Benefits**:
1. ✅ Tests real DataStorage integration
2. ✅ Tests real PostgreSQL persistence
3. ✅ Tests concurrent access with correlation ID filtering
4. ✅ Tests actual audit event query API
5. ✅ Parallel execution safe

---

## 🔐 Test Isolation Strategy

### **Problem: Shared DataStorage**
```
Test A (parallel): Signal "cpu-high" → Correlation ID: "rr-abc123-1234567890"
Test B (parallel): Signal "mem-high" → Correlation ID: "rr-def456-1234567891"
Test C (parallel): Signal "disk-full" → Correlation ID: "rr-ghi789-1234567892"

All 3 tests → SAME DataStorage instance → SAME audit_events table
```

### **Solution: Correlation ID Filtering**

| Approach | Isolation | Parallel Safe | Correct |
|----------|-----------|---------------|---------|
| Query all events | ❌ | ❌ | ❌ |
| Query by event type only | ❌ | ❌ | ❌ |
| Query by correlation ID | ✅ | ✅ | ✅ |

**Example: Without Correlation ID** (BROKEN):
```go
// ❌ WRONG: Gets first event of type, could be from ANY test
events := dsClient.ListAuditEvents(ctx, api.ListAuditEventsParams{
    EventType: api.NewOptString("gateway.signal.received"),
})
auditEvent := events[0]  // ← Could be from Test B or Test C!
```

**Example: With Correlation ID** (CORRECT):
```go
// ✅ CORRECT: Gets THIS test's event only
events := dsClient.ListAuditEvents(ctx, api.ListAuditEventsParams{
    EventType:     api.NewOptString("gateway.signal.received"),
    CorrelationID: api.NewOptString(signal.CorrelationID),  // ← Test isolation
})
auditEvent := events[0]  // ← Guaranteed to be from THIS test
```

---

## 📚 Required Helper Functions

### **1. FindAuditEventByTypeAndCorrelationID** (Primary)

```go
func FindAuditEventByTypeAndCorrelationID(
    ctx context.Context,
    dsClient *api.Client,
    eventType api.GatewayAuditPayloadEventType,  // Use OpenAPI constant
    correlationID string,                         // Test isolation
    timeout time.Duration,
) *api.AuditEvent {
    var event *api.AuditEvent
    
    Eventually(func() bool {
        resp, err := dsClient.ListAuditEvents(ctx, api.ListAuditEventsParams{
            EventType:     api.NewOptString(string(eventType)),
            CorrelationID: api.NewOptString(correlationID),  // ← CRITICAL
            Limit:         api.NewOptInt(1),
        })
        
        if err != nil || len(resp.Events) == 0 {
            return false
        }
        
        event = &resp.Events[0]
        return true
    }, timeout, 500*time.Millisecond).Should(BeTrue())
    
    return event
}
```

**Usage**:
```go
// Always pass correlation ID from YOUR signal
auditEvent := FindAuditEventByTypeAndCorrelationID(
    ctx,
    dsClient,
    api.GatewayAuditPayloadEventTypeGatewaySignalReceived,
    signal.CorrelationID,  // ← YOUR test's correlation ID
    30*time.Second,
)
```

### **2. FindAllAuditEventsByCorrelationID** (Full Trail)

```go
func FindAllAuditEventsByCorrelationID(
    ctx context.Context,
    dsClient *api.Client,
    correlationID string,
    timeout time.Duration,
) []api.AuditEvent {
    var events []api.AuditEvent
    
    Eventually(func() bool {
        resp, err := dsClient.ListAuditEvents(ctx, api.ListAuditEventsParams{
            CorrelationID: api.NewOptString(correlationID),  // ← Test isolation
            Limit:         api.NewOptInt(100),
        })
        
        if err != nil || len(resp.Events) == 0 {
            return false
        }
        
        events = resp.Events
        return true
    }, timeout, 500*time.Millisecond).Should(BeTrue())
    
    return events
}
```

**Usage**:
```go
// Get all audit events for YOUR signal (received + crd.created + deduplicated)
allEvents := FindAllAuditEventsByCorrelationID(
    ctx,
    dsClient,
    signal.CorrelationID,
    30*time.Second,
)

Expect(allEvents).To(HaveLen(2), "Should have signal.received + crd.created")
```

---

## 🎯 OpenAPI Constants Usage

### **Event Type Constants**

```go
import api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

// ✅ Use these constants (type-safe, generated from OpenAPI spec)
api.GatewayAuditPayloadEventTypeGatewaySignalReceived     // "gateway.signal.received"
api.GatewayAuditPayloadEventTypeGatewaySignalDeduplicated // "gateway.signal.deduplicated"
api.GatewayAuditPayloadEventTypeGatewayCrdCreated         // "gateway.crd.created"
api.GatewayAuditPayloadEventTypeGatewayCrdFailed          // "gateway.crd.failed"

// ❌ Don't use magic strings
"gateway.signal.received"  // Hard to refactor, typo-prone
```

### **Deduplication Status Constants**

```go
// ✅ Use OpenAPI constants
api.GatewayAuditPayloadDeduplicationStatusNew        // "new"
api.GatewayAuditPayloadDeduplicationStatusDuplicate  // "duplicate"

// ❌ Don't use magic strings
"duplicate"  // Hard to refactor
```

### **Signal Type Constants**

```go
// ✅ Use OpenAPI constants
api.GatewayAuditPayloadSignalTypePrometheusAlert  // "prometheus-alert"
api.GatewayAuditPayloadSignalTypeKubernetesEvent  // "kubernetes-event"
```

---

## 📋 Checklist for ALL Integration Tests

### **Before Writing Any Test**
- [ ] Import `api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"`
- [ ] Initialize real `dsClient` in `BeforeSuite` or `BeforeEach`
- [ ] Initialize real `k8sClient` (Kind or envtest)
- [ ] NO `MockAuditStore` or any mocks

### **In Every Test**
- [ ] Capture `correlationID` from Gateway response
- [ ] Use `FindAuditEventByTypeAndCorrelationID()` with OpenAPI constants
- [ ] Pass `signal.CorrelationID` for test isolation
- [ ] Verify `auditEvent.CorrelationID == signal.CorrelationID`
- [ ] Use `ParseGatewayPayload()` to extract typed payload
- [ ] Use OpenAPI constants for all enums

### **Anti-Patterns to AVOID**
- ❌ `auditStore.Events` (mock)
- ❌ `findEventByType(events, "gateway.signal.received")` (magic string)
- ❌ Querying DataStorage without correlation ID filter
- ❌ Assuming DataStorage is empty
- ❌ Using first event in list without correlation ID check

---

## 🎓 Complete Example

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    var (
        dsClient  *api.Client
        gateway   *gateway.Service
        k8sClient client.Client
        ctx       context.Context
    )
    
    BeforeEach(func() {
        ctx = context.Background()
        
        // Real DataStorage client (Podman container)
        dsClient = testutil.NewDataStorageClient()
        
        // Real Kubernetes client
        k8sClient = testutil.NewK8sClient()
        
        // Real Gateway service
        gateway = gateway.NewService(dsClient, k8sClient, logger)
    })
    
    // Test ID: GW-INT-AUD-001
    It("[GW-INT-AUD-001] should emit gateway.signal.received audit event for Prometheus signal", func() {
        // Given: Prometheus alert with unique fingerprint
        alert := createTestPrometheusAlert()
        
        // When: Gateway processes signal
        correlationID, err := gateway.ProcessSignal(ctx, alert)
        Expect(err).ToNot(HaveOccurred())
        Expect(correlationID).ToNot(BeEmpty())
        
        // Then: Query DataStorage by correlation ID (parallel-safe)
        auditEvent := FindAuditEventByTypeAndCorrelationID(
            ctx,
            dsClient,
            api.GatewayAuditPayloadEventTypeGatewaySignalReceived,  // OpenAPI constant
            correlationID,                                           // Test isolation
            30*time.Second,
        )
        
        // Verify audit event
        Expect(auditEvent).ToNot(BeNil())
        Expect(auditEvent.EventType).To(Equal(string(api.GatewayAuditPayloadEventTypeGatewaySignalReceived)))
        Expect(auditEvent.CorrelationID).To(Equal(correlationID), "Correlation ID must match")
        
        // Parse typed payload
        gatewayPayload := ParseGatewayPayload(auditEvent)
        Expect(gatewayPayload.SignalType).To(Equal(api.GatewayAuditPayloadSignalTypePrometheusAlert))
        
        // Validate RR reconstruction fields
        signalLabels, ok := gatewayPayload.SignalLabels.Get()
        Expect(ok).To(BeTrue())
        Expect(signalLabels).To(HaveKey("severity"))
    })
})
```

---

## 🚀 Migration Path

### **Updating Existing Tests**

1. **Remove Mocks**:
   ```go
   // ❌ Delete
   auditStore := NewMockAuditStore()
   
   // ✅ Add
   dsClient := testutil.NewDataStorageClient()
   ```

2. **Capture Correlation ID**:
   ```go
   // ✅ Get correlation ID from Gateway
   correlationID, err := gateway.ProcessSignal(ctx, signal)
   ```

3. **Update Event Queries**:
   ```go
   // ❌ Old
   events := auditStore.Events
   auditEvent := findEventByType(events, "gateway.signal.received")
   
   // ✅ New
   auditEvent := FindAuditEventByTypeAndCorrelationID(
       ctx,
       dsClient,
       api.GatewayAuditPayloadEventTypeGatewaySignalReceived,
       correlationID,
       30*time.Second,
   )
   ```

4. **Add Correlation ID Verification**:
   ```go
   // ✅ Always verify
   Expect(auditEvent.CorrelationID).To(Equal(correlationID))
   ```

---

## ✅ Success Criteria

### **Integration Test Must**:
1. ✅ Use real DataStorage client (no mocks)
2. ✅ Query by correlation ID for test isolation
3. ✅ Use OpenAPI constants for all enums
4. ✅ Verify correlation ID matches
5. ✅ Work in parallel execution
6. ✅ Test actual PostgreSQL persistence
7. ✅ Test actual audit event query API

### **Integration Test Must NOT**:
1. ❌ Use MockAuditStore or any mocks
2. ❌ Use magic strings for event types
3. ❌ Query DataStorage without correlation ID
4. ❌ Assume DataStorage is empty
5. ❌ Rely on event order without correlation ID
6. ❌ Share correlation IDs between tests

---

**Status**: 🚨 **MANDATORY for all 77 Gateway integration tests**  
**Priority**: P0 - Blocking implementation  
**Authority**: INTEGRATION_E2E_NO_MOCKS_POLICY.md, 03-testing-strategy.mdc
