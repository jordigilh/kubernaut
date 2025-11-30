# SignalProcessing Service - Handoff Request from AIAnalysis Team

**Date**: 2025-11-29
**From**: AIAnalysis Service Team
**To**: SignalProcessing Service Team
**Priority**: 🔴 HIGH (Blocking AIAnalysis Implementation)

---

## Summary

During AIAnalysis service design, we identified **two blocking issues** in the SignalProcessing CRD that need to be addressed for proper upstream integration.

---

## Issue 1: CRD Rename Required

### Current State
- Go package: `api/remediationprocessing/v1alpha1/`
- Type names: `RemediationProcessing`, `RemediationProcessingSpec`, `RemediationProcessingStatus`

### Required State (per official naming)
- Go package: `api/signalprocessing/v1alpha1/`
- Type names: `SignalProcessing`, `SignalProcessingSpec`, `SignalProcessingStatus`

### Files to Update
```
api/remediationprocessing/v1alpha1/remediationprocessing_types.go
api/remediationprocessing/v1alpha1/groupversion_info.go
api/remediationprocessing/v1alpha1/zz_generated.deepcopy.go
```

### References to Update
- `pkg/gateway/processing/deduplication.go`
- `pkg/gateway/types/types.go`
- `api/remediation/v1alpha1/remediationrequest_types.go`
- `api/aianalysis/v1alpha1/aianalysis_types.go`

### Related Documentation
- [DD-SIGNAL-PROCESSING-001](../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md)
- [ADR-015: Alert to Signal Naming Migration](../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

---

## Issue 2: Unstructured Data in Status (Anti-Pattern)

### Current State
```go
// api/remediationprocessing/v1alpha1/remediationprocessing_types.go
type RemediationProcessingStatus struct {
    Phase       string             `json:"phase,omitempty"`
    ContextData map[string]string  `json:"contextData,omitempty"` // ❌ Unstructured
    StartTime   *metav1.Time       `json:"startTime,omitempty"`
    CompletedAt *metav1.Time       `json:"completedAt,omitempty"`
}
```

### Problem
- `ContextData map[string]string` loses type safety
- AIAnalysis needs structured `KubernetesContext` and `HistoricalContext` for HolmesGPT-API
- Project policy: No unstructured data in CRDs unless vetted

### Required State (per DD-CONTRACT-002)
```go
type SignalProcessingStatus struct {
    Phase             string             `json:"phase,omitempty"`
    EnrichmentResults EnrichmentResults  `json:"enrichmentResults,omitempty"` // ✅ Structured
    StartTime         *metav1.Time       `json:"startTime,omitempty"`
    CompletedAt       *metav1.Time       `json:"completedAt,omitempty"`
}

type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"` // 0.0-1.0
}

type KubernetesContext struct {
    Namespace         string            `json:"namespace"`
    NamespaceLabels   map[string]string `json:"namespaceLabels,omitempty"`
    PodDetails        *PodDetails       `json:"podDetails,omitempty"`
    DeploymentDetails *DeploymentDetails `json:"deploymentDetails,omitempty"`
    NodeDetails       *NodeDetails      `json:"nodeDetails,omitempty"`
}

type HistoricalContext struct {
    PreviousSignals        int     `json:"previousSignals"`
    LastSignalTimestamp    string  `json:"lastSignalTimestamp,omitempty"`
    SignalFrequency        float64 `json:"signalFrequency"` // signals per hour
    ResolutionSuccessRate  float64 `json:"resolutionSuccessRate"` // 0.0-1.0
}
```

### Reference
The full structured types are already documented in:
- `docs/services/crd-controllers/01-signalprocessing/crd-schema.md` (lines 192-300)

---

## Impact on AIAnalysis

Without these changes:
1. ❌ AIAnalysis cannot receive structured Kubernetes context from SignalProcessing
2. ❌ HolmesGPT-API receives degraded context, reducing investigation quality
3. ❌ Type mismatches between documented contracts and implementation

---

## Requested Actions

### Action 1: Rename CRD (Priority: HIGH)
- [ ] Rename `api/remediationprocessing/` → `api/signalprocessing/`
- [ ] Update all type names: `RemediationProcessing*` → `SignalProcessing*`
- [ ] Update references in `pkg/gateway/`, `api/remediation/`, `api/aianalysis/`
- [ ] Regenerate `zz_generated.deepcopy.go`

### Action 2: Restructure Status (Priority: HIGH)
- [ ] Replace `ContextData map[string]string` with structured `EnrichmentResults`
- [ ] Add `KubernetesContext` and `HistoricalContext` types (from crd-schema.md)
- [ ] Update controller to populate structured types

### Action 3: Verify Contract Alignment
- [ ] Review `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md`
- [ ] Ensure status output matches AIAnalysis input expectations

---

## Timeline

**Requested Completion**: Before AIAnalysis implementation begins

**Blocking**: AIAnalysis schema updates depend on SignalProcessing providing structured context data.

---

## Questions?

If you have questions about:
- **AIAnalysis input requirements**: See `DD-CONTRACT-002`
- **Structured type definitions**: See `crd-schema.md` lines 192-300
- **Naming conventions**: See `ADR-015` and `DD-SIGNAL-PROCESSING-001`

---

## Contact

For clarification, please reach out to the AIAnalysis team.


