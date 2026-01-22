# Gap #8 Webhook Infrastructure Triage - January 12, 2026

## 🎯 **User Request**

> "the webhook is already available in the RO e2e deployment infrastructure. Triage and reassess"

**Context**: User challenged assistant's assumption that webhook infrastructure wasn't ready for implementing operator mutation auditing (`webhook.remediationrequest.timeout_modified`).

---

## ✅ **VALIDATED: Webhook Infrastructure IS Available**

### **What's Already Deployed in RO E2E:**

1. **AuthWebhook Service** (deployed in Phase 4.5)
   - File: `test/infrastructure/remediationorchestrator_e2e_hybrid.go:346-356`
   - Function: `DeployAuthWebhookToCluster()`
   - Status: ✅ Fully operational in RO e2e tests

2. **Existing Webhook Configurations**:
   - ✅ **WorkflowExecution**: `workflowexecution.mutate.kubernaut.ai` (status updates)
   - ✅ **RemediationApprovalRequest**: `remediationapprovalrequest.mutate.kubernaut.ai` (status updates)
   - ✅ **NotificationRequest**: `notificationrequest.validate.kubernaut.ai` (delete validation)

3. **Webhook Infrastructure**:
   - ✅ TLS certificate generation (`generateWebhookCertsOnly()`)
   - ✅ CA bundle patching (`patchWebhookConfigurations()`)
   - ✅ Service account + RBAC
   - ✅ Coverage instrumentation enabled
   - ✅ DataStorage audit client integration

---

## 🔍 **What's Missing: RemediationRequest Webhook**

### **Comparison with Existing Webhooks:**

| Component | WorkflowExecution | RemediationApprovalRequest | **RemediationRequest (Gap #8)** |
|---|---|---|---|
| **Handler File** | `pkg/authwebhook/workflowexecution_handler.go` | `pkg/authwebhook/remediationapprovalrequest_handler.go` | ❌ **NOT IMPLEMENTED** |
| **Webhook Config** | `test/e2e/authwebhook/manifests/authwebhook-deployment.yaml:157-174` | Lines 176-193 | ❌ **NOT IN MANIFEST** |
| **RBAC Permission** | `workflowexecutions/status` (line 25) | `remediationapprovalrequests/status` (line 25) | ❌ **NOT IN RBAC** |
| **Registration** | `cmd/authwebhook/main.go` | `cmd/authwebhook/main.go` | ❌ **NOT REGISTERED** |
| **Audit Event** | `workflowexecution.block.cleared` | `remediation.approval.approved` | ✅ **OpenAPI READY** (`webhook.remediationrequest.timeout_modified`) |

---

## 📋 **Implementation Plan: Add RemediationRequest Webhook**

### **Phase 3.1: Webhook Handler (TDD GREEN)**
**File**: `pkg/authwebhook/remediationrequest_handler.go` (new)

**Pattern**: Follow `remediationapprovalrequest_handler.go` pattern

```go
// RemediationRequestStatusHandler handles status updates to RemediationRequest CRDs
// Per Gap #8: Captures operator modifications to TimeoutConfig for SOC2 compliance
type RemediationRequestStatusHandler struct {
    decoder     *admission.Decoder
    auditClient *dsgen.Client
    log         logr.Logger
}

// Handle intercepts RemediationRequest status updates
func (h *RemediationRequestStatusHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    // 1. Decode old and new RemediationRequest objects
    // 2. Compare status.TimeoutConfig for changes
    // 3. If changed, emit webhook.remediationrequest.timeout_modified
    // 4. Populate status.LastModifiedBy and status.LastModifiedAt
    // 5. Return patched response
}
```

**Business Logic**:
- ✅ Detect `TimeoutConfig` mutations (old vs new comparison)
- ✅ Emit `webhook.remediationrequest.timeout_modified` audit event
- ✅ Populate `LastModifiedBy` from `req.UserInfo.Username`
- ✅ Populate `LastModifiedAt` with current timestamp
- ✅ Return admission response with JSON patch

**Estimated Time**: 1-2 hours

---

### **Phase 3.2: Webhook Configuration**
**File**: `test/e2e/authwebhook/manifests/authwebhook-deployment.yaml`

**Changes**:

1. **Add to MutatingWebhookConfiguration** (after line 193):
```yaml
  - name: remediationrequest.mutate.kubernaut.ai
    admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: authwebhook
        namespace: ${WEBHOOK_NAMESPACE}
        path: /mutate-remediationrequest
      caBundle: ""  # Populated by infrastructure
    failurePolicy: Fail
    matchPolicy: Equivalent
    rules:
      - apiGroups: ["kubernaut.ai"]
        apiVersions: ["v1alpha1"]
        operations: ["UPDATE"]
        resources: ["remediationrequests/status"]
        scope: "Namespaced"
    sideEffects: None
    timeoutSeconds: 10
```

2. **Update RBAC ClusterRole** (line 21):
```yaml
rules:
  # Read CRDs for webhook validation/mutation
  - apiGroups: ["kubernaut.ai"]
    resources: ["workflowexecutions", "remediationapprovalrequests", "notificationrequests", "remediationrequests"]  # ADD remediationrequests
    verbs: ["get", "list", "watch"]
  # Update CRD status for webhook mutation
  - apiGroups: ["kubernaut.ai"]
    resources: ["workflowexecutions/status", "remediationapprovalrequests/status", "remediationrequests/status"]  # ADD remediationrequests/status
    verbs: ["update", "patch"]
```

3. **Update `patchWebhookConfigurations()`** in `test/infrastructure/authwebhook_e2e.go:493`:
```go
// Patch each webhook in MutatingWebhookConfiguration
webhookNames := []string{
    "workflowexecution.mutate.kubernaut.ai",
    "remediationapprovalrequest.mutate.kubernaut.ai",
    "remediationrequest.mutate.kubernaut.ai",  // ADD THIS
}
```

**Estimated Time**: 30 minutes

---

### **Phase 3.3: Webhook Registration**
**File**: `cmd/authwebhook/main.go`

**Changes**:
```go
// Register RemediationRequest status handler
rrStatusHandler := &webhooks.RemediationRequestStatusHandler{
    Decoder:     decoder,
    AuditClient: dsClient,
    Log:         ctrl.Log.WithName("remediationrequest-status-handler"),
}
mgr.GetWebhookServer().Register("/mutate-remediationrequest", &webhook.Admission{Handler: rrStatusHandler})
setupLog.Info("Registered RemediationRequest status webhook", "path", "/mutate-remediationrequest")
```

**Estimated Time**: 15 minutes

---

### **Phase 3.4: Integration Test (Re-enable Scenario 2)**
**File**: `test/integration/remediationorchestrator/gap8_timeout_config_audit_test.go`

**Changes**:
- ✅ Scenario 2 is already written (currently `Pending`)
- ✅ Just remove `Pending` status
- ✅ Test validates:
  - RO initializes `status.TimeoutConfig` with defaults
  - Operator modifies `TimeoutConfig` via `kubectl edit`
  - Webhook emits `webhook.remediationrequest.timeout_modified` event
  - Event captures `old_timeout_config`, `new_timeout_config`, `modified_by`, `modified_at`

**Estimated Time**: 15 minutes (test is already written)

---

### **Phase 3.5: End-to-End Validation**
**Commands**:
```bash
# Run Gap #8 complete test suite (all 3 scenarios)
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
ginkgo -v --focus="Gap #8" ./test/integration/remediationorchestrator/...

# Expected output:
# ✅ Scenario 1: Default TimeoutConfig capture - PASSED
# ✅ Scenario 2: Operator modifies TimeoutConfig - PASSED (webhook validates)
# ✅ Scenario 3: Event timing validation - PASSED
```

**Estimated Time**: 10 minutes

---

## 📊 **Total Effort Estimate**

| Phase | Task | Time |
|---|---|---|
| 3.1 | Webhook Handler Implementation | 1-2 hours |
| 3.2 | Webhook Configuration Updates | 30 min |
| 3.3 | Webhook Registration | 15 min |
| 3.4 | Re-enable Integration Test | 15 min |
| 3.5 | E2E Validation | 10 min |
| **TOTAL** | **Complete Gap #8 + Webhook** | **~2.5-3 hours** |

---

## ✅ **Corrected Assessment**

### **Assistant's Original Error:**
> "❌ Webhook implementation (operator mutations) can be completed in a future sprint when you're ready to add the admission webhook infrastructure."

### **Corrected Reality:**
> "✅ **Webhook infrastructure is FULLY OPERATIONAL** in RO e2e tests. Only the RemediationRequest handler needs to be added (2-3 hours of work)."

---

## 🎯 **Next Steps**

**User has approved continuation**. Proceeding with:
1. ✅ Phase 3.1: Create `remediationrequest_handler.go` following TDD
2. ✅ Phase 3.2: Update webhook manifest + RBAC
3. ✅ Phase 3.3: Register in `cmd/authwebhook/main.go`
4. ✅ Phase 3.4: Re-enable Scenario 2 integration test
5. ✅ Phase 3.5: Run complete Gap #8 test suite

**Target**: Complete Gap #8 implementation including operator mutation auditing.

---

## 📚 **References**

- **Existing Handlers**: `pkg/authwebhook/remediationapprovalrequest_handler.go` (best pattern to follow)
- **Webhook Deployment**: `test/infrastructure/remediationorchestrator_e2e_hybrid.go:346-356`
- **Webhook Manifest**: `test/e2e/authwebhook/manifests/authwebhook-deployment.yaml`
- **Test Plan**: `docs/handoff/GAP8_COMPLETE_IMPLEMENTATION_PLAN_JAN12.md`
- **OpenAPI Schema**: `api/openapi/data-storage-v1.yaml` (already includes `RemediationRequestWebhookAuditPayload`)

**Priority**: HIGH - Gap #8 is the last remaining SOC2 compliance gap (BR-AUDIT-005 v2.0).
