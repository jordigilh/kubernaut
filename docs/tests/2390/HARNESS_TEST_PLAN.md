# Test Plan: A2A Transcript Scenario Harness

**Test Plan Identifier**: TP-2390-HARNESS-v1
**Feature**: Explicit transcript-driven Mock LLM scenarios for multi-turn A2A E2E tests
**Version**: 1.0
**Created**: 2026-09-12
**Status**: Active
**Branch**: `fix/issue-2390-workflow-snapshot`

## 1. Purpose and Objectives

This plan verifies that multi-turn A2A E2E tests exercise the declared protocol phases rather
than whichever keyword scenario wins a substring or registration-order collision.

1. Transcript YAML loads valid ordered user/tool steps.
2. Exact last-user matching selects only the intended transcript step.
3. `$from_tool` arguments resolve per request without state leakage.
4. A tool result does not cause the same transcript step to emit an infinite repeat.
5. Invalid transcript entries fail configuration loading visibly.
6. Issue #2390 starts with a fresh interactive investigation and completes discover, select, and
   watch as separate user-driven turns.

## 2. Authority

- `BR-TESTING-001`: Golden Transcript Mock LLM Fidelity
- `BR-INTERACTIVE-001`: Interactive Investigation Sessions
- `BR-INTERACTIVE-004`: Dynamic Takeover of Autonomous Investigations
- `BR-INTERACTIVE-011`: AF Remediation Mode Selection and Full Interactive Remediation
- `DD-TEST-016`: Explicit Transcript Scenarios for A2A E2E Tests
- `DD-AA-KA-001`: AgentSession CRD dispatch and fresh interactive ordering
- `docs/tests/2390/IMPLEMENTATION_PLAN.md`: Issue #2390 execution metadata outcome

## 3. Risks and Mitigations

| ID | Risk | Mitigation | Tests |
|---|---|---|---|
| R1 | A keyword scenario shadows the intended A2A step | Exact transcript matching has higher priority than keyword scenarios | `UT-ML-2410-002`, `E2E-FP-2390-001` |
| R2 | A later request cannot resolve the RR ID | Preserve `$from_tool` resolution against prior tool results | `UT-ML-2410-003`, `E2E-FP-2390-001` |
| R3 | A tool call repeats forever after its result | Reuse the existing last-tool-result repeat guard | `UT-ML-2410-004` |
| R4 | Malformed configuration silently falls through to generic text | Validate required transcript fields during load | `UT-ML-2410-005` |
| R5 | Autonomous processing races interactive setup | Start with fresh `kubernaut_investigate` resource arguments and `interaction_mode: interactive` | `E2E-FP-2390-001` |

## 4. Test Scenarios

| Test ID | Tier | Requirement | Scenario | Expected Result |
|---|---|---|---|---|
| `UT-ML-2410-001` | Unit | `BR-TESTING-001` | Parse ordered transcript steps | All fields and order are preserved |
| `UT-ML-2410-002` | Unit | `BR-TESTING-001` | Exact match ignores accumulated tool text and substring overlap | Only the exact current user turn matches |
| `UT-ML-2410-003` | Unit | `BR-TESTING-001` | Resolve an RR ID in a later transcript step | A fresh argument map contains the resolved ID |
| `UT-ML-2410-004` | Unit | `BR-TESTING-001` | Re-evaluate the same step after its tool result | No duplicate tool call is emitted |
| `UT-ML-2410-005` | Unit | `BR-TESTING-001` | Load missing user message or tool name | Configuration load returns a descriptive error |
| `E2E-FP-2390-001` | E2E | `BR-INTERACTIVE-001`, `BR-INTERACTIVE-004`, `BR-INTERACTIVE-011` | Run GitOps interactive selection through AF | Fresh interactive RR reaches selected GitOps WorkflowExecution and preserves metadata |

## 5. Pass Criteria

- All `UT-ML-2410-*` tests pass.
- `E2E-FP-2390-001` passes in FullPipeline CI.
- Existing keyword, golden replay, and Mock LLM tests have zero regressions.
- `go build ./...` and `golangci-lint run --timeout=5m` pass.
- No skipped or pending tests are introduced.
