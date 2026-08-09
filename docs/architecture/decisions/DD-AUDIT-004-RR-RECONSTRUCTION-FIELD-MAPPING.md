# DD-AUDIT-004: RR Reconstruction Field Mapping

**Date**: 2025-12-18
**Status**: ✅ Approved
**Parent ADR**: [ADR-034 v1.3: Unified Audit Table Design](./ADR-034-unified-audit-table-design.md)
**Business Requirement**: [BR-AUDIT-005 v2.0](../../requirements/11_SECURITY_ACCESS_CONTROL.md)
**Deciders**: Architecture Team

---

## Context

BR-AUDIT-005 v2.0 requires 100% RemediationRequest CRD reconstruction from audit traces for enterprise compliance (SOC 2 Type II, ISO 27001, NIST 800-53). This design decision establishes the authoritative mapping between RR CRD fields and audit event `event_data` JSONB fields.

**Problem**: Services need explicit guidance on which audit fields to capture to enable exact RR reconstruction after TTL expiration (90-365 days).

**Goal**: Define the authoritative field mappings that all services MUST follow when emitting audit events for RR reconstruction.

---

## Decision

This document establishes the **authoritative field mapping matrix** for RemediationRequest CRD reconstruction from audit traces.

**Authority**: All services emitting audit events for RR reconstruction MUST follow these mappings. This is a mandatory compliance requirement for V1.0.

**Scope**: 7 critical fields across 4 services (Gateway, Workflow Engine, Execution, Orchestrator).
Originally 8 fields across 5 services; Field #4 (AI Analysis `providerData`) was removed 2026-08-02
([#1806](https://github.com/jordigilh/kubernaut/issues/1806) /
[#1882](https://github.com/jordigilh/kubernaut/issues/1882)) — see Field #4 entry below for why.

---

## 🎯 **Overview**

This document provides the authoritative mapping between RemediationRequest CRD fields and the corresponding audit event `event_data` JSONB fields used for 100% reconstruction.

**Key Principle**: Each RR field must be captured in at least ONE audit event during the remediation lifecycle.

---

## 📋 **Complete Field Mapping**

### **1. Critical Spec Fields** (MUST HAVE - 100% Required)

| # | RR CRD Field Path | Audit Event Field | Service | Event Type | Data Type | Required | Size |
|---|------------------|-------------------|---------|-----------|-----------|----------|------|
| 1 | `.spec.originalPayload` | `event_data.original_payload` | Gateway | `gateway.signal.received` | JSON Object | ✅ YES | 2-5KB |
| 2 | `.spec.signalLabels` | `event_data.signal_labels` | Gateway | `gateway.signal.received` | JSON Object | ✅ YES | 0.5-2KB |
| 3 | `.spec.signalAnnotations` | `event_data.signal_annotations` | Gateway | `gateway.signal.received` | JSON Object | ✅ YES | 0.5-2KB |
| ~~4~~ | ~~`.spec.aiAnalysis.providerData`~~ | **REMOVED** — see Field #4 below | — | — | — | — | — |
| 5 | `.status.selectedWorkflowRef` | `event_data.selected_workflow_ref` | Workflow Engine | `workflow.selection.completed` | JSON Object | ✅ YES | 200B |
| 6 | `.status.executionRef` | `event_data.execution_ref` | Execution | `execution.started` | JSON Object | ✅ YES | 200B |
| 7 | `.status.error` | `event_data.error_details` | All Services | `*.failure` | JSON Object | ⚠️ OPTIONAL | 0.5-1KB |
| 8 | `.status.timeoutConfig` | `event_data.timeout_config` | Orchestrator | `orchestration.remediation.created` | JSON Object | ⚠️ OPTIONAL | 100-200B |

*Row numbering (1, 2, 3, 5-8) is preserved as-is rather than renumbered, so cross-references elsewhere
in this document and in dependent docs stay valid.*

**Total Storage Impact**: ~4-9KB per remediation (compressed) — 7 active fields (Field #4 removed)

---

## 📊 **Field-by-Field Details**

### **Field #1: `originalPayload`** (CRITICAL)

**RR CRD Path**: `.spec.originalPayload`

**Audit Event Capture**:
```yaml
event_type: gateway.signal.received
event_category: gateway
event_data:
  original_payload:  # ← NEW FIELD
    apiVersion: "v1"
    kind: "Event"
    metadata:
      name: "api-server.oom.12345"
      namespace: "web"
      uid: "abc-123-def-456"
      creationTimestamp: "2025-01-15T10:30:00Z"
    involvedObject:
      kind: "Pod"
      name: "api-server"
      namespace: "web"
    reason: "OOMKilled"
    message: "Container exceeded memory limit"
    firstTimestamp: "2025-01-15T10:29:45Z"
    lastTimestamp: "2025-01-15T10:29:45Z"
    count: 1
    type: "Warning"
    source:
      component: "kubelet"
      host: "node-01"
```

**Implementation**:
- **File**: `pkg/gateway/signal_processor.go`
- **Change**: Add `original_payload` to `GatewayEventData`
- **Validation**: JSON structure must be valid K8s Event or Prometheus AlertManager webhook

**Reconstruction Logic**:
```go
// Reconstruct RR from audit traces
gatewayEvent := getAuditEvent(ctx, "gateway.signal.received", correlationID)
rr.Spec.OriginalPayload = gatewayEvent.EventData["original_payload"]
```

---

### **Field #2: `signalLabels`** (CRITICAL)

**RR CRD Path**: `.spec.signalLabels`

**Audit Event Capture**:
```yaml
event_type: gateway.signal.received
event_category: gateway
event_data:
  signal_labels:  # ← NEW FIELD
    app: "api-server"
    environment: "production"
    tier: "backend"
    version: "v2.1.0"
    cost-tier: "standard"
    team: "platform"
```

**Implementation**:
- **File**: `pkg/gateway/signal_processor.go`
- **Change**: Add `signal_labels` to `GatewayEventData`
- **Type**: `map[string]string`

**Reconstruction Logic**:
```go
gatewayEvent := getAuditEvent(ctx, "gateway.signal.received", correlationID)
rr.Spec.SignalLabels = gatewayEvent.EventData["signal_labels"].(map[string]string)
```

---

### **Field #3: `signalAnnotations`** (CRITICAL)

**RR CRD Path**: `.spec.signalAnnotations`

**Audit Event Capture**:
```yaml
event_type: gateway.signal.received
event_category: gateway
event_data:
  signal_annotations:  # ← NEW FIELD
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    kubectl.kubernetes.io/last-applied-configuration: "..."
    description: "Production API server for user authentication"
```

**Implementation**:
- **File**: `pkg/gateway/signal_processor.go`
- **Change**: Add `signal_annotations` to `GatewayEventData`
- **Type**: `map[string]string`

**Reconstruction Logic**:
```go
gatewayEvent := getAuditEvent(ctx, "gateway.signal.received", correlationID)
rr.Spec.SignalAnnotations = gatewayEvent.EventData["signal_annotations"].(map[string]string)
```

---

### **Field #4: `providerData`** — ❌ REMOVED (2026-08-03)

> **✅ RESOLVED ([#1882](https://github.com/jordigilh/kubernaut/issues/1882))**: This field
> mapping was stale (flagged 2026-08-02, [#1806](https://github.com/jordigilh/kubernaut/issues/1806))
> and has now been removed from RR reconstruction scope per an explicit architecture-team decision:
> - `.spec.aiAnalysis.providerData` never existed on `RemediationRequestSpec`
>   (`api/remediation/v1alpha1/remediationrequest_types.go`). AI decision data (confidence,
>   reasoning, selected workflow, provider/model) belongs to the **AIAnalysis CRD**
>   (`AIAnalysis.Status`), not `RemediationRequest`.
> - **Decision**: RR CRD reconstruction does **not** need a "Field #4". `RemediationRequest`'s
>   own spec/status never carried AI provider/reasoning data, so there is nothing to remove from
>   the RR CRD's reconstructed shape — this was a mapping error, not a missing capability.
> - The underlying need — capturing AI provider/confidence/reasoning/recommendations data in the
>   audit trail so it can be reconstructed later — is real, but it targets a **future AIAnalysis
>   CRD reconstruction**, not RR reconstruction. That work is already captured as deferred,
>   not-yet-implemented future scope in
>   [DD-AUDIT-007: Full Child CRD Reconstruction (Future)](./DD-AUDIT-007-full-child-crd-reconstruction-future.md),
>   which explicitly proposes an `aianalysis.lifecycle.snapshot` event carrying the full
>   AIAnalysis spec + status (including `selected_workflow`, `confidence`,
>   `root_cause_analysis`) — see that document's "OpenAPI Schema Extensions" section.
> - Today (V1.0), the AI decision fields that *do* exist on the real
>   `aianalysis.analysis.completed` payload (`Confidence`, `WorkflowID`, `ApprovalRequired`,
>   `ApprovalReason`, `DegradedMode`, `WarningsCount`, `Reason`, `SubReason`) are already captured
>   per [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md) ("2. AI Analysis
>   Controller" → "SOC2 Compliance Event") and queryable via `correlation_id` — they are just not
>   part of *RR* reconstruction, consistent with the decision above.
>
> **No action required in this document** beyond removing the field (done). If/when
> DD-AUDIT-007 is implemented, its own field mapping table is the correct place for an
> AIAnalysis-CRD equivalent of this document — not a re-added Field #4 here.

This field has been struck from the mapping table above. No RR CRD path, audit event, or
reconstruction logic exists for it — see the resolution note for where this data actually lives.

---

### **Field #5: `selectedWorkflowRef`** (CRITICAL)

**RR CRD Path**: `.status.selectedWorkflowRef`

**Audit Event Capture**:
```yaml
event_type: workflow.selection.completed
event_category: workflow
event_data:
  selected_workflow_ref:  # ← NEW FIELD
    name: "pod-oom-remediation-v2"
    namespace: "kubernaut-workflows"
    version: "v2.1.0"
    score: 0.89
    reason: "Best match for OOMKilled events with 89% confidence"
```

**Implementation**:
- **File**: `pkg/workflowengine/workflow_selector.go`
- **Change**: Add `selected_workflow_ref` to workflow selection audit event
- **Type**: JSON object with workflow metadata

**Reconstruction Logic**:
```go
workflowEvent := getAuditEvent(ctx, "workflow.selection.completed", correlationID)
rr.Status.SelectedWorkflowRef = workflowEvent.EventData["selected_workflow_ref"]
```

---

### **Field #6: `executionRef`** (CRITICAL)

**RR CRD Path**: `.status.executionRef`

**Audit Event Capture**:
```yaml
event_type: execution.started
event_category: execution
event_data:
  execution_ref:  # ← NEW FIELD
    name: "workflow-execution-abc123"
    namespace: "kubernaut-system"
    uid: "def-456-ghi-789"
    kind: "WorkflowExecution"
    apiVersion: "execution.kubernaut.ai/v1alpha1"
```

**Implementation**:
- **File**: `pkg/remediationexecution/controller.go`
- **Change**: Add `execution_ref` to execution start audit event
- **Type**: JSON object with CRD reference

**Reconstruction Logic**:
```go
executionEvent := getAuditEvent(ctx, "execution.started", correlationID)
rr.Status.ExecutionRef = executionEvent.EventData["execution_ref"]
```

---

### **Field #7: `error` (detailed)** (OPTIONAL)

**RR CRD Path**: `.status.error`

**Audit Event Capture**:
```yaml
event_type: *.failure  # Any failure event
event_category: <any>
event_data:
  error_details:  # ← NEW FIELD
    message: "Failed to execute workflow step: increase-memory-limit"
    code: "EXECUTION_STEP_FAILED"
    component: "remediation-execution"
    phase: "execution"
    step: "increase-memory-limit"
    retry_possible: true
    retry_count: 2
    original_error: "kubectl patch failed: timeout after 30s"
```

**Implementation**:
- **File**: All services (gateway, aianalysis, workflow, execution, orchestrator)
- **Change**: Add `error_details` to all `*.failure` audit events
- **Type**: JSON object with structured error information

**Reconstruction Logic**:
```go
failureEvent := getAuditEvent(ctx, "*.failure", correlationID)
if failureEvent != nil {
    rr.Status.Error = failureEvent.EventData["error_details"]
}
```

---

### **Field #8: `timeoutConfig`** (OPTIONAL)

**RR CRD Path**: `.status.timeoutConfig`

**Audit Event Capture**:
```yaml
event_type: orchestration.remediation.created
event_category: orchestration
event_data:
  timeout_config:  # ← NEW FIELD
    global_timeout: "30m"
    phase_timeouts:
      aianalysis: "5m"
      workflow_selection: "2m"
      execution: "20m"
      notification: "1m"
```

**Implementation**:
- **File**: `pkg/remediationorchestrator/controller.go`
- **Change**: Add `timeout_config` to remediation creation audit event
- **Type**: JSON object with timeout durations

**Reconstruction Logic**:
```go
orchestratorEvent := getAuditEvent(ctx, "orchestration.remediation.created", correlationID)
if timeoutConfig, ok := orchestratorEvent.EventData["timeout_config"]; ok {
    rr.Status.TimeoutConfig = timeoutConfig
}
```

---

## 🔍 **Reconstruction Algorithm**

### **Step-by-Step Reconstruction Process**

```go
// ReconstructRR rebuilds a RemediationRequest from audit traces
func ReconstructRR(ctx context.Context, correlationID string) (*RemediationRequest, error) {
    rr := &RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Annotations: map[string]string{
                "kubernaut.ai/reconstructed": "true",
            },
        },
    }

    // STEP 1: Gateway fields (original_payload, labels, annotations)
    gatewayEvent := getAuditEvent(ctx, "gateway.signal.received", correlationID)
    if gatewayEvent == nil {
        return nil, errors.New("missing gateway.signal.received event")
    }
    rr.Spec.OriginalPayload = gatewayEvent.EventData["original_payload"]
    rr.Spec.SignalLabels = gatewayEvent.EventData["signal_labels"].(map[string]string)
    rr.Spec.SignalAnnotations = gatewayEvent.EventData["signal_annotations"].(map[string]string)

    // STEP 2 removed (Field #4, resolved #1882): RemediationRequestSpec has no AIAnalysis
    // field — AI provider/confidence/reasoning data belongs to the AIAnalysis CRD, not RR.
    // See DD-AUDIT-007 for the (deferred, not yet implemented) AIAnalysis CRD reconstruction
    // path that would carry this data instead.

    // STEP 3: Workflow selection (selected_workflow_ref)
    workflowEvent := getAuditEvent(ctx, "workflow.selection.completed", correlationID)
    if workflowEvent != nil {
        rr.Status.SelectedWorkflowRef = workflowEvent.EventData["selected_workflow_ref"]
    }

    // STEP 4: Execution reference (execution_ref)
    executionEvent := getAuditEvent(ctx, "execution.started", correlationID)
    if executionEvent != nil {
        rr.Status.ExecutionRef = executionEvent.EventData["execution_ref"]
    }

    // STEP 5: Error details (if any failure occurred)
    failureEvent := getAuditEvent(ctx, "*.failure", correlationID)
    if failureEvent != nil {
        rr.Status.Error = failureEvent.EventData["error_details"]
    }

    // STEP 6: Timeout config (optional)
    orchestratorEvent := getAuditEvent(ctx, "orchestration.remediation.created", correlationID)
    if orchestratorEvent != nil {
        if timeoutConfig, ok := orchestratorEvent.EventData["timeout_config"]; ok {
            rr.Status.TimeoutConfig = timeoutConfig
        }
    }

    // STEP 7: Calculate reconstruction accuracy
    accuracy := calculateAccuracy(rr)
    rr.ObjectMeta.Annotations["kubernaut.ai/reconstruction-accuracy"] = fmt.Sprintf("%d%%", accuracy)

    return rr, nil
}
```

---

## 📊 **Validation Rules**

### **Field Validation Matrix**

| Field | Required for 100%? | Validation Rule | Failure Handling |
|-------|-------------------|-----------------|------------------|
| `original_payload` | ✅ YES | Must be valid JSON | Return error |
| `signal_labels` | ✅ YES | Must be map[string]string | Return error |
| `signal_annotations` | ✅ YES | Must be map[string]string | Return error |
| ~~`provider_data`~~ | — | **removed, see Field #4** | — |
| `selected_workflow_ref` | ✅ YES | Must have `name` field | Return error |
| `execution_ref` | ✅ YES | Must have `name` field | Return error |
| `error_details` | ❌ NO | Optional | Skip if missing |
| `timeout_config` | ❌ NO | Optional | Skip if missing |

---

## 🎯 **Reconstruction Accuracy Calculation**

```go
// calculateAccuracy determines reconstruction completeness
func calculateAccuracy(rr *RemediationRequest) int {
    totalFields := 7 // Field #4 removed 2026-08-03, see resolution note (#1882)

    capturedFields := 0

    // Required fields (5 fields = 71% of accuracy)
    if rr.Spec.OriginalPayload != nil { capturedFields++ }
    if len(rr.Spec.SignalLabels) > 0 { capturedFields++ }
    if len(rr.Spec.SignalAnnotations) > 0 { capturedFields++ }
    if rr.Status.SelectedWorkflowRef != nil { capturedFields++ }
    if rr.Status.ExecutionRef != nil { capturedFields++ }

    // Optional fields (2 fields = 29% of accuracy)
    if rr.Status.Error != nil { capturedFields++ }
    if rr.Status.TimeoutConfig != nil { capturedFields++ }

    return (capturedFields * 100) / totalFields
}
```

**Accuracy Targets**:
- **100%**: All 7 fields captured (5 required + 2 optional)
- **71%**: All 5 required fields captured (minimum for V1.0)
- **<71%**: Incomplete reconstruction (ERROR)

---

## 📋 **Implementation Checklist**

### **Pre-Implementation** (Before Day 1)
- [x] Field mapping matrix created (this document)
- [ ] Schema validation added to event builders
- [ ] Reconstruction algorithm implemented
- [ ] Unit tests for field mapping

### **Per-Field Implementation** (Days 1-4)
For each of the 7 active fields (#1-3, #5-8; Field #4 removed):
- [ ] Add field to service's event builder
- [ ] Update audit event emission code
- [ ] Add unit test for field capture
- [ ] Add integration test for reconstruction
- [ ] Verify field appears in database

### **Reconstruction API** (Days 5-6)
- [ ] Implement `ReconstructRR()` function
- [ ] Add accuracy calculation
- [ ] Add validation for required fields
- [ ] Handle missing optional fields gracefully
- [ ] Add integration tests

---

## 📊 **Storage Impact Analysis**

| Field | Average Size | Compression Ratio | Compressed Size |
|-------|-------------|-------------------|-----------------|
| `original_payload` | 3KB | 3:1 | 1KB |
| `signal_labels` | 1KB | 2:1 | 0.5KB |
| `signal_annotations` | 1KB | 2:1 | 0.5KB |
| ~~`provider_data`~~ | — | — | **removed, see Field #4** |
| `selected_workflow_ref` | 200B | 1.5:1 | 133B |
| `execution_ref` | 200B | 1.5:1 | 133B |
| `error_details` | 0.5KB | 2:1 | 250B |
| `timeout_config` | 150B | 1.5:1 | 100B |

**Total Per Remediation** (Field #4 removed):
- **Uncompressed**: ~6KB
- **Compressed**: ~2.5KB
- **With 7-year retention**: 2.5KB × 1M remediations = **2.5GB** (manageable)

---

---

## Extension: `workflow_content` Field Mapping (Issue #1661, 2026-07-14)

> **Scope note**: unlike Fields #1-8 above, `workflow_content` does not reconstruct a `RemediationRequest` CRD
> field -- it reconstructs a **`RemediationWorkflow`** CRD's exact admitted spec, independent of etcd or Data
> Storage's cache (SOC2 CC8.1). It is documented here because it follows the same authoritative
> field-mapping-and-reconstruction-algorithm pattern as Fields #1-8, and because the cross-event join it enables
> (workflow execution events joining back to workflow admission events) directly supports the same audit
> completeness goal (BR-AUDIT-005 v2.0) this document governs.

### Field #9: `workflow_content` + `content_hash` (RemediationWorkflow CRD reconstruction)

**Source CRD**: `RemediationWorkflow.spec` (not RemediationRequest)

**Audit Event Capture**:
```yaml
event_type: remediationworkflow.admitted.create   # also: .update, .denied (best-effort)
event_category: authwebhook
event_data:
  workflow_name: "fix-security-context-job"
  content_hash: "3f9a1c...e02b"          # ← NEW: SHA-256 of the marshaled clean CRD content
  workflow_content:                       # ← NEW: full RemediationWorkflowSpec, field-for-field
    version: "1.1.0"
    action_type: "RestartPod"
    description:
      what: "Restarts a Pod stuck in CrashLoopBackOff"
      when_to_use: "..."
    labels:
      severity: ["critical", "high"]
      environment: ["production"]
      component: ["api-server"]
      priority: "P1"
      cluster: ["prod-east"]
    maintainers:
      - name: "platform-team"
        email: "platform@example.com"
    parameters:
      - name: "namespace"
        type: "string"
        required: true
    rollback_parameters: []
```

**Implementation**:
- **File**: `pkg/authwebhook/remediationworkflow_audit.go` (`buildWorkflowContentPayload`, `attachWorkflowContent`)
- **Schema**: `RemediationWorkflowContentPayload` in `api/openapi/data-storage-v1.yaml` -- fully decomposed/typed
  (not an opaque JSON blob), mirroring `RemediationWorkflowSpec` field-for-field, consistent with this document's
  "structured, not raw JSON" precedent for Fields #1-8.
- **Hash algorithm**: SHA-256 hex digest of the same clean-CRD-content bytes DS's own `computeContentHash` hashes,
  computed locally by AuthWebhook (zero divergence risk, no DS round-trip required to attach it).
- **Coverage**: emitted on `create`/`update` unconditionally, and on `denied` best-effort (whenever `rw.Spec`
  unmarshaled successfully, even if the request was ultimately denied) -- extending audit forensic value beyond
  the strict SOC2 CC8.1 floor, per DD-WORKFLOW-018 Change 2.

**Reconstruction Logic**:
```go
// Reconstruct a RemediationWorkflow's exact admitted spec at any point in its history --
// including versions superseded or CRDs since deleted, independent of etcd or DS's cache.
event := getLatestAuditEvent(ctx, "remediationworkflow.admitted.create", "remediationworkflow.admitted.update",
	byDetail("workflow_id", workflowID))
spec := event.EventData["workflow_content"].(RemediationWorkflowContentPayload)
```

### Cross-Event Join: Execution Events → Workflow Admission Events

`WorkflowExecutionAuditPayload` (`workflowexecution.execution.started`/`.workflow.completed`/`.failed`) carries
`workflow_name` and `action_type` for direct human readability (DD-WORKFLOW-018 Change 8/9), but the durable,
collision-free join key back to the exact admitted workflow definition remains `workflow_id`, present on both
sides:

```sql
-- "What did the workflow that executed for remediation X actually look like when it was admitted?"
SELECT
    exec.event_data->>'workflow_name'  AS executed_workflow_name,
    exec.event_data->>'action_type'    AS executed_action_type,
    admit.event_data->'workflow_content' AS admitted_workflow_spec,
    admit.event_data->>'content_hash'    AS admitted_content_hash
FROM audit_events exec
JOIN audit_events admit
    ON admit.event_data->>'workflow_id' = exec.event_data->>'workflow_id'
   AND admit.event_type IN ('remediationworkflow.admitted.create', 'remediationworkflow.admitted.update')
WHERE exec.correlation_id = $1
  AND exec.event_type = 'workflowexecution.execution.started'
ORDER BY admit.created_at DESC
LIMIT 1;
```

This join is what makes the CRD-embedded-snapshot design (DD-WORKFLOW-018 Change 8) auditable end-to-end without
relying on the audit trail as an *execution-time* fallback (that alternative was considered and rejected -- see
DD-WORKFLOW-018 Change 8's "Rejected Alternative" note): `WorkflowExecution` never queries `audit_events` to
execute, but an auditor or incident-reconstruction query can always use `audit_events` after the fact to answer
"what workflow definition actually ran," even for a `RemediationWorkflow` version since superseded or a CRD since
deleted (DD-WORKFLOW-018's deletion-semantics decision: CRD DELETE never mutates historical audit rows).

---

## Extension: `apifrontend.rr.created` as an Alternate Genesis Event (Issue #2043, 2026-08-09)

> **Scope note**: this extension does not add a new RR field to the mapping matrix above -- the
> fields it covers (`signal_name`/`signal_type`/`signal_fingerprint`, plus `cluster_id`) were
> already implicitly assumed to live solely on `gateway.signal.received` (see the Reconstruction
> Algorithm's STEP 1, which historically hard-failed reconstruction when that event was absent).
> It is documented here because it changes the mandatory precondition of STEP 1 for a real,
> shipped ingestion path that never emits a Gateway event at all.

### Problem

`RemediationRequest`s created directly via APIFrontend's `kubernaut_remediate` MCP tool
(`pkg/apifrontend/tools/af_create_rr.go`) bypass the Gateway service entirely -- there is no
signal ingestion, webhook, or Prometheus alert involved. These RRs therefore **never** emit a
`gateway.signal.received` event. Before this fix, `MergeAuditData` unconditionally required
`gateway.signal.received` to be present (`pkg/datastorage/reconstruction/mapper.go`), which made
reconstruction of every AF-created RR's genesis data **architecturally impossible** -- not a
missing-data gap, but a hard `return nil, error` for a real, shipped ingestion path.

### Decision

`apifrontend.rr.created` (`ApifrontendRRCreatedPayload` in `api/openapi/data-storage-v1.yaml`) is
now a second, equally-valid **genesis event** for RR reconstruction. Exactly one of the two
genesis event types must be present; either satisfies the requirement:

| RR CRD Field Path | Audit Event Field | Service | Event Type (either satisfies) |
|---|---|---|---|
| `.spec.signalName` | `event_data.signal_name` | Gateway **or** APIFrontend | `gateway.signal.received` **or** `apifrontend.rr.created` |
| `.spec.signalType` | *(Gateway: explicit field; APIFrontend: hardcoded `"alert"`)* | Gateway **or** APIFrontend | `gateway.signal.received` **or** `apifrontend.rr.created` |
| `.spec.signalFingerprint` | `event_data.fingerprint` | Gateway **or** APIFrontend | `gateway.signal.received` **or** `apifrontend.rr.created` |
| `.spec.clusterID` | envelope `cluster_id` | Gateway **or** APIFrontend | `gateway.signal.received` **or** `apifrontend.rr.created` |

`signal_type` is not carried as an explicit field on `ApifrontendRRCreatedPayload` because
AF-created RRs always hardcode it to `"alert"` in production (`buildRRObject`,
`pkg/apifrontend/tools/af_create_rr.go`); the parser mirrors this constant rather than inferring
it, so there is nothing ambiguous to reconstruct.

**Implementation**:
- `pkg/datastorage/reconstruction/query.go`: `apifrontend.rr.created` added to the reconstruction
  query allowlist, `eventDataDecoders` (via `decodeApifrontendRRCreatedPayload`), and
  `IsReconstructionRelevant`.
- `pkg/datastorage/reconstruction/parser.go`: `parseApifrontendRRCreated` dispatch case --
  extracts `SignalName`/`SignalFingerprint` from the payload, hardcodes `SignalType: "alert"`, and
  returns an error if `signal_name` is missing (mirrors the Gateway parser's required-field
  contract).
- `pkg/datastorage/reconstruction/mapper.go`: `mapApifrontendRRCreatedFields` (mirrors
  `mapGatewaySignalFields`'s Spec assignments); `genesisEventTypes`/`hasGenesisEvent` replace the
  single-event-type check in `MergeAuditData`.
- **Emission-side prerequisite** (same issue, same PR): `pkg/apifrontend/tools/af_create_rr.go`
  and `pkg/apifrontend/audit/store_adapter.go` were also fixed to actually populate
  `CorrelationID`, `target_kind`/`target_name`/`rr_name`/`fingerprint`/`signal_name` in the emitted
  event -- these fields were schema-required since Issue #1021 but silently persisted as `""`
  because the `Detail` map's key names never matched what `buildRRCreatedPayload` read. Without
  this emission-side fix, the reconstruction-side dispatch above would successfully parse an event
  whose `signal_name` was always empty, and immediately fail the "missing signal_name" check.

**Reconstruction Logic** (STEP 1 replacement, mapper.go's `MergeAuditData`):
```go
// Previously: unconditionally required gateway.signal.received, hard-failing
// reconstruction for every AF-created RR.
if !hasGenesisEvent(events) {  // gateway.signal.received OR apifrontend.rr.created
    return nil, fmt.Errorf("gateway.signal.received or apifrontend.rr.created event is required for reconstruction")
}
```

**Tests**:
- Unit: `pkg/datastorage/reconstruction/parser_test.go` (`PARSER-AF-01`),
  `pkg/datastorage/reconstruction/mapper_test.go` (`MAPPER-AF-01`, `MAPPER-MERGE-AF-01`)
- Integration (real PostgreSQL, full production dispatch path -- query allowlist → decoder →
  parser dispatch → mapper genesis-event acceptance):
  `test/integration/datastorage/reconstruction_integration_test.go` (`INTEGRATION-AF-01`)

---

## Related Decisions

- **ADR-034 v1.3**: [Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) - Parent ADR establishing this as authoritative subdocument
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Defines distributed audit pattern
- **DD-AUDIT-002**: [Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md) - Event builder implementation
- **DD-AUDIT-003**: [Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md) - Service responsibilities

---

## Implementation References

- **Business Requirement**: [BR-AUDIT-005 v2.0](../../requirements/11_SECURITY_ACCESS_CONTROL.md)
- **Implementation Plan**: [AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md](../../handoff/AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md)
- **Gap Closure Plan**: [RR_RECONSTRUCTION_V1_0_GAP_CLOSURE_PLAN_DEC_18_2025.md](../../handoff/RR_RECONSTRUCTION_V1_0_GAP_CLOSURE_PLAN_DEC_18_2025.md)

---

**Approved By**: Architecture Team
**Date**: 2025-12-18
**Next Review**: After V1.0 implementation complete

