# Test Plan: Interactive-Investigation Status Messages Leak Internal Acronyms (main port)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1937
**Feature**: `main` port of #1916's fix — three hardcoded `launcher.EmitStatusSafe` status/warning strings in the interactive-investigation flow leak internal service acronyms (`KA`, `AA`) and CRD name (`IS CRD`) to the console user.
**Version**: 1.0
**Created**: 2026-08-04
**Author**: AI Agent
**Status**: Implemented
**Branch**: `backport/1916-af-status-message-acronym-leak-main`

---

## 1. Introduction

### 1.1 Purpose

This is the `main` port of the fix for GitHub issue #1916, originally found
and fixed against `release/v1.5` (#1932). This is tracked by #1937 (reverse
of the usual v1.5-first sequencing note only in the sense that this bug was
found on `release/v1.5` first — the port direction is the same as
#1912→#1913, #1915→#1917, #1918→#1924: implement on `release/v1.5`, then
port to `main`).

The bug predates neither branch — the same three literals exist unchanged on
both `release/v1.5` and `main`, carried forward by `main`'s later refactor of
`HandleInvestigationMCPWithRegistry` into smaller helper functions
(`resolveInvestigationRR`, `signalInteractiveSession`,
`awaitInvestigationReady`, `startKAInvestigation`,
`finalizeInvestigationStart`) taking a `*InvestigateConfig` struct, instead
of `release/v1.5`'s single function with 13 positional parameters:

- `awaitInvestigationReady` (main, `ka_investigate_mcp.go:474`):
  `"Investigation session ready, connecting to KA..."`
- `awaitInvestigationReady` (main, `ka_investigate_mcp.go:487`):
  `"Interactive session acknowledged by AA, starting investigation..."`
- `finalizeInvestigationStart` (main, `ka_investigate_mcp.go:568`):
  `"Warning: IS CRD creation failed (%s), investigation continues"`

Same behavioral constraint applies: `pkg/apifrontend/agent/prompt.txt`'s
`## Behavioral Constraints` item 1 forbids internal names in LLM-generated
text; these are Go harness literals and bypass it entirely, identically to
`release/v1.5`.

### 1.2 Objectives

Identical to #1916's objectives on `release/v1.5` (`docs/tests/1916/TEST_PLAN.md`),
re-verified against `main`'s refactored code shape:

1. **No internal-name leakage**: all three status/warning strings emitted
   during the await-and-finalize sequence are reworded identically to the
   `release/v1.5` fix.
2. **Behavioral proof, not string inspection**: tests exercise the real
   production entry point (`HandleInvestigationMCPWithRegistry`, still wired
   to the `kubernaut_investigate` MCP tool on `main`) via
   `launcher.WithEventBridge`, asserting on the text actually emitted.
3. **No regression**: `security.RedactError`'s redaction of the underlying
   hook error is unchanged; only the wrapper text changes.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-1916" -v -count=1` |
| Regression pass rate (pre-existing) | 100% | `ka_investigate_wiring_test.go` / `ka_investigate_mcp_test.go` suites unchanged (`make test-unit-apifrontend`) |
| Build/Lint | Clean | `go build ./...`, `golangci-lint run` |

---

## 2. References

### 2.1 Authority

- Issue #1916: original bug report (`release/v1.5`).
- PR #1932: `release/v1.5` implementation.
- Issue #1937: `main` port tracking issue and divergence analysis (this plan).
- `pkg/apifrontend/agent/prompt.txt`, `## Behavioral Constraints` item 1.

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| SI-11 (Error Handling) | Error/status messages do not leak information that could reveal system architecture to an unauthorized viewer | The three console status/warning strings emitted during interactive investigation must not reference internal service acronyms (`KA`, `AA`) or CRD names (`IS CRD`) | UT-AF-1916-001, UT-AF-1916-002, UT-AF-1916-003 |

---

## 3. Scope

### 3.1 In Scope

| Area | Description |
|------|--------------|
| `awaitInvestigationReady` status text | Reword the two literals at `pkg/apifrontend/tools/ka_investigate_mcp.go:474,487` (main line numbers) |
| `finalizeInvestigationStart` warning text | Reword the literal at `pkg/apifrontend/tools/ka_investigate_mcp.go:568` (main line numbers) |

### 3.2 Out of Scope

- Any change to `security.RedactError` or the redaction logic.
- Any change to `prompt.txt`'s LLM-facing constraint.
- Any new wiring, component, or CRD.
- `finalizeInvestigationStart`'s server-side `logr...Error(hookErr, "IS CRD creation failed after investigate", ...)` log line — this is a structured log, not console-emitted user-facing text, so it is not in scope for this console-leak fix.

---

## 4. Test Scenarios (Unit — behavioral, via production entry point)

All three scenarios call `tools.HandleInvestigationMCPWithRegistry` directly
with `launcher.WithEventBridge` installed, and assert on the
`*a2a.TaskStatusUpdateEvent` text actually emitted. Adapted from
`release/v1.5`'s test to `main`'s `&tools.InvestigateConfig{...}` call
signature (`ctx, cfg *InvestigateConfig, args, blocking, username`) instead
of 13 positional parameters, following the existing
`UT-AF-WIRE-SESSION-001/002` pattern already on `main`
(`ka_investigate_wiring_test.go`). `newTypedIS` also drops the namespace
parameter on `main` (hardcoded to `"kubernaut-system"` internally) —
`newTypedAIAnalysisWithSession`, `bridgeQueue`, and
`launcher.WithEventBridge` port unchanged.

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| UT-AF-1916-001 | SI-11 | "Session ready" status (triggered when `HandleAwaitSession` finds a ready KA session for the RR via a pre-populated `AIAnalysis.Status.KASession.ID`) omits the `KA` acronym | Emitted status text does not contain `"KA"`; matches the new friendly wording | Implemented — PASS |
| UT-AF-1916-002 | SI-11 | "Session acknowledged" status (triggered when `AwaitISPhaseActive` finds an `InvestigationSession` CRD with `Status.Phase == Active` for the RR) omits the `AA` acronym | Emitted status text does not contain `"AA"`; matches the new friendly wording | Implemented — PASS |
| UT-AF-1916-003 | SI-11 | "Session tracking failed" warning (triggered when the `onStarted` `SessionStartedHook` returns an error) omits the `IS CRD` reference while still surfacing the redacted underlying error | Emitted warning text does not contain `"IS CRD"`; still contains the redacted hook error substring | Implemented — PASS |

**Test file**: `pkg/apifrontend/tools/status_message_leak_1916_test.go`

---

## 5. Wiring Manifest

Not applicable — no new component, handler, or callback is introduced. All
three fixes are literal-string edits inside the existing, already-wired
`awaitInvestigationReady`/`finalizeInvestigationStart` helpers, called from
`HandleInvestigationMCPWithRegistry` (production entry point:
`kubernaut_investigate` MCP tool registration, unchanged by this fix). Their
wiring is already proven by pre-existing tests
(`UT-AF-WIRE-SESSION-001/002/003` in `ka_investigate_wiring_test.go`), which
are unaffected by this change.

---

## 6. TDD Execution Phases

| Phase | Type | Scope | Tests |
|-------|------|-------|-------|
| 1 | RED | Port the 3 tests, adapted to `main`'s `&InvestigateConfig{}` call signature and `newTypedIS` arity; confirm failure against unfixed `main` code | UT-AF-1916-001..003 |
| 2 | GREEN | Reword the 3 literals inside `awaitInvestigationReady`/`finalizeInvestigationStart` in `ka_investigate_mcp.go` | All Phase 1 tests pass; no regression in existing `tools_test` suite |
| 3 | REFACTOR | Re-audit new wording for leaks; lint/build/race | All tests remain green |

---

## 7. Execution

```bash
go build ./...
go test ./pkg/apifrontend/tools/... -run "UT-AF-1916" -v -count=1
make test-unit-apifrontend
golangci-lint run --timeout=5m ./pkg/apifrontend/...
```
