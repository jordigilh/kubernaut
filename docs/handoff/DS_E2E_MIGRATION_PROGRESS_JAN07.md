# DataStorage E2E OpenAPI Client Migration - Progress Tracker

**Date**: January 7, 2026
**Status**: 🔄 **IN PROGRESS** - Phase 1 Step 3
**Authority**: DD-API-001 (OpenAPI Client Mandate)

---

## 📊 **Overall Progress**

| Phase | Status | Progress |
|---|---|---|
| **Step 0: Pre-generation** | ✅ COMPLETE | 100% |
| **Step 1: Suite Setup** | ✅ COMPLETE | 100% |
| **Step 2: Helper Functions** | ✅ COMPLETE | 100% |
| **Step 3: Migrate Test Files** | 🔄 IN PROGRESS | **25%** (3/12 files) |
| **Step 4: Verification** | ⏳ PENDING | 0% |

**Total Progress**: **Phase 1: 62.5%** (2.5/4 steps complete)

---

## ✅ **Completed Work**

### **Step 0: Pre-Generation Validation** (10 min) ✅
- Created `pkg/datastorage/client/doc.go` with `//go:generate` directive
- Added `make generate-datastorage-client` target
- Modified `test-e2e-datastorage` to auto-validate client before tests
- **Benefit**: Caught real drift (removed obsolete `UserIdHeaderScopes`)

### **Step 1: Suite Setup** (30 min) ✅
- Added `dsClient *dsgen.ClientWithResponses` global variable
- Initialized `dsClient` in both SynchronizedBeforeSuite functions
- Used `testutil.NewMockUserTransport` for E2E (no oauth-proxy)
- **Benefit**: Type-safe OpenAPI client available to all tests

### **Step 2: Helper Functions** (1 hour) ✅
- Created `createAuditEventOpenAPI()` - Type-safe wrapper
- Created `convertMapToAuditEventRequest()` - Map to struct converter
- Created `createAuditEventFromMap()` - Backward compatibility
- **Benefit**: Incremental migration path for tests

### **Step 3: Test File Migrations** 🔄

#### **File 1/12: `01_happy_path_test.go`** ✅ (100%)
- **Migrated**: 5 POST calls, 1 GET query
- **Removed**: 60 lines (raw HTTP boilerplate)
- **Changes**:
  - Replaced `postAuditEvent()` with `createAuditEventFromMap()`
  - Replaced `httpClient.Get()` with `QueryAuditEventsWithResponse()`
  - Removed `postAuditEvent()` helper function
  - Removed imports: `bytes`, `encoding/json`, `io`
- **Status**: ✅ Committed (2d510b897)

#### **File 2/12: `02_dlq_fallback_test.go`** ✅ (100%)
- **Migrated**: 2 POST calls
- **Changes**:
  - Replaced 2 `postAuditEvent()` calls
  - Updated response handling
- **Status**: ✅ Committed (68d4605e0)

#### **File 3/12: `03_query_api_timeline_test.go`** 🔄 (37.5%)
- **Migrated**: 3 POST calls (in loops)
- **Remaining**: 8 GET queries with complex filtering
- **Status**: 🔄 Partially committed (579b574f5)
- **Next**: Migrate 8 `httpClient.Get()` calls to OpenAPI client

---

## 🔄 **Remaining Work**

### **File 3/12: `03_query_api_timeline_test.go`** (Continuation)
**Remaining**: 8 GET queries
**Pattern**:
```go
// Before (❌ FORBIDDEN):
resp, err := httpClient.Get(fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", serviceURL, correlationID))
var queryResponse map[string]interface{}
json.NewDecoder(resp.Body).Decode(&queryResponse)

// After (✅ REQUIRED):
queryResp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,
})
// Use queryResp.JSON200.Data (typed []dsgen.AuditEvent)
```

### **Files 4-12: Remaining Test Files**
Need to analyze for raw HTTP usage:
- `04_workflow_search_test.go`
- `06_workflow_search_audit_test.go`
- `07_workflow_version_management_test.go`
- `08_workflow_search_edge_cases_test.go`
- `09_event_type_jsonb_comprehensive_test.go`
- `10_malformed_event_rejection_test.go`
- `11_connection_pool_exhaustion_test.go`

**Analysis Needed**: Grep for `httpClient.Get`, `httpClient.Post`, `postAuditEvent` to identify violations

---

## 📈 **Metrics**

| Metric | Current | Target | Progress |
|---|---|---|---|
| **Files Migrated** | 2.5/12 | 12 | 21% |
| **Raw HTTP Calls Removed** | 8+ | ~30 | 27% |
| **Code Reduction** | ~75 lines | ~200 lines | 37% |
| **Time Invested** | ~3.5 hours | ~6 hours | 58% |
| **Time Remaining** | ~2.5 hours | - | - |

---

## 🎯 **Next Actions**

### **Immediate** (Next 30 min)
1. Complete `03_query_api_timeline_test.go` (8 GET queries)
2. Commit completed file

### **Short-term** (Next 1-2 hours)
3. Analyze files 4-12 for raw HTTP usage
4. Migrate files 4-12 (batch processing)
5. Commit each batch

### **Final** (30 min)
6. Run Step 4 verification
7. Confirm no raw HTTP remains
8. Run E2E tests to validate

---

## 🚧 **Challenges Identified**

1. **Complex Query Patterns**: File 03 has 8 different query patterns with various filters
2. **Response Handling**: Need to convert `map[string]interface{}` to typed structs
3. **Timestamp Parsing**: Simplified with typed `event.EventTimestamp` (no more string parsing)
4. **Volume**: More files to migrate than initially estimated

---

## ✅ **Success Patterns Established**

1. **POST Replacement**:
   ```go
   // Simple: createAuditEventFromMap(ctx, dsClient, eventMap)
   // Removes: 3-5 lines of boilerplate per call
   ```

2. **GET Query Replacement**:
   ```go
   // Before: 10+ lines (httpClient.Get, json.Decode, type assertions)
   // After: 3-5 lines (QueryAuditEventsWithResponse, typed access)
   ```

3. **Type Safety**:
   ```go
   // Before: data.([]interface{}), event["timestamp"].(string)
   // After: *queryResp.JSON200.Data, event.EventTimestamp
   ```

---

## 📋 **Commit History**

1. `0946aded4` - Step 0: Pre-generation validation
2. `0e39d9c0f` - Step 1: Suite setup
3. `d6fa18cdd` - Step 2: Helper functions
4. `2d510b897` - File 1: `01_happy_path_test.go`
5. `68d4605e0` - File 2: `02_dlq_fallback_test.go`
6. `579b574f5` - File 3: `03_query_api_timeline_test.go` (partial)

---

**Last Updated**: January 7, 2026
**Estimated Completion**: ~2.5 hours remaining
**Blocker Status**: None - proceeding with migration
**Authority**: DD-API-001 (OpenAPI Client Mandate)

