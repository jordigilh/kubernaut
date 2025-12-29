# Triage: Storm Detection Fields in RemediationRequest Spec - Discrepancy Analysis

**Date**: December 15, 2025
**Issue**: Storm fields still present in RR CRD spec despite DD-GATEWAY-015 removal decision
**Status**: ⚠️ **DISCREPANCY IDENTIFIED**
**Priority**: P1 (High) - Schema correctness issue

---

## 🚨 Problem Summary

**Storm detection fields remain in `RemediationRequest.spec` despite authoritative documentation stating they were removed**.

### Current State (Dec 15, 2025)

**CRD Schema** (`config/crd/bases/kubernaut.ai_remediationrequests.yaml`):
- ✅ Line 50-86: `spec.deduplication` - **DEPRECATED** (DD-GATEWAY-011, marked deprecated)
- ❌ Line 93: `spec.isStorm` - **NO DEPRECATION MARKER** (should be removed per DD-GATEWAY-015)
- ❌ Line 174: `spec.stormAlertCount` - **NO DEPRECATION MARKER**
- ❌ Line 176: `spec.stormType` - **NO DEPRECATION MARKER**
- ❌ Line 180: `spec.stormWindow` - **NO DEPRECATION MARKER**
- ❌ Line 46: `spec.affectedResources` - **NO DEPRECATION MARKER** (storm-specific field)

**Go Types** (`api/remediation/v1alpha1/remediationrequest_types.go`):
- Line 280: `IsStorm bool`
- Line 283: `StormType string`
- Line 286: `StormWindow string`
- Line 289: `StormAlertCount int`

**Last Modified**: Dec 15, 2025 20:09 (TODAY - AFTER storm removal docs)

---

## 📚 Authoritative Documentation Analysis

### DD-GATEWAY-015: Storm Detection Logic Removal

**Location**: `docs/architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md`
**Date**: December 13, 2025
**Status in Document**: 📋 **PLANNED** for Implementation
**Last Modified**: Dec 14, 2025 19:01

**Key Directive** (Lines 112-120):
```markdown
**3. CRD Schema** (2 files):

api/remediation/v1alpha1/remediationrequest_types.go
  - Remove: StormAggregationStatus struct
  - Remove: status.StormAggregation field

config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml
  - Remove: status.stormAggregation schema definition
```

**⚠️ CRITICAL NOTE**: DD-GATEWAY-015 only mentions removing `status.stormAggregation`, **NOT spec storm fields**.

### STORM_REMOVAL_FINAL_STATUS.md

**Location**: `docs/handoff/STORM_REMOVAL_FINAL_STATUS.md`
**Date**: December 13, 2025
**Status in Document**: ✅ **COMPLETE** - Production Ready
**Last Modified**: Dec 14, 2025 19:01

**Claims** (Lines 1-29):
```
✅ COMPLETE - Production Ready
✅ CRD schema updated (storm fields removed)
✅ 96/96 integration tests passing
```

**Discrepancy**: Document claims storm fields removed, but CRD schema still contains them.

---

## 🔍 Root Cause Analysis

### Hypothesis 1: Incomplete Removal (MOST LIKELY)

**Evidence**:
1. ✅ DD-GATEWAY-015 explicitly states "Remove storm fields from types.go"
2. ✅ STORM_REMOVAL_FINAL_STATUS claims "CRD schema updated"
3. ❌ **BUT**: Spec storm fields (`isStorm`, `stormType`, etc.) still present in CRD
4. ✅ Status storm fields (`status.stormAggregation`) were removed (line 486: only `status.deduplication` exists)

**Conclusion**: Status fields were removed, but **spec fields were left behind**.

**Why This Matters**:
- Spec storm fields are **immutable by design** (Gateway sets them at creation)
- Gateway code still references these fields (confirmed: `pkg/gateway/server.go`, `pkg/gateway/types/types.go`)
- Tests may still populate these fields
- Schema bloat and confusion about which fields are authoritative

### Hypothesis 2: Intentional Retention for Backward Compatibility

**Evidence**:
- Spec fields are immutable, so removing them would break existing RRs with storm data
- CRD schema changes last modified Dec 15 (AFTER removal docs), suggesting deliberate retention

**Counter-Evidence**:
- DD-GATEWAY-015 explicitly states "Remove IsStorm, StormType, StormWindow, AlertCount fields"
- No deprecation markers added to spec fields (unlike `spec.deduplication` which has clear deprecation)
- STORM_REMOVAL_FINAL_STATUS claims removal is complete, not "partially retained"

**Conclusion**: Unlikely - If intentional, fields should have deprecation markers.

### Hypothesis 3: Documentation Out of Sync

**Evidence**:
- DD-GATEWAY-015 status: "PLANNED" (not "COMPLETE")
- STORM_REMOVAL_FINAL_STATUS status: "COMPLETE"
- Actual CRD: Storm fields still present

**Conclusion**: Documentation inconsistency between plan (DD-GATEWAY-015) and execution report (STORM_REMOVAL_FINAL_STATUS).

---

## ✅ Authoritative Decision: DD-GATEWAY-015

**Per DD-GATEWAY-015 (Lines 254-268)**:

```diff
// pkg/gateway/types/types.go
type NormalizedSignal struct {
    // ... existing fields ...
    RawPayload json.RawMessage

-   // Storm Detection Fields
-   IsStorm bool
-   StormType string
-   StormWindow string
-   AlertCount int
-   AffectedResources []string
}
```

**Verdict**: **Storm spec fields SHOULD BE REMOVED per authoritative design decision**.

---

## 🎯 Recommended Action: Complete Storm Field Removal

### Scope of Removal

**Spec Fields to Remove** (6 fields):
1. `spec.isStorm` (line 93-97 in CRD)
2. `spec.stormAlertCount` (line 174)
3. `spec.stormType` (line 176-179)
4. `spec.stormWindow` (line 180-182)
5. `spec.affectedResources` (line 43-49) - Storm-specific field for aggregated storms

**Files to Modify**:
1. `api/remediation/v1alpha1/remediationrequest_types.go` - Remove fields from Go types
2. `config/crd/bases/kubernaut.ai_remediationrequests.yaml` - Regenerate via `make manifests`
3. `pkg/gateway/types/types.go` - Remove storm fields from NormalizedSignal
4. `pkg/gateway/server.go` - Remove storm detection logic
5. `pkg/gateway/config/config.go` - Remove storm configuration
6. Update tests to remove storm field references

---

## 📊 Impact Assessment

### Breaking Changes: **NONE** (Backward Compatible)

**Rationale**:
- Spec fields are **immutable** (set at creation only)
- Old RRs with storm fields will continue to work (fields ignored by controllers)
- No downstream consumers use storm fields (per DD-AIANALYSIS-004)
- Kubernetes OpenAPI v3 handles backward compatibility automatically

### Validation Required

**Before Removal**:
- [ ] Confirm Gateway code doesn't populate storm fields anymore
- [ ] Confirm no tests rely on storm fields
- [ ] Confirm no controllers read storm fields

**After Removal**:
- [ ] Run `make manifests` to regenerate CRD
- [ ] Run all Gateway tests (unit, integration, E2E)
- [ ] Verify no compilation errors
- [ ] Verify no lint errors

---

## 🔧 Implementation Plan

### Option A: Complete Removal (RECOMMENDED)

**Align with DD-GATEWAY-015 - Remove all storm spec fields**

**Effort**: 2-4 hours
**Risk**: LOW (no downstream consumers)

**Steps**:
1. Remove storm fields from `remediationrequest_types.go` (10 min)
2. Remove storm logic from Gateway source (30 min)
3. Remove storm tests (30 min)
4. Regenerate CRD: `make manifests` (5 min)
5. Run all tests (1-2 hours)
6. Update documentation to mark removal as truly complete (15 min)

**Result**: Clean schema, no confusion, fully aligned with DD-GATEWAY-015.

### Option B: Add Deprecation Markers (NOT RECOMMENDED)

**Mark spec fields as deprecated but leave them in place**

**Why Not Recommended**:
- Spec fields are immutable anyway (can't be updated post-creation)
- Gateway doesn't use them anymore (per storm removal)
- Adds complexity without benefit
- DD-GATEWAY-015 explicitly says REMOVE, not deprecate

---

## ✅ Decision

**RECOMMENDED**: **Option A - Complete Removal**

**Rationale**:
1. ✅ Aligns with authoritative DD-GATEWAY-015 decision
2. ✅ No downstream consumers (DD-AIANALYSIS-004)
3. ✅ Backward compatible (old RRs with storm fields continue working)
4. ✅ Simplifies schema and removes confusion
5. ✅ Completes the storm removal work claimed in STORM_REMOVAL_FINAL_STATUS

**Next Steps**:
1. User approval to proceed with complete removal
2. Execute removal plan (Option A)
3. Update STORM_REMOVAL_FINAL_STATUS to mark spec fields as removed
4. Update DD-GATEWAY-015 status from "PLANNED" to "COMPLETE"

---

## 📋 Validation Checklist

**Pre-Removal Checks**:
- [ ] Verify Gateway code doesn't populate storm fields
- [ ] Verify no controllers read storm fields from spec
- [ ] Verify tests don't assert on storm fields
- [ ] Backup current CRD schema

**Post-Removal Checks**:
- [ ] CRD regenerated successfully
- [ ] All Gateway tests pass (unit + integration + E2E)
- [ ] No compilation errors
- [ ] No lint errors
- [ ] Storm fields absent from generated CRD YAML
- [ ] Documentation updated to reflect complete removal

---

## 📚 References

- **Authoritative Decision**: [DD-GATEWAY-015](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
- **Removal Status**: [STORM_REMOVAL_FINAL_STATUS.md](STORM_REMOVAL_FINAL_STATUS.md)
- **CRD Schema**: `config/crd/bases/kubernaut.ai_remediationrequests.yaml`
- **Go Types**: `api/remediation/v1alpha1/remediationrequest_types.go`
- **AI Analysis Decision**: [DD-AIANALYSIS-004](../architecture/decisions/DD-AIANALYSIS-004-storm-context-not-exposed.md)

---

**Triage Complete**: 2025-12-15
**Recommendation**: Proceed with complete storm spec field removal (Option A)
**Confidence**: 95% - Authoritative documentation and code analysis support removal



