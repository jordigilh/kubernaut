# Test Plan: Selected Workflow Identity in Rego Policy Input

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2714-v1.0
**Feature**: Expose selected workflow identity (ID, name, version) to the Rego approval policy as `input.selected_workflow`
**Version**: 1.0
**Created**: 2026-11-27
**Author**: jordigilh (via agent)
**Status**: Active
**Branch**: `issue-2714-workflow-identity-rego`

---

## 1. Introduction

### 1.1 Purpose

The Rego approval policy cannot see which workflow the agent selected, blocking
selective autonomy per reviewed workflow (trust ladder Stage 3). This plan proves
that `input.selected_workflow.id`, `.name` and `.version` are populated from the
catalog-authoritative `WorkflowSnapshot` and reach a real OPA evaluation, without
breaking existing policies when the field is absent.

### 1.2 Objectives

1. **Identity exposure**: `PolicyInput.SelectedWorkflow` carries `id`/`name`/`version` sourced from `analysis.Status.RCAResult.SelectedWorkflow` (`sharedtypes.WorkflowSnapshot`), never from LLM output.
2. **Wire fidelity**: `buildRegoInputMap` emits a `selected_workflow` object only when non-nil (`omitempty` semantics preserved for existing policies).
3. **Wiring**: `buildPolicyInput` (production path in `pkg/aianalysis/handlers/analyzing.go`, reached via `AnalyzingHandler.Handle`) populates the field.
4. **Backward compatibility**: existing policies and tests unaffected when the field is absent.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/aianalysis/...` |
| Integration test pass rate | 100% | `make test-integration-aianalysis GINKGO_FOCUS=<focus>` (supplies envtest assets); or `KUBEBUILDER_ASSETS=$(setup-envtest use 1.31.0 -p path) go test ./test/integration/aianalysis/ -ginkgo.label-filter=selected-workflow` |
| Backward compatibility | 0 regressions | Existing suites pass unmodified |
| Scenario coverage (UT) | 100% of in-scope scenarios | Matrix below |

---

## 2. References

- BR-AI-085 (FR-AI-085-007, new): Rego policy input schema — `selected_workflow`
- Issue #2714: Expose selected workflow identity (ID, name, version) to the Rego approval policy
- DD-WORKFLOW-018 / Issue #1661 Change 12: `WorkflowSnapshot` is the catalog-authoritative, immutable execution snapshot
- `pkg/aianalysis/rego/evaluator.go`, `pkg/aianalysis/handlers/analyzing.go`
- Precedent pattern: identity input (#774, BR-AI-085, `rego_identity_test.go`, `rego_identity_integration_test.go`)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Policy input shape drift between `PolicyInput` JSON tags and `buildRegoInputMap` manual map | Rego reads wrong/missing keys | Medium | UT-AA-2714-001..003, IT-AA-2714-001..003 | UT asserts the input-map key set via real OPA evaluation; IT evaluates a policy using `input.selected_workflow` |
| R2 | Field sourced from LLM output instead of catalog snapshot | Trust-ladder guarantee broken (autonomy granted on unreviewed identity) | Low | UT-AA-2714-002 | UT asserts population comes from `Status.RCAResult.SelectedWorkflow` (WorkflowSnapshot embed) — that struct is catalog-resolved by construction (DD-WORKFLOW-018) |
| R3 | Existing policies break when field absent | Regression in approval decisions | Low | UT-AA-2714-004/005, IT-AA-2714-003, unmodified existing suites | `omitempty`/nil-guard: key omitted from input map when nil; default-deny on absent key proven against the new fixture; backward compatibility proven by all pre-existing suites passing unmodified with nil `SelectedWorkflow` |

---

## 4. Test Scenarios

### 4.1 Unit (Ginkgo/Gomega, `pkg/aianalysis/rego_selected_workflow_test.go` + `pkg/aianalysis/rego/build_input_map_selected_workflow_test.go`)

| ID | Scenario | Priority | Outcome |
|----|----------|----------|---------|
| UT-AA-2714-001 | Policy using `input.selected_workflow.name`/`.version` auto-approves for trusted workflow (real OPA eval) | P0 | Pass |
| UT-AA-2714-002 | `buildPolicyInput` (via `Handle` + spy evaluator, production dispatch path) populates `SelectedWorkflow{ID,Name,Version}` from `RCAResult.SelectedWorkflow` snapshot | P0 | Pass |
| UT-AA-2714-003 | Untrusted workflow name/version requires approval | P1 | Pass |
| UT-AA-2714-004 | Nil `SelectedWorkflow` input: `selected_workflow` key absent from Rego input; new-policy default-deny on absent key (no panic); backward compatibility proven by all pre-existing suites passing unmodified with nil `SelectedWorkflow` | P0 | Pass |
| UT-AA-2714-005 | Wire-shape pin: `buildRegoInputMap` omits `selected_workflow` key entirely (absent, not null) when nil; emits `id`/`name`/`version` object when present | P0 | Pass |

### 4.2 Integration (`test/integration/aianalysis/rego_selected_workflow_integration_test.go`)

| ID | Scenario | Priority | Outcome |
|----|----------|----------|---------|
| IT-AA-2714-001 | End-to-end chain: `PolicyInput` with `SelectedWorkflow` -> hot-reloaded evaluator -> policy referencing `input.selected_workflow` auto-approves | P0 | Pass |
| IT-AA-2714-002 | Same chain with non-trusted workflow identity requires approval | P0 | Pass |
| IT-AA-2714-003 | Nil `SelectedWorkflow` through the real evaluator: policy defaults to require/approve without error | P1 | Pass |

### 4.3 Control Objectives

- **FedRAMP AC-6 (least privilege)**: selective autonomy can now be scoped to a specific reviewed workflow (name+version), narrowing auto-approval blast radius from whole action types.
- **FedRAMP AC-4 / SI-10**: workflow identity is catalog-authoritative (DataStorage-resolved `WorkflowSnapshot`), not LLM-suppliable — the policy gates on validated input.
- **SOC2 CC7.2**: approval-decision audit trails already record workflow identity (`workflow_id` join key, IT-AW-1111-001); this change does not alter audit emission — no new audit events required (field is policy-input only).

---

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|-----------|
| `rego.PolicyInput.SelectedWorkflow` / `SelectedWorkflowInput` | `AnalyzingHandler.Handle` (AIAnalysis controller, `cmd/aianalysis`) | `pkg/aianalysis/handlers/analyzing.go` `buildPolicyInput` + `pkg/aianalysis/rego/evaluator.go` `buildRegoInputMap` | IT-AA-2714-001..003 (evaluator through real OPA, per #774 precedent) |

No new components are created; existing ones are extended (CHECKPOINT B satisfied: identity-input pattern reused).

---

## 6. TDD Phase Mapping

- **RED**: UT-AA-2714-001..004 + IT-AA-2714-001..003 fail (field/types do not exist).
- **GREEN**: minimal `SelectedWorkflowInput` type, `PolicyInput` field, `buildRegoInputMap` nil-guarded key, `buildPolicyInput` population.
- **REFACTOR**: doc comments tying to BR-AI-085 FR-AI-085-007 / trust ladder; anti-pattern check (no god-struct growth: field count stays below 15; pointer sub-struct).
