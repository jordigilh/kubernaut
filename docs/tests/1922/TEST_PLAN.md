# Test Plan: session_active Fallback RCA Card Content Completeness

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1922-v1
**Feature**: `investigation_summary` fallback artifact carries `causal_chain`/`tool_calls_count` so the Console's RCA card renders on `session_active`
**Version**: 1.0
**Created**: 2026-08-04
**Author**: AI Agent
**Status**: Implemented (UT/IT verified locally; E2E written and passes lint/build but requires a live E2E cluster — see Section 3.3)
**Branch**: TBD

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for GitHub issue #1922: when KA rejects a `kubernaut_investigate` call with `session_active` (a second driver contending for an already-driven investigation), AF emits a fallback `investigation_summary` artifact built from severity-triage data alone. That artifact's `rca` object omits `causal_chain` and `tool_calls_count`, so the Console's `hasRCAData` render guard (`AgentBubble.tsx`) never shows the RCA card — the rejected user sees no diagnostic feedback at all, even though AF has honest severity/summary data to show.

### 1.2 Objectives

1. **Content completeness**: the fallback `investigation_summary` artifact's `rca` object includes a non-empty `causal_chain` (and always includes `tool_calls_count`/`llm_turns`, even when zero) so the Console's render guard is satisfied.
2. **Truthfulness**: the synthetic placeholder never fabricates findings — it states investigation status honestly (AU-3).
3. **Shape consistency**: the fallback artifact uses the exact same JSON field names as the final `present_decision` artifact (`RCAData`), so Console parsing logic does not need a fallback-specific branch (SI-10).
4. **Wiring proof**: both callers of the shared helper (`session_active` and KA-produced-no-events fallback) route through the fix.
5. **Journey proof**: a real rejected concurrent driver (`session_active`, BR-INTERACTIVE-004) actually sees a rendered RCA card end-to-end.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-1922"` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-WIRE-SESSION"` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend GINKGO_FOCUS="1922"` |
| Regression pass rate (pre-existing) | 100% | `WIRE-SESSION-001/002`, `UT-AF-1407-*`, `UT-AF-1396-*`, `E2E-AF-1408-001` unchanged |

---

## 2. References

### 2.1 Authority

- Issue #1922: session_active fallback RCA card renders blank
- `BR-INTERACTIVE-004` (Dynamic Takeover of Autonomous Investigations), success criterion 7: "Single-driver guarantee via K8s Lease (concurrent drivers rejected)" — `docs/requirements/BR-INTERACTIVE.md:74-93`
- `BR-INTERACTIVE-003` (Audit Attribution for Interactive Actions), success criterion 4: "Full conversation reconstruction possible via DS query" — `docs/requirements/BR-INTERACTIVE.md:54-71`
- Precedent: Issue #1407/#1408 (Progressive RCA Emission / Structured Artifact Contract), `docs/tests/1407/TEST_PLAN.md` — same functions, sibling defect

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| AU-3 | Content of audit records | Fallback artifact's `causal_chain`/`summary` truthfully reflect investigation status, never fabricate findings | UT-AF-1922-001 |
| SI-10 | Information input validation / data integrity | `investigation_summary` schema self-identification stays consistent between fallback and final (`present_decision`) artifacts — identical field names/shapes | UT-AF-1922-002 |
| SI-4 | Audit classification | `metadata.type=decision` unchanged; fallback remains classified identically to the final decision artifact | UT-AF-WIRE-SESSION-003 |
| AC-4 (via BR-INTERACTIVE-004) | Information flow enforcement (single-driver rejection) | The rejected caller's `session_active` response still carries a renderable, honest status via the same audit-traceable artifact channel | E2E-AF-1922-001 |

---

## 3. Test Scenarios

### 3.1 Unit Tests (fallback artifact content)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-1922-001 | `EmitFallbackInvestigationArtifact` with empty `CausalChain` | Emitted `rca.causal_chain` is a non-empty, truthful placeholder (no fabricated findings) | Implemented — PASS |
| UT-AF-1922-002 | `EmitFallbackInvestigationArtifact` with real `CausalChain`/`TotalToolCalls` | Real values pass through unchanged; `tool_calls_count`/`llm_turns` keys always present (even zero) | Implemented — PASS |

### 3.2 Integration Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-WIRE-SESSION-003 | `HandleInvestigationMCPWithRegistry` with triaged severity + KA `session_active` error | Emitted `TaskArtifactUpdateEvent` (schema=`investigation_summary`) has non-empty `rca.causal_chain` | Implemented — PASS |

### 3.3 E2E Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| E2E-AF-1922-001 | Two concurrent `kubernaut_investigate` calls (real KA) against the same target; second caller receives `session_active` | Second caller's SSE stream contains an `investigation_summary` artifact-update with non-empty `rca.causal_chain` | Implemented — written, builds and lints clean; **not executed** in this session (no live Kind/E2E cluster available locally — podman machine not running). Requires `make test-e2e-apifrontend GINKGO_FOCUS="1922"` against a deployed E2E environment (CI or a locally provisioned cluster). |

---

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|-----------|------------------------|-----------------------|---------|
| `emitFallbackInvestigationArtifact` (enhanced, pre-existing) | `HandleInvestigationMCPWithRegistry` → `startKAInvestigation` / `runBlockingInvestigation` | `pkg/apifrontend/tools/ka_investigate_mcp.go:513,629` | UT-AF-WIRE-SESSION-003 (IT), E2E-AF-1922-001 (E2E) |
| `EmitFallbackInvestigationArtifact` (new, test-only exported seam) | n/a — test seam, not a production entry point | `pkg/apifrontend/tools/ka_investigate_bridge.go` | UT-AF-1922-001, UT-AF-1922-002 |

---

## 5. Execution

```bash
go test ./pkg/apifrontend/tools/... -run "UT-AF-1922" -v -count=1
go test ./pkg/apifrontend/tools/... -run "UT-AF-WIRE-SESSION" -v -count=1
make test-e2e-apifrontend GINKGO_FOCUS="1922"
```
