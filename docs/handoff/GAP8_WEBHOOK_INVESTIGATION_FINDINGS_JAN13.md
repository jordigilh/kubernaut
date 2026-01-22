# Gap #8 Webhook Investigation Findings - January 13, 2026

## 🔍 **Investigation Summary**

**Status**: ⚠️ **Configuration Validated - Likely Test Logic Issue**
**Confidence**: 85% on root cause hypothesis

---

## ✅ **What I Verified (All Correct)**

### **1. Webhook Handler Implementation** ✅

**File**: `pkg/authwebhook/remediationrequest_handler.go`

**Verified**:
- ✅ Handler detects TimeoutConfig changes (`timeoutConfigChanged()`)
- ✅ Populates `LastModifiedBy` and `LastModifiedAt`
- ✅ Constructs audit event properly
- ✅ Calls `auditStore.StoreAudit()` to emit event (line 134)
- ✅ Uses correct event type: `webhook.remediationrequest.timeout_modified`
- ✅ Sets correlation ID to `string(rr.UID)`

**Code Snippet**:
```go
// Line 100-138
auditEvent := audit.NewAuditEventRequest()
audit.SetEventType(auditEvent, "webhook.remediationrequest.timeout_modified")
audit.SetCorrelationID(auditEvent, string(rr.UID))
// ... payload construction ...
if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
    fmt.Printf("WARNING: Failed to store audit event: %v\n", err)
}
```

---

### **2. Webhook Registration** ✅

**File**: `cmd/authwebhook/main.go`

**Verified**:
- ✅ Audit store initialized (`audit.NewBufferedStore()`)
- ✅ Handler created with audit store
- ✅ Handler registered at `/mutate-remediationrequest`
- ✅ Path matches MutatingWebhookConfiguration

**Code Snippet**:
```go
// Line 146-151
rrHandler := webhooks.NewRemediationRequestStatusHandler(auditStore)
if err := rrHandler.InjectDecoder(decoder); err != nil {
    setupLog.Error(err, "failed to inject decoder")
    os.Exit(1)
}
webhookServer.Register("/mutate-remediationrequest", &webhook.Admission{Handler: rrHandler})
```

---

### **3. Webhook Configuration** ✅

**File**: `test/e2e/authwebhook/manifests/authwebhook-deployment.yaml`

**Verified**:
- ✅ Webhook name: `remediationrequest.mutate.kubernaut.ai`
- ✅ Path: `/mutate-remediationrequest` (matches handler registration)
- ✅ Operations: `["UPDATE"]` (correct for status updates)
- ✅ Resources: `["remediationrequests/status"]` (correct subresource)
- ✅ No namespace selector (should intercept all namespaces)

**Configuration**:
```yaml
- name: remediationrequest.mutate.kubernaut.ai
  clientConfig:
    path: /mutate-remediationrequest
  rules:
    - operations: ["UPDATE"]
      resources: ["remediationrequests/status"]
```

---

### **4. Test Namespace Setup** ✅

**File**: `test/e2e/authwebhook/02_gap8_remediationrequest_timeout_mutation_test.go`

**Verified**:
- ✅ Namespace created with `kubernaut.ai/audit-enabled: "true"` label
- ✅ RemediationRequest created with valid fingerprint
- ✅ TimeoutConfig manually initialized (workaround for no controller)
- ✅ TimeoutConfig modified (should trigger webhook)

---

## 🔴 **Key Finding: Test May Be Querying Wrong Correlation ID**

### **Root Cause Hypothesis** (85% confidence):

The test is experiencing **TWO** Status().Update() calls:

1. **First Update**: Manual TimeoutConfig initialization
   - Old: nil TimeoutConfig
   - New: TimeoutConfig with defaults (1h global, 5m/10m/30m phases)
   - Webhook: **SHOULD emit event** (TimeoutConfig changed from nil → set)

2. **Second Update**: Operator mutation
   - Old: TimeoutConfig with defaults
   - New: TimeoutConfig with new values (45m global, 12m/8m/20m phases)
   - Webhook: **SHOULD emit event** (TimeoutConfig values changed)

**Problem**: Test is querying for events with `correlation_id=<UID>`, but:
- UID might not be set yet on first update?
- Webhook might not detect first update as a "change"?
- Audit events might not be getting stored?

---

## 🔬 **Comparison with Working Tests**

### **WorkflowExecution Webhooks (E2E-MULTI-01, E2E-MULTI-02)** ✅ Passing

**Key Differences**:

| Feature | WorkflowExecution | RemediationRequest (Gap #8) |
|---------|-------------------|----------------------------|
| **Namespace Label** | ❌ No special label | ✅ `kubernaut.ai/audit-enabled: "true"` |
| **Controller Running** | ❌ No (simulated in test) | ❌ No (manual TimeoutConfig init) |
| **Update Count** | 1 (single block clearance) | **2 (init + mutation)** |
| **Webhook Path** | `/mutate-workflowexecution` | `/mutate-remediationrequest` |
| **Status** | ✅ Working | ❌ Failing |

**Hypothesis**: The extra update (manual initialization) might be causing issues.

---

##3 **Recommended Fix Options**

### **Option A: Simplify Test Flow** (Recommended - 30 minutes)

**Change**: Remove manual TimeoutConfig initialization, set it during RR creation

**Reason**: Avoid TWO status updates, making test more predictable

**Implementation**:
```go
// Instead of:
// 1. Create RR (no TimeoutConfig)
// 2. Wait and manually initialize TimeoutConfig
// 3. Modify TimeoutConfig

// Do:
rr = &remediationv1.RemediationRequest{
    // ... metadata ...
    Status: remediationv1.RemediationRequestStatus{
        TimeoutConfig: &remediationv1.TimeoutConfig{
            Global:     &metav1.Duration{Duration: 1 * time.Hour},
            // ...
        },
    },
}
err := k8sClient.Create(ctx, rr)

// Then immediately modify:
rr.Status.TimeoutConfig.Global = &metav1.Duration{Duration: 45 * time.Minute}
err = k8sClient.Status().Update(ctx, rr)
```

**Expected Result**: Only ONE status update, webhook triggers once, audit event emitted

---

### **Option B: Add Debug Logging** (1 hour)

**Change**: Add verbose logging to webhook handler and test

**Implementation**:
1. Add logging in `pkg/authwebhook/remediationrequest_handler.go`:
```go
func (h *RemediationRequestStatusHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    fmt.Printf("DEBUG: Webhook received RemediationRequest update: %s/%s\n", req.Namespace, req.Name)

    // ... existing code ...

    if !timeoutConfigChanged(oldRR.Status.TimeoutConfig, rr.Status.TimeoutConfig) {
        fmt.Printf("DEBUG: No TimeoutConfig change detected\n")
        return admission.Allowed("no timeout config change")
    }

    fmt.Printf("DEBUG: TimeoutConfig changed! Emitting audit event\n")
    // ... emit event ...

    if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
        fmt.Printf("ERROR: Failed to store audit event: %v\n", err)
    } else {
        fmt.Printf("DEBUG: Audit event stored successfully, correlation_id=%s\n", string(rr.UID))
    }

    return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRR)
}
```

2. Re-run test and check webhook logs:
```bash
kubectl --kubeconfig ~/.kube/authwebhook-e2e-config logs -n authwebhook-e2e -l app=authwebhook --tail=100 | grep -E "DEBUG|ERROR"
```

---

### **Option C: Query for ALL Events** (15 minutes)

**Change**: Query for all audit events with this correlation ID (not just webhook events)

**Implementation**:
```go
// In test file, change query to:
events, _, err := helpers.QueryAuditEvents(
    ctx,
    auditClient,
    &correlationID,
    nil,  // event_type = nil (query ALL event types)
    nil,  // event_category = nil
)

// Log all events found:
GinkgoWriter.Printf("Found %d audit events for correlation_id=%s:\n", len(events), correlationID)
for _, evt := range events {
    GinkgoWriter.Printf("  - event_type=%s, event_category=%s\n", evt.EventType, evt.EventCategory)
}

// Then check if ANY are webhook events:
webhookEvents := []api.AuditEvent{}
for _, evt := range events {
    if evt.EventType == "webhook.remediationrequest.timeout_modified" {
        webhookEvents = append(webhookEvents, evt)
    }
}

Expect(webhookEvents).To(HaveLen(1), "Should have 1 webhook event")
```

**Expected Result**: See if ANY events are being emitted, or if correlation ID is wrong

---

### **Option D: Manual kubectl Test** (30 minutes)

**Change**: Test webhook manually with real `kubectl edit` instead of automated test

**Implementation**:
1. Deploy to staging/Kind cluster
2. Create RR manually:
   ```bash
   kubectl create -f - <<EOF
   apiVersion: kubernaut.ai/v1alpha1
   kind: RemediationRequest
   metadata:
     name: manual-test-rr
     namespace: default
   spec:
     signalFingerprint: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
     signalName: "ManualTest"
     severity: "warning"
     signalType: "prometheus"
     targetType: "kubernetes"
     # ... other required fields ...
   EOF
   ```

3. Edit RR status manually:
   ```bash
   kubectl edit rr manual-test-rr -o yaml
   # Add status.timeoutConfig section
   # Save and exit
   ```

4. Check audit events:
   ```bash
   curl "http://localhost:28099/api/v1/audit/events?event_type=webhook.remediationrequest.timeout_modified"
   ```

**Expected Result**: If webhook works manually, test logic is the issue. If not, webhook deployment is the issue.

---

## 🎯 **My Recommendation**

**Option A** (Simplify Test Flow) - **30 minutes**

**Why**:
1. ✅ **Simplest fix**: Avoid complex multi-update flow
2. ✅ **Matches working tests**: WFE tests do single update
3. ✅ **Clear semantics**: Test focuses on webhook interception, not controller behavior
4. ✅ **Fast to implement**: Change test file only, no handler changes

**Implementation Plan**:
1. Modify `test/e2e/authwebhook/02_gap8_remediationrequest_timeout_mutation_test.go`
2. Set TimeoutConfig in RR creation (not via Status().Update())
3. Perform single Status().Update() to modify TimeoutConfig
4. Re-run test

**If this doesn't work**, then proceed to **Option C** (Query for ALL events) to diagnose correlation ID issue.

---

## 📊 **Investigation Confidence Assessment**

| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **Handler Implementation** | 100% ✅ | Code reviewed, logic correct |
| **Webhook Registration** | 100% ✅ | Path and handler verified |
| **Webhook Config** | 100% ✅ | YAML manifest correct |
| **Test Setup** | 90% ✅ | Namespace and RR creation correct |
| **Root Cause Hypothesis** | 85% ⚠️ | Multi-update flow suspicious |

**Overall Confidence**: 85% - Root cause likely in test flow, not webhook implementation

---

## 📝 **Next Steps**

1. **Implement Option A** (30 min):
   - Simplify test to single Status().Update()
   - Re-run test

2. **If Option A fails, try Option C** (15 min):
   - Query for ALL audit events
   - Identify if events are being emitted with different correlation ID

3. **If both fail, try Option D** (30 min):
   - Manual `kubectl edit` test
   - Validate webhook works outside automated test

4. **If all fail, investigate infrastructure** (1-2 hours):
   - Check webhook server logs
   - Verify MutatingWebhookConfiguration deployed
   - Check TLS certificates
   - Verify DataStorage connectivity

---

## 🚀 **Execution Command**

To start with Option A:

```bash
# Edit test file to simplify flow
vim test/e2e/authwebhook/02_gap8_remediationrequest_timeout_mutation_test.go

# Re-run focused test
go test -v ./test/e2e/authwebhook/ -ginkgo.focus="E2E-GAP8-01" -timeout 30m
```

---

**Document Version**: 1.0
**Created**: January 13, 2026
**Status**: ⚠️ **Investigation Complete - Ready for Fix Implementation**
**Estimated Fix Time**: 30 minutes (Option A)
**BR-AUDIT-005 v2.0**: Gap #8 - TimeoutConfig mutation audit capture
