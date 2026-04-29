# Test Plan: CRD-Aware Engine Registration

> **Template Version**: 2.0 — Hybrid IEEE 829 + Kubernaut

**Test Plan Identifier**: TP-868-v1.0
**Feature**: WorkflowExecution engine registration with CRD auto-discovery and graceful degradation
**Version**: 1.0
**Created**: 2026-04-29
**Author**: AI Assistant
**Status**: Active
**Branch**: `main`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the CRD-aware engine registration feature introduced by Issue #868. The WorkflowExecution service now probes for Tekton CRDs at startup and degrades gracefully to job-only mode when Tekton is not installed, instead of failing to start.

### 1.2 Objectives

1. Validate Tekton executor is registered only when CRDs are present
2. Validate graceful degradation to job-only mode when Tekton CRDs are absent
3. Validate `TektonConfig.TektonEnabled()` respects explicit opt-out
4. Validate workflows targeting Tekton produce actionable error when Tekton is unavailable
5. Validate `EngineAvailability` startup summary is accurate

### 1.3 Success Metrics

| Metric | Target |
|--------|--------|
| Unit test pass rate | 100% |
| All degraded-mode paths covered | Yes |

---

## 2. References

- [Issue #868](https://github.com/jordigilh/kubernaut/issues/868) — CRD-aware engine registration with degraded status
- `cmd/workflowexecution/main.go` — `tektonCRDsAvailable`, engine registration logic
- `pkg/workflowexecution/config/config.go` — `TektonConfig`, `TektonEnabled()`
- `internal/controller/workflowexecution/workflowexecution_controller.go` — `engineGuidance`

---

## 3. Test Scenarios

### 3.1 Unit Tests

| ID | Description | BR |
|----|-------------|-----|
| UT-WE-868-001 | Default `TektonConfig` (nil) allows auto-discovery | BR-WORKFLOW-868 |
| UT-WE-868-002 | `TektonEnabled()` returns false when `enabled: false` | BR-WORKFLOW-868 |
| UT-WE-868-003 | `TektonEnabled()` returns true when `enabled: true` | BR-WORKFLOW-868 |
| UT-WE-868-005 | `ExecutorRegistry.Get` returns error for unregistered engine | BR-WORKFLOW-868 |
| UT-WE-868-010 | `EngineAvailability` reports available/unavailable correctly | BR-WORKFLOW-868 |
| UT-WE-868-011 | `EngineAvailability` with zero optional engines (job only) | BR-WORKFLOW-868 |
| UT-WE-868-012 | `engineGuidance` produces actionable message for tekton | BR-WORKFLOW-868 |
| UT-WE-868-020 | Readiness check passes with job-only (zero optional engines) | BR-WORKFLOW-868 |
| UT-WE-868-021 | Readiness check fails with zero engines registered | BR-WORKFLOW-868 |

### 3.2 E2E Coverage (Indirect)

Existing WorkflowExecution E2E tests exercise both Tekton and Job execution paths. When Tekton CRDs are installed in the Kind cluster, both engines are registered. The degraded path (no Tekton) is covered by unit tests since Kind E2E clusters include Tekton.

---

## 4. Existing Test Coverage

| File | Test IDs | Tier |
|------|----------|------|
| `test/unit/workflowexecution/engine_discovery_test.go` | UT-WE-868-001..021 | Unit |

---

## 5. Execution

```bash
go test ./test/unit/workflowexecution/... -v -run "868"
```

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-29 | Initial test plan — documents existing coverage for QE readiness |
