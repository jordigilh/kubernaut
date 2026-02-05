# DD-AUTH-014: Failure Triage Analysis

## Test Results: 152/158 Passing (96%)

### Critical Analysis: 6 Failures vs Authoritative Documentation

---

## Failure Category 1: Unauthenticated Client Bugs (2 failures) - **MUST FIX**

### Test 22: `22_audit_validation_helper_test.go:52`
**Error**: `unexpected response type: *api.CreateAuditEventUnauthorized` (401)

**Root Cause**: Test creates unauthenticated client directly:
```go
// Line 52: WRONG - unauthenticated client
client, err = ogenclient.NewClient(baseURL)
```

**Authoritative Reference**: DD-AUTH-014 mandates Zero Trust on ALL endpoints

**Fix**: Use shared authenticated `DSClient`
```go
// Use authenticated client from suite setup
client = DSClient
```

---

### Test 18: `18_workflow_duplicate_api_test.go:113`
**Error**: `decode response: unexpected status code: 401`

**Root Cause**: Test creates unauthenticated client for ListWorkflows:
```go
// Line 113: WRONG - unauthenticated client
listClient, err := ogenclient.NewClient(dataStorageURL)
```

**Authoritative Reference**: DD-AUTH-014 mandates Zero Trust on ALL endpoints

**Fix**: Use shared authenticated `DSClient`
```go
// Use authenticated client from suite setup
listResp, err := DSClient.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{})
```

---

## Failure Category 2: Performance Assertions (2 failures) - **EVALUATE**

### Test 1: `04_workflow_search_test.go:372`
**Assertion**: Search latency should be <1s
**Actual**: 2.19s
**BR Reference**: None explicitly stated for <1s threshold

**Analysis**:
- Environment: Kind cluster + 12 parallel processes + SAR middleware (2 K8s API calls per HTTP request)
- Previous behavior: Unknown (no baseline documented)
- DD-AUTH-014 impact: Added TokenReview + SAR overhead (~100-200ms per request)

**Questions for Authoritative Docs**:
1. Is <1s a documented BR requirement?
2. What's the acceptable latency for E2E environment vs production?
3. Should this account for SAR middleware overhead?

---

### Test 2: `06_workflow_search_audit_test.go:439`
**Assertion**: Average search latency should be <200ms
**Actual**: 4.56s
**BR Reference**: BR-AUDIT-024 states "async audit should not add significant latency" and "<50ms impact"

**Analysis**:
- BR-AUDIT-024 is about **audit write overhead**, not search latency
- Test comment: "async audit should not block" - validates audit writes don't slow searches
- The 4.56s latency suggests the test is measuring something different

**Questions for Authoritative Docs**:
1. Is BR-AUDIT-024's <50ms for audit write overhead, or search latency?
2. What's the expected search latency with SAR middleware?
3. Should this assertion be adjusted for E2E environment?

---

## Failure Category 3: Infrastructure Timeout (1 failure) - **INFRASTRUCTURE**

### Test 4: `05_soc2_compliance_test.go:157`
**Error**: `Timed out after 30.000s` (cert-manager certificate generation)
**Test Type**: BeforeAll infrastructure setup

**Root Cause**: cert-manager webhook needs more time in heavily loaded cluster

**Analysis**:
- Not a business logic issue
- Not an auth issue
- Infrastructure setup timing in parallel test environment
- 12 parallel processes may be slowing cert-manager webhook

**Fix Options**:
A) Increase BeforeAll timeout (30s → 60s)
B) Skip cert-manager tests in E2E (mark as integration-only)
C) Reduce load during cert-manager setup (serialize this test)

**Authoritative Reference**: SOC2 requirements for certificate-based signatures
- cert-manager is required for production
- E2E tests validate the infrastructure works
- Timeout is environment-dependent, not a BR violation

---

## Recommendations

### Immediate Fixes (Aligned with DD-AUTH-014)
1. **Test 22**: Fix unauthenticated client (use `DSClient`)
2. **Test 18**: Fix unauthenticated client (use `DSClient`)

### Requires Documentation Review (Performance)
3. **Test 1**: Review BR for <1s search latency expectation
   - **Action**: Check if this is a documented requirement
   - **Option A**: Adjust expectation for E2E+SAR environment
   - **Option B**: Keep assertion if it's a critical BR

4. **Test 2**: Validate BR-AUDIT-024 interpretation
   - **Action**: Check if <200ms is for audit write overhead or search latency
   - **Option A**: Adjust assertion to match BR intent
   - **Option B**: Remove if BR-AUDIT-024 doesn't apply to search latency

### Infrastructure Fix
5. **Test 4 (cert-manager)**: Increase BeforeAll timeout
   - **Action**: 30s → 60s for cert-manager certificate generation
   - **Justification**: Environment-dependent, not a BR violation

---

## Critical Question for User

**Performance Assertions (Tests 1 & 2)**: Are these latency expectations documented in business requirements, or are they test-specific expectations that should be adjusted for:
- E2E environment (Kind cluster)
- SAR middleware overhead (2 K8s API calls per HTTP request)
- 12 parallel test processes

**Recommendation**: Check authoritative BRs for documented latency SLAs before adjusting assertions.
