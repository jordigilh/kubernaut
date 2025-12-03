# RO Contract Gaps - Gateway Team

**From**: Remediation Orchestrator Team
**To**: Gateway Team
**Date**: December 1, 2025
**Status**: ✅ **IMPLEMENTED**

---

## Summary

The following gaps affect the `RemediationRequest` CRD schema owned by Gateway. These are **bug fixes** (not design questions) that block RO implementation.

| Gap ID | Issue | Severity | Status |
|--------|-------|----------|--------|
| GAP-C1-02 | Priority enum constraint | 🟠 High | ✅ **DONE** |
| GAP-C1-03 | TargetResource optional | 🟠 High | ✅ **DONE** |
| GAP-C1-04 | Deduplication type | 🟠 High | ✅ **DONE** |

---

## GAP-C1-02: Priority Enum Constraint (BUG FIX)

**File**: `api/remediation/v1alpha1/remediationrequest_types.go:60-62`

**Current** (incorrect):
```go
// +kubebuilder:validation:Enum=P0;P1;P2;P3  // ❌ WRONG
Priority string `json:"priority"`
```

**Required** (correct):
```go
// Priority value provided by Rego policies - no enum enforcement
// Best practice examples: P0 (critical), P1 (high), P2 (normal), P3 (low)
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
Priority string `json:"priority"`
```

**Reason**: Priority values are defined by Rego policies (operator-customizable), not hardcoded enums. P0/P1/P2/P3 are best practice examples only.

---

## GAP-C1-03: TargetResource Must Be Required (BUG FIX)

**File**: `api/remediation/v1alpha1/remediationrequest_types.go:88`

**Current** (incorrect):
```go
TargetResource *ResourceIdentifier `json:"targetResource,omitempty"`  // ❌ WRONG - optional
```

**Required** (correct):
```go
// TargetResource identifies the Kubernetes resource that triggered this signal.
// Populated by Gateway from NormalizedSignal.Resource - REQUIRED for V1.
// Non-Kubernetes signals (AWS, Datadog) will be addressed in V2.
// +kubebuilder:validation:Required
TargetResource ResourceIdentifier `json:"targetResource"`
```

**Reason**: For V1, we only support Kubernetes signals. Gateway is responsible for populating this field. Making it optional creates ambiguity for downstream services.

---

## GAP-C1-04: Deduplication Shared Type (✅ DONE)

**Status**: ✅ **IMPLEMENTED**

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

**Implementation**:
```go
import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

// In RemediationRequestSpec:
Deduplication sharedtypes.DeduplicationInfo `json:"deduplication"`
```

**Changes Made**:
1. ✅ Imported shared type from `pkg/shared/types`
2. ✅ Deleted local `DeduplicationInfo` type
3. ✅ JSON field names changed: `firstSeen` → `firstOccurrence`, `lastSeen` → `lastOccurrence`
4. ✅ Updated all Gateway code (`crd_creator.go`, `crd_updater.go`, `deduplication.go`, `server.go`)
5. ✅ Updated all Gateway tests (unit and integration)

---

## Gateway Team Response

**Date**: December 1, 2025
**Respondent**: Gateway Team (AI-assisted)

### GAP-C1-02 (Priority) - ✅ COMPLETED

**Changes Made**:
- Removed `+kubebuilder:validation:Enum=P0;P1;P2;P3` constraint
- Removed `+kubebuilder:validation:Pattern="^P[0-3]$"` constraint
- Added `+kubebuilder:validation:MinLength=1` and `+kubebuilder:validation:MaxLength=63`
- Priority is now free-text, allowing Rego policies to define values

**Files Changed**:
- `api/remediation/v1alpha1/remediationrequest_types.go`

### GAP-C1-03 (TargetResource) - ✅ COMPLETED

**Changes Made**:
- Changed `TargetResource *ResourceIdentifier` to `TargetResource ResourceIdentifier` (value type)
- Added `+kubebuilder:validation:Required` marker
- Gateway now always populates `TargetResource` from `NormalizedSignal.Resource`
- Default to `Kind: "Unknown"` if no resource info available (edge case)

**Files Changed**:
- `api/remediation/v1alpha1/remediationrequest_types.go`
- `pkg/gateway/processing/crd_creator.go` (new `buildTargetResource` helper)
- Unit tests: `test/unit/gateway/crd_metadata_test.go` (7 new test cases)
- Integration tests: `test/integration/gateway/webhook_integration_test.go` (2 new test cases)

### GAP-C1-04 (Deduplication) - ✅ COMPLETED

**Changes Made**:
- Imported `sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"`
- Replaced local `DeduplicationInfo` with `sharedtypes.DeduplicationInfo`
- Updated all field references: `FirstSeen` → `FirstOccurrence`, `LastSeen` → `LastOccurrence`
- Updated Redis key names in deduplication processing

**Files Changed**:
- `api/remediation/v1alpha1/remediationrequest_types.go`
- `pkg/gateway/processing/crd_creator.go`
- `pkg/gateway/processing/crd_updater.go`
- `pkg/gateway/processing/deduplication.go`
- `pkg/gateway/server.go`
- All related unit and integration tests

### Post-Implementation Verification

```bash
# CRD regeneration completed
make generate && make manifests

# Test results
make test-gateway-unit       # ✅ PASSED (85 specs)
make test-gateway-integration # ✅ PASSED (17 specs)
```

---

## Status: COMPLETE

All three contract gaps have been addressed. RO team can now proceed with implementation.

---

## WE Team Questions ✅ RESOLVED

WE team questions for Gateway have been answered. See:
- **Centralized Q&A**: [`docs/handoff/QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md`](./QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md#questions-for-gateway-team)

### Summary of Resolved Questions

| Question | Answer |
|----------|--------|
| **Namespace for cluster-scoped** | ✅ Empty string (not omitted) |
| **Population source** | ✅ NormalizedSignal.Resource via adapters |
| **Unknown handling** | ✅ Never happens in V1 |

---

**Document Version**: 1.4
**Last Updated**: December 2, 2025
**Migrated From**: `docs/services/stateless/gateway-service/RO_CONTRACT_GAPS.md`
**Changelog**:
- v1.4: Migrated to `docs/handoff/` as authoritative Q&A directory
- v1.3: Gateway team responded. All questions resolved.
- v1.0: Initial RO contract gaps (GAP-C1-02, C1-03, C1-04)


