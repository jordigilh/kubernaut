# Test Plan: Progressive RCA Emission

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1407-v1
**Feature**: Early RCA emission and auto-proceed investigate-to-discover behavior
**Version**: 1.0
**Created**: 2026-06-13
**Author**: AI Agent
**Status**: Implemented
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the progressive RCA (Root Cause Analysis) feature where:

1. The investigation tool emits an early `early_rca` decision event via EventBridge
   as soon as investigation completes (before final response)
2. The LLM is prompted to auto-proceed from investigation to workflow discovery
   without stopping for user confirmation
3. The final `investigation_summary` remains the definitive structured artifact

### 1.2 Objectives

1. **Early RCA emission**: `early_rca` status-update emitted on investigation complete
2. **Auto-proceed**: LLM transitions from investigate → discover_workflows without pause
3. **FedRAMP audit trail**: Early RCA includes confidence + causal chain
4. **Nil-safety**: No panic when EventBridge is absent (non-streaming context)
5. **Prompt compliance**: System prompt allows investigate-to-discover without asking

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-1407"` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "IT-AF-1407"` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend` (progressive_flow label) |
| Prompt assertion pass rate | 100% | `go test ./pkg/apifrontend/agent/... -run "UT-AF-1407"` |

---

## 2. References

### 2.1 Authority

- Issue #1407: Progressive RCA emission
- A2A Streaming Protocol v1.2 (§ Event Routing Pipeline)

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| SI-4(5) | Automated detection mechanisms | Early RCA surfaces causal chain for immediate action | UT-AF-1407-001 |
| AU-3 | Audit content (confidence, chain) | Early RCA includes confidence + causal chain | UT-AF-1407-003 |
| IR-4(1) | Automated incident handling | Auto-proceed enables faster remediation start | UT-AF-1407-011 |

---

## 3. Test Scenarios

### 3.1 Unit Tests (RCA Emission)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-1407-001 | Early RCA emitted on investigation complete | StatusUpdate with `type=decision` | Implemented |
| UT-AF-1407-002 | No early RCA when investigation has no structured result | No emission | Implemented |
| UT-AF-1407-003 | Early RCA includes confidence and causal chain | Confidence + chain in payload | Implemented |
| UT-AF-1407-004 | Early RCA emission without EventBridge is no-op | No panic | Implemented |

### 3.2 Unit Tests (Prompt)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-1407-010 | Prompt does NOT contain unconditional MUST STOP | No blocking directive | Implemented |
| UT-AF-1407-011 | Prompt contains auto-proceed from investigate to discover | Directive present | Implemented |
| UT-AF-1407-012 | Prompt preserves investigate-only exception | Exception for explicit user requests | Implemented |
| UT-AF-1407-013 | Prompt retains present_decision as final artifact | Structured decision requirement | Implemented |

### 3.3 Integration Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| IT-AF-1407-001 | Early RCA decision event flows through EventBridge | Event received in queue | Implemented |

### 3.4 E2E Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| E2E-AF-1407-001 | Early RCA decision event emitted during progressive flow | Event in SSE stream | Implemented |
| E2E-AF-1407-002 | Progressive flow reaches terminal state without user intervention | Task completes | Implemented |
| E2E-AF-1407-003 | early_rca payload contains severity and confidence | Fields present and valid | Implemented |

---

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|----------------------|---------------------|----------------|
| emitEarlyRCA | HandleInvestigate() | pkg/apifrontend/tools/ka_investigate.go | IT-AF-1407-001 |
| EventBridge.EmitStructuredMeta | emitEarlyRCA() | pkg/apifrontend/launcher/event_bridge.go:218 | UT-AF-1407-001 |
| Auto-proceed prompt directive | prompt.txt Phase 3 | pkg/apifrontend/agent/prompt.txt | UT-AF-1407-011 |

---

## 5. Execution

```bash
go test ./pkg/apifrontend/tools/... -run "UT-AF-1407" -v -count=1
go test ./pkg/apifrontend/tools/... -run "IT-AF-1407" -v -count=1
go test ./pkg/apifrontend/agent/... -run "UT-AF-1407" -v -count=1
make test-e2e-apifrontend GINKGO_FOCUS="1407"
```
