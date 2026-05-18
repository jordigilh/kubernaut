# Test Plan: Protocol Conformance Tests — MCP, A2A, and Agent Card Schema

**Test Plan Identifier:** TP-AF-042
**Issue:** [#42](https://github.com/jordigilh/kubernaut-apifrontend/issues/42)
**Version:** 1.0
**Date:** 2026-05-06
**Status:** Draft

---

## 1. Introduction

This test plan validates protocol conformance for MCP Streamable HTTP, A2A JSON-RPC 2.0, and Agent Card JSON Schema. Tests ensure interoperability with RHDH, kagenti, and third-party clients.

### 1.1 Scope

- MCP: `tools/list` returns all 14 tools with correct schemas
- MCP: `tools/call` returns spec-compliant responses (stub: not-yet-wired)
- MCP: Error codes match spec (-32600 invalid request, -32601 method not found, -32602 invalid params)
- MCP: Session management (initialize required before tools/list)
- A2A: Agent Card at `/.well-known/agent-card.json` validates against schema
- A2A: Agent Card has all required fields (name, url, skills, capabilities, protocolVersion)
- A2A: `message/send` creates task and returns task state

### 1.2 References

- Issue #42: Protocol conformance tests
- MCP Streamable HTTP spec (2025-03-26)
- `modelcontextprotocol/go-sdk v1.6.0` conformance test data at `mcp/testdata/conformance/server/tools.txtar`
- A2A Protocol v0.3.x via `a2aproject/a2a-go v0.3.13`
- `internal/handler/mcp.go` — MCP handler
- `internal/handler/agentcard.go` — Agent Card handler
- `internal/launcher/launcher.go` — A2A handler

### 1.3 Definitions

| Term | Definition |
|------|-----------|
| JSON-RPC 2.0 | Remote procedure call protocol encoded in JSON |
| Streamable HTTP | MCP transport: POST JSON-RPC, response as JSON or SSE |
| tools/list | MCP method returning available tool definitions |
| tools/call | MCP method invoking a specific tool |
| message/send | A2A method to send a message to an agent |

---

## 2. Test Items

| Item | Package | Source |
|------|---------|--------|
| MCP tools/list conformance | `internal/handler` | New tests |
| MCP error codes | `internal/handler` | New tests |
| MCP session lifecycle | `internal/handler` | New tests |
| A2A Agent Card schema | `internal/handler` | New tests |
| A2A message/send lifecycle | `internal/launcher` | New tests |

---

## 3. Features to Be Tested

### 3.1 Business Acceptance Criteria

| ID | Criterion | Testable |
|----|-----------|----------|
| BAC-1 | MCP tool discovery returns all 14 tools | Yes |
| BAC-2 | MCP tool calls return spec-compliant responses | Yes |
| BAC-3 | A2A agent card passes JSON Schema validation | Yes |
| BAC-4 | Error responses match protocol-specified error codes | Yes |
| BAC-5 | Tests run in CI as part of unit tier | Yes |

### 3.2 Features by Tier

#### Tier 1: MCP tools/list Conformance

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-42.1 | tools/list returns 14 tools | Response `result.tools` length == 14 |
| F-42.2 | Each tool has name with kubernaut_ prefix | All names match prefix |
| F-42.3 | Each tool has non-empty description | No empty descriptions |
| F-42.4 | Each tool has inputSchema (JSON Schema object) | type == "object" |
| F-42.5 | Response is valid JSON-RPC 2.0 | Has jsonrpc, id, result fields |

#### Tier 2: MCP Error Codes

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-42.6 | Missing params in tools/call returns -32600 | error.code == -32600 |
| F-42.7 | Unknown method returns -32601 | error.code == -32601 |
| F-42.8 | tools/list before initialize returns error | error with lifecycle message |
| F-42.9 | Invalid JSON body returns -32700 | Parse error |

#### Tier 3: A2A Agent Card Schema

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-42.10 | Agent Card has required `name` field | Non-empty string |
| F-42.11 | Agent Card has required `url` field | Valid URL |
| F-42.12 | Agent Card has `skills` array | Length == 14, matches tools |
| F-42.13 | Agent Card has `capabilities` object | streaming, stateTransitionHistory |
| F-42.14 | Agent Card has `protocolVersion` | Value "0.3.0" |
| F-42.15 | Agent Card has `authentication.schemes` | Contains bearer |

#### Tier 4: A2A message/send

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-42.16 | POST /a2a/invoke with message/send returns JSON-RPC response | Status 200, valid JSON-RPC |
| F-42.17 | Response contains task state | Result has task object |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| MCP SSE streaming (subscribe) | Requires async stream handling; deferred |
| A2A tasks/get, tasks/cancel | Require full session lifecycle; deferred |
| MCP session resumption | Complex state management; separate issue |
| A2A push notifications | Not supported per capabilities |

---

## 5. Approach

### 5.1 Test Methodology

TDD in three phases. Tests use `httptest.NewRequest` + `httptest.NewRecorder` for MCP, and the existing A2A test pattern from `launcher_test.go`.

### 5.2 MCP Test Pattern

```go
// 1. Initialize session
initReq := jsonRPC("initialize", initParams)
// POST with Accept: application/json
// 2. Send notifications/initialized
// 3. Send tools/list
toolsReq := jsonRPC("tools/list", nil)
// Parse JSON-RPC response, assert result.tools
```

The MCP SDK `StreamableHTTPHandler` responds with `application/json` when the client sends `Accept: application/json` (confirmed from SDK conformance tests).

### 5.3 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | MCP tools/list, error codes | httptest, json.Unmarshal |
| Unit | Agent Card schema | httptest, json.Unmarshal, field assertions |
| Integration | A2A message/send round-trip | httptest.Server, launcher.NewA2AHandler |

---

## 6. Test Cases

### 6.1 MCP tools/list (5 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-042-001 | tools/list returns JSON-RPC response with 14 tools | P0 | BAC-1 |
| UT-AF-042-002 | All tool names have kubernaut_ prefix | P0 | BAC-1 |
| UT-AF-042-003 | All tools have non-empty description | P0 | BAC-1 |
| UT-AF-042-004 | All tools have inputSchema with type "object" | P0 | BAC-2 |
| UT-AF-042-005 | Response has jsonrpc "2.0" and matching id | P0 | BAC-2 |

### 6.2 MCP Error Codes (4 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-042-006 | tools/call without params returns error code -32600 | P0 | BAC-4 |
| UT-AF-042-007 | Unknown method returns error code -32601 | P0 | BAC-4 |
| UT-AF-042-008 | tools/list before initialize returns lifecycle error | P0 | BAC-4 |
| UT-AF-042-009 | Malformed JSON body returns error code -32700 | P0 | BAC-4 |

### 6.3 Agent Card Schema (6 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-042-010 | Agent Card response has name field | P0 | BAC-3 |
| UT-AF-042-011 | Agent Card response has url field | P0 | BAC-3 |
| UT-AF-042-012 | Agent Card skills array has 14 entries matching MCP tools | P0 | BAC-3 |
| UT-AF-042-013 | Agent Card capabilities includes streaming=true | P0 | BAC-3 |
| UT-AF-042-014 | Agent Card protocolVersion is "0.3.0" | P0 | BAC-3 |
| UT-AF-042-015 | Agent Card authentication.schemes includes bearer | P0 | BAC-3 |

### 6.4 A2A message/send (2 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-042-016 | POST /a2a/invoke with message/send returns 200 with JSON-RPC result | P0 | BAC-2 |
| UT-AF-042-017 | Response result contains task object with id and status | P1 | BAC-2 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass

- All 17 conformance tests pass with `-race`
- MCP tools/list returns exactly 14 tools
- All error codes match MCP spec
- Agent Card includes protocolVersion
- Coverage >= 80% on handler conformance paths

### 7.2 Fail

- tools/list returns wrong tool count
- Error code mismatch with spec
- Agent Card missing required field
- Test flakiness due to session state

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| MCP conformance tests | `internal/handler/mcp_conformance_test.go` |
| Agent Card schema tests | `internal/handler/agentcard_test.go` (extended) |
| A2A conformance tests | `internal/launcher/launcher_conformance_test.go` |
| This test plan | `docs/tests/42/test_plan.md` |

---

## 9. Implementation Phases

### Phase 1: TDD Red

Write 17 failing tests. MCP tests need the initialize+tools/list flow. Agent Card tests extend existing suite. Create minimal stubs if needed.

### Phase 2: TDD Green

- Add `protocolVersion` field to `agentCard` struct and set to "0.3.0"
- Fix any MCP conformance gaps (tools/list should already work via SDK)
- Ensure error codes propagate correctly from SDK

### Phase 3: TDD Refactor

100 Go Mistakes checklist, lint, coverage verification, 8-persona audit.

---

## 10. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-1 | UT-AF-042-001 to 003 | 3 |
| BAC-2 | UT-AF-042-004, 005, 016, 017 | 4 |
| BAC-3 | UT-AF-042-010 to 015 | 6 |
| BAC-4 | UT-AF-042-006 to 009 | 4 |
| BAC-5 | All (run in CI) | 17 |
