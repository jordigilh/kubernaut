# Test Plan: AF status/subscribe drops workflow-selection update during Analyzing

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut (lightweight variant for a
> single-package bug fix; see `docs/testing/TEST_PLAN_TEMPLATE.md` for the full template)

**Test Plan Identifier**: TP-2325-v1.0
**Feature**: Fix two compounding bugs in the AF `status/subscribe` SSE stream that silently
drop the workflow-selection update while `OverallPhase` stays `Analyzing`
**Version**: 1.0
**Created**: 2026-08-31
**Status**: Active
**Branch**: `fix/2325-af-status-stream-workflow-selection-drop`

---

## 1. Introduction

### 1.1 Purpose

The Console's investigation view stays blank on `workflow_id` until `OverallPhase` reaches
`Executing`, even though workflow selection completes tens of seconds earlier while the RR is
still `Analyzing`. Two compounding bugs in `pkg/apifrontend/handler/` cause this: the SSE loop
only emits on phase change, and the metadata builder only sets `workflow_id` for the `Executing`
phase. This plan proves both bugs are fixed and guards against the SSE stream emitting spurious
duplicate events as a side effect of loosening the emission gate.

### 1.2 Objectives

1. **workflow_id is phase-independent**: `BuildPhaseMetadata` returns `workflow_id` whenever
   `status.workflowSelection.selectedWorkflowRef` is set, regardless of `OverallPhase`.
2. **Selection changes emit mid-phase**: the `status/subscribe` SSE loop emits a `status/update`
   when `status.workflowSelection` changes even if `OverallPhase` does not.
3. **No regression**: a true no-op RR touch (neither phase nor workflow selection changed) must
   not emit a spurious extra `status/update` event.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-apifrontend` |
| Integration test pass rate | 100% | `make test-integration-apifrontend` |
| E2E test pass rate (regression) | 100% | `make test-e2e-apifrontend` |
| Backward compatibility | 0 regressions | Full `pkg/apifrontend/...` and `test/integration/apifrontend/...` suites pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-API-008: workflow status tracking and real-time updates
- [DD-AF-008](../../architecture/decisions/DD-AF-008-status-subscribe-sse.md): `status/subscribe` SSE contract (Per-Phase Metadata table + RR Sub-Field Events section updated by this fix)
- Issue #2325: AF status stream drops the workflow-selection update

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Loosening the emission gate (phase OR workflow-ID change) causes duplicate/spurious `status/update` events on unrelated RR touches | Console re-renders unnecessarily; SSE bandwidth waste | Medium | IT-AF-2325-002 | Explicit regression-guard IT asserting a genuine no-op touch produces zero extra events |
| R2 | `workflow_id` promoted to the phase-independent block leaks into phases where it was previously absent by design (e.g. `Blocked`) | Console displays a field it didn't expect for that phase | Low | UT-AF-2325-002, full `status_types_test.go` regression run | `workflow_id` is additive metadata (raw CRD field passthrough per DD-AF-008); existing per-phase tests for other phases assert exact keys present and continue to pass, proving no unexpected key removal/shape change elsewhere |

### 3.1 Risk-to-Test Traceability

R1 -> IT-AF-2325-002. R2 -> UT-AF-2325-002 plus full existing `status_types_test.go` suite (regression, unmodified).

---

## 4. Test Scenarios

### 4.1 Unit Tests (`pkg/apifrontend/handler/status_types_test.go`)

| Test ID | Scenario | FedRAMP Control | Assertion |
|---------|----------|-----------------|-----------|
| UT-AF-2325-001 | `Analyzing` phase + `workflowSelection.selectedWorkflowRef` set | SI-4 (monitoring) | `BuildPhaseMetadata` returns `workflow_id` |
| UT-AF-2325-002 | `Analyzing` phase, no selection yet | SI-10 (input validation / no fabricated data) | `BuildPhaseMetadata` omits `workflow_id` |

### 4.2 Integration Tests (`test/integration/apifrontend/status_subscribe_test.go`, envtest)

| Test ID | Scenario | FedRAMP Control | Assertion |
|---------|----------|-----------------|-----------|
| IT-AF-2325-001 | RR created as `Analyzing`; `workflowSelection` populated mid-stream, phase unchanged | SI-4 | A second `status/update` event is received with `phase: "Analyzing"` and `metadata.workflow_id` set |
| IT-AF-2325-002 | RR created as `Analyzing`; unrelated status field touched (no phase/selection change), then a real `Executing` transition | SI-4 (regression guard) | Exactly one event for the no-op window (zero extra), followed by exactly one `Executing` event |

### 4.3 E2E (regression only, no new scenarios)

This is a bug fix to an existing, well-covered endpoint — no new user journey is introduced, so
no new E2E specs are added. `make test-e2e-apifrontend` (AF's dedicated Kind-cluster E2E suite,
which already exercises `/a2a/status` SSE streaming) is run as a regression gate to confirm the
loosened emission gate does not destabilize the live SSE path end-to-end.

---

## 5. Environment

- **Unit + Integration**: local envtest (`KUBEBUILDER_ASSETS` via `setup-envtest`), run on the
  dedicated helios08 host (`/root/kubernaut-2325`) due to local sandbox Podman/image-build
  resource constraints.
- **E2E**: Kind cluster on helios08, per `make test-e2e-apifrontend` (`test/e2e/apifrontend/...`).

---

## 6. Completion Criteria

- [x] UT-AF-2325-001, UT-AF-2325-002 pass (`make test-unit-apifrontend` / local `go test`)
- [ ] IT-AF-2325-001, IT-AF-2325-002 pass (`make test-integration-apifrontend` on helios08)
- [ ] `make test-e2e-apifrontend` passes with zero regressions on helios08
- [x] `go build ./...`, `golangci-lint run` clean on changed packages
- [x] DD-AF-008 Per-Phase Metadata table and lifecycle notes updated
