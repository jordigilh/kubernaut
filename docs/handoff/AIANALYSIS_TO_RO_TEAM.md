# Questions from AIAnalysis Team

**From**: AIAnalysis Service Team
**To**: Remediation Orchestrator (RO) Team
**Date**: December 1, 2025
**Context**: Follow-up after resolving RO_CONTRACT_GAPS.md

---

## Background

We've resolved all contract gaps identified in `docs/services/crd-controllers/02-aianalysis/RO_CONTRACT_GAPS.md`:

| Gap | Resolution |
|-----|------------|
| GAP-C3-01 | `Environment` changed to free-text validation |
| GAP-C3-02 | `RiskTolerance` field **removed** |
| GAP-C3-03 | `BusinessCategory` field **removed** |
| GAP-C3-04 | Shared types package created at `pkg/shared/types/enrichment.go` |

---

## Questions

### Q1: Field Population After Removal

With `RiskTolerance` and `BusinessCategory` removed from `SignalContextInput`, these values are now **only** available in `CustomLabels` (extracted by SignalProcessing via Rego policies).

**Previous approach** (no longer valid):
```go
aiAnalysis.Spec.AnalysisRequest.SignalContext.RiskTolerance = "low"        // ❌ REMOVED
aiAnalysis.Spec.AnalysisRequest.SignalContext.BusinessCategory = "payments" // ❌ REMOVED
```

**New approach**:
```go
// RO should pass CustomLabels directly from SignalProcessing status
aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.CustomLabels =
    signalProcessing.Status.EnrichmentResults.CustomLabels

// CustomLabels structure (per DD-WORKFLOW-001 v1.4):
// {
//   "constraint": ["risk-tolerance=low", "cost-constrained"],
//   "business": ["category=payments"],
//   "team": ["name=platform"]
// }
```

**Question**: Is this mapping clear? Do you need us to document this more explicitly in a handoff document?

---

### Q2: Shared Types Import

We created `pkg/shared/types/enrichment.go` as the **authoritative source** for enrichment types. Both AIAnalysis and SignalProcessing now import from this package.

**If RO references enrichment types directly**, you should import:

```go
import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

// Use: sharedtypes.EnrichmentResults, sharedtypes.DetectedLabels, etc.
```

**If RO only reads from SignalProcessing status and writes to AIAnalysis spec**, the type aliases in both packages will work transparently:

```go
// This still works - aliases resolve to shared types
spStatus := signalProcessing.Status.EnrichmentResults  // signalprocessingv1alpha1.EnrichmentResults
aiSpec.EnrichmentResults = spStatus                     // aianalysisv1alpha1.EnrichmentResults
```

**Question**: Which approach does RO use? Do you need to update your controller code, or does the alias approach work for your use case?

---

### Q3: Environment and Priority Free-Text

`Environment` and `BusinessPriority` are now free-text fields (no enum validation). RO should pass whatever values come from SignalProcessing/RemediationRequest.

**Previous** (enum-constrained):
```go
// +kubebuilder:validation:Enum=production;staging;development
Environment string `json:"environment"`
```

**New** (free-text):
```go
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Environment string `json:"environment"`
```

**Question**: Any concerns with this change? RO should now accept any environment string (e.g., `prod`, `uat`, `qa-eu`, `canary`).

---

## RO Team Response

**Date**: December 2, 2025
**Respondent**: Remediation Orchestrator Team
**Status**: ✅ **ALL QUESTIONS ANSWERED**

---

### Q1 (CustomLabels mapping):
- [x] ✅ **Clear - no additional documentation needed**

**Details**:
- RO passes `SignalProcessing.status.enrichmentResults.customLabels` directly to `AIAnalysis.spec.analysisRequest.signalContext.enrichmentResults.customLabels`
- This is a pass-through operation - RO does not transform or interpret the values
- `RiskTolerance` and `BusinessCategory` are now extracted by operators via Rego policies and exist in CustomLabels (e.g., `{"constraint": ["risk-tolerance=low"], "business": ["category=payments"]}`)
- HolmesGPT-API uses CustomLabels for workflow filtering, per DD-WORKFLOW-001 v1.4

**RO Implementation**:
```go
aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = &sharedtypes.EnrichmentResults{
    KubernetesContext: sp.Status.EnrichmentResults.KubernetesContext,
    DetectedLabels:    sp.Status.EnrichmentResults.DetectedLabels,
    OwnerChain:        sp.Status.EnrichmentResults.OwnerChain,
    CustomLabels:      sp.Status.EnrichmentResults.CustomLabels, // Pass-through
    EnrichmentQuality: sp.Status.EnrichmentResults.EnrichmentQuality,
}
```

---

### Q2 (Shared types import):
- [x] ✅ **We only read/write CRD specs - aliases work fine**

**Details**:
- RO reads from `SignalProcessing.status` and writes to `AIAnalysis.spec`
- Both CRDs use type aliases that resolve to `pkg/shared/types/enrichment.go`
- No direct manipulation of enrichment types needed in RO controller code
- Type aliases are transparent at runtime - Go treats them as the same type

**No code changes required in RO** - the alias approach provides seamless compatibility:
```go
// This works because both are aliases to sharedtypes.EnrichmentResults
spStatus := signalProcessing.Status.EnrichmentResults  // signalprocessingv1alpha1.EnrichmentResults
aiSpec.EnrichmentResults = spStatus                     // aianalysisv1alpha1.EnrichmentResults
```

---

### Q3 (Free-text environment/priority):
- [x] ✅ **No concerns**

**Details**:
- RO treats `Environment` and `BusinessPriority` as opaque strings
- RO does not validate or constrain these values
- Values flow through from RemediationRequest → SignalProcessing → AIAnalysis unchanged
- Operators define their own environment naming conventions via Rego policies
- Examples: `prod`, `production`, `qa-eu`, `canary`, `pre-prod` - all valid

**RO Implementation**: Simple pass-through without validation:
```go
aiAnalysis.Spec.AnalysisRequest.SignalContext.Environment = sp.Status.Environment
aiAnalysis.Spec.AnalysisRequest.SignalContext.BusinessPriority = sp.Status.Priority
```

---

## ✅ Summary

| Question | Answer | Status |
|----------|--------|--------|
| Q1 (CustomLabels) | Clear - pass-through | ✅ Resolved |
| Q2 (Shared types) | Aliases work fine | ✅ Resolved |
| Q3 (Free-text) | No concerns | ✅ Resolved |

---

## References

- `docs/services/crd-controllers/02-aianalysis/RO_CONTRACT_GAPS.md` - Original gaps (now resolved)
- `pkg/shared/types/enrichment.go` - Authoritative enrichment types
- `docs/architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md` v1.4 - CustomLabels design

