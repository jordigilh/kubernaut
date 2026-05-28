# Test Plan — #1307: AF Agent Tool Ordering Guard

**IEEE 829 Compliant** | **Issue**: [#1307](https://github.com/jordigilh/kubernaut/issues/1307)

## 1. Test Plan Identifier

TP-1307-TOOL-ORDERING-GUARD

## 2. Introduction

The AF ADK agent (LLM) calls KA MCP tools in the wrong order. The system prompt
defines a 4-phase journey but nothing enforces it. The LLM sometimes jumps
directly to Phase 2 (`kubernaut_discover_workflows`) without calling
`kubernaut_takeover` first, causing `not_driving` errors.

This test plan covers: (A) a `BeforeToolCallback` that blocks MCP-dependent
tools unless `takeover` has succeeded, (B) prompt fixes to document `takeover`
in the 4-phase journey, and (C) tool description updates.

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `newPhaseGuard` | `pkg/apifrontend/agent/phase_guard.go` | BeforeToolCallback enforcing tool order |
| Phase state tracking | `pkg/apifrontend/agent/phase_guard.go` | AfterToolCallback recording phase transitions |
| Prompt update | `pkg/apifrontend/agent/prompt.txt` | Document takeover in 4-phase journey |
| Tool descriptions | `pkg/apifrontend/tools/ka_tools.go` | Prerequisite docs in tool descriptions |

## 4. Features to Be Tested

- BR-ORDERING-001: `discover_workflows` blocked without prior `takeover`
- BR-ORDERING-002: `select_workflow` blocked without prior `takeover`
- BR-ORDERING-003: `message`/`status`/`complete`/`cancel` blocked without prior `takeover`
- BR-ORDERING-004: `takeover` always allowed (entry point for interactive flow)
- BR-ORDERING-005: `kubernaut_investigate` always allowed (Phase 1, merged tool)
- BR-ORDERING-006: Guard returns LLM-guiding error message (not generic deny)
- BR-ORDERING-007: Phase state resets across A2A sessions
- BR-PROMPT-001: Prompt includes `kubernaut_takeover` in Phase 2 prerequisites
- BR-PROMPT-002: Tool descriptions state prerequisites

## 5. Features Not Tested

- LLM actually following the corrected prompt (non-deterministic)
- MCP session persistence (#1306 handles this independently)
- RBAC enforcement (existing RBAC guard tests)

## 6. Approach

Testing Pyramid Invariant: UT proves logic. IT proves wiring. E2E proves the journey.

### 6.1 Unit Tests (pkg/apifrontend/agent/)

| ID | Scenario | Asserts |
|----|----------|---------|
| UT-AF-1307-001 | discover_workflows blocked without takeover | returns `{error: "...takeover..."}`, not `(nil, nil)` |
| UT-AF-1307-002 | select_workflow blocked without takeover | same deny pattern |
| UT-AF-1307-003 | message blocked without takeover | same deny pattern |
| UT-AF-1307-004 | complete blocked without takeover | same deny pattern |
| UT-AF-1307-005 | cancel blocked without takeover | same deny pattern |
| UT-AF-1307-006 | status blocked without takeover | same deny pattern |
| UT-AF-1307-007 | takeover always allowed | returns `(nil, nil)` |
| UT-AF-1307-008 | kubernaut_investigate always allowed | returns `(nil, nil)` |
| UT-AF-1307-009 | kubectl_get always allowed (not MCP-gated) | returns `(nil, nil)` |
| UT-AF-1307-010 | After takeover succeeds, discover_workflows allowed | AfterTool sets state; BeforeTool reads state → allow |
| UT-AF-1307-011 | Error message guides LLM to call takeover first | error string contains "kubernaut_takeover" |
| UT-AF-1307-012 | reconnect allowed without takeover (re-entry point) | returns `(nil, nil)` |

### 6.2 Integration Tests (cmd/apifrontend/)

| ID | Scenario | Asserts |
|----|----------|---------|
| IT-AF-1307-001 | Phase guard wired in NewRootAgent callback chain | BeforeToolCallbacks includes phase guard |
| IT-AF-1307-002 | Phase guard runs after RBAC guard | RBAC deny emits before phase guard check |

### 6.3 Prompt / Description Validation (static)

| ID | Scenario | Asserts |
|----|----------|---------|
| UT-AF-1307-020 | prompt.txt Phase 2 mentions takeover prerequisite | string search in embedded prompt |
| UT-AF-1307-021 | discover_workflows description mentions prerequisite | tool Description contains "takeover" |
| UT-AF-1307-022 | select_workflow description mentions prerequisite | tool Description contains "takeover" |

## 7. Pass/Fail Criteria

- All UT/IT tests pass with >=80% coverage of `phase_guard.go`
- No regression in existing root_test.go or callback tests
- Prompt change does not break E2E mock-LLM scenarios
- Zero data races under `go test -race`

## 8. Suspension / Resumption

Suspend if ADK `session.State` API changes. Resume after migration.

## 9. Environmental Needs

- Go 1.23+, ADK v1.2.0
- Mock `tool.Context` with `session.State` for UT
- Kind cluster for E2E (existing CI infrastructure)
