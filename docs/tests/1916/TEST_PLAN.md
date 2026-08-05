# Test Plan: Interactive-Investigation Status Messages Leak Internal Acronyms

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1916
**Feature**: Three hardcoded `launcher.EmitStatusSafe` status/warning strings in `HandleInvestigationMCPWithRegistry` leak internal service acronyms (`KA`, `AA`) and CRD name (`IS CRD`) to the console user, bypassing the harness's own documented no-internal-names constraint.
**Version**: 1.0
**Created**: 2026-08-04
**Author**: AI Agent
**Status**: Implemented (UT verified locally; full `apifrontend` unit suite + `-race` + lint clean)
**Branch**: `fix/1916-af-status-message-acronym-leak`

---

## 1. Introduction

### 1.1 Purpose

GitHub issue #1916 reports that three Go-harness literals in
`pkg/apifrontend/tools/ka_investigate_mcp.go` are emitted verbatim to the
console chat UI as status lines during an interactive investigation:

- `"Investigation session ready, connecting to KA..."` (line 327)
- `"Interactive session acknowledged by AA, starting investigation..."` (line 340)
- `"Warning: IS CRD creation failed (%s), investigation continues"` (line 406)

`KA` (Kubernaut Agent), `AA` (the AIAnalysis controller/reconciler), and
`IS CRD` (the InvestigationSession custom resource) are internal service and
CRD names that mean nothing to a console user, and are explicitly forbidden by
`pkg/apifrontend/agent/prompt.txt`'s `## Behavioral Constraints` item 1:
*"Never reference internal system names (RemediationRequest, AIAnalysis,
SignalProcessing, KA, CRD, etcd) in responses."* That constraint governs only
LLM-generated text; these three strings are Go harness literals and bypass it
entirely.

### 1.2 Objectives

1. **No internal-name leakage**: all three status/warning strings emitted by
   `HandleInvestigationMCPWithRegistry` are reworded to drop the internal
   service/CRD names while preserving their informational intent.
2. **Behavioral proof, not string inspection**: tests exercise the real
   production entry point (`HandleInvestigationMCPWithRegistry`, wired to the
   `kubernaut_investigate` MCP tool) via the existing `launcher.WithEventBridge`
   test harness, and assert on the text actually emitted on the A2A event
   channel — not on the source literal directly.
3. **No regression**: the underlying redacted-error content in the line-406
   warning (via `security.RedactError`) is unchanged; only the wrapper text
   changes.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-1916" -v -count=1` |
| Regression pass rate (pre-existing) | 100% | `ka_investigate_wiring_test.go` / `ka_investigate_mcp_test.go` suites unchanged (`make test-unit-apifrontend`) |
| Build/Lint | Clean | `go build ./...`, `golangci-lint run` |

---

## 2. References

### 2.1 Authority

- Issue #1916: AF interactive-investigation status messages leak internal
  acronyms (KA, AA, IS CRD) to the console user.
- `pkg/apifrontend/agent/prompt.txt`, `## Behavioral Constraints` item 1 — the
  product's own documented no-internal-names constraint (enforced today only
  for LLM output, via `UT-AF-131-004` in `pkg/apifrontend/agent/prompt_test.go`).
- No standalone `BR-XXX` requirement exists for "console status text must not
  leak internal names" (`BR-INTERACTIVE.md`, `BR-INTERACTIVE-010.md` checked).
  Following the established convention for pure bug fixes in this repo
  (issues #1922, #1440, #1928), this fix is anchored to **Issue #1916** and
  the FedRAMP control below rather than an unbacked BR-XXX.

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| SI-11 (Error Handling) | Error/status messages do not leak information that could reveal system architecture to an unauthorized viewer | The three console status/warning strings emitted during interactive investigation must not reference internal service acronyms (`KA`, `AA`) or CRD names (`IS CRD`) | UT-AF-1916-001, UT-AF-1916-002, UT-AF-1916-003 |

This is the same control family already used in this codebase for this exact
class of "message must not leak X" bug (`docs/tests/1293/TEST_PLAN_INTERACTIVE_INVESTIGATION.md`
line 91, `pkg/apifrontend/session/deferred_crd_test.go:538`,
`pkg/datastorage/server/response/rfc7807.go:90`).

---

## 3. Scope

### 3.1 In Scope

| Area | Description |
|------|--------------|
| `HandleInvestigationMCPWithRegistry` status text | Reword the three literals at lines 327, 340, 406 of `pkg/apifrontend/tools/ka_investigate_mcp.go` |

### 3.2 Out of Scope

- Any change to `security.RedactError` or the redaction logic applied to the
  underlying hook error (unaffected — only the wrapper text changes).
- Any change to `prompt.txt`'s LLM-facing constraint (already correct; this
  fix closes the harness-side gap it does not cover).
- Any new wiring, component, or CRD — this is a literal-text-only fix inside
  an already-wired, already-integration-tested function.

---

## 4. Test Scenarios (Unit — behavioral, via production entry point)

All three scenarios call `tools.HandleInvestigationMCPWithRegistry` directly
(the same function registered as the `kubernaut_investigate` MCP tool) with
`launcher.WithEventBridge` installed, and assert on the `*a2a.TaskStatusUpdateEvent`
text actually emitted — proving the fix through the real dispatch path rather
than a static string check.

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
`HandleInvestigationMCPWithRegistry` function. Its production entry point
(`kubernaut_investigate` MCP tool registration) and wiring are already proven
by pre-existing tests (`WIRE-SESSION-001/002/003`, `WIRE-C01`, `WIRE-W04` in
`ka_investigate_wiring_test.go`), which are unaffected by this change (per
2.1 above, they carry no direct dependency on the three literals under fix).

---

## 6. TDD Execution Phases

| Phase | Type | Scope | Tests |
|-------|------|-------|-------|
| 1 | RED | Add 3 failing tests asserting the new (not-yet-implemented) friendly wording | UT-AF-1916-001..003 |
| 2 | GREEN | Reword the 3 literals in `ka_investigate_mcp.go` | All Phase 1 tests pass; no regression in existing `tools_test` suite |
| 3 | REFACTOR | Extract literals to named constants; re-audit new wording for leaks; lint/build | All tests remain green |

---

## 7. Execution

```bash
go build ./...
go test ./pkg/apifrontend/tools/... -run "UT-AF-1916" -v -count=1
make test-unit-apifrontend
golangci-lint run --timeout=5m ./pkg/apifrontend/...
```
