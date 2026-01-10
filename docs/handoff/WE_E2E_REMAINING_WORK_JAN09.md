# WorkflowExecution E2E Tests - Remaining Work (Jan 09, 2026)

**Date**: 2026-01-09
**Status**: 🟡 **E2E Infrastructure Fixed, Test Code Migration Needed**
**Team**: WorkflowExecution
**Priority**: MEDIUM - Infrastructure working, need to update test code

---

## 🎉 **INFRASTRUCTURE SUCCESS**

✅ **MAJOR ACHIEVEMENT**: AuthWebhook deployment issue **PERMANENTLY RESOLVED**

- **Root Cause**: Different waiting strategies (kubectl wait vs Pod API polling)
- **Solution**: Implemented `waitForAuthWebhookPodReady()` with direct Pod API polling
- **Result**: AuthWebhook deploys successfully, E2E infrastructure fully operational
- **Authority**: DD-TEST-008 (K8s v1.35.0 probe bug workaround)

**Files Modified**:
- `test/infrastructure/authwebhook_shared.go` - Added Pod API polling function
- `docker/workflowexecution-controller.Dockerfile` - ARM64 fix (upstream Go builder)
- `docker/webhooks.Dockerfile` - ARM64 fix (upstream Go builder)

**Verification**: AuthWebhook E2E tests pass 100% (2/2 tests)

---

## 🔧 **REMAINING WORK: Update E2E Test Code**

### **Issue**: E2E Test File Needs ogen Client API Migration

**File**: `test/e2e/workflowexecution/02_observability_test.go`

**Problem**: Test code still uses old client API pattern:
```go
// OLD (broken):
auditClient, err := dsgen.NewClientWithResponses(url)
resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &params)
if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
    ...
}
auditEvents = resp.JSON200.Data
```

**Required**: Update to new ogen client API (same as integration tests):
```go
// NEW (correct):
auditClient, err := ogenclient.NewClient(url)
resp, err := auditClient.QueryAuditEvents(ctx, params)
if err != nil {
    ...
}
auditEvents = resp.Data
```

### **Specific Changes Needed**

1. **Import Statement** ✅ (DONE)
   ```go
   import ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
   ```

2. **Client Creation** (3 locations - lines 464, 583, 703)
   ```go
   - auditClient, err := ogenclient.NewClientWithResponses(url)
   + auditClient, err := ogenclient.NewClient(url)
   ```

3. **Query Parameters** (3 locations)
   ```go
   - QueryAuditEventsWithResponse(ctx, &params)
   + QueryAuditEvents(ctx, params)  // No pointer!

   - EventCategory: &eventCategory
   + EventCategory: ogenclient.NewOptString(eventCategory)

   - CorrelationId: &wfe.Name
   + CorrelationID: ogenclient.NewOptString(wfe.Name)  // Capital I!
   ```

4. **Response Handling** (3 locations)
   ```go
   - if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil
   + if err != nil  // Check error only

   - auditEvents = resp.JSON200.Data
   + auditEvents = resp.Data

   - resp.JSON200.Pagination.Value.Total.Value
   + resp.Pagination.Value.Total.Value
   ```

5. **Event Category Value** (3 locations)
   ```go
   - eventCategory := "workflow"
   + eventCategory := "workflowexecution"  // Per ADR-034 v1.5
   ```

6. **Event Type Strings** ✅ (DONE)
   ```go
   - "workflow.started"
   + "workflowexecution.workflow.started"

   - "workflow.completed"
   + "workflowexecution.workflow.completed"

   - "workflow.failed"
   + "workflowexecution.workflow.failed"
   ```

7. **Event Category Constant** ✅ (DONE)
   ```go
   - ogenclient.AuditEventEventCategoryWorkflow
   + ogenclient.AuditEventEventCategoryWorkflowexecution
   ```

8. **Event Data Access** (1 location - line 649)
   ```go
   - eventData, ok := failedEvent.EventData.(WorkflowExecutionAuditPayload)
   + eventData, ok := failedEvent.EventData.GetWorkflowExecutionAuditPayload()
   ```

---

## 📋 **DETAILED LOCATIONS**

### **Location 1**: Lines 464-496 (First audit test)
**Function**: `should persist audit events to Data Storage for completed workflow`

**Changes**:
```go
// Line 464:
- auditClient, err := ogenclient.NewClientWithResponses(dataStorageServiceURL)
+ auditClient, err := ogenclient.NewClient(dataStorageServiceURL)

// Line 467:
- eventCategory := "workflow"
+ eventCategory := "workflowexecution"

// Line 470-473:
- resp, err := auditClient.QueryAuditEventsWithResponse(ctx, &ogenclient.QueryAuditEventsParams{
-     EventCategory: &eventCategory,
-     CorrelationId: &wfe.Name,
- })
+ resp, err := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
+     EventCategory: ogenclient.NewOptString(eventCategory),
+     CorrelationID: ogenclient.NewOptString(wfe.Name),
+ })

// Lines 483-492: REMOVE status code checks, simplify:
- GinkgoWriter.Printf("🔍 Response: status=%d, JSON200 nil? %v\n", resp.StatusCode(), resp.JSON200 == nil)
- if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
-     GinkgoWriter.Printf("⚠️ Status not 200 or JSON200 nil. Body length: %d bytes\n", len(resp.Body))
-     if len(resp.Body) > 0 && len(resp.Body) < 500 {
-         GinkgoWriter.Printf("⚠️ Response body: %s\n", string(resp.Body))
-     }
-     return 0
- }

// Lines 493-496:
- auditEvents = resp.JSON200.Data
+ auditEvents = resp.Data
- if resp.JSON200.Pagination.IsSet() && resp.JSON200.Pagination.Value.Total.IsSet() {
-     GinkgoWriter.Printf("📊 Total in DB: %d\n", resp.JSON200.Pagination.Value.Total.Value)
+ if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
+     GinkgoWriter.Printf("📊 Total in DB: %d\n", resp.Pagination.Value.Total.Value)
```

### **Location 2**: Lines 583-608 (workflow.failed test)
**Function**: `should emit workflow.failed audit event with complete failure details`

**Similar changes** as Location 1

### **Location 3**: Lines 703-732 (payload fields test)
**Function**: `should persist audit events with correct WorkflowExecutionAuditPayload fields`

**Similar changes** as Location 1

### **Location 4**: Line 649 (EventData access)
```go
- eventData, ok := failedEvent.EventData.(ogenclient.WorkflowExecutionAuditPayload)
+ eventData, ok := failedEvent.EventData.GetWorkflowExecutionAuditPayload()
```

---

## 🎯 **REFERENCE: Working Pattern (From Integration Tests)**

```go
// test/integration/workflowexecution/reconciler_test.go:399-431

auditClient, err := ogenclient.NewClient(dataStorageBaseURL)
Expect(err).ToNot(HaveOccurred(), "Failed to create ogen audit client")

eventCategory := "workflowexecution" // Per ADR-034 v1.5
correlationID := wfe.Name

Eventually(func() bool {
    resp, err := auditClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
        EventCategory: ogenclient.NewOptString(eventCategory),
        CorrelationID: ogenclient.NewOptString(correlationID),
    })
    if err != nil {
        return false
    }

    // Find specific event
    for i := range resp.Data {
        if resp.Data[i].EventType == "workflowexecution.workflow.started" {
            startedEvent = &resp.Data[i]
            return true
        }
    }
    return false
}, 30*time.Second, 500*time.Millisecond).Should(BeTrue())

// Access event data
eventData, ok := startedEvent.EventData.GetWorkflowExecutionAuditPayload()
Expect(ok).To(BeTrue())
```

---

## ⏱️ **ESTIMATED EFFORT**

- **Time**: ~30-45 minutes
- **Complexity**: LOW (mechanical find-and-replace)
- **Risk**: LOW (pattern is well-established in integration tests)
- **Testing**: Run `make test-e2e-workflowexecution` to verify

---

## ✅ **WHEN COMPLETE**

After updating the E2E test file:

1. ✅ Run E2E tests: `make test-e2e-workflowexecution`
2. ✅ Verify all 12 tests pass (currently 9/12 passing with infrastructure fix)
3. ✅ Update `WE_FINAL_STATUS_JAN09_2026.md` with 100% pass rate
4. ✅ Mark WorkflowExecution E2E tests as fully production-ready

---

## 📚 **RELATED DOCUMENTATION**

- **Working Example**: `test/integration/workflowexecution/reconciler_test.go`
- **API Reference**: `pkg/datastorage/ogen-client/` (generated by ogen)
- **ADR-034 v1.5**: Event type prefix changes (`workflowexecution.*`)
- **WE_FINAL_STATUS_JAN09_2026.md**: Comprehensive status document

---

**Prepared by**: WE Team
**Date**: 2026-01-09
**Status**: Infrastructure ✅ Fixed | Test Code 🔧 Needs Migration
**Next Step**: Apply mechanical find-and-replace updates per this document
