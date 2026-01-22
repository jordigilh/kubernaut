# Webhook Integration Test Architecture - Real Services

**Date**: 2026-01-05  
**Critical Correction**: Integration tests MUST use real services, not mocks  
**Pattern**: DD-TEST-002 Sequential Startup (Programmatic Podman)

---

## 🚨 **CRITICAL: Mock Anti-Pattern Identified**

### **Current Implementation (WRONG)**
```go
// test/integration/authwebhook/suite_test.go
mockAuditMgr = &mockAuditManager{events: []audit.AuditEvent{}}  // ❌ MOCK
nrHandler := webhooks.NewNotificationRequestDeleteHandler(mockAuditMgr)
```

**Problem**: Integration tests using mocks violate project testing guidelines.

---

## ✅ **Correct Pattern: Real Data Storage Service**

###  **Example: AI Analysis Integration Tests**

```go
// test/integration/aianalysis/suite_test.go (CORRECT PATTERN)

// Start REAL Data Storage service
err = infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)
Expect(err).ToNot(HaveOccurred())

// Use REAL OpenAPI audit client
dsClient, err := audit.NewOpenAPIClientAdapter(
    "http://localhost:18095",  // Real Data Storage service
    5*time.Second,
)
Expect(err).ToNot(HaveOccurred())

// Use REAL audit store
auditStore, err := audit.NewBufferedStore(dsClient, config, "aianalysis", logger)
```

---

## 📋 **Webhook Integration Test Requirements**

### **Infrastructure Needed**

| Service | Port | Purpose |
|---------|------|---------|
| **PostgreSQL** | TBD | Data Storage persistence |
| **Redis** | TBD | Data Storage caching |
| **Data Storage API** | TBD | Audit event storage (OpenAPI) |

### **NOT Needed**
- ❌ HolmesGPT API (webhooks don't use AI)
- ❌ Mock audit manager (use real Data Storage)

---

## 🏗️ **Implementation Plan**

### **Phase 1: Create Infrastructure Startup Function** (2 hours)

**File**: `test/infrastructure/authwebhook.go` (NEW)

**Pattern**: Follow AI Analysis integration infrastructure pattern

```go
package infrastructure

// StartAuthWebhookIntegrationInfrastructure starts Data Storage for webhook tests
// Per DD-TEST-002: Programmatic podman run commands
//
// Infrastructure:
// - PostgreSQL (port: TBD per DD-TEST-001)
// - Redis (port: TBD per DD-TEST-001)
// - Data Storage API (port: TBD per DD-TEST-001)
//
// Returns:
// - error: Any errors during infrastructure startup
func StartAuthWebhookIntegrationInfrastructure(writer io.Writer) error {
    // 1. Cleanup existing containers
    // 2. Start PostgreSQL
    // 3. Start Redis
    // 4. Run migrations
    // 5. Start Data Storage API
    // 6. Wait for health checks
}

// StopAuthWebhookIntegrationInfrastructure stops all containers
func StopAuthWebhookIntegrationInfrastructure(writer io.Writer) error {
    // Cleanup containers
}
```

**Reference Implementation**: `test/infrastructure/aianalysis.go:1924-2172`

---

### **Phase 2: Update suite_test.go** (1 hour)

**File**: `test/integration/authwebhook/suite_test.go`

**Changes**:

```go
// REMOVE mock audit manager
// DELETE:
// type mockAuditManager struct { ... }
// mockAuditMgr = &mockAuditManager{events: []audit.AuditEvent{}}

// ADD real infrastructure
var _ = SynchronizedBeforeSuite(NodeTimeout(5*time.Minute), func(specCtx SpecContext) []byte {
    By("Starting Auth Webhook integration infrastructure")
    err := infrastructure.StartAuthWebhookIntegrationInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())
    
    By("Creating REAL OpenAPI audit client")
    dsClient, err := audit.NewOpenAPIClientAdapter(
        "http://localhost:[PORT]",  // Per DD-TEST-001 port allocation
        5*time.Second,
    )
    Expect(err).ToNot(HaveOccurred())
    
    By("Creating REAL audit store")
    auditConfig := audit.RecommendedConfig("authwebhook-test")
    auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "authwebhook", logger)
    Expect(err).ToNot(HaveOccurred())
    
    // ... envtest setup continues ...
    
    By("Registering webhook handlers with REAL audit store")
    wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(auditStore)
    rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler(auditStore)
    nrHandler := webhooks.NewNotificationRequestDeleteHandler(auditStore)
    
    // ... rest of setup ...
}, func(data []byte) {
    // Parallel process initialization
})

var _ = SynchronizedAfterSuite(func() {
    // Per-process cleanup
}, func() {
    By("Stopping Auth Webhook integration infrastructure")
    infrastructure.StopAuthWebhookIntegrationInfrastructure(GinkgoWriter)
})
```

---

### **Phase 3: Update Integration Tests** (1 hour)

**Files**: 
- `test/integration/authwebhook/workflowexecution_test.go`
- `test/integration/authwebhook/remediationapprovalrequest_test.go`
- `test/integration/authwebhook/notificationrequest_test.go`

**Changes**:

```go
// REMOVE mock audit manager assertions
// DELETE:
// Eventually(func() int {
//     return len(mockAuditMgr.events)
// }).Should(BeNumerically(">=", 1))

// ADD real Data Storage API queries
By("Verifying webhook wrote audit event to Data Storage")
// Query real Data Storage API via OpenAPI client
ctx := context.Background()
auditClient := dsClient.AuditAPI // OpenAPI generated client

// Wait for async audit write (buffered store)
Eventually(func() int {
    params := &audit.ListAuditEventsParams{
        EventType: ptr.To("workflowexecution.block.cleared"),
        ActorID:   ptr.To("admin"),
        Limit:     ptr.To(int32(10)),
    }
    resp, err := auditClient.ListAuditEvents(ctx, params)
    if err != nil {
        return 0
    }
    return len(resp.Events)
}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Data Storage should have audit event from webhook")

// Verify event content
resp, err := auditClient.ListAuditEvents(ctx, &audit.ListAuditEventsParams{
    EventType: ptr.To("workflowexecution.block.cleared"),
    Limit:     ptr.To(int32(1)),
})
Expect(err).ToNot(HaveOccurred())
Expect(resp.Events).ToNot(BeEmpty())

event := resp.Events[0]
Expect(event.ActorID).To(Equal("admin"))
Expect(event.EventCategory).To(Equal("workflow"))
Expect(event.EventOutcome).To(Equal("success"))
```

---

### **Phase 4: Port Allocation** (30 min)

**Update**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`

**Add new section**:

```markdown
### AuthWebhook Integration Tests

| Service | Port | Container Name |
|---------|------|----------------|
| PostgreSQL | [TBD] | authwebhook_postgres_1 |
| Redis | [TBD] | authwebhook_redis_1 |
| Data Storage API | [TBD] | authwebhook_datastorage_1 |

**Dependencies**:
- AuthWebhook → Data Storage (audit events)
- Data Storage → PostgreSQL (persistence)
- Data Storage → Redis (caching/DLQ)
```

**Recommendation**: Use shared Data Storage ports from another service if tests don't run concurrently.

---

### **Phase 5: Update Webhook Handlers** (30 min)

**Files**:
- `pkg/authwebhook/workflowexecution_handler.go`
- `pkg/authwebhook/remediationapprovalrequest_handler.go`
- `pkg/authwebhook/notificationrequest_handler.go`

**Changes**:

```go
// Change from audit.Manager interface to audit.AuditStore (concrete type)
type WorkflowExecutionAuthHandler struct {
    authenticator *authwebhook.Authenticator
    decoder       admission.Decoder
    auditStore    audit.AuditStore  // CHANGED: was audit.Manager
}

func NewWorkflowExecutionAuthHandler(auditStore audit.AuditStore) *WorkflowExecutionAuthHandler {
    return &WorkflowExecutionAuthHandler{
        authenticator: authwebhook.NewAuthenticator(),
        auditStore:    auditStore,  // CHANGED
    }
}

func (h *WorkflowExecutionAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    // ... authentication logic ...
    
    // Use RecordEvent (audit.AuditStore method)
    err := h.auditStore.RecordEvent(ctx, audit.Event{
        // ... event details ...
    })
    if err != nil {
        // Per ADR-032: Webhooks are P0 - MUST succeed
        return admission.Denied(fmt.Sprintf("audit write failed: %v", err))
    }
    
    // ... rest of logic ...
}
```

---

## 📊 **Testing Strategy**

### **Unit Tests** (`test/unit/authwebhook/`)
- ✅ Mock audit.AuditStore interface
- ✅ Test authentication extraction logic
- ✅ Test validation rules
- ✅ Fast (<100ms per test)

### **Integration Tests** (`test/integration/authwebhook/`)
- ✅ Real Data Storage service (programmatic podman)
- ✅ Real OpenAPI audit client
- ✅ Real envtest (K8s API server)
- ✅ Verify audit events in real database
- ⚠️ Slower (~10-30s per test due to infrastructure)

### **E2E Tests** (if needed)
- ✅ Real Kind cluster
- ✅ Real webhook service deployment
- ✅ Real Data Storage service
- ✅ Complete end-to-end flow

---

## ⏱️ **Implementation Timeline**

| Phase | Task | Duration | Priority |
|-------|------|----------|----------|
| **Phase 1** | Create infrastructure startup function | 2 hours | HIGH |
| **Phase 2** | Update suite_test.go (remove mock) | 1 hour | HIGH |
| **Phase 3** | Update integration tests (real API) | 1 hour | HIGH |
| **Phase 4** | Port allocation (DD-TEST-001) | 30 min | HIGH |
| **Phase 5** | Update webhook handlers (AuditStore) | 30 min | HIGH |
| **Total** | End-to-end real service integration | **5 hours** | - |

---

## ✅ **Success Criteria**

1. ✅ No mock audit manager in integration tests
2. ✅ Real Data Storage service started programmatically
3. ✅ Webhook handlers use real OpenAPI audit client
4. ✅ Integration tests query real Data Storage API
5. ✅ Audit events verified in real PostgreSQL database
6. ✅ All tests passing (9/9)
7. ✅ Infrastructure cleanup working (no orphaned containers)

---

## 🚀 **Benefits of Real Services**

| Aspect | Mock (Current) | Real Services (Correct) |
|--------|----------------|-------------------------|
| **Test Validity** | ❌ Tests mock behavior | ✅ Tests actual integration |
| **API Compatibility** | ❌ Can drift from OpenAPI spec | ✅ Validates OpenAPI contract |
| **Database Schema** | ❌ Not validated | ✅ Tests real schema |
| **Network Issues** | ❌ Not detected | ✅ Catches connectivity problems |
| **Async Buffering** | ❌ Not tested | ✅ Tests BufferedStore behavior |
| **Confidence** | ⚠️ Low (70%) | ✅ High (95%) |

---

## 📚 **References**

- **DD-TEST-002**: Integration Test Container Orchestration Pattern
- **DD-TEST-001**: Port Allocation Strategy
- **DD-API-001**: OpenAPI Client Usage Mandate
- **ADR-032**: Audit Requirements for P0 Services
- **AI Analysis Integration Tests**: `test/integration/aianalysis/suite_test.go:113-220`
- **Infrastructure Utilities**: `test/infrastructure/aianalysis.go:1924-2172`

---

**Last Updated**: 2026-01-05  
**Status**: Implementation Plan  
**Priority**: HIGH (Corrects integration test anti-pattern)  

