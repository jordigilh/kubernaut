# Gateway Team Questions for SignalProcessing Team

**From**: Gateway Team
**To**: SignalProcessing Team
**Date**: December 1, 2025
**Status**: ✅ RESOLVED
**Context**: Follow-up to RO_CONTRACT_GAPS implementation

---

## Summary

Gateway has completed implementation of contract gaps related to `DeduplicationInfo` schema alignment. The following question requires SignalProcessing team confirmation.

---

## Q5: SignalProcessing DeduplicationInfo Alignment

**Question**: Will SignalProcessing also adopt the shared `DeduplicationInfo` type?

**Background**: RO team requested that Gateway create a shared `DeduplicationInfo` type to align schemas across services. Gateway has implemented this.

**Current Shared Type Location**: `pkg/shared/types/deduplication.go`

```go
package types

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DeduplicationInfo tracks duplicate signal suppression
// Shared between RemediationRequest and SignalProcessing CRDs
type DeduplicationInfo struct {
    // True if this signal is a duplicate of an active remediation
    IsDuplicate bool `json:"isDuplicate,omitempty"`

    // Timestamp when this signal fingerprint was first seen
    FirstOccurrence metav1.Time `json:"firstOccurrence"`

    // Timestamp when this signal fingerprint was last seen
    LastOccurrence metav1.Time `json:"lastOccurrence"`

    // Total count of occurrences of this signal
    OccurrenceCount int `json:"occurrenceCount"`

    // Optional correlation ID for grouping related signals
    CorrelationID string `json:"correlationId,omitempty"`

    // Reference to previous RemediationRequest CRD (if duplicate)
    PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}
```

**JSON Field Name Changes**:
| Old Field | New Field |
|-----------|-----------|
| `firstSeen` | `firstOccurrence` |
| `lastSeen` | `lastOccurrence` |

**Options**:
- [ ] A) Yes - SignalProcessing will adopt the shared type from `pkg/shared/types/`
- [ ] B) No - SignalProcessing will keep its own local definition (please explain why): _______________
- [ ] C) Need modifications to shared type before adopting (please specify): _______________
- [ ] D) Need to discuss field name differences: _______________

---

## Additional Context

### Services Currently Using DeduplicationInfo

| Service | Location | Status |
|---------|----------|--------|
| Gateway (RemediationRequest) | `api/remediation/v1alpha1/` | ✅ Using shared type |
| SignalProcessing | `api/signalprocessing/v1alpha1/` | ✅ Using shared type |

### Benefits of Shared Type

1. **Single Source of Truth**: Changes propagate to all consumers automatically
2. **Schema Alignment**: RO can process both CRDs with identical deduplication handling
3. **Reduced Duplication**: No need to maintain parallel type definitions
4. **Type Safety**: Go compiler catches mismatches at build time

---

## Response Template

Please respond below:

### SignalProcessing Team Response

**Date**: December 1, 2025
**Respondent**: SignalProcessing Team

| Question | Response | Notes |
|----------|----------|-------|
| Q5 (Adopt Shared Type) | **A** ✅ | Already implemented - see details below |

**Additional Comments**:

#### ✅ Implementation Complete

SignalProcessing has **already adopted** the shared `DeduplicationInfo` type as part of the RO contract gaps resolution (GAP-C1-03/04).

**Changes Made**:

1. **Import Added** (`api/signalprocessing/v1alpha1/signalprocessing_types.go`):
   ```go
   import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
   ```

2. **Field Updated** (in `SignalProcessingSpec`):
   ```go
   // Before:
   Deduplication DeduplicationContext `json:"deduplication,omitempty"`

   // After:
   Deduplication sharedtypes.DeduplicationInfo `json:"deduplication,omitempty"`
   ```

3. **Local Type Removed**: The local `DeduplicationContext` struct was deleted.

4. **DeepCopy Functions**: Created `pkg/shared/types/zz_generated.deepcopy.go` for CRD generation support.

5. **CRD Regenerated**: `make generate manifests` completed successfully.

**Commit Reference**:
```
fix(signalprocessing): Resolve RO contract gaps GAP-C1-01/02/05/06
```

#### JSON Field Mapping Confirmation

| Gateway Field | SignalProcessing Field | Status |
|---------------|------------------------|--------|
| `firstOccurrence` | `firstOccurrence` | ✅ Aligned |
| `lastOccurrence` | `lastOccurrence` | ✅ Aligned |
| `correlationId` | `correlationId` | ✅ Aligned |
| `isDuplicate` | `isDuplicate` | ✅ Aligned |
| `occurrenceCount` | `occurrenceCount` | ✅ Aligned |
| `previousRemediationRequestRef` | `previousRemediationRequestRef` | ✅ Aligned |

#### Open Item for RO Team

We have a question for RO about the `correlationId` field casing (lowercase 'd' vs uppercase 'D'). See `QUESTIONS_FOR_RO_TEAM.md` Q1.

---

## Timeline

- **Gateway Implementation**: Complete (December 1, 2025)
- **SP Response**: Complete (December 1, 2025) ✅
- **Coordinated Release**: Not needed - both services already aligned

---

## Reference

- **Related Documents**:
  - `docs/services/stateless/gateway-service/RO_CONTRACT_GAPS.md` (Gateway implementation)
  - `pkg/shared/types/deduplication.go` (shared type definition)
  - `docs/services/crd-controllers/01-signalprocessing/RO_CONTRACT_GAPS.md` (SP contract gaps)

