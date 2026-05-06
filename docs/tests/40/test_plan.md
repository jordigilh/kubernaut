# Test Plan: OpenAPI Spec and Protocol Version Declaration

**Test Plan Identifier:** TP-AF-040
**Issue:** [#40](https://github.com/jordigilh/kubernaut-apifrontend/issues/40)
**Version:** 1.0
**Date:** 2026-05-06
**Status:** Draft

---

## 1. Introduction

This test plan validates the OpenAPI specification for REST endpoints, the `make validate-openapi` target, CI integration, and protocol version declaration in the Agent Card.

### 1.1 Scope

- Author `api/openapi/apifrontend-v1.yaml` (OpenAPI 3.1) covering all 6 HTTP paths
- `make validate-openapi` Makefile target using `vacuum` CLI
- CI workflow step to validate OpenAPI spec on every PR
- Agent Card `protocolVersion` field set to A2A protocol version
- Documentation of MCP and A2A protocol versions

### 1.2 References

- Issue #40: OpenAPI spec for REST endpoints and protocol version declaration
- `internal/handler/router.go` — 6 registered routes
- `internal/handler/agentcard.go` — Agent Card struct (missing `protocolVersion`)
- `a2a-go v0.3.13` — `AgentCard.ProtocolVersion` field confirmed
- MCP Streamable HTTP spec version: `2025-03-26`
- A2A spec version: `0.3.0`

### 1.3 Definitions

| Term | Definition |
|------|-----------|
| OpenAPI 3.1 | API description standard (JSON Schema compatible) |
| vacuum | pb33f OpenAPI linter/validator CLI tool |
| protocolVersion | A2A Agent Card field declaring supported protocol version |

---

## 2. Test Items

| Item | Location | Source |
|------|----------|--------|
| `apifrontend-v1.yaml` | `api/openapi/` | New |
| `validate-openapi` target | `Makefile` | New |
| CI validation step | `.github/workflows/ci.yml` | Modified |
| `agentCard.ProtocolVersion` | `internal/handler/agentcard.go` | Modified |
| Agent Card test | `internal/handler/agentcard_test.go` | Modified |

---

## 3. Features to Be Tested

### 3.1 Business Acceptance Criteria

| ID | Criterion | Testable |
|----|-----------|----------|
| BAC-1 | `api/openapi/apifrontend-v1.yaml` authored | Yes (file exists, validates) |
| BAC-2 | Spec covers all REST endpoints with request/response schemas | Yes |
| BAC-3 | `make validate-openapi` validates spec syntax | Yes |
| BAC-4 | CI runs OpenAPI validation on every PR | Yes (workflow change) |
| BAC-5 | MCP and A2A protocol versions documented | Yes |
| BAC-6 | Agent Card includes protocol version metadata | Yes |

### 3.2 Features by Tier

#### Tier 1: OpenAPI Spec Completeness

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-40.1 | Spec declares OpenAPI 3.1 | `openapi: "3.1.0"` in YAML |
| F-40.2 | GET /healthz documented | Path with 200 response schema |
| F-40.3 | GET /readyz documented | Path with 200 and 503 responses |
| F-40.4 | GET /metrics documented | Path with 200 response (text/plain) |
| F-40.5 | GET /.well-known/agent-card.json documented | Path with 200 JSON schema |
| F-40.6 | POST /a2a/invoke documented | Path with auth, request/response schemas |
| F-40.7 | POST /mcp documented | Path with auth, 501 when disabled |

#### Tier 2: Makefile and CI

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-40.8 | `make validate-openapi` runs vacuum lint | Exit 0 on valid spec |
| F-40.9 | `make validate-openapi` fails on invalid spec | Exit non-zero on broken YAML |
| F-40.10 | CI workflow includes openapi validation step | Job runs after checkout |

#### Tier 3: Protocol Version in Agent Card

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-40.11 | Agent Card JSON includes `protocolVersion` field | Field present in response |
| F-40.12 | protocolVersion value is "0.3.0" | Matches a2a.Version constant |
| F-40.13 | Agent Card test validates protocolVersion | Test assertion added |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| OpenAPI code generation | Not using generated code; spec is documentation |
| Runtime spec serving | Spec is static file, not served by the API |
| vacuum CLI installation | CI downloads binary; local dev uses `go install` |

---

## 5. Approach

### 5.1 Test Methodology

This issue is primarily documentation and tooling. Testing approach:
- **Spec validation**: `vacuum lint` in Makefile target
- **Agent Card**: Add protocolVersion field, test in existing agentcard_test.go
- **CI**: Verify workflow YAML is syntactically correct

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Static | OpenAPI spec syntax | `vacuum lint` |
| Unit | Agent Card protocolVersion | Ginkgo/Gomega (existing test suite) |
| CI | End-to-end validation | GitHub Actions |

---

## 6. Test Cases

### 6.1 OpenAPI Spec Validation (3 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-040-001 | `make validate-openapi` exits 0 on current spec | P0 | BAC-3 |
| UT-AF-040-002 | Spec contains all 6 paths (/healthz, /readyz, /metrics, /.well-known/agent-card.json, /a2a/invoke, /mcp) | P0 | BAC-2 |
| UT-AF-040-003 | Spec version is OpenAPI 3.1.0 | P0 | BAC-1 |

### 6.2 Agent Card Protocol Version (3 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-040-004 | Agent Card JSON response includes `protocolVersion` field | P0 | BAC-6 |
| UT-AF-040-005 | protocolVersion value equals "0.3.0" | P0 | BAC-6 |
| UT-AF-040-006 | Agent Card test validates protocolVersion is non-empty | P0 | BAC-6 |

### 6.3 CI Integration (2 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-040-007 | CI workflow YAML contains openapi validation job/step | P0 | BAC-4 |
| UT-AF-040-008 | MCP and A2A versions documented in spec info section | P1 | BAC-5 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass

- `make validate-openapi` exits 0
- Agent Card test passes with protocolVersion assertion
- CI workflow YAML is valid (actionlint or manual review)
- Spec covers all 6 paths with correct methods

### 7.2 Fail

- vacuum reports errors on the spec
- Agent Card response missing protocolVersion
- CI step missing from workflow

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| OpenAPI spec | `api/openapi/apifrontend-v1.yaml` |
| Makefile target | `Makefile` (validate-openapi) |
| CI step | `.github/workflows/ci.yml` |
| Agent Card test update | `internal/handler/agentcard_test.go` |
| This test plan | `docs/tests/40/test_plan.md` |

---

## 9. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-1 | UT-AF-040-003 | 1 |
| BAC-2 | UT-AF-040-002 | 1 |
| BAC-3 | UT-AF-040-001 | 1 |
| BAC-4 | UT-AF-040-007 | 1 |
| BAC-5 | UT-AF-040-008 | 1 |
| BAC-6 | UT-AF-040-004, 005, 006 | 3 |
