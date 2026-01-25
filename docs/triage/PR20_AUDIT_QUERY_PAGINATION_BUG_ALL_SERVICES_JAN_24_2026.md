# Critical Bug: Audit Query Pagination Not Handled Across All Integration Tests

**Date**: 2026-01-24
**Status**: 🚨 CRITICAL - Affects 5 services
**Severity**: HIGH (explains flakiness under concurrent load)
**Root Cause**: All integration tests query audit events without pagination handling

---

## 📊 **Impact Summary**

| Service | Test Files Affected | Default Limit | Pagination? | Status |
|---------|---------------------|---------------|-------------|--------|
| **Notification** | 1 file | 50 | ❌ → ✅ FIXED | Fixed |
| **Remediation Orchestrator** | 3 files | 100 | ❌ | **Needs Fix** |
| **AIAnalysis** | 5 files | 100 | ❌ | **Needs Fix** |
| **SignalProcessing** | 1 file | 50/100 | ❌ | **Needs Fix** |
| **WorkflowExecution** | 1 file | 50 | ❌ | **Needs Fix** |

**Total Files Affected**: **11 integration test files**

---

## 🔍 **Root Cause**

All integration tests use Data Storage REST API to query audit events, but **NONE handle pagination**:

```go
// ❌ BUGGY PATTERN (used everywhere)
resp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    EventType: ogenclient.NewOptString(eventType),
    Limit:     ogenclient.NewOptInt(100),  // Only gets first page!
})
return resp.Data  // Missing events beyond position 100!
```

**Under Concurrent Load**:
- 12 parallel test processes run simultaneously
- Hundreds of audit events created in seconds
- Test's events pushed beyond page 1 limit (50 or 100)
- Tests fail to find their events → flaky failures

---

## 🚨 **Evidence: Notification Test Failure (PR#20 Run 3/3)**

**Logs**:
```
04:13:02.670  POST /audit/events/batch: 121 events (61 correlation_ids)
04:13:02.774  GET /audit/events: "count": 50, "total": 212
              ^ Only 50 of 212 events returned!
```

**Result**: Test expected to find event with `correlation_id=remediation-audit-corr-1`, but it was beyond position 50 → timeout after 10 seconds.

---

## 📁 **Detailed File Analysis**

### 1. ✅ **Notification (FIXED)**

**File**: `test/integration/notification/controller_audit_emission_test.go`

**Before (BUGGY)**:
```go
queryAuditEvents := func(eventType, resourceID string) []ogenclient.AuditEvent {
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        EventCategory: ogenclient.NewOptString("notification"),
    }
    resp, err := dsClient.QueryAuditEvents(queryCtx, params)
    if err != nil || resp.Data == nil {
        return nil
    }
    return resp.Data  // ❌ Only first 50 events!
}
```

**After (FIXED)**:
```go
queryAuditEvents := func(eventType, resourceID string) []ogenclient.AuditEvent {
    var allEvents []ogenclient.AuditEvent
    offset := 0
    limit := 50

    // ✅ Fetch ALL pages
    for {
        params := ogenclient.QueryAuditEventsParams{
            EventType:     ogenclient.NewOptString(eventType),
            EventCategory: ogenclient.NewOptString("notification"),
            Limit:         ogenclient.NewOptInt(limit),
            Offset:        ogenclient.NewOptInt(offset),
        }
        resp, err := dsClient.QueryAuditEvents(queryCtx, params)
        if err != nil || resp.Data == nil {
            break
        }

        allEvents = append(allEvents, resp.Data...)

        // If fewer results than limit, we've reached the end
        if len(resp.Data) < limit {
            break
        }

        offset += limit
    }
    return allEvents
}
```

**Status**: ✅ FIXED

---

### 2. ❌ **Remediation Orchestrator (NEEDS FIX)**

**Files**:
1. `test/integration/remediationorchestrator/audit_phase_lifecycle_integration_test.go` (line 69)
2. `test/integration/remediationorchestrator/audit_emission_integration_test.go`
3. `test/integration/remediationorchestrator/audit_integration_test.go`

**Buggy Code** (audit_phase_lifecycle_integration_test.go:69-89):
```go
queryAuditEvents := func(correlationID, eventType string) ([]ogenclient.AuditEvent, error) {
    if dsClient == nil {
        return nil, fmt.Errorf("dsClient is nil")
    }

    eventCategory := "orchestration"
    limit := 100  // ❌ Only first 100 events!

    resp, err := dsClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
        CorrelationID: ogenclient.NewOptString(correlationID),
        EventCategory: ogenclient.NewOptString(eventCategory),
        EventType:     ogenclient.NewOptString(eventType),
        Limit:         ogenclient.NewOptInt(limit),
    })

    if err != nil {
        return nil, err
    }

    return resp.Data, nil  // ❌ Missing events beyond position 100!
}
```

**Impact**: Under high load (>100 orchestration events), tests will miss events and fail.

**Status**: ❌ NEEDS FIX

---

### 3. ❌ **AIAnalysis (NEEDS FIX)**

**Files**:
1. `test/integration/aianalysis/audit_provider_data_integration_test.go` (line 101)
2. `test/integration/aianalysis/audit_flow_integration_test.go`
3. `test/integration/aianalysis/error_handling_integration_test.go`
4. `test/integration/aianalysis/graceful_shutdown_test.go`
5. `test/integration/aianalysis/audit_integration_test.go`

**Buggy Code** (audit_provider_data_integration_test.go:101-119):
```go
queryAuditEvents := func(correlationID string, eventType *string) ([]ogenclient.AuditEvent, error) {
    limit := 100  // ❌ Only first 100 events!
    params := ogenclient.QueryAuditEventsParams{
        CorrelationID: ogenclient.NewOptString(correlationID),
        Limit:         ogenclient.NewOptInt(limit),
    }
    if eventType != nil {
        params.EventType = ogenclient.NewOptString(*eventType)
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return nil, fmt.Errorf("failed to query DataStorage: %w", err)
    }
    if resp.Data == nil {
        return []ogenclient.AuditEvent{}, nil
    }
    return resp.Data, nil  // ❌ Missing events beyond position 100!
}
```

**Impact**: Under high load (>100 AI events), tests will miss events and fail.

**Status**: ❌ NEEDS FIX

---

### 4. ❌ **SignalProcessing (NEEDS FIX)**

**File**: `test/integration/signalprocessing/audit_integration_test.go`

**Buggy Helpers**:
1. **`countAuditEvents`** (line 83): No limit specified (default 50)
2. **`countAuditEventsByCategory`** (line 110): No limit specified (default 50)
3. **`getLatestAuditEvent`** (line 127): Limit=1 (OK for latest)
4. **`getFirstAuditEvent`** (line 148): Limit=100 ❌

**Buggy Code** (getFirstAuditEvent:148-168):
```go
func getFirstAuditEvent(eventType, correlationID string) (*ogenclient.AuditEvent, error) {
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),
        Limit: ogenclient.NewOptInt(100),  // ❌ What if first event is at position 101?
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return nil, err
    }

    events := resp.Data
    if len(events) == 0 {
        return nil, nil
    }

    // Return the last event (earliest timestamp)
    return &events[len(events)-1], nil  // ❌ Wrong if >100 events!
}
```

**Impact**: Under high load, `getFirstAuditEvent` will return the wrong event.

**Status**: ❌ NEEDS FIX

---

### 5. ❌ **WorkflowExecution (NEEDS FIX)**

**File**: `test/integration/workflowexecution/audit_flow_integration_test.go`

**Buggy Code** (lines 168, 267, 282 - direct calls, no helper):
```go
// Line 168
resp, err := dsClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString(eventType),
    EventCategory: ogenclient.NewOptString(eventCategory),
    CorrelationID: ogenclient.NewOptString(correlationID),
    // ❌ No Limit specified - defaults to 50!
})
auditEvents = resp.Data  // ❌ Only first 50 events!

// Line 267
resp, err := dsClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(correlationID),
    // ❌ No Limit - defaults to 50!
})
return len(resp.Data)  // ❌ Wrong count if >50 events!

// Line 282
resp, err := dsClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(correlationID),
    // ❌ No Limit - defaults to 50!
})
auditEvents = resp.Data  // ❌ Only first 50 events!
```

**Impact**: Under high load (>50 workflow events), tests will miss events and fail.

**Status**: ❌ NEEDS FIX

---

## 💡 **Recommended Fix Pattern**

### **Standard Pagination Helper** (copy to all services)

```go
// queryAllAuditEvents fetches ALL audit events across ALL pages.
// Required for concurrent test environments where 100+ events may exist.
func queryAllAuditEvents(params ogenclient.QueryAuditEventsParams) ([]ogenclient.AuditEvent, error) {
    var allEvents []ogenclient.AuditEvent
    offset := 0
    limit := 100 // Reasonable page size

    for {
        // Set pagination params
        params.Limit = ogenclient.NewOptInt(limit)
        params.Offset = ogenclient.NewOptInt(offset)

        resp, err := dsClient.QueryAuditEvents(context.Background(), params)
        if err != nil {
            return nil, err
        }

        if resp.Data == nil || len(resp.Data) == 0 {
            break
        }

        // Accumulate events from this page
        allEvents = append(allEvents, resp.Data...)

        // If fewer results than limit, we've reached the end
        if len(resp.Data) < limit {
            break
        }

        // Move to next page
        offset += limit
    }

    return allEvents, nil
}
```

---

## 🎯 **Fix Priority**

### **Critical (Before PR Merge)**
1. ✅ **Notification** - FIXED
2. ❌ **Remediation Orchestrator** - 3 files
3. ❌ **AIAnalysis** - 5 files

**Rationale**: RO and AA have the most test files and are most likely to hit the >100 event threshold under load.

### **High (Before Next Release)**
4. ❌ **SignalProcessing** - 1 file (multiple helpers)
5. ❌ **WorkflowExecution** - 1 file (multiple direct calls)

**Rationale**: Fewer files, but still critical for CI reliability.

---

## 🏷️ **Classification**

- **Type**: Systemic Bug (Query Pattern)
- **Severity**: HIGH (affects all services)
- **Impact**: CI flakiness under concurrent load (12 procs)
- **Root Cause**: Missing pagination handling in audit query helpers
- **Resolution**: Add pagination loop to all query helpers

---

## 📝 **User Validation**

**User Quote**: "if the issue is the test logic not handling pagination, triage if this could be also happening in other tests that they are not handling pagination and that explains why they fail sometimes and are flaky"

**Result**: ✅ User was 100% correct - **ALL services** have the same pagination bug.

---

## ✅ **Success Criteria**

Fix is successful when:
- ✅ All 11 test files have pagination handling
- ✅ NT integration tests pass 10/10 runs under high load
- ✅ RO integration tests pass 10/10 runs under high load
- ✅ AA integration tests pass 10/10 runs under high load
- ✅ SP integration tests pass 10/10 runs under high load
- ✅ WE integration tests pass 10/10 runs under high load

---

**Document Status**: ✅ Active
**Created**: 2026-01-24
**Priority**: CRITICAL
**Owner**: Platform Team
