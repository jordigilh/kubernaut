# TimeoutConfig Webhook Audit - Extension Triage

**Date**: January 12, 2026
**Context**: Gap #8 + Operator Mutation Auditing
**Priority**: 🟡 **MEDIUM** - Extends Gap #8 with operator attribution
**Status**: ⏸️ **AWAITING USER DECISION**

---

## 🎯 **Executive Summary**

**Discovery**: Webhook infrastructure exists and is ready for extension
**Effort**: **+2 hours** to add RemediationRequest status mutation webhook
**Benefit**: Complete audit trail for both system initialization AND operator overrides

---

## 🔍 **Current Webhook Infrastructure**

### **Existing Webhooks** (cmd/authwebhook/main.go)

| Webhook | CRD | Operation | Audit Event | Status |
|---------|-----|-----------|-------------|--------|
| `workflowexecution_handler.go` | WorkflowExecution | UPDATE (block clear) | `webhook.workflowexecution.block_cleared` | ✅ Implemented |
| `remediationapprovalrequest_handler.go` | RemediationApprovalRequest | UPDATE (decision) | `webhook.remediationapprovalrequest.decided` | ✅ Implemented |
| `notificationrequest_validator.go` | NotificationRequest | DELETE | `webhook.notificationrequest.deleted` | ✅ Implemented |
| **NEW** | **RemediationRequest** | **UPDATE (timeout config)** | **`webhook.remediationrequest.timeout_modified`** | ❌ **Not Implemented** |

---

### **Webhook Server Configuration**

```go
// cmd/authwebhook/main.go:83-105
auditStore, err := audit.NewBufferedStore(
    dsClient,
    auditConfig,
    "authwebhook",
    ctrl.Log.WithName("audit"),
)

// Lines 127-143: Existing handler registrations
webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})
webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})
webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{Handler: nrHandler})
```

**Analysis**:
- ✅ Audit store infrastructure ready
- ✅ Handler registration pattern established
- ✅ DD-WEBHOOK-003 pattern (Complete audit events)
- ✅ SOC2 CC8.1 compliance framework in place

---

## 📊 **Gap #8 vs Operator Mutation - Two Separate Events**

### **Gap #8: System Initialization** (CURRENT SCOPE)

```yaml
EventType: orchestrator.lifecycle.created
EventCategory: orchestration
Actor: remediationorchestrator-controller (SYSTEM)
When: RO initializes RR.Status.TimeoutConfig on first reconcile
What:
  - System sets defaults (global: 1h, processing: 5m, analyzing: 10m, executing: 30m)
  - OR captures nil if no timeouts configured
Captured By: RO controller audit event (Gap #8)
```

**Implementation**: RO controller reconciliation loop

---

### **Operator Mutation: Status Override** (WEBHOOK EXTENSION)

```yaml
EventType: webhook.remediationrequest.timeout_modified
EventCategory: webhook
Actor: operator@example.com (OPERATOR)
When: Operator edits rr.Status.TimeoutConfig via kubectl
What:
  - Operator overrides timeout (e.g., global: 1h → 2h)
  - Changed fields captured with old/new values
Captured By: NEW RemediationRequest mutating webhook
```

**Implementation**: NEW webhook handler

---

## 🚀 **Proposed Implementation**

### **Phase 1: Gap #8 Only** (CURRENT SCOPE - 2 hours)
- ✅ Emit `orchestrator.lifecycle.created` on RR initialization
- ✅ Capture `status.timeoutConfig` (defaults or nil)
- ✅ Actor: `remediationorchestrator-controller`

**Result**: Audit trail for system initialization

---

### **Phase 2: Webhook Extension** (OPTIONAL - +2 hours)

#### **Step 1: Create RemediationRequest Webhook Handler** (+1 hour)

**File**: `pkg/authwebhook/remediationrequest_handler.go`

```go
package webhooks

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "reflect"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/authwebhook"
    api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// RemediationRequestStatusHandler handles status mutations for RemediationRequest
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// BR-AUDIT-005 Gap #8 Extension: Operator timeout configuration changes
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// This mutating webhook intercepts RemediationRequest status updates and:
// 1. Detects TimeoutConfig changes
// 2. Writes complete audit event (WHO + WHAT + WHEN)
// 3. Populates status.LastModifiedBy and status.LastModifiedAt
type RemediationRequestStatusHandler struct {
    authenticator *authwebhook.Authenticator
    decoder       admission.Decoder
    auditStore    audit.AuditStore
}

// NewRemediationRequestStatusHandler creates a new RemediationRequest status handler
func NewRemediationRequestStatusHandler(auditStore audit.AuditStore) *RemediationRequestStatusHandler {
    return &RemediationRequestStatusHandler{
        authenticator: authwebhook.NewAuthenticator(),
        auditStore:    auditStore,
    }
}

// Handle processes the admission request for RemediationRequest status updates
func (h *RemediationRequestStatusHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    rr := &remediationv1.RemediationRequest{}
    oldRR := &remediationv1.RemediationRequest{}

    // Decode current and old objects
    if err := json.Unmarshal(req.Object.Raw, rr); err != nil {
        return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode RemediationRequest: %w", err))
    }
    if err := json.Unmarshal(req.OldObject.Raw, oldRR); err != nil {
        return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode old RemediationRequest: %w", err))
    }

    // Detect TimeoutConfig changes
    timeoutChanged := !reflect.DeepEqual(rr.Status.TimeoutConfig, oldRR.Status.TimeoutConfig)

    if !timeoutChanged {
        // No timeout changes - allow without modification
        return admission.Allowed("no timeout config changes")
    }

    // Extract authenticated user
    authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    if err != nil {
        return admission.Denied(fmt.Sprintf("authentication required: %v", err))
    }

    // Build audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
    auditEvent := audit.NewAuditEventRequest()
    audit.SetEventType(auditEvent, "webhook.remediationrequest.timeout_modified")
    audit.SetEventCategory(auditEvent, "webhook") // Per ADR-034 v1.4
    audit.SetEventAction(auditEvent, "status_updated")
    audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
    audit.SetActor(auditEvent, "user", authCtx.Username)
    audit.SetResource(auditEvent, "RemediationRequest", rr.Name)
    audit.SetCorrelationID(auditEvent, rr.Name)
    audit.SetNamespace(auditEvent, rr.Namespace)

    // Build event data payload with changes
    payload := api.RemediationRequestTimeoutModifiedPayload{
        RrName:     rr.Name,
        Namespace:  rr.Namespace,
        ModifiedBy: authCtx.Username,
    }

    // Capture old and new TimeoutConfig
    if oldRR.Status.TimeoutConfig != nil {
        oldTimeout := api.TimeoutConfigPayload{}
        if oldRR.Status.TimeoutConfig.Global != nil {
            oldTimeout.Global.SetTo(oldRR.Status.TimeoutConfig.Global.Duration.String())
        }
        if oldRR.Status.TimeoutConfig.Processing != nil {
            oldTimeout.Processing.SetTo(oldRR.Status.TimeoutConfig.Processing.Duration.String())
        }
        if oldRR.Status.TimeoutConfig.Analyzing != nil {
            oldTimeout.Analyzing.SetTo(oldRR.Status.TimeoutConfig.Analyzing.Duration.String())
        }
        if oldRR.Status.TimeoutConfig.Executing != nil {
            oldTimeout.Executing.SetTo(oldRR.Status.TimeoutConfig.Executing.Duration.String())
        }
        payload.OldTimeoutConfig.SetTo(oldTimeout)
    }

    if rr.Status.TimeoutConfig != nil {
        newTimeout := api.TimeoutConfigPayload{}
        if rr.Status.TimeoutConfig.Global != nil {
            newTimeout.Global.SetTo(rr.Status.TimeoutConfig.Global.Duration.String())
        }
        if rr.Status.TimeoutConfig.Processing != nil {
            newTimeout.Processing.SetTo(rr.Status.TimeoutConfig.Processing.Duration.String())
        }
        if rr.Status.TimeoutConfig.Analyzing != nil {
            newTimeout.Analyzing.SetTo(rr.Status.TimeoutConfig.Analyzing.Duration.String())
        }
        if rr.Status.TimeoutConfig.Executing != nil {
            newTimeout.Executing.SetTo(rr.Status.TimeoutConfig.Executing.Duration.String())
        }
        payload.NewTimeoutConfig.SetTo(newTimeout)
    }

    auditEvent.EventData = api.NewAuditEventRequestEventDataRemediationRequestTimeoutModifiedAuditEventRequestEventData(payload)

    // Write audit event (non-blocking)
    if err := h.auditStore.Store(ctx, auditEvent); err != nil {
        // Log error but don't block operation (operational resilience)
        // Per DD-WEBHOOK-003: Audit failure should not block business operations
        setupLog.Error(err, "failed to store audit event", "event_type", "webhook.remediationrequest.timeout_modified")
    }

    // Populate status fields (MANDATORY for UPDATE operations)
    now := metav1.Now()
    rr.Status.LastModifiedBy = authCtx.Username
    rr.Status.LastModifiedAt = &now

    // Return patched object
    marshaledRR, err := json.Marshal(rr)
    if err != nil {
        return admission.Errored(http.StatusInternalServerError, err)
    }

    return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRR)
}

// InjectDecoder injects the decoder into the handler
func (h *RemediationRequestStatusHandler) InjectDecoder(d admission.Decoder) error {
    h.decoder = d
    return nil
}
```

---

#### **Step 2: Register Webhook Handler** (+15 minutes)

**File**: `cmd/authwebhook/main.go`

```go
// After line 143, add:

// Register RemediationRequest status mutation handler (Gap #8 Extension)
// Captures operator timeout configuration changes
rrHandler := webhooks.NewRemediationRequestStatusHandler(auditStore)
if err := rrHandler.InjectDecoder(decoder); err != nil {
    setupLog.Error(err, "failed to inject decoder into RemediationRequest handler")
    os.Exit(1)
}
webhookServer.Register("/mutate-remediationrequest-status", &webhook.Admission{Handler: rrHandler})
setupLog.Info("Registered RemediationRequest status webhook handler with audit store")
```

---

#### **Step 3: Add Status Fields to RemediationRequest CRD** (+15 minutes)

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

```go
// In RemediationRequestStatus struct (after line 430):

// ========================================
// OPERATOR MUTATION TRACKING (SOC2 CC8.1)
// ========================================

// LastModifiedBy tracks the last operator who modified this RR's status.
// Populated by RemediationRequest mutating webhook.
// Reference: BR-AUTH-001 (SOC2 CC8.1 Operator Attribution)
// +optional
LastModifiedBy string `json:"lastModifiedBy,omitempty"`

// LastModifiedAt tracks when the last status modification occurred.
// Populated by RemediationRequest mutating webhook.
// +optional
LastModifiedAt *metav1.Time `json:"lastModifiedAt,omitempty"`
```

---

#### **Step 4: Add OpenAPI Audit Event Type** (+15 minutes)

**File**: `pkg/datastorage/api/openapi.yaml`

```yaml
# Add new event type payload
RemediationRequestTimeoutModifiedPayload:
  type: object
  required:
    - rr_name
    - namespace
    - modified_by
  properties:
    rr_name:
      type: string
      description: RemediationRequest name
    namespace:
      type: string
      description: Namespace
    modified_by:
      type: string
      description: Operator who modified timeout config
    old_timeout_config:
      $ref: '#/components/schemas/TimeoutConfigPayload'
      description: Previous timeout configuration
    new_timeout_config:
      $ref: '#/components/schemas/TimeoutConfigPayload'
      description: New timeout configuration

# Add to AuditEventRequestEventData discriminator
AuditEventRequestEventData:
  discriminator:
    mapping:
      # ... existing mappings ...
      remediationrequest.timeout.modified: '#/components/schemas/RemediationRequestTimeoutModifiedPayload'
```

---

#### **Step 5: Create Webhook Deployment Manifest** (+15 minutes)

**File**: `deploy/webhooks/03-mutatingwebhook.yaml`

```yaml
# Add RemediationRequest webhook configuration
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kubernaut-remediationrequest-webhook
webhooks:
  - name: remediationrequest-status.kubernaut.ai
    clientConfig:
      service:
        name: kubernaut-webhook-service
        namespace: kubernaut-system
        path: /mutate-remediationrequest-status
      caBundle: ${CA_BUNDLE}
    rules:
      - operations: ["UPDATE"]
        apiGroups: ["kubernaut.ai"]
        apiVersions: ["v1alpha1"]
        resources: ["remediationrequests/status"]
    admissionReviewVersions: ["v1"]
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
```

---

## ⏱️ **Effort Summary**

| Phase | Task | Effort |
|-------|------|--------|
| **Phase 1** (Gap #8 Only) | | |
| 1.1 | TimeoutConfig migration to status | 8 hours |
| 1.2 | Gap #8 implementation (system event) | 2 hours |
| **Phase 1 TOTAL** | | **10 hours (~1.25 days)** |
| | | |
| **Phase 2** (Webhook Extension - OPTIONAL) | | |
| 2.1 | Create RemediationRequest webhook handler | 1 hour |
| 2.2 | Register webhook in cmd/authwebhook | 15 min |
| 2.3 | Add status fields to CRD | 15 min |
| 2.4 | Add OpenAPI audit event type | 15 min |
| 2.5 | Create webhook deployment manifest | 15 min |
| **Phase 2 TOTAL** | | **+2 hours** |
| | | |
| **GRAND TOTAL** | | **12 hours (~1.5 days)** |

---

## 🎯 **Decision Matrix**

### **Option A: Gap #8 Only** (10 hours)
- ✅ Audit system initialization of timeouts
- ✅ Capture defaults set by RO
- ✅ Actor: `remediationorchestrator-controller`
- ❌ NO operator mutation auditing
- ❌ NO status field tracking

**Use Case**: "When was this RR created and what defaults were set?"

---

### **Option B: Gap #8 + Webhook Extension** (12 hours)
- ✅ Audit system initialization of timeouts
- ✅ Audit operator timeout overrides
- ✅ Two separate events (system + operator)
- ✅ Status fields: `LastModifiedBy`, `LastModifiedAt`
- ✅ Complete SOC2 CC8.1 compliance

**Use Case**: "Who changed this timeout to 2h and when?"

---

## 📊 **Recommendation**

### **Phased Approach** ✅ RECOMMENDED

**Sprint 1**: **Option A** (Gap #8 Only)
- Immediate value: System initialization auditing
- Lower risk: No new webhook infrastructure
- Faster delivery: 10 hours vs 12 hours

**Sprint 2**: **Option B Extension** (Add Webhook)
- Based on user feedback: "Do we need operator attribution?"
- Standalone value: Webhook can be added later
- Lower pressure: Not blocking Gap #8 closure

---

## ✅ **Success Criteria**

### **Phase 1 (Gap #8)**:
- [ ] `status.timeoutConfig` field exists in RemediationRequest CRD
- [ ] RO initializes defaults on first reconcile
- [ ] `orchestrator.lifecycle.created` audit event emitted
- [ ] TimeoutConfig captured in audit payload

### **Phase 2 (Webhook Extension)**:
- [ ] RemediationRequest webhook handler implemented
- [ ] `webhook.remediationrequest.timeout_modified` audit event emitted
- [ ] Status fields `LastModifiedBy`, `LastModifiedAt` populated
- [ ] Webhook deployment manifest created
- [ ] Integration tests pass

---

## 🚨 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Webhook adds complexity** | Medium | Medium | Phase 2 is optional, can defer |
| **Status field conflicts** | Low | Medium | RO owns status, webhook only adds tracking fields |
| **Audit event schema drift** | Low | High | Use OpenAPI discriminator pattern (established) |
| **Performance impact** | Low | Low | Webhook latency <10ms (per existing webhooks) |

---

## 🔗 **Related Work**

- **Gap #8 Core**: `TIMEOUTCONFIG_MIGRATION_TO_STATUS_TRIAGE_JAN12.md`
- **SOC2 Operator Attribution**: `TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md`
- **Webhook Pattern**: `DD-WEBHOOK-003` (Webhook-Complete Audit Pattern)
- **Existing Handlers**: `pkg/authwebhook/remediationapprovalrequest_handler.go` (reference implementation)

---

## 📝 **Next Steps**

**Immediate**:
1. **User decision**: Option A (Gap #8 only) or Option B (Gap #8 + Webhook)?
2. **If Option A**: Proceed with TimeoutConfig migration (8 hours) + Gap #8 (2 hours)
3. **If Option B**: Implement both phases sequentially (12 hours total)

**Future**:
- Monitor operator feedback: "Do we need timeout mutation auditing?"
- If yes: Implement Phase 2 webhook extension (2 hours)

---

**Status**: ⏸️ **AWAITING USER DECISION**
**Question**: Should we implement Gap #8 only (Option A) or Gap #8 + Webhook (Option B)?
