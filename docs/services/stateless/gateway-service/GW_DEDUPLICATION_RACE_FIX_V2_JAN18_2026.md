# Gateway Deduplication Race Condition Fix - Version 2

**Date**: January 18, 2026
**Issue**: GW-DEDUP-002 test failing - 5 RemediationRequests created instead of 1
**Root Cause**: `NewServerForTesting` function was still using cached `ctrlClient` for deduplication

## Problem Analysis

### Initial Fix (Incomplete)
- Changed `createServerWithClients` to use `apiReader` for `PhaseBasedDeduplicationChecker` ✅
- However, `NewServerForTesting` function (used in E2E tests) was NOT using `createServerWithClients`
- `NewServerForTesting` directly instantiated components and used undefined `apiReader` variable ❌

### Test Evidence
```
Expected
    <int>: 5
to equal
    <int>: 1
```
**Result**: Test failed - race condition still occurring despite initial fix

## Root Cause - Dual Code Paths

Found **TWO instances** of `PhaseBasedDeduplicationChecker` creation:

1. **Line 225** (`NewServerForTesting`): Using `apiReader` (undefined) → **CAUSED LINTER ERROR**
2. **Line 425** (`createServerWithClients`): Using `apiReader` correctly ✅

**Issue**: E2E tests use `NewServerForTesting`, which had the incomplete/broken code path.

## Complete Fix

### Changes Made

**File**: `pkg/gateway/server.go`

#### 1. `NewServerForTesting` function (Line 212-227)
```go
// BEFORE (broken):
phaseChecker := processing.NewPhaseBasedDeduplicationChecker(apiReader) // undefined variable!

// AFTER (fixed):
// DD-GATEWAY-011: Use ctrlClient as apiReader for deduplication (test environment uses direct API access)
// This ensures concurrent requests see each other's CRD creations immediately (GW-DEDUP-002 fix)
phaseChecker := processing.NewPhaseBasedDeduplicationChecker(ctrlClient)
```

**Rationale**: In test environments, `ctrlClient` is configured to use direct API access (no caching), so it serves as both the cached client and the `apiReader`.

#### 2. `createServerWithClients` function (Line 425)
✅ Already correct - using `apiReader` parameter

## Technical Details

### Why `ctrlClient` Works as `apiReader` in Tests

In E2E tests (`NewServerWithK8sClient`):
```go
// DD-STATUS-001: Use ctrlClient as apiReader for cache-bypassed reads
// In test environments, this provides direct K8s API access
return createServerWithClients(cfg, logger, metricsInstance, ctrlClient, ctrlClient, k8sClient)
                                                                        ^^^^^^^^^^^  ^^^^^^^^^^
                                                                        both are ctrlClient
```

**Key Insight**: Tests pass the **same `ctrlClient`** for both `ctrlClient` and `apiReader` parameters to `createServerWithClients`, ensuring direct API access without caching.

`NewServerForTesting` should follow the same pattern.

## Verification Plan

1. **Build Check**: ✅ No linter errors
2. **E2E Test**: Run `make test-e2e-gateway TEST_FLAGS="-focus 'GW-DEDUP-002'"`
3. **Expected Result**: Only 1 RemediationRequest created despite 5 concurrent requests

## Impact Analysis

### Files Changed
- `pkg/gateway/server.go`: Fixed `NewServerForTesting` to use `ctrlClient` for `phaseChecker`

### Tests Affected
- `test/e2e/gateway/35_deduplication_edge_cases_test.go`: GW-DEDUP-002

### Production Code
- ✅ No impact - production uses `createServerWithClients` which was already correct
- ✅ Only test helper function was affected

## Confidence Assessment

**Confidence**: 90%

**Rationale**:
- Fix addresses the actual code path used by E2E tests
- Pattern matches existing working code (`NewServerWithK8sClient`)
- `PhaseBasedDeduplicationChecker` accepts `client.Reader`, and `ctrlClient` implements this interface
- No production code changes - only test infrastructure

**Remaining Risk**:
- 10% possibility that the test environment's `ctrlClient` still has caching enabled
- If test still fails, may need to investigate test client configuration

## Related Documentation

- **DD-GATEWAY-011**: Deduplication Architecture (CRD-based, Redis-free)
- **DD-STATUS-001**: Status Updater Pattern (apiReader for cache-bypassed reads)
- **GW_CONCURRENT_DEDUPLICATION_RACE_ANALYSIS_JAN18_2026.md**: Original race condition analysis
- **GW_DEDUPLICATION_RACE_FIX_JAN18_2026.md**: First fix attempt (incomplete)

## Next Steps

1. ✅ Fixed linter error (`undefined: apiReader`)
2. 🔄 Running E2E test: `make test-e2e-gateway TEST_FLAGS="-focus 'GW-DEDUP-002'"`
3. ⏳ Awaiting test results
4. 📋 If test passes: Update test plan documentation and mark todo as complete
5. 📋 If test fails: Investigate test client configuration and consider alternative approaches
