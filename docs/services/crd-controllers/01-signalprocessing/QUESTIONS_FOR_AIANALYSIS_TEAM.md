# Questions for AIAnalysis Team

**From**: SignalProcessing Team
**Date**: December 1, 2025
**Status**: ⏳ Awaiting Response

---

## Context

SignalProcessing passes enrichment data to AIAnalysis. We want to confirm the integration path and data ownership.

---

## Questions

### Q1: Enrichment Data Path Confirmation

**Context**: Per our previous validation (RESPONSE_SIGNALPROCESSING_INTEGRATION_VALIDATION.md), AIAnalysis reads enrichment data from:

```
spec.analysisRequest.signalContext.enrichmentResults.*
```

**Question**: Can you confirm this is still the correct path? We want to ensure our integration tests target the right location.

**Fields we're passing**:
- `ownerChain` - Kubernetes ownership hierarchy
- `detectedLabels` - Auto-detected cluster characteristics
- `customLabels` - User-defined labels from Rego

**Options**:
- [ ] A) Confirmed - path is correct
- [ ] B) Path changed to: _______________
- [ ] C) Different fields at different paths - provide mapping

---

### Q2: DetectedLabels Usage

**Context**: SignalProcessing auto-detects cluster characteristics:

```go
type DetectedLabels struct {
    IsGitOpsManaged    bool   // ArgoCD/Flux detected
    GitOpsTool         string // "argocd", "flux", ""
    HasPDB             bool   // PodDisruptionBudget exists
    HasHPA             bool   // HorizontalPodAutoscaler exists
    IsStatefulSet      bool   // StatefulSet workload
    IsHelmManaged      bool   // Helm release detected
    HasNetworkPolicy   bool   // NetworkPolicy applies
    PodSecurityStandard string // "privileged", "baseline", "restricted"
    IsServiceMesh      bool   // Istio/Linkerd detected
    ServiceMeshType    string // "istio", "linkerd", ""
}
```

**Question**: Does AIAnalysis use all these fields, or only a subset? This helps us prioritize detection accuracy.

**Options**:
- [ ] A) All fields are used
- [ ] B) Only these fields: _______________
- [ ] C) Currently unused - future feature
- [ ] D) Passed through to HolmesGPT only - AIAnalysis doesn't interpret

---

### Q3: Missing Enrichment Data Behavior

**Context**: SignalProcessing might fail to detect certain characteristics (e.g., RBAC denies PDB access).

**Question**: How should AIAnalysis handle missing/nil enrichment fields?

**SignalProcessing Behavior**:
- If detection fails: field is `nil`/`false`/empty
- If detection succeeds with negative result: field is explicitly `false`/empty

**Options**:
- [ ] A) Treat missing as "unknown" - different from explicit `false`
- [ ] B) Treat missing as `false` - same behavior
- [ ] C) Fail/degrade AIAnalysis - enrichment is required
- [ ] D) Doesn't matter - HolmesGPT handles gracefully

---

## Response Section

### AIAnalysis Team Response

**Date**:
**Respondent**:

**Q1 (Data path)**:
- [ ] Option A / B / C
- Notes:

**Q2 (DetectedLabels usage)**:
- [ ] Option A / B / C / D
- Notes:

**Q3 (Missing data behavior)**:
- [ ] Option A / B / C / D
- Notes:

---

**SignalProcessing Team**


