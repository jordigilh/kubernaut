# BR-WORKFLOW-005: Float Parameter Type

**Business Requirement ID**: BR-WORKFLOW-005
**Category**: Workflow Catalog Service
**Priority**: **P2 (MEDIUM)** - AWX Survey Compatibility
**Target Version**: **V1.0**
**Status**: Active
**Date**: March 2, 2026
**Related ADRs**: ADR-043 (Execution Engine Schema)
**Related BRs**: BR-WORKFLOW-004 (Schema Format), BR-WE-015 (Ansible Engine)
**GitHub Issue**: [#45](https://github.com/jordigilh/kubernaut/issues/45)

---

## Business Need

### Problem Statement

The `WorkflowParameter.Type` field in `workflow-schema.yaml` currently supports `string`, `integer`, `boolean`, and `array`. AWX/AAP surveys support a `float` type for decimal parameters (thresholds, percentages, ratios). Without `float` support, workflow authors must use `string` for decimal values and rely on Ansible's Jinja2 type coercion, which is error-prone and loses schema-level validation.

### Impact Without This BR

- No schema-level validation for decimal parameters (thresholds, percentages)
- Workflow authors must encode floats as strings with manual documentation
- AWX survey `float` type cannot be mapped to a Kubernaut parameter type
- LLM parameter population lacks type guidance for decimal values

---

## Business Objective

**WorkflowParameter SHALL support `float` as a parameter type, with `minimum`/`maximum` validation supporting decimal bounds.**

### Success Criteria

1. `WorkflowParameter.Type` accepts `"float"` in addition to existing types
2. `Minimum` and `Maximum` fields support decimal values (type change from `*int` to `*float64`)
3. Existing workflows with integer `minimum`/`maximum` remain valid (backward compatible — JSON/YAML int-to-float coercion is automatic)
4. DS validates float parameters at registration time
5. Float parameters are passed through the pipeline as strings in `map[string]string` (consistent with existing behavior)
6. The AnsibleExecutor's `buildExtraVars()` correctly coerces string representations of floats to JSON number types

---

## Technical Requirements

### TR-1: Type Validator Update

```go
// Before
Type string `yaml:"type" json:"type" validate:"required,oneof=string integer boolean array"`

// After
Type string `yaml:"type" json:"type" validate:"required,oneof=string integer boolean array float"`
```

### TR-2: Min/Max Type Change

```go
// Before
Minimum *int `yaml:"minimum,omitempty" json:"minimum,omitempty"`
Maximum *int `yaml:"maximum,omitempty" json:"maximum,omitempty"`

// After
Minimum *float64 `yaml:"minimum,omitempty" json:"minimum,omitempty"`
Maximum *float64 `yaml:"maximum,omitempty" json:"maximum,omitempty"`
```

### TR-3: Example Usage

```yaml
parameters:
  - name: MEMORY_THRESHOLD_PERCENT
    type: float
    required: true
    description: "Memory usage percentage threshold for triggering remediation"
    minimum: 0.0
    maximum: 100.0
    default: 85.0
```

---

## Acceptance Criteria

```gherkin
Given a workflow-schema.yaml with a parameter of type "float"
When the workflow is registered in DS
Then the parameter is accepted and stored correctly

Given a float parameter with minimum 0.0 and maximum 100.0
When a value of 85.5 is provided
Then validation passes

Given an existing workflow with integer minimum/maximum values
When the workflow is registered after the type change
Then registration succeeds (backward compatible)
```

---

## Dependencies

- **BR-WORKFLOW-004**: Workflow schema format (parameter type field)
- **BR-WE-015**: Ansible engine (primary consumer — AWX survey `float` compatibility)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-02 | Initial BR |
