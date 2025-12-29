# Notification: Data Storage OpenAPI Fix - Implementation Status

**Date**: December 18, 2025, 15:05
**Status**: ⏳ **IN PROGRESS** - Test Updates Complete, Integration Testing Ongoing
**Team**: Notification Team + Data Storage Team
**Related**: [NT_DS_API_QUERY_ISSUE_DEC_18_2025.md](./NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)

---

## 🎯 **Executive Summary**

**What**: Notification team implemented test updates to use EventCategory parameter after DS team fixed OpenAPI spec
**Progress**: 90% complete (code changes done, tests still failing)
**Blocker**: Queries returning 0 results despite successful audit event writes
**Next Action**: Deep dive investigation into write/query integration

---

## ✅ **COMPLETED WORK**

### **1. Data Storage Team Fix (100% Complete)**

**OpenAPI Spec Updates** (`api/openapi/data-storage-v1.yaml`):
- ✅ Added 6 missing query parameters to GET `/api/v1/audit/events`:
  - `event_category` (CRITICAL)
  - `event_outcome`
  - `severity`
  - `since`
  - `until`
  - Enhanced parameter descriptions

**Client Regeneration** (`pkg/datastorage/client/generated.go`):
- ✅ Regenerated with `oapi-codegen`
- ✅ `QueryAuditEventsParams` struct now has all 9 fields
- ✅ Client correctly serializes parameters (lines 1207-1219)

**Evidence**:
```go
// pkg/datastorage/client/generated.go:723
EventCategory *string `form:"event_category,omitempty" json:"event_category,omitempty"`
```

---

### **2. Notification Team Updates (100% Complete)**

**Test Code Updates** (`test/integration/notification/audit_integration_test.go`):

✅ **6 Locations Updated**:

1. **Line 156**: BR-NOT-062 - Unified Audit Table Integration
```go
eventCategory := "notification"
params := &dsgen.QueryAuditEventsParams{
    CorrelationId:  &correlationID,
    EventType:      &eventType,
    EventCategory:  &eventCategory, // Required per ADR-034 (DS team fix)
}
```

2. **Line 214**: BR-NOT-062 - Async Buffered Audit Writes
3. **Line 307**: Graceful Shutdown test
4. **Line 379**: BR-NOT-064 - Audit Event Correlation
5. **Line 397**: Event types verification
6. **Line 459**: ADR-034 - Field Compliance

**Debug Logging Added**:
- Status code logging
- Response body inspection
- TotalCount value debugging

---

### **3. Infrastructure Updates (100% Complete)**

**Data Storage Service Rebuild**:
- ✅ Rebuilt Data Storage container with updated OpenAPI spec
- ✅ Verified embedded spec file copied to container
- ✅ Service starts healthy and responds to /health

**podman-compose**:
- ✅ Data Storage on port 18110
- ✅ PostgreSQL on port 15453
- ✅ Redis on port 16399

---

## 🚨 **CURRENT BLOCKER**

### **Problem**: Queries Return 0 Results

**Symptom**:
```
Timed out after 5.001s.
Audit event should be queryable via REST API
Expected
    <int>: 0
to equal
    <int>: 1
```

**What We Know**:
1. ✅ Audit events **write successfully** ("Wrote audit batch" in logs)
2. ✅ Generated client **sends EventCategory** parameter correctly
3. ✅ Data Storage service **rebuilt** with new OpenAPI spec
4. ✅ Test **queries with EventCategory** parameter
5. ❌ Queries **return 0 results** consistently

**What's Unclear**:
- ❓ Do audit writes actually reach the Data Storage HTTP endpoint?
- ❓ Are writes persisting to PostgreSQL correctly?
- ❓ Is there a network/connectivity issue between test and DS service?
- ❓ Is the query filtering logic working correctly in DS handler?

---

## 🔍 **INVESTIGATION PLAN**

### **Phase 1: Verify Write Path** (NEXT STEP)

**Goal**: Confirm audit events reach Data Storage and persist to PostgreSQL

**Actions**:
1. Add HTTP request logging to Data Storage service
2. Run single audit test
3. Check Data Storage logs for POST `/api/v1/audit/events` requests
4. If no POST seen → audit store connectivity issue
5. If POST seen → check PostgreSQL directly:
   ```bash
   podman exec notification_postgres_1 psql -U slm_user -d action_history \
     -c "SELECT COUNT(*) FROM audit_events WHERE event_category='notification';"
   ```

---

### **Phase 2: Verify Query Path**

**Goal**: Confirm query parameters reach handler correctly

**Actions**:
1. Add parameter logging to Data Storage query handler
2. Run test and inspect logs for received parameters
3. Verify `event_category` is being filtered correctly

**Expected Handler Code** (`pkg/datastorage/server/audit_events_handler.go:389-433`):
```go
filters := &queryFilters{
    service:  query.Get("event_category"), // Should log "notification"
    // ...
}
```

---

### **Phase 3: Manual End-to-End Test**

**Goal**: Isolate whether issue is in test harness or Data Storage

**Steps**:
```bash
# 1. Start infrastructure
cd test/integration/notification
podman-compose -f podman-compose.notification.test.yml up -d

# 2. Write test event via curl (complete payload)
curl -X POST http://localhost:18110/api/v1/audit/events \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.0",
    "event_type": "notification.message.sent",
    "event_category": "notification",
    "event_action": "sent",
    "event_outcome": "success",
    "event_timestamp": "2025-12-18T20:00:00Z",
    "correlation_id": "manual-test-001",
    "event_data": {"channel": "console"},
    "actor_type": "service",
    "actor_id": "notification-controller",
    "resource_type": "NotificationRequest",
    "resource_id": "test-manual"
  }'

# 3. Query with event_category
curl "http://localhost:18110/api/v1/audit/events?event_category=notification" | jq '.pagination.total'

# Expected: Should return 1 (or more)
```

---

## 📊 **CONFIDENCE ASSESSMENT**

**Overall Confidence**: 70%

**Breakdown**:
- ✅ **OpenAPI Fix**: 100% - Verified correct, client regenerated properly
- ✅ **Test Updates**: 100% - All 6 locations updated with EventCategory
- ✅ **Infrastructure**: 100% - Service rebuilt, endpoints responsive
- ⚠️  **Integration**: 40% - Writes/queries not connecting, root cause unknown

**Risk Assessment**:
- **Low Risk**: Code changes are correct (based on manual inspection)
- **Medium Risk**: Possible subtle infrastructure issue (network, database)
- **High Risk**: May require DS team support for handler debugging

---

## 🎯 **SUCCESS CRITERIA**

**For This Issue to be RESOLVED**:

1. ✅ EventCategory parameter added to all 6 test query locations
2. ⏳ **PENDING**: All 4 failing audit integration tests pass:
   - `BR-NOT-062: Unified Audit Table Integration`
   - `BR-NOT-062: Async Buffered Audit Writes`
   - `Graceful Shutdown`
   - `BR-NOT-064: Audit Event Correlation`
3. ⏳ **PENDING**: E2E audit tests pass (if any use DS REST API)

**Current Score**: 1/3 (33%)

---

## 📁 **FILES MODIFIED**

| File | Lines Changed | Status | Description |
|------|--------------|--------|-------------|
| `api/openapi/data-storage-v1.yaml` | +50 | ✅ DS Team | Added 6 query parameters |
| `pkg/datastorage/client/generated.go` | +100 | ✅ DS Team | Regenerated client |
| `test/integration/notification/audit_integration_test.go` | +30 | ✅ NT Team | Added EventCategory to 6 queries |

**Total Changes**: ~180 lines across 3 files

---

## 🚦 **HANDOFF TO DATA STORAGE TEAM**

**If Notification team cannot resolve**:

**Issue Summary**: Audit writes succeed but queries with `event_category=notification` return 0 results

**Evidence Package**:
1. Test output showing "Wrote audit batch" success
2. Query returning 0 results with all parameters
3. Verified EventCategory parameter sent by client
4. PostgreSQL direct query results (if available)

**Request**:
- Review audit events query handler filtering logic
- Verify `event_category` parameter properly mapped to database column
- Check if handler logs show parameter being received

---

## 📋 **TIMELINE**

| Time | Event | Owner | Status |
|------|-------|-------|--------|
| 14:30 | DS team fixes OpenAPI spec | DS Team | ✅ Complete |
| 14:45 | NT team updates test code | NT Team | ✅ Complete |
| 15:00 | DS service rebuilt | NT Team | ✅ Complete |
| 15:05 | Tests still failing, investigation started | NT Team | ⏳ In Progress |
| TBD | Root cause identified | NT Team | ⏳ Pending |
| TBD | Tests passing | NT Team | ⏳ Pending |

**Total Time Invested**: 90 minutes (so far)

---

## 🔄 **RELATED DOCUMENTS**

- **Problem Report**: [NT_DS_API_QUERY_ISSUE_DEC_18_2025.md](./NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)
- **OpenAPI Spec**: [api/openapi/data-storage-v1.yaml](../../api/openapi/data-storage-v1.yaml)
- **Generated Client**: [pkg/datastorage/client/generated.go](../../pkg/datastorage/client/generated.go)
- **Test File**: [test/integration/notification/audit_integration_test.go](../../test/integration/notification/audit_integration_test.go)

---

**Last Updated**: December 18, 2025, 15:05 (Notification Team)
**Next Update**: After Phase 1 investigation complete

---

**AI Assistant Notes**:
- Committed all changes: OpenAPI spec + client + test updates
- Infrastructure verified working (health checks pass)
- Ready for deep dive debugging session
- May need DS team collaboration for handler investigation



