# NOTICE: Data Storage Audit Batch Endpoint Missing

**From**: AIAnalysis Team
**To**: Data Storage Team
**Date**: December 8, 2025
**Priority**: 🟡 P1 (HIGH) - Blocks audit integration tests
**Status**: 🔴 BLOCKING

---

## 📋 Summary

The Data Storage service's audit events handler (`handleCreateAuditEvent`) **only accepts single JSON objects**, but the authoritative design documents (DD-AUDIT-002, ADR-038) specify that the shared audit library (`pkg/audit/`) sends **batch arrays** of events.

This API contract mismatch is causing all audit integration tests to fail across services that use the shared audit library.

---

## 🔍 Root Cause Analysis

### Authoritative Documents Specify Batch API

**DD-AUDIT-002** (Audit Shared Library Design) - Lines 444-446:
```go
// DataStorageClient interface for writing audit events
type DataStorageClient interface {
    StoreBatch(ctx context.Context, events []*AuditEvent) error  // ← BATCH (array)
}
```

**ADR-038** (Async Buffered Audit Ingestion) - Lines 145-150:
- "Batches events (1000 events)"
- "Writes to Data Storage Service"

**DD-AUDIT-002** Section "Implementation Details" - Lines 553-555:
```go
// Write when batch is full
if len(batch) >= s.config.BatchSize {
    s.writeBatchWithRetry(batch)  // ← Sends array
```

### Data Storage Handler Expects Single Object

**File**: `pkg/datastorage/server/audit_events_handler.go` - Lines 100-116:
```go
// 1. Parse request body (JSON payload with all fields)
var payload map[string]interface{}  // ← SINGLE OBJECT, not array
if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
    s.logger.Info("Invalid JSON in request body",
        "error", err,  // "json: cannot unmarshal array into Go value of type map[string]interface {}"
```

### Error Message
```
json: cannot unmarshal array into Go value of type map[string]interface {}
```

---

## 📊 Impact Assessment

| Service | Impact | Severity |
|---------|--------|----------|
| **AIAnalysis** | Audit integration tests fail | 🟡 Medium (graceful degradation) |
| **Gateway** | Audit integration tests fail | 🟡 Medium |
| **Context API** | Audit integration tests fail | 🟡 Medium |
| **All Services** | Cannot verify audit persistence in E2E | 🔴 High |

**Business Impact**:
- DD-AUDIT-003 compliance cannot be verified in integration/E2E tests
- Audit trail persistence is untestable outside unit tests with mocks

---

## ✅ Recommended Solution

### Option A: Add Batch Endpoint (RECOMMENDED)

Add a new endpoint for batch writes that the shared library uses:

```go
// POST /api/v1/audit/events/batch
func (s *Server) handleCreateAuditEventsBatch(w http.ResponseWriter, r *http.Request) {
    var events []map[string]interface{}
    if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
        // Handle error
    }
    
    // Process each event in batch
    for _, event := range events {
        // ... existing single event logic
    }
}
```

**Pros**:
- ✅ Aligns with DD-AUDIT-002 and ADR-038
- ✅ Maintains backward compatibility (single event endpoint still works)
- ✅ Better performance (batch INSERT)

### Option B: Modify Existing Endpoint

Modify `handleCreateAuditEvent` to accept both single objects and arrays:

```go
var payload interface{}
if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
    // Handle error
}

switch v := payload.(type) {
case map[string]interface{}:
    // Single event (existing logic)
case []interface{}:
    // Batch of events (new logic)
}
```

**Cons**:
- ⚠️ More complex error handling
- ⚠️ Response format changes for batch

---

## 📋 Action Items for Data Storage Team

| # | Action | Priority | Status |
|---|--------|----------|--------|
| 1 | Review DD-AUDIT-002 and ADR-038 for batch write requirements | P0 | ⏳ Pending |
| 2 | Implement batch endpoint (`POST /api/v1/audit/events/batch`) | P0 | ⏳ Pending |
| 3 | Update `pkg/audit/http_client.go` to use batch endpoint | P1 | ⏳ Pending |
| 4 | Run AIAnalysis audit integration tests to verify | P1 | ⏳ Pending |
| 5 | Update OpenAPI spec if needed | P2 | ⏳ Pending |

---

## 🔗 Related Documents

- **DD-AUDIT-002**: `docs/architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md`
- **ADR-038**: `docs/architecture/decisions/ADR-038-async-buffered-audit-ingestion.md`
- **DD-AUDIT-003**: `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md`
- **Failing Test**: `test/integration/aianalysis/audit_integration_test.go`
- **Handler**: `pkg/datastorage/server/audit_events_handler.go`
- **Shared Library**: `pkg/audit/store.go`

---

## 🧪 Reproduction Steps

1. Start Data Storage with dependencies:
   ```bash
   podman-compose -f podman-compose.test.yml up -d postgres redis datastorage
   ```

2. Apply database migrations:
   ```bash
   awk '/\+goose Up/{flag=1} /\+goose Down/{flag=0} flag' migrations/013_create_audit_events_table.sql | \
     podman exec -i kubernaut_postgres_1 psql -U slm_user -d action_history
   ```

3. Run AIAnalysis audit integration tests:
   ```bash
   go test ./test/integration/aianalysis/... -v --count=1 -run "Audit"
   ```

4. Observe error:
   ```
   ERROR: Failed to write audit batch
   error: "Data Storage Service batch write returned status 400"
   
   Data Storage logs:
   Invalid JSON in request body: json: cannot unmarshal array into Go value of type map[string]interface {}
   ```

---

## ✅ Acceptance Criteria

The issue is resolved when:
- [ ] Data Storage accepts batch writes (array of audit events)
- [ ] `pkg/audit/http_client.go` successfully writes batches to Data Storage
- [ ] AIAnalysis audit integration tests pass
- [ ] Gateway audit integration tests pass (if applicable)

---

## 📞 Contact

**AIAnalysis Team**: Available for questions or clarification

---

## 📜 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | Dec 8, 2025 | AIAnalysis Team | Initial notice |

---

### Data Storage Team Response

```
⏳ AWAITING RESPONSE

Please acknowledge this notice and provide:
1. Confirmation of root cause analysis
2. Selected solution approach (A or B)
3. Estimated timeline for fix
```

