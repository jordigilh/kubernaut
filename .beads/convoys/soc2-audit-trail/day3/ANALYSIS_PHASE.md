# Day 3 ANALYSIS Phase: Workflow Selection & Execution (Gap 5-6)

**Formula**: integration-test-full-validation v2.1
**Phase**: ANALYSIS
**Duration**: 15 minutes
**Date**: January 6, 2026

---

## 📋 **Business Context**

### Business Requirements
- **BR-AUDIT-005 v2.0**: Gap 5-6 - Workflow references for RR reconstruction
- **BR-WE-013**: Audit-tracked workflow execution
- **SOC2 Compliance**: Complete RemediationRequest reconstruction from audit events

### Problem Statement
Currently, the audit trail lacks two critical pieces of information needed for complete RR reconstruction:
1. **Gap #5**: `selected_workflow_ref` - Which workflow was selected by the AI Analysis
2. **Gap #6**: `execution_ref` - Which Tekton PipelineRun was created for execution

### Business Value
- **SOC2 Type II Compliance**: Complete audit trail for remediation actions
- **Forensic Analysis**: Reconstruct exact workflow chosen and executed
- **Debugging Support**: Trace workflow selection → execution → results

---

## 🔍 **Technical Context**

### Existing Implementations Found

#### 1. WorkflowExecution Audit Infrastructure (ALREADY EXISTS) ✅
**Location**: `internal/controller/workflowexecution/audit.go`

```go
// RecordAuditEvent writes an audit event to the Data Storage Service
func (r *WorkflowExecutionReconciler) RecordAuditEvent(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    action string,
    category string,
) error
```

**Current Events Emitted**:
- `workflow.started` - When execution begins
- `workflow.completed` - When execution succeeds
- `workflow.failed` - When execution fails

**Status**: ✅ Audit infrastructure exists, just need to add 2 new event types

#### 2. Existing Test Patterns (3 FOUND) ✅

**Pattern 1**: `test/integration/workflowexecution/audit_flow_integration_test.go`
- ✅ Uses real WorkflowExecution controller
- ✅ Creates WFE CRD, waits for controller processing
- ✅ Queries Data Storage via OpenAPI client
- ✅ Validates event metadata and structured event_data
- ✅ Uses `Eventually()` with 60s timeout (CRD controller pattern)
- ✅ No `time.Sleep()`, no `Skip()`

**Pattern 2**: `test/integration/workflowexecution/audit_comprehensive_test.go`
- ✅ Comprehensive lifecycle testing (started, completed, failed)
- ✅ Defense-in-depth strategy documented
- ✅ Real controller + real Data Storage (EnvTest)
- ✅ BR-WE-005 compliance validation

**Pattern 3**: `test/integration/workflowexecution/reconciler_test.go`
- ✅ Integration audit tests with real DataStorage
- ✅ HTTP API query validation
- ✅ Correct field value validation

**Assessment**: Existing patterns are **excellent** and follow all TESTING_GUIDELINES.md standards.

---

## 🎯 **Gap 5-6 Requirements (from SOC2 Test Plan)**

### Expected Audit Events

| Event Type | Count | Trigger | Fields Required |
|-----------|-------|---------|----------------|
| `workflow.selection.completed` | 1 | Workflow selected by AA | `selected_workflow_ref` |
| `execution.workflow.started` | 1 | PipelineRun created | `execution_ref` |

### Event Schema (DD-AUDIT-003 Compliant)

**Event 1: workflow.selection.completed**
```json
{
  "event_type": "workflow.selection.completed",
  "correlation_id": "<wfe-name>",
  "actor_id": "workflowexecution-controller",
  "category": "workflow",
  "outcome": "success",
  "event_data": {
    "selected_workflow_ref": {
      "workflow_id": "k8s/restart-pod-v1",
      "version": "v1.0.0",
      "container_image": "ghcr.io/kubernaut/workflows/restart-pod@sha256:abc123"
    },
    "remediation_request_ref": "default/remediation-request-123",
    "target_resource": "default/deployment/api-server"
  }
}
```

**Event 2: execution.workflow.started**
```json
{
  "event_type": "execution.workflow.started",
  "correlation_id": "<wfe-name>",
  "actor_id": "workflowexecution-controller",
  "category": "execution",
  "outcome": "success",
  "event_data": {
    "execution_ref": {
      "api_version": "tekton.dev/v1",
      "kind": "PipelineRun",
      "name": "wfe-restart-pod-abc123",
      "namespace": "kubernaut-workflows"
    },
    "workflow_ref": {
      "workflow_id": "k8s/restart-pod-v1",
      "version": "v1.0.0"
    },
    "parameters": {
      "pod_name": "api-server-xyz",
      "namespace": "default"
    }
  }
}
```

---

## 🧪 **Integration Points**

### Services Involved
1. **Workflow Execution Controller** (primary) - Emits events
2. **Data Storage Service** (dependency) - Stores audit events
3. **AI Analysis Controller** (upstream) - Provides selected workflow
4. **Remediation Orchestrator** (downstream) - Consumes execution results

### No Blocking Dependencies ✅
- Gap 5-6 work is **independent** of Day 4+ work
- Can start immediately (Day 2 complete at 100%)
- No cross-service coordination needed for Day 3

---

## 🏗️ **Infrastructure**

### Required Services
- ✅ **Workflow Execution Controller** (CRD controller running in EnvTest)
- ✅ **Data Storage** (HTTP API at localhost:18095)
- ✅ **PostgreSQL** (port 15438)
- ✅ **Redis** (port 16384)

**Status**: All infrastructure already running from Day 1-2 tests

### Test Environment
- **Location**: `test/integration/workflowexecution/`
- **Test File**: `audit_workflow_selection_integration_test.go` (NEW)
- **Pattern**: Follow `audit_flow_integration_test.go` (existing, proven)
- **Duration**: ~2-2.5 hours implementation

---

## 📊 **Complexity Assessment**

### Implementation Complexity: **SIMPLE** ✅

**Rationale**:
1. ✅ Audit infrastructure already exists (`RecordAuditEvent()`)
2. ✅ Test patterns proven and documented
3. ✅ No new infrastructure needed
4. ✅ No cross-service dependencies
5. ✅ Event schema straightforward (DD-AUDIT-003 compliant)

### Test Complexity: **MEDIUM** ⚠️

**Challenges**:
1. ⚠️ CRD controller async behavior (60s timeout needed)
2. ⚠️ Need to validate 2 events (not just 1)
3. ⚠️ Event correlation validation required
4. ⚠️ Structured event_data validation (nested objects)

**Mitigations**:
- Use `Eventually()` with 60s timeout (existing pattern)
- Use `countEventsByType()` helper (existing from Day 1-2)
- Use `validateEventMetadata()` helper (existing from DD-TESTING-001)

---

## ✅ **Recommended Approach**

### Option 1: Enhance Existing Implementation (RECOMMENDED) ✅

**What**: Add 2 new event types to existing `audit.go`

**Pros**:
- ✅ Follows TDD REFACTOR principle (enhance, don't create)
- ✅ Reuses proven audit infrastructure
- ✅ Minimal code changes (~50 lines)
- ✅ Fast implementation (2-2.5h total)

**Cons**:
- ❌ None identified

### Option 2: Create New Workflow Selector Module ❌

**What**: New file `workflow_selector.go` with separate audit logic

**Pros**:
- ✅ Better separation of concerns (theoretically)

**Cons**:
- ❌ Violates TDD REFACTOR principle
- ❌ Duplicates existing audit infrastructure
- ❌ More complex (unnecessary abstraction)
- ❌ Longer implementation time

---

## 🔢 **Success Criteria**

### Measurable Outcomes
1. **2 new event types** emitted by WE controller
2. **3 integration tests** passing at 100%
3. **Event structure** validates against DD-AUDIT-003
4. **No anti-patterns** detected (time.Sleep, Skip, etc.)
5. **Documentation updated** (test plan, DD-AUDIT-003)

### Validation Commands
```bash
# Test execution (must pass at 100%)
go test -v ./test/integration/workflowexecution/... -run "Gap 5-6"

# Anti-pattern detection
grep -r 'time\.Sleep' test/integration/workflowexecution/ | grep -v vendor
grep -r '\.Skip(' test/integration/workflowexecution/ | grep -v vendor

# Event count validation
# Should show exactly 2 new event types
```

---

## 🎯 **Confidence Assessment**

**Overall Confidence**: 90%

**Breakdown**:
- **Technical Feasibility**: 95% (audit infrastructure exists, proven patterns)
- **Test Implementation**: 90% (CRD controller async complexity)
- **Integration**: 95% (no cross-service dependencies)
- **Timeline**: 85% (2-2.5h estimate, may need 3h if debugging needed)

**Risks**:
- ⚠️ **Minor**: CRD controller timing (mitigated by Eventually() with 60s timeout)
- ⚠️ **Minor**: Event structure validation complexity (mitigated by existing helpers)

**Assumptions**:
- ✅ Data Storage service remains stable (Day 1-2 tests passing)
- ✅ No breaking changes to DD-AUDIT-003 schema
- ✅ EnvTest Workflow Execution controller works as expected

---

## 🚨 **Analysis Phase Complete - Ready for Checkpoint**

**Phase Duration**: 15 minutes (as planned)
**Discovery Findings**: Existing patterns excellent, minimal implementation needed
**Blocking Issues**: None
**Ready to Proceed**: YES (pending user approval)

---

**Next Step**: ANALYSIS CHECKPOINT - Human approval required

