# AIAnalysis QE Team - Next Steps for E2E Debugging

**Date**: January 2, 2026  
**Status**: 🟢 **INTEGRATION TESTS FIXED** - Ready for E2E Investigation  
**For**: QE Team  

---

## ✅ **Integration Tests - COMPLETE**

**Status**: All 54 integration tests now passing with real Rego evaluator!

**What was fixed**:
- Replaced mock Rego evaluator with real production policies
- All audit events now being created correctly:
  - ✅ `aianalysis.rego.evaluation`
  - ✅ `aianalysis.approval.decision`
  - ✅ `aianalysis.phase.transition`
  - ✅ `aianalysis.analysis.completed`

---

## 🎯 **Next: E2E Tests Investigation**

### Current E2E Status
- **34/36 tests passing** (94.4%)
- **2 tests failing** - missing audit events:
  1. `aianalysis.rego.evaluation` not appearing
  2. `aianalysis.approval.decision` not appearing

### Root Cause Hypothesis
**The Analyzing phase is not being reached in E2E tests**

**Evidence**:
- Only **1 phase transition event** (should be 3+ for full cycle)
- Log excerpt shows: `"aianalysis.phase.transition": 1`
- Expected: Pending → Investigating → Analyzing → Completed (3 transitions)

**Most Likely Cause**: HolmesGPT HTTP 500 errors blocking the Investigating phase

---

## 📋 **Required: Log Collection**

To diagnose the E2E issue, we need these 5 log files:

### 1. AIAnalysis Controller Logs (CRITICAL)
```bash
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config logs \
  -n kubernaut-system deployment/aianalysis-controller \
  --tail=500 > /tmp/aa_controller_logs.txt
```

**What we're looking for**:
- Which phases are reached (Pending, Investigating, Analyzing, Completed)
- HolmesGPT call results (HTTP status codes)
- Any "Rego evaluation complete" logs
- Error messages in Investigating phase

### 2. AIAnalysis CRD Status (CRITICAL)
```bash
# Get the AIAnalysis CR name from the failing test
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config get aianalysis -A

# Then get its full status
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config get aianalysis \
  -n <namespace> <name> -o yaml > /tmp/aa_crd_status.yaml
```

**What we're looking for**:
- `status.phase` - What phase did it reach?
- `status.selectedWorkflow` - Did investigation succeed?
- `status.message` and `status.reason` - Why did it stop?

### 3. HolmesGPT-API Logs (HIGH PRIORITY)
```bash
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config logs \
  -n kubernaut-system deployment/holmesgpt-api \
  --tail=200 > /tmp/hapi_logs.txt
```

**What we're looking for**:
- HTTP 500 errors (confirms our hypothesis)
- Request processing errors
- Mock LLM mode confirmation

### 4. Data Storage Audit Query (MEDIUM)
```bash
# Query all audit events for the test
curl "http://localhost:8091/api/v1/audit/events?correlation_id=<remediation-id>&limit=100" \
  | jq . > /tmp/aa_audit_events.json
```

**What we're looking for**:
- Total event count
- Which event types are present
- Event timestamps (chronological order)

### 5. Complete E2E Test Output (MEDIUM)
The full test output file:
```bash
cat /tmp/aianalysis_e2e_validation_aa_bug_003.log > aa_e2e_complete_output.txt
```

---

## 🔧 **How to Keep E2E Cluster Running**

The E2E test suite is configured to preserve the cluster on failure:

```bash
# Check if cluster is still running
kind get clusters | grep aianalysis-e2e

# If cluster exists, logs can be collected using commands above
# If cluster was deleted, you'll need to re-run tests:

# Set environment variable to keep cluster
export KEEP_CLUSTER=1

# Run E2E tests
make test-e2e-aianalysis

# Cluster will be preserved even if tests fail
```

---

## 📊 **Diagnostic Flowchart**

Once we have the logs, follow this diagnostic path:

```
1. Check AIAnalysis CRD status.phase
   ├─ "Completed" → Why no Analyzing events? (Check controller logs)
   ├─ "Failed" → Check status.reason (likely HolmesGPT error)
   └─ "Investigating" → Stuck in phase (check HolmesGPT logs)

2. Check HolmesGPT-API logs
   ├─ HTTP 500 errors? → Fix HolmesGPT E2E mock setup
   └─ HTTP 200 success? → Check workflow selection in CRD

3. Check AIAnalysis controller logs
   ├─ "Rego evaluation complete"? → Analyzing phase WAS reached!
   ├─ No Rego logs? → Investigating failed, stopped early
   └─ Error logs? → Specific failure to fix

4. Check audit events in Data Storage
   ├─ How many phase.transition events? (should be 3+)
   ├─ Are there any error.occurred events?
   └─ What's the timeline of events?
```

---

## 🎯 **Expected Timeline**

Once logs are provided:

1. **Log Analysis**: 30 minutes
2. **Root Cause Confirmation**: 15 minutes
3. **Fix Implementation**: 1-2 hours
4. **Verification**: 30 minutes
5. **Total**: 2.5-3 hours

---

## 💡 **Key Insights from Integration Test Fix**

What we learned that applies to E2E:

1. **The audit code itself is perfect** ✅
   - No changes needed to audit methods
   - Controller wiring is correct
   - E2E infrastructure uses real Rego evaluator

2. **The issue is business logic flow** ❌
   - Not reaching Analyzing phase
   - Likely blocked at Investigating phase

3. **HolmesGPT HTTP 500 is the smoking gun** 🔍
   - Mentioned in original handoff document
   - Would explain skipping Analyzing phase
   - Need logs to confirm

---

## 📞 **Contact & Questions**

**To share logs**:
1. Upload the 5 log files to shared location
2. Update `AA_E2E_AUDIT_GAPS_ANALYSIS_JAN_02_2026.md` with log file paths
3. Or attach to this handoff document

**Questions?**
- Review analysis: `AA_E2E_AUDIT_GAPS_ANALYSIS_JAN_02_2026.md`
- Integration fix: `AA_INTEGRATION_TEST_REGO_FIX_JAN_02_2026.md`
- Testing guidelines: `docs/development/business-requirements/TESTING_GUIDELINES.md`

---

**Status**: ⏳ Awaiting QE Team log collection to proceed with E2E fix

