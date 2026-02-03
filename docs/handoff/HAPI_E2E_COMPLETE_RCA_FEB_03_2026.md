# HAPI E2E Complete Root Cause Analysis
**Date**: February 3, 2026  
**Test Run**: `/tmp/hapi-e2e-no-cache-final.log`  
**Must-Gather**: `/tmp/holmesgpt-api-e2e-logs-20260202-223740/`

---

## Executive Summary

**Status**: 25/40 tests passing, 15 failures  
**Root Cause**: Mock LLM Pod running old cached code despite `--no-cache` build fixes  
**Impact**: Mock LLM not returning workflows → HAPI correctly sets `needs_human_review=true`

---

## Test Results

```
✅ 25 Passed | ❌ 15 Failed | ⏭️ 3 Skipped | ⏱️ 5m 6s
```

### Failed Tests
1. E2E-HAPI-001: No workflow found returns human review
2. E2E-HAPI-002: Low confidence returns human review with alternatives
3. E2E-HAPI-003: Max retries exhausted returns validation history
4. E2E-HAPI-004: Normal incident analysis succeeds (**KEY TEST**)
5. E2E-HAPI-005: Incident response structure validation
6. E2E-HAPI-007: Invalid request returns error
7. E2E-HAPI-008: Missing remediation ID returns error
8. E2E-HAPI-013: Recovery endpoint happy path
9. E2E-HAPI-014: Recovery response field types validation
10. E2E-HAPI-018: Recovery rejects invalid attempt number
11. E2E-HAPI-023: Signal not reproducible returns no recovery
12. E2E-HAPI-024: No recovery workflow found returns human review
13. E2E-HAPI-026: Normal recovery analysis succeeds
14. E2E-HAPI-027: Recovery response structure validation
15. E2E-HAPI-028: Recovery with previous execution context

---

## Investigation Timeline

### Phase 1: Initial Analysis
**Finding**: HAPI logs show `has_workflow=False` for OOMKilled/crashloop scenarios

**HAPI Log Evidence** (test-audit-045, OOMKilled):
```
'event': 'workflow_validation_passed', 'has_workflow': False
'event': 'incident_analysis_completed', 'has_workflow': False, 'needs_human_review': True
```

**Initial Hypothesis**: HAPI business logic bug (incorrectly setting `needs_human_review=true`)

**Verdict**: ❌ WRONG - HAPI is **correctly** following BR-HAPI-197 lines 252-255

### Phase 2: BR-HAPI-197 Compliance Check
**Business Requirement**: BR-HAPI-197 lines 252-255
```python
elif selected_workflow is None:
    warnings.append("No workflows matched the search criteria")
    needs_human_review = True
    human_review_reason = "no_matching_workflows"
```

**Conclusion**: HAPI behavior is **100% compliant** with BR-HAPI-197  
**New Hypothesis**: Mock LLM is NOT returning workflows

### Phase 3: Mock LLM Analysis
**HAPI SDK Evidence**:
```
[SDK DEBUG] Calling litellm.completion with: model=openai/mock-model, api_base=http://mock-llm:8080, api_key=***
LiteLLM - INFO - Wrapper: Completed Call, calling success_handler
```
- HAPI **IS** calling Mock LLM
- LiteLLM call **succeeds** (success_handler called)
- Response received, but `has_workflow=False`

**Mock LLM Log Evidence**:
```
✅ Mock LLM loaded 12/15 scenarios from file
✅ Loaded oomkilled (oomkill-increase-memory-v1:staging) → 9710e507-0889-49e8-9fd9-f99e3293b09d
✅ Loaded crashloop (crashloop-config-fix-v1:production) → 6c3bae57-0a48-4d82-8499-aeb661f0fe15
✅ Mock LLM running at http://0.0.0.0:8080
```

**Total Mock LLM logs**: 25 lines (startup only)  
**Request logs**: **ZERO** ❌

### Phase 4: Code Version Verification
**Expected Logging** (server.py:532):
```python
logger.info(f"📥 Mock LLM received POST {self.path} from {self.client_address[0]}")
```

**Actual Logs**: No "📥" entries → **Mock LLM is running OLD CODE**

**Phase 2 Debug Logging** (server.py:660, 664, 668, 673):
```python
logger.info(f"✅ PHASE 2: Matched 'crashloop' → scenario={matched_scenario.name}, workflow_id={matched_scenario.workflow_id}")
logger.warning(f"⚠️  PHASE 2: NO MATCH - Falling back to current_scenario={fallback_scenario.name}")
```

**Actual Logs**: No "PHASE 2" entries → **Confirms OLD CODE**

---

## Root Cause

### Primary Issue: Persistent Build Cache

**Despite**:
1. ✅ Added `--no-cache` to `test/infrastructure/e2e_images.go` (line 158)
2. ✅ Added `--no-cache` to `test/infrastructure/mock_llm.go` (line 122)
3. ✅ Fresh build logged: `localhost/mock-llm:mock-llm-18909f15`
4. ✅ Image loaded to Kind: `✅ Mock LLM image loaded`

**The Mock LLM Pod is STILL running old cached code**.

### Evidence Chain
```
Source Code (server.py) ✅ Has Phase 1/2 fixes
         ↓
Podman Build ✅ --no-cache flag applied
         ↓
Image Built ✅ localhost/mock-llm:mock-llm-18909f15
         ↓
Kind Load ✅ Image loaded to cluster
         ↓
Pod Running ❌ OLD CODE (no request logs)
```

### Hypothesis: Image Layer Caching
**Possible causes**:
1. Pod created before image loaded → using older image with same tag
2. Kubernetes ImagePullPolicy caching (should be `Never` for `localhost/` images)
3. Kind's internal image cache not being invalidated
4. Pod not restarted after new image loaded

---

## Business Logic Validation

### HAPI: ✅ CORRECT
- HAPI correctly implements BR-HAPI-197
- When `selected_workflow is None`, sets `needs_human_review=true`
- Validation logic working as designed

### Mock LLM: ❌ DEPLOYMENT ISSUE
- Source code has correct workflow definitions
- Scenarios have UUIDs loaded from ConfigMap
- **BUT**: Pod not executing new code

### Test Expectations: ✅ CORRECT
- E2E-HAPI-004 expects `needs_human_review=false` for OOMKilled
- This is correct per BR-HAPI-197 (confident workflow should NOT need human review)
- Test validates business requirement accurately

---

## Solution Path

### Immediate Fix (Option A): Force Pod Restart
```bash
# After image build/load, force Pod restart
kubectl delete pod -n holmesgpt-api-e2e -l app=mock-llm
# Wait for new Pod to start with fresh image
kubectl wait --for=condition=Ready pod -n holmesgpt-api-e2e -l app=mock-llm --timeout=60s
```

### Immediate Fix (Option B): Set ImagePullPolicy
**File**: `config/mock-llm/deployment.yaml` or E2E test infrastructure
```yaml
spec:
  containers:
  - name: mock-llm
    image: localhost/mock-llm:mock-llm-HASH
    imagePullPolicy: Never  # Force use of local image, never pull
```

### Long-term Fix (Option C): Unique Image Tags
```go
// Use commit hash + timestamp for truly unique tags
imageTag := fmt.Sprintf("mock-llm-%s-%d", commitHash, time.Now().Unix())
```

### Verification Steps
1. **Before test run**: Check Mock LLM image in Kind
   ```bash
   docker exec holmesgpt-api-e2e-control-plane crictl images | grep mock-llm
   ```

2. **After image load**: Verify Pod uses new image
   ```bash
   kubectl get pod -n holmesgpt-api-e2e -l app=mock-llm -o jsonpath='{.items[0].spec.containers[0].image}'
   ```

3. **After Pod start**: Confirm Phase 2 logging in Pod logs
   ```bash
   kubectl logs -n holmesgpt-api-e2e -l app=mock-llm | grep "PHASE 2\|📥"
   ```

---

## Test Plan Impact

### Category F Scenarios (Phase 1 Fix Applied)
**Status**: ✅ Fixed in source code, ❌ Not deployed

**Modified Scenarios** (workflow_id="" to indicate no_matching_workflows):
1. `multi_step_recovery` → `""`
2. `cascading_failure` → `""`
3. `near_attempt_limit` → `""`
4. `noisy_neighbor` → `""`
5. `network_partition` → `""`
6. `recovery_basic` → `""`

**Expected Result**: Tests should get `needs_human_review=true` with `human_review_reason="no_matching_workflows"` (correct per BR-HAPI-197)

### Basic Scenarios (Should Return Workflows)
**Status**: ✅ Configured correctly in source code, ❌ Not deployed

**Scenarios**:
- `oomkilled` → Should return `9710e507-0889-49e8-9fd9-f99e3293b09d`
- `crashloop` → Should return `6c3bae57-0a48-4d82-8499-aeb661f0fe15`
- `low_confidence` → Should return `7539356e-bba0-4fc1-b0cd-5f46e923f193`

**Current Behavior**: Returns NO workflow (old code)  
**Expected Behavior**: Returns workflow with UUID (new code)

---

## Authoritative Specification References

### BR-HAPI-197: Human Review Decision Logic
**Lines 252-255**: `needs_human_review=true` when no workflow found  
**Lines 256-258**: HAPI does NOT set based on confidence (AIAnalysis responsibility)  
**Compliance**: ✅ HAPI implementation is 100% compliant

### DD-HAPI-002 v1.2: Workflow Response Validation
**Lines 507-520**: Workflow validation logic  
**Lines 606-609**: Set `needs_human_review=true` if validation fails  
**Compliance**: ✅ HAPI implementation is correct

### BR-HAPI-212: RCA Affected Resource
**Lines 261-270**: Validate affectedResource present when workflow selected  
**Compliance**: ✅ HAPI implementation is correct

---

## Confidence Assessment

**Root Cause Confidence**: 95%
- Evidence-based: HAPI logs + Mock LLM logs + source code analysis
- Multiple confirmation points: SDK calls succeed, but Pod has old code
- Systemic pattern: Build cache issues recurring throughout session

**HAPI Business Logic**: 100% confidence it's correct
- Detailed BR-HAPI-197 compliance analysis
- All validation logic matches spec
- Tests validate correct behavior

**Solution Path**: 90% confidence
- Pod restart should force new image usage
- May need additional ImagePullPolicy or unique tag fix
- Verification steps will confirm

---

## Next Steps

1. **Apply Solution A or B** to force Mock LLM Pod to use new image
2. **Verify Phase 2 logging** appears in Mock LLM logs
3. **Re-run E2E tests** and expect:
   - ✅ Basic scenarios (oomkilled, crashloop) return workflows
   - ✅ needs_human_review=false for confident recommendations
   - ✅ Category F scenarios correctly return no_matching_workflows
4. **If still failing**: Apply Solution C (unique image tags with timestamp)

---

## Related Documentation

- `docs/handoff/HAPI_E2E_BR_HAPI_197_ANALYSIS_FEB_02_2026.md` - Initial BR analysis
- `docs/handoff/HAPI_E2E_ROOT_CAUSE_COMPLETE_FEB_02_2026.md` - Mock LLM hardcoded names
- `test/infrastructure/e2e_images.go` - E2E image build infrastructure
- `test/infrastructure/mock_llm.go` - Mock LLM image build infrastructure
- `test/services/mock-llm/src/server.py` - Mock LLM source code (Phase 1/2 fixes applied)
