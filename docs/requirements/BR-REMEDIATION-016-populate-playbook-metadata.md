# BR-REMEDIATION-016: Populate Playbook Metadata on Audit Creation

**Business Requirement ID**: BR-REMEDIATION-016
**Category**: RemediationExecutor Service
**Priority**: P1
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **playbook-based success tracking** (SECONDARY dimension). The RemediationExecutor Service must populate `playbook_id`, `playbook_version`, `playbook_step_number`, and `playbook_execution_id` fields when creating audit records to enable playbook effectiveness analysis.

**Current Limitations**:
- ❌ RemediationExecutor creates audit records without playbook metadata
- ❌ Data Storage schema has playbook columns but they remain NULL
- ❌ Cannot calculate success rates by playbook (BR-STORAGE-031-02 blocked)
- ❌ Cannot track step-by-step playbook execution
- ❌ No way to compare playbook version effectiveness

**Impact**:
- BR-STORAGE-031-02 (Playbook Success Rate API) cannot function with NULL playbook_id
- Cannot validate playbook improvements across versions
- No data for playbook deprecation decisions
- Missing foundation for continuous playbook improvement

---

## 🎯 **Business Objective**

**Ensure RemediationExecutor populates playbook_id, playbook_version, playbook_step_number, and playbook_execution_id fields in all audit records to enable playbook-based success tracking.**

### **Success Criteria**
1. ✅ RemediationExecutor extracts playbook metadata from workflow/execution context
2. ✅ Populates `playbook_id` field (REQUIRED) in audit records
3. ✅ Populates `playbook_version` field (REQUIRED) in audit records
4. ✅ Populates `playbook_step_number` field (OPTIONAL) for multi-step playbooks
5. ✅ Populates `playbook_execution_id` field (REQUIRED) to group playbook steps
6. ✅ 100% of playbook-based remediations have non-null playbook metadata
7. ✅ Manual remediations use "manual-remediation" as playbook_id with version "v1.0"

---

## 📊 **Use Cases**

### **Use Case 1: Single-Step Playbook Execution**

**Scenario**: RemediationExecutor executes `pod-restart-simple v1.0` (single-step playbook).

**Current Flow** (Without BR-REMEDIATION-016):
```
1. AI selects pod-restart-simple v1.0
2. RemediationExecutor executes: restart_pod action
3. RemediationExecutor creates audit:
   {
     "action_id": "act-123",
     "action_type": "restart_pod",
     "playbook_id": null,  ← ❌ NULL
     "playbook_version": null,  ← ❌ NULL
     "playbook_execution_id": null  ← ❌ NULL
   }
4. ❌ Cannot track playbook effectiveness
```

**Desired Flow with BR-REMEDIATION-016**:
```
1. AI selects pod-restart-simple v1.0
2. RemediationExecutor receives execution context:
   {
     "playbook_id": "pod-restart-simple",
     "playbook_version": "v1.0",
     "execution_id": "exec-abc123"
   }
3. RemediationExecutor executes: restart_pod action
4. RemediationExecutor creates audit:
   {
     "action_id": "act-123",
     "action_type": "restart_pod",
     "playbook_id": "pod-restart-simple",  ← ✅ POPULATED
     "playbook_version": "v1.0",  ← ✅ POPULATED
     "playbook_execution_id": "exec-abc123",  ← ✅ POPULATED
     "playbook_step_number": 1  ← ✅ POPULATED (optional)
   }
5. ✅ Data Storage can aggregate by playbook
6. ✅ Team can compare v1.0 vs future versions
```

---

### **Use Case 2: Multi-Step Playbook Execution**

**Scenario**: RemediationExecutor executes `pod-oom-recovery v1.2` (3-step playbook).

**Current Flow**:
```
1. AI selects pod-oom-recovery v1.2 (3 steps)
2. RemediationExecutor executes steps:
   - Step 1: increase_memory
   - Step 2: restart_pod
   - Step 3: verify_health
3. RemediationExecutor creates 3 audits:
   - Audit 1: action_type=increase_memory, playbook_step_number=null
   - Audit 2: action_type=restart_pod, playbook_step_number=null
   - Audit 3: action_type=verify_health, playbook_step_number=null
4. ❌ Cannot identify which step failed (no step numbers)
5. ❌ Cannot group steps into single playbook execution
```

**Desired Flow with BR-REMEDIATION-016**:
```
1. AI selects pod-oom-recovery v1.2
2. RemediationExecutor generates execution_id: "exec-xyz789"
3. RemediationExecutor executes steps:
   - Step 1: increase_memory (success)
   - Step 2: restart_pod (success)
   - Step 3: verify_health (failure)
4. RemediationExecutor creates 3 audits:
   - Audit 1: playbook_id=pod-oom-recovery, version=v1.2, step_number=1, execution_id=exec-xyz789, status=completed
   - Audit 2: playbook_id=pod-oom-recovery, version=v1.2, step_number=2, execution_id=exec-xyz789, status=completed
   - Audit 3: playbook_id=pod-oom-recovery, version=v1.2, step_number=3, execution_id=exec-xyz789, status=failed
5. ✅ Data Storage groups 3 audits by execution_id
6. ✅ Team identifies: Step 3 (verify_health) causes failures
7. ✅ Target improvement: Fix verify_health logic in v1.3
```

---

### **Use Case 3: Manual Remediation (No Playbook)**

**Scenario**: Operator manually executes remediation without selecting a playbook.

**Current Flow**:
```
1. Operator manually executes action (no playbook selected)
2. RemediationExecutor creates audit with NULL playbook metadata
3. ❌ Audit excluded from playbook aggregations
4. ❌ Manual remediations not tracked separately
```

**Desired Flow with BR-REMEDIATION-016**:
```
1. Operator manually executes action
2. RemediationExecutor detects: No playbook context
3. RemediationExecutor applies default values:
   - playbook_id = "manual-remediation"
   - playbook_version = "v1.0"
   - playbook_execution_id = "manual-{timestamp}"
   - playbook_step_number = 1
4. RemediationExecutor creates audit with default metadata
5. ✅ Manual remediations tracked separately
6. ✅ Can compare manual vs playbook-based effectiveness
7. ✅ No NULL playbook_id records
```

---

## 🔧 **Functional Requirements**

### **FR-REMEDIATION-016-01: Playbook Metadata Extraction**

**Requirement**: RemediationExecutor SHALL extract playbook metadata from execution context.

**Implementation Example**:
```go
package remediationexecutor

// ExecutionContext contains playbook metadata for audit population
type ExecutionContext struct {
    PlaybookID        string  // e.g., "pod-oom-recovery"
    PlaybookVersion   string  // e.g., "v1.2"
    ExecutionID       string  // e.g., "exec-abc123" (groups multi-step execution)
    CurrentStepNumber int     // e.g., 1, 2, 3 (for multi-step playbooks)
}

// ExtractPlaybookMetadata extracts metadata from workflow/execution context
func ExtractPlaybookMetadata(workflow *RemediationWorkflow) *ExecutionContext {
    // Case 1: Playbook-based remediation
    if workflow.PlaybookRef != nil {
        return &ExecutionContext{
            PlaybookID:        workflow.PlaybookRef.PlaybookID,
            PlaybookVersion:   workflow.PlaybookRef.Version,
            ExecutionID:       workflow.ExecutionID,  // Generated by workflow controller
            CurrentStepNumber: workflow.CurrentStep,
        }
    }

    // Case 2: Manual remediation (no playbook)
    return &ExecutionContext{
        PlaybookID:        "manual-remediation",
        PlaybookVersion:   "v1.0",
        ExecutionID:       fmt.Sprintf("manual-%d", time.Now().Unix()),
        CurrentStepNumber: 1,
    }
}
```

**Acceptance Criteria**:
- ✅ Extracts playbook_id from workflow.PlaybookRef.PlaybookID
- ✅ Extracts playbook_version from workflow.PlaybookRef.Version
- ✅ Extracts execution_id from workflow.ExecutionID
- ✅ Extracts current_step_number from workflow.CurrentStep
- ✅ Returns default values for manual remediations

---

### **FR-REMEDIATION-016-02: Playbook Execution ID Generation**

**Requirement**: RemediationExecutor SHALL generate unique execution IDs to group multi-step playbook executions.

**Execution ID Format**: `exec-{uuid}` or `manual-{timestamp}`

**Implementation**:
```go
// GenerateExecutionID creates unique ID for playbook execution
func GenerateExecutionID(isManual bool) string {
    if isManual {
        return fmt.Sprintf("manual-%d", time.Now().Unix())
    }
    return fmt.Sprintf("exec-%s", uuid.New().String()[:8])
}
```

**Acceptance Criteria**:
- ✅ Execution ID is unique across all executions
- ✅ Execution ID remains constant for all steps in multi-step playbook
- ✅ Manual executions use "manual-{timestamp}" format
- ✅ Playbook executions use "exec-{uuid}" format

---

### **FR-REMEDIATION-016-03: Audit Record Population**

**Requirement**: RemediationExecutor SHALL populate playbook metadata when creating audit records.

**Implementation Example**:
```go
// CreateNotificationAudit populates playbook metadata
func (r *RemediationExecutor) CreateNotificationAudit(ctx context.Context, action *Action, executionCtx *ExecutionContext) error {
    audit := &datastorage.NotificationAudit{
        ActionID:        action.ID,
        ActionType:      action.Type,
        ActionTimestamp: time.Now(),
        Status:          action.Status,

        // ADR-033: DIMENSION 1 (INCIDENT TYPE) - BR-REMEDIATION-015
        IncidentType:     ExtractIncidentType(action.Signal),
        IncidentSeverity: ExtractSeverity(action.Signal),

        // ADR-033: DIMENSION 2 (PLAYBOOK) - BR-REMEDIATION-016
        PlaybookID:          executionCtx.PlaybookID,        // REQUIRED
        PlaybookVersion:     executionCtx.PlaybookVersion,   // REQUIRED
        PlaybookStepNumber:  executionCtx.CurrentStepNumber, // OPTIONAL (0 if not multi-step)
        PlaybookExecutionID: executionCtx.ExecutionID,       // REQUIRED

        // ... other fields ...
    }

    // Validate playbook metadata is non-empty
    if audit.PlaybookID == "" || audit.PlaybookVersion == "" || audit.PlaybookExecutionID == "" {
        return fmt.Errorf("playbook metadata validation failed: playbook_id, playbook_version, and playbook_execution_id are required")
    }

    // Send to Data Storage Service
    return r.dataStorageClient.CreateNotificationAudit(ctx, audit)
}
```

**Acceptance Criteria**:
- ✅ `playbook_id` field is always non-empty (REQUIRED)
- ✅ `playbook_version` field is always non-empty (REQUIRED)
- ✅ `playbook_execution_id` field is always non-empty (REQUIRED)
- ✅ `playbook_step_number` field may be 0 for single-step playbooks (OPTIONAL)
- ✅ Validation error if any required field is empty

---

## 📈 **Non-Functional Requirements**

### **NFR-REMEDIATION-016-01: Performance**

- ✅ Metadata extraction adds <5ms latency to audit creation
- ✅ No additional network calls (metadata available in workflow context)
- ✅ Execution ID generation is stateless and thread-safe

### **NFR-REMEDIATION-016-02: Reliability**

- ✅ Extraction logic never causes audit creation to fail
- ✅ Graceful degradation: uses "manual-remediation" fallback if context unavailable
- ✅ Logging for unexpected workflow formats (but still creates audit)

### **NFR-REMEDIATION-016-03: Data Integrity**

- ✅ Execution ID is immutable (same ID for all steps in playbook)
- ✅ Step numbers are sequential (1, 2, 3, ...) for multi-step playbooks
- ✅ Playbook version matches Playbook Catalog registry

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (defines playbook as secondary dimension)
- ✅ BR-STORAGE-031-03: Schema migration (playbook_id, playbook_version, etc. columns exist)
- ✅ BR-PLAYBOOK-001: Playbook Catalog provides playbook_id and version values
- ✅ Workflow controller: Provides execution context with playbook metadata

### **Downstream Impacts**
- ✅ BR-STORAGE-031-02: Playbook Success Rate API can now aggregate by playbook
- ✅ BR-PLAYBOOK-002: Playbook versioning uses historical effectiveness data
- ✅ BR-EFFECTIVENESS-002: Effectiveness Monitor tracks playbook trends

---

## 🚀 **Implementation Phases**

### **Phase 1: Execution Context Structure** (Day 9 - 2 hours)
- Define `ExecutionContext` struct
- Implement `GenerateExecutionID()` function
- Unit tests for execution ID generation

### **Phase 2: Metadata Extraction** (Day 9 - 3 hours)
- Implement `ExtractPlaybookMetadata()` function
- Add fallback logic for manual remediations
- Unit tests for extraction logic (10+ test cases)

### **Phase 3: Audit Creation Integration** (Day 10 - 3 hours)
- Update `CreateNotificationAudit()` to populate playbook fields
- Add validation for non-empty required fields
- Add logging for playbook metadata population

### **Phase 4: Testing** (Day 10 - 3 hours)
- Unit tests: Single-step playbooks, multi-step playbooks, manual remediations
- Integration tests: Full audit creation with real Data Storage Service
- Test edge cases: Missing workflow context, invalid playbook references

**Total Estimated Effort**: 11 hours (1.5 days)

---

## 📊 **Success Metrics**

### **Population Rate**
- **Target**: 100% of audit records have non-null `playbook_id` and `playbook_version`
- **Measure**: Query Data Storage for NULL playbook_id records

### **Manual Remediation Tracking**
- **Target**: 5-10% of audits use "manual-remediation" playbook_id
- **Measure**: Count audits with playbook_id="manual-remediation"

### **Multi-Step Playbook Tracking**
- **Target**: 30%+ of playbook executions have 2+ steps
- **Measure**: Count distinct playbook_execution_id with step_number > 1

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Data Storage Extracts Playbook Metadata**

**Approach**: RemediationExecutor sends raw workflow context, Data Storage extracts metadata

**Rejected Because**:
- ❌ Violates separation of concerns (Data Storage is persistence, not business logic)
- ❌ Requires Data Storage to understand workflow formats (tight coupling)
- ❌ Harder to test extraction logic in Data Storage

---

### **Alternative 2: No Execution ID (Use Action ID)**

**Approach**: Use action_id to group multi-step playbook executions

**Rejected Because**:
- ❌ Action IDs are unique per action (cannot group steps)
- ❌ Requires complex JOIN queries to reconstruct playbook execution
- ❌ Loss of explicit grouping semantics

---

### **Alternative 3: Allow NULL Playbook Metadata**

**Approach**: Make playbook_id optional, handle NULLs in aggregation queries

**Rejected Because**:
- ❌ Breaks ADR-033 secondary dimension requirement
- ❌ Complicates aggregation queries (must handle NULL case)
- ❌ Reduces data quality and playbook improvement insights

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority (enables playbook-based success tracking)
**Rationale**: Required for playbook version comparison and continuous improvement
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-02: Playbook Success Rate API (depends on this BR)
- BR-STORAGE-031-03: Schema Migration (provides playbook columns)
- BR-REMEDIATION-015: Populate incident type
- BR-PLAYBOOK-001: Playbook Catalog provides playbook_id/version values

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

