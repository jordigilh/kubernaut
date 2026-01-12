# Gap #8 Complete Implementation Plan - TimeoutConfig + Webhook

**Date**: January 12, 2026
**Decision**: **Option B** - Gap #8 + Webhook Extension
**Total Effort**: 12 hours (~1.5 days)
**Priority**: 🔴 **HIGH** - SOC2 BR-AUDIT-005 Gap #8 Closure

---

## 🎯 **Executive Summary**

**Scope**: Complete implementation of Gap #8 (TimeoutConfig audit) with operator mutation auditing

**Three-Phase Approach**:
1. **Phase 1** (8 hours): Move `TimeoutConfig` from `spec` to `status`
2. **Phase 2** (2 hours): Implement Gap #8 (`orchestrator.lifecycle.created` event)
3. **Phase 3** (2 hours): Add webhook for operator mutations (`webhook.remediationrequest.timeout_modified`)

**Key Events** (Corrected per ADR-034):
- ✅ `orchestrator.lifecycle.created` (RO controller, system initialization)
- ✅ `webhook.remediationrequest.timeout_modified` (Webhook, operator mutation)

---

## 📋 **Phase 1: TimeoutConfig Migration to Status**

**Duration**: 8 hours
**Goal**: Move `TimeoutConfig` from immutable spec to mutable status

### **Step 1.1: Update CRD Schema** (1 hour)

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

**Change 1: Remove from Spec**
```go
// Line 334-338: REMOVE TimeoutConfig from RemediationRequestSpec
type RemediationRequestSpec struct {
    // ... other fields ...

    // ❌ REMOVE THIS:
    // TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}
```

**Change 2: Add to Status**
```go
// After line 430: ADD TimeoutConfig to RemediationRequestStatus
type RemediationRequestStatus struct {
    // ... existing status fields ...

    // ========================================
    // TIMEOUT CONFIGURATION (BR-ORCH-027/028)
    // ========================================

    // TimeoutConfig provides operational timeout overrides for this remediation.
    // OWNER: Remediation Orchestrator (sets defaults on first reconcile)
    // MUTABLE BY: Operators (can adjust mid-remediation via kubectl edit)
    // Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
    // +optional
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`

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
}
```

**Change 3: Regenerate CRD**
```bash
make manifests
make generate
```

---

### **Step 1.2: Add Timeout Initialization** (2 hours)

**File**: `internal/controller/remediationorchestrator/reconciler.go`

**Add new function** (after line 2292):
```go
// initializeTimeoutDefaults initializes status.timeoutConfig with controller defaults.
// Only runs on first reconcile when status.timeoutConfig is nil.
// Per Gap #8: RO owns timeout initialization, operators can override later.
//
// Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
func (r *RemediationOrchestratorReconciler) initializeTimeoutDefaults(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) error {
    // Only initialize if status.timeoutConfig is nil (first reconcile)
    if rr.Status.TimeoutConfig != nil {
        return nil // Already initialized
    }

    // Set defaults from controller config
    rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
        Global:     &metav1.Duration{Duration: r.timeoutConfig.GlobalTimeout},
        Processing: &metav1.Duration{Duration: r.timeoutConfig.ProcessingTimeout},
        Analyzing:  &metav1.Duration{Duration: r.timeoutConfig.AnalyzingTimeout},
        Executing:  &metav1.Duration{Duration: r.timeoutConfig.ExecutingTimeout},
    }

    // Update status
    if err := r.Status().Update(ctx, rr); err != nil {
        return fmt.Errorf("failed to initialize timeout defaults: %w", err)
    }

    r.logger.Info("Initialized timeout defaults in status",
        "name", rr.Name,
        "namespace", rr.Namespace,
        "global", r.timeoutConfig.GlobalTimeout,
        "processing", r.timeoutConfig.ProcessingTimeout,
        "analyzing", r.timeoutConfig.AnalyzingTimeout,
        "executing", r.timeoutConfig.ExecutingTimeout)

    return nil
}
```

**Update Reconcile function** (around line 283-288):
```go
// Initialize timeout defaults on first reconcile
if err := r.initializeTimeoutDefaults(ctx, rr); err != nil {
    return ctrl.Result{}, fmt.Errorf("failed to initialize timeout defaults: %w", err)
}
```

**Update all references** (11 occurrences):
```go
// Line 318, 328, 1956-1957, 1966-1978, 2039, 2271-2292
// Change: rr.Status.TimeoutConfig → rr.Status.TimeoutConfig
```

---

### **Step 1.3: Update Timeout Detector** (30 minutes)

**File**: `pkg/remediationorchestrator/timeout/detector.go`

**Update 8 references** (lines 83-84, 146-158):
```go
// Line 83-84
if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil && rr.Status.TimeoutConfig.Global.Duration > 0 {
    globalTimeout = rr.Status.TimeoutConfig.Global.Duration
}

// Lines 146-158
if rr.Status.TimeoutConfig != nil {
    switch phase {
    case remediationv1.RemediationPhaseProcessing:
        if rr.Status.TimeoutConfig.Processing != nil && rr.Status.TimeoutConfig.Processing.Duration > 0 {
            return rr.Status.TimeoutConfig.Processing.Duration
        }
    case remediationv1.RemediationPhaseAnalyzing:
        if rr.Status.TimeoutConfig.Analyzing != nil && rr.Status.TimeoutConfig.Analyzing.Duration > 0 {
            return rr.Status.TimeoutConfig.Analyzing.Duration
        }
    case remediationv1.RemediationPhaseExecuting:
        if rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration > 0 {
            return rr.Status.TimeoutConfig.Executing.Duration
        }
    }
}
```

---

### **Step 1.4: Update WFE Creator** (15 minutes)

**File**: `pkg/remediationorchestrator/creator/workflowexecution.go`

**Update line 186-188**:
```go
if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration > 0 {
    wfe.Spec.Timeout = &workflowexecutionv1.Timeout{
        Timeout: rr.Status.TimeoutConfig.Executing,
    }
}
```

---

### **Step 1.5: Update Tests** (3 hours)

**Files to Update**:
- `test/unit/remediationorchestrator/timeout_detector_test.go`
- `test/unit/remediationorchestrator/workflowexecution_creator_test.go`
- `test/unit/remediationorchestrator/controller/*.go`
- `test/shared/helpers/remediation.go`
- `test/integration/remediationorchestrator/timeout_integration_test.go`
- `test/integration/remediationorchestrator/audit_errors_integration_test.go`

**Pattern**:
```go
// BEFORE
rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
    Global: &metav1.Duration{Duration: 2 * time.Hour},
}

// AFTER
rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
    Global: &metav1.Duration{Duration: 2 * time.Hour},
}
```

---

### **Step 1.6: Update Documentation** (1 hour)

**Find-replace in docs**:
```bash
grep -r "spec\.timeoutConfig" docs/ --include="*.md" | wc -l
# Expected: ~50 files

# Replace: status.timeoutConfig → status.timeoutConfig
```

**Update BR-ORCH-027/028**: Reflect new location

---

## 📋 **Phase 2: Gap #8 Implementation**

**Duration**: 2 hours
**Goal**: Emit `orchestrator.lifecycle.created` audit event with TimeoutConfig

### **Step 2.1: Update Audit Manager** (1 hour)

**File**: `pkg/remediationorchestrator/audit/manager.go`

**Add constant** (around line 29):
```go
const (
    // ... existing constants ...

    // EventTypeLifecycleCreated is emitted when a new RemediationRequest is created.
    // Per BR-AUDIT-005 Gap #8: Captures initial RR state including TimeoutConfig.
    // ADR-034 v1.2: Uses orchestrator.lifecycle.* naming pattern.
    EventTypeLifecycleCreated = "orchestrator.lifecycle.created" // Gap #8
)
```

**Add new method** (after existing methods):
```go
// BuildRemediationCreatedEvent builds an audit event for a new RemediationRequest creation.
// Per BR-AUDIT-005 Gap #8, this captures the initial state of the RR, including TimeoutConfig.
//
// Event Details:
// - event_type: orchestrator.lifecycle.created (ADR-034 v1.2 naming convention)
// - event_category: orchestration
// - event_action: created
// - actor: remediationorchestrator-controller
//
// Parameters:
// - correlationID: RR name for correlation
// - namespace: RR namespace
// - rrName: RR name
// - timeoutConfig: TimeoutConfig from status (nil if not set)
//
// Returns audit event request ready for emission.
func (m *Manager) BuildRemediationCreatedEvent(
    correlationID string,
    namespace string,
    rrName string,
    timeoutConfig *remediationv1.TimeoutConfig,
) (*ogenclient.AuditEventRequest, error) {
    event := audit.NewAuditEventRequest()
    event.Version = "1.0"
    audit.SetEventType(event, EventTypeLifecycleCreated)
    audit.SetEventCategory(event, CategoryOrchestration)
    audit.SetEventAction(event, "created")
    audit.SetEventOutcome(event, audit.OutcomeSuccess)
    audit.SetActor(event, "service", m.serviceName)
    audit.SetResource(event, "RemediationRequest", rrName)
    audit.SetCorrelationID(event, correlationID)
    audit.SetNamespace(event, namespace)

    payload := api.RemediationOrchestratorAuditPayload{
        EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
        RrName:    rrName,
        Namespace: namespace,
    }

    // Gap #8: Capture TimeoutConfig if present
    if timeoutConfig != nil {
        timeoutConfigPayload := api.TimeoutConfigPayload{}
        if timeoutConfig.Global != nil {
            timeoutConfigPayload.Global.SetTo(timeoutConfig.Global.Duration.String())
        }
        if timeoutConfig.Processing != nil {
            timeoutConfigPayload.Processing.SetTo(timeoutConfig.Processing.Duration.String())
        }
        if timeoutConfig.Analyzing != nil {
            timeoutConfigPayload.Analyzing.SetTo(timeoutConfig.Analyzing.Duration.String())
        }
        if timeoutConfig.Executing != nil {
            timeoutConfigPayload.Executing.SetTo(timeoutConfig.Executing.Duration.String())
        }
        payload.TimeoutConfig.SetTo(timeoutConfigPayload)
    }

    event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCreatedAuditEventRequestEventData(payload)

    return event, nil
}
```

---

### **Step 2.2: Emit Audit Event** (30 minutes)

**File**: `internal/controller/remediationorchestrator/reconciler.go`

**After initializing timeout defaults** (around line 290):
```go
// Emit orchestrator.lifecycle.created audit event (Gap #8)
// Captures initial RR state including timeout configuration
lifecycleEvent, err := r.auditManager.BuildRemediationCreatedEvent(
    rr.Name, // correlationID
    rr.Namespace,
    rr.Name,
    rr.Status.TimeoutConfig, // From status (either defaults or nil)
)
if err != nil {
    r.logger.Error(err, "failed to build lifecycle.created audit event",
        "name", rr.Name,
        "namespace", rr.Namespace)
    // Don't fail reconciliation on audit error
} else {
    if err := r.auditStore.Store(ctx, lifecycleEvent); err != nil {
        r.logger.Error(err, "failed to store lifecycle.created audit event",
            "name", rr.Name,
            "namespace", rr.Namespace)
        // Don't fail reconciliation on audit error
    }
}
```

---

### **Step 2.3: Update OpenAPI Schema** (30 minutes)

**File**: `pkg/datastorage/api/openapi.yaml`

**Add new event type** (under `RemediationOrchestratorAuditPayload`):
```yaml
RemediationOrchestratorAuditPayload:
  type: object
  required:
    - event_type
    - rr_name
    - namespace
  properties:
    event_type:
      type: string
      enum:
        # ... existing values ...
        - orchestrator.lifecycle.created  # Gap #8
    # ... other properties ...
    timeout_config:
      $ref: '#/components/schemas/TimeoutConfigPayload'
      description: 'Timeout configuration (Gap #8: BR-AUDIT-005)'
```

**Regenerate client**:
```bash
cd pkg/datastorage/ogen-client
make generate
```

---

## 📋 **Phase 3: Webhook Extension**

**Duration**: 2 hours
**Goal**: Add operator mutation auditing with `webhook.remediationrequest.timeout_modified`

### **Step 3.1: Create Webhook Handler** (1 hour)

**File**: `pkg/webhooks/remediationrequest_handler.go` (NEW)

```go
/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
        return admission.Denied(fmt.Errorf("authentication required: %v", err))
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
    payload := api.WebhookRemediationRequestTimeoutModifiedPayload{
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

    auditEvent.EventData = api.NewAuditEventRequestEventDataWebhookRemediationRequestTimeoutModifiedAuditEventRequestEventData(payload)

    // Write audit event (non-blocking)
    if err := h.auditStore.Store(ctx, auditEvent); err != nil {
        // Log error but don't block operation (operational resilience)
        // Per DD-WEBHOOK-003: Audit failure should not block business operations
        fmt.Printf("WARNING: Failed to store audit event: %v\n", err)
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

### **Step 3.2: Register Webhook** (15 minutes)

**File**: `cmd/webhooks/main.go`

**Add after line 153**:
```go
// Register RemediationRequest status mutation handler (Gap #8 Extension)
// Captures operator timeout configuration changes (BR-AUTH-001)
rrHandler := webhooks.NewRemediationRequestStatusHandler(auditStore)
if err := rrHandler.InjectDecoder(decoder); err != nil {
    setupLog.Error(err, "failed to inject decoder into RemediationRequest handler")
    os.Exit(1)
}
webhookServer.Register("/mutate-remediationrequest-status", &webhook.Admission{Handler: rrHandler})
setupLog.Info("Registered RemediationRequest status webhook handler with audit store")
```

---

### **Step 3.3: Update OpenAPI Schema** (30 minutes)

**File**: `pkg/datastorage/api/openapi.yaml`

**Add new payload type**:
```yaml
WebhookRemediationRequestTimeoutModifiedPayload:
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

# Update discriminator
AuditEventRequestEventData:
  discriminator:
    mapping:
      # ... existing mappings ...
      webhook.remediationrequest.timeout_modified: '#/components/schemas/WebhookRemediationRequestTimeoutModifiedPayload'
```

**Regenerate**:
```bash
cd pkg/datastorage/ogen-client
make generate
```

---

### **Step 3.4: Create Webhook Manifest** (15 minutes)

**File**: `deploy/webhooks/03-mutatingwebhook.yaml`

**Add to existing file**:
```yaml
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kubernaut-remediationrequest-webhook
  namespace: kubernaut-system
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

## ✅ **Success Criteria**

### **Phase 1 Complete**:
- [ ] `TimeoutConfig` moved from spec to status in CRD
- [ ] CRD manifests regenerated
- [ ] `initializeTimeoutDefaults()` function added
- [ ] All `Status.TimeoutConfig` references updated to `Status.TimeoutConfig`
- [ ] All tests passing

### **Phase 2 Complete (Gap #8)**:
- [ ] `BuildRemediationCreatedEvent()` method added
- [ ] `orchestrator.lifecycle.created` event emitted on RR creation
- [ ] TimeoutConfig captured in audit payload
- [ ] OpenAPI schema updated
- [ ] Integration test validates event emission

### **Phase 3 Complete (Webhook)**:
- [ ] `RemediationRequestStatusHandler` webhook implemented
- [ ] Webhook registered in `cmd/webhooks/main.go`
- [ ] `webhook.remediationrequest.timeout_modified` event emitted
- [ ] Status fields `LastModifiedBy`, `LastModifiedAt` populated
- [ ] OpenAPI schema updated
- [ ] Webhook deployment manifest created

---

## 🚀 **Execution Order**

1. **Day 1 Morning** (4 hours): Phase 1 Steps 1-3 (CRD schema + initialization)
2. **Day 1 Afternoon** (4 hours): Phase 1 Steps 4-6 (Tests + docs)
3. **Day 2 Morning** (2 hours): Phase 2 (Gap #8 implementation)
4. **Day 2 Afternoon** (2 hours): Phase 3 (Webhook extension)

---

## 📊 **Risk Mitigation**

| Risk | Mitigation |
|------|-----------|
| Breaking existing RRs | Document migration, provide kubectl command |
| Test failures | Run tests after each phase |
| Missed references | Grep for `Status.TimeoutConfig` before commit |
| OpenAPI schema issues | Test ogen regeneration in isolation |
| Webhook deployment | Test in Kind cluster before production |

---

## 🎯 **Next Step**

**Begin Phase 1, Step 1.1**: Update CRD schema to move `TimeoutConfig` from spec to status.

**Ready to proceed?**
