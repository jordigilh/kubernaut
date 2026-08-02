# Effectiveness Monitor Service - CRD Schema

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | **v2.0** | 2026-08-02 | **RENAMED + REWRITE** ([#1806](https://github.com/jordigilh/kubernaut/issues/1806)): Renamed from `api-specification.md` — EM has no REST API, so an "API specification" framing was itself fictional. Replaced the entire fictional HTTP API (`POST /api/v1/assess/effectiveness`, `GET /api/v1/assess/effectiveness/:actionID`, `POST /api/v1/assess/batch`, `GET /api/v1/assess/data-availability`, port 8080, `shouldCallAI()`) with the real `EffectivenessAssessment` CRD schema, regenerated against `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go`. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
> | v1.0 | 2025-10-06 | ⚠️ **STALE (superseded by v2.0)** — Original `api-specification.md`. Described a stateless HTTP API service that was never built as such. | — |

**Source of Truth**: `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go`

**API Group**: `kubernaut.ai/v1alpha1` (single unified API group for all Kubernaut CRDs, DD-CRD-001)

**Kind**: `EffectivenessAssessment` (short name: `ea`)

EM has no REST/HTTP API. This document is the "API specification" in the sense that matters for a CRD controller: the spec/status contract of the `EffectivenessAssessment` (EA) CRD that clients (the Remediation Orchestrator) and EM itself read and write.

---

## Key Design Decisions

| Document | Impact on CRD |
|----------|---------------|
| **ADR-001** | Spec immutability — `EffectivenessAssessmentSpec` carries a CEL `self == oldSelf` rule |
| **ADR-EM-001** | Integration architecture — EM emits raw per-component audit events; DataStorage computes the final weighted score on demand, not the EA CRD |
| **DD-EM-002** | Canonical spec hash algorithm (`pkg/shared/hash.CanonicalResourceFingerprint`) and the Spec Drift Guard (`Match=false` → remediation considered unsuccessful) |
| **DD-EM-003** | Dual-target assessment — `signalTarget` (what alerted) vs. `remediationTarget` (what the workflow changed) are tracked separately on the spec |
| **DD-EM-004** | Async hash deferral — `config.hashComputeDelay` and the `WaitingForPropagation` phase, for GitOps/operator-managed targets where spec changes propagate asynchronously |
| **DD-EM-005** | Cluster-scoped Node/PersistentVolume metrics via kube-state-metrics, for targets with no namespace |
| **DD-CRD-002-EA** | The 5 status Conditions (`Ready`, `AssessmentComplete`, `SpecIntegrity`, `AlertDecayDetected`, `PostHashCaptured`) and their canonical reasons |
| **BR-EM-012 / Issue #369** | Alert decay detection — `components.alertDecayRetries` and the `AlertDecayTimeout` terminal outcome |
| **DD-017** | V1.0 (Level 1, this schema) vs. V1.1 (Level 2, AI analysis — not part of this CRD) scoping |

---

## V1.0 Scope Clarifications

| Feature | Status | Notes |
|---------|--------|-------|
| **Final effectiveness score** | Not on this CRD | EM emits per-component scores to `status.components.*Score`; the single weighted score (`health×0.40 + alert×0.35 + metrics×0.25`) is computed by **DataStorage on demand** from audit events, not stored on the EA |
| **AI/LLM analysis fields** | None exist | No `RootCauseValidation`, `OscillationDetected`, or similar V1.1 fields exist on spec or status today (planned, DD-017) |
| **Request/response contract** | N/A | There is no way to "request" an assessment via API — the EA is created once by the RO and its spec is immutable |
| **Batch assessment** | N/A | Each `RemediationRequest` gets exactly one EA; there is no batch endpoint or CRD |

---

## CRD Definition

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ea
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.assessmentReason`
// +kubebuilder:printcolumn:name="CorrelationID",type=string,JSONPath=`.spec.correlationID`
// +kubebuilder:printcolumn:name="ReadyReason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EffectivenessAssessment is the Schema for the effectivenessassessments API.
// It is created by the Remediation Orchestrator and watched by the Effectiveness Monitor.
type EffectivenessAssessment struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   EffectivenessAssessmentSpec   `json:"spec,omitempty"`
    Status EffectivenessAssessmentStatus `json:"status,omitempty"`
}
```

---

## Spec Fields

### EffectivenessAssessmentSpec

Set once by the Remediation Orchestrator at creation time. Immutable (`self == oldSelf` CEL rule, ADR-001).

```go
type EffectivenessAssessmentSpec struct {
    // CorrelationID is the name of the parent RemediationRequest.
    // Used as the correlation ID for audit events (DD-AUDIT-CORRELATION-002).
    CorrelationID string `json:"correlationID"`

    // RemediationRequestPhase is the RemediationRequest's OverallPhase at the time
    // the EA was created: Verifying (happy path) | Completed | Failed | TimedOut.
    RemediationRequestPhase string `json:"remediationRequestPhase"`

    // SignalTarget is the resource that triggered the alert.
    // Source: RR.Spec.TargetResource (from Gateway alert extraction).
    // Used by: health assessment, alert resolution, metrics queries (DD-EM-003).
    SignalTarget TargetResource `json:"signalTarget"`

    // RemediationTarget is the resource the workflow modified.
    // Source: AA.Status.RootCauseAnalysis.RemediationTarget (from KA RCA resolution).
    // Used by: spec hash computation, drift detection (DD-EM-003).
    RemediationTarget TargetResource `json:"remediationTarget"`

    // Config contains the assessment configuration parameters.
    Config EAConfig `json:"config"`

    // RemediationCreatedAt is the creation timestamp of the parent RemediationRequest.
    // Used to compute resolution_time_seconds in the assessment.completed audit event.
    RemediationCreatedAt *metav1.Time `json:"remediationCreatedAt,omitempty"`

    // SignalName is the original alert/signal name from the parent RemediationRequest.
    SignalName string `json:"signalName,omitempty"`

    // PreRemediationSpecHash is the canonical spec hash of the target resource BEFORE
    // remediation was applied. Copied from rr.Status.PreRemediationSpecHash by the RO.
    PreRemediationSpecHash string `json:"preRemediationSpecHash,omitempty"`

    // ClusterID is the remote cluster identifier for fleet-managed signals (BR-FLEET-054).
    // When non-empty, target-facing reads (health, alert, hash) route through
    // fleet.ReaderFactory to the remote cluster.
    ClusterID string `json:"clusterID,omitempty"`
}
```

### TargetResource

```go
type TargetResource struct {
    Kind       string `json:"kind"`                 // e.g. "Deployment", "StatefulSet", "Node"
    Name       string `json:"name"`
    Namespace  string `json:"namespace,omitempty"`   // empty for cluster-scoped resources (Node, PersistentVolume)
    APIVersion string `json:"apiVersion,omitempty"`   // "group/version"; disambiguates Kinds that exist in multiple API groups (Issue #1040)
}
```

### EAConfig

```go
type EAConfig struct {
    // StabilizationWindow is the duration to wait after remediation before assessment.
    StabilizationWindow metav1.Duration `json:"stabilizationWindow"`

    // HashComputeDelay defers post-remediation spec hash computation for
    // async-managed targets (GitOps, operator CRDs). Nil = compute immediately.
    // Reference: DD-EM-004, BR-EM-010.
    HashComputeDelay *metav1.Duration `json:"hashComputeDelay,omitempty"`

    // AlertCheckDelay is an additional deferral for alert resolution checks,
    // beyond StabilizationWindow, for proactive/predictive alerts. Nil = no extra delay.
    // Reference: ADR-EM-001, BR-EM-009.
    AlertCheckDelay *metav1.Duration `json:"alertCheckDelay,omitempty"`
}
```

Note: `PrometheusEnabled`, `AlertManagerEnabled`, and `ValidityWindow` are **not** on the CRD spec — they are EM-internal configuration read from `internal/config/effectivenessmonitor/config.go`, not set per-assessment by the RO.

---

## Status Fields

### EffectivenessAssessmentStatus

Written only by the EM controller.

```go
type EffectivenessAssessmentStatus struct {
    // Phase is the current lifecycle phase.
    // Enum: Pending | WaitingForPropagation | Stabilizing | Assessing | Completed | Failed
    Phase string `json:"phase,omitempty"`

    // ValidityDeadline is the absolute time after which the assessment expires.
    // Computed on first reconcile: EA.creationTimestamp + validityWindow (EM config).
    ValidityDeadline *metav1.Time `json:"validityDeadline,omitempty"`

    // PrometheusCheckAfter is the earliest time to query Prometheus for metrics.
    // Computed on first reconcile: EA.creationTimestamp + StabilizationWindow.
    PrometheusCheckAfter *metav1.Time `json:"prometheusCheckAfter,omitempty"`

    // AlertManagerCheckAfter is the earliest time to check AlertManager.
    // Computed: EA.creationTimestamp + StabilizationWindow + AlertCheckDelay (if set).
    AlertManagerCheckAfter *metav1.Time `json:"alertManagerCheckAfter,omitempty"`

    // Components tracks the completion state and scores of each assessment component.
    Components EAComponents `json:"components,omitempty"`

    // AssessmentReason describes why the assessment completed with this outcome.
    // Enum: Full | Partial | NoExecution | MetricsTimedOut | Expired | SpecDrift | AlertDecayTimeout | Unrecoverable
    AssessmentReason string `json:"assessmentReason,omitempty"`

    // CompletedAt is the timestamp when the assessment finished.
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // Message provides human-readable details about the current state.
    Message string `json:"message,omitempty"`

    // Conditions represent the latest available observations of the EA's state.
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### EAComponents

```go
type EAComponents struct {
    HealthAssessed bool     `json:"healthAssessed,omitempty"`
    HealthScore    *float64 `json:"healthScore,omitempty"`     // 0.0-1.0, nil if not yet assessed

    HashComputed             bool   `json:"hashComputed,omitempty"`
    PostRemediationSpecHash  string `json:"postRemediationSpecHash,omitempty"`
    CurrentSpecHash          string `json:"currentSpecHash,omitempty"` // re-computed each reconcile after HashComputed=true; diverges from PostRemediationSpecHash => drift

    AlertAssessed bool     `json:"alertAssessed,omitempty"`
    AlertScore    *float64 `json:"alertScore,omitempty"`      // 0.0 or 1.0, nil if not yet assessed

    MetricsAssessed bool     `json:"metricsAssessed,omitempty"`
    MetricsScore    *float64 `json:"metricsScore,omitempty"`  // 0.0-1.0, nil if not yet assessed

    // AlertDecayRetries counts re-checks of a firing alert during decay monitoring
    // (BR-EM-012, Issue #369). Non-zero means EM confirmed the resource healthy
    // but the AlertManager alert persisted (Prometheus lookback-window decay).
    AlertDecayRetries int32 `json:"alertDecayRetries,omitempty"`
}
```

---

## Status Conditions (DD-CRD-002-EA)

5 condition types, set via the canonical `meta.SetStatusCondition`/`meta.FindStatusCondition` helpers (`pkg/effectivenessmonitor/conditions/conditions.go`):

| Condition | Meaning | Reasons |
|-----------|---------|---------|
| `Ready` | Aggregate readiness | `Ready`, `NotReady` |
| `AssessmentComplete` | Assessment reached a terminal state | `AssessmentFull`, `AssessmentPartial`, `AssessmentExpired`, `SpecDrift`, `MetricsTimedOut`, `NoExecution`, `AlertDecayTimeout` |
| `SpecIntegrity` | Whether the post-remediation spec hash is still valid (set every reconcile once `HashComputed=true`, DD-EM-002 v1.1) | `SpecUnchanged`, `SpecDrifted` |
| `AlertDecayDetected` | Whether EM is actively monitoring Prometheus alert decay (BR-EM-012) | `DecayActive`, `DecayResolved`, `DecayTimeout` |
| `PostHashCaptured` | Whether EM successfully captured the post-remediation spec hash (Issue #546) | `PostHashCaptured`, `PostHashCaptureFailed` |

---

## Example: Sync Target (Deployment)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: EffectivenessAssessment
metadata:
  name: rr-a1b2c3d4
  namespace: kubernaut-system
  ownerReferences:
    - apiVersion: kubernaut.ai/v1alpha1
      kind: RemediationRequest
      name: rr-a1b2c3d4
      blockOwnerDeletion: false
spec:
  correlationID: rr-a1b2c3d4
  remediationRequestPhase: Verifying
  signalTarget:
    kind: Deployment
    name: payment-service
    namespace: prod-payment
  remediationTarget:
    kind: Deployment
    name: payment-service
    namespace: prod-payment
  config:
    stabilizationWindow: 5m
  preRemediationSpecHash: "sha256:1a2b3c4d..."
  signalName: KubePodCrashLooping
status:
  phase: Assessing
  validityDeadline: "2026-08-02T14:35:00Z"
  prometheusCheckAfter: "2026-08-02T14:10:00Z"
  alertManagerCheckAfter: "2026-08-02T14:10:00Z"
  components:
    healthAssessed: true
    healthScore: 1.0
    hashComputed: true
    postRemediationSpecHash: "sha256:5e6f7a8b..."
    currentSpecHash: "sha256:5e6f7a8b..."
    alertAssessed: true
    alertScore: 1.0
  conditions:
    - type: SpecIntegrity
      status: "True"
      reason: SpecUnchanged
    - type: PostHashCaptured
      status: "True"
      reason: PostHashCaptured
```

## Example: Async Target (GitOps-managed) — `WaitingForPropagation`

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: EffectivenessAssessment
metadata:
  name: rr-e5f6a7b8
  namespace: kubernaut-system
spec:
  correlationID: rr-e5f6a7b8
  remediationRequestPhase: Completed
  signalTarget:
    kind: Deployment
    name: checkout-service
    namespace: prod-checkout
  remediationTarget:
    kind: Deployment
    name: checkout-service
    namespace: prod-checkout
  config:
    stabilizationWindow: 5m
    hashComputeDelay: 3m   # DD-EM-004: wait for ArgoCD sync before hashing
status:
  phase: WaitingForPropagation
  message: "waiting for GitOps propagation before computing post-remediation spec hash"
```

---

## RBAC

EM's `ClusterRole` (see `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml`) grants:
- `get`, `list`, `watch`, `update`, `patch` on `effectivenessassessments` and `effectivenessassessments/status`
- `get`, `list`, `watch` on target-resource kinds it health-checks (`pods`, `deployments`, `statefulsets`, `daemonsets`, `nodes`, `persistentvolumes`, etc.)
- No write access to any resource other than the EA's own status and Kubernetes Events

---

## Related Documents

- [Overview](./overview.md) — architecture, phases, component scorers
- [Integration Points](./integration-points.md) — upstream/downstream services, audit event contract
- [Observability & Logging](./observability-logging.md)
- [Audit Event Catalog](./security/AUDIT_EVENT_CATALOG.md)
- [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md)
- [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)
