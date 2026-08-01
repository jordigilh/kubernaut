# BR-KA-191: Workflow Parameter Validation with Self-Correction

**Business Requirement ID**: BR-KA-191
**Category**: Kubernaut Agent (KA)
**Priority**: P1
**Target Version**: V1
**Status**: ✅ Approved (implemented, mechanism corrected below)
**Date**: December 1, 2025

---

> **Note (2026-08-01, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: Renamed `BR-HAPI-191` → `BR-KA-191` and corrected the validation mechanism to match the Go implementation. The business need (catch parameter errors before execution, let the LLM self-correct, avoid restarting RCA from scratch) is unchanged and still met. What changed is *how*: this document originally described a `validate_workflow_parameters` **MCP tool** that the LLM would call explicitly, inspect the result, and retry. **KA has no LLM tool-calling/agent framework by design** ([DD-KA-019](../architecture/decisions/DD-KA-019-go-rewrite-design/DD-KA-019-go-rewrite-design.md)) — instead, KA validates the LLM's *returned* workflow selection programmatically in Go (`internal/kubernautagent/parser/validator.go`) and, on failure, re-prompts the LLM with a rendered error message as the next conversation turn (`internal/kubernautagent/prompt/templates/validation_error.tmpl`). This is a validate-then-reprompt loop, not a tool invocation, but achieves the same self-correction outcome. See [DD-KA-001](../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md) for the authoritative architecture.
>
> **Known unrelated dead code found during this review**: `pkg/apifrontend/tools/ka_tools.go` (a different service — API Frontend, KA's external-facing sibling) contains an unwired function also named `ValidateWorkflowParameters` with no tool registration and no production caller (test-only). It implements a *different, stricter* semantic (unknown parameters are a hard error, not stripped) and should not be confused with KA's validator. Tracked as a candidate for cleanup in a future issue; out of scope here.

---

## 📋 **Business Need**

### **Problem Statement**

When Kubernaut Agent (KA) selects a workflow and supplies parameters, those parameters must be validated against the workflow's parameter schema before the recommendation reaches AIAnalysis/execution. Validating only downstream (in the Workflow Engine, after the LLM investigation session has already ended) creates significant problems.

**Problems avoided by in-session validation**:
- ❌ Parameter validation happening only in downstream services, after the LLM session ends
- ❌ If validation fails downstream, the LLM's investigation context is lost
- ❌ Root Cause Analysis would need to restart from scratch
- ❌ LLM non-determinism means a repeated RCA attempt may produce a different (possibly worse) result
- ❌ Wasted compute/tokens on failed remediations

**Impact if not addressed**:
- Users would wait for a full downstream execution attempt to discover parameter errors
- RCA would need to be repeated on validation failures (expensive)
- Lower remediation success rate due to parameter mismatches
- Poor user experience when workflows fail on invalid parameters

---

## 🎯 **Business Objective**

**KA validates workflow parameters as part of its own investigation pipeline — before the result is returned to AIAnalysis — giving the LLM the opportunity to self-correct within the same investigation, not after a downstream failure.**

### **Success Criteria (as implemented)**

1. ✅ KA validates the LLM's `selected_workflow` (ID + parameters) against the workflow's schema immediately after the LLM returns it, as part of Phase 3 (Workflow Selection) — see [DD-KA-001](../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md)
2. ✅ The schema comes from KA's in-process workflow catalog (`internal/kubernautagent/workflowcatalog`), populated per-request from the full DataStorage-backed catalog — not a live per-workflow DS REST call (superseded by [DD-WORKFLOW-019](../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md))
3. ✅ Validates: workflow existence (hallucination detection), required fields, types, enums, numeric min/max, regex patterns, `dependsOn` relationships
4. ✅ On failure, KA renders an actionable error message (`validation_error.tmpl`) and re-prompts the LLM as the next conversation turn, rather than returning an opaque failure
5. ✅ The LLM gets up to 3 total attempts (`maxSelfCorrectionAttempts`, `internal/kubernautagent/investigator/investigator.go`) to produce a valid workflow selection
6. ✅ If the LLM cannot self-correct within 3 attempts, KA sets `needs_human_review: true` and clears the unresolved `workflow_id` (not a silent failure) — see [BR-KA-197](BR-KA-197-needs-human-review-field.md)
7. ✅ Undeclared/hallucinated parameters (e.g., credentials the LLM invented) are silently stripped rather than rejected outright, recorded for LLM feedback (Issue #241) — this is deliberately more lenient than an outright rejection, since it lets a workflow with a mostly-correct parameter set still succeed

---

## 📊 **Use Cases**

### **Use Case 1: LLM Self-Correction on Invalid Parameters**

**Scenario**: LLM selects an OOM-recovery workflow but supplies an invalid `MEMORY_LIMIT` format.

**Flow (as implemented)**:
```
1. LLM performs RCA -> identifies OOMKilled
2. LLM's structured JSON response selects workflow "oom-recovery-v1"
   with selected_workflow.parameters = {"MEMORY_LIMIT": "2 gigabytes"}  // Invalid format
3. KA parses the response and calls validator.Validate() (Phase 3 validation)
4. Validation fails: "MEMORY_LIMIT must match K8s quantity format (e.g., '512Mi', '2Gi')"
5. KA renders validation_error.tmpl and appends it as the next conversation turn
   (attempt 2 of 3), instead of ending the session
6. LLM re-submits: selected_workflow.parameters = {"MEMORY_LIMIT": "2Gi"}
7. validator.Validate() passes
8. KA returns the validated InvestigationResult -- no downstream re-validation
   (DD-KA-001: KA is the sole validation authority)
```

Contrast with the failure mode this requirement prevents: if validation only happened in the Workflow Engine after the chat session ended, step 4's failure would force a full new RCA investigation rather than a same-session re-prompt.

### **Use Case 2: Missing Required Parameter**

```
1. Workflow requires: NAMESPACE (required), TARGET_NAME (required)
2. LLM's selected_workflow.parameters = {"TARGET_NAME": "api-server"}  // Missing NAMESPACE
3. validator.Validate() fails: "missing required parameter: NAMESPACE"
4. KA re-prompts; LLM corrects to {"NAMESPACE": "production", "TARGET_NAME": "api-server"}
5. Validation passes on attempt 2
```

### **Use Case 3: Enum Value Mismatch**

```
1. Workflow requires RESTART_POLICY in {Always, OnFailure, Never}
2. LLM supplies {"RESTART_POLICY": "always"}  // Wrong case
3. validator.Validate() fails: "RESTART_POLICY must be one of [Always, OnFailure, Never]"
4. LLM corrects to {"RESTART_POLICY": "Always"}; validation passes
```

### **Use Case 4: LLM-Hallucinated Credentials Stripped (Issue #241)**

**Scenario**: Workflow schema declares only `TARGET_NAMESPACE` and `TARGET_RESOURCE_NAME`, but the LLM also supplies `GIT_PASSWORD`/`GIT_USERNAME`.

```
1. LLM selects workflow "fix-certificate-gitops-v1" with parameters:
   {"TARGET_NAMESPACE": "demo-cert-gitops", "TARGET_RESOURCE_NAME": "demo-app-cert",
    "GIT_PASSWORD": "kubernaut-token", "GIT_USERNAME": "kubernaut"}  // last two: hallucinated, not in schema
2. KA validates the declared params (TARGET_NAMESPACE, TARGET_RESOURCE_NAME) -- valid
3. KA silently strips the undeclared params (GIT_PASSWORD, GIT_USERNAME) in place
4. The stripped-parameter list is recorded for LLM feedback (not a hard validation failure)
5. Final parameters: {"TARGET_NAMESPACE": "demo-cert-gitops", "TARGET_RESOURCE_NAME": "demo-app-cert"}
6. Credentials for the workflow come from schema-declared dependency mounts (DD-WE-006),
   never from LLM-supplied parameters
```

### **Use Case 5: Workflow Without a Parameter Schema**

```
1. LLM selects a workflow with no `parameters` section in its schema
2. LLM supplies {"SOME_PARAM": "value"}
3. KA finds no declared parameters -- nothing is allowed
4. KA strips ALL parameters (nothing declared = nothing allowed)
5. Validation passes with parameters: {}
```

---

## 🔧 **Technical Requirements (as implemented)**

### **TR-1: Programmatic Validation, Not an LLM Tool**

There is no `validate_workflow_parameters` tool the LLM invokes. Validation is Go orchestration code that runs automatically on every LLM workflow-selection response, in `internal/kubernautagent/parser/validator.go`:

```go
// Validate checks the result against the allowlist, confidence bounds, and
// parameter schema constraints.
func (v *Validator) Validate(result *katypes.InvestigationResult) error

// SelfCorrect drives up to maxAttempts re-prompt cycles, invoking
// correctionFn (which re-queries the LLM) after each validation failure.
func (v *Validator) SelfCorrect(result *katypes.InvestigationResult, maxAttempts int, correctionFn CorrectionFunc) (*katypes.InvestigationResult, error)
```

Orchestrated from `internal/kubernautagent/investigator/investigator_workflow_selection.go`'s `selfCorrectWorkflowSelection`.

### **TR-2: Workflow Schema Source**

KA fetches the *entire* current workflow catalog (not a per-workflow schema endpoint) once per investigation request, from its in-process, cache-backed catalog (`workflowcatalog.LazyCatalog.List`, unfiltered/unbounded) — not a live DataStorage REST call per [DD-WORKFLOW-019](../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md), which moved workflow-discovery ownership from DataStorage's REST surface into KA. This is a strict improvement over the originally proposed `GET /api/v1/workflows/{workflow_id}/schema` endpoint design: zero network hop, zero dependency on a paginated REST surface, no artificial page-size cap.

### **TR-3: LLM Prompt / Re-Prompt**

There is no separate "you MUST call `validate_workflow_parameters`" system-prompt instruction. Instead, validation failures are surfaced as a rendered conversation turn via `internal/kubernautagent/prompt/templates/validation_error.tmpl`, which:
- States the attempt number out of the maximum (`Attempt {{ .AttemptDisplay }}/{{ .MaxAttempts }}`)
- Lists the specific validation errors
- Includes the expected parameter schema as a hint when available
- Instructs the LLM to re-check the workflow ID against the catalog and re-submit corrected JSON

### **TR-4: Retry Limit**

- Maximum 3 total self-correction attempts (`maxSelfCorrectionAttempts`, `internal/kubernautagent/investigator/investigator.go`) — matches the original design intent
- After exhaustion, `HumanReviewNeeded` is set to `true` and any unresolved `workflow_id` is cleared (not left dangling) — see [BR-KA-197](BR-KA-197-needs-human-review-field.md) and [DD-KA-001](../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md)'s Issue #1711 clarification

---

## 📈 **Metrics & KPIs**

| Metric | Target |
|--------|--------|
| Parameter validation errors caught before execution | >95% |
| RCA restarts due to parameter errors | <5% |
| Average validation retries per request | <1.5 |
| Human review escalations due to validation exhaustion | <2% |

---

## 🔗 **Dependencies**

| Dependency | Owner | Status |
|------------|-------|--------|
| Workflow catalog with parameter schemas | KA (in-process cache, backed by DataStorage) | ✅ Implemented |
| Programmatic validator + self-correction loop | KA (`internal/kubernautagent/parser`, `investigator`) | ✅ Implemented |
| Validation-error re-prompt template | KA (`prompt/templates/validation_error.tmpl`) | ✅ Implemented |

---

## 📐 **Design Decision**

**Reference**: [DD-KA-001: Workflow Response Validation Architecture](../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md) (supersedes the retired `DD-HAPI-002`)

---

## 🔄 **Related Requirements**

| BR ID | Description | Relationship |
|-------|-------------|---------------|
| BR-WE-001 | Defense-in-Depth Parameter Validation | Historical WE-side re-check; retired per DD-KA-001 (KA is now sole validator) |
| BR-KA-197 | Needs-Human-Review Field | Validation exhaustion sets this field |
| BR-WORKFLOW-001 | Workflow Registry Management | Provides workflow definitions/schemas KA validates against |

---

## ✅ **Acceptance Criteria**

```gherkin
Feature: Workflow Parameter Validation with Self-Correction

  Scenario: Valid parameters pass on first attempt
    Given the LLM has selected workflow "oom-recovery-v1"
    And the LLM's parameters are {"MEMORY_LIMIT": "2Gi", "NAMESPACE": "production"}
    When KA validates the selected workflow
    Then validation succeeds
    And no re-prompt is issued

  Scenario: Invalid parameters trigger a re-prompt and self-correction
    Given the LLM has selected workflow "oom-recovery-v1"
    And the LLM's parameters are {"MEMORY_LIMIT": "2 gigabytes"}
    When KA validates the selected workflow
    Then validation fails
    And KA re-prompts the LLM with the rendered validation error
    And the LLM corrects to {"MEMORY_LIMIT": "2Gi"}
    And the second validation attempt succeeds

  Scenario: Missing required parameter detected
    Given the workflow requires parameter "NAMESPACE"
    And the LLM omits "NAMESPACE" from its parameters
    When KA validates the selected workflow
    Then validation fails with "missing required parameter: NAMESPACE"

  Scenario: Self-correction exhausted escalates to human review
    Given the LLM fails validation on all 3 attempts
    When the third re-prompt also fails validation
    Then HumanReviewNeeded is set to true
    And the unresolved workflow_id is cleared, not left dangling

  Scenario: Undeclared parameters stripped silently (Issue #241)
    Given the workflow schema declares only parameter "TARGET_NAMESPACE"
    And the LLM supplies {"TARGET_NAMESPACE": "prod", "GIT_PASSWORD": "secret"}
    When KA validates the selected workflow
    Then "GIT_PASSWORD" is removed from the parameters
    And the parameters contain only {"TARGET_NAMESPACE": "prod"}
    And validation succeeds

  Scenario: No-schema workflow has all parameters stripped
    Given the workflow has no declared parameters
    And the LLM supplies {"ANY_PARAM": "value"}
    When KA validates the selected workflow
    Then the parameters are empty
    And validation succeeds
```

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-08-01 | Renamed `BR-HAPI-191` → `BR-KA-191` ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). Corrected the validation mechanism from a proposed LLM-callable `validate_workflow_parameters` tool to KA's actual programmatic validate-then-reprompt loop; corrected the schema source from a proposed per-workflow REST endpoint to KA's in-process catalog cache; corrected all requirement/scenario prose to reference KA instead of HolmesGPT-API/HAPI. Flagged an unrelated, unwired same-named function in `pkg/apifrontend` discovered during this review. |
| 1.1 | 2026-03-02 | Added Use Cases 4-5 and acceptance criteria for undeclared parameter stripping (Issue #241) |
| 1.0 | 2025-12-01 | Initial requirement |
