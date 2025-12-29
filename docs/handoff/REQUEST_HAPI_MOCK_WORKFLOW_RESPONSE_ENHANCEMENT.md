# REQUEST: HolmesGPT-API Mock Response Enhancement for E2E Testing

**From**: AIAnalysis Team
**To**: HolmesGPT-API (HAPI) Team
**Date**: 2025-12-12
**Priority**: High
**Issue Type**: E2E Test Blocker
**Related**: [DIAGNOSIS_AA_E2E_TEST_FAILURES.md](DIAGNOSIS_AA_E2E_TEST_FAILURES.md)

---

## 📋 **Executive Summary**

AIAnalysis E2E tests are failing because HAPI's mock LLM responses don't include workflow selection data that the AIAnalysis controller expects. This blocks 9/11 failing E2E tests (41% of total test suite).

**Current Status**: 11/22 tests passing (50%)
**Expected After Fix**: 20-22/22 tests passing (91-100%)
**Impact**: High - blocks AIAnalysis E2E test completion

---

## 🔍 **Problem Description**

### **Observed Behavior**

When AIAnalysis controller calls HAPI mock endpoints:
1. ✅ HAPI returns 200 OK
2. ❌ Response missing `selected_workflow` field
3. ❌ Response missing `recovery_analysis` field (for recovery endpoint)
4. ❌ AIAnalysis controller transitions to "Failed" phase
5. ❌ E2E tests timeout waiting for "Completed" phase

### **Controller Log Evidence**

```
ERROR  controllers.AIAnalysis.analyzing-handler
  No workflow selected - investigation may have failed
  {"name": "e2e-recovery-conditions-1765597772912708000"}

DEBUG  controllers.AIAnalysis.investigating-handler
  HAPI did not return recovery_analysis, skipping RecoveryStatus population
  {"isRecoveryAttempt": true, "attemptNumber": 1}

INFO   Phase changed, requeuing
  {"from": "Analyzing", "to": "Failed"}
```

### **Root Cause**

HAPI mock responses are simplified for API contract testing but don't include the business logic data that AIAnalysis controller needs to proceed through its workflow.

---

## 🎯 **Requested Changes**

### **1. Initial Incident Analysis Endpoint**

**Endpoint**: `POST /api/v1/incident/analyze`

**Current Mock Response** (assumed):
```json
{
  "status": "success",
  "confidence": 0.85,
  "analysis": "..."
}
```

**Requested Mock Response**:
```json
{
  "status": "success",
  "confidence": 0.85,
  "analysis": "Mock analysis for E2E testing",
  "selected_workflow": {
    "workflow_id": "test-workflow-001",
    "name": "Pod Restart Remediation",
    "confidence": 0.85,
    "reasoning": "Mock workflow selection for E2E testing",
    "labels": {
      "component": "*",
      "environment": "*",
      "severity": "*"
    }
  },
  "alternative_workflows": [
    {
      "workflow_id": "test-workflow-002",
      "name": "Node Drain Alternative",
      "confidence": 0.65,
      "reasoning": "Alternative mock workflow"
    }
  ],
  "warnings": [
    {
      "type": "data_quality",
      "message": "Mock warning for testing",
      "severity": "medium"
    }
  ],
  "target_in_owner_chain": false,
  "needs_human_review": false
}
```

**Why**: AIAnalysis controller requires `selected_workflow` to proceed to Analyzing phase. Without it, the controller fails with "No workflow selected" error.

---

### **2. Recovery Analysis Endpoint**

**Endpoint**: `POST /api/v1/recovery/analyze`

**Current Mock Response** (assumed):
```json
{
  "status": "success",
  "confidence": 0.75,
  "analysis": "..."
}
```

**Requested Mock Response**:
```json
{
  "status": "success",
  "confidence": 0.75,
  "analysis": "Mock recovery analysis for E2E testing",
  "recovery_analysis": {
    "previous_attempt_assessment": "Previous attempt failed due to resource constraints",
    "state_changed": false,
    "current_signal_type": "same",
    "suggested_actions": [
      "retry_with_backoff",
      "escalate_if_fails_again"
    ],
    "confidence_adjustment": -0.05,
    "reasoning": "Mock recovery assessment for E2E testing"
  },
  "selected_workflow": {
    "workflow_id": "test-recovery-workflow-001",
    "name": "Recovery Workflow with Backoff",
    "confidence": 0.75,
    "reasoning": "Mock recovery workflow selection",
    "labels": {
      "component": "*",
      "environment": "*",
      "severity": "*"
    }
  },
  "alternative_workflows": [
    {
      "workflow_id": "test-recovery-workflow-002",
      "name": "Escalation Workflow",
      "confidence": 0.60,
      "reasoning": "Alternative recovery path"
    }
  ],
  "warnings": [
    {
      "type": "recovery_attempt",
      "message": "Third attempt - consider escalation",
      "severity": "high"
    }
  ],
  "needs_human_review": false
}
```

**Why**: AIAnalysis controller needs `recovery_analysis` field to populate RecoveryStatus in the AIAnalysis CR. Without it, recovery-specific tests fail.

---

## 📊 **Impact Analysis**

### **Tests Currently Blocked** (9/22 tests)

#### **Recovery Flow Tests** (5 tests)
- BR-AI-080: Recovery attempt support
- BR-AI-081: Previous execution context handling
- BR-AI-082: Recovery endpoint routing verification
- BR-AI-083: Multi-attempt recovery escalation
- Conditions population during recovery flow

#### **Full Flow Tests** (4 tests)
- BR-AI-001: Production incident - full 4-phase cycle
- BR-AI-011: Data quality warnings - approval required
- BR-AI-013: Recovery attempt escalation - approval required
- Staging incident with auto-approval

**Total Impact**: 9/22 tests (41% of E2E suite)

---

## 🔧 **Implementation Guidance**

### **File to Modify**
`holmesgpt-api/src/mock_responses.py`

### **Suggested Approach**

```python
# For incident analysis
MOCK_INCIDENT_RESPONSE = {
    "status": "success",
    "confidence": 0.85,
    "analysis": "Mock analysis for E2E testing",
    "selected_workflow": {
        "workflow_id": "test-workflow-001",
        "name": "Pod Restart Remediation",
        "confidence": 0.85,
        "reasoning": "Mock workflow selection",
        "labels": {
            "component": "*",  # Wildcard matches all components
            "environment": "*",
            "severity": "*"
        }
    },
    "alternative_workflows": [
        {
            "workflow_id": "test-workflow-002",
            "name": "Alternative Workflow",
            "confidence": 0.65,
            "reasoning": "Alternative mock workflow"
        }
    ],
    "warnings": [],
    "target_in_owner_chain": False,
    "needs_human_review": False
}

# For recovery analysis
MOCK_RECOVERY_RESPONSE = {
    "status": "success",
    "confidence": 0.75,
    "analysis": "Mock recovery analysis",
    "recovery_analysis": {
        "previous_attempt_assessment": "Previous attempt failed",
        "state_changed": False,
        "current_signal_type": "same",
        "suggested_actions": ["retry_with_backoff", "escalate_if_fails_again"],
        "confidence_adjustment": -0.05,
        "reasoning": "Mock recovery assessment"
    },
    "selected_workflow": {
        "workflow_id": "test-recovery-workflow-001",
        "name": "Recovery Workflow",
        "confidence": 0.75,
        "reasoning": "Mock recovery workflow",
        "labels": {
            "component": "*",
            "environment": "*",
            "severity": "*"
        }
    },
    "alternative_workflows": [],
    "warnings": [],
    "needs_human_review": False
}
```

### **Workflow ID Convention**

Use consistent test workflow IDs:
- Initial incidents: `test-workflow-001`, `test-workflow-002`, etc.
- Recovery attempts: `test-recovery-workflow-001`, `test-recovery-workflow-002`, etc.

**Why**: AIAnalysis E2E tests may seed matching workflows in DataStorage catalog using these IDs.

### **Wildcard Component Matching**

Use `"component": "*"` in workflow labels to match any component filter in workflow search.

**Context**: DataStorage was recently updated to support wildcard matching:
- Workflow with `component='*'` matches ANY search filter
- Workflow with `component='deployment'` matches only 'deployment'

This allows mock workflows to be universally applicable for E2E testing.

---

## ✅ **Acceptance Criteria**

1. **Incident Analysis Response**:
   - ✅ Includes `selected_workflow` object with all required fields
   - ✅ Includes `alternative_workflows` array (can be empty)
   - ✅ Workflow labels use wildcard (`*`) for universal matching
   - ✅ AIAnalysis controller transitions from "Investigating" → "Analyzing" → "Completed"

2. **Recovery Analysis Response**:
   - ✅ Includes `recovery_analysis` object with all required fields
   - ✅ Includes `selected_workflow` object
   - ✅ AIAnalysis controller populates RecoveryStatus in CR
   - ✅ Recovery flow tests pass (5/5)

3. **E2E Test Results**:
   - ✅ Recovery flow tests: 5/5 passing
   - ✅ Full flow tests: 4/4 passing
   - ✅ Overall E2E: 20-22/22 passing (91-100%)

---

## 🧪 **Testing Strategy**

### **Verification Steps**

1. **Update HAPI mock responses** with workflow data
2. **Rebuild HAPI image**: `podman build -t kubernaut-holmesgpt-api:test -f holmesgpt-api/Dockerfile .`
3. **Run AIAnalysis E2E tests**: `make test-e2e-aianalysis`
4. **Verify logs**:
   ```bash
   export KUBECONFIG=~/.kube/aianalysis-e2e-config
   kubectl logs -n kubernaut-system deployment/aianalysis-controller | grep "selected_workflow"
   kubectl logs -n kubernaut-system deployment/aianalysis-controller | grep "recovery_analysis"
   ```

### **Expected Controller Logs After Fix**

```
INFO   Processing Investigating phase  {"name": "test-analysis"}
INFO   Using recovery endpoint  {"attemptNumber": 1}
DEBUG  Populating RecoveryStatus from HAPI response  {"hasRecoveryAnalysis": true}
INFO   Phase changed  {"from": "Investigating", "to": "Analyzing"}
INFO   Selected workflow found  {"workflow_id": "test-workflow-001", "confidence": 0.85}
INFO   Phase changed  {"from": "Analyzing", "to": "Completed"}
```

---

## 📁 **Related Documents**

- **Diagnosis**: [DIAGNOSIS_AA_E2E_TEST_FAILURES.md](DIAGNOSIS_AA_E2E_TEST_FAILURES.md)
- **Previous HAPI Fix**: [RESPONSE_HAPI_RECOVERY_ENDPOINT_CONFIG_FIX.md](RESPONSE_HAPI_RECOVERY_ENDPOINT_CONFIG_FIX.md)
- **Test Breakdown**: [AA_TEST_BREAKDOWN_ALL_TIERS.md](AA_TEST_BREAKDOWN_ALL_TIERS.md)

---

## ❓ **Questions & Clarifications**

### **Q1: Should mock responses vary based on request parameters?**

**Answer**: For E2E testing, consistent responses are fine. However, if you want to test different scenarios:
- High confidence → `confidence: 0.85`
- Low confidence → `confidence: 0.50`
- Multiple attempts → `recovery_analysis.confidence_adjustment: -0.10`

### **Q2: What if DataStorage doesn't have matching workflows?**

**Answer**: AIAnalysis team will seed test workflows in E2E infrastructure setup. HAPI team just needs to return consistent workflow IDs that match what we seed.

**Recommendation**: Use the suggested IDs (`test-workflow-001`, `test-recovery-workflow-001`) for consistency.

### **Q3: Do we need to test failure scenarios?**

**Answer**: Not immediately. Focus on happy path for initial fix. We can add failure scenario testing later.

---

## 🚀 **Timeline & Priority**

**Priority**: High (blocks 41% of AIAnalysis E2E tests)

**Estimated Effort**: 1-2 hours
- Update mock responses: 30-60 minutes
- Test locally: 15-30 minutes
- Verify with AIAnalysis E2E: 15-30 minutes

**Expected Completion**: Within 1 business day

**Coordination**: AIAnalysis team will:
1. Seed matching test workflows in E2E setup
2. Rerun E2E tests after HAPI changes
3. Report results back to HAPI team

---

## 📞 **Contact**

**For Questions**:
- AIAnalysis Team Lead: [Contact via docs/handoff/]
- E2E Test Issues: See [DIAGNOSIS_AA_E2E_TEST_FAILURES.md](DIAGNOSIS_AA_E2E_TEST_FAILURES.md)

**Response Document**: Please create `RESPONSE_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md` when complete.

---

## 📝 **Summary**

**Request**: Add `selected_workflow` and `recovery_analysis` fields to HAPI mock responses
**Impact**: Unblocks 9/22 AIAnalysis E2E tests (41% of suite)
**Effort**: 1-2 hours
**Expected Result**: 20-22/22 E2E tests passing (91-100%)

Thank you for your collaboration! 🙏

---

**Status**: ✅ **HAPI INVESTIGATION COMPLETE - ALL FIELDS ALREADY EXIST**
**Created**: 2025-12-12
**HAPI Response**: [FINAL_HAPI_TO_AA_MOCK_FIELDS_ANALYSIS.md](FINAL_HAPI_TO_AA_MOCK_FIELDS_ANALYSIS.md)
**Finding**: All requested fields already implemented in HAPI mock responses
**Next Step**: AA team to run diagnostics to identify actual root cause of test failures

---

## 🔍 **HAPI TEAM INVESTIGATION RESULTS** (2025-12-13)

**Status**: ✅ **COMPLETE - NO HAPI CHANGES NEEDED**

**Finding**: After reviewing `holmesgpt-api/src/mock_responses.py`, ALL requested fields already exist:

### Incident Analysis Response - ALL PRESENT ✅
- ✅ `selected_workflow` (line 300)
- ✅ `selected_workflow.workflow_id`
- ✅ `selected_workflow.title`
- ✅ `selected_workflow.confidence`
- ✅ `alternative_workflows` (line 309)
- ✅ `warnings` (line 320)
- ✅ `target_in_owner_chain` (line 319)
- ✅ `needs_human_review` (line 324)

### Recovery Analysis Response - ALL PRESENT ✅
- ✅ `recovery_analysis` (line 617)
- ✅ `recovery_analysis.previous_attempt_assessment`
- ✅ `recovery_analysis.state_changed`
- ✅ `recovery_analysis.current_signal_type`
- ✅ `selected_workflow` (line 627)
- ✅ `alternative_workflows` (line 639)
- ✅ `warnings` (line 642)
- ✅ `needs_human_review` (line 646)

**Conclusion**: AA test failures are NOT due to missing HAPI response fields.

---

## 🎯 **RECOMMENDED NEXT STEPS FOR AA TEAM**

Since HAPI already returns all requested fields, please investigate:

1. **Verify Mock Mode is Enabled**:
   ```bash
   kubectl get deployment holmesgpt-api -n kubernaut-system -o yaml | grep MOCK_LLM_MODE
   ```

2. **Test HAPI Response Directly**:
   ```bash
   kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:8080
   curl -X POST http://localhost:8080/api/v1/incident/analyze \
     -H "Content-Type: application/json" \
     -d '{"incident_id": "test", "signal_type": "OOMKilled",
          "resource_namespace": "default", "resource_name": "test-pod",
          "resource_kind": "Pod"}' | jq '.selected_workflow'
   ```

3. **Check Field Name Mapping**:
   - HAPI uses: `rationale` (not `reasoning`)
   - HAPI uses: `title` (not `name`)
   - Check if AA controller expects different names

4. **Review Controller Logs**:
   ```bash
   kubectl logs -n kubernaut-system deployment/aianalysis-controller | grep -E "selected_workflow|error"
   ```

**Please create**: `RESPONSE_AA_DIAGNOSTIC_RESULTS.md` with findings

---

**Updated**: 2025-12-13
**HAPI Status**: ✅ All fields present - no changes needed
**Awaiting**: AA team diagnostic results
