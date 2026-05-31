# Test Plan — #1307: AF Agent Tool Ordering Guard

**IEEE 829 Compliant** | **Issue**: [#1307](https://github.com/jordigilh/kubernaut/issues/1307)

## 1. Test Plan Identifier

TP-1307-TOOL-ORDERING-GUARD

## 2. Introduction

The AF ADK agent (LLM) calls KA MCP tools in the wrong order. The system prompt
defines a 4-phase journey but nothing enforces it. The LLM sometimes jumps
directly to Phase 2 (`kubernaut_discover_workflows`) without calling
`kubernaut_investigate` first, causing `not_driving` errors.

> **Note (#1332):** `kubernaut_takeover` was consolidated into `kubernaut_investigate`.
> All references to `kubernaut_takeover` in this plan now refer to `kubernaut_investigate`.

This test plan covers: (A) a `BeforeToolCallback` that blocks MCP-dependent
tools unless `kubernaut_investigate` has succeeded, (B) prompt fixes to document
the investigate prerequisite in the 4-phase journey, and (C) tool description updates.

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `newPhaseGuard` | `pkg/apifrontend/agent/phase_guard.go` | BeforeToolCallback enforcing tool order |
| Phase state tracking | `pkg/apifrontend/agent/phase_guard.go` | AfterToolCallback recording phase transitions |
| Prompt update | `pkg/apifrontend/agent/prompt.txt` | Document investigate prerequisite in 4-phase journey |
| Tool descriptions | `pkg/apifrontend/tools/ka_tools.go` | Prerequisite docs in tool descriptions |

## 4. Features to Be Tested

- BR-ORDERING-001: `discover_workflows` blocked without prior `kubernaut_investigate`
- BR-ORDERING-002: `select_workflow` blocked without prior `kubernaut_investigate`
- BR-ORDERING-003: `message`/`status`/`complete`/`cancel` blocked without prior `kubernaut_investigate`
- BR-ORDERING-004: `kubernaut_investigate` always allowed (entry point for interactive flow, consolidates takeover)
- BR-ORDERING-005: (Removed — merged into BR-ORDERING-004 per #1332)
- BR-ORDERING-006: Guard returns LLM-guiding error message (not generic deny)
- BR-ORDERING-007: Phase state resets across A2A sessions
- BR-PROMPT-001: Prompt includes `kubernaut_investigate` in Phase 2 prerequisites
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
| UT-AF-1307-001 | discover_workflows blocked without investigate | returns `{error: "...investigate..."}`, not `(nil, nil)` |
| UT-AF-1307-002 | select_workflow blocked without investigate | same deny pattern |
| UT-AF-1307-003 | message blocked without investigate | same deny pattern |
| UT-AF-1307-004 | complete blocked without investigate | same deny pattern |
| UT-AF-1307-005 | cancel blocked without investigate | same deny pattern |
| UT-AF-1307-006 | status blocked without investigate | same deny pattern |
| UT-AF-1307-007 | kubernaut_investigate always allowed (entry point, consolidates takeover per #1332) | returns `(nil, nil)` |
| UT-AF-1307-008 | (merged into 007 per #1332) | — |
| UT-AF-1307-009 | kubectl_get always allowed (not MCP-gated) | returns `(nil, nil)` |
| UT-AF-1307-010 | After investigate succeeds, discover_workflows allowed | AfterTool sets state; BeforeTool reads state → allow |
| UT-AF-1307-011 | Error message guides LLM to call investigate first | error string contains "kubernaut_investigate" |
| UT-AF-1307-012 | reconnect allowed without investigate (re-entry point) | returns `(nil, nil)` |

### 6.2 Integration Tests (cmd/apifrontend/)

| ID | Scenario | Asserts |
|----|----------|---------|
| IT-AF-1307-001 | Phase guard wired in NewRootAgent callback chain | BeforeToolCallbacks includes phase guard |
| IT-AF-1307-002 | Phase guard runs after RBAC guard | RBAC deny emits before phase guard check |

### 6.3 Prompt / Description Validation (static)

| ID | Scenario | Asserts |
|----|----------|---------|
| UT-AF-1307-020 | prompt.txt Phase 2 mentions investigate prerequisite | string search in embedded prompt |
| UT-AF-1307-021 | discover_workflows description mentions prerequisite | tool Description contains "investigate" |
| UT-AF-1307-022 | select_workflow description mentions prerequisite | tool Description contains "investigate" |

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
