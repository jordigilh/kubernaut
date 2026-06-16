# Test Plan: Execution Progress Artifacts

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1403-v1
**Feature**: Execution progress artifact emission during remediation watch
**Version**: 1.0
**Created**: 2026-06-13
**Author**: AI Agent
**Status**: Implemented
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the execution progress artifact system that emits structured
progress snapshots via `TaskArtifactUpdateEvent` (DataPart + TextPart) during the
`kubernaut_watch` tool execution. This enables Console/enhanced clients to render
step-by-step remediation progress indicators.

### 1.2 Objectives

1. **Progress snapshot emission**: Phase transitions emit `execution_progress` artifacts
2. **DataPart + TextPart structure**: Multi-part artifact with JSON data and text fallback
3. **Stabilization window**: EA CRD `stabilizationWindow` included in Verifying phase metadata
4. **Schema compliance**: Payload validates against `execution_progress.v1.schema.json`
5. **Nil-safety**: `EmitArtifactSafe` is no-op when EventBridge absent

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-1403"` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend` (execution_progress label) |
| Schema validation | 100% | All emitted payloads pass JSON Schema validation |

---

## 2. References

### 2.1 Authority

- Issue #1403: Execution progress artifacts in SSE stream
- A2A Streaming Protocol v1.2 (§ Execution Progress Artifacts)

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| AU-3 | Audit content completeness | Progress events carry phase, rr_id, timestamp | UT-AF-1403-001..003 |
| SI-4(5) | Automated monitoring | Real-time phase transitions visible to operators | E2E-AF-1403-001 |

---

## 3. Test Scenarios

### 3.1 Unit Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-1403-001 | Non-terminal phase builds snapshot | `completed: false`, valid JSON | Implemented |
| UT-AF-1403-002 | Terminal phase includes completed_at | `completed: true` with timestamp | Implemented |
| UT-AF-1403-003 | Non-terminal phase omits completed_at | No `completed_at` field | Implemented |
| UT-AF-1403-004 | FetchStabilizationWindow returns EA CRD value | Returns stabilization window string | Implemented |
| UT-AF-1403-005 | FetchStabilizationWindow returns empty on missing EA | Graceful fallback | Implemented |
| UT-AF-1403-006 | Schema validates correct payload | JSON Schema validation passes | Implemented |
| UT-AF-1403-007 | EmitArtifactSafe nil-safe (no bridge) | No panic, no-op | Implemented |
| UT-AF-1403-008 | HandleWatch emits artifact on phase transition | TaskArtifactUpdateEvent emitted | Implemented |
| UT-AF-1403-009 | Includes stabilization_window on Verifying phase | Metadata contains window | Implemented |
| UT-AF-1403-010 | Omits stabilization_window when EA ref absent | No window in metadata | Implemented |
| UT-AF-1403-011 | DataPart JSON structure validates | Expected fields present and serializable | Implemented |

### 3.2 E2E Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| E2E-AF-1403-001 | SSE stream emits execution_progress artifact on phase transitions | Artifact with DataPart received in SSE | Implemented |

---

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|----------------------|---------------------|----------------|
| BuildProgressSnapshot | HandleWatch() | pkg/apifrontend/tools/af_watch.go | UT-AF-1403-001 |
| EmitArtifact (EventBridge) | emitStructuredOutput() | pkg/apifrontend/launcher/part_converter.go:416 | UT-AF-1403-008 |
| execution_progress schema | pkg/apifrontend/launcher/schemas/ | embedded via go:embed | UT-AF-1403-006 |

---

## 5. Execution

```bash
go test ./pkg/apifrontend/tools/... -run "UT-AF-1403" -v -count=1
make test-e2e-apifrontend GINKGO_FOCUS="1403"
```
