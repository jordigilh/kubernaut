# Gateway CRD Creator Schema Gaps

**Date**: 2025-10-09
**Status**: Identified before integration testing

## Schema Comparison

Comparing `api/remediation/v1alpha1/remediationrequest_types.go` (CRD spec) with `pkg/gateway/processing/crd_creator.go` (populator):

### ✅ Fields Currently Populated
1. SignalFingerprint ✅
2. SignalName ✅ (as AlertName)
3. Severity ✅
4. Environment ✅
5. Priority ✅
6. SignalType ✅ (as SourceType)
7. TargetType ✅
8. FiringTime ✅
9. ReceivedTime ✅
10. Deduplication ✅
11. SignalLabels ✅
12. SignalAnnotations ✅
13. OriginalPayload ✅

### ❌ Fields Missing from CRD Creator
1. **SignalSource** (optional) - Adapter name (e.g., "prometheus-adapter")
2. **IsStorm** (optional) - Storm detection flag
3. **StormType** (optional) - "rate" or "pattern"
4. **StormWindow** (optional) - Time window (e.g., "5m")
5. **StormAlertCount** (optional) - Number of alerts in storm
6. **ProviderData** (optional) - Provider-specific JSON data

## Impact Assessment

### Critical (Blocks Integration Testing)
**NONE** - All optional fields can be added later

### High (Should Add Before Testing)
1. **SignalSource**: Useful for debugging which adapter processed the signal
2. **Storm Fields**: Server.go already detects storms, but doesn't populate CRD fields

### Medium (Nice to Have)
1. **ProviderData**: Future extensibility for non-Kubernetes targets

## Recommended Fixes (Before Integration Testing)

### Fix 1: Add SignalSource
```go
// In crd_creator.go CreateRemediationRequest()
Spec: remediationv1alpha1.RemediationRequestSpec{
    // ... existing fields ...
    SignalSource: signal.Source, // NEW: Adapter name
```

**Prerequisite**: Add `Source` field to `types.NormalizedSignal`

### Fix 2: Add Storm Fields
```go
// In crd_creator.go CreateRemediationRequest()
Spec: remediationv1alpha1.RemediationRequestSpec{
    // ... existing fields ...
    IsStorm:         signal.IsStorm,         // NEW
    StormType:       signal.StormType,       // NEW
    StormWindow:     signal.StormWindow,     // NEW
    StormAlertCount: signal.AlertCount,      // NEW
```

**Prerequisite**: Add storm fields to `types.NormalizedSignal`

### Fix 3: Optional - Add ProviderData (Future)
Not needed for initial testing. Can add later when supporting non-K8s targets.

## Action Plan

**Option A: Fix Now** (Recommended for completeness)
1. Add missing fields to `types.NormalizedSignal`
2. Update `crd_creator.go` to populate them
3. Start integration testing

**Time**: 30 minutes

**Option B: Fix During Testing** (Pragmatic)
1. Start integration testing immediately
2. Add fields when storm detection test fails
3. Iterate based on test feedback

**Time**: Same (30 min), but distributed

## Recommendation

**Option A** - Fix schema alignment now before testing. Reasons:
1. Takes only 30 minutes
2. Avoids test failures from known issues
3. Storm detection is already implemented (just not wired to CRD)
4. Cleaner test results (no noise from schema mismatches)

## Next Steps

1. ✅ Add storm + source fields to `types.NormalizedSignal` (5 min)
2. ✅ Update `crd_creator.go` to populate them (10 min)
3. ✅ Update `server.go` to set storm fields on signal (already done, just needs NormalizedSignal to have them) (5 min)
4. ✅ Verify compilation (2 min)
5. ✅ Start integration testing (Day 7)

**Total**: ~25 minutes, then ready for integration tests ✅

