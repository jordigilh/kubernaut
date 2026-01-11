# SignalProcessing Audit Test Refactoring Pattern

**Date**: January 10, 2026
**Status**: Phase 2 - In Progress
**Reference**: HTTP_ANTIPATTERN_REFACTORING_ANSWERS_JAN10_2026.md (Q7, Q8)

---

## 🎯 Refactoring Goal

Replace HTTP audit queries with direct PostgreSQL queries in `audit_integration_test.go`.

**Anti-Pattern** (BEFORE):
- Uses `ogenclient.NewClient(dataStorageURL)` - HTTP client
- Queries via `auditClient.QueryAuditEvents(...)` - HTTP API
- Validates via `ogenclient.AuditEvent` structs - HTTP response types

**Correct Pattern** (AFTER):
- Uses `testDB *sql.DB` - Direct PostgreSQL connection (from suite)
- Queries via `testDB.QueryRow(...)` - SQL queries
- Validates via database rows - Direct column access

---

## 📋 Refactoring Checklist

### **Phase 2a: Remove HTTP Dependencies** ✅
- [x] Add PostgreSQL imports to suite_test.go
- [x] Add `testDB *sql.DB` package variable
- [x] Initialize testDB in SynchronizedBeforeSuite
- [x] Add testDB cleanup in SynchronizedAfterSuite

### **Phase 2b: Refactor Test File**
- [ ] Remove `ogenclient` import from audit_integration_test.go
- [ ] Replace HTTP health check with PostgreSQL ping (or remove)
- [ ] Refactor 7 test cases to use PostgreSQL queries
- [ ] Update validation logic to use SQL row data
- [ ] Remove all `dataStorageURL` references

---

## 🔄 Refactoring Pattern (Applies to ALL 7 Test Cases)

### **BEFORE: HTTP Query Pattern**

```go
// Create HTTP client
auditClient, err := ogenclient.NewClient(dataStorageURL)
Expect(err).ToNot(HaveOccurred())

eventType := "signalprocessing.signal.processed"
var auditEvents []ogenclient.AuditEvent

// Query via HTTP
Eventually(func() int {
    resp, err := auditClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),
    })
    if err != nil {
        return 0
    }
    auditEvents = resp.Data
    return len(auditEvents)
}, 120*time.Second, 1*time.Second).Should(BeNumerically(">=", 1))

// Validate via HTTP response struct
var processedEvent *ogenclient.AuditEvent
for i := range auditEvents {
    if auditEvents[i].EventType == "signalprocessing.signal.processed" {
        processedEvent = &auditEvents[i]
        break
    }
}
Expect(processedEvent).ToNot(BeNil())
Expect(processedEvent.EventCategory).To(Equal("signalprocessing"))
```

### **AFTER: PostgreSQL Query Pattern**

```go
// Query PostgreSQL directly (no HTTP client)
var eventCount int
Eventually(func() int {
    err := testDB.QueryRow(`
        SELECT COUNT(*)
        FROM audit_events
        WHERE event_type = $1
          AND correlation_id = $2
          AND service_name = 'SignalProcessing'
    `, "signalprocessing.signal.processed", correlationID).Scan(&eventCount)
    if err != nil {
        return 0
    }
    return eventCount
}, 120*time.Second, 1*time.Second).Should(BeNumerically(">=", 1))

// Validate via PostgreSQL row
var (
    eventID        string
    eventTimestamp time.Time
    eventCategory  string
    eventAction    string
    eventData      []byte // JSONB stored as bytes
)
err := testDB.QueryRow(`
    SELECT event_id, event_timestamp, event_category, event_action, event_data
    FROM audit_events
    WHERE event_type = $1
      AND correlation_id = $2
      AND service_name = 'SignalProcessing'
    ORDER BY event_timestamp DESC
    LIMIT 1
`, "signalprocessing.signal.processed", correlationID).Scan(
    &eventID,
    &eventTimestamp,
    &eventCategory,
    &eventAction,
    &eventData,
)
Expect(err).ToNot(HaveOccurred())

// Validate fields
Expect(eventCategory).To(Equal("signalprocessing"))
Expect(eventAction).To(Equal("processed"))

// Validate event_data JSONB
var eventDataMap map[string]interface{}
err = json.Unmarshal(eventData, &eventDataMap)
Expect(err).ToNot(HaveOccurred())
Expect(eventDataMap["environment"]).To(Equal("production"))
```

---

## 📊 audit_events Table Schema (Reference)

```sql
CREATE TABLE audit_events (
    -- Event Identity
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',

    -- Temporal Information
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL,  -- Partition key

    -- Event Classification
    event_type VARCHAR(100) NOT NULL,        -- 'signalprocessing.signal.processed'
    event_category VARCHAR(50) NOT NULL,     -- 'signal' or 'signalprocessing'
    event_action VARCHAR(50) NOT NULL,       -- 'processed', 'classification', etc.

    -- Context
    service_name VARCHAR(50) NOT NULL,       -- 'SignalProcessing'
    correlation_id UUID NOT NULL,            -- For querying related events
    causation_id UUID,

    -- Kubernetes Context
    namespace VARCHAR(253),
    resource_kind VARCHAR(63),
    resource_name VARCHAR(253),

    -- Event Data (JSONB)
    event_data JSONB NOT NULL,               -- Signal-specific data

    -- Primary Key (includes partition key)
    PRIMARY KEY (event_id, event_date)
);
```

---

## 🎯 7 Test Cases to Refactor

### **Test 1: Signal Processing Completion Auditing (BR-SP-090)** ⏳
- **File**: audit_integration_test.go:100-229
- **Event Type**: `signalprocessing.signal.processed`
- **HTTP Calls**: 1 (line 155)
- **Validation**: Event category, action, outcome, actor, event_data fields

### **Test 2: Classification Decision Auditing**
- **File**: audit_integration_test.go:231-331
- **Event Type**: `signalprocessing.classification.decision`
- **HTTP Calls**: 1 (line 285)
- **Validation**: Classification category, priority, event_data

### **Test 3: Business Classification Auditing**
- **File**: audit_integration_test.go:333-433
- **Event Type**: `signalprocessing.business.classified`
- **HTTP Calls**: 1 (line 383)
- **Validation**: Business unit, event_data

### **Test 4: Enrichment Completed Auditing**
- **File**: audit_integration_test.go:435-566
- **Event Type**: `signalprocessing.enrichment.completed`
- **HTTP Calls**: 1 (line 509)
- **Validation**: Kubernetes enrichment data, degraded mode flags

### **Test 5: Phase Transition Auditing**
- **File**: audit_integration_test.go:568-695
- **Event Type**: `signalprocessing.phase.transition`
- **HTTP Calls**: 1 (line 611)
- **Validation**: Phase information, from_phase, to_phase

### **Test 6: Error Occurred Auditing**
- **File**: audit_integration_test.go:697-808
- **Event Type**: `signalprocessing.error.occurred`
- **HTTP Calls**: 1 (line 727)
- **Validation**: Error outcome, error_data fields

### **Test 7: Fatal Error Auditing**
- **File**: audit_integration_test.go:810-924
- **Event Type**: `signalprocessing.error.occurred`
- **HTTP Calls**: 1 (line 859)
- **Validation**: Fatal error details, namespace errors

---

## ⚙️ Implementation Strategy

### **Option A: In-Place Refactoring** (Conservative, safer)
1. Refactor each test case one at a time
2. Run test after each refactoring to verify
3. Commit after each successful test case
4. **Effort**: 1 hour (15 min setup + 7 test cases * 5 min each)

### **Option B: Bulk Refactoring** (Faster, riskier)
1. Create new version of entire file with all changes
2. Run all tests once at end
3. Fix any issues found
4. Single commit for entire refactoring
5. **Effort**: 45 min (if no issues)

**Recommendation**: **Option A** - Safer, easier to debug if issues arise

---

## 🔧 Common SQL Query Patterns

### **Pattern 1: Count Events**
```sql
SELECT COUNT(*)
FROM audit_events
WHERE event_type = $1
  AND correlation_id = $2
  AND service_name = 'SignalProcessing'
```

### **Pattern 2: Fetch Single Event**
```sql
SELECT event_id, event_timestamp, event_category, event_action, event_data
FROM audit_events
WHERE event_type = $1
  AND correlation_id = $2
  AND service_name = 'SignalProcessing'
ORDER BY event_timestamp DESC
LIMIT 1
```

### **Pattern 3: Fetch All Events (For Multi-Event Tests)**
```sql
SELECT event_id, event_type, event_category, event_action, event_data
FROM audit_events
WHERE correlation_id = $1
  AND service_name = 'SignalProcessing'
ORDER BY event_timestamp ASC
```

### **Pattern 4: Validate JSONB event_data**
```go
var eventData []byte
err := testDB.QueryRow(`...`).Scan(..., &eventData)
Expect(err).ToNot(HaveOccurred())

var eventDataMap map[string]interface{}
err = json.Unmarshal(eventData, &eventDataMap)
Expect(err).ToNot(HaveOccurred())

Expect(eventDataMap["environment"]).To(Equal("production"))
Expect(eventDataMap["priority"]).To(Equal("P2"))
```

---

## ✅ Success Criteria

- [ ] All 7 test cases refactored to use PostgreSQL queries
- [ ] No `ogenclient` imports remain
- [ ] No HTTP calls to DataStorage API
- [ ] All tests pass with `make test-integration-signalprocessing`
- [ ] Test execution time similar or faster (no HTTP overhead)
- [ ] Validation logic remains comprehensive (no shortcuts)

---

**Status**: ⏳ Phase 2a Complete (Suite Setup) → Now starting Phase 2b (Test Refactoring)
**Estimated Time Remaining**: 45 min (7 test cases * 5 min each + validation)
