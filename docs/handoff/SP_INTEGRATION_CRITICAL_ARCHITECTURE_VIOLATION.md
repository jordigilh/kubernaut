# SignalProcessing Integration Tests - CRITICAL ARCHITECTURE VIOLATION
**Date**: January 10, 2026
**Status**: 🚨 BLOCKING - Must revert incorrect changes
**Severity**: CRITICAL - Service boundary violated

---

## 🚨 **CRITICAL ISSUE**

The SignalProcessing integration tests were **incorrectly modified** to query the PostgreSQL database directly. This violates service boundaries.

### **WRONG Approach (Current)**
```go
// ❌ WRONG: SignalProcessing querying DataStorage's database directly
testDB, err = sql.Open("pgx", postgresConnStr)
err := testDB.QueryRow(`
    SELECT COUNT(*)
    FROM audit_events
    WHERE event_type = $1
      AND correlation_id = $2
      AND actor_id = 'signalprocessing-service'
`, eventType, correlationID).Scan(&eventCount)
```

**Why This Is Wrong**:
- ❌ Violates service boundaries (SignalProcessing shouldn't access DataStorage's database)
- ❌ Breaks microservices architecture
- ❌ Not how production code works (production uses HTTP API)
- ❌ Creates tight coupling between services

---

## ✅ **CORRECT Approach**

SignalProcessing should use the **DataStorage HTTP API client** (ogen-generated) to query events:

```go
// ✅ CORRECT: SignalProcessing using DataStorage HTTP API
import ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

// Create ogen client (or reuse from suite)
dsClient, err := ogenclient.NewClient(
    fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort),
    ogenclient.WithClient(http.DefaultClient),
)
Expect(err).ToNot(HaveOccurred())

// Query using HTTP API
params := ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString(eventType),
    CorrelationID: ogenclient.NewOptString(correlationID),
}

resp, err := dsClient.QueryAuditEvents(ctx, params)
Expect(err).ToNot(HaveOccurred())
Expect(resp.Events).To(HaveLen(1))

// Verify event data
event := resp.Events[0]
Expect(event.EventType).To(Equal(eventType))
Expect(event.CorrelationID).To(Equal(correlationID))
```

---

## 🔧 **Required Changes**

### 1. Remove Direct Database Access

**File**: `test/integration/signalprocessing/suite_test.go`

**Remove**:
```go
testDB *sql.DB  // HTTP Anti-Pattern Phase 2: PostgreSQL direct query connection
```

**Remove**:
```go
postgresConnStr := fmt.Sprintf(
    "host=127.0.0.1 port=%d user=slm_user password=test_password dbname=action_history sslmode=disable",
    infrastructure.SignalProcessingIntegrationPostgresPort,
)
testDB, err = sql.Open("pgx", postgresConnStr)
```

---

### 2. Add ogen Client Variable

**File**: `test/integration/signalprocessing/suite_test.go`

**Add**:
```go
var (
    cfg        *rest.Config
    k8sClient  client.Client
    k8sManager ctrl.Manager
    auditStore audit.AuditStore
    dsClient   *ogenclient.Client  // ✅ DataStorage HTTP API client for queries
)
```

**In BeforeSuite (Process 2)**:
```go
// Create DataStorage ogen client for query operations
By("Creating DataStorage ogen client for audit queries")
dsClient, err = ogenclient.NewClient(
    fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort),
    ogenclient.WithClient(http.DefaultClient),
)
Expect(err).ToNot(HaveOccurred())
GinkgoWriter.Println("✅ DataStorage ogen client ready for audit queries")
```

---

### 3. Update All Test Queries

**File**: `test/integration/signalprocessing/audit_integration_test.go`

**Replace all 17 instances** of SQL queries with ogen client calls:

**Before (WRONG)**:
```go
err := testDB.QueryRow(`
    SELECT COUNT(*)
    FROM audit_events
    WHERE event_type = $1
      AND correlation_id = $2
      AND actor_id = 'signalprocessing-service'
`, eventType, correlationID).Scan(&eventCount)
```

**After (CORRECT)**:
```go
params := ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString(eventType),
    CorrelationID: ogenclient.NewOptString(correlationID),
}
resp, err := dsClient.QueryAuditEvents(ctx, params)
Expect(err).ToNot(HaveOccurred())
eventCount := len(resp.Events)
```

---

## 📋 **Service Boundary Rules**

### **WHO Can Query the Database Directly**

| Service | Can Query DB? | Query Method |
|---------|---------------|--------------|
| **DataStorage** | ✅ YES | Direct SQL (owns the database) |
| **SignalProcessing** | ❌ NO | HTTP API (via ogen client) |
| **AIAnalysis** | ❌ NO | HTTP API (via ogen client) |
| **RemediationOrchestrator** | ❌ NO | HTTP API (via ogen client) |
| **Gateway** | ❌ NO | HTTP API (via ogen client) |
| **Notification** | ❌ NO | HTTP API (via ogen client) |

### **Integration Test Pattern**

```
✅ CORRECT Pattern:
┌─────────────────┐     HTTP API     ┌──────────────┐      SQL     ┌────────────┐
│ Service Tests   │ ──────────────>  │ DataStorage  │ ──────────> │ PostgreSQL │
│ (SP, AA, RO...) │                  │   Service    │             │  Database  │
└─────────────────┘                  └──────────────┘             └────────────┘

❌ WRONG Pattern:
┌─────────────────┐                                                ┌────────────┐
│ Service Tests   │ ──────────────────────────────────────────────> │ PostgreSQL │
│ (SP, AA, RO...) │          Direct SQL (VIOLATES BOUNDARY)        │  Database  │
└─────────────────┘                                                └────────────┘
```

---

## 🎯 **Why This Matters**

1. **Production Accuracy**: Tests should mirror production behavior (services use HTTP API, not direct DB)
2. **Service Independence**: Services should not know about DataStorage's internal database schema
3. **Schema Evolution**: Database schema changes should only affect DataStorage, not all services
4. **Security**: Database credentials should only be known to DataStorage service
5. **Testability**: HTTP API is the contract between services

---

## 📝 **Confidence Assessment**

**Confidence**: 100% that this is an architecture violation

**Evidence**:
- Production SignalProcessing uses `audit.BufferedStore` → HTTP API
- DataStorage owns the database schema
- Microservices principles require service boundaries
- Integration tests should test actual integration (HTTP API), not bypass it

---

## ✅ **Action Items**

1. **REVERT**: All changes that added direct database queries to SignalProcessing tests
2. **ADD**: ogen client setup in SignalProcessing suite
3. **UPDATE**: All 17 test queries to use ogen client HTTP API
4. **VERIFY**: Tests still validate business outcomes (not implementation)
5. **DOCUMENT**: Service boundary rules in testing guidelines

---

**Status**: ⚠️ BLOCKING - Must fix before continuing
**Priority**: P0-CRITICAL
**Impact**: Architecture violation, incorrect test patterns
