# Test Plan: Tool-Based Structured Output (submit_result)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-686-v1
**Feature**: Replace text-based structured output with `submit_result` tool call for universal provider compatibility
**Version**: 1.0
**Created**: 2026-04-14
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/684-vertex-ai-claude`

---

## 1. Introduction

### 1.1 Purpose

Validate that the `submit_result` tool-based structured output mechanism works correctly across all LLM providers (Vertex AI, Anthropic, OpenAI, Azure, Bedrock), replacing the fragile text-parsing approach that fails when `output_config` (constrained decoding) is unavailable. This test plan ensures the LLM's investigation results are reliably captured via tool call arguments — a universally supported mechanism.

### 1.2 Objectives

1. **submit_result sentinel detection**: `runLLMLoop` correctly intercepts `submit_result` tool calls and returns arguments as content without executing a tool
2. **Tool definition injection**: `submit_result` tool is appended to tool definitions for both RCA and WorkflowDiscovery phases with the correct JSON schema
3. **Parser compatibility**: Existing `ResultParser` correctly parses the JSON arguments returned by the sentinel path
4. **Prompt unification**: Both prompt templates produce a single instruction format that works with all providers
5. **Section-header fallback**: Parser handles section-header format as safety net when LLM emits text instead of calling the tool
6. **No regressions**: All existing parser, investigator, and adapter tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-kubernautagent` |
| Integration test pass rate | 100% | `make test-integration-kubernautagent` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on parser, investigator, prompt |
| Backward compatibility | 0 regressions | All 22 existing unit suites pass |
| End-to-end validation | Workflow selection parsed | Kind cluster + Vertex AI demo scenario |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-AI-056: LLM framework isolation pattern
- DD-HAPI-019: Framework isolation — business logic never imports underlying LLM framework
- Issue #684: Vertex AI + Claude 3-bug regression
- Issue #686: GCP credential indirection + structured output gap

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Plan: Tool-Based Structured Output](../../../.cursor/plans/tool-based_structured_output_8bdbd577.plan.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | LLM ignores submit_result tool and emits free text | Workflow selection not parsed, escalated to manual review | Low (tool calling is reliable across all tested providers) | UT-KA-686-010, UT-KA-686-011 | Section-header fallback parser + existing extractJSON chain |
| R2 | Anomaly detector blocks submit_result tool call | Investigation result lost | Medium | UT-KA-686-004 | submit_result bypasses anomaly detector (sentinel, not a real tool) |
| R3 | submit_result schema drift from parser expectations | Parse failures on valid tool arguments | Low | UT-KA-686-005 | Schema is the same investigationResultSchemaJSON used by both tool def and parser |
| R4 | Prompt change causes regressions for providers with structured output | Existing anthropic/openai flow breaks | Low | UT-KA-686-008 | StructuredOutputTransport remains as defense-in-depth for direct anthropic; unified prompt is simpler |
| R5 | Multi-turn loop exits before submit_result if LLM produces text + tool call in same turn | Partial result | Low | UT-KA-686-006 | Sentinel check handles mixed content blocks (text + tool_use) |

### 3.1 Risk-to-Test Traceability

- **R1 (High)**: UT-KA-686-010 (section-header fallback), UT-KA-686-011 (fenced JSON fallback)
- **R2 (Medium)**: UT-KA-686-004 (anomaly detector bypass)
- **R3 (Low)**: UT-KA-686-005 (schema round-trip)
- **R4 (Low)**: UT-KA-686-008 (prompt renders without StructuredOutput branching)
- **R5 (Low)**: UT-KA-686-006 (mixed text + submit_result)

---

## 4. Scope

### 4.1 Features to be Tested

- **submit_result sentinel** (`internal/kubernautagent/investigator/investigator.go`): Detection in runLLMLoop, argument extraction, anomaly detector bypass
- **Tool definition injection** (`internal/kubernautagent/investigator/investigator.go`): submit_result appended to tool defs for RCA and WorkflowDiscovery phases
- **Section-header parser fallback** (`internal/kubernautagent/parser/parser.go`): parseSectionHeaders extracts structured result from "# header" format
- **Prompt templates** (`internal/kubernautagent/prompt/templates/`): Unified output instructions, no StructuredOutput branching
- **Prompt builder** (`internal/kubernautagent/prompt/builder.go`): Renders correctly with and without StructuredOutput flag

### 4.2 Features Not to be Tested

- **vertexanthropic SDK adapter** (`pkg/kubernautagent/llm/vertexanthropic/client.go`): Already tested and validated (14 unit tests, 22 suites pass). No changes in this plan.
- **StructuredOutputTransport** (`pkg/kubernautagent/llm/transport/structured_output.go`): Remains unchanged for direct anthropic API path. No modifications.
- **Credential resolution** (`internal/kubernautagent/credentials/resolver.go`): Already fixed and tested in prior commits.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| submit_result is a sentinel, not a registered tool | Avoids polluting the tool registry; simple string comparison in the loop |
| Keep existing parser as fallback | Defense-in-depth if LLM emits text instead of calling the tool |
| Keep StructuredOutputTransport for direct anthropic | Zero risk, provides additional constraint layer for that path |
| Section-header parser as secondary fallback | Safety net for edge cases; prompt no longer instructs this format |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (parser, prompt builder, tool definition logic, sentinel detection)
- **Integration**: >=80% of integration-testable code (full investigator loop with httptest LLM)
- **E2E**: Deferred to demo team Kind cluster validation (Vertex AI requires real GCP credentials)

### 5.2 Two-Tier Minimum

Every business requirement covered by at least UT + IT.

### 5.3 Business Outcome Quality Bar

Tests validate that the system reliably captures structured investigation results from any LLM provider, ensuring workflow selection and RCA data flow through to downstream services.

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage >=80%
4. No regressions in existing 22 unit suites
5. Demo team validates end-to-end on Kind cluster with Vertex AI

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage below 80%
3. Existing tests regress
4. Vertex AI end-to-end produces "workflow selection parse failed"

### 5.5 Suspension & Resumption Criteria

**Suspend**: Kind cluster unavailable, Podman VM disk full (do NOT prune — cluster runs on Podman)
**Resume**: Cluster restored, disk space freed via targeted image removal (not system prune)

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/parser/parser.go` | `Parse`, `parseSectionHeaders`, `extractSections`, `extractJSON` | ~520 |
| `internal/kubernautagent/parser/schema.go` | `InvestigationResultSchema` | ~86 |
| `internal/kubernautagent/prompt/builder.go` | `RenderWorkflowSelection`, `RenderIncidentInvestigation` | ~337 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `runLLMLoop`, `toolDefinitionsForPhase`, `runWorkflowSelection`, `runRCA` | ~811 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-056 | LLM results captured via tool call (submit_result sentinel) | P0 | Unit | UT-KA-686-001 | Pending |
| BR-AI-056 | submit_result tool definition includes correct schema | P0 | Unit | UT-KA-686-002 | Pending |
| BR-AI-056 | submit_result returns arguments as content, not executed | P0 | Unit | UT-KA-686-003 | Pending |
| BR-AI-056 | submit_result bypasses anomaly detector | P0 | Unit | UT-KA-686-004 | Pending |
| BR-AI-056 | Schema round-trip: tool args parse to InvestigationResult | P0 | Unit | UT-KA-686-005 | Pending |
| BR-AI-056 | Mixed text + submit_result tool call handled | P1 | Unit | UT-KA-686-006 | Pending |
| BR-AI-056 | Regular tool calls still executed normally | P0 | Unit | UT-KA-686-007 | Pending |
| BR-AI-056 | Prompt renders without StructuredOutput branching | P0 | Unit | UT-KA-686-008 | Pending |
| BR-AI-056 | Prompt includes submit_result instruction | P0 | Unit | UT-KA-686-009 | Pending |
| BR-AI-056 | Section-header fallback parses # header format | P1 | Unit | UT-KA-686-010 | Pending |
| BR-AI-056 | Section-header fallback with fenced JSON blocks | P1 | Unit | UT-KA-686-011 | Pending |
| BR-AI-056 | Section-header fallback with RCA only (no workflow) | P1 | Unit | UT-KA-686-012 | Pending |
| BR-AI-056 | Full investigator loop with submit_result (httptest) | P0 | Integration | IT-KA-686-001 | Pending |
| BR-AI-056 | Investigator falls back to text when no submit_result | P1 | Integration | IT-KA-686-002 | Pending |
| BR-AI-056 | Self-correction loop works with submit_result | P1 | Integration | IT-KA-686-003 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `parser/parser.go`, `parser/schema.go`, `prompt/builder.go`, investigator sentinel logic

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-686-001 | submit_result tool call detected as sentinel in runLLMLoop | Pending |
| UT-KA-686-002 | submit_result tool definition has correct name, description, and schema params | Pending |
| UT-KA-686-003 | submit_result arguments returned as loop content, not passed to executeTool | Pending |
| UT-KA-686-004 | submit_result call bypasses anomaly detector check | Pending |
| UT-KA-686-005 | Tool arguments matching investigationResultSchemaJSON parse to valid InvestigationResult | Pending |
| UT-KA-686-006 | Response with text content AND submit_result tool call extracts arguments | Pending |
| UT-KA-686-007 | Non-submit_result tool calls (kubectl_describe, etc.) still route to executeTool | Pending |
| UT-KA-686-008 | Prompt templates render without StructuredOutput if/else branching | Pending |
| UT-KA-686-009 | Rendered prompt contains "submit_result" tool instruction | Pending |
| UT-KA-686-010 | parseSectionHeaders extracts RCA + workflow from "# header" format | Pending |
| UT-KA-686-011 | parseSectionHeaders handles fenced JSON blocks under headers | Pending |
| UT-KA-686-012 | parseSectionHeaders handles RCA-only (no workflow, actionable=false) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `investigator/investigator.go` full loop with httptest LLM server

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-686-001 | Full investigator loop: LLM calls submit_result, workflow selection parsed | Pending |
| IT-KA-686-002 | Full investigator loop: LLM emits text (no tool call), parser extracts result | Pending |
| IT-KA-686-003 | Self-correction: validation fails, LLM re-submits via submit_result | Pending |

### Tier 3: E2E Tests

**Tier skip rationale**: E2E validation requires real GCP Vertex AI credentials and a live Claude model. Deferred to demo team Kind cluster validation with the built image. The golden transcript from the demo team serves as the E2E acceptance test.

---

## 9. Test Cases

### UT-KA-686-001: submit_result sentinel detection

**BR**: BR-AI-056
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/submit_result_test.go`

**Preconditions**: runLLMLoop receives a ChatResponse with a single ToolCall named "submit_result"

**Test Steps**:
1. **Given**: A mock LLM client that returns a ChatResponse with ToolCalls=[{Name: "submit_result", Arguments: `{"root_cause_analysis": {"summary": "OOM"}, "confidence": 0.95}`}]
2. **When**: runLLMLoop processes the response
3. **Then**: The loop returns the tool call arguments as the content string, does not call executeTool

**Expected Results**:
1. Returned content is the JSON arguments string
2. executeTool is never called for submit_result

### UT-KA-686-005: Schema round-trip

**BR**: BR-AI-056
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Preconditions**: JSON string matching investigationResultSchemaJSON structure

**Test Steps**:
1. **Given**: A JSON string with root_cause_analysis, selected_workflow, confidence, severity
2. **When**: ResultParser.Parse() processes it
3. **Then**: InvestigationResult has all fields populated correctly

**Expected Results**:
1. RCASummary, WorkflowID, Confidence, Severity, Parameters, RemediationTarget all populated
2. AlternativeWorkflows parsed

### UT-KA-686-010: Section-header fallback

**BR**: BR-AI-056
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Preconditions**: LLM output in "# header\nJSON" format

**Test Steps**:
1. **Given**: Content with `# root_cause_analysis`, `# confidence`, `# selected_workflow`, `# alternative_workflows` headers followed by JSON values
2. **When**: ResultParser.Parse() processes it
3. **Then**: InvestigationResult correctly assembled from all sections

**Expected Results**:
1. RCASummary from root_cause_analysis section
2. WorkflowID from selected_workflow section
3. Confidence from confidence section
4. AlternativeWorkflows from alternative_workflows section

### IT-KA-686-001: Full investigator loop with submit_result

**BR**: BR-AI-056
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/submit_result_test.go`

**Preconditions**: httptest server simulating multi-turn LLM conversation ending with submit_result tool call

**Test Steps**:
1. **Given**: httptest LLM that responds to turn 0 with tool calls (kubectl_describe), turn 1 with submit_result tool call containing full investigation result
2. **When**: Investigator.Investigate() runs the full pipeline
3. **Then**: InvestigationResult returned with correct WorkflowID, RCASummary, Parameters

**Expected Results**:
1. Investigation completes without error
2. WorkflowID matches the value in submit_result arguments
3. No "workflow selection parse failed" warning in logs

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Mock llm.Client (returns predefined ChatResponse with submit_result tool call)
- **Location**: `test/unit/kubernautagent/parser/`, `test/unit/kubernautagent/investigator/`, `test/unit/kubernautagent/prompt/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO (httptest server simulates real LLM HTTP endpoint)
- **Infrastructure**: httptest, no external services
- **Location**: `test/integration/kubernautagent/investigator/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Podman | 5.x | Image build (no system prune!) |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Anthropic SDK v1.35.1 | Library | Merged | vertexanthropic adapter | Already in go.mod |
| Prior commits (SDK migration, multi-turn fix) | Code | Merged | Base for this work | Already on branch |

### 11.2 Execution Order

1. **Phase 1 — TDD RED**: Write all failing tests (UT-KA-686-001 through 012, IT-KA-686-001 through 003)
2. **Checkpoint 1**: Comprehensive audit — verify all tests fail for the right reason, no anti-patterns, schema correctness
3. **Phase 2 — TDD GREEN**: Implement submit_result sentinel, tool definition injection, prompt changes, section-header parser fix
4. **Checkpoint 2**: Comprehensive audit — all tests pass, no regressions, go build/vet clean, coverage >=80%
5. **Phase 3 — TDD REFACTOR**: Clean up dead code, remove unused StructuredOutput branching, verify no dead imports
6. **Checkpoint 3**: Final audit — lint clean, all 22+ suites pass, commit and push
7. **Phase 4 — Build & Validate**: Build local image, load into Kind, demo team validates end-to-end

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/686/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/parser/parser_test.go`, `test/unit/kubernautagent/prompt/builder_test.go` | Parser fallback + prompt tests |
| Integration test suite | `test/integration/kubernautagent/investigator/` | Full loop with submit_result |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
make test-unit-kubernautagent

# Focused tests
go test ./test/unit/kubernautagent/parser/... --ginkgo.focus="686"
go test ./test/unit/kubernautagent/prompt/... --ginkgo.focus="686"

# Integration tests
make test-integration-kubernautagent

# Coverage
go test ./test/unit/kubernautagent/parser/... -coverprofile=coverage_parser.out
go tool cover -func=coverage_parser.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/kubernautagent/prompt/builder_test.go` | Asserts StructuredOutput branch produces section headers | Update to assert unified submit_result instruction | Prompt template change removes branching |
| `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go` | May assert on response content format | Verify still passes with tool-based output | Integration loop change |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-14 | Initial test plan |
