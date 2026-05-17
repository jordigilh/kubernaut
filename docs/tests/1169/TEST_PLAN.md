# Test Plan: Surface LLM-Populated Workflow Parameters in discover_workflows (#1169)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1169-v1
**Feature**: Per-workflow parameters in MCP discover_workflows response for AF/interactive clients
**Version**: 1.0
**Created**: 2026-05-17
**Author**: AI-assisted
**Status**: Draft
**Branch**: `feat/1169-discover-workflows-parameters`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that LLM-populated workflow parameters are surfaced to
interactive MCP clients (AF and others) in the `discover_workflows` response, and
that selecting any workflow (recommended or alternative) merges the correct
per-workflow parameters into the final `InvestigationResult`.

Currently, `InvestigationResult.Parameters` only carries the **recommended**
workflow's parameters. Alternative workflows have no parameter slot at any layer
(types, parser, LLM schema, MCP DTOs). When a user selects an alternative via
`select_workflow`, the recommended workflow's parameters leak into the final
result — a **correctness bug** that blocks AF from showing accurate parameter
previews.

### 1.2 Objectives

1. **Parser fidelity**: `parseLLMFormat` extracts `parameters` from `alternative_workflows` items and propagates them into `katypes.AlternativeWorkflow.Parameters`
2. **Discovery completeness**: `extractDiscoveryResult` copies per-workflow parameters onto `DiscoveredWorkflow` for both recommended and alternatives
3. **Selection correctness**: `buildFinalResult` replaces `InvestigationResult.Parameters` with the parameters from the user-selected workflow (recommended or alternative), not stale RCA params
4. **Schema alignment**: LLM JSON schema, Go types, and MCP DTOs all include `parameters` on alternative workflows
5. **Backward compatibility**: Omitted parameters remain `nil` / omitted in JSON (no breaking change for existing consumers)

### 1.3 Success Metrics

- Unit test pass rate: 100% (`go test ./internal/kubernautagent/parser/... ./internal/kubernautagent/mcp/tools/...`)
- Unit-testable code coverage: >=80% on changed functions (`parseLLMFormat` alternatives loop, `extractDiscoveryResult`, `buildFinalResult`, `lookupDiscoveredParameters`)
- Integration test pass rate: 100% (`go test ./test/integration/kubernautagent/mcp/...`)
- Backward compatibility: 0 regressions — all existing tests pass without modification (except signature updates)
- Feature-specific: discover_workflows JSON contains `parameters` on recommended and alternatives when LLM provides them

---

## 2. References

### 2.1 Authority (governing documents)

- BR-LLM-024/026: LLM structured extraction must produce complete parameter sets
- BR-INTERACTIVE: Interactive MCP tool surface (kubernaut_select_workflow, discover_workflows)
- ADR-045 v1.2: Alternative workflows for audit and operator context
- Issue #1169: Parent feature issue
- Issue #1166: Parent KA MCP tracking issue

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Anti-Pattern Detection](../ANTI_PATTERN_DETECTION.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

### 3.1 Risk Register

- R1: Cross-cutting schema change across 5 representations of AlternativeWorkflow
  - Impact: Type inconsistency breaks serialization
  - Probability: Medium
  - Affected Tests: UT-KA-433-PRS-010, UT-KA-EDR-001/002/003
  - Mitigation: TDD RED phase ensures all representations are exercised before implementation

- R2: `buildFinalResult` signature change breaks 3 existing tests
  - Impact: Compile failure in test suite
  - Probability: High (certain)
  - Affected Tests: UT-KA-SW-BUILDFINAL-001 (3 It blocks)
  - Mitigation: Update existing tests to pass nil as third arg during GREEN phase

- R3: Prompt engineering risk — LLM may not produce parameters for alternatives
  - Impact: Empty parameters in production (feature not exercised)
  - Probability: Medium
  - Affected Tests: E2E (FP-MCP-005)
  - Mitigation: Mock-LLM produces deterministic JSON; production LLM guided by updated prompt

- R4: JSON round-trip fidelity — numbers deserialize as float64 (100-go-mistakes #77)
  - Impact: Type mismatch in parameter values
  - Probability: Low
  - Affected Tests: UT-KA-433-PRS-010
  - Mitigation: Tests use string parameter values; document float64 behavior

- R5: `extractDiscoveryResult` has ZERO existing tests (QE audit finding)
  - Impact: Parameter mapping bugs undetected
  - Probability: High
  - Affected Tests: UT-KA-EDR-001/002/003 (new)
  - Mitigation: Add dedicated unit tests in RED phase

- R6: Stale recommended params leak when user selects alternative (product audit finding)
  - Impact: Wrong parameters applied to workflow execution — correctness bug
  - Probability: High (current behavior)
  - Affected Tests: UT-KA-SW-BUILDFINAL-002b, UT-KA-SW-BUILDFINAL-002d
  - Mitigation: `buildFinalResult` discovery-aware merge replaces params based on selected workflow

- R7: No parameter validation against catalog schema before execution (security audit finding)
  - Impact: LLM-hallucinated keys pass through to workflow execution
  - Probability: Medium
  - Affected Tests: Out of scope (tracked separately)
  - Mitigation: WE `FilterDeclaredParameters` provides defense-in-depth when catalog declares parameter names

- R8: No dedicated audit event for select_workflow choice (security audit finding)
  - Impact: Forensics gap when operator picks non-recommended workflow
  - Probability: Low (informational)
  - Affected Tests: Out of scope
  - Mitigation: Existing session completion audit captures final InvestigationResult

### 3.2 Risk-to-Test Traceability

- R1 mitigated by: UT-KA-433-PRS-010, UT-KA-EDR-001/002/003, UT-KA-SW-BUILDFINAL-002a/b/c/d
- R2 mitigated by: UT-KA-SW-BUILDFINAL-001 (updated)
- R5 mitigated by: UT-KA-EDR-001/002/003
- R6 mitigated by: UT-KA-SW-BUILDFINAL-002b, UT-KA-SW-BUILDFINAL-002d

---

## 4. Scope

### 4.1 Features to be Tested

- **Parser alternative parameter extraction** (`internal/kubernautagent/parser/parser.go`): LLM JSON with `parameters` on `alternative_workflows` items is faithfully extracted into `katypes.AlternativeWorkflow.Parameters`
- **Discovery result parameter propagation** (`internal/kubernautagent/mcp/tools/investigate.go`): `extractDiscoveryResult` copies parameters from `InvestigationResult` onto `DiscoveredWorkflow` for both recommended and alternatives
- **Per-workflow parameter merge** (`internal/kubernautagent/mcp/tools/select_workflow.go`): `buildFinalResult` replaces `InvestigationResult.Parameters` with the parameters from the user-selected workflow in the discovery result
- **MCP types** (`internal/kubernautagent/mcp/interfaces.go`): `DiscoveredWorkflow.Parameters` serializes correctly in JSON
- **LLM schema** (`internal/kubernautagent/parser/schema.go`): `alternative_workflows` items schema includes `parameters`

### 4.2 Features Not to be Tested

- **CRD AlternativeWorkflow type** (`api/aianalysis/v1alpha1/`): CRD stores selected workflow parameters only; alternatives are informational
- **Ogen/OpenAPI client generation**: HTTP API path already carries parameters on `InvestigationResult`
- **AA controller response processor**: Maps from HTTP API response, not MCP discovery
- **Parameter validation against catalog schema**: Tracked as separate security hardening work
- **Mock-LLM scenario updates**: Separate concern; current mock-LLM does not produce alternative parameters
- **Audit event for select_workflow**: Tracked as separate observability work

### 4.3 Design Decisions

- `buildFinalResult` uses variadic `discovery` parameter (not required) to maintain backward compatibility with nil callers
- `lookupDiscoveredParameters` returns nil (not empty map) when workflow has no parameters, causing `omitempty` to omit the field in JSON
- Parameters are `map[string]interface{}` (not `map[string]string`) to match existing `InvestigationResult.Parameters` type and preserve LLM fidelity
- `DiscoveredWorkflow.Parameters` may duplicate `FullResult.Parameters` for the recommended workflow — this is intentional for MCP client convenience (AF does not need to cross-reference FullResult)

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (parseLLMFormat alternatives loop, extractDiscoveryResult, buildFinalResult, lookupDiscoveredParameters)
- **Integration**: >=80% of integration-testable code (discover -> select flow parameter assertions in IT-KA-DISC-001)
- **E2E**: Deferred — depends on mock-LLM producing alternative parameters (separate concern)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Parser extraction, discovery mapping, merge logic (fast, isolated)
- **Integration tests**: Full discover -> select flow with real session manager and HTTP completer

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "AF receives per-workflow parameters in discover_workflows JSON" (not "Parameters field is not nil")
- "Selecting an alternative replaces recommended params" (not "buildFinalResult returns a result")
- "Omitted parameters are absent from JSON" (not "Parameters is nil")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% threshold on changed functions
3. No regressions in existing test suites
4. discover_workflows JSON contains `parameters` on recommended and alternatives when LLM provides them
5. Selecting an alternative workflow produces correct per-workflow parameters in final InvestigationResult

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail
4. Stale recommended parameters appear when selecting an alternative

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken (code does not compile due to cross-cutting type changes)
- extractDiscoveryResult or buildFinalResult signature mismatch between export_test.go and production

**Resume testing when**:
- All types, parser, MCP interfaces, and export wrappers are aligned

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

- `pkg/kubernautagent/types/types.go`: `AlternativeWorkflow` struct (~5 lines)
- `internal/kubernautagent/parser/parser.go`: `llmAlternative` struct + `parseLLMFormat` alternatives loop (~10 lines)
- `internal/kubernautagent/parser/schema.go`: `investigationResultSchemaJSON` alternatives items (~3 lines)
- `internal/kubernautagent/mcp/interfaces.go`: `DiscoveredWorkflow` struct (~8 lines)
- `internal/kubernautagent/mcp/tools/investigate.go`: `extractDiscoveryResult` function (~30 lines)
- `internal/kubernautagent/mcp/tools/select_workflow.go`: `buildFinalResult` + `lookupDiscoveredParameters` (~25 lines)

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

- `internal/kubernautagent/mcp/tools/select_workflow.go`: `Handle` method (production caller of buildFinalResult) (~75 lines)

---

## 7. BR Coverage Matrix

- BR-LLM-024: Parser extracts complete parameter sets from LLM JSON (P0, Unit, UT-KA-433-PRS-010, Pending)
- BR-LLM-026: All workflows (recommended + alternatives) carry LLM-populated parameters (P0, Unit, UT-KA-EDR-001/002, Pending)
- BR-INTERACTIVE: discover_workflows response includes per-workflow parameters for AF preview (P0, Unit+IT, UT-KA-EDR-001, IT-KA-DISC-001, Pending)
- ADR-045 v1.2: Alternative workflows carry context including parameters (P1, Unit, UT-KA-EDR-002/003, Pending)
- BR-INTERACTIVE: select_workflow merges correct per-workflow parameters (P0, Unit, UT-KA-SW-BUILDFINAL-002a/b/c/d, Pending)

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`
- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `KA` (Kubernaut Agent)

### Tier 1: Unit Tests

**Testable code scope**: parser alternatives loop, extractDiscoveryResult, buildFinalResult, lookupDiscoveredParameters (>=80% coverage target)

- `UT-KA-433-PRS-010`: Parser extracts parameters from alternative_workflows items (BR-LLM-024, Pending)
- `UT-KA-EDR-001`: extractDiscoveryResult copies recommended workflow parameters (BR-LLM-026, Pending)
- `UT-KA-EDR-002`: extractDiscoveryResult copies alternative workflow parameters (BR-LLM-026, Pending)
- `UT-KA-EDR-003`: extractDiscoveryResult handles alternatives with nil parameters (ADR-045, Pending)
- `UT-KA-SW-BUILDFINAL-002a`: buildFinalResult uses recommended workflow parameters when selecting recommended (BR-INTERACTIVE, Pending)
- `UT-KA-SW-BUILDFINAL-002b`: buildFinalResult replaces parameters when selecting alternative (BR-INTERACTIVE, Pending)
- `UT-KA-SW-BUILDFINAL-002c`: buildFinalResult passes RCA params through when discovery is nil (BR-INTERACTIVE, Pending)
- `UT-KA-SW-BUILDFINAL-002d`: buildFinalResult clears params when alternative has nil parameters (BR-INTERACTIVE, Pending)

### Tier 2: Integration Tests

**Testable code scope**: Full discover -> select flow with parameter assertions

- `IT-KA-DISC-001` (existing, update): Assert `alternatives[].parameters` in discover_workflows response (BR-INTERACTIVE, Pending)

### Tier Skip Rationale

- **E2E**: Deferred — FP-MCP-005 exercises discover + select but mock-LLM does not produce alternative parameters. Mock-LLM scenario update is a separate concern tracked outside this issue.

---

## 9. Test Cases

### UT-KA-433-PRS-010: Parser extracts parameters from alternative_workflows

**BR**: BR-LLM-024
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/parser/adversarial_parser_test.go`

**Test Steps**:
1. **Given**: LLM JSON with `selected_workflow.parameters` and `alternative_workflows[0].parameters` and `alternative_workflows[1]` without parameters
2. **When**: `parser.Parse(content)` is called
3. **Then**: `result.AlternativeWorkflows[0].Parameters` contains the LLM-provided parameter map; `result.AlternativeWorkflows[1].Parameters` is nil; `result.Parameters` contains selected workflow parameters

**Acceptance Criteria**:
- **Behavior**: Parser faithfully extracts per-alternative parameters
- **Correctness**: Parameter keys and values match LLM JSON exactly
- **Accuracy**: Alternatives without parameters have nil Parameters (not empty map)

### UT-KA-EDR-001: extractDiscoveryResult copies recommended workflow parameters

**BR**: BR-LLM-026
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/mcp/tools/select_workflow_test.go` (or investigate_test.go)

**Test Steps**:
1. **Given**: InvestigationResult with WorkflowID, Parameters, and AlternativeWorkflows
2. **When**: extractDiscoveryResult is called
3. **Then**: Recommended DiscoveredWorkflow has Parameters matching InvestigationResult.Parameters

### UT-KA-SW-BUILDFINAL-002b: buildFinalResult replaces parameters when selecting alternative

**BR**: BR-INTERACTIVE
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/mcp/tools/select_workflow_test.go`

**Test Steps**:
1. **Given**: RCA with recommended params `{MEMORY_LIMIT_NEW: 512Mi}`, discovery with alternative `{REPLICA_COUNT: 5, MAX_SURGE: 2}`
2. **When**: buildFinalResult is called with the alternative's catalog workflow and the discovery
3. **Then**: Result.Parameters contains `{REPLICA_COUNT: 5, MAX_SURGE: 2}` and does NOT contain `MEMORY_LIMIT_NEW`

**Acceptance Criteria**:
- **Behavior**: Alternative parameters fully replace recommended parameters
- **Correctness**: No parameter key leakage from recommended workflow
- **Accuracy**: Result.Parameters exactly matches the alternative's parameters from discovery

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None for parser; mockSessionManager + mockHTTPCompleter for buildFinalResult context
- **Location**: `internal/kubernautagent/parser/`, `internal/kubernautagent/mcp/tools/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks (real session manager, real HTTP completer)
- **Infrastructure**: In-process server
- **Location**: `test/integration/kubernautagent/mcp/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

- None — PRs #1163 and #1168 are merged

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write all failing tests — parser, extractDiscoveryResult, buildFinalResult
2. **Phase 2 (GREEN)**: Minimal implementation — types, parser, schema, MCP types, extractDiscoveryResult, buildFinalResult, export_test, production caller, prompt
3. **Phase 3 (REFACTOR)**: Code cleanup, 100-go-mistakes validation, slice pre-allocation, nil guards
4. **Phase 4 (WIRING VERIFICATION)**: Integration test update (IT-KA-DISC-001)
5. **Phase 5 (COMMIT)**: Logical commit groups, push, PR creation

---

## 12. Test Deliverables

- This test plan: `docs/tests/1169/TEST_PLAN.md`
- Unit test suite: `internal/kubernautagent/parser/adversarial_parser_test.go`, `internal/kubernautagent/mcp/tools/select_workflow_test.go`
- Integration test update: `test/integration/kubernautagent/mcp/discovery_flow_test.go`
- Coverage report: CI artifact

---

## 13. Execution

```bash
# Unit tests — parser
go test ./internal/kubernautagent/parser/... -ginkgo.focus="UT-KA-433-PRS-010" -ginkgo.v

# Unit tests — MCP tools
go test ./internal/kubernautagent/mcp/tools/... -ginkgo.focus="UT-KA-EDR|UT-KA-SW-BUILDFINAL-002" -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/mcp/... -ginkgo.focus="IT-KA-DISC-001" -ginkgo.v

# Coverage
go test ./internal/kubernautagent/mcp/tools/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Wiring Verification (TDD Phase 4)

- `lookupDiscoveredParameters`: Entry=`buildFinalResult` / Exit=`InvestigationResult.Parameters` / Wiring IT=IT-KA-DISC-001 / Status=Pending
- `extractDiscoveryResult` parameters: Entry=`handleDiscoverWorkflows` / Exit=MCP JSON response / Wiring IT=IT-KA-DISC-001 / Status=Pending
- `buildFinalResult` discovery merge: Entry=`SelectWorkflowTool.Handle` / Exit=HTTP session completion / Wiring IT=IT-KA-DISC-001 / Status=Pending

---

## 15. Existing Tests Requiring Updates

- `UT-KA-SW-BUILDFINAL-001` (lines 490, 510, 519): Currently calls `BuildFinalResult(rca, workflow)` with 2 args. Must add `nil` as third arg: `BuildFinalResult(rca, workflow, nil)`. Reason: signature change to accept optional discovery parameter.
- `export_test.go` (line 25): Wrapper must match new 3-arg signature of `buildFinalResult`. Reason: bridge between internal function and external test package.

---

## 16. GA Readiness Audit Findings

### QE Findings (incorporated into test design)

- extractDiscoveryResult had ZERO tests — addressed by UT-KA-EDR-001/002/003
- No IT/E2E asserts discover_workflows response content — addressed by IT-KA-DISC-001 update
- Mock-LLM does not produce alternative parameters — deferred (separate concern)

### Product Findings (incorporated into design decisions)

- No explicit BR for this feature — mapped to BR-LLM-024/026 + BR-INTERACTIVE
- buildFinalResult stale params bug — addressed by UT-KA-SW-BUILDFINAL-002b/d
- discover_workflows JSON schema undocumented — out of scope (AF team notified via issue)

### Security Findings (tracked separately)

- Parameter validation against catalog schema — out of scope (defense-in-depth via WE FilterDeclaredParameters)
- Parameter redaction — out of scope (no secrets expected in workflow parameters by design)
- Audit event for select_workflow — out of scope (existing session completion audit sufficient for v1.5)

### Test Quality Findings (incorporated into test improvements)

- Tests must use different RCA vs discovery params in recommended case (prevents false positive)
- Tests must include BR references in Describe blocks
- Tests must cover empty map `{}` vs nil parameter behavior

---

## 17. GA Readiness Audit — Additional Dimensions

### Documentation Readiness

- **CRITICAL**: No `docs/mcp/` directory — no MCP tool response format documentation exists
- **HIGH**: ADR-045 `AlternativeWorkflow` has no `parameters` — needs clarification that MCP DTO diverges
- **HIGH**: BR-INTERACTIVE does not define `discover_workflows` behavior or per-workflow parameters
- **HIGH**: User guide (`docs/user-guide/interactive-mode.md`) has no MCP tool list or discovery section
- **MEDIUM**: Phase 3 prompt not documented under `docs/services/kubernaut-agent/`
- **Action**: Create minimal MCP contract doc; update BR-INTERACTIVE; add ADR note

### Observability Readiness

- **MEDIUM**: `handleDiscoverWorkflows` has no structured success logging (workflow count, parameter presence)
- **MEDIUM**: No Prometheus counters for discover_workflows outcomes; SLO histogram merges all actions
- **MEDIUM**: No OpenTelemetry spans in MCP tool handlers
- **Action**: Add structured log on discovery success; consider action-level metrics (deferred to separate issue)

### Performance Readiness

- **HIGH**: Map aliasing — `buildFinalResult` does `result := *rca` (shallow copy). `Parameters` map is shared with original. Mutating result.Parameters corrupts RCA. `lookupDiscoveredParameters` MUST assign new map, not mutate.
- **MEDIUM**: Payload size unbounded if LLM produces rich parameter maps
- **LOW**: extractDiscoveryResult and buildFinalResult are cold path (single call each, dominated by LLM latency)
- **Action**: Clone parameter map in buildFinalResult; add mutation test

### Reliability/Resilience Readiness

- **HIGH**: `handleComplete`/`handleCancel` do NOT acquire per-rrID session mutex (pre-existing gap, not caused by #1169)
- **MEDIUM**: `json.Unmarshal` into `map[string]interface{}` fails if LLM produces `"parameters": "not-an-object"` — parser returns error (graceful)
- **LOW**: nil AlternativeWorkflows on InvestigationResult — range over nil slice is safe in Go
- **Action**: Add adversarial test for malformed parameters type; document mutex gap for future fix

### Compliance/Audit Readiness

- **HIGH**: No dedicated audit event for `select_workflow` — auditors cannot trace who selected which workflow from which discovery set
- **HIGH**: `ResultToAuditJSON` omits per-alternative parameters — forensic gap
- **MEDIUM**: Discovery result is ephemeral (session-scoped, no durable record) — long-term evidence depends on downstream InvestigationResult
- **MEDIUM**: `select_workflow` does not call `emitInteractiveCompleted` — audit gap for "workflow selected" exit path
- **Action**: Update `ResultToAuditJSON` alt serialization to include parameters; track audit event as follow-up

### Deployment/Release Readiness

- **MEDIUM**: If OpenAPI changes, Ogen regen required (`make generate-agentclient`)
- **LOW**: No DB migration needed (JSON audit blobs are schema-flexible)
- **LOW**: No CRD change needed (MCP-only surface)
- **LOW**: No new Go dependencies
- **Action**: Add CHANGELOG entry; no Helm changes needed

---

## 18. Changelog

- v1.0 (2026-05-17): Initial test plan with QE, product, UX, security, and test quality audit findings
- v1.1 (2026-05-17): Added documentation, observability, performance, reliability, compliance, and deployment audits. Removed backward compatibility requirement per user direction. Added map aliasing risk (R6) and audit serialization gap (R7).
