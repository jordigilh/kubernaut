# BR-HAPI-191: Workflow Parameter Validation in Chat Session

**Business Requirement ID**: BR-HAPI-191
**Category**: HolmesGPT API Service
**Priority**: P1
**Target Version**: V1
**Status**: ✅ Approved
**Date**: December 1, 2025

---

## 📋 **Business Need**

### **Problem Statement**

When HolmesGPT-API selects a workflow and suggests parameters, those parameters must be validated against the workflow's parameter schema before execution. Currently, validation happens **after** the LLM chat session ends, which creates significant problems.

**Current Limitations**:
- ❌ Parameter validation happens in downstream services (after LLM session)
- ❌ If validation fails, the LLM context is lost
- ❌ Must restart Root Cause Analysis from scratch
- ❌ LLM non-determinism means repeated attempts may produce different results
- ❌ Wasted compute/tokens on failed remediations

**Impact**:
- Users wait for entire execution to discover parameter errors
- RCA must be repeated on validation failures (expensive)
- Lower remediation success rate due to parameter mismatches
- Poor user experience when workflows fail on invalid parameters

---

## 🎯 **Business Objective**

**Enable HolmesGPT-API to validate workflow parameters INSIDE the LLM chat session, allowing the LLM to self-correct before the session ends.**

### **Success Criteria**

1. ✅ New tool `validate_workflow_parameters` available to LLM during investigation
2. ✅ Tool fetches workflow parameter schema from Data Storage
3. ✅ Validates: required fields, types, enums, numeric ranges
4. ✅ Returns actionable error messages for LLM self-correction
5. ✅ LLM can retry validation up to 3 times with corrected parameters
6. ✅ If LLM cannot self-correct, flags for human review (not silent failure)
7. ✅ >95% of parameter validation errors caught before execution

---

## 📊 **Use Cases**

### **Use Case 1: LLM Self-Correction on Invalid Parameters**

**Scenario**: LLM selects OOMKill recovery workflow but suggests invalid MEMORY_LIMIT format.

**Current Flow** (Without BR-HAPI-191):
```
1. LLM performs RCA → identifies OOMKilled
2. LLM selects workflow: oomkill-increase-memory-v1
3. LLM suggests: {"MEMORY_LIMIT": "2 gigabytes"}  // Invalid format
4. Chat session ends
5. AIAnalysis creates WorkflowExecution CRD
6. Workflow Engine validates parameters
7. ❌ Validation fails: "MEMORY_LIMIT must match K8s format (e.g., '2Gi')"
8. ❌ Entire RCA must be repeated
9. ❌ May get different workflow selection on retry
```

**Desired Flow with BR-HAPI-191**:
```
1. LLM performs RCA → identifies OOMKilled
2. LLM selects workflow: oomkill-increase-memory-v1
3. LLM suggests: {"MEMORY_LIMIT": "2 gigabytes"}
4. LLM calls validate_workflow_parameters(workflow_id, params)
5. Tool returns: {
     "status": "invalid",
     "errors": ["MEMORY_LIMIT must match K8s format (e.g., '512Mi', '2Gi')"]
   }
6. LLM self-corrects: {"MEMORY_LIMIT": "2Gi"}
7. LLM calls validate_workflow_parameters again
8. Tool returns: {"status": "valid"}
9. ✅ Chat session returns validated parameters
10. ✅ Workflow executes successfully
```

### **Use Case 2: Missing Required Parameter**

**Scenario**: LLM forgets to include NAMESPACE parameter.

```
1. LLM selects workflow requiring: NAMESPACE (required), TARGET_NAME (required)
2. LLM suggests: {"TARGET_NAME": "api-server"}  // Missing NAMESPACE
3. LLM calls validate_workflow_parameters
4. Tool returns: {"errors": ["Missing required parameter: NAMESPACE"]}
5. LLM corrects: {"NAMESPACE": "production", "TARGET_NAME": "api-server"}
6. Validation passes
7. ✅ Workflow executes with correct parameters
```

### **Use Case 3: Enum Value Mismatch**

**Scenario**: Workflow requires RESTART_POLICY ∈ {Always, OnFailure, Never}.

```
1. LLM suggests: {"RESTART_POLICY": "always"}  // Wrong case
2. Validation fails: "RESTART_POLICY must be one of [Always, OnFailure, Never]"
3. LLM corrects: {"RESTART_POLICY": "Always"}
4. ✅ Validation passes
```

---

## 🔧 **Technical Requirements**

### **TR-1: validate_workflow_parameters Tool**

```python
class ValidateWorkflowParametersTool(Tool):
    name = "validate_workflow_parameters"

    parameters = {
        "workflow_id": ToolParameter(type="string", required=True),
        "parameters": ToolParameter(type="object", required=True)
    }

    def invoke(self, workflow_id: str, parameters: dict) -> ToolResult:
        # Fetch schema from Data Storage
        # Validate parameters
        # Return errors or success
```

### **TR-2: Data Storage Schema Endpoint**

HolmesGPT-API must be able to fetch workflow parameter schema:

```
GET /api/v1/workflows/{workflow_id}/schema
→ Returns parameter definitions (name, type, required, enum, min, max)
```

### **TR-3: LLM Prompt Update**

Add instruction to LLM system prompt:

```
After selecting a workflow, you MUST validate parameters using
validate_workflow_parameters before returning your recommendation.
If validation fails, correct the parameters and retry (max 3 attempts).
```

### **TR-4: Retry Limit**

- Maximum 3 validation attempts
- After 3 failures, return `needs_human_review: true`

---

## 📈 **Metrics & KPIs**

| Metric | Target | Current |
|--------|--------|---------|
| Parameter validation errors caught before execution | >95% | 0% |
| RCA restarts due to parameter errors | <5% | Unknown |
| Average validation retries per request | <1.5 | N/A |
| Human review escalations due to validation | <2% | N/A |

---

## 🔗 **Dependencies**

| Dependency | Service | Status |
|------------|---------|--------|
| Workflow schema endpoint | Data Storage | 🟡 Needs implementation |
| `validate_workflow_parameters` tool | HolmesGPT-API | 🟡 Needs implementation |
| LLM prompt update | HolmesGPT-API | 🟡 Needs implementation |

---

## 📐 **Design Decision**

**Reference**: [DD-HAPI-002: Workflow Parameter Validation Architecture](../../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)

---

## 🔄 **Related Requirements**

| BR ID | Description | Relationship |
|-------|-------------|--------------|
| BR-WE-001 | Defense-in-Depth Parameter Validation | WE validates as safety net |
| BR-HAPI-046-050 | Data Storage Workflow Tool | Prerequisite - selects workflow |
| BR-WORKFLOW-001 | Workflow Registry Management | Provides workflow definitions |

---

## ✅ **Acceptance Criteria**

```gherkin
Feature: Workflow Parameter Validation in Chat Session

  Scenario: Valid parameters pass on first attempt
    Given LLM has selected workflow "oomkill-recovery-v1"
    And LLM provides parameters {"MEMORY_LIMIT": "2Gi", "NAMESPACE": "production"}
    When LLM calls validate_workflow_parameters
    Then validation returns status "valid"
    And no retries are needed

  Scenario: Invalid parameters trigger self-correction
    Given LLM has selected workflow "oomkill-recovery-v1"
    And LLM provides parameters {"MEMORY_LIMIT": "2 gigabytes"}
    When LLM calls validate_workflow_parameters
    Then validation returns status "invalid" with errors
    And LLM corrects parameters to {"MEMORY_LIMIT": "2Gi"}
    And second validation returns status "valid"

  Scenario: Missing required parameter detected
    Given workflow requires parameter "NAMESPACE" as required
    And LLM omits "NAMESPACE" from parameters
    When LLM calls validate_workflow_parameters
    Then validation returns error "Missing required parameter: NAMESPACE"

  Scenario: Max retries exceeded escalates to human
    Given LLM fails validation 3 times
    When LLM attempts fourth validation
    Then response includes "needs_human_review": true
```

---

## 📅 **Timeline**

| Phase | Duration | Deliverable |
|-------|----------|-------------|
| Design | Complete | DD-HAPI-002 |
| Data Storage schema endpoint | 1 day | GET /workflows/{id}/schema |
| HolmesGPT-API tool implementation | 2 days | validate_workflow_parameters |
| LLM prompt update | 0.5 day | System prompt modification |
| Testing | 1 day | Unit + E2E tests |
| **Total** | **~5 days** | |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial requirement |

