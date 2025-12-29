# WorkflowExecution E2E Audit Failures - Deep Investigation

**Date**: December 28, 2025
**Status**: 🔍 **AUDIT PIPELINE FULLY FUNCTIONAL** | ❌ **E2E TESTS FAILING** (Root cause TBD)
**Confidence**: 95% pipeline works | 70% on E2E test issue
**Priority**: HIGH - Blocks E2E test suite pass

---

## 🎯 **Executive Summary**

**AUDIT PIPELINE STATUS**: ✅ **100% FUNCTIONAL END-TO-END**
- Controller emits audit events correctly
- BufferedAuditStore buffers and sends batches successfully
- Data Storage receives, validates, and persists to PostgreSQL
- **23 workflow events verified in PostgreSQL** (12 started, 7 completed, 4 failed)

**E2E TEST STATUS**: ❌ **3/12 TESTS FAILING**
- Tests query Data Storage via OpenAPI client
- Infrastructure (port mapping, service) is configured correctly
- **Query endpoint works when tested manually**
- Tests timeout after 60s without finding events

---

## 🔬 **Investigation Timeline**

### **Phase 1: Controller Instrumentation** ✅

**Action**: Added comprehensive debug logging to `pkg/audit/store.go`

**Debug Logs Added**:
```go
// StoreAudit entry (line 227-232)
s.logger.Info("🔍 StoreAudit called",
    "event_type", event.EventType,
    "correlation_id", event.CorrelationId,
    "buffer_capacity", cap(s.buffer))

// Validation success (line 241-243)
s.logger.Info("✅ Validation passed, attempting to buffer event")

// Buffer success (line 252-256)
s.logger.Info("✅ Event buffered successfully",
    "total_buffered", newCount)
```

**Outcome**: Rebuilt controller with debug logging enabled

---

### **Phase 2: Pipeline Layer Validation** ✅

Verified **EVERY LAYER** of the audit pipeline end-to-end:

#### **Layer 1: Controller Emission** ✅ VERIFIED WORKING

**Evidence**:
```log
2025-12-28T18:54:14Z DEBUG Audit event recorded
  {"action": "workflow.started", "wfe": "e2e-lifecycle-1766948054441816000", "outcome": "success"}
2025-12-28T18:54:17Z DEBUG Audit event recorded
  {"action": "workflow.failed", "wfe": "e2e-audit-failure-1766948052736030000", "outcome": "failure"}
2025-12-28T18:54:20Z DEBUG Audit event recorded
  {"action": "workflow.completed", "wfe": "e2e-audit-fields-1766948052956811000", "outcome": "success"}
```

✅ **Controller logs "Audit event recorded" for all 3 event types**

---

#### **Layer 2: BufferedAuditStore Ingestion** ✅ VERIFIED WORKING

**Evidence**:
```log
2025-12-28T18:54:14Z INFO audit.audit-store 🔍 StoreAudit called
  {"event_type": "workflow.started", "correlation_id": "e2e-lifecycle-1766948054441816000",
   "buffer_capacity": 50000, "buffer_current_size": 0}
2025-12-28T18:54:14Z INFO audit.audit-store ✅ Validation passed, attempting to buffer event
  {"event_type": "workflow.started", "buffer_size_before": 0}
2025-12-28T18:54:14Z INFO audit.audit-store ✅ Event buffered successfully
  {"event_type": "workflow.started", "buffer_size_after": 0, "total_buffered": 16}
```

✅ **Events pass OpenAPI validation**
✅ **Events added to buffer channel** (`total_buffered` counter: 16 → 23)
✅ **Buffer consumed immediately** (`buffer_size_after: 0` shows backgroundWriter processing)

---

#### **Layer 3: Batch Sending** ✅ VERIFIED WORKING

**Evidence**:
```log
2025-12-28T18:53:59Z DEBUG audit.audit-store ✅ Wrote audit batch
  {"batch_size": 5, "attempt": 1, "write_duration": "10.643542ms"}
2025-12-28T18:54:14Z DEBUG audit.audit-store ✅ Wrote audit batch
  {"batch_size": 1, "attempt": 1, "write_duration": "8.427691ms"}
2025-12-28T18:54:22Z DEBUG audit.audit-store ✅ Wrote audit batch
  {"batch_size": 2, "attempt": 1, "write_duration": "2.549645ms"}
```

✅ **12+ successful batch writes**
✅ **Fast write latency** (2-27ms)
✅ **No write failures or retries**

---

#### **Layer 4: Data Storage Receipt** ✅ VERIFIED WORKING

**Evidence**:
```log
2025-12-28T18:54:13.363Z INFO datastorage server/handlers.go:119 HTTP request
  {"method": "POST", "path": "/api/v1/audit/events/batch", "status": 201, "duration": "3.112608ms"}
2025-12-28T18:54:14.369Z INFO datastorage server/handlers.go:119 HTTP request
  {"method": "POST", "path": "/api/v1/audit/events/batch", "status": 201, "duration": "7.878145ms"}
2025-12-28T18:54:21.370Z INFO datastorage server/handlers.go:119 HTTP request
  {"method": "POST", "path": "/api/v1/audit/events/batch", "status": 201, "duration": "10.938503ms"}
```

✅ **12+ POST requests received**
✅ **All return 201 Created**
✅ **Fast processing** (2-11ms)

---

#### **Layer 5: PostgreSQL Persistence** ✅ VERIFIED WORKING

**Evidence**:
```sql
-- Direct PostgreSQL query
SELECT COUNT(*), event_type FROM audit_events
WHERE event_type LIKE 'workflow.%'
GROUP BY event_type ORDER BY event_type;

 count |     event_type
-------+--------------------
     7 | workflow.completed
     4 | workflow.failed
    12 | workflow.started
(3 rows)
```

✅ **23 total workflow events persisted**
✅ **All 3 event types present**
✅ **Events have correct `event_category='workflow'` and `correlation_id` values**

**Sample Event**:
```sql
SELECT event_type, correlation_id, event_category FROM audit_events
WHERE event_type='workflow.started' LIMIT 1;

     event_type     |              correlation_id              | event_category
--------------------+-----------------------------------------+----------------
 workflow.started   | e2e-lifecycle-1766948054441816000       | workflow
```

✅ **Event structure matches E2E test query parameters**

---

## 🔍 **Phase 3: E2E Test Investigation** ⚠️ ISSUE IDENTIFIED

### **Infrastructure Validation** ✅

#### **Port Mapping** ✅ CORRECT
```bash
$ podman ps --filter "name=workflowexecution-e2e"
workflowexecution-e2e-control-plane:
  0.0.0.0:8081->30081/tcp  # ← localhost:8081 → NodePort 30081 → DS pod:8080
```

#### **Data Storage Service** ✅ CORRECT
```bash
$ kubectl get svc -n kubernaut-system datastorage
NAME          TYPE       CLUSTER-IP      EXTERNAL-IP   PORT(S)
datastorage   NodePort   10.96.137.123   <none>        8080:30081/TCP
```

#### **E2E Test URL** ✅ CORRECT
```go
// test/e2e/workflowexecution/02_observability_test.go:423
const dataStorageServiceURL = "http://localhost:8081"
// Comment: NodePort access per DD-TEST-001: localhost:8081 → NodePort 30081 → DS pod:8080
```

---

### **Manual Query Testing** ✅ WORKS PERFECTLY

#### **Test 1: Query by `event_category` only**
```bash
$ curl -s "http://localhost:8081/api/v1/audit/events?event_category=workflow&limit=5" | jq '.pagination.total'
23  # ← All 23 workflow events found
```

#### **Test 2: Query by `event_category` + `correlation_id` (E2E test parameters)**
```bash
$ curl -s "http://localhost:8081/api/v1/audit/events?event_category=workflow&correlation_id=e2e-lifecycle-1766948054441816000" \
  | jq '.data | length, .[].event_type'
2
"workflow.completed"
"workflow.started"
```

✅ **Query endpoint returns correct events with E2E test parameters**

---

### **E2E Test Query Logs** ⚠️ SUSPICIOUS FINDING

**Evidence**:
```log
2025-12-28T18:54:12.784Z INFO datastorage server/handlers.go:119 HTTP request
  {"method": "GET", "path": "/api/v1/audit/events", "status": 200, "bytes": 1094}
2025-12-28T18:54:12.799Z INFO datastorage server/handlers.go:119 HTTP request
  {"method": "GET", "path": "/api/v1/audit/events", "status": 200, "bytes": 1094}
2025-12-28T18:54:12.814Z INFO datastorage server/handlers.go:119 HTTP request
  {"method": "GET", "path": "/api/v1/audit/events", "status": 200, "bytes": 1094}
... (30+ more identical requests)
```

**Observations**:
- ✅ E2E tests ARE making queries to Data Storage
- ✅ All queries return 200 OK
- ⚠️ All queries return EXACTLY 1094 bytes (suspiciously consistent)
- ⚠️ Logs don't show query parameters (can't verify what tests are querying for)

**Comparison**:
```bash
# Empty result (no correlation_id match)
$ curl -s "http://localhost:8081/api/v1/audit/events?event_category=workflow&correlation_id=nonexistent" | wc -c
76

# Result with 2 events
$ curl -s "http://localhost:8081/api/v1/audit/events?event_category=workflow&correlation_id=e2e-lifecycle-1766948054441816000" | wc -c
2142
```

**⚠️ 1094 bytes doesn't match empty (76) or successful (2142) response sizes**

---

## 🧩 **Hypotheses**

### **Hypothesis 1: Timing Issue** ⏱️ (60% Confidence)

**Evidence**:
- E2E test queries: `18:54:12-13` (30+ rapid queries)
- First event persisted: `18:54:14+` (events written after queries)

**Counter-Evidence**:
- Tests use `Eventually(... 60*time.Second)` - should retry for 60 seconds
- Tests would query again every 2 seconds until timeout
- Later queries (after 18:54:14) should have found events

**Status**: ⚠️ Partially explains initial failures, but not full 60s timeout

---

### **Hypothesis 2: OpenAPI Client Response Parsing** 🔧 (75% Confidence - MOST LIKELY)

**Evidence**:
- Manual `curl` queries work perfectly
- E2E tests get 200 OK responses
- 1094-byte responses don't match known response sizes
- Tests timeout despite queries succeeding

**Possible Issue**: OpenAPI generated client (`dsgen.ClientWithResponses`) might:
- Fail to parse response JSON
- Return empty `resp.JSON200.Data` despite successful HTTP response
- Have type mismatch between expected and actual response structure

**Test**: Check E2E test logs for parsing errors (not yet done)

---

### **Hypothesis 3: Query Parameter Encoding** 🔤 (40% Confidence)

**Evidence**:
- Logs don't show query parameters
- Can't verify if E2E tests send correct parameters

**Possible Issue**: OpenAPI client might:
- Not URL-encode correlation_id (contains hyphenated UUIDs)
- Send different parameter names than expected
- Not send parameters at all

**Status**: ⚠️ Can't confirm without parameter logging

---

## 📊 **Current Status Summary**

| Component | Status | Evidence |
|-----------|--------|----------|
| **Controller → BufferedAuditStore** | ✅ WORKING | Debug logs show events buffered |
| **BufferedAuditStore → Data Storage** | ✅ WORKING | Batch write logs + POST requests |
| **Data Storage → PostgreSQL** | ✅ WORKING | 23 events in database |
| **Data Storage Query Endpoint** | ✅ WORKING | Manual curl queries succeed |
| **Port Mapping (8081→30081)** | ✅ CORRECT | Podman ps + manual queries work |
| **E2E Test URL Configuration** | ✅ CORRECT | `localhost:8081` matches port mapping |
| **E2E Test Queries** | ❌ FAILING | Tests timeout after 60s |

---

## 🎯 **Recommended Next Steps**

### **Step 1: Add OpenAPI Client Debug Logging** (PRIORITY 1)

**Goal**: Determine if OpenAPI client is parsing responses correctly

**Action**:
```go
// test/e2e/workflowexecution/02_observability_test.go
Eventually(func() int {
    resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
        EventCategory: &eventCategory,
        CorrelationId: &wfe.Name,
    })

    // ADD DEBUG LOGGING
    GinkgoWriter.Printf("🔍 Query response: status=%d, err=%v\n", resp.StatusCode(), err)
    if resp.JSON200 != nil {
        GinkgoWriter.Printf("   Data length: %d, Total: %d\n",
            len(*resp.JSON200.Data), *resp.JSON200.Pagination.Total)
    } else {
        GinkgoWriter.Printf("   JSON200 is nil! Body: %s\n", string(resp.Body))
    }

    // ... rest of test
}, 60*time.Second, 2*time.Second)
```

**Expected Outcome**:
- If response parsing fails → Fix OpenAPI client usage
- If parameters not sent → Fix parameter encoding
- If timing issue → Adjust test synchronization

---

### **Step 2: Enable Data Storage Query Parameter Logging** (PRIORITY 2)

**Goal**: Verify E2E tests send correct query parameters

**Action**: Add query parameter logging to Data Storage handler
```go
// cmd/datastorage/server/handlers.go (query handler)
log.Printf("Query params: event_category=%s, correlation_id=%s",
    params.EventCategory, params.CorrelationId)
```

**Expected Outcome**: Confirm E2E tests send correct parameters

---

### **Step 3: Direct OpenAPI Client Test** (PRIORITY 3)

**Goal**: Isolate OpenAPI client behavior outside E2E test

**Action**: Create minimal test:
```go
package main

import (
    "context"
    dsgen "github.com/jordigilh/kubernaut/api/generated/datastorage/go-client"
)

func main() {
    client, _ := dsgen.NewClientWithResponses("http://localhost:8081")

    eventCategory := "workflow"
    correlationID := "e2e-lifecycle-1766948054441816000"

    resp, err := client.QueryAuditEventsWithResponse(context.Background(),
        &dsgen.QueryAuditEventsParams{
            EventCategory: &eventCategory,
            CorrelationId: &correlationID,
        })

    fmt.Printf("Status: %d, Error: %v\n", resp.StatusCode(), err)
    fmt.Printf("JSON200 nil? %v\n", resp.JSON200 == nil)
    if resp.JSON200 != nil {
        fmt.Printf("Events: %d\n", len(*resp.JSON200.Data))
    }
}
```

**Expected Outcome**: Confirm if OpenAPI client works standalone

---

## 📝 **Key Findings**

### ✅ **Confirmed Working**
1. **Full audit pipeline** (controller → BufferedAuditStore → Data Storage → PostgreSQL)
2. **Event persistence** (23 events in database with correct structure)
3. **Query endpoint** (manual curl queries return correct events)
4. **Infrastructure** (port mapping, service config, E2E test URL)

### ❌ **Confirmed Failing**
1. **E2E test queries** (timeout after 60s without finding events)
2. **Response size mismatch** (1094 bytes doesn't match known good/empty responses)

### ⚠️ **Uncertain**
1. **OpenAPI client response parsing** (might fail silently)
2. **Query parameter encoding** (can't verify from logs)
3. **Timing synchronization** (tests query early, but Eventually should handle it)

---

## 🏆 **Success Metrics**

**Pipeline Validation**: ✅ **100% COMPLETE**
- All 5 layers verified working end-to-end
- Events persisted correctly in PostgreSQL
- No errors in any pipeline component

**E2E Test Fix**: ⏳ **IN PROGRESS**
- Root cause narrowed to OpenAPI client or test code
- Infrastructure confirmed correct
- Next step: Add debug logging to pinpoint exact failure

---

## 📚 **References**

### **Files Modified**
- `pkg/audit/store.go` (lines 227-256): Added comprehensive debug logging

### **Key Evidence Locations**
- Controller logs: `kubectl logs -n kubernaut-system -l app=workflowexecution-controller`
- Data Storage logs: `kubectl logs -n kubernaut-system -l app=datastorage`
- PostgreSQL: `kubectl exec -n kubernaut-system <postgres-pod> -- psql -U slm_user -d action_history`
- E2E test: `test/e2e/workflowexecution/02_observability_test.go` (lines 423, 470, 576, 680)

### **Related Documents**
- ADR-032: Audit Trail Persistence (mandate for audit)
- DD-AUDIT-003: Audit event specifications
- DD-TEST-001: E2E test infrastructure standards

---

**Document Version**: 1.0
**Last Updated**: December 28, 2025
**Investigator**: AI Assistant
**Status**: Investigation ongoing - awaiting debug logging results

