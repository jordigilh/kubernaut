# Test Plan: Prevent LLM Auto-Approval (Approval Consent Guard)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1415-v1
**Feature**: Remove kubernaut_approve from A2A agent to prevent LLM auto-approval
**Version**: 1.0
**Created**: 2026-06-13
**Author**: AI Agent
**Status**: Implemented
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the structural removal of `kubernaut_approve` from the A2A LLM agent toolset, ensuring the LLM cannot autonomously approve Remediation Approval Requests (RARs). Approval actions are restricted to the Console UI via the MCP endpoint, enforcing human-in-the-loop consent.

### 1.2 Objectives

1. **Tool absence**: `kubernaut_approve` not present in A2A agent tool list under any configuration
2. **Prompt reinforcement**: LLM explicitly told it cannot approve/reject
3. **Console path intact**: MCP `kubernaut_approve` still works for authenticated Console users
4. **Adversarial resilience**: Prompt injection cannot re-introduce the tool
5. **Audit attribution**: All approvals attributed to human users, never LLM

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Adversarial test pass rate | 100% | `go test ./pkg/apifrontend/agent/... -run "ADV-AF-1415"` |
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/agent/... -count=1` |
| E2E pass rate | 100% | `make test-e2e-apifrontend` (A2A approval scenarios) |
| Tool count regression | 0 | Agent reports 23 tools (not 24) |

---

## 2. References

### 2.1 Authority

- Issue #1415: Prevent LLM auto-approval of RARs
- DD-AF-006: Approval Consent Guard
- ADR-022: AF SA unified security model
- ADR-040: Remediation Approval Request architecture

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| AC-6 (Least Privilege) | LLM must not hold approval capability | Tool structurally absent from A2A agent | ADV-AF-1415-001 |
| AU-2 (Audit Events) | Approval actions auditable with user attribution | MCP path emits audit with username | TC-E2E-MCP-FULL-04 |
| SI-10 (Input Validation) | Block adversarial paths to auto-approve | Tool absence prevents any invocation path | ADV-AF-1415-002 |

### 2.3 Cross-References

- [DD-AF-006](../../architecture/decisions/DD-AF-006-approval-consent-guard.md)
- Console sub-issue: `jordigilh/kubernaut-demo-console#2`
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)

---

## 3. Scope

### 3.1 In Scope

- A2A agent tool registration (`buildToolList` in `root.go`)
- LLM system prompt (`prompt.txt`) Console-only guidance
- Adversarial test scenarios
- E2E refactoring (TC-E2E-A2A-T07, TC-E2E-A2A-WF-04)
- Mock-LLM scenario removal (`af_approve`)

### 3.2 Out of Scope

- Console UI implementation of Approve/Reject buttons (tracked in console repo)
- MCP bridge approval handler (existing, unchanged)
- RBAC policy changes (existing SAR grants unchanged)

---

## 4. Test Scenarios

### 4.1 Unit Tests (Adversarial)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| ADV-AF-1415-001 | Enumerate all A2A tools; check kubernaut_approve absent | Tool not in list (interactive mode) | Implemented |
| ADV-AF-1415-002 | Enumerate all A2A tools in non-interactive mode | Tool not in list (non-interactive mode) | Implemented |

### 4.2 Unit Tests (Structural)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-100-002 | Agent tool count in interactive mode | HaveLen(23) | Implemented |
| UT-AF-100-002 (non-interactive) | Agent tool count in non-interactive mode | HaveLen(14) | Implemented |

### 4.3 E2E Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| TC-E2E-A2A-T07 | User asks agent to approve via A2A | Agent returns guidance text ("use Console"), no tool call | Implemented |
| TC-E2E-A2A-WF-04 | Workflow: list + get approval requests (no approve step) | Console-only guidance in response | Implemented |
| TC-E2E-MCP-FULL-02+04 | Console path: kubernaut_approve via MCP | Approval succeeds with user attribution | Implemented |

---

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|----------------------|---------------------|----------------|
| Tool removal | buildToolList() | pkg/apifrontend/agent/root.go | ADV-AF-1415-001 |
| Prompt update | embedded prompt.txt | pkg/apifrontend/agent/prompt.txt | TC-E2E-A2A-T07 |
| MCP approve (retained) | RegisterTools() | pkg/apifrontend/handler/mcp_bridge.go:156 | TC-E2E-MCP-FULL-04 |
| Mock-LLM removal | af_approve scenario deleted | deploy/apifrontend/overlays/e2e/mock-llm.yaml | TC-E2E-A2A-T07 |

---

## 6. Execution

```bash
# Adversarial tests
go test ./pkg/apifrontend/agent/... -run "ADV-AF-1415" -v -count=1

# Full agent unit tests (includes tool count assertions)
go test ./pkg/apifrontend/agent/... -v -count=1

# E2E tests
make test-e2e-apifrontend GINKGO_FOCUS="A2A-T07|MCP-FULL-02"
```
