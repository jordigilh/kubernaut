# Test Plan: Remove KA Impersonate SSAR Gate and Runtime Impersonation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1288-v1.0
**Feature**: Remove impersonate SSAR startup gate and runtime K8s impersonation from KA
**Version**: 1.0
**Created**: 2026-05-25
**Author**: AI Agent
**Status**: Active
**Branch**: `feat/1287-1288-af-sa-ka-auth`

---

## 1. Introduction

### 1.1 Purpose

KA's startup performs a `SelfSubjectAccessReview` for the `impersonate` verb. If the SA lacks impersonate RBAC, interactive mode is soft-disabled and the MCP endpoint at `/api/v1/mcp` is never mounted. Additionally, KA impersonates the user for K8s API calls at runtime via `WithImpersonatedUser`. With the trusted intermediary model (#1287), KA operates with its own SA for all K8s calls and `acting_user` is audit-only. Both the startup gate and runtime impersonation are removed.

### 1.2 Objectives

1. **MCP mounts unconditionally**: When `interactive.enabled: true`, MCP endpoint mounts regardless of impersonate RBAC
2. **No runtime impersonation**: K8s API calls use KA SA identity, not user impersonation
3. **Clean removal**: Dead code (`CheckImpersonatePermission`, `InteractiveReadiness`, `NewImpersonatingConfig`) removed

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| No impersonate references in KA runtime | 0 matches | `grep -r WithImpersonatedUser internal/kubernautagent/` |

---

## 2. References

### 2.1 Authority

- Issue #1288: KA: impersonate SSAR check gates interactive mode unnecessarily
- Issue #1287: AF SA token for KA communication (dependency)
- #891: Original impersonate check introduction
- #895/#896: Impersonation hardening

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | KA SA lacks K8s permissions for operations done under impersonation | Runtime K8s API call failures | Medium | UT-KA-1288-002 | Verify KA SA ClusterRole covers pod/node/secret reads |
| R2 | Removing impersonation breaks K8s call audit attribution | Audit shows KA SA, not human user | Low | — | `acting_user` in audit events via tool input (separate path) |
| R3 | Existing impersonate tests fail | Test suite regression | High | UT-KA-891-001 | Update/remove tests that assert impersonate behavior |

---

## 4. Scope

### 4.1 Features to be Tested

- **SSAR gate removal** (`cmd/kubernautagent/main.go`): MCP mounts without impersonate RBAC check
- **Runtime impersonation removal** (`internal/kubernautagent/mcp/tools/investigate.go`, `select_workflow.go`): No `WithImpersonatedUser` calls
- **Dead code removal** (`internal/kubernautagent/rbac/impersonate_check.go`, `internal/kubernautagent/mcp/impersonate.go`)

### 4.2 Features Not to be Tested

- `pkg/shared/transport/impersonate.go` (shared infrastructure, not deleted)
- Helm chart RBAC (dev-only chart, no production security requirements)

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #1288 | MCP mounts without impersonate RBAC | P0 | Unit | UT-KA-1288-001 | Pending |
| #1288 | K8s calls use KA SA (no impersonation headers) | P0 | Unit | UT-KA-1288-002 | Pending |
| #1288 | Interactive turn runs without WithImpersonatedUser | P0 | Unit | UT-KA-1288-003 | Pending |
| #1288 | MCP handler serves requests without impersonate RBAC | P0 | Integration | IT-KA-1288-001 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-1288-001` | MCP endpoint mounts when `interactive.enabled: true` without impersonate RBAC on SA | Pending |
| `UT-KA-1288-002` | K8s API calls during interactive turn use KA SA identity (no Impersonate-User/Group headers) | Pending |
| `UT-KA-1288-003` | Interactive turn executes successfully without `WithImpersonatedUser` context enrichment | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-1288-001` | MCP handler mounts and serves tool calls without impersonate RBAC on the SA | Pending |

---

## 9. Test Cases

### UT-KA-1288-001: MCP mounts without impersonate RBAC

**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/rbac/impersonate_check_test.go`

**Test Steps**:
1. **Given**: `interactive.enabled: true` and SA lacks impersonate permission
2. **When**: KA startup logic evaluates MCP mounting condition
3. **Then**: MCP route is mounted (not soft-disabled)

### UT-KA-1288-002: K8s calls use KA SA identity

**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/mcp/tools/investigate_test.go`

**Test Steps**:
1. **Given**: An interactive turn with a mock K8s backend
2. **When**: KA makes K8s API calls during tool execution
3. **Then**: No `Impersonate-User` or `Impersonate-Group` headers are present in K8s requests

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `internal/kubernautagent/rbac/impersonate_check_test.go` (UT-KA-891-001) | Asserts SSAR check and InteractiveReadiness states | Remove or update — SSAR check no longer exists | Gate removed |
| `internal/kubernautagent/mcp/impersonate_test.go` | Tests `NewImpersonatingConfig` | Remove — function deleted | Dead code |
| `pkg/shared/transport/impersonate_test.go` (UT-KA-703-F01..F09) | Tests `WithImpersonatedUser` and `ImpersonatingRoundTripper` | Keep unchanged — shared infrastructure | Not deleted |
| `pkg/shared/transport/impersonate_audit_test.go` (UT-KA-898-001) | Tests impersonation audit events | Keep unchanged — shared infrastructure | Not deleted |
| `test/integration/kubernautagent/transport/impersonate_audit_integration_test.go` | IT for impersonation audit | Keep unchanged — shared infrastructure | Not deleted |

---

## 16. FedRAMP Control Mapping

| Test ID | Control | Behavior Verified |
|---------|---------|-------------------|
| UT-KA-1288-001 | AC-6 | MCP mounts unconditionally (no SSAR gate) |
| UT-KA-1288-002 | AC-6 | No impersonation headers in enrichment hook |
| UT-KA-1288-003 | AC-6 | No impersonation headers in interactive turns |

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-25 | Initial test plan |
| 1.1 | 2026-05-25 | Added FedRAMP control mapping (F-10) |
