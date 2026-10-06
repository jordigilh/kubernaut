# Test Plan: session_active Status Visibility

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1922-v1
**Feature**: `session_active` is detected before readiness waits and delivers visible status guidance without fabricating an RCA
**Version**: 1.0
**Created**: 2026-08-04
**Author**: AI Agent
**Status**: Implemented (UT/IT verified locally; E2E written and passes lint/build but requires a live E2E cluster — see Section 3.3)
**Branch**: `fix/1922-session-active-fallback`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for GitHub issue #1922: when KA rejects a `kubernaut_investigate` call with `session_active` (a second driver contending for an already-driven investigation), AF emits visible status guidance through the existing A2A status channel. AF does not synthesize an RCA, causal chain, or confidence value for a caller that has no investigation findings of its own. Genuine KA RCA results continue to use the existing `investigation_summary` artifact contract.

### 1.2 Objectives

1. **Visible guidance**: a known interactive lease holder is rejected before readiness waits and the caller receives a status update explaining the active investigation.
2. **Truthfulness**: status-only outcomes carry grounded driver/severity information but no synthetic findings or confidence value (AU-3).
3. **Genuine RCA contract**: real KA RCA results retain the existing `investigation_summary` field names and shape (SI-10).
4. **Wiring proof**: both the status preflight and the StartInvestigation race path use the status-only result helper.
5. **Journey proof**: a real rejected concurrent driver (`session_active`, BR-INTERACTIVE-004) receives visible status guidance end-to-end.
6. **Latency protection**: a known interactive lease holder is rejected before the readiness barriers; autonomous status and probe failures retain the normal takeover/race-safe path.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "TestTools" -ginkgo.focus="WIRE-SESSION|1922"` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-WIRE-SESSION"` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend GINKGO_FOCUS="1922"` |
| Regression pass rate (pre-existing) | 100% | `WIRE-SESSION-001/002`, `UT-AF-1407-*`, `UT-AF-1396-*`, `E2E-AF-1408-001` unchanged |

---

## 2. References

### 2.1 Authority

- Issue #1922: concurrent session_active responses were not visible promptly
- `BR-INTERACTIVE-004` (Dynamic Takeover of Autonomous Investigations), success criterion 7: "Single-driver guarantee via K8s Lease (concurrent drivers rejected)" — `docs/requirements/BR-INTERACTIVE.md:74-93`
- `BR-INTERACTIVE-003` (Audit Attribution for Interactive Actions), success criterion 4: "Full conversation reconstruction possible via DS query" — `docs/requirements/BR-INTERACTIVE.md:54-71`
- Precedent: Issue #1407/#1408 (Progressive RCA Emission / Structured Artifact Contract), `docs/tests/1407/TEST_PLAN.md` — same functions, sibling defect

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| AU-3 | Content of audit records | Status-only guidance contains grounded ownership/severity information and no fabricated RCA findings or confidence | UT-AF-WIRE-SESSION-003 |
| SI-10 | Information input validation / data integrity | Genuine RCA results retain the `investigation_summary` field names and shape used by `present_decision` | UT-AF-WIRE-2247-001 |
| SI-4 | Audit classification | `metadata.type=status` identifies the rejected-driver guidance on the existing A2A status channel | UT-AF-WIRE-SESSION-003 |
| AC-4 (via BR-INTERACTIVE-004) | Information flow enforcement (single-driver rejection) | The rejected caller's `session_active` response remains observable through the audit-traceable status channel | E2E-AF-1922-001 |

---

## 3. Test Scenarios

### 3.1 Unit Tests (status-only no-RCA behavior)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-WIRE-SESSION-003 | `HandleInvestigationMCPWithRegistry` with triaged severity + KA `session_active` error | Returns `session_active`, emits `metadata.type=status` guidance, and returns no RCA object | Implemented — PASS |
| UT-AF-WIRE-SESSION-004 | `HandleInvestigationMCPWithRegistry` completes with no KA RCA after severity triage | Emits severity-only status guidance, returns no RCA object, and emits no synthetic artifact | Implemented — PASS |

### 3.2 Integration Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| IT-AF-1922-004 | `HandleInvestigationMCPWithRegistry` sees KA `action=status` with `mode=interactive` | Returns `session_active`, emits status guidance, and does not call `action=start` or enter readiness waits | Implemented — PASS |
| UT-AF-1922-005 | Status preflight returns `mode=autonomous` | Normal takeover/start path remains authoritative | Implemented — PASS |

### 3.3 E2E Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| E2E-AF-1922-001 | Two concurrent `kubernaut_investigate` calls (real KA) against the same target; second caller receives `session_active` | Second caller's SSE stream contains a status-update with `metadata.type=status` and visible active-investigation guidance; no synthetic RCA artifact is required | Implemented — written, builds and lints clean; **not executed** in this session (no live Kind/E2E cluster available locally — podman machine not running). Requires `make test-e2e-apifrontend GINKGO_FOCUS="1922"` against a deployed E2E environment (CI or a locally provisioned cluster). |

---

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|-----------|------------------------|-----------------------|---------|
| `sessionActiveInvestigationResult` | `HandleInvestigationMCPWithRegistry` → known-active preflight / StartInvestigation race | `pkg/apifrontend/tools/ka_investigate_mcp.go` | UT-AF-WIRE-SESSION-003, IT-AF-1922-004, E2E-AF-1922-001 |
| `noRCAStatusText` | `runBlockingInvestigation` when KA returns no RCA | `pkg/apifrontend/tools/ka_investigate_mcp.go` | UT-AF-WIRE-SESSION-004 |
| `activeInteractiveSession` preflight | `HandleInvestigationMCPWithRegistry` → KA `action=status` before readiness | `pkg/apifrontend/tools/ka_investigate_mcp.go` | IT-AF-1922-004 |
| `emitInvestigationSummaryArtifact` | `runBlockingInvestigation` after genuine KA RCA completion | `pkg/apifrontend/tools/ka_investigate_mcp.go` / `ka_investigate_bridge.go` | UT-AF-WIRE-2247-001 |

---

## 5. Execution

```bash
go test ./pkg/apifrontend/tools/... -run "TestTools" -ginkgo.focus="WIRE-SESSION|1922" -v -count=1
go test ./pkg/apifrontend/tools/... -run "UT-AF-WIRE-SESSION" -v -count=1
go test ./pkg/apifrontend/tools/... -run "IT-AF-1922-004" -v -count=1
make test-e2e-apifrontend GINKGO_FOCUS="1922"
```
