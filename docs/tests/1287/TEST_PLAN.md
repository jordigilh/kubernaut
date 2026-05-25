# Test Plan: AF SA Token for KA Communication

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1287-v1.0
**Feature**: Replace JWT delegation with SA token for AF-to-KA communication
**Version**: 1.0
**Created**: 2026-05-25
**Author**: AI Agent
**Status**: Active
**Branch**: `feat/1287-1288-af-sa-ka-auth`

---

## 1. Introduction

### 1.1 Purpose

AF currently forwards the user's Keycloak JWT to KA via `ContextJWTDelegationTransport`. KA rejects it with 401 when `jwtProviders` is not configured. This plan validates the switch to SA token authentication (same pattern as AF-to-DS), with user identity flowing through MCP tool payload arguments for session tracking and audit.

### 1.2 Objectives

1. **AF transport swap**: AF uses `bearerTokenTransport` with SA token file for both KA MCP and REST clients
2. **KA schema acceptance**: KA tool input schemas accept `acting_user` and `acting_user_groups`
3. **Session ownership**: KA uses `acting_user` from payload for session driver attribution
4. **Audit attribution**: `acting_user`/`acting_user_groups` recorded in audit events
5. **No user AuthZ in KA**: KA performs service-level SAR only, not user-level authorization

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/... ./internal/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/apifrontend/... ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | coverprofile on modified files |
| Backward compatibility | 0 regressions | Existing test suites pass |

---

## 2. References

### 2.1 Authority

- Issue #1287: AF: use SA token for KA communication instead of JWT delegation
- DD-AUTH-MCP-001 v2.0 (being superseded)
- ADR-013 (AF JWT forwarding, being superseded)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Existing tests assert JWT delegation behavior | Test failures | High | UT-AF-1287-001..004 | Update assertions to verify SA token injection |
| R2 | KA session ownership breaks if `acting_user` absent | Session driver is SA identity, not human | Medium | UT-KA-1287-008 | Validate `acting_user` presence in tool input |
| R3 | `userFromContext` change breaks Pattern A callers | Non-AF callers lose identity | Low | UT-KA-1287-007 | Fallback to middleware identity when payload absent |

---

## 4. Scope

### 4.1 Features to be Tested

- **AF config** (`pkg/apifrontend/config/config.go`): `KABearerTokenFile` field
- **AF MCP transport** (`cmd/apifrontend/main.go`): `bearerTokenTransport` for KA MCP client
- **AF REST transport** (`pkg/apifrontend/ka/rest_client.go`): Remove internal JWT delegation
- **KA tool schemas** (`internal/kubernautagent/mcp/tools/`): `acting_user`/`acting_user_groups` in inputs
- **KA identity resolution** (`internal/kubernautagent/mcp/tools/registration.go`): Payload-based user identity
- **KA auth wiring** (`cmd/kubernautagent/main.go`): Pattern A only

### 4.2 Features Not to be Tested

- `ContextJWTDelegationTransport` and `AuditingJWTDelegationTransport` types (not deleted, may be used elsewhere)
- DS bearer token pattern (already tested, unchanged)

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of transport swap logic, schema changes, identity resolution
- **Integration**: >=80% of wiring (AF HTTP roundtrip, KA MCP tool dispatch)

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #1287 | AF injects SA token for KA MCP | P0 | Unit | UT-AF-1287-001 | Pending |
| #1287 | AF injects SA token for KA REST | P0 | Unit | UT-AF-1287-002 | Pending |
| #1287 | Missing token file falls back to no auth | P1 | Unit | UT-AF-1287-003 | Pending |
| #1287 | InvokeAction sends acting_user in payload | P0 | Unit | UT-AF-1287-004 | Pending |
| #1287 | KA schema accepts acting_user fields | P0 | Unit | UT-KA-1287-005 | Pending |
| #1287 | KA resolves acting_user from payload | P0 | Unit | UT-KA-1287-006 | Pending |
| #1287 | Non-intermediary caller uses middleware identity | P1 | Unit | UT-KA-1287-007 | Pending |
| #1287 | Session ownership uses acting_user | P0 | Unit | UT-KA-1287-008 | Pending |
| #1287 | AF SA token reaches KA mock (not JWT) | P0 | Integration | IT-AF-1287-001 | Pending |
| #1287 | MCP tool call with acting_user creates session | P0 | Integration | IT-KA-1287-002 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AF-1287-001` | `bearerTokenTransport` reads SA token from file and injects as Bearer header for KA MCP calls | Pending |
| `UT-AF-1287-002` | `bearerTokenTransport` injects SA token for KA REST calls (no JWT delegation) | Pending |
| `UT-AF-1287-003` | Missing `kaBearerTokenFile` config means no auth header injected | Pending |
| `UT-AF-1287-004` | `InvokeAction` still sends `acting_user` and `acting_user_groups` in MCP tool arguments | Pending |
| `UT-KA-1287-005` | `InvestigateInput`, `SelectWorkflowInput`, `CompleteNoActionInput` accept `acting_user` fields | Pending |
| `UT-KA-1287-006` | When `acting_user` present in tool input, KA uses it for UserInfo (session tracking + audit) | Pending |
| `UT-KA-1287-007` | When `acting_user` absent, KA falls back to middleware-extracted identity (Pattern A) | Pending |
| `UT-KA-1287-008` | Session ownership check (cancel, complete, takeover) uses `acting_user` from payload | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AF-1287-001` | Full HTTP roundtrip: AF SA token reaches KA mock server, not user JWT | Pending |
| `IT-KA-1287-002` | MCP tool call with AF SA token + `acting_user` in payload creates session with correct owner | Pending |

---

## 9. Test Cases

### UT-AF-1287-001: bearerTokenTransport injects SA token for KA MCP

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/ka/mcp_sdk_client_test.go`

**Test Steps**:
1. **Given**: A temp file containing a mock SA token; `KABearerTokenFile` config points to it
2. **When**: MCP client makes a request to KA
3. **Then**: The outbound `Authorization` header is `Bearer <SA-token-content>`, not the user's JWT

### UT-KA-1287-006: KA resolves acting_user from payload

**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/mcp/tools/investigate_test.go`

**Test Steps**:
1. **Given**: A tool call with `InvestigateInput{RRID: "rr-1", Action: "start", ActingUser: "alice"}`
2. **When**: The tool handler resolves user identity
3. **Then**: The session is created with driver = "alice", not the SA identity from auth middleware

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `pkg/apifrontend/auth/jwt_delegation_test.go` | Asserts JWT header injection for KA | Tests remain valid (types not deleted) | JWT delegation types kept for other uses |
| `pkg/apifrontend/ka/rest_client_test.go` (UT-AF-110-005*) | Asserts JWT forwarding in REST client | Update to verify no JWT; SA token from transport | REST client no longer wraps with JWT delegation |
| `pkg/apifrontend/ka/invoke_action_test.go` | Asserts `acting_user` in MCP args | No change — still sends acting_user | Behavior preserved |
| `test/e2e/apifrontend/jwt_delegation_test.go` (G7) | E2E JWT delegation to KA | Needs rework for SA token model | E2E validates new auth path |

---

## 16. FedRAMP Control Mapping

| Test ID | Control | Behavior Verified |
|---------|---------|-------------------|
| UT-AF-1287-001 | IA-5 | KABearerTokenFile parsed from config |
| UT-AF-1287-002 | IA-5 | REST client does NOT inject JWT from context |
| UT-AF-1287-003 | IA-5 | Missing KABearerTokenFile means empty (no auth) |
| UT-AF-1287-004 | IA-5 | bearerTokenTransport injects SA token from file |
| UT-AF-1287-008 | IA-5 | Config rejects inaccessible token file |
| UT-AF-1287-009 | AU-3 | SelectWorkflow includes acting_user in args |
| UT-AF-1287-010 | AU-3 | DiscoverWorkflows includes acting_user in args |
| UT-KA-1287-005 | AU-3 | KA schemas unmarshal acting_user |
| UT-KA-1287-006 | AU-3 | KA prefers acting_user from payload |
| UT-KA-1287-007 | AU-3 | KA falls back to middleware identity |
| UT-KA-1287-011 | SC-5 | KA rate limiter accepts SA identity (accepted risk) |
| IT-AF-1287-001 | IA-5, SC-7 | buildBackendDeps sends SA token to KA MCP mock |
| IT-AF-1287-002 | IA-5 | REST client does NOT inject user JWT (SA token at transport layer) |
| TC-E2E-SA-01 | IA-5 | AF authenticates to KA with SA token in live cluster |
| TC-E2E-SA-02 | IA-5 | Expired caller JWT rejected at AF edge |

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-25 | Initial test plan |
| 1.1 | 2026-05-25 | Added FedRAMP control mapping (F-10) |
