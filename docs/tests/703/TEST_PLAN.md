# Test Plan: KA MCP Interactive Mode (#703)

> **Template Version**: 2.0 -- Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-703-v1
**Feature**: Dynamic takeover interactive investigation via MCP
**Version**: 2.0
**Created**: 2026-04-29
**Revised**: 2026-04-29
**Author**: AI-assisted
**Status**: Draft
**Branch**: `feat/703-ka-mcp-interactive`

---

## 1. Introduction

### 1.1 Purpose

Validate that the KA MCP Interactive Mode enables secure, auditable, dynamic takeover of autonomous investigations with zero regression to existing autonomous behavior.

### 1.2 Objectives

1. **Security**: All 14 impersonation penetration scenarios pass (CP-2)
2. **Correctness**: All 14 dynamic takeover adversarial scenarios pass (CP-3)
3. **Audit**: Every interactive action is attributable and reconstructable from DS
4. **Regression**: Existing autonomous tests pass unchanged
5. **Integration**: Full 14-step interactive flow works end-to-end in Kind

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | Security-critical: >=90% (`auth/`, `mcp/auth.go`, `mcp/impersonate.go`) |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| E2E pass rate | 100% | `make test-e2e-kubernautagent-interactive` |
| Backward compatibility | 0 regressions | Existing autonomous tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-INTERACTIVE-001 through BR-INTERACTIVE-008 (`docs/requirements/BR-INTERACTIVE.md`)
- DD-AUTH-MCP-001: MCP Endpoint Security and User Impersonation
- DD-INTERACTIVE-002: Dynamic Takeover Model (supersedes DD-001)
- Issue #703: KA MCP Interactive Mode

### 2.2 Cross-References

- Testing Strategy (`.cursor/rules/03-testing-strategy.mdc`)
- ADR-038: Async Buffered Audit Ingestion
- DD-AUDIT-003: Service Audit Trace Requirements
- DD-TEST-006: Test Plan Policy

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Impersonation header injection allows privilege escalation | Critical | Low | PEN-01 through PEN-14 | Middleware header stripping + SAR verification |
| R2 | Cancel+reconstruct loses conversation context | High | Low | TAKE-05, TAKE-06 | Reconstruct from DS audit events with golden sequence validation |
| R3 | go-sdk v1.5.0 API incompatibility | High | Low | All PR2 tests | Compilation verified; SDK API matches design |
| R4 | Lease orphan prevents new sessions | Medium | Low | SESS-04 | 30s Lease expiry + AfterEach cleanup |
| R5 | Interactive E2E exceeds CI timeout | Medium | Medium | E2E-* | Separate Ginkgo label group, 20m timeout |
| R6 | DS unavailable during auto-inject | Medium | Low | TAKE-07 | Best-effort; proceed with empty context |

---

## 4. Scope

### 4.1 Features to be Tested

- **MCP Server Transport** (`internal/kubernautagent/mcp/`): Streamable HTTP, auth, impersonation
- **Interactive Session** (`internal/kubernautagent/mcp/session/`): Lease, lifecycle, timeout
- **kubernaut_investigate** (`internal/kubernautagent/mcp/tools/investigate.go`): Dynamic takeover, cancel+reconstruct
- **kubernaut_enrich** (`internal/kubernautagent/mcp/tools/enrich.go`): Enrichment via MCP
- **kubernaut_select_workflow** (`internal/kubernautagent/mcp/tools/select_workflow.go`): Workflow selection
- **NotificationBus** (`internal/kubernautagent/mcp/notifications/bus.go`): Observer event delivery
- **Audit** (`internal/kubernautagent/audit/`): `SessionID` top-level field, new event types, DS store
- **Auth** (`pkg/shared/auth/`): `ValidateTokenFull`, header stripping, impersonation
- **CRD** (`api/aianalysis/v1alpha1/`): `InteractiveSessionInfo` status type
- **RO** (`internal/controller/remediationorchestrator/`): Dynamic timeout extension
- **Helm** (`charts/kubernaut/`): Conditional RBAC, interactive config

### 4.2 Features Not to be Tested

- **kubernaut-apifrontend**: Separate repo, separate test plan
- **OIDC/OAuth2**: Not in KA scope (lives in apifrontend)
- **NetworkPolicy for apifrontend**: Tracked in #894, blocked on apifrontend chart

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code. Security-critical files >=90%.
- **Integration**: >=80% of integration-testable code.
- **E2E**: Separate Ginkgo label group (`Label("interactive")`) with 20m timeout.

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass, per-tier coverage meets thresholds, zero regressions, all checkpoint checks green.

**FAIL**: Any P0 test fails, coverage below thresholds, existing tests regress, any Critical checkpoint finding.

### 5.3 Suspension Criteria

Testing shall be suspended when:
1. Build failures prevent test compilation (`go build ./...` fails)
2. Critical infrastructure unavailable (Kind cluster, mock-LLM, DS) for >10 minutes
3. Blocking dependency not available (go-sdk v1.5.0 for CP-2+)
4. Security vulnerability discovered in auth path (immediate stop until patched)

### 5.4 Resumption Requirements

Testing resumes when:
1. Build/infrastructure issue resolved and verified
2. Root cause of suspension documented in this plan's changelog
3. All previously-passing tests re-verified green before proceeding

---

## 6. Test Deliverables

| Deliverable | Format | Location |
|-------------|--------|----------|
| Test Plan (this document) | Markdown | `docs/tests/703/TEST_PLAN.md` |
| CP-0 Test Specifications | Markdown | `docs/tests/703/CP-0_DESIGN_GATE.md` |
| CP-1 Test Specifications | Markdown | `docs/tests/703/CP-1_CRD_CONTROLLER_GATE.md` |
| CP-2 Test Specifications | Markdown | `docs/tests/703/CP-2_SECURITY_GATE.md` |
| CP-3 Test Specifications | Markdown | `docs/tests/703/CP-3_SESSION_AUDIT_GATE.md` |
| CP-4 Test Specifications | Markdown | `docs/tests/703/CP-4_TOOL_COMPLETENESS_GATE.md` |
| CP-5 Test Specifications | Markdown | `docs/tests/703/CP-5_RELEASE_GATE.md` |
| Unit test code | Go (Ginkgo) | `test/unit/kubernautagent/mcp/` |
| Integration test code | Go (Ginkgo) | `test/integration/kubernautagent/mcp/` |
| E2E test code | Go (Ginkgo) | `test/e2e/kubernautagent/` |
| Coverage reports | HTML/text | CI artifacts |

---

## 7. Responsibilities

| Role | Responsibility | Personnel |
|------|---------------|-----------|
| Test Plan Author | Create and maintain test specifications | AI-assisted + Project Lead |
| Test Implementer | Write test code following specifications | AI-assisted |
| Test Reviewer | Validate tests match specifications | Project Lead |
| Security Reviewer | Validate CP-2 penetration coverage | Architecture Team |
| Sign-off Authority | Approve checkpoint gates | Project Lead |

---

## 8. Checkpoint-to-Test Mapping (Summary)

Detailed test case specifications (preconditions, steps, acceptance criteria) are in the per-checkpoint files linked below.

### CP-0: Design Gate (22 checks) → [CP-0_DESIGN_GATE.md](CP-0_DESIGN_GATE.md)

| Check ID | Test ID | Tier | Description |
|----------|---------|------|-------------|
| THREAT-01..05 | N/A (doc review) | Review | Threat model completeness |
| SEC-01..06 | N/A (doc review) | Review | Security audit of DD-AUTH-MCP-001 |
| QE-01..05 | N/A (doc review) | Review | Test plan coverage audit |
| GOV-01..06 | N/A (doc review) | Review | Governance documents exist |

### CP-1: CRD & Controller Gate (13 checks) → [CP-1_CRD_CONTROLLER_GATE.md](CP-1_CRD_CONTROLLER_GATE.md)

| Check ID | Test ID | Tier | Description |
|----------|---------|------|-------------|
| ADV-CRD-01 | UT-KA-INT-001 | Unit | v1.5 CRD over v1.4 AA data |
| ADV-CRD-02 | UT-KA-INT-002 | Unit | v1.4 controller reads v1.5 AAs |
| ADV-CRD-03 | UT-KA-INT-003 | Unit | InteractiveSessionInfo mutable |
| ADV-CRD-04 | UT-KA-INT-004 | Unit | Unknown poll status handling |
| ADV-CRD-05 | UT-KA-INT-005 | Unit | Timeout extension bounded by global max |
| ADV-CRD-06 | UT-KA-INT-006 | Unit | Nil InteractiveSession returns default 10m |
| SEC-CRD-01 | UT-KA-INT-007 | Unit | No spec field changes |
| SEC-CRD-02 | UT-KA-INT-008 | Unit | ActingUser from authenticated source |
| REG-01..05 | UT-KA-INT-009..013 | Unit | Regression: existing tests pass unchanged |

### CP-2: SECURITY GATE (28 checks) → [CP-2_SECURITY_GATE.md](CP-2_SECURITY_GATE.md)

| Check ID | Test ID | Tier | Description |
|----------|---------|------|-------------|
| PEN-01 | UT-KA-SEC-001 | Unit | Header injection Pattern A (user) |
| PEN-02 | UT-KA-SEC-002 | Unit | Header injection Pattern A (groups) |
| PEN-03 | UT-KA-SEC-003 | Unit | Header injection Pattern A (extras) |
| PEN-04 | UT-KA-SEC-004 | Unit | Unauthorized delegation Pattern B |
| PEN-05 | UT-KA-SEC-005 | Unit | Missing impersonation header Pattern B |
| PEN-06 | UT-KA-SEC-006 | Unit | Empty impersonation |
| PEN-07 | UT-KA-SEC-007 | Unit | Self-impersonation |
| PEN-08 | UT-KA-SEC-008 | Unit | No auth |
| PEN-09 | UT-KA-SEC-009 | Unit | Expired token |
| PEN-10 | UT-KA-SEC-010 | Unit | Forged Bearer |
| PEN-11 | UT-KA-SEC-011 | Unit | Feature gate off returns 404 |
| PEN-12 | UT-KA-SEC-012 | Unit | Auth middleware nil guard |
| PEN-13 | IT-KA-SEC-001 | Integration | Rate limit bypass via MCP |
| PEN-14 | UT-KA-SEC-013 | Unit | ImpersonationConfig has UserName + Groups |
| PROP-01..05 | UT-KA-SEC-014..018 | Unit | Security properties |
| QE-INT-01..05 | IT-KA-SEC-002..006 | Integration | MCP integration checks |

### CP-3: SESSION & AUDIT GATE (34 checks) → [CP-3_SESSION_AUDIT_GATE.md](CP-3_SESSION_AUDIT_GATE.md)

| Check ID | Test ID | Tier | Description |
|----------|---------|------|-------------|
| SESS-01 | UT-KA-SESS-001 | Unit | Session hijacking rejection |
| SESS-02 | UT-KA-SESS-002 | Unit | Empty identity bypass |
| SESS-03 | UT-KA-SESS-003 | Unit | Concurrent lock contention |
| SESS-04 | IT-KA-SESS-001 | Integration | Lease orphan after crash |
| SESS-05 | UT-KA-SESS-004 | Unit | Inactivity timeout |
| SESS-06 | UT-KA-SESS-005 | Unit | Absolute timeout warning |
| SESS-07 | UT-KA-SESS-006 | Unit | MCP disconnect cleanup |
| SESS-08 | UT-KA-SESS-007 | Unit | Double-complete idempotent |
| SESS-09 | UT-KA-SESS-008 | Unit | Message on completed session |
| TAKE-01 | IT-KA-TAKE-001 | Integration | Takeover mid-LLM-turn |
| TAKE-02 | UT-KA-TAKE-001 | Unit | Takeover timing race |
| TAKE-03 | IT-KA-TAKE-002 | Integration | Rapid connect/disconnect |
| TAKE-04 | UT-KA-TAKE-002 | Unit | Resume identity (KA SA) |
| TAKE-05 | IT-KA-TAKE-003 | Integration | Combined conversation coherence |
| TAKE-06 | IT-KA-TAKE-004 | Integration | Auto-inject correctness |
| TAKE-07 | UT-KA-TAKE-003 | Unit | DS query failure during auto-inject |
| TAKE-08 | UT-KA-TAKE-004 | Unit | Concurrent takeover rejection |
| TAKE-09 | IT-KA-TAKE-005 | Integration | Timeout extension on takeover |
| TAKE-10 | UT-KA-TAKE-005 | Unit | Explicit takeover required |
| TAKE-11 | UT-KA-TAKE-006 | Unit | Timeout visibility T-10m/T-2m |
| TAKE-12 | UT-KA-TAKE-007 | Unit | Inactivity warning |
| TAKE-13 | UT-KA-TAKE-008 | Unit | NotificationBus ordering |
| TAKE-14 | UT-KA-TAKE-009 | Unit | NotificationBus slow consumer |
| AUD-01..07 | UT-KA-AUD-001..007 | Unit | Audit completeness |
| PROP-SESS-01..04 | UT-KA-SESS-009..012 | Unit | Session security properties |

### CP-4: Tool Completeness Gate (12 checks) → [CP-4_TOOL_COMPLETENESS_GATE.md](CP-4_TOOL_COMPLETENESS_GATE.md)

| Check ID | Test ID | Tier | Description |
|----------|---------|------|-------------|
| TOOL-01..07 | UT-KA-TOOL-001..007 | Unit | Adversarial tool scenarios |
| CONSIST-01..05 | UT-KA-TOOL-008..012 | Unit | Cross-tool consistency |

### CP-5: RELEASE GATE (29 checks) → [CP-5_RELEASE_GATE.md](CP-5_RELEASE_GATE.md)

| Check ID | Test ID | Tier | Description |
|----------|---------|------|-------------|
| HARM-01..06 | E2E-KA-HARM-001..006 | E2E | HARM scenarios |
| E2E-01..07 | E2E-KA-INT-001..007 | E2E | Full interactive flow |
| HELM-01..05 | UT-HELM-001..005 | Unit (helm template) | Helm validation |
| DOC-01..03 | N/A (doc review) | Review | Documentation exists |
| COMPAT-01..03 | E2E-KA-COMPAT-001..003 | E2E | Backward compatibility |
| UX-01..05 | E2E-KA-UX-001..005 | E2E | UX and operational readiness |

---

## 9. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test IDs | Status |
|-------|-------------|----------|------|----------|--------|
| BR-INTERACTIVE-001 | Interactive sessions via MCP | P0 | Unit+IT | UT-KA-SEC-011, IT-KA-SEC-002..006 | Pending |
| BR-INTERACTIVE-002 | User-scoped RBAC | P0 | Unit+IT+E2E | UT-KA-SEC-001..018, E2E-KA-INT-004 | Pending |
| BR-INTERACTIVE-003 | Audit attribution | P0 | Unit+IT | UT-KA-AUD-001..007, IT-KA-TAKE-004 | Pending |
| BR-INTERACTIVE-004 | Dynamic takeover | P0 | Unit+IT+E2E | UT-KA-TAKE-001..009, IT-KA-TAKE-001..005, E2E-KA-INT-001 | Pending |
| BR-INTERACTIVE-005 | Session lifecycle | P1 | Unit+IT | UT-KA-SESS-001..012, IT-KA-SESS-001 | Pending |
| BR-INTERACTIVE-006 | Cross-session visibility | P1 | Unit+IT | UT-KA-TAKE-005..009, IT-KA-TAKE-004 | Pending |
| BR-INTERACTIVE-007 | CRD observability | P2 | Unit | UT-KA-INT-001..013 | Pending |
| BR-INTERACTIVE-008 | Graceful degradation | P2 | Unit+E2E | UT-KA-SEC-011..012, UT-KA-TAKE-003, E2E-KA-INT-002 | Pending |

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Mock LLM, mock K8s (fake.NewClientBuilder), mock DS client
- **Location**: `test/unit/kubernautagent/mcp/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: httptest server, envtest (K8s API), mock LLM ConfigMap
- **Location**: `test/integration/kubernautagent/mcp/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD with `Label("interactive")`
- **Infrastructure**: Kind cluster, mock LLM, DS, full Helm deployment
- **Location**: `test/e2e/kubernautagent/`
- **Timeout**: 20m (separate from existing 15m autonomous E2E)

---

## 11. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/... -ginkgo.v

# E2E tests (interactive only)
make test-e2e-kubernautagent-interactive

# Full suite
make test-e2e-kubernautagent

# Coverage
go test ./test/unit/kubernautagent/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E 'auth|mcp|impersonate'
```

---

## 12. Dependencies & Schedule

### 12.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available |
|------------|------|--------|-------------------------|
| go-sdk v1.5.0 (`github.com/modelcontextprotocol/go-sdk`) | Library | Available (verified) | All MCP tests blocked |
| kubernaut-operator#26 | Cross-repo | Open | E2E impersonation tests blocked (use Helm RBAC) |

### 12.2 Execution Order

1. **PR0** (this plan + design docs) → CP-0
2. **PR1** (CRD) → CP-1
3. **PR2** (MCP transport + auth) → CP-2
4. **PR3** (investigate tool + takeover) → CP-3
5. **PR4 + PR5** (enrich + select_workflow) → CP-4
6. **PR6** (integration + Helm + E2E) → CP-5

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-29 | Initial test plan. 138 checkpoint checks mapped. |
| 2.0 | 2026-04-29 | IEEE 829 compliance: added suspension/resumption criteria, test deliverables, responsibilities, per-checkpoint detail files with full test case specifications. |
