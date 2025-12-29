# Questions from HolmesGPT-API Team

**From**: HolmesGPT-API Team
**To**: Workflow Engine Team
**Date**: December 1, 2025
**Re**: Workflow Parameter Validation Architecture

---

## Context

The HolmesGPT-API team has implemented workflow selection and parameter generation as part of the v3.2 release. We've designed a validation architecture to ensure workflow parameters are correct before execution.

---

## Questions

### ~~Q1: Workflow Parameter Validation Architecture (DD-HAPI-002)~~ ✅ RESOLVED

> ✅ **TRIAGED**: December 2, 2025 - This question is **RESOLVED** per DD-HAPI-002 v1.1

**Original Question**: Is the 3-layer validation architecture approved?

**Answer**: ✅ **APPROVED** → then **SIMPLIFIED** to 2-layer architecture.

**Final Architecture** (DD-HAPI-002 v1.1):

| Layer | Responsibility | Status |
|-------|---------------|--------|
| **HolmesGPT-API** | Sole validator with LLM self-correction | ✅ Active |
| ~~Workflow Engine~~ | ~~Defense-in-depth~~ | ❌ **REMOVED** |
| **Tekton Tasks** | Runtime K8s state validation | ✅ Active |

**Authoritative Reference**: DD-HAPI-002 v1.1, BR-WE-001 (CANCELLED)

---

### ~~Q2: ValidateParameters Implementation~~ ❌ OBSOLETE

> ⚠️ **TRIAGED**: December 2, 2025 - This question is **OBSOLETE** per DD-HAPI-002 v1.1

~~BR-WE-001 specifies that Workflow Engine should implement:~~

```go
// ❌ NO LONGER NEEDED - WE does not validate parameters
// func (e *WorkflowEngine) ValidateParameters(...)
```

**Reason**: BR-WE-001 is **CANCELLED**. DD-HAPI-002 v1.1 establishes:
- HolmesGPT-API is the **sole** parameter validator
- WE trusts HAPI validation and passes parameters to Tekton as-is
- No Data Storage dependency needed at WE

**Authoritative Reference**: DD-HAPI-002 v1.1 (December 1, 2025)

---

### ~~Q3: Metrics for Validation Failures~~ ❌ OBSOLETE

> ⚠️ **TRIAGED**: December 2, 2025 - This question is **OBSOLETE** per DD-HAPI-002 v1.1

~~BR-WE-001 suggests a Prometheus metric for WE validation failures.~~

**Reason**: Since WE no longer validates parameters, no validation failure metrics are needed at WE layer. Validation metrics belong to HolmesGPT-API.

**Authoritative Reference**: DD-HAPI-002 v1.1, BR-WE-001 (CANCELLED)

---

## Action Items

| Item | Owner | Status |
|------|-------|--------|
| ~~Review DD-HAPI-002~~ | ~~WE Team~~ | ✅ **RESOLVED** - DD-HAPI-002 v1.1 approved |
| ~~Confirm ValidateParameters approach~~ | ~~WE Team~~ | ❌ **OBSOLETE** - BR-WE-001 cancelled |
| ~~Confirm metrics pattern~~ | ~~WE Team~~ | ❌ **OBSOLETE** - No validation at WE |

---

## Response

**Date**: December 1, 2025
**Responder**: WorkflowExecution Team

---

### Q1 Response: Validation Architecture (DD-HAPI-002) ✅ APPROVED → ⚠️ UPDATED

**Status**: ✅ **APPROVED** (Updated December 1, 2025)

> ⚠️ **UPDATE**: Architecture simplified. WE validation layer **REMOVED**.

**Original response approved 3-layer architecture. After discussion, simplified to 2-layer:**

| Layer | Responsibility | Status |
|-------|---------------|--------|
| **HolmesGPT-API** | In-session validation with LLM self-correction | ✅ **SOLE VALIDATOR** |
| ~~WorkflowEngine~~ | ~~Pre-execution validation~~ | ❌ **REMOVED** |
| **Tekton Tasks (Runtime)** | K8s state validation at execution | ✅ Unchanged |

**Rationale for removing WE validation**:
- If validation fails at WE → must restart entire RCA flow (expensive, poor UX)
- If validation fails at HAPI → LLM can self-correct in same session (cheap, good UX)
- Edge cases (HAPI bugs) should be fixed at source, not duplicated

**Updated Documents**:
- DD-HAPI-002 v1.1: WE validation layer removed
- BR-WE-001: **CANCELLED**

---

### Q2 Response: ValidateParameters Implementation ❌ CANCELLED

**Status**: ❌ **CANCELLED** (December 1, 2025)

> WE will **NOT** implement `ValidateParameters`.
> HolmesGPT-API (BR-HAPI-191) is the **sole** parameter validator.

**WE behavior**:
- ✅ Trusts HAPI validation
- ✅ Passes parameters to Tekton as-is
- ✅ No Data Storage dependency for schema access
- ✅ Simpler architecture

**If invalid parameters reach WE** (HAPI bug):
- Tekton task will fail with runtime error
- WE reports failure with `reason: TaskFailed`
- RO/HAPI team should fix the HAPI validation bug

---

### Q3 Response: Metrics ❌ CANCELLED

**Status**: ❌ **CANCELLED**

> Since WE no longer validates parameters, the `parameter_validation_failures` metric is not needed.

**WE v3.1 metrics** (resource locking, not validation):
```go
// Resource locking metrics (still applicable)
var workflowsSkipped = prometheus.NewCounterVec(...)  // By skip_reason
var activeResourceLocks = prometheus.NewGauge(...)     // Current lock count
var manualReviewRequired = prometheus.NewCounter(...)  // Execution failures requiring review
```

---

### Additional Information: WE v3.1 Schema Updates

For your awareness, WE has updated to v3.1 with new features that may affect HolmesGPT-API integration:

**New failure detail fields** (useful for recovery prompts):
```yaml
status:
  failureDetails:
    reason: "TaskFailed"                # K8s-style enum (no ValidationFailed from WE)
    message: "..."                      # Human readable
    wasExecutionFailure: true           # true = NOT safe to retry
    requiresManualReview: true          # true = manual review required
    naturalLanguageSummary: "..."       # LLM-friendly summary for recovery
```

**Key distinction for HolmesGPT-API** (updated - no ValidationFailed from WE):
| Failure Type | `wasExecutionFailure` | Safe to Retry? | Source |
|--------------|----------------------|----------------|--------|
| ImagePullBackOff | `false` | ✅ Yes | WE (pre-execution) |
| TaskFailed | `true` | ❌ No | WE (during execution) |
| Timeout (during exec) | `true` | ❌ No | WE (during execution) |

> **Note**: `ValidationFailed` is no longer a WE failure reason. Parameter validation errors will surface as Tekton `TaskFailed` if HAPI validation has a bug.

**Reference**: See DD-WE-001 and WE CRD Schema v3.1 for full details.

---

### Action Items Updated

| Item | Owner | Status |
|------|-------|--------|
| Review DD-HAPI-002 | WE Team | ✅ **APPROVED** → **SIMPLIFIED (v1.1)** |
| ~~Confirm ValidateParameters approach~~ | ~~WE Team~~ | ❌ **CANCELLED** - WE doesn't validate |
| ~~Confirm metrics pattern~~ | ~~WE Team~~ | ❌ **CANCELLED** - No validation metrics |
| Share WE v3.1 schema updates | WE Team | ✅ **DONE** (see above) |

---

**Architectural Decision**:

> **HAPI is the SOLE parameter validator** (BR-HAPI-191).
>
> WE trusts HAPI validation and passes parameters to Tekton as-is.
> This simplifies WE architecture and eliminates Data Storage dependency.
>
> See: DD-HAPI-002 v1.1, BR-WE-001 (CANCELLED)

---

---

# Questions from Gateway Team

**From**: Gateway Team
**To**: Workflow Engine Team
**Date**: December 2, 2025
**Re**: TargetResource Format Validation

---

## Q-GW-01: TargetResource String Format Validation

**Context**: RO builds `targetResource` string from `RemediationRequest.Spec.TargetResource`:

```go
// Namespaced: "payment/Deployment/payment-api"
// Cluster-scoped: "Node/worker-node-1"

func buildTargetResource(rr *RemediationRequest) string {
    tr := rr.Spec.TargetResource
    if tr.Namespace != "" {
        return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
    }
    return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}
```

**Question**: Does WE validate this format, or trust it implicitly?

**Concern**: Malformed strings (missing slashes, empty components) could cause issues in resource locking.

**Options**:
- [ ] A) WE trusts RO implicitly - no validation needed
- [ ] B) WE validates format - add validation logic
- [ ] C) WE validates and rejects malformed strings with `Failed` status

**Suggestion**: Consider adding format validation:
```go
func validateTargetResource(tr string) error {
    parts := strings.Split(tr, "/")
    if len(parts) < 2 || len(parts) > 3 {
        return fmt.Errorf("invalid targetResource format: %s", tr)
    }
    for _, part := range parts {
        if part == "" {
            return fmt.Errorf("empty component in targetResource: %s", tr)
        }
    }
    return nil
}
```

---

### WE Team Response ✅ ANSWERED

**Date**: December 2, 2025
**Respondent**: WorkflowExecution Team

| Question | Response | Notes |
|----------|----------|-------|
| Q-GW-01 (Format Validation) | **C** ✅ | WE validates and rejects malformed strings with `Failed` status |

---

#### Q-GW-01 Detailed Response

**Answer**: **Option C - WE validates and rejects malformed strings with `Failed` status**

**Rationale**:
1. **Defense-in-depth**: While RO should always populate correctly, WE validates as a safety net
2. **Fail fast**: Invalid format causes immediate `Failed` status with clear error message
3. **Deterministic behavior**: Malformed strings could cause unpredictable resource locking behavior

**Implementation** (in WE controller):

```go
// pkg/workflowexecution/validation.go

// ValidateTargetResource validates the targetResource string format
// Called before resource lock check in handlePending()
func validateTargetResource(tr string) error {
    if tr == "" {
        return fmt.Errorf("targetResource is required")
    }

    parts := strings.Split(tr, "/")

    // Valid formats:
    // - "namespace/kind/name" (namespaced resources)
    // - "kind/name" (cluster-scoped resources)
    if len(parts) < 2 || len(parts) > 3 {
        return fmt.Errorf(
            "invalid targetResource format '%s': expected 'kind/name' or 'namespace/kind/name'",
            tr,
        )
    }

    for i, part := range parts {
        if part == "" {
            return fmt.Errorf(
                "invalid targetResource format '%s': empty component at position %d",
                tr, i,
            )
        }
    }

    return nil
}
```

**Failure Handling**:

```yaml
# WorkflowExecution.Status when targetResource is invalid
status:
  phase: Failed
  failureDetails:
    reason: "ConfigurationError"
    message: "Invalid targetResource format 'payment//payment-api': empty component at position 1"
    wasExecutionFailure: false  # Pre-execution failure - safe to retry after fix
    requiresManualReview: false
    naturalLanguageSummary: |
      Workflow execution failed before starting due to invalid targetResource format.
      The targetResource field must be 'namespace/kind/name' for namespaced resources
      or 'kind/name' for cluster-scoped resources. Empty components are not allowed.
```

**Additional Notes**:
- This is a **pre-execution** failure (`wasExecutionFailure: false`)
- RO can retry after fixing the `targetResource` value
- Metric: Counted under `workflow_execution_failures_total{reason="ConfigurationError"}`

**Authoritative Reference**: DD-WE-001 v1.0, CRD Schema v3.1

---

**Document Version**: 1.6
**Last Updated**: December 2, 2025
**Changelog**:
- v1.6: Added WE response to Q-GW-01 (Option C - validate and reject)
- v1.5: Triaged Q1 as RESOLVED per DD-HAPI-002 v1.1 (architecture approved/simplified)
- v1.4: Triaged Q2, Q3 as OBSOLETE per DD-HAPI-002 v1.1 (WE doesn't validate parameters)
- v1.3: Added Gateway team question Q-GW-01 (targetResource format validation)
- v1.2: Updated to reflect BR-WE-001 cancellation and DD-HAPI-002 v1.1 simplification
- v1.1: Initial WE response

