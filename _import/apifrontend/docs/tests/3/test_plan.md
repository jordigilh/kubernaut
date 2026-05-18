# Test Plan: MCP Tool Surface (Issue #3)

**Test Plan Identifier:** TP-AF-003
**Version:** 1.0
**Date:** 2026-05-04
**Author:** AI Assistant

---

## 1. Introduction

This test plan covers the MCP tool surface for the kubernaut API Frontend, including the ADK root agent skeleton, tool registration, and the overall tool dispatch framework. It validates that 14 tools are correctly registered with proper schemas, that the agent is created with Claude Sonnet 4.6 via Vertex AI, and that RBAC-based tool filtering works correctly.

## 2. Test Items

- `internal/agent/root.go` -- ADK root agent creation
- `internal/agent/config.go` -- Agent configuration
- `internal/agent/prompt.go` -- System prompt (no-internals, polling constraints)
- `internal/auth/rbac_tools.go` -- Role-to-tool RBAC mapping

## 3. Features to Be Tested

| Feature | Source | Priority |
|---------|--------|----------|
| Root agent creation with model | ADK Investigation Plan | P0 |
| 14-tool registration | Issue #3, #19, #20 | P0 |
| Tool name uniqueness | Issue #3 | P0 |
| `kubernaut_` prefix convention | Issue #3 | P1 |
| `present_decision` IsLongRunning flag | ADK Investigation Plan Finding 2 | P0 |
| System prompt no-internals constraint | ADK Investigation Plan UX Section | P0 |
| System prompt polling re-call rule | ADK Investigation Plan Change 4 | P0 |
| RBAC per-role tool filtering | Issues #21-#25, #71 | P0 |
| Fail-closed on unknown role | Security model | P0 |
| Tool input schema validation | MCP spec requirement | P1 |

## 4. Approach

- **Framework:** Ginkgo v2 / Gomega (per ADR-015)
- **Naming:** `UT-AF-1XX-NNN` where 1XX is the component and NNN is the spec
- **Isolation:** Tools tested with mock backends (fake K8s, mock KA, mock DS)
- **TDD:** Red/Green/Refactor with explicit checkpoints per component

## 5. Pass/Fail Criteria

- All specs pass with `ginkgo -race`
- Coverage >= 80% per package
- `golangci-lint run` produces 0 errors
- No `panic()` in production code
- `go mod tidy` clean

## 6. Test Deliverables

- `internal/agent/root_test.go` -- 12 specs (ADK skeleton)
- `internal/auth/rbac_tools_test.go` -- 7 specs (RBAC)
- `internal/agent/prompt_test.go` -- 5 specs (system prompt)

## 7. Environmental Needs

- Go 1.25.6+
- `google.golang.org/adk` v1.2.0
- `github.com/Alcova-AI/adk-anthropic-go` v0.1.15
- `github.com/modelcontextprotocol/go-sdk` v1.6.0

## 8. Schedule

| Phase | Scope | Specs |
|-------|-------|-------|
| Phase 1 (Red) | ADK skeleton failing tests | 12 |
| Phase 2 (Green) | ADK skeleton implementation | 12 pass |
| Phase 13 (Red) | RBAC + prompt failing tests | 12 |
| Phase 14 (Green) | RBAC + prompt implementation | 12 pass |

## 9. Test Case Matrix

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-100-001 | Agent creates with model | Valid AgentConfig | Non-nil agent |
| UT-AF-100-002 | Agent has 14 tools | Full tool list | len(tools) == 14 |
| UT-AF-100-003 | Nil model rejected | AgentConfig{Model: nil} | Error |
| UT-AF-100-004 | Tool names unique | All tools | No duplicates |
| UT-AF-100-005 | kubernaut_ prefix | All tool names | All except present_decision start with kubernaut_ |
| UT-AF-100-006 | Non-empty descriptions | All tools | All have description |
| UT-AF-100-007 | Valid input schemas | All tools | JSON Schema valid |
| UT-AF-100-008 | present_decision IsLongRunning | present_decision tool | IsLongRunning() == true |
| UT-AF-100-009 | Others NOT IsLongRunning | Non-present tools | IsLongRunning() == false |
| UT-AF-100-010 | Instruction present | Agent config | Non-empty instruction |
| UT-AF-100-011 | RBAC filter works | SRE role + tools | 14 tools returned |
| UT-AF-100-012 | Empty tools rejected | Empty slice | Error |
| UT-AF-130-001 | SRE gets all | SRE role | 14 tools |
| UT-AF-130-002 | CI/CD gets subset | CI/CD role | 4 tools |
| UT-AF-130-003 | L3 Audit gets DS | L3 role | 6 tools |
| UT-AF-130-004 | AI Orchestrator | AI role | 10 tools |
| UT-AF-130-005 | Observability | Obs role | 5 tools |
| UT-AF-130-006 | Unknown fail-closed | "unknown" | 0 tools |
| UT-AF-130-007 | Filter is non-mutating | Any role | Original slice unchanged |
| UT-AF-131-001 | No-internals in prompt | Prompt text | Contains constraint |
| UT-AF-131-002 | Polling re-call | Prompt text | Contains re-call rule |
| UT-AF-131-003 | present_decision handoff | Prompt text | Contains handoff |
| UT-AF-131-004 | No internal names | Prompt text | No RR/AA/SP/KA/CRD |
| UT-AF-131-005 | Tool inventory | Prompt text | Contains tool list |
