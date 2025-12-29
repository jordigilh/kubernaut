# 🎯 ROOT CAUSE FOUND: Field Name Mismatch in Recovery Request

**Date**: 2025-12-13
**Status**: ✅ **ROOT CAUSE IDENTIFIED**
**Responsibility**: **BOTH TEAMS** (Contract mismatch)

---

## 🚨 **THE BUG**

### **AIAnalysis Controller Sends** (Go):
```go
// pkg/aianalysis/client/holmesgpt.go
type RecoveryRequest struct {
    IncidentID        string             `json:"incident_id"`
    RemediationID     string             `json:"remediation_id"`
    SignalType        *string            `json:"signal_type,omitempty"`
    PreviousExecution *PreviousExecution `json:"previous_execution,omitempty"`  // ← OBJECT
    // ...
}

type PreviousExecution struct {
    WorkflowExecutionRef string                  `json:"workflow_execution_ref"`
    OriginalRCA          OriginalRCA             `json:"original_rca"`
    SelectedWorkflow     SelectedWorkflowSummary `json:"selected_workflow"`  // ← Nested
    Failure              ExecutionFailure        `json:"failure"`
}
```

**Example JSON Sent**:
```json
{
  "incident_id": "e2e-recovery-basic-abc123",
  "remediation_id": "e2e-recovery-rem-001",
  "signal_type": "OOMKilled",
  "previous_execution": {
    "workflow_execution_ref": "workflow-exec-001",
    "selected_workflow": {
      "workflow_id": "oomkill-increase-memory-v1",
      "container_image": "quay.io/kubernaut/workflow-oomkill:v1.0.0"
    },
    "failure": {...}
  }
}
```

---

### **HAPI Mock Response Expects** (Python):
```python
# holmesgpt-api/src/mock_responses.py line 556
def generate_mock_recovery_response(request_data: Dict[str, Any]) -> Dict[str, Any]:
    remediation_id = request_data.get("remediation_id", "mock-remediation-unknown")
    signal_type = request_data.get("signal_type", request_data.get("current_signal_type", "Unknown"))
    previous_workflow_id = request_data.get("previous_workflow_id", "unknown-workflow")  # ← WRONG!
    namespace = request_data.get("namespace", request_data.get("resource_namespace", "default"))
    incident_id = request_data.get("incident_id", "mock-incident-unknown")
```

**Expected JSON**:
```json
{
  "incident_id": "test-001",
  "remediation_id": "test-recovery-001",
  "signal_type": "OOMKilled",
  "previous_workflow_id": "mock-oomkill-increase-memory-v1",  // ← FLAT STRING
  "namespace": "production"
}
```

---

## 🔍 **The Mismatch**

| Field | AA Controller Sends | HAPI Mock Expects | Match? |
|-------|---------------------|-------------------|--------|
| `incident_id` | ✅ Yes | ✅ Yes | ✅ MATCH |
| `remediation_id` | ✅ Yes | ✅ Yes | ✅ MATCH |
| `signal_type` | ✅ Yes | ✅ Yes | ✅ MATCH |
| `previous_execution` | ✅ Yes (object) | ❌ No | ❌ **MISMATCH** |
| `previous_workflow_id` | ❌ No | ✅ Yes (string) | ❌ **MISMATCH** |
| `namespace` | ❌ No (sends `resource_namespace`) | ✅ Yes | ⚠️ **FALLBACK WORKS** |

---

## 💥 **What Happens**

1. **AA Controller** sends `previous_execution` object with nested `selected_workflow.workflow_id`
2. **HAPI Mock** looks for flat `previous_workflow_id` string
3. **HAPI Mock** doesn't find it, uses default: `"unknown-workflow"`
4. **HAPI Mock** generates response with `recovery_workflow_id = "unknown-workflow-recovery"`
5. **Response IS generated** with `selected_workflow` and `recovery_analysis` fields populated
6. **BUT** the workflow_id is wrong: `"unknown-workflow-recovery"` instead of `"oomkill-increase-memory-v1-recovery"`

**WAIT** - This doesn't explain why fields are `null`! Let me check if there's another code path...

---

## 🤔 **Re-Analysis Needed**

Actually, looking at the mock response code (lines 617-638), it DOES populate both fields regardless of `previous_workflow_id` value:

```python
response = {
    ...
    "recovery_analysis": {...},   # Always populated
    "selected_workflow": {...}     # Always populated
}
```

So the fields SHOULD be present even with wrong `previous_workflow_id`.

**This means there's ANOTHER issue causing the null fields!**

---

## 🔍 **Next Debugging Steps**

### **Option 1: Check if E2E test sends ANY previous execution data**

```bash
# Check test code
grep -A20 "PreviousExecutions" test/e2e/aianalysis/04_recovery_flow_test.go
```

**If test DOESN'T send `PreviousExecutions`**:
- Mock response might take a different code path
- Need to check if there's conditional logic based on previous execution presence

---

### **Option 2: Check HAPI's RecoveryRequest Pydantic model**

```bash
# Check if Pydantic model expects different field names
grep -A30 "class RecoveryRequest" holmesgpt-api/src/models/recovery_models.py
```

**If Pydantic model expects `previous_workflow_id`**:
- FastAPI validation might be rejecting the request
- Or converting `previous_execution` incorrectly

---

### **Option 3: Add debug logging to HAPI**

```python
# In holmesgpt-api/src/mock_responses.py
def generate_mock_recovery_response(request_data: Dict[str, Any]) -> Dict[str, Any]:
    logger.info(f"DEBUG: request_data keys: {list(request_data.keys())}")
    logger.info(f"DEBUG: previous_execution present: {'previous_execution' in request_data}")
    logger.info(f"DEBUG: previous_workflow_id present: {'previous_workflow_id' in request_data}")

    # ... rest of function
```

---

## 📝 **Responsibility Assignment**

### **Field Name Mismatch** (Contract Issue)

**Problem**: AA sends `previous_execution` (object), HAPI mock expects `previous_workflow_id` (string)

**Responsibility**: **BOTH TEAMS**
- **AA Team**: Update to send `previous_workflow_id` if that's the agreed contract
- **HAPI Team**: Update mock to extract from `previous_execution.selected_workflow.workflow_id`

**Recommended Fix**: **HAPI Team** should adapt mock to handle both formats:
```python
# Extract previous_workflow_id from either format
previous_workflow_id = request_data.get("previous_workflow_id")
if not previous_workflow_id and "previous_execution" in request_data:
    prev_exec = request_data.get("previous_execution", {})
    selected_wf = prev_exec.get("selected_workflow", {})
    previous_workflow_id = selected_wf.get("workflow_id", "unknown-workflow")
else:
    previous_workflow_id = previous_workflow_id or "unknown-workflow"
```

---

### **Null Fields Issue** (Still Unknown)

**Problem**: Fields are `null` despite mock response populating them

**Responsibility**: **UNKNOWN** (need more debugging)

**Next Steps**:
1. Check if E2E test sends `PreviousExecutions` at all
2. Check HAPI's Pydantic model validation
3. Add debug logging to see actual request received
4. Check if there's a code path that returns early with null fields

---

## 🎯 **Action Items**

### **Immediate** (5 minutes)

1. **Check E2E test**: Does it send `PreviousExecutions`?
   ```bash
   grep -B5 -A20 "PreviousExecutions" test/e2e/aianalysis/04_recovery_flow_test.go
   ```

2. **Check HAPI Pydantic model**: What fields does it expect?
   ```bash
   grep -A50 "class RecoveryRequest" holmesgpt-api/src/models/recovery_models.py
   ```

### **Short-term** (30 minutes)

3. **Add debug logging** to HAPI mock response generator
4. **Rerun E2E test** and capture HAPI logs
5. **Verify** what request HAPI actually receives

### **Fix** (1-2 hours)

6. **Update HAPI mock** to handle `previous_execution` object format
7. **OR Update AA controller** to send `previous_workflow_id` string
8. **Verify** fields are no longer null
9. **Document** the correct contract in OpenAPI spec

---

## 📊 **Confidence Assessment**

**Field Name Mismatch**: 95% confident this is A problem
**Null Fields Root Cause**: 30% confident this is THE problem (need more data)

**Why Low Confidence on Null Fields**:
- Mock response code shows fields ARE populated
- Need to see actual request received by HAPI
- Need to verify E2E test sends previous execution data
- Need to check if Pydantic validation is stripping fields

---

**Status**: 🔄 **PARTIAL ROOT CAUSE** - Found contract mismatch, but null fields issue needs more investigation

**Next**: Check E2E test code to see if it sends `PreviousExecutions` at all


