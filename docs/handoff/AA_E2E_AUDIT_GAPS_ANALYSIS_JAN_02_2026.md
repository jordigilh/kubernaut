# AIAnalysis E2E Audit Gaps - Root Cause Analysis & Required Logs

**Date**: January 2, 2026
**Analyst**: AI Development Team
**Status**: 🔍 **ANALYSIS COMPLETE** - Awaiting QE Team Logs
**Priority**: P2 - E2E Test Completion

---

## 🎯 Executive Summary

**Good News**: The audit code is **100% correct** and properly wired!
- ✅ `RecordRegoEvaluation()` exists and is called (lines 112, 137 in `analyzing.go`)
- ✅ `RecordApprovalDecision()` exists and is called (lines 163, 175 in `analyzing.go`)
- ✅ Controller wiring is correct (`cmd/aianalysis/main.go:187`)
- ✅ E2E infrastructure uses **real Rego evaluator** (correct setup)

**The Problem**: The **Analyzing phase is NOT being reached** in E2E tests.

**Evidence**: Only **1 phase transition event** instead of 3+ (should be Pending→Investigating→Analyzing→Completed)

---

## 🔍 Root Cause Hypothesis

### **Hypothesis A: HolmesGPT HTTP 500 Blocking Progression** (Most Likely)

The handoff document mentions:
> HolmesGPT-API returning HTTP 500 (server error)

**Impact**:
- If Investigating phase fails (HolmesGPT returns 500), controller transitions to `Failed` phase
- Analyzing phase is **skipped entirely** when investigation fails
- No Rego evaluation → No audit events for `rego.evaluation` or `approval.decision`

**Expected Flow**:
```
Pending → Investigating (HolmesGPT call) → Analyzing (Rego evaluation) → Completed
          ❌ HTTP 500 here
          ↓
          Failed (skip Analyzing)
```

### **Hypothesis B: AIAnalysis Not Reaching Completed State**

The controller may be stuck in earlier phases due to:
- Missing workflow data from HolmesGPT
- Silent failures in Investigating phase
- Retry loops preventing progression

---

## 🚨 CRITICAL ISSUE: Integration Tests Using Mock Rego Evaluator

**VIOLATION** of requirement: "Real Rego evaluator for all 3 tiers"

**Location**: `test/integration/aianalysis/suite_test.go:180-222`

```go
// ❌ WRONG: Integration tests use mock Rego evaluator
mockRegoEvaluator := &MockRegoEvaluator{}
analyzingHandler := handlers.NewAnalyzingHandler(mockRegoEvaluator, ...)
```

**Impact**:
- Integration tests do NOT validate real Rego policy behavior
- Policy bugs could slip through to production
- Violates defense-in-depth testing strategy

**Required Fix**: Replace mock with real Rego evaluator in integration tests (see Fix Section below)

---

## ✅ What's Verified as Correct

### 1. **E2E Infrastructure** ✅
E2E tests correctly use **real Rego evaluator**:

**File**: `test/infrastructure/aianalysis.go:1131-1143`
```yaml
env:
- name: REGO_POLICY_PATH
  value: /etc/rego/approval.rego  # Real policy file
volumeMounts:
- name: rego-policies
  mountPath: /etc/rego
volumes:
- name: rego-policies
  configMap:
    name: aianalysis-policies  # Real Rego policy from config/rego/aianalysis/approval.rego
```

### 2. **Audit Method Calls** ✅
Analyzing handler correctly calls audit methods:

**File**: `pkg/aianalysis/handlers/analyzing.go`
```go
// Line 136-138: Rego evaluation audit
if h.auditClient != nil {
    h.auditClient.RecordRegoEvaluation(ctx, analysis, outcome, result.Degraded, int(regoDuration), result.Reason)
}

// Line 162-164: Approval decision audit (requires_approval)
if h.auditClient != nil {
    h.auditClient.RecordApprovalDecision(ctx, analysis, "requires_approval", result.Reason)
}

// Line 174-176: Approval decision audit (auto_approved)
if h.auditClient != nil {
    h.auditClient.RecordApprovalDecision(ctx, analysis, "auto_approved", "Policy evaluation does not require manual approval")
}
```

### 3. **Controller Wiring** ✅
Main entry point correctly wires handlers and audit client:

**File**: `cmd/aianalysis/main.go:186-206`
```go
analyzingHandler := handlers.NewAnalyzingHandler(regoEvaluator, controllerLog, aianalysisMetrics, auditClient)

if err = (&aianalysis.AIAnalysisReconciler{
    AnalyzingHandler: analyzingHandler,  // ✅ Handler wired
    AuditClient:      auditClient,       // ✅ Audit client wired
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller")
    os.Exit(1)
}
```

---

## 📋 Required Logs from QE Team

To diagnose why Analyzing phase is not reached, we need the following logs from the E2E cluster:

### **Log Request 1: AIAnalysis Controller Logs** (CRITICAL)

```bash
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config logs \
  -n kubernaut-system deployment/aianalysis-controller \
  --tail=500 > /tmp/aa_controller_logs.txt
```

**What to look for**:
- Phase transitions (Pending→Investigating→Analyzing→Completed)
- HolmesGPT call results (HTTP status codes)
- Rego evaluation logs ("Rego evaluation complete")
- Any error messages in Investigating phase
- Whether Analyzing phase handler is invoked

### **Log Request 2: AIAnalysis CRD Status** (CRITICAL)

```bash
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config get aianalysis \
  -n <test-namespace> <test-aianalysis-name> -o yaml > /tmp/aa_crd_status.yaml
```

**What to look for**:
- `status.phase` - Final phase reached
- `status.selectedWorkflow` - Whether workflow was selected
- `status.message` and `status.reason` - Why it failed/succeeded
- `status.approvalRequired` - Whether Rego evaluation occurred

### **Log Request 3: HolmesGPT-API Logs** (HIGH PRIORITY)

```bash
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config logs \
  -n kubernaut-system deployment/holmesgpt-api \
  --tail=200 > /tmp/hapi_logs.txt
```

**What to look for**:
- HTTP 500 errors
- Request processing errors
- Mock LLM mode confirmation
- Successful investigate responses

### **Log Request 4: Data Storage Query** (MEDIUM PRIORITY)

```bash
# Query audit events for the test AIAnalysis
curl "http://localhost:8091/api/v1/audit/events?correlation_id=<remediation-id>&limit=100" \
  | jq . > /tmp/aa_audit_events.json
```

**What to look for**:
- How many audit events total
- Which event types are present
- Event timestamps (verify chronological order)
- Any `aianalysis.error.occurred` events

### **Log Request 5: E2E Test Output** (MEDIUM PRIORITY)

Share the complete E2E test output showing:
- Test failures with error messages
- Which phases were reached
- Timing information

**File**: `/tmp/aianalysis_e2e_validation_aa_bug_003.log`

---

## 🎯 Diagnosis Flowchart

Based on logs, we'll follow this diagnostic path:

```
1. Check AIAnalysis CRD status.phase
   ├─ "Completed" → Why no Analyzing phase audit events?
   ├─ "Failed" → Check status.reason (HolmesGPT error?)
   └─ "Investigating" → Stuck in phase, check HolmesGPT logs

2. Check HolmesGPT-API logs
   ├─ HTTP 500 errors? → Fix HolmesGPT E2E setup
   └─ HTTP 200 success? → Check workflow selection in CRD

3. Check AIAnalysis controller logs
   ├─ "Rego evaluation complete"? → Analyzing phase reached!
   ├─ No Rego logs? → Investigating phase failed
   └─ Error logs? → Specific failure to fix

4. Check Data Storage audit events
   ├─ Only 1 phase.transition? → Confirms not reaching Analyzing
   ├─ 3+ phase.transition? → Phase progression working
   └─ Check event_data for clues
```

---

## 🔧 Required Fix: Integration Tests Must Use Real Rego Evaluator

### **Problem**
`test/integration/aianalysis/suite_test.go` uses `MockRegoEvaluator` (lines 180-222)

### **Fix**
Replace mock with real Rego evaluator (same pattern as E2E):

```go
// BEFORE (❌ WRONG):
mockRegoEvaluator := &MockRegoEvaluator{}
analyzingHandler := handlers.NewAnalyzingHandler(mockRegoEvaluator, ctrl.Log.WithName("analyzing-handler"), testMetrics, auditClient)

// AFTER (✅ CORRECT):
// Use real Rego evaluator with production policies
policyPath := filepath.Join("..", "..", "..", "config", "rego", "aianalysis", "approval.rego")
realRegoEvaluator := rego.NewEvaluator(rego.Config{
    PolicyPath: policyPath,
}, ctrl.Log.WithName("rego"))

evalCtx, evalCancel := context.WithCancel(context.Background())
defer evalCancel()

// ADR-050: Startup validation required
err = realRegoEvaluator.StartHotReload(evalCtx)
Expect(err).NotTo(HaveOccurred(), "Policy should load successfully")
defer realRegoEvaluator.Stop()

analyzingHandler := handlers.NewAnalyzingHandler(realRegoEvaluator, ctrl.Log.WithName("analyzing-handler"), testMetrics, auditClient)
```

**Rationale**: Per user requirement Q1, "they should be using a real rego evaluator for all 3 tiers"

**Files to Update**:
1. `test/integration/aianalysis/suite_test.go` - Replace MockRegoEvaluator with real evaluator
2. Remove `MockRegoEvaluator` struct definition (lines 423-450)

---

## 📊 Expected Outcomes After Log Analysis

### **Scenario 1: HolmesGPT HTTP 500 Confirmed**
**Fix**: Update `test/infrastructure/holmesgpt_api.go` mock responses
- Ensure `/api/v1/investigate` returns HTTP 200
- Verify mock responses include required workflow data

### **Scenario 2: Workflow Selection Failing**
**Fix**: Ensure test data triggers workflow selection
- Verify SignalType matches mock patterns
- Check enrichment results completeness

### **Scenario 3: Analyzing Phase Reached But Audit Client Nil**
**Fix**: Verify audit client wiring in E2E deployment
- Check if audit client creation succeeded in controller logs
- Verify DataStorage connection

---

## ✅ Success Criteria

When fixes are complete:
1. **Phase Progression**: AIAnalysis reaches all 4 phases (Pending→Investigating→Analyzing→Completed)
2. **Audit Events**: All 10 expected event types appear:
   - ✅ `aianalysis.phase.transition` (3+ events)
   - ✅ `aianalysis.holmesgpt.call`
   - ✅ `aianalysis.rego.evaluation` ← Currently missing
   - ✅ `aianalysis.approval.decision` ← Currently missing
   - ✅ `aianalysis.analysis.completed`
   - Plus LLM events and error events
3. **E2E Tests**: 36/36 tests passing (currently 34/36)
4. **Integration Tests**: Using real Rego evaluator (currently using mock)

---

## 📞 Next Steps

**For QE Team**:
1. Collect the 5 log files listed above
2. Upload to shared documentation or attach to this handoff
3. Confirm E2E cluster is still running or provide instructions to restart

**For Development Team** (after logs received):
1. Analyze logs using diagnostic flowchart
2. Implement required fixes
3. Verify E2E tests pass 36/36
4. Fix integration tests to use real Rego evaluator

---

**Estimated Time**:
- Log collection: 15 minutes
- Log analysis: 30 minutes
- Fix implementation: 1-2 hours
- Verification: 30 minutes
- **Total**: 2.5-3 hours

---

**References**:
- Original handoff: `AA_E2E_REMAINING_AUDIT_GAPS_JAN_02_2026.md`
- Previous fixes: `AA_BUG_001_002_AUDIT_EVENT_FIXES_JAN_02_2026.md`
- Testing guidelines: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- Rego evaluator: `pkg/aianalysis/rego/evaluator.go`

