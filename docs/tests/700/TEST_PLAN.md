# Test Plan: Phase Separation — RCA / Workflow Selection

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-700-v1
**Feature**: Enforce clean RCA / Workflow Selection phase separation with per-session structured output
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Agent (supervised)
**Status**: Draft
**Branch**: `fix-700`

---

## 1. Introduction

### 1.1 Purpose

Validate that the KA investigation pipeline enforces clean session separation between RCA (Phase 1) and Workflow Selection (Phase 3), aligned with the HAPI v1.2.1 baseline. The LLM must not be able to set `needs_human_review` or reference workflow selection during RCA, and the structured output schema must be phase-specific — both at the tool level (`submit_result` Parameters) and at the transport level (`output_config.format`).

### 1.2 Objectives

1. **Phase-specific schemas**: RCA phase uses `RCAResultSchema()` (no workflow/escalation fields); Workflow phase uses `InvestigationResultSchema()` (full schema).
2. **Focused prompts**: RCA prompt contains only investigation instructions (no Phases 4-5, no workflow references, no `needs_human_review`).
3. **Per-session structured output**: `StructuredOutputTransport` reads schema from request context, not from a global field. Each LLM call carries its own schema.
4. **Defense-in-depth**: Investigator clears `HumanReviewNeeded` after RCA parsing. Only max-turns exhaustion can abort during RCA.
5. **No regressions**: Existing passing scenarios (crashloop, stuck-rollout, orphaned-pvc) continue to pass.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on schema, prompt, transport, investigator |
| Backward compatibility | 0 regressions | All pre-existing tests pass |

---

## 2. References

### 2.1 Authority

- Issue #700: v1.3.0 KA: LLM escalates to ManualReview on scenarios that HAPI v1.2.1 remediates successfully
- HAPI v1.2.1 source: `holmesgpt-api/src/extensions/incident/prompt_builder.py` (PHASE1_SECTIONS, PHASE3_SECTIONS)
- HAPI v1.2.1 source: `holmesgpt-api/src/extensions/incident/llm_integration.py` (three-phase orchestrator)
- HAPI v1.2.1 source: `holmesgpt-api/src/extensions/incident/result_parser.py` (parser-driven needs_human_review)
- BR-HAPI-002: Incident Analysis
- BR-HAPI-197: needs_human_review field
- BR-HAPI-200: Investigation inconclusive outcome

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- kubernaut-docs#112: Architecture Guide for KA Investigation Pipeline

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Prompt changes affect passing scenarios | RCA quality degrades for crashloop, stuck-rollout, orphaned-pvc | Low | IT-KA-700-001 | Removed text is workflow/escalation-specific, not RCA-relevant. IT regression tests provide coverage. |
| R2 | LangChainGo context dropped before transport | OutputSchema not available in RoundTrip | Very Low | UT-KA-700-013 | Preflight verified: LangChainGo v0.1.14 uses `http.NewRequestWithContext(ctx)` throughout. |
| R3 | Vertex AI path not constrained by OutputSchema | Global schema gap for production provider | Medium | UT-KA-700-007, UT-KA-700-008 | Vertex AI constrained by `submit_result` tool Parameters schema. Transport-level constraint is defense-in-depth. |
| R4 | Parser fallback paths still set HumanReviewNeeded from RCA | Defense-in-depth bypass | Low | UT-KA-700-009 | Investigator explicitly clears HumanReviewNeeded after RCA parsing. |

### 3.1 Risk-to-Test Traceability

| Risk | Primary Test(s) | Secondary Test(s) |
|------|----------------|-------------------|
| R1 | IT-KA-700-001 | UT-KA-700-003, UT-KA-700-004 |
| R2 | UT-KA-700-013 | UT-KA-700-011 |
| R3 | UT-KA-700-007, UT-KA-700-008 | UT-KA-700-001, UT-KA-700-002 |
| R4 | UT-KA-700-009 | IT-KA-700-002 |

---

## 4. Test Items

### 4.1 Components Under Test

| Component | File | What Changes |
|-----------|------|-------------|
| RCA schema | `internal/kubernautagent/parser/schema.go` | New `RCAResultSchema()` function |
| Investigation schema | `internal/kubernautagent/parser/schema.go` | Unchanged (regression guard) |
| RCA prompt template | `internal/kubernautagent/prompt/templates/incident_investigation.tmpl` | Remove Phases 4-5, workflow refs, needs_human_review, remediation history |
| Prompt builder | `internal/kubernautagent/prompt/builder.go` | Skip remediation history in RenderInvestigation |
| Investigator | `internal/kubernautagent/investigator/investigator.go` | Phase-specific submit_result schema, per-session OutputSchema, clear HumanReviewNeeded after RCA |
| LLM types | `pkg/kubernautagent/llm/types.go` | Add OutputSchema to ChatOptions |
| Structured output transport | `pkg/kubernautagent/llm/transport/structured_output.go` | Context-based schema, remove global schema field |
| LangChainGo adapter | `pkg/kubernautagent/llm/langchaingo/adapter.go` | Propagate OutputSchema to context |
| Main entrypoint | `cmd/kubernautagent/main.go` | Remove global schema from transport constructor |

---

## 5. Test Scenarios

### 5.1 Tier 1: Unit Tests

#### Schema & Prompt (6 tests)

| ID | Description | Component | Acceptance Criterion |
|----|------------|-----------|---------------------|
| UT-KA-700-001 | RCAResultSchema contains only RCA fields | `parser/schema.go` | Schema JSON has `root_cause_analysis`, `confidence`, `investigation_outcome`, `actionable`, `severity`, `detected_labels`. Does NOT have `selected_workflow`, `alternative_workflows`, `needs_human_review`, `human_review_reason`. |
| UT-KA-700-002 | InvestigationResultSchema unchanged | `parser/schema.go` | Schema JSON still contains `selected_workflow`, `alternative_workflows`, `needs_human_review`. Byte-for-byte identical to pre-#700 value. |
| UT-KA-700-003 | RCA prompt excludes workflow discovery | `incident_investigation.tmpl`, `builder.go` | Rendered RCA prompt does NOT contain "list_available_actions", "list_workflows", "get_workflow", "Three-Step Protocol", "Phase 4", "Phase 5". |
| UT-KA-700-004 | RCA prompt excludes escalation fields | `incident_investigation.tmpl`, `builder.go` | Rendered RCA prompt does NOT contain "selected_workflow", "needs_human_review", "human_review_reason", "alternative_workflows". |
| UT-KA-700-005 | RCA prompt excludes remediation history | `builder.go` | Rendered RCA prompt does NOT contain "Remediation History Context", "CONFIGURATION REGRESSION DETECTED". Even when enrichment data includes history. |
| UT-KA-700-006 | Workflow selection prompt retains full content | `phase3_workflow_selection.tmpl`, `builder.go` | Rendered workflow prompt contains "list_available_actions", "list_workflows", "get_workflow", "submit_result", "selected_workflow". |

#### Investigator Phase Separation (4 tests)

| ID | Description | Component | Acceptance Criterion |
|----|------------|-----------|---------------------|
| UT-KA-700-007 | RCA phase submit_result uses RCAResultSchema | `investigator.go` | `toolDefinitionsForPhase(PhaseRCA)` returns submit_result with Parameters matching `RCAResultSchema()`. |
| UT-KA-700-008 | Workflow phase submit_result uses InvestigationResultSchema | `investigator.go` | `toolDefinitionsForPhase(PhaseWorkflowDiscovery)` returns submit_result with Parameters matching `InvestigationResultSchema()`. |
| UT-KA-700-009 | runRCA clears HumanReviewNeeded after parsing | `investigator.go` | When LLM returns JSON with `needs_human_review: true` during RCA, `runRCA` returns result with `HumanReviewNeeded == false`. |
| UT-KA-700-010 | Max-turns exhaustion preserves HumanReviewNeeded | `investigator.go` | When RCA exhausts max turns, result has `HumanReviewNeeded == true` with reason containing "max turns". |

#### Per-Session Structured Output Transport (3 tests)

| ID | Description | Component | Acceptance Criterion |
|----|------------|-----------|---------------------|
| UT-KA-700-011 | Transport uses schema from context | `structured_output.go` | When request context contains OutputSchema via `WithOutputSchema`, the injected `output_config.format.schema` matches that schema. |
| UT-KA-700-012 | Transport skips injection when no schema in context | `structured_output.go` | When request context has no OutputSchema, the request body passes through unmodified (no `output_config` added). |
| UT-KA-700-013 | ChatOptions.OutputSchema propagates through adapter | `adapter.go`, `structured_output.go` | End-to-end: setting `ChatOptions.OutputSchema` on a ChatRequest results in the correct schema being injected into the HTTP request body by the transport. |

### 5.2 Tier 2: Integration Tests

| ID | Description | Component | Acceptance Criterion |
|----|------------|-----------|---------------------|
| IT-KA-700-001 | Two-session flow: RCA feeds workflow selection | `investigator.go` | Full `Investigate()` call: RCA session produces summary, workflow session receives it and selects a workflow. Final result has both `RCASummary` and `WorkflowID` populated. |
| IT-KA-700-002 | RCA cannot abort pipeline via needs_human_review | `investigator.go` | Mock LLM returns `needs_human_review: true` in RCA submit_result. Pipeline does NOT abort — proceeds to workflow selection. Final result has `HumanReviewNeeded` determined by workflow phase. |
| IT-KA-700-003 | RCA investigation_outcome does not skip workflow selection | `investigator.go` | Mock LLM returns `investigation_outcome: "inconclusive"` in RCA. Pipeline proceeds to workflow selection (does not short-circuit). |

---

## 6. TDD Phase Mapping

| TDD Phase | Test IDs | Implementation Files |
|-----------|----------|---------------------|
| RED | All UT-KA-700-*, IT-KA-700-* | Tests only (no implementation) |
| GREEN | — | `schema.go`, `incident_investigation.tmpl`, `builder.go`, `investigator.go`, `types.go`, `structured_output.go`, `adapter.go`, `main.go` |
| REFACTOR | — | Dead template removal, documentation alignment |

---

## 7. Environment

| Component | Version |
|-----------|---------|
| Go | 1.24+ |
| LangChainGo | v0.1.14 |
| Ginkgo/Gomega | v2 |
| Test runner | `go test` / `ginkgo` |
