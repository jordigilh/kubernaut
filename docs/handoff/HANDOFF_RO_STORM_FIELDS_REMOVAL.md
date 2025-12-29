# Handoff to RO Team: Storm Detection Fields Removal from RemediationRequest Spec

**Date**: December 15, 2025
**From**: Gateway Team
**To**: Remediation Orchestrator (RO) Team
**Priority**: P2 (Medium) - Schema cleanup for V1.0 polish
**Effort**: 2-4 hours
**Risk**: LOW - No business logic impact

---

## 🎯 Executive Summary

Storm detection was removed from Gateway Service per **DD-GATEWAY-015** (December 13, 2025). However, storm fields remain in the `RemediationRequest.spec` schema. These fields should be removed to maintain schema cleanliness and prevent confusion.

**Current State**: Storm `status` fields removed ✅, Storm `spec` fields remain ❌
**Desired State**: All storm fields removed from RR CRD
**Impact**: Zero - No controllers read/write these fields anymore

---

## 📋 Background

### Why Storm Detection Was Removed

**Authoritative Decision**: [DD-GATEWAY-015](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)

**Rationale**:
1. ✅ **Redundant**: `status.deduplication.occurrenceCount` already tracks signal persistence
2. ✅ **No Consumers**: AI Analysis does NOT expose storm context to LLM (DD-AIANALYSIS-004)
3. ✅ **Zero Business Value**: Storm = boolean flag based on `occurrenceCount >= 5` (derivable)
4. ✅ **No Workflow Routing**: Remediation Orchestrator treats all signals equally

**Completion Status**: Gateway code removal complete (Dec 13), CRD spec cleanup pending (Dec 15).

---

## 🔍 Current State Analysis

### Storm Fields Still in RemediationRequest.spec

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

**Lines 278-293** (spec section):
```go
// Storm Detection
// True if this signal is part of a detected alert storm
IsStorm bool `json:"isStorm,omitempty"`

// Storm type: "rate" (frequency-based) or "pattern" (similar alerts)
StormType string `json:"stormType,omitempty"`

// Time window for storm detection (e.g., "5m")
StormWindow string `json:"stormWindow,omitempty"`

// Number of alerts in the storm
StormAlertCount int `json:"stormAlertCount,omitempty"`

// List of affected resources in an aggregated storm (e.g., "namespace:Pod:name")
// Only populated for aggregated storm CRDs
AffectedResources []string `json:"affectedResources,omitempty"`
```

**Current CRD Schema**: `config/crd/bases/kubernaut.ai_remediationrequests.yaml`
- Line 93-97: `spec.isStorm`
- Line 174: `spec.stormAlertCount`
- Line 176-179: `spec.stormType`
- Line 180-182: `spec.stormWindow`
- Line 46-49: `spec.affectedResources` (storm-specific aggregation field)

### Why These Fields Were Missed

**Root Cause**: Incomplete removal execution

**DD-GATEWAY-015 Removal Plan** (Lines 254-268) explicitly states:
```diff
// pkg/gateway/types/types.go
type NormalizedSignal struct {
-   // Storm Detection Fields
-   IsStorm bool
-   StormType string
-   StormWindow string
-   AlertCount int
-   AffectedResources []string
}
```

**Status vs Spec Confusion**:
- ✅ `status.stormAggregation` was successfully removed (no longer in CRD)
- ❌ `spec.isStorm`, `spec.stormType`, etc. were left behind

---

## ✅ Verification: Gateway No Longer Uses Storm Fields

### Gateway Service Status

**Confirmed**: Gateway Service does NOT populate storm spec fields anymore.

**Evidence**:
1. ✅ **STORM_REMOVAL_FINAL_STATUS.md** (Dec 14, 2025): Claims removal complete
2. ✅ **Integration Tests**: 96/96 passing after storm removal
3. ✅ **Source Code**: `pkg/gateway/server.go` storm logic removed
4. ✅ **Metrics**: `gateway_alert_storms_detected_total` removed
5. ✅ **Audit Events**: `gateway.storm.detected` removed

**Conclusion**: Gateway creates RRs without storm fields. Schema cleanup is safe.

---

## 🎯 Requested Action: Remove Storm Fields from RR Spec

### Scope of Removal

**Files to Modify**:
1. `api/remediation/v1alpha1/remediationrequest_types.go` - Remove 5 storm fields from spec struct
2. `config/crd/bases/kubernaut.ai_remediationrequests.yaml` - Regenerate via `make manifests`

**Fields to Remove** (5 fields):
1. `IsStorm bool` (line 280)
2. `StormType string` (line 283)
3. `StormWindow string` (line 286)
4. `StormAlertCount int` (line 289)
5. `AffectedResources []string` (line 293) - Storm-specific aggregation field

### Step-by-Step Implementation

#### Step 1: Remove Storm Fields from Go Types (10 minutes)

**Edit**: `api/remediation/v1alpha1/remediationrequest_types.go`

```diff
// RemediationRequestSpec defines the desired state of RemediationRequest.
type RemediationRequestSpec struct {
	// ... existing fields ...

	// Deduplication Metadata (DEPRECATED per DD-GATEWAY-011)
	Deduplication sharedtypes.DeduplicationInfo `json:"deduplication,omitempty"`

-	// Storm Detection
-	// True if this signal is part of a detected alert storm
-	IsStorm bool `json:"isStorm,omitempty"`
-
-	// Storm type: "rate" (frequency-based) or "pattern" (similar alerts)
-	StormType string `json:"stormType,omitempty"`
-
-	// Time window for storm detection (e.g., "5m")
-	StormWindow string `json:"stormWindow,omitempty"`
-
-	// Number of alerts in the storm
-	StormAlertCount int `json:"stormAlertCount,omitempty"`
-
-	// List of affected resources in an aggregated storm (e.g., "namespace:Pod:name")
-	// Only populated for aggregated storm CRDs
-	AffectedResources []string `json:"affectedResources,omitempty"`

	// ========================================
	// SIGNAL METADATA (PHASE 1 ADDITION)
	// ========================================
```

#### Step 2: Regenerate CRD Schema (5 minutes)

```bash
cd /path/to/kubernaut
make manifests
```

**Expected Result**: `config/crd/bases/kubernaut.ai_remediationrequests.yaml` updated without storm fields.

#### Step 3: Verify CRD Changes (5 minutes)

```bash
# Ensure storm fields removed from spec schema
grep -q "isStorm\|stormType\|stormWindow\|stormAlertCount\|affectedResources" \
  config/crd/bases/kubernaut.ai_remediationrequests.yaml && \
  echo "❌ Storm fields still present" || \
  echo "✅ Storm fields removed"
```

#### Step 4: Run RO Tests (1-2 hours)

```bash
# Unit tests
go test ./pkg/remediationorchestrator/... -v

# Integration tests
cd test/integration/remediationorchestrator && go test -v

# E2E tests
cd test/e2e/remediationorchestrator && go test -v
```

**Expected Result**: All tests pass (storm fields not used in test assertions).

#### Step 5: Check for Storm Field References in RO Code (30 minutes)

```bash
# Search for storm field usage in RO code
grep -r "IsStorm\|StormType\|StormWindow\|StormAlertCount" \
  pkg/remediationorchestrator/ --include="*.go"

# Search for storm field usage in RO tests
grep -r "IsStorm\|StormType\|StormWindow\|StormAlertCount" \
  test/unit/remediationorchestrator/ test/integration/remediationorchestrator/ \
  --include="*.go"
```

**Expected Result**: Zero matches (RO never used storm fields).

**If Matches Found**: Remove references in RO code/tests (should be rare).

---

## 📊 Impact Assessment

### Breaking Changes: **NONE** (Backward Compatible)

**Why This Is Safe**:
1. ✅ **Gateway stopped populating storm fields** (Dec 13, 2025)
2. ✅ **RO never consumed storm fields** (confirmed by code analysis)
3. ✅ **Old RRs with storm fields continue to work** (fields ignored by controllers)
4. ✅ **Kubernetes OpenAPI v3 handles backward compatibility** automatically

**Migration**: Not required - Old RRs with storm data are not affected.

### Test Impact

**Expected**: Zero test failures (storm fields not asserted in tests).

**If Test Failures Occur**:
- Check test builders: `pkg/testutil/builders/remediation_request.go`
- Remove `.WithStorm()` or similar builder methods if present
- Update test assertions to not check storm fields

---

## 🚨 Risk Analysis

### Risk: LOW

**Why Low Risk**:
1. ✅ Gateway removal already complete and validated (96/96 integration tests passing)
2. ✅ No downstream consumers (DD-AIANALYSIS-004 confirmed AI Analysis ignores storm)
3. ✅ Spec fields are immutable (controllers can't update them post-creation anyway)
4. ✅ Backward compatible (old CRDs continue to work)

### Rollback Plan

**If Removal Causes Issues** (highly unlikely):

**Option A: Revert Git Commit** (Recommended)
```bash
git revert <storm-removal-commit-sha>
git push
```
**Impact**: Restores storm fields in 5 minutes.

**Option B: Manual Restoration**
- Re-add storm fields to `remediationrequest_types.go`
- Run `make manifests` to regenerate CRD
- **Effort**: 15 minutes

---

## ✅ Validation Checklist

### Pre-Removal Validation
- [x] Gateway Service no longer populates storm fields (validated Dec 13)
- [x] RO code analysis confirms no storm field usage
- [x] AI Analysis confirmed not using storm (DD-AIANALYSIS-004)
- [x] Backup current CRD schema (Git history)

### Post-Removal Validation
- [ ] CRD regenerated successfully (`make manifests`)
- [ ] Storm fields absent from `kubernaut.ai_remediationrequests.yaml`
- [ ] RO unit tests pass
- [ ] RO integration tests pass
- [ ] RO E2E tests pass
- [ ] No compilation errors (`go build ./pkg/remediationorchestrator/...`)
- [ ] No lint errors (`golangci-lint run ./pkg/remediationorchestrator/...`)

---

## 📚 References

### Authoritative Documentation
- **DD-GATEWAY-015**: [Storm Detection Logic Removal](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
- **DD-AIANALYSIS-004**: [Storm Context NOT Exposed to LLM](../architecture/decisions/DD-AIANALYSIS-004-storm-context-not-exposed.md)
- **STORM_REMOVAL_FINAL_STATUS**: [Storm Removal Execution Report](STORM_REMOVAL_FINAL_STATUS.md)

### Triage Documents
- **TRIAGE_STORM_FIELDS_SPEC_DISCREPANCY**: [Current Discrepancy Analysis](TRIAGE_STORM_FIELDS_SPEC_DISCREPANCY.md)

### Related Schema Changes
- **DD-GATEWAY-011**: [Shared Status Ownership](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) - Deduplication moved to status
- **NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE**: Environment/Priority moved to SP status

---

## 📞 Contact Points

**Questions?**
- **Gateway Team**: Storm removal implementation complete (Dec 13)
- **RO Team**: Schema cleanup ownership (this handoff)
- **Architecture Team**: Schema design decisions and backward compatibility

**Timeline Flexibility**: No hard deadline - V1.0 polish item, not a blocker.

---

## ✅ Acceptance Criteria

**Handoff Complete When**:
1. ✅ Storm fields removed from `remediationrequest_types.go`
2. ✅ CRD schema regenerated without storm fields
3. ✅ All RO tests passing (unit + integration + E2E)
4. ✅ No compilation or lint errors
5. ✅ Documentation updated (STORM_REMOVAL_FINAL_STATUS marked truly complete)

---

**Handoff Date**: December 15, 2025
**Estimated Effort**: 2-4 hours
**Priority**: P2 (Medium) - Schema cleanup, not urgent
**Confidence**: 95% - Low risk, backward compatible, well-scoped

**Thank you for completing the storm removal work! 🎉**



