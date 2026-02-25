# ⚠️ NOTICE: AIAnalysis CRD Path Correction

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed: DetectedLabels is now computed by HAPI post-RCA (ADR-056), and OwnerChain is resolved via get_resource_context (ADR-055).

**Date**: December 2, 2025
**From**: AIAnalysis Service Team
**To**: SignalProcessing Team, Remediation Orchestrator Team
**Priority**: 🔴 HIGH - Please read before implementation

---

## Summary

The enrichment data paths in AIAnalysis CRD are **different than previously communicated**. Please use the **correct paths** below.

---

## ❌ INCORRECT Paths (Do NOT Use)

```
spec.signalContext.ownerChain
spec.signalContext.detectedLabels
spec.signalContext.customLabels
```

---

## ✅ CORRECT Paths (Use These)

```
spec.analysisRequest.signalContext.enrichmentResults.ownerChain
spec.analysisRequest.signalContext.enrichmentResults.detectedLabels
spec.analysisRequest.signalContext.enrichmentResults.customLabels
```

---

## Full Mapping Table

| Field | Correct AIAnalysis Path |
|-------|------------------------|
| `ownerChain` | `spec.analysisRequest.signalContext.enrichmentResults.ownerChain` |
| `detectedLabels` | `spec.analysisRequest.signalContext.enrichmentResults.detectedLabels` |
| `customLabels` | `spec.analysisRequest.signalContext.enrichmentResults.customLabels` |
| `kubernetesContext` | `spec.analysisRequest.signalContext.enrichmentResults.kubernetesContext` |
| `enrichmentQuality` | `spec.analysisRequest.signalContext.enrichmentResults.enrichmentQuality` |

---

## RO Implementation Impact

When RO creates AIAnalysis CRD, copy from SignalProcessing using this mapping:

```go
// Correct mapping
aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
    KubernetesContext: signalProcessing.Status.EnrichmentResults.KubernetesContext,
    DetectedLabels:    signalProcessing.Status.EnrichmentResults.DetectedLabels,
    OwnerChain:        signalProcessing.Status.EnrichmentResults.OwnerChain,
    CustomLabels:      signalProcessing.Status.EnrichmentResults.CustomLabels,
    EnrichmentQuality: signalProcessing.Status.EnrichmentResults.EnrichmentQuality,
}
```

---

## Why the Difference?

The AIAnalysis CRD uses a nested structure for modularity:

```
AIAnalysisSpec
├── remediationRequestRef
├── remediationId
├── analysisRequest          ← Wrapper for analysis input
│   ├── signalContext        ← Signal-specific context
│   │   ├── fingerprint
│   │   ├── severity
│   │   ├── signalType
│   │   ├── environment
│   │   ├── businessPriority
│   │   ├── targetResource
│   │   └── enrichmentResults    ← Enrichment data HERE
│   │       ├── kubernetesContext
│   │       ├── detectedLabels   ✅
│   │       ├── ownerChain       ✅
│   │       ├── customLabels     ✅
│   │       └── enrichmentQuality
│   └── analysisTypes
├── isRecoveryAttempt [Deprecated - Issue #180]
├── recoveryAttemptNumber [Deprecated - Issue #180]
└── previousExecutions
```

---

## Authoritative Source

**File**: `api/aianalysis/v1alpha1/aianalysis_types.go`

This is the **single source of truth** for AIAnalysis CRD schema.

---

## Team Acknowledgments

### ✅ RO Team Acknowledgment (December 2, 2025)

**Status**: ✅ **ACKNOWLEDGED AND CONFIRMED**

The Remediation Orchestrator team confirms:
1. **Correct paths understood** - Will use the nested `spec.analysisRequest.signalContext.enrichmentResults.*` paths
2. **Implementation already correct** - RO's Q1 response in `AIANALYSIS_TO_RO_TEAM.md` uses the correct paths

**Verification**:
```go
// RO Implementation (AIANALYSIS_TO_RO_TEAM.md Q1) - ✅ CORRECT
aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = &sharedtypes.EnrichmentResults{
    KubernetesContext: sp.Status.EnrichmentResults.KubernetesContext,
    DetectedLabels:    sp.Status.EnrichmentResults.DetectedLabels,    // ✅ CORRECT PATH
    OwnerChain:        sp.Status.EnrichmentResults.OwnerChain,        // ✅ CORRECT PATH
    CustomLabels:      sp.Status.EnrichmentResults.CustomLabels,      // ✅ CORRECT PATH
    EnrichmentQuality: sp.Status.EnrichmentResults.EnrichmentQuality,
}
```

**Cross-verified against**: `api/aianalysis/v1alpha1/aianalysis_types.go` ✅

---

### ✅ SignalProcessing Team Acknowledgment (December 2, 2025)

**Status**: ✅ **ACKNOWLEDGED AND CONFIRMED**

The SignalProcessing team confirms:
1. **Correct paths understood** - Will populate `status.enrichmentResults.*` which RO copies to the nested AIAnalysis path
2. **No implementation impact** - SignalProcessing writes to its own CRD status; RO handles the mapping to AIAnalysis CRD

**Data Flow Confirmed**:
```
SignalProcessing.Status.EnrichmentResults
        │
        ▼ (RO copies to)
AIAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults
```

**SignalProcessing populates**:
- `status.enrichmentResults.ownerChain`
- `status.enrichmentResults.detectedLabels`
- `status.enrichmentResults.customLabels`
- `status.enrichmentResults.kubernetesContext`

**Cross-verified against**: `api/aianalysis/v1alpha1/aianalysis_types.go` ✅

---

## Questions?

Contact AIAnalysis team if you need clarification.

---

**AIAnalysis Service Team**

