# BR-WE-001: Defense-in-Depth Parameter Validation

**Business Requirement ID**: BR-WE-001
**Category**: Workflow Engine Service
**Priority**: P2
**Target Version**: V1
**Status**: ❌ **CANCELLED** (December 1, 2025)
**Date**: December 1, 2025

---

## ❌ CANCELLATION NOTICE

**Cancelled**: December 1, 2025
**Reason**: Validation responsibility consolidated to HolmesGPT-API only.

**Rationale**:
1. If validation fails at WE, the entire RCA flow must restart (expensive)
2. If validation fails at HAPI, LLM can self-correct in the same session (cheap)
3. Edge cases (HAPI bugs, API bypass) should be addressed at source, not duplicated
4. Simplifies architecture - WE doesn't need Data Storage access for schema

**Replacement**: BR-HAPI-191 is now the **sole** parameter validation requirement.

**Updated Documents**:
- DD-HAPI-002 v1.1: Removed WE validation layer
- WE CRD Schema: No `ValidateParameters` function needed

---

## Original Requirement (ARCHIVED)

---

## 📋 **Business Need**

### **Problem Statement**

While BR-HAPI-191 implements primary parameter validation in HolmesGPT-API's chat session, edge cases may still occur where invalid parameters reach the Workflow Engine:

1. LLM fails to self-correct after 3 attempts
2. Bug in HolmesGPT-API validation logic
3. Direct API calls bypassing HolmesGPT-API validation
4. Schema mismatch between validation and execution

**Defense-in-Depth Principle**: Multiple validation layers ensure invalid parameters never reach Tekton execution.

**Impact Without This BR**:
- Invalid parameters could cause Tekton Pipeline failures
- Difficult to diagnose parameter errors at execution time
- Wasted cluster resources on doomed executions
- Poor error messages to users

---

## 🎯 **Business Objective**

**Workflow Engine SHALL validate workflow parameters before creating Tekton resources, providing a safety net for edge cases not caught by HolmesGPT-API.**

### **Success Criteria**

1. ✅ Workflow Engine validates parameters before Tekton PipelineRun creation
2. ✅ Validation checks: required fields, types, enums, ranges
3. ✅ Clear error messages returned to caller on validation failure
4. ✅ Metrics track validation failures at this layer (should be <5%)
5. ✅ No Tekton resources created for invalid parameters

---

## 📊 **Use Cases**

### **Use Case 1: Catch Edge Case from HolmesGPT-API**

**Scenario**: HolmesGPT-API validation has a bug that misses a type error.

```
1. HolmesGPT-API validates parameters (bug: doesn't check numeric types)
2. Parameters include: {"REPLICA_COUNT": "five"}  // Should be integer
3. AIAnalysis creates WorkflowExecution CRD
4. Workflow Engine receives request
5. WE validates: "REPLICA_COUNT must be integer, got string 'five'"
6. ❌ Validation fails → no Tekton resources created
7. ✅ Error returned to AIAnalysis for handling
8. ✅ No wasted Tekton execution
```

### **Use Case 2: Direct API Call Bypass**

**Scenario**: Internal tool creates WorkflowExecution directly (bypassing HolmesGPT-API).

```
1. Admin tool creates WorkflowExecution CRD directly
2. Parameters not validated by HolmesGPT-API (bypassed)
3. Workflow Engine validates parameters
4. If invalid → reject before Tekton creation
5. ✅ Defense-in-depth catches bypass scenario
```

### **Use Case 3: All Validations Pass**

**Scenario**: Normal flow where HolmesGPT-API already validated.

```
1. HolmesGPT-API validates parameters ✅
2. Workflow Engine re-validates parameters ✅ (redundant but safe)
3. Tekton PipelineRun created
4. ✅ Double validation = high confidence
```

---

## 🔧 **Technical Requirements**

### **TR-1: Validation Before Tekton Creation**

```go
func (e *WorkflowEngine) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Fetch WorkflowExecution
    var we v1alpha1.WorkflowExecution
    if err := e.Get(ctx, req.NamespacedName, &we); err != nil {
        return ctrl.Result{}, err
    }

    // Fetch workflow definition
    workflowDef, err := e.fetchWorkflowDefinition(ctx, we.Spec.WorkflowID)
    if err != nil {
        return ctrl.Result{}, err
    }

    // DEFENSE-IN-DEPTH: Validate parameters
    if err := e.ValidateParameters(workflowDef, we.Spec.Parameters); err != nil {
        // Update status with validation error
        we.Status.Phase = v1alpha1.WorkflowExecutionPhaseFailed
        we.Status.Message = fmt.Sprintf("Parameter validation failed: %v", err)
        e.Status().Update(ctx, &we)

        // Emit metric
        parameterValidationFailures.Inc()

        return ctrl.Result{}, nil // Don't requeue - permanent failure
    }

    // Proceed with Tekton PipelineRun creation
    return e.createTektonResources(ctx, &we, workflowDef)
}
```

### **TR-2: Validation Function**

```go
func (e *WorkflowEngine) ValidateParameters(
    def *v1alpha1.WorkflowDefinition,
    params map[string]string,
) error {
    var errs []string

    for _, p := range def.Spec.Parameters {
        value, exists := params[p.Name]

        if p.Required && !exists {
            errs = append(errs, fmt.Sprintf("missing required: %s", p.Name))
            continue
        }

        if !exists {
            continue
        }

        if err := validateType(value, p.Type); err != nil {
            errs = append(errs, fmt.Sprintf("%s: %v", p.Name, err))
        }

        if len(p.Enum) > 0 && !contains(p.Enum, value) {
            errs = append(errs, fmt.Sprintf("%s: must be one of %v", p.Name, p.Enum))
        }
    }

    if len(errs) > 0 {
        return &ValidationError{Errors: errs}
    }
    return nil
}
```

### **TR-3: Metrics**

```go
var parameterValidationFailures = prometheus.NewCounter(prometheus.CounterOpts{
    Name: "workflow_engine_parameter_validation_failures_total",
    Help: "Number of parameter validation failures at Workflow Engine layer",
})
```

**Expected**: <5% of requests should fail here (most caught by HolmesGPT-API).

---

## 📈 **Metrics & KPIs**

| Metric | Target | Rationale |
|--------|--------|-----------|
| WE validation failure rate | <5% | Most errors caught by HolmesGPT-API |
| Tekton failures due to parameters | 0% | WE should catch all before Tekton |
| WE validation latency | <10ms | Should not significantly delay execution |

---

## 🔗 **Dependencies**

| Dependency | Service | Status |
|------------|---------|--------|
| Workflow definition schema | Data Storage | ✅ Exists |
| BR-HAPI-191 (primary validation) | HolmesGPT-API | 🟡 In progress |

---

## 📐 **Design Decision**

**Reference**: [DD-HAPI-002: Workflow Parameter Validation Architecture](../../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)

---

## 🔄 **Related Requirements**

| BR ID | Description | Relationship |
|-------|-------------|--------------|
| BR-HAPI-191 | Primary Parameter Validation | HolmesGPT-API does primary validation |
| BR-WE-002 | Tekton Pipeline Creation | WE creates Tekton after validation |

---

## ✅ **Acceptance Criteria**

```gherkin
Feature: Defense-in-Depth Parameter Validation

  Scenario: Valid parameters proceed to Tekton
    Given WorkflowExecution with valid parameters
    When Workflow Engine reconciles
    Then parameters are validated
    And Tekton PipelineRun is created

  Scenario: Invalid parameters rejected before Tekton
    Given WorkflowExecution with invalid parameters
    When Workflow Engine reconciles
    Then validation fails with clear error message
    And WorkflowExecution status is "Failed"
    And NO Tekton resources are created
    And parameterValidationFailures metric increments

  Scenario: Missing required parameter rejected
    Given workflow requires "NAMESPACE" parameter
    And WorkflowExecution omits "NAMESPACE"
    When Workflow Engine reconciles
    Then validation fails: "missing required: NAMESPACE"

  Scenario: Redundant validation after HolmesGPT-API
    Given HolmesGPT-API already validated parameters
    When Workflow Engine re-validates
    Then validation passes (redundant but safe)
    And execution proceeds normally
```

---

## 📅 **Timeline**

| Phase | Duration | Deliverable |
|-------|----------|-------------|
| Design | Complete | DD-HAPI-002 |
| Validation function | 1 day | ValidateParameters() |
| Integration in reconciler | 0.5 day | Pre-Tekton validation |
| Metrics | 0.5 day | Prometheus counter |
| Testing | 1 day | Unit + integration tests |
| **Total** | **~3 days** | |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial requirement |

