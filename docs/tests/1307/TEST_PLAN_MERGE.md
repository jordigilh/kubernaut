# Test Plan — #1307 Addendum: Merge Investigation Tools into `kubernaut_investigate`

**IEEE 829 Compliant** | **Issue**: [#1307](https://github.com/jordigilh/kubernaut/issues/1307) | **Parent**: TP-1307-TOOL-ORDERING-GUARD

## 1. Test Plan Identifier

TP-1307-MERGE-INVESTIGATE

## 2. Introduction

This addendum extends TP-1307-TOOL-ORDERING-GUARD with the "Option B" solution:
merge `kubernaut_start_investigation`, `kubernaut_stream_investigation`, and
`kubernaut_poll_investigation` into a single `kubernaut_investigate` tool.

**Rationale**: The LLM agent repeatedly fails to call the three investigation tools
in the correct sequence (start -> stream -> poll-fallback). A single tool eliminates
the sequencing problem entirely by handling the full lifecycle internally:
Analyze -> StreamEvents -> poll-fallback.

**Business Requirements**:
- BR-ORDERING-001 through BR-ORDERING-007 (original #1307 — tool ordering enforcement)
- BR-INVESTIGATE-001: Single-call investigation initiation (new resource)
- BR-INVESTIGATE-002: Single-call investigation resume (existing session_id)
- BR-INVESTIGATE-003: Single-call investigation status poll (existing session_id, completed)
- BR-INVESTIGATE-004: Audit emission preserved for all investigation paths (FedRAMP AU-2, AU-12)
- BR-INVESTIGATE-005: SSE bridge streaming preserved for real-time UI updates (FedRAMP SI-4)
- BR-INVESTIGATE-006: Tool timeout of 15m applied to merged tool (operational safety)

**FedRAMP Control Mapping**:
| Control | Objective | Test Coverage |
|---------|-----------|---------------|
| AU-2    | Auditable events defined | All investigation paths emit audit events |
| AU-3    | Audit record content | Audit events contain session_id, correlation_id, result_type |
| AU-12   | Audit generation | Audit emitter called for start, complete, and failed paths |
| SI-4    | Information system monitoring | SSE bridge emits reasoning and tool events in real-time |
| SC-4    | Information in shared resources | Session IDs are not leaked across user contexts |
| SC-7    | Boundary protection | Tool validates session ownership before streaming |
| CM-3    | Configuration change control | Prompt, config, and deploy artifacts updated atomically |

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `HandleInvestigation` | `pkg/apifrontend/tools/ka_investigate.go` | Merged handler: Analyze + StreamEvents + poll fallback |
| `NewInvestigateTool` | `pkg/apifrontend/tools/ka_investigate.go` | ADK tool constructor (`kubernaut_investigate`) |
| Tool wiring (ADK) | `pkg/apifrontend/agent/root.go` | Single entry replacing 3 constructors |
| Tool wiring (MCP) | `pkg/apifrontend/handler/mcp_bridge.go` | Single `registerTool` replacing 3 calls |
| Tool manifest | `pkg/apifrontend/handler/mcptools.go` | Single entry replacing 3 entries |
| Part converter | `pkg/apifrontend/launcher/part_converter.go` | Updated tool name maps + streaming suppression |
| Config timeout | `pkg/apifrontend/config/config.go` | `kubernaut_investigate: 15m` |
| Agent prompt | `pkg/apifrontend/agent/prompt.txt` | Simplified Phase 1 + Fix journey |
| Prompt builder | `pkg/apifrontend/agent/prompt.go` | Updated tool usage rules |

## 4. Features to Be Tested

### 4.1 Core Handler Logic (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-INV-001 | BR-INVESTIGATE-001 | AU-2, AU-12 | New investigation (no session_id) | Calls `Analyze()`, then `StreamEvents()`, returns summary; audit emitted |
| UT-AF-INV-002 | BR-INVESTIGATE-002 | SI-4 | Resume in-flight (session_id present, status=in_progress) | Calls `Status()`, then `StreamEvents()`; bridge emits events |
| UT-AF-INV-003 | BR-INVESTIGATE-003 | AU-3 | Poll completed (session_id present, status=completed) | Calls `Status()`, then `Result()`; audit contains result_type=rca_complete |
| UT-AF-INV-004 | BR-INVESTIGATE-003 | AU-12 | Poll failed (session_id present, status=failed) | Returns error status; audit contains result_type=rca_failed |
| UT-AF-INV-005 | BR-INVESTIGATE-001 | SC-7 | Missing required args (no namespace, no name) | Returns validation error, no KA calls |
| UT-AF-INV-006 | BR-INVESTIGATE-002 | SC-7 | Invalid session_id (Status returns error) | Returns clear error guiding user |
| UT-AF-INV-007 | BR-INVESTIGATE-005 | SI-4 | Bridge emits reasoning_delta and token_delta | `emitViaBridge` called for both event types |
| UT-AF-INV-008 | BR-INVESTIGATE-005 | SI-4 | Bridge emits tool_call events | `emitViaBridge` called with `[Tool: name]` format |
| UT-AF-INV-009 | BR-INVESTIGATE-001 | AU-2 | Stream disconnects mid-investigation | Returns status=disconnected with partial events |
| UT-AF-INV-010 | BR-INVESTIGATE-001 | — | Context cancelled during stream | Returns status=cancelled |
| UT-AF-INV-011 | BR-INVESTIGATE-003 | — | Poll cancelled session | Returns status=cancelled |
| UT-AF-INV-012 | BR-INVESTIGATE-001 | AU-2 | Analyze returns error | Returns wrapped error, no stream attempted |

### 4.2 Constructor and Tool Metadata (UT tier)

| ID | BR | Scenario | Asserts |
|----|-----|----------|---------|
| UT-AF-INV-020 | — | Constructor creates tool | `Name() == "kubernaut_investigate"` |
| UT-AF-INV-021 | — | Constructor description | Description mentions "investigation" |

### 4.3 Part Converter (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-INV-030 | BR-INVESTIGATE-005 | SI-4 | FunctionCall `kubernaut_investigate` produces status text | Text contains "Starting investigation" or "Streaming" |
| UT-AF-INV-031 | BR-INVESTIGATE-005 | SI-4 | FunctionResponse `kubernaut_investigate` summarized | Summarizer extracts summary field |
| UT-AF-INV-032 | BR-INVESTIGATE-005 | SI-4 | Streaming converter suppresses `kubernaut_investigate` FunctionResponse | Returns nil (bridge already delivered) |

### 4.4 Wiring Tests (WT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| WT-AF-INV-040 | CM-3 | CM-3 | Prompt Phase 1 uses `kubernaut_investigate` | `strings.Contains(prompt, "kubernaut_investigate")` |
| WT-AF-INV-041 | CM-3 | CM-3 | Prompt does NOT reference old tool names | `!strings.Contains(prompt, "kubernaut_start_investigation")` etc. |
| WT-AF-INV-042 | CM-3 | CM-3 | Prompt Fix journey uses single investigate call | Journey string contains `kubernaut_investigate` |
| WT-AF-INV-043 | — | — | Phase guard allows `kubernaut_investigate` | Returns `(nil, nil)` without takeover |
| WT-AF-INV-044 | — | — | ADK tool list contains `kubernaut_investigate` | Tool name present in `buildToolList()` output |
| WT-AF-INV-045 | — | — | ADK tool list does NOT contain old 3 tools | Old names absent |
| WT-AF-INV-046 | BR-INVESTIGATE-006 | — | Config timeout is 15m for `kubernaut_investigate` | `GetToolTimeoutFor("kubernaut_investigate") == 15m` |

### 4.5 Integration Tests (IT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-AF-INV-050 | BR-INVESTIGATE-001 | AU-2 | MCP bridge exposes `kubernaut_investigate` | Tool appears in `tools/list` |
| IT-AF-INV-051 | BR-INVESTIGATE-001 | AU-2 | MCP bridge does NOT expose old 3 tools | Old names absent from `tools/list` |
| IT-AF-INV-052 | BR-INVESTIGATE-004 | AU-12 | MCP tool call emits audit event | Audit log contains `tool_name=kubernaut_investigate` |
| IT-AF-INV-053 | BR-INVESTIGATE-006 | — | MCP tool call uses 15m timeout | Context deadline matches expected |
| IT-AF-INV-054 | — | SC-7 | RBAC denies `kubernaut_investigate` for unauthorized role | SAR returns denied |

## 5. Features Not Tested

- LLM actually calling `kubernaut_investigate` correctly (non-deterministic; prompt testing covers static compliance)
- KA backend investigation quality (KA-side testing)
- MCP session persistence (#1306 — independent effort)
- Backward compatibility with old tool names (no alias period — clean break)

## 6. Approach

**Testing Pyramid Invariant**: UT proves logic. IT proves wiring. E2E proves the journey.

### 6.1 TDD Red Phase
Write failing tests for all scenarios in sections 4.1-4.5 before implementing any production code.
Tests reference `HandleInvestigation` and `NewInvestigateTool` which do not yet exist.

### 6.2 TDD Green Phase
Implement minimal production code to make all tests pass:
1. `ka_investigate.go` — merged handler + constructor
2. Wire in `root.go`, `mcp_bridge.go`, `mcptools.go`
3. Update `part_converter.go`, `config.go`, `prompt.txt`, `prompt.go`
4. Update deploy artifacts: `config.yaml`, `values.yaml`, `e2e-user-rbac.yaml`, `mock-llm.yaml`
5. Update all test files referencing old tool names

### 6.3 TDD Refactor Phase
1. Validate against 100-go-mistakes checklist
2. Remove dead code from `ka_tools.go` and `ka_stream.go`
3. Delete obsolete test files
4. Ensure no duplication between old and new handler logic

## 7. Pass/Fail Criteria

- All UT/WT/IT tests pass with >=80% coverage of `ka_investigate.go`
- `go build ./...` succeeds
- `go test -race ./...` succeeds (zero data races)
- `golangci-lint run --timeout=5m` reports zero new issues
- `rg 'kubernaut_start_investigation|kubernaut_stream_investigation|kubernaut_poll_investigation' --type go` returns zero hits (excluding docs/)
- No regression in existing test suites

## 8. Suspension / Resumption

Suspend if:
- KA SSE API changes (StreamEvents contract)
- ADK `functiontool` API changes
- A2A EventBridge API changes

Resume after updating handler to match new API contracts.

## 9. Environmental Needs

- Go 1.23+, ADK v1.2.0
- Mock `ka.Client` with controllable `Analyze()`, `Status()`, `Result()`, `StreamEvents()` responses
- Mock `audit.Emitter` for capturing audit events
- Mock HTTP server for KA SSE stream simulation
- Kind cluster for E2E (existing CI infrastructure)

## 10. Traceability Matrix

| Business Requirement | Test IDs |
|---------------------|----------|
| BR-INVESTIGATE-001 | UT-AF-INV-001, UT-AF-INV-005, UT-AF-INV-009, UT-AF-INV-010, UT-AF-INV-012, IT-AF-INV-050 |
| BR-INVESTIGATE-002 | UT-AF-INV-002, UT-AF-INV-006 |
| BR-INVESTIGATE-003 | UT-AF-INV-003, UT-AF-INV-004, UT-AF-INV-011 |
| BR-INVESTIGATE-004 | UT-AF-INV-003, UT-AF-INV-004, IT-AF-INV-052 |
| BR-INVESTIGATE-005 | UT-AF-INV-007, UT-AF-INV-008, UT-AF-INV-030, UT-AF-INV-031, UT-AF-INV-032 |
| BR-INVESTIGATE-006 | WT-AF-INV-046, IT-AF-INV-053 |
