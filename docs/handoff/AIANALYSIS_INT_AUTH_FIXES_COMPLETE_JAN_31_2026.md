# AIAnalysis INT Tests - Auth Fixes Complete

> ⚠️ **DEPRECATION NOTICE**: ENV_MODE pattern removed as of Jan 31, 2026 (commit `5dce72c5d`)
>
> **What Changed**: HAPI production code no longer uses ENV_MODE conditional logic.
> - Production & Integration: Both use `K8sAuthenticator` + `K8sAuthorizer`
> - KUBECONFIG environment variable determines K8s API endpoint (in-cluster vs envtest)
> - Mock auth classes available for unit tests only (not in main.py)
>
> **See**: `holmesgpt-api/AUTH_RESPONSES.md` for current architecture


**Date:** January 31, 2026  
**Status:** ✅ **FIXES APPLIED** - Awaiting test validation  
**Priority:** P1 - Unblocks PR merge  
**Estimated Impact:** 20+ tests → passing (auth failures resolved)

---

## Executive Summary

**Completed:**
1. ✅ Event category updates (HAPI events → `"aiagent"`, 3 locations)
2. ✅ SAR resource name fix (`holmesgpt-api-service` → `holmesgpt-api`)
3. ✅ Unauthenticated client fix (added ServiceAccount token auth)

**Commits:**
- `e37986cd7` - Event category changes (already committed by team)
- `12432fc6b` - Auth fixes (just committed)

**Expected Result:** AIAnalysis INT tests → 90-100% pass rate (from ~40%)

---

## Problem 1: Event Category Update ✅ COMPLETE

### Issue
HAPI changed `event_category` from `"analysis"` to `"aiagent"` per ADR-034 v1.6.

### Fix Applied
Updated 3 locations in AIAnalysis INT tests:
- `audit_provider_data_integration_test.go:266` - HAPI validation
- `error_handling_integration_test.go:164` - HAPI query
- `error_handling_integration_test.go:368` - HAPI query

**Status:** ✅ Already committed in `e37986cd7`

---

## Problem 2: SAR Resource Name Mismatch ✅ FIXED

### Root Cause
```go
// suite_test.go:241 (BEFORE)
ResourceNames: []string{"holmesgpt-api-service"}, // ❌ Wrong resource name
```

**HAPI middleware expects:** `"holmesgpt-api"`  
**ClusterRole specified:** `"holmesgpt-api-service"` ❌

**Result:** All SubjectAccessReview checks failing → HTTP 401

### Fix Applied
```go
// suite_test.go:241 (AFTER)
ResourceNames: []string{"holmesgpt-api"}, // ✅ Correct resource name
```

**Impact:** Expected to fix 20+ tests failing with auth errors

**Commit:** `12432fc6b`

---

## Problem 3: Unauthenticated Client Creation ✅ FIXED

### Root Cause
```go
// recovery_integration_test.go:347 (BEFORE)
shortClient, err := client.NewHolmesGPTClient(client.Config{...})  // ❌ No auth!
```

**Issue:** `NewHolmesGPTClient()` tries to read ServiceAccount token from filesystem (`/var/run/secrets/kubernetes.io/serviceaccount/token`), which doesn't exist in local INT tests.

**Result:** Client created without Bearer token → HTTP 401

### Fix Applied

**Step 1:** Added global token variable
```go
// suite_test.go:100
serviceAccountToken string  // ✅ Available to all tests
```

**Step 2:** Set token during Phase 2 setup
```go
// suite_test.go:525
serviceAccountToken = token  // ✅ Extracted from phase1Data
```

**Step 3:** Use authenticated client in test
```go
// recovery_integration_test.go:347 (AFTER)
hapiAuthTransport := testauth.NewServiceAccountTransport(serviceAccountToken)
shortClient, err := client.NewHolmesGPTClientWithTransport(
    client.Config{...},
    hapiAuthTransport,  // ✅ With auth
)
```

**Impact:** Fixes 1-2 tests creating custom clients

**Commit:** `12432fc6b`

---

## Files Modified

| File | Changes | Lines |
|------|---------|-------|
| `api/openapi/data-storage-v1.yaml` | Added `aiagent` enum | 1571, 1573, 1579 |
| `pkg/datastorage/ogen-client/*.go` | Regenerated with new constant | Multiple |
| `test/integration/aianalysis/audit_provider_data_integration_test.go` | HAPI event category | 266 |
| `test/integration/aianalysis/error_handling_integration_test.go` | HAPI event category | 164, 368 |
| `test/integration/aianalysis/suite_test.go` | SAR resource + token global | 100, 241, 525 |
| `test/integration/aianalysis/recovery_integration_test.go` | Authenticated client | 27, 347-351 |

---

## Technical Details

### DD-AUTH-014 Compliance

**HAPI Middleware Auth Flow:**
1. Request arrives with `Authorization: Bearer <token>` header
2. HAPI performs Kubernetes TokenReview (validates token)
3. HAPI performs SubjectAccessReview with:
   - **Resource**: `services`
   - **ResourceName**: `holmesgpt-api` (MUST MATCH)
   - **Verb**: `create`
4. If SAR approved → Request proceeds
5. If SAR denied → HTTP 403
6. If no token → HTTP 401

**Our Fix:** Corrected ResourceName in ClusterRole to match HAPI's expectation

### Auth Architecture

```
┌─────────────────────────────────────────────────────┐
│ Phase 1 (Shared Setup)                              │
│  - Create ServiceAccount: aianalysis-ds-client      │
│  - Extract token → authConfig.Token                 │
│  - Create ClusterRole: holmesgpt-api-client        │
│    - Resource: services                             │
│    - ResourceName: "holmesgpt-api" ✅               │
│    - Verb: create                                   │
│  - Bind to aianalysis-ds-client SA                  │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│ Phase 2 (Per-Process Setup)                         │
│  - Deserialize token from Phase 1                   │
│  - Set serviceAccountToken = token ✅               │
│  - Create authenticated HAPI client:                │
│    hapiAuthTransport := testauth.                   │
│        NewServiceAccountTransport(token)            │
│  - All controller requests → Bearer token ✅        │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│ Test Execution                                       │
│  - Controller uses realHGClient (authenticated)     │
│  - Tests can create custom clients with:            │
│    NewHolmesGPTClientWithTransport(                 │
│        ...,                                          │
│        testauth.NewServiceAccountTransport(          │
│            serviceAccountToken)) ✅                  │
└─────────────────────────────────────────────────────┘
```

---

## Validation Checklist

**Before Running Tests:**
- [x] Event category updated for HAPI events
- [x] DataStorage OpenAPI spec includes `aiagent`
- [x] OpenAPI client regenerated
- [x] SAR resource name corrected
- [x] Global token variable added
- [x] Token set during Phase 2
- [x] Unauthenticated client fixed
- [x] No lint errors
- [x] Changes committed

**After Running Tests (Expected):**
- [ ] AIAnalysis INT tests pass rate: 90-100%
- [ ] No HTTP 401 errors in logs
- [ ] No HTTP 403 errors in logs
- [ ] All HAPI event queries return results
- [ ] Recovery integration test passes

---

## Expected Test Results

### Before Fixes
```
AIAnalysis INT Tests: ~40% pass rate
- ~20+ tests failing with: 
  "decode response: unexpected Content-Type: application/problem+json"
- Root cause: HTTP 401 (auth failures)
```

### After Fixes (Expected)
```
AIAnalysis INT Tests: 90-100% pass rate
- Auth failures resolved
- HAPI event queries working
- SAR checks passing
```

---

## Confidence Assessment

| Fix | Confidence | Impact |
|-----|------------|--------|
| **Event category** | 100% | 3 tests (event validation) |
| **SAR resource name** | 95% | 20+ tests (all auth failures) |
| **Unauthenticated client** | 100% | 1-2 tests (timeout tests) |
| **Overall** | **90%** | **Expected: 90-100% pass rate** |

**High Confidence Rationale:**
1. ✅ SAR resource name confirmed by user ("we settled for holmesgpt-api")
2. ✅ Auth pattern matches working E2E tests
3. ✅ Token properly propagated from Phase 1 → Phase 2
4. ✅ No remaining unauthenticated client creations found

**Remaining Risk (10%):**
- HAPI middleware configuration might have additional requirements
- Integration test infrastructure might have other auth-related issues
- Edge cases we haven't encountered yet

---

## Known Limitations

### Not Fixed in This PR
1. **E2E Auth Issues** - Separate issue (different auth pattern needed)
2. **Metrics Tests** - Some may still fail (unrelated to auth)
3. **Recovery Endpoint** - May need additional HAPI configuration

### Still Being Investigated by E2E Team
- E2E tests have similar auth failures
- E2E team working on controller → HAPI auth
- INT fixes may inform E2E solution

---

## Next Steps

### Immediate (User Action)
1. **Run tests:**
   ```bash
   make test-integration-aianalysis
   ```

2. **Verify results:**
   - Check pass rate (expect 90-100%)
   - Check logs for HTTP 401/403 errors (should be none)
   - Note any remaining failures (likely unrelated to auth)

### If Tests Still Fail
1. **Check HAPI logs** for SAR errors:
   ```bash
   docker logs aianalysis_holmesgpt_test | grep -i "sar\|unauthorized"
   ```

2. **Verify HAPI middleware config:**
   - ENV_MODE=integration
   - SAR_RESOURCE_NAME=holmesgpt-api
   - DD_AUTH_014 enabled

3. **Debug specific failure:**
   - Extract must-gather
   - Analyze controller logs
   - Check HAPI response bodies

---

## Related Documentation

- **ADR-034 v1.6:** Event category table (added `aiagent`)
- **DD-AUTH-014:** Middleware-based SAR authentication
- **HAPI_INT_AUTH_FIX_JAN_31_2026.md:** Priority 1 auth fixes (Python tests)
- **AIANALYSIS_INT_HAPI_EVENT_CATEGORY_UPDATE_JAN_31_2026.md:** Event category guide
- **HAPI_AUDIT_ARCHITECTURE_FIX_JAN_31_2026.md:** HAPI event category change details

---

## Commands

### Run Tests
```bash
make test-integration-aianalysis
```

### Check Test Output
```bash
# Summary
grep -E "Ran [0-9]|PASS|FAIL" /path/to/test/output

# Auth errors
grep -E "401|403|Unauthorized|Forbidden" /path/to/test/output
```

### Debug HAPI
```bash
# Check HAPI logs
docker logs aianalysis_holmesgpt_test | tail -100

# Check SAR configuration
docker exec aianalysis_holmesgpt_test env | grep -i auth
```

---

## Summary

**Work Completed:**
- ✅ Event category updates (HAPI → `"aiagent"`)
- ✅ SAR resource name fix (`holmesgpt-api-service` → `holmesgpt-api`)
- ✅ Unauthenticated client fix (added ServiceAccount token)
- ✅ All changes committed and ready for validation

**Expected Outcome:**
- 🎯 AIAnalysis INT tests: 90-100% pass rate
- 🎯 Auth errors eliminated
- 🎯 HAPI integration working correctly

**Confidence:** **90%** (high confidence, proven auth pattern)

**Status:** ✅ **READY FOR VALIDATION** - Run tests to confirm

---

**Prepared by:** AI Assistant  
**Date:** January 31, 2026  
**Commit:** `12432fc6b` (auth fixes)  
**Previous Commit:** `e37986cd7` (event category changes)
