# Response: Target Resource Schema (RemediationRequest)

**From**: Gateway Service Team
**To**: Remediation Orchestrator Team
**Date**: 2025-12-01
**Status**: ✅ APPROVED

---

## Summary

**Gateway agrees with Option A**: Add `spec.targetResource` as a top-level field in `RemediationRequest.Spec`.

---

## Decision

| Aspect | Decision |
|--------|----------|
| **Approach** | Option A - Top-level `TargetResource` field |
| **ProviderData** | Remove resource info from ProviderData (no duplication) |
| **Backward Compatibility** | Not required (pre-release) |
| **Implementation Owner** | Gateway Team (CRD population) + RO Team (schema change) |

---

## Rationale

1. **Gateway already has the data**: `NormalizedSignal.Resource` contains Kind, Name, Namespace
2. **Trivial implementation**: One struct assignment in `CRDCreator`
3. **Cleaner contract**: Explicit typed field vs JSON parsing
4. **Reduces coupling**: RO doesn't need to understand provider-specific JSON structures
5. **SignalProcessing alignment**: Matches expected contract

---

## Implementation Plan

### Phase 1: Schema Change (RO Team)

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

```go
// Add to RemediationRequestSpec (before ProviderData section)

// ========================================
// TARGET RESOURCE IDENTIFICATION
// ========================================

// TargetResource identifies the Kubernetes resource that triggered this signal.
// Populated by Gateway from NormalizedSignal.Resource.
// Used by SignalProcessing for context enrichment and RO for workflow routing.
TargetResource *ResourceIdentifier `json:"targetResource,omitempty"`
```

**Add type definition**:

```go
// ResourceIdentifier uniquely identifies a Kubernetes resource.
type ResourceIdentifier struct {
    // Kind of the Kubernetes resource (e.g., "Pod", "Deployment", "Node")
    Kind string `json:"kind"`

    // Name of the Kubernetes resource
    Name string `json:"name"`

    // Namespace of the Kubernetes resource (empty for cluster-scoped resources)
    // +optional
    Namespace string `json:"namespace,omitempty"`
}
```

### Phase 2: Gateway Population (Gateway Team)

**File**: `pkg/gateway/processing/crd_creator.go`

**Change in `createRemediationRequest()`**:

```go
// Before (embedded in ProviderData):
ProviderData: c.buildProviderData(signal),

// After (top-level field):
TargetResource: &remediation.ResourceIdentifier{
    Kind:      signal.Resource.Kind,
    Name:      signal.Resource.Name,
    Namespace: signal.Resource.Namespace,
},
ProviderData: c.buildProviderDataWithoutResource(signal), // Remove resource duplication
```

### Phase 3: ProviderData Cleanup (Gateway Team)

**File**: `pkg/gateway/processing/crd_creator.go`

Update `buildProviderData()` to exclude resource info (now in `TargetResource`):

```go
func (c *CRDCreator) buildProviderData(signal *types.NormalizedSignal) []byte {
    // Provider-specific data EXCLUDING resource identifier
    // (resource is now in spec.targetResource)
    providerData := map[string]interface{}{
        "alertname":       signal.AlertName,
        "alertmanagerURL": signal.AlertmanagerURL,
        // ... other provider-specific fields
        // NOTE: "resource" removed - now in spec.targetResource
    }
    // ...
}
```

---

## Timeline

| Task | Owner | Dependency | ETA |
|------|-------|------------|-----|
| 1. Add `ResourceIdentifier` type | RO Team | None | Day 1 |
| 2. Add `TargetResource` field to spec | RO Team | Task 1 | Day 1 |
| 3. Run `make generate` for CRD | RO Team | Task 2 | Day 1 |
| 4. Update Gateway `CRDCreator` | Gateway Team | Task 3 | Day 2 |
| 5. Remove resource from ProviderData | Gateway Team | Task 4 | Day 2 |
| 6. Update unit tests | Both Teams | Task 4-5 | Day 2-3 |
| 7. Update integration tests | Both Teams | Task 6 | Day 3 |

---

## Validation

**Gateway will validate**:
- [ ] `TargetResource` populated for all signal types (Prometheus, K8s Events, etc.)
- [ ] `ProviderData` no longer contains resource duplication
- [ ] Unit tests updated for new field
- [ ] Integration tests verify field presence

**RO will validate**:
- [ ] `TargetResource` accessible without JSON parsing
- [ ] SignalProcessing can read field directly
- [ ] Workflow routing uses `TargetResource.Kind`

---

## Questions Resolved

| Question | Answer |
|----------|--------|
| Remove resource from ProviderData? | ✅ Yes - no duplication needed |
| Backward compatibility? | ✅ Not required (pre-release) |
| Who implements schema? | RO Team (owns CRD) |
| Who populates field? | Gateway Team |

---

## Contact

**Gateway Team Lead**: [Assigned]
**Implementation Start**: Upon RO schema merge

