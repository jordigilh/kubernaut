# BR-AI-085: Rego Policy Input Schema for Approval Decisions

**Business Requirement ID**: BR-AI-085
**Category**: AIAnalysis Service
**Priority**: P1
**Target Version**: V1.1
**Status**: ✅ Approved
**Date**: 2026-01-20
**Last Updated**: 2026-02-12

**Related Design Decisions**:
- [DD-HAPI-006: Affected Resource in Root Cause Analysis](../architecture/decisions/DD-HAPI-006-affectedResource-in-rca.md)
- [DD-AIANALYSIS-001: Rego Policy Loading Strategy](../architecture/decisions/DD-AIANALYSIS-001-rego-policy-loading-strategy.md)
- [DD-CONTRACT-002: Service Integration Contracts](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [ADR-055: LLM-Driven Context Enrichment](../architecture/decisions/ADR-055-llm-driven-context-enrichment.md)

**Related Business Requirements**:
- BR-AI-012: Approval Policy Evaluation (referenced - not found, needs creation)
- BR-HAPI-212: HAPI RCA Target Resource
- BR-AI-084: AIAnalysis Extract RCA Target
- BR-SCOPE-010: RO Routing Validation

---

## 📋 **Business Need**

### **Problem Statement**

AIAnalysis evaluates Rego policies to determine if remediation workflows require human approval. Currently, Rego policies have access to the **signal source resource** (`target_resource`), but not the **RCA-determined target resource** (`affected_resource`) that will actually be remediated.

**Example Scenario**:
- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled) - namespace: `staging`
- **RCA target**: `Deployment/payment-api` - namespace: `production`
- **Current Rego policy**: Checks `input.target_resource.namespace == "production"` → **FALSE** (checks Pod in staging)
- **Desired Rego policy**: Checks `input.affected_resource.namespace == "production"` → **TRUE** (checks Deployment in production)

**Gap**: Rego policies cannot make approval decisions based on the **actual remediation target**, leading to:
1. ❌ **Incorrect approval decisions**: Workflows targeting production resources may not require approval if signal source is in staging
2. ❌ **Limited policy expressiveness**: Cannot write policies like "require approval if RCA targets critical Deployments"
3. ❌ **Inconsistent approval logic**: Approval based on symptom (signal source) instead of root cause (RCA target)

### **Supersession Note (ADR-055)**

Per [ADR-055: LLM-Driven Context Enrichment](../architecture/decisions/ADR-055-llm-driven-context-enrichment.md), the previous `target_in_owner_chain` boolean field is **superseded** by `affected_resource`. The LLM now identifies the actual remediation target during RCA (Phase 1) and provides it as a structured `affectedResource` object. This replaces the former boolean that only indicated whether the signal source appeared in the owner chain, providing significantly richer context for Rego policy decisions (kind, name, namespace).

---

## 🎯 **Business Objective**

**Enable Rego policies to access the RCA-determined target resource (`affected_resource`) for more accurate and expressive approval decisions.**

**Value Proposition**:
- ✅ **Accurate Approval**: Policies evaluate the resource that will actually be remediated
- ✅ **Policy Expressiveness**: Write policies based on RCA target kind, namespace, and apiVersion
- ✅ **Consistency**: Approval logic aligns with remediation logic (both use RCA target)
- ✅ **Flexibility**: Support complex approval rules (e.g., "require approval if RCA targets production Deployment")

---

## 🔍 **Functional Requirements**

### **FR-AI-085-001: Policy Input Schema**

**Requirement**: AIAnalysis MUST expose `affected_resource` in the Rego policy input schema, alongside the existing `target_resource` (signal source).

**PolicyInput Struct** (`pkg/aianalysis/rego/evaluator.go`):
```go
type PolicyInput struct {
    // Existing fields
    SignalName       string                `json:"signal_name"`
    Severity         string                `json:"severity"`
    SeverityLevel    string                `json:"severity_level"`
    Environment      string                `json:"environment"`
    ConfidenceScore  float64               `json:"confidence_score"`

    // Signal source resource (what triggered the alert)
    TargetResource   TargetResourceInput   `json:"target_resource"`

    // NEW: RCA-determined target resource (what will be remediated)
    // This is the resource identified by HolmesGPT as the root cause.
    // May differ from target_resource (e.g., OOMKilled Pod → Deployment).
    // Use this for approval decisions based on the ACTUAL remediation target.
    // +optional (nil if HAPI didn't determine different target)
    AffectedResource *TargetResourceInput  `json:"affected_resource,omitempty"`

    // #225: Operator-configurable confidence threshold for auto-approval.
    // When set, passed as input.confidence_threshold to the Rego policy,
    // overriding the policy's built-in default (0.8).
    // Configured via rego.confidenceThreshold in the AIAnalysis config YAML.
    // nil means "use the Rego policy's built-in default".
    // +optional
    ConfidenceThreshold *float64           `json:"confidence_threshold,omitempty"`

    // Workflow metadata
    WorkflowID       string                `json:"workflow_id"`
    WorkflowName     string                `json:"workflow_name"`

    // Existing fields...
}

type TargetResourceInput struct {
    Kind       string `json:"kind"`
    APIVersion string `json:"api_version"`  // NEW: snake_case for Rego (optional - empty if not provided)
    Name       string `json:"name"`
    Namespace  string `json:"namespace"`    // Empty for cluster-scoped resources
}
```

**Acceptance Criteria**:
1. ✅ `PolicyInput` struct includes `AffectedResource *TargetResourceInput` field
2. ✅ `TargetResourceInput` includes `api_version` field (snake_case for Rego)
3. ✅ `AffectedResource` is optional (pointer type) - nil if not provided by HAPI
4. ✅ Existing `TargetResource` field remains unchanged (backward compatibility)
5. ✅ `PolicyInput` struct includes `ConfidenceThreshold *float64` field (#225) — optional, nil means Rego default

---

### **FR-AI-085-002: Input Builder Logic**

**Requirement**: AIAnalysis MUST populate `affected_resource` in the policy input from `Status.RootCauseAnalysis.TargetResource` when building input for Rego evaluation.

**Builder Logic** (`pkg/aianalysis/handlers/analyzing.go` - `buildPolicyInput()` function):
```go
func (h *AnalyzingHandler) buildPolicyInput(analysis *aianalysisv1.AIAnalysis) (*rego.PolicyInput, error) {
    input := &rego.PolicyInput{
        SignalName:      analysis.Status.SignalName,
        Severity:        analysis.Status.RootCauseAnalysis.Severity,
        SeverityLevel:   mapSeverityToLevel(analysis.Status.RootCauseAnalysis.Severity),
        Environment:     analysis.Spec.AnalysisRequest.SignalContext.Environment,
        ConfidenceScore: analysis.Status.ConfidenceScore,

        // Signal source (existing)
        TargetResource: &rego.TargetResourceInput{
            Kind:       analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Kind,
            APIVersion: analysis.Spec.AnalysisRequest.SignalContext.TargetResource.APIVersion,
            Name:       analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Name,
            Namespace:  analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Namespace,
        },

        // NEW: RCA target (if different from signal source)
        AffectedResource: buildAffectedResourceInput(analysis.Status.RootCauseAnalysis),

        // Workflow metadata
        WorkflowID:   analysis.Status.SelectedWorkflow.WorkflowID,
        WorkflowName: analysis.Status.SelectedWorkflow.Name,
    }

    return input, nil
}

// NEW: Extract affected resource from RCA status
func buildAffectedResourceInput(rca *aianalysisv1.RootCauseAnalysis) *rego.TargetResourceInput {
    if rca == nil || rca.TargetResource == nil {
        return nil // No RCA target - policies will use target_resource (signal source)
    }

    return &rego.TargetResourceInput{
        Kind:       rca.TargetResource.Kind,
        APIVersion: rca.TargetResource.APIVersion,  // NEW: Optional - empty if not provided
        Name:       rca.TargetResource.Name,
        Namespace:  rca.TargetResource.Namespace,   // Empty for cluster-scoped resources
    }
}
```

**Acceptance Criteria**:
1. ✅ `buildPolicyInput()` extracts `affected_resource` from `Status.RootCauseAnalysis.TargetResource`
2. ✅ `affected_resource` is nil if `TargetResource` is not present (fallback to signal source)
3. ✅ `api_version` is populated from `APIVersion` field (empty string if not provided)
4. ✅ Existing `target_resource` population remains unchanged

---

### **FR-AI-085-003: Policy Examples**

**Requirement**: Documentation MUST provide clear examples of Rego policies using `affected_resource` for approval decisions.

**Example Policies**:

**Example 1: Require approval for production Deployments (based on RCA target)**
```rego
package kubernaut.approval

# Require approval if RCA targets a production Deployment
require_approval if {
    input.affected_resource.kind == "Deployment"
    input.affected_resource.namespace == "production"
}

# Require approval if remediation targets critical infrastructure
require_approval if {
    input.affected_resource.kind == "StatefulSet"
    input.affected_resource.namespace == "production"
    input.severity_level == "critical"
}
```

**Example 2: Compare signal source vs RCA target**
```rego
package kubernaut.approval

# Require approval if RCA target differs from signal source
require_approval if {
    input.target_resource.kind != input.affected_resource.kind
}

# Require approval if RCA escalates from staging to production
require_approval if {
    input.target_resource.namespace == "staging"
    input.affected_resource.namespace == "production"
}
```

**Example 3: API version-specific policies (for custom resources)**
```rego
package kubernaut.approval

# Require approval for custom resource remediations
require_approval if {
    input.affected_resource.api_version != "apps/v1"
    input.affected_resource.api_version != "v1"
    input.affected_resource.api_version != "batch/v1"
    # Non-core Kubernetes resources require approval
}
```

**Example 4: Fallback logic when affected_resource is not provided**
```rego
package kubernaut.approval

# Helper: Get remediation target (affected_resource if available, else target_resource)
remediation_target := input.affected_resource if {
    input.affected_resource != null
} else := input.target_resource

# Require approval based on remediation target
require_approval if {
    remediation_target.namespace == "production"
    remediation_target.kind == "Deployment"
}
```

**Example 5: Default-deny for missing affected_resource (recommended safety pattern)**
```rego
package kubernaut.approval

# Safety net: require approval when LLM failed to identify remediation target.
# Without this rule, kind-specific approval rules silently do not fire
# when affected_resource is nil, allowing remediation without approval.
require_approval if {
    not input.affected_resource
}

# These rules only evaluate when affected_resource is present
require_approval if {
    input.affected_resource.kind == "StatefulSet"
    input.affected_resource.namespace == "production"
}
```

**Acceptance Criteria**:
1. ✅ Documentation includes 5+ example policies using `affected_resource`
2. ✅ Examples cover: production targeting, source vs target comparison, API version checks, fallback logic, **default-deny safety pattern**
3. ✅ Examples are tested with unit tests in AIAnalysis
4. ✅ Examples use snake_case naming (`affected_resource`, `api_version`, `target_resource`)

---

### **FR-AI-085-004: Backward Compatibility**

**Requirement**: Existing Rego policies that do NOT reference `affected_resource` MUST continue to work unchanged.

**Backward Compatibility**:
- ✅ `affected_resource` is **optional** (pointer type) - policies can ignore it
- ✅ Existing policies using `target_resource` continue to work
- ✅ No breaking changes to existing `PolicyInput` fields
- ✅ Policies can check `if input.affected_resource != null` before using it

**Acceptance Criteria**:
1. ✅ Existing Rego policies pass without modification
2. ✅ New policies can use `affected_resource` without breaking old policies
3. ✅ Policy evaluation does not fail if `affected_resource` is nil

---

### **FR-AI-085-005: Default-Deny When `affected_resource` Is Missing**

**Requirement**: Rego policies that rely on `affected_resource` for kind-specific or namespace-specific approval decisions MUST include a **default-deny rule** that requires approval when `affected_resource` is undefined or null.

**Rationale**: Per ADR-055, the LLM is instructed to always provide `affectedResource` in its response, and the `WorkflowResponseValidator` enforces this with a 3-attempt self-correction loop. However, if after all retries `affectedResource` is still missing (e.g., LLM consistently fails to provide it), the Rego policy input will have `affected_resource = null`. In this scenario, kind-specific rules like `require_approval if { input.affected_resource.kind == "Deployment" }` would silently **not fire**, resulting in no approval being required. This is a **production safety gap**: unknown remediation targets must default to requiring approval.

**Default-Deny Pattern**:
```rego
package kubernaut.approval

# Default-deny: require approval when affected_resource is missing.
# This prevents silent bypass of kind/namespace-specific approval rules
# when the LLM fails to identify the target resource.
require_approval if {
    not input.affected_resource
}

# Kind-specific rules only fire when affected_resource is present
require_approval if {
    input.affected_resource.kind == "Deployment"
    input.affected_resource.namespace == "production"
}
```

**User-Facing Documentation**: This pattern MUST be documented in the Rego policy authoring guide as a **recommended safety practice** for any policy that uses `affected_resource` for approval decisions. Users should be advised that omitting the default-deny rule may allow remediations to proceed without approval when the LLM cannot determine the target resource.

**Acceptance Criteria**:
1. ✅ Default Rego approval policy includes a `require_approval` rule for missing `affected_resource`
2. ✅ Policy authoring documentation includes the default-deny pattern with explanation
3. ✅ Unit tests verify that missing `affected_resource` triggers approval requirement
4. ✅ Unit tests verify that present `affected_resource` bypasses the default-deny rule (kind-specific rules take precedence)

---

### **FR-AI-085-006: `action_type`-Based Infrastructure-Action Approval**

**Requirement**: AIAnalysis MUST expose `action_type` in the Rego policy input schema so approval policies can force human approval for infrastructure-impacting action types, independent of `remediation_target.kind` (the ADR-055-ADDENDUM-001 rename of `affected_resource`).

**Rationale (Issue #247)**: `remediation_target.kind`-based rules (FR-AI-085-005's `is_sensitive_resource`) require human approval for sensitive resource *kinds* (`Node`, `StatefulSet`). However, for infrastructure-provisioning workflows such as `ProvisionNode`, the LLM's RCA identifies the **source workload** (e.g. `Deployment`) as the remediation target -- not the `Node` actually being provisioned -- because the workload, not the not-yet-created node, is what the investigation observed. `remediation_target.kind` alone therefore cannot gate on "this action provisions infrastructure"; the *action type itself* is a more reliable signal, since `action_type` is catalog-authoritative (sourced from the `action_type_taxonomy` table via the KA three-step discovery protocol, per DD-WORKFLOW-016) rather than LLM-inferred.

**PolicyInput Struct** (`pkg/aianalysis/rego/evaluator.go`):
```go
type PolicyInput struct {
    // ... existing fields ...

    // ActionType is the catalog action type selected for remediation
    // (DD-WORKFLOW-016 taxonomy, e.g. ScaleReplicas, ProvisionNode).
    // Unlike RemediationTarget.Kind, this is catalog-authoritative
    // (not LLM-inferred), enabling reliable gating on infrastructure-
    // impacting actions regardless of which resource the LLM reports
    // as the remediation target (#247).
    ActionType string `json:"action_type"`
}
```

**Example Rego Rule**:
```rego
package aianalysis.approval

# #247: Infrastructure-provisioning action types always require approval,
# regardless of the (LLM-reported) remediation_target kind.
is_infrastructure_action if {
    input.action_type == "ProvisionNode"
}

require_approval if {
    is_infrastructure_action
}
```

**Acceptance Criteria**:
1. ✅ `PolicyInput` struct includes `ActionType string` field, populated from `Status.SelectedWorkflow.ActionType`
2. ✅ Default Rego approval policy includes an `is_infrastructure_action` rule gating on `action_type`
3. ✅ Unit tests verify that `action_type == "ProvisionNode"` triggers `require_approval` regardless of `remediation_target.kind` or confidence
4. ✅ Integration tests verify the rule against the real OPA evaluator (compile + query + extract)

---

## 📊 **Non-Functional Requirements**

### **NFR-AI-085-001: Performance Impact**

**Requirement**: Adding `affected_resource` to policy input MUST NOT degrade Rego evaluation performance.

**Performance Analysis**:
- ✅ `affected_resource` is already in memory (extracted from `Status.RootCauseAnalysis.TargetResource`)
- ✅ No additional API calls or database queries
- ✅ Minimal struct copy overhead (~80 bytes)
- ✅ Rego evaluation time unchanged (same policy complexity)

**Acceptance Criteria**:
1. ✅ Policy evaluation time increase < 1ms
2. ✅ No additional external API calls
3. ✅ Memory overhead < 100 bytes per evaluation

---

## 🔗 **Integration Points**

### **Upstream: AIAnalysis Response Processor**

**Integration**: `buildPolicyInput()` reads `Status.RootCauseAnalysis.TargetResource` (populated by BR-AI-084).

**Contract**:
- AIAnalysis response processor extracts `affectedResource` from HAPI and stores in `Status.RootCauseAnalysis.TargetResource`
- `buildPolicyInput()` reads this field and populates `input.affected_resource`
- If `TargetResource` is nil, `input.affected_resource` is nil (fallback to `target_resource`)

---

### **Downstream: Rego Policy Evaluation**

**Integration**: Rego policies receive `affected_resource` in `input` and use it for approval decisions.

**Contract**:
- `input.affected_resource` is OPTIONAL (may be nil)
- `input.target_resource` is ALWAYS present (signal source)
- Policies should check `if input.affected_resource != null` before using it
- Policies can use `affected_resource` for approval logic based on RCA target

---

## ✅ **Success Criteria**

### **Business Success**
1. ✅ 100% of approval policies can reference RCA target (`affected_resource`) for decisions
2. ✅ 0% of incorrect approvals due to signal source vs RCA target mismatch
3. ✅ Policy authors can write expressive rules based on actual remediation target

### **Technical Success**
1. ✅ `PolicyInput` struct includes `affected_resource` field
2. ✅ `buildPolicyInput()` correctly populates `affected_resource` from RCA status
3. ✅ Rego policies can access `input.affected_resource` without errors
4. ✅ Existing policies continue to work (backward compatible)

### **Quality Success**
1. ✅ 100% unit test coverage for `buildPolicyInput()` with `affected_resource`
2. ✅ 100% unit test coverage for example Rego policies
3. ✅ Policy evaluation performance unchanged (<1ms delta)

---

## 📈 **Business Value & Metrics**

### **Before (Current State)**
- ❌ Policies evaluate signal source, not RCA target (10-20% mismatch)
- ❌ Cannot write policies like "require approval for production Deployments" when signal is Pod
- ❌ Approval logic inconsistent with remediation logic

### **After (Target State)**
- ✅ Policies evaluate RCA target (100% alignment with remediation)
- ✅ Expressive policy language for complex approval rules
- ✅ Approval logic consistent with remediation logic

### **KPIs**
| Metric | Baseline | Target | Measurement |
|--------|----------|--------|-------------|
| Approval accuracy (correct target evaluated) | 80% | 100% | Policy evaluation logs |
| Policy expressiveness (uses RCA target) | 0% | 80% | Policy code analysis |
| Incorrect approvals (signal vs RCA mismatch) | 5-10% | 0% | Audit event analysis |

---

## 🚀 **Implementation Plan**

### **Phase 1: Schema Update**
1. Update `PolicyInput` struct with `AffectedResource *TargetResourceInput` field
2. Update `TargetResourceInput` struct with `APIVersion string` field
3. Update `buildPolicyInput()` to populate `affected_resource`

**Deliverables**:
- Updated `pkg/aianalysis/rego/evaluator.go`
- Updated `pkg/aianalysis/handlers/analyzing.go`

**Timeline**: 0.5 day

---

### **Phase 2: Testing**
1. Add unit tests for `buildPolicyInput()` with `affected_resource`
2. Add unit tests for example Rego policies
3. Validate backward compatibility with existing policies

**Deliverables**:
- AIAnalysis unit tests (8 test cases)
- Rego policy unit tests (4 example policies)

**Timeline**: 0.5 day

---

### **Phase 3: Documentation**
1. Update Rego policy documentation with `affected_resource` examples
2. Update policy authoring guide
3. Update DD-CONTRACT-002 with policy input schema section

**Deliverables**:
- Updated Rego policy documentation
- Updated DD-CONTRACT-002

**Timeline**: 0.5 day

---

## 📚 **Related Documents**

### **Design Decisions**
- **DD-HAPI-006**: Affected Resource in Root Cause Analysis (defines `affectedResource` contract)
- **DD-AIANALYSIS-001**: Rego Policy Loading Strategy (referenced - policy evaluation flow)
- **DD-CONTRACT-002**: Service Integration Contracts (needs update with policy input schema)

### **Business Requirements**
- **BR-AI-012**: Approval Policy Evaluation (referenced - needs creation/update)
- **BR-HAPI-212**: HAPI RCA Target Resource (upstream - provides `affectedResource`)
- **BR-AI-084**: AIAnalysis Extract RCA Target (upstream - populates `Status.RootCauseAnalysis.TargetResource`)
- **BR-SCOPE-010**: RO Routing Validation (downstream - uses RCA target for scope validation)

### **Architecture Decisions**
- **ADR-053**: Resource Scope Management (related - both use RCA target)

---

## 🎯 **Approval**

✅ **Approved by user**: 2026-01-20

**Approval Context**:
- User confirmed Rego policies should receive `affected_resource` (not `target_resource`) for approval decisions
- User confirmed `api_version` should use snake_case for Rego (Rego convention)
- User confirmed backward compatibility requirement (existing policies must work)

---

## 🔒 **Confidence Assessment**

**Confidence Level**: 95%

**Strengths**:
- ✅ Clear use cases for `affected_resource` in approval policies
- ✅ Minimal code changes (struct field + builder logic)
- ✅ Backward compatible (optional field)
- ✅ Aligns with existing Rego policy infrastructure
- ✅ Default-deny pattern (FR-AI-085-005) prevents silent approval bypass when `affected_resource` is missing

**Risks**:
- ⚠️ **5% Gap**: Policy authors may forget to include the default-deny rule in custom policies
  - **Mitigation**: Default approval policy ships with the default-deny rule pre-configured
  - **Mitigation**: Policy authoring documentation prominently features the default-deny pattern
  - **Mitigation**: Example policies (Example 5) demonstrate the safety pattern

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-07-07
- **Version**: 1.4
- **Status**: ✅ Approved
- **Changes in 1.1**: Added FR-AI-085-005 (default-deny for missing `affected_resource`), added ADR-055 reference, documented `target_in_owner_chain` supersession, added Example 5 (default-deny safety pattern).
- **Changes in 1.2**: Added `ConfidenceThreshold *float64` to PolicyInput schema (#225). Confidence threshold is now configurable via `input.confidence_threshold` in the Rego policy, with a built-in default of 0.8. Stepping stone toward BR-HAPI-198.
- **Changes in 1.3**: Added FR-AI-085-006 (`action_type`-based infrastructure-action approval, #247). `action_type` is now exposed in the Rego policy input alongside `remediation_target` (the ADR-055-ADDENDUM-001 rename of `affected_resource`), so infrastructure-provisioning workflows (e.g. `ProvisionNode`) require approval based on the catalog-authoritative action type rather than the (potentially LLM-misreported) remediation target kind.
- **Changes in 1.4**: `#247` follow-up -- decomposed the Go `PolicyInput` struct into `SignalContext`/`Classification`/`KAResponse` sub-structs to keep the top-level field count below the AGENTS.md Go Anti-Pattern Checklist's "God struct" threshold (had reached 15 fields). Go-side organization only: `buildRegoInputMap` still flattens to the same top-level Rego input keys, so this is **not a Rego policy contract change** -- no `.rego` policy file needs updating.
- **Next Review**: After BR-HAPI-198 V1.1 implementation

**Note on field naming**: This document's earlier FRs (001-005) use the original `affected_resource` naming from when this BR was authored. Per [ADR-055-ADDENDUM-001](../architecture/decisions/remediation-target-rename.md) (#542), this field was subsequently renamed to `remediation_target` end-to-end (Go struct, JSON tag, Rego input key) to avoid LLM misinterpretation. FR-AI-085-006 uses the current `remediation_target` naming; the code examples in FR-AI-085-001 through 005 are retained as historical context and should be read as `remediation_target` in the current implementation.
