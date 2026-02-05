# Mock LLM Scenario Control - Triage and Fix Plan

**Date**: February 2, 2026  
**Issue**: 3 tests incorrectly skipped (`XIt`) due to misunderstanding of Mock LLM scenario control  
**Status**: ✅ **TRIAGE COMPLETE** - Fix plan identified

---

## 🔍 **Discovery**

During AIAnalysis integration test refactoring (move from mocks to real HAPI), 3 tests were marked as `XIt` (skipped) with the rationale:

> "Mock LLM returns deterministic responses based on signal type. Cannot force specific human_review_reason values without controlling Mock LLM scenarios."

**USER INSIGHT**: "We DO control those. That's the purpose of the mockLLM."

**Result**: User was correct! Mock LLM HAS built-in scenario control via special signal types.

---

## 🏗️ **How Mock LLM Scenario Control Works**

### **Architecture Pattern: DD-TEST-011 v2.0 (File-Based Configuration)**

```
┌─────────────────────────────────────────────────────┐
│ Phase 1: Bootstrap (Process 1 Only)                │
├─────────────────────────────────────────────────────┤
│ 1. Seed workflows in DataStorage                   │
│    → Returns actual UUIDs (server-generated)       │
│                                                     │
│ 2. Write mock-llm-config.yaml                      │
│    Format: "workflow_name:environment" → "uuid"    │
│    Example:                                         │
│      scenarios:                                     │
│        oomkill-increase-memory-v1:staging: uuid1   │
│        crashloop-config-fix-v1:production: uuid2   │
│                                                     │
│ 3. Start Mock LLM container with config mounted    │
│    → Mock LLM reads file at startup                │
│    → Updates MOCK_SCENARIOS dict with real UUIDs   │
└─────────────────────────────────────────────────────┘
```

### **Mock LLM Scenario Matching Logic**

**Location**: `test/services/mock-llm/src/server.py` (lines 605-694)

Mock LLM matches incoming requests to scenarios using **signal type detection**:

```python
def _detect_scenario(self, messages: List[Dict[str, Any]]) -> MockScenario:
    content = " ".join(str(m.get("content", "")) for m in messages).lower()
    
    # Check for test-specific signal types FIRST (human review tests)
    if "mock_no_workflow_found" in content:
        return MOCK_SCENARIOS.get("no_workflow_found", DEFAULT_SCENARIO)
    if "mock_low_confidence" in content:
        return MOCK_SCENARIOS.get("low_confidence", DEFAULT_SCENARIO)
    if "mock_problem_resolved" in content:
        return MOCK_SCENARIOS.get("problem_resolved", DEFAULT_SCENARIO)
    if "mock_max_retries_exhausted" in content:
        return MOCK_SCENARIOS.get("max_retries_exhausted", DEFAULT_SCENARIO)
    
    # Then check for normal signal types
    if "crashloop" in content:
        return MOCK_SCENARIOS.get("crashloop", DEFAULT_SCENARIO)
    elif "oomkilled" in content:
        return MOCK_SCENARIOS.get("oomkilled", DEFAULT_SCENARIO)
    # ... etc
```

---

## 🎯 **Built-In Test Scenarios**

### **Scenario 1: `no_workflow_found` (Lines 151-163)**

```python
"no_workflow_found": MockScenario(
    name="no_workflow_found",
    signal_type="MOCK_NO_WORKFLOW_FOUND",  # ← Trigger
    workflow_id="",  # Empty workflow_id indicates no workflow found
    confidence=0.0,  # Zero confidence triggers human review
    root_cause="No suitable workflow found in catalog for this signal type",
)
```

**How to trigger**: Use signal type `"MOCK_NO_WORKFLOW_FOUND"` in test request  
**Response**:
- `selected_workflow`: `null`
- `confidence`: `0.0`
- `needs_human_review`: `true` (set by HAPI based on confidence < 0.7)
- `human_review_reason`: `"no_matching_workflows"` (set by HAPI)

---

### **Scenario 2: `low_confidence` (Lines 164-177)**

```python
"low_confidence": MockScenario(
    name="low_confidence",
    workflow_name="generic-restart-v1",
    signal_type="MOCK_LOW_CONFIDENCE",  # ← Trigger
    workflow_id="<real-uuid-from-config>",  # Gets real UUID at startup
    confidence=0.35,  # Low confidence (<0.5) triggers human review
    root_cause="Multiple possible root causes identified, requires human judgment",
)
```

**How to trigger**: Use signal type `"MOCK_LOW_CONFIDENCE"` in test request  
**Response**:
- `selected_workflow`: { workflow_id, title, confidence: 0.35, ... }
- `alternative_workflows`: [array of 2 alternatives]
- `needs_human_review`: `true` (confidence < 0.7)
- `human_review_reason`: `"low_confidence"` (set by HAPI)

---

### **Scenario 3: `problem_resolved` (Lines 178-191)**

```python
"problem_resolved": MockScenario(
    name="problem_resolved",
    signal_type="MOCK_PROBLEM_RESOLVED",  # ← Trigger
    workflow_id="",  # Empty - no workflow needed
    confidence=0.85,  # HIGH confidence (>= 0.7) that problem is resolved
    root_cause="Problem self-resolved through auto-scaling or transient condition cleared",
)
```

**How to trigger**: Use signal type `"MOCK_PROBLEM_RESOLVED"` in test request  
**Response** (Lines 903-921):
- `selected_workflow`: `null`
- `investigation_outcome`: `"resolved"` (BR-HAPI-200)
- `confidence`: `0.85`
- `can_recover`: `false` (no recovery needed)

---

### **Scenario 4: `max_retries_exhausted` (Lines 193-206)**

```python
"max_retries_exhausted": MockScenario(
    name="max_retries_exhausted",
    signal_type="MOCK_MAX_RETRIES_EXHAUSTED",  # ← Trigger
    workflow_id="",  # Empty - couldn't parse/select workflow
    confidence=0.0,  # Zero confidence indicates parsing failure
    root_cause="LLM analysis completed but failed validation after maximum retry attempts",
)
```

**How to trigger**: Use signal type `"MOCK_MAX_RETRIES_EXHAUSTED"` in test request  
**Response** (Lines 976-1004):
- `selected_workflow`: `null`
- `confidence`: `0.0`
- `needs_human_review`: `true`
- `human_review_reason`: `"llm_parsing_error"`
- `validation_attempts_history`: [array of 3 failed validation attempts]

---

## ❌ **Why The 3 Tests Were Incorrectly Skipped**

### **Test 1: "all 7 human_review_reason enum values" (Line 212)**

```go
XIt("should handle all 7 human_review_reason enum values - BR-HAPI-197", func() {
    // SKIP: Mock LLM returns deterministic responses based on signal type
    // Cannot force specific human_review_reason values without controlling Mock LLM scenarios
```

**INCORRECT REASONING**: We CAN force specific `human_review_reason` values!

**ACTUAL BEHAVIOR**:
- `"workflow_not_found"`: Use signal type `"MOCK_NO_WORKFLOW_FOUND"` (triggers `no_workflow_found` scenario)
- `"low_confidence"`: Use signal type `"MOCK_LOW_CONFIDENCE"` (triggers `low_confidence` scenario)
- `"llm_parsing_error"`: Use signal type `"MOCK_MAX_RETRIES_EXHAUSTED"` (triggers `max_retries_exhausted` scenario)
- `"investigation_inconclusive"`: Use signal type `"Unknown"` or similar vague input
- `"no_matching_workflows"`: Use signal type `"MOCK_NO_WORKFLOW_FOUND"`
- `"image_mismatch"`: **NOT directly testable** (HAPI business logic determines this based on workflow catalog validation)
- `"parameter_validation_failed"`: **NOT directly testable** (HAPI business logic determines this)

**VERDICT**: 5 out of 7 enum values CAN be tested. 2 require HAPI E2E suite (business logic validation).

---

### **Test 2: "problem resolved scenario" (Line 254)**

```go
XIt("should handle problem resolved scenario (no workflow needed)", func() {
    // SKIP: Mock LLM returns workflows based on signal type
    // Cannot force "problem resolved" scenario without specific Mock LLM configuration
```

**INCORRECT REASONING**: We CAN force "problem resolved" scenario!

**FIX**: Use signal type `"MOCK_PROBLEM_RESOLVED"`

```go
It("should handle problem resolved scenario (no workflow needed)", func() {
    resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
        SignalType:  "MOCK_PROBLEM_RESOLVED", // ← Triggers problem_resolved scenario
        // ... other fields ...
    })
    
    Expect(err).NotTo(HaveOccurred())
    Expect(resp.SelectedWorkflow.Set).To(BeFalse(), "No workflow should be selected")
    Expect(resp.Confidence).To(BeNumerically(">=", 0.7), "High confidence problem is resolved")
    // BR-HAPI-200: Check investigation_outcome field
})
```

---

### **Test 3: "server failures" (Line 384)**

```go
XIt("should return error for server failures - BR-AI-009", func() {
    // SKIP: Cannot simulate server failures without stopping HAPI container
    // Server error handling better tested in HAPI E2E suite with chaos engineering
```

**CORRECT REASONING**: This one IS legitimately difficult to test in integration tests.

**VERDICT**: Keep skipped (XIt) - requires infrastructure manipulation (chaos engineering).

---

## ✅ **Fix Plan**

### **Phase 1: Un-skip 2 Tests (Immediate)**

1. **Test 1**: "all 7 human_review_reason enum values"
   - Change `XIt` → `It`
   - Test 5/7 enum values using Mock LLM scenarios
   - Document that 2 values (`image_mismatch`, `parameter_validation_failed`) require HAPI E2E

2. **Test 2**: "problem resolved scenario"
   - Change `XIt` → `It`
   - Use `SignalType: "MOCK_PROBLEM_RESOLVED"`

3. **Test 3**: "server failures"
   - Keep as `XIt` (legitimately requires chaos engineering)

---

### **Phase 2: Enhanced Test Coverage (Optional)**

Add additional test cases for other Mock LLM scenarios:

```go
Context("Mock LLM Scenario Control - BR-HAPI-197", func() {
    It("should handle no_workflow_found scenario", func() {
        resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
            SignalType: "MOCK_NO_WORKFLOW_FOUND",
            // ... other fields ...
        })
        
        Expect(err).NotTo(HaveOccurred())
        Expect(resp.SelectedWorkflow.Set).To(BeFalse())
        Expect(resp.NeedsHumanReview.Value).To(BeTrue())
        Expect(string(resp.HumanReviewReason.Value)).To(Equal("no_matching_workflows"))
    })
    
    It("should handle low_confidence scenario with alternatives", func() {
        resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
            SignalType: "MOCK_LOW_CONFIDENCE",
            // ... other fields ...
        })
        
        Expect(err).NotTo(HaveOccurred())
        Expect(resp.SelectedWorkflow.Set).To(BeTrue())
        Expect(resp.Confidence).To(BeNumerically("<", 0.5))
        Expect(resp.NeedsHumanReview.Value).To(BeTrue())
        Expect(string(resp.HumanReviewReason.Value)).To(Equal("low_confidence"))
        // E2E-HAPI-002: Verify alternative_workflows field exists
    })
    
    It("should handle max_retries_exhausted with validation history", func() {
        resp, err := realHGClient.Investigate(testCtx, &client.IncidentRequest{
            SignalType: "MOCK_MAX_RETRIES_EXHAUSTED",
            // ... other fields ...
        })
        
        Expect(err).NotTo(HaveOccurred())
        Expect(resp.NeedsHumanReview.Value).To(BeTrue())
        Expect(string(resp.HumanReviewReason.Value)).To(Equal("llm_parsing_error"))
        // E2E-HAPI-003: Verify validation_attempts_history exists
    })
})
```

---

## 📋 **Mock LLM Scenario Reference**

| Scenario Name | Trigger Signal Type | Workflow ID | Confidence | Human Review | Review Reason |
|---------------|---------------------|-------------|------------|--------------|---------------|
| `oomkilled` | `"OOMKilled"` | real UUID | 0.95 | false | - |
| `crashloop` | `"CrashLoopBackOff"` | real UUID | 0.88 | false | - |
| `node_not_ready` | `"NodeNotReady"` | real UUID | 0.90 | false | - |
| `recovery` | `"OOMKilled"` + recovery context | real UUID | 0.85 | false | - |
| `test_signal` | `"TestSignal"` | real UUID | 0.90 | false | - |
| `no_workflow_found` | `"MOCK_NO_WORKFLOW_FOUND"` | empty | 0.0 | true | `no_matching_workflows` |
| `low_confidence` | `"MOCK_LOW_CONFIDENCE"` | real UUID | 0.35 | true | `low_confidence` |
| `problem_resolved` | `"MOCK_PROBLEM_RESOLVED"` | empty | 0.85 | false | - |
| `max_retries_exhausted` | `"MOCK_MAX_RETRIES_EXHAUSTED"` | empty | 0.0 | true | `llm_parsing_error` |
| `rca_incomplete` | `"MOCK_RCA_INCOMPLETE"` | real UUID | 0.88 | false | - |

---

## 🔗 **Key Files**

- **Mock LLM Scenarios**: `test/services/mock-llm/src/server.py` (lines 60-316)
- **Scenario Detection**: `test/services/mock-llm/src/server.py` (lines 605-694)
- **Workflow Bootstrap**: `test/integration/aianalysis/test_workflows.go` (lines 56-139)
- **Config File Writer**: `test/integration/aianalysis/test_workflows.go` (lines 183-200)
- **Suite Setup**: `test/integration/aianalysis/suite_test.go` (lines 400-433)

---

## 🎓 **Lessons Learned**

1. **Always verify assumptions about external dependencies** - Mock LLM had extensive scenario control capabilities that were overlooked.

2. **Read the source code** - Mock LLM's `server.py` clearly documents all available scenarios and their triggers.

3. **File-based configuration is powerful** - DD-TEST-011 v2.0 pattern allows complete control over Mock LLM behavior without HTTP calls or timing issues.

4. **Test scenarios != production scenarios** - Mock LLM has special `MOCK_*` signal types specifically for testing edge cases.

---

## ✅ **Next Steps**

1. **Immediate**: Un-skip 2 tests (enum values, problem resolved)
2. **Quick win**: Add test cases for other Mock LLM scenarios
3. **Documentation**: Update testing guidelines with Mock LLM scenario reference
4. **Validation**: Run integration tests to verify fixes

---

**Status**: Ready for implementation  
**Confidence**: 95% (Mock LLM scenarios are well-documented and tested)  
**Risk**: Low (using existing, tested Mock LLM functionality)
