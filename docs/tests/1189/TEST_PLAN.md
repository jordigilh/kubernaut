# Test Plan: A2A Progressive Status Updates and 4-Phase Interactive Remediation Journey

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1189-v2.0
**Feature**: A2A progressive status updates via GenAIPartConverter, completing the 4-phase interactive remediation journey for external agents
**Version**: 2.0
**Created**: 2026-05-19
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/1189-fp-af-integration`
**Predecessor**: TP-1189-v1.0 (SSE parser, CorrelationID, prompt hardening — completed and merged)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the A2A progressive status update mechanism that transforms raw ADK `FunctionCall`/`FunctionResponse` genai parts into human-readable narrative artifacts, so external agents experience the 4-phase remediation journey as a guided narrative — not raw JSON tool payloads.

This is the final AC gap identified in the GA readiness audit of PR #1193. Prior work (TP-1189-v1.0) addressed SSE parser robustness, CorrelationID propagation, prompt rules, and task-to-RR correlation. This plan addresses:
- AC 5: A2A tasks remain open through all 4 phases with progressive status updates
- AC 10 (mapped): External agent receives progressive task status updates through all phases

### 1.2 Feature Description

The ADK v1.2.0 `ExecutorConfig` supports a `GenAIPartConverter` callback and `OutputArtifactPerEvent` output mode. When AF's LLM calls tools during an A2A task, each `session.Event` is converted to an A2A artifact before reaching the external agent. By default, `FunctionCall` and `FunctionResponse` parts are forwarded as raw `DataPart` JSON. The converter transforms them:

1. **FunctionCall** -> `TextPart` with human-readable status indicator (e.g., "Streaming live investigation events...")
2. **FunctionResponse** -> `TextPart` with summarized output for key tools, `nil` (dropped) for others
3. **Text** -> pass through unchanged (LLM reasoning / inner thoughts)

### 1.3 Golden Transcript Grounding

Design validated against `kubernaut-demo-scenarios/golden-transcripts/disk-pressure-emptydir` (17 LLM turns, 23 tool calls, real OCP cluster, 276K tokens). KA's SSE wire format:
- `tool_call_start` with `tool_name` — BEFORE tool execution
- `tool_result` with `result_preview` (200 chars) — AFTER tool execution
- `reasoning_delta` with `content_preview` (200 chars) — LLM inner thoughts after digesting results

The user sees status indicators during tool execution, then LLM reasoning after — never raw tool payloads.

### 1.4 Objectives

1. Validate `GenAIPartConverter` transforms `FunctionCall` parts into status text
2. Validate `GenAIPartConverter` summarizes key `FunctionResponse` outputs
3. Validate `GenAIPartConverter` drops non-key `FunctionResponse` outputs
4. Validate `GenAIPartConverter` passes through `Text` parts unchanged
5. Validate `OutputArtifactPerEvent` is wired in `ExecutorConfig`
6. Validate context extraction from `FunctionCall.Args` (namespace, name, session_id)
7. Validate graceful handling of nil/malformed args
8. Validate no regression on existing launcher tests (enrichRRDetail, audit emission)

### 1.5 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/launcher/... -ginkgo.v` |
| Unit test code coverage (part_converter.go) | >=80% | `go test -coverprofile` |
| Race detector | 0 races | `go test -race` |
| Build success | 0 errors | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run --timeout=5m` |
| BR coverage | All applicable ACs | Coverage matrix in Section 7 |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #1189](https://github.com/jordigilh/kubernaut/issues/1189) — 4-Phase Interactive Remediation Journey
- [PR #1193](https://github.com/jordigilh/kubernaut/pull/1193) — Implementation PR
- BR-AF-1189 — Progressive status updates for A2A tasks
- [DD-TEST-006](../../architecture/decisions/DD-TEST-006-test-plan-policy.md) — Test Plan Policy
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) — Per-tier coverage >=80%
- [ANTI_PATTERN_DETECTION.md](../../testing/ANTI_PATTERN_DETECTION.md) — Forbidden test patterns
- [100 Go Mistakes](https://100go.co) — TDD REFACTOR reference
- ADK v1.2.0 `server/adka2a/executor.go` — `GenAIPartConverter`, `OutputArtifactPerEvent`
- `kubernaut-demo-scenarios/golden-transcripts/disk-pressure-emptydir.json` — Real scenario (17 turns, 23 tools)

### 2.2 Implementation Files

| File | Role |
|------|------|
| `pkg/apifrontend/launcher/part_converter.go` | GenAIPartConverter: FunctionCall->status, FunctionResponse->summary/drop, Text->passthrough (new) |
| `pkg/apifrontend/launcher/launcher.go` | Wire converter + OutputArtifactPerEvent into ExecutorConfig (modify, 2 lines) |

### 2.3 Existing Related Tests (v1.0, completed)

| File | Test IDs | Relationship |
|------|----------|-------------|
| `pkg/apifrontend/ka/sse_parser_test.go` | UT-AF-1189-010..022 | SSE parser edge cases (v1.0, merged) |
| `pkg/apifrontend/ka/rest_client_test.go` | UT-AF-1189-050..051 | Content-Type validation (v1.0, merged) |
| `pkg/apifrontend/launcher/enrichrr_test.go` | UT-AF-1189-040..043 | enrichRRDetail AC 12 (v1.0, merged) |
| `pkg/apifrontend/agent/prompt_test.go` | UT-AF-1189-030..035 | Prompt assertions (v1.0, merged) |
| `pkg/apifrontend/launcher/launcher_test.go` | UT-AF-210-001..007 | Existing launcher tests (regression baseline) |
| `pkg/apifrontend/launcher/a2a_errors_test.go` | UT-AF-PR6A-001..002 | Audit emission + error sanitization |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | GenAIPartConverter returning nil for all parts in a session.Event produces empty artifact | External agent receives empty artifact update | Medium | UT-AF-1189-115 | Test validates ADK drops empty-part events per `processor.go` line 82: `if len(parts) == 0 { return nil, nil }` |
| R2 | FunctionResponse JSON structure changes across tool versions | Summary extraction returns empty string | Low | UT-AF-1189-112 | Graceful fallback: unknown structure returns generic "Completed" text |
| R3 | OutputArtifactPerEvent high chattiness for 17-turn investigations | External agent overwhelmed by artifact events | Low | N/A (consumer concern) | Document in A2A agent card; external agents can aggregate |
| R4 | FunctionCall.Args containing PII (namespace, pod names) in status text | Information leakage to external agent | Low | UT-AF-1189-107 | Args already visible in normal A2A flow; converter makes them friendlier, not more exposed |
| R5 | Concurrent converter calls from parallel tool execution | Race condition in converter | None | Verified | Converter is stateless (pure function), no shared mutable state |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-AF-1189-115 (all parts dropped -> verify no panic)
- **R2**: UT-AF-1189-112 (unknown FunctionResponse structure -> fallback)

---

## 4. Scope

### 4.1 Features to be Tested

- **`buildPartConverter()`**: Factory returning `adka2a.GenAIPartConverter` function
- **FunctionCall transformation**: 12 tool-name-to-status mappings + generic fallback
- **FunctionCall context extraction**: Parse `Args` JSON for namespace, name, session_id, workflow_id
- **FunctionResponse summarization**: 6 key tools with structured summary extraction
- **FunctionResponse dropping**: Non-key tools return nil
- **Text passthrough**: LLM reasoning text forwarded unchanged
- **ExecutorConfig wiring**: `GenAIPartConverter` and `OutputMode` set correctly

### 4.2 Features Not to be Tested

- **MCP path**: Console agent drives tool calls interactively; converter only affects A2A
- **AC 9 (approval)**: Out of scope — RAR is sole authority; AF observes via `kubernaut_watch`
- **AC 13 (DS bidirectional query)**: Deferred — requires Data Storage OpenAPI + handler changes
- **E2E with real LLM**: Requires full Kind cluster with mock LLM; covered by existing FP E2E tests
- **ADK executor internals**: `OutputArtifactPerEvent` behavior is ADK's responsibility; we test our converter

### 4.3 Design Decisions

- **Stateless converter**: Pure function with no shared state; safe for concurrent use.
- **Drop non-key FunctionResponses**: The LLM's reasoning text (which passes through) already incorporates kubectl/prometheus results in human-readable form. Forwarding raw tool results would be redundant.
- **Summarize key FunctionResponses**: Investigation summaries, workflow lists, RR creation results are structured data the external agent can act on programmatically.
- **No phase metadata in artifacts**: Phase tracking is available via audit trail (`AfterEventCallback`). The converter focuses on narrative UX.

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: TESTING_GUIDELINES.md — Per-Tier Testable Code Coverage (>=80% per tier).

| Tier | Scope | Target | Code Subset |
|------|-------|--------|-------------|
| Unit | `buildPartConverter`, all transformation paths | >=80% | Pure logic: status message mapping, arg parsing, summary extraction |
| Integration | N/A for converter (pure function) | N/A | Converter is stateless; integration tested via existing launcher tests |
| E2E | Existing FP E2E tests (E2E-FP-1189-001..003) | Existing | Full A2A flow in Kind cluster |

### 5.2 TDD Phases

| Phase | Description | Deliverables | Checkpoint |
|-------|-------------|-------------|------------|
| **Phase 1: TDD RED** | Write all failing tests | `part_converter_test.go` with Ginkgo specs that compile but fail | CHECKPOINT 1 |
| **Phase 2: TDD GREEN** | Minimal implementation to pass all tests | `part_converter.go` + 2-line wiring in `launcher.go` | CHECKPOINT 2 |
| **Phase 3: TDD REFACTOR** | Code quality: 100 Go Mistakes audit, lint, dedup | Cleaned code, no new lint errors | CHECKPOINT 3 |

### 5.3 Anti-Pattern Compliance

Per ANTI_PATTERN_DETECTION.md:

- Test business outcomes, not implementation details (no NULL-TESTING)
- No `Skip()` or pending tests
- No `time.Sleep()` without approved exception
- Use table-driven tests where appropriate
- All test descriptions include test ID (e.g., `UT-AF-1189-100`)
- Mock only external dependencies (ADK converter uses real `adka2a.ToA2APart` for passthrough)

---

## 6. Test Design Specification

### 6.1 Unit Tests — FunctionCall Transformation (Tier 1)

**Test file**: `pkg/apifrontend/launcher/part_converter_test.go`

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-AF-1189-100 | `kubernaut_start_investigation` FunctionCall -> "Starting investigation for {namespace}/{name}..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-101 | `kubernaut_stream_investigation` FunctionCall -> "Streaming live investigation events..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-102 | `kubernaut_discover_workflows` FunctionCall -> "Discovering available remediation workflows..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-103 | `kubernaut_select_workflow` FunctionCall -> "Selecting remediation workflow {workflow_id}..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-104 | `kubernaut_watch` FunctionCall -> "Watching remediation progress..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-105 | `af_create_rr` FunctionCall -> "Creating remediation request..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-106 | Unknown tool FunctionCall -> "Processing..." (generic fallback) | BR-AF-1189 AC5 | Fallback |
| UT-AF-1189-107 | FunctionCall with nil Args -> status text without context (no panic) | BR-AF-1189 AC5 | Nil/Zero |
| UT-AF-1189-108 | FunctionCall with malformed Args JSON -> status text without context (no panic) | BR-AF-1189 AC5 | Error |
| UT-AF-1189-109 | `af_get_pods` FunctionCall -> "Fetching pod status..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-110 | `af_list_events` FunctionCall -> "Fetching cluster events..." | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-116 | `af_check_existing_rr` FunctionCall -> "Checking for existing remediation..." | BR-AF-1189 AC5 | Happy Path |

### 6.2 Unit Tests — FunctionResponse Summarization (Tier 1)

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-AF-1189-111 | `kubernaut_stream_investigation` response with `summary` field -> TextPart with summary text | BR-AF-1189 AC10 | Happy Path |
| UT-AF-1189-112 | `kubernaut_stream_investigation` response with unknown structure -> generic "Investigation completed" | BR-AF-1189 AC10 | Fallback |
| UT-AF-1189-113 | `kubernaut_discover_workflows` response with `workflows` array -> TextPart listing names + confidence | BR-AF-1189 AC10 | Happy Path |
| UT-AF-1189-114 | `af_create_rr` response with `rr_id` -> TextPart "Remediation request created: {rr_id}" | BR-AF-1189 AC10 | Happy Path |
| UT-AF-1189-115 | Non-key tool (`kubernaut_list_remediations`) FunctionResponse -> nil (dropped) | BR-AF-1189 AC10 | Drop |
| UT-AF-1189-117 | `kubernaut_select_workflow` response with `status`, `message` -> TextPart with both | BR-AF-1189 AC10 | Happy Path |
| UT-AF-1189-118 | `kubernaut_watch` response with phase transition -> TextPart "Phase: {old} -> {new}" | BR-AF-1189 AC10 | Happy Path |
| UT-AF-1189-119 | FunctionResponse with nil Response map -> generic fallback text (no panic) | BR-AF-1189 AC10 | Nil/Zero |
| UT-AF-1189-120 | `kubernaut_start_investigation` response with `session_id` -> TextPart "Investigation started: {session_id}" | BR-AF-1189 AC10 | Happy Path |

### 6.3 Unit Tests — Text Passthrough (Tier 1)

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-AF-1189-130 | Text part with LLM reasoning -> passed through as TextPart unchanged | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-131 | Text part with `Thought: true` -> passed through with thought metadata preserved | BR-AF-1189 AC5 | Happy Path |
| UT-AF-1189-132 | Empty text part -> passed through (not dropped) | BR-AF-1189 AC5 | Edge Case |

### 6.4 Unit Tests — ExecutorConfig Wiring (Tier 1)

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-AF-1189-140 | `NewA2AHandler` sets `GenAIPartConverter` in ExecutorConfig (not nil) | BR-AF-1189 AC5 | Wiring |
| UT-AF-1189-141 | `NewA2AHandler` sets `OutputMode` to `OutputArtifactPerEvent` | BR-AF-1189 AC5 | Wiring |

### 6.5 Unit Tests — Adversarial Inputs (Tier 1)

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| UT-AF-1189-150 | FunctionCall with 10KB Args JSON -> no OOM, status text with truncated context | BR-AF-1189 AC5 | Adversarial |
| UT-AF-1189-151 | FunctionResponse with 100KB Response map -> summary truncated to reasonable length | BR-AF-1189 AC10 | Adversarial |
| UT-AF-1189-152 | FunctionCall with tool name containing special chars -> generic fallback (no panic) | BR-AF-1189 AC5 | Adversarial |

---

## 7. BR Coverage Matrix

| BR ID | AC | Description | Test Type | Test IDs | Status |
|-------|----|-------------|-----------|----------|--------|
| BR-AF-1189 | AC1 | AF streams investigation events from KA SSE in real-time | Unit | UT-AF-1189-010..022 (v1.0 merged) | Done |
| BR-AF-1189 | AC2 | `kubernaut_discover_workflows` in prompt tool inventory | Unit | UT-AF-1189-031 (v1.0 merged) | Done |
| BR-AF-1189 | AC3 | Prompt enforces 4-phase journey | Unit | UT-AF-1189-032..035 (v1.0 merged) | Done |
| BR-AF-1189 | AC4 | `kubernaut_watch` called after workflow selection | Unit | UT-AF-1189-034 (v1.0 merged) | Done |
| BR-AF-1189 | AC5 | A2A tasks with progressive status updates | Unit | UT-AF-1189-100..110, 116, 130..132, 140..141, 150..152 | **This plan** |
| BR-AF-1189 | AC6 | E2E validates full 4-phase journey | E2E | E2E-FP-1189-001..003 (v1.0 merged) | Done |
| BR-AF-1189 | AC7 | A2A autonomous intent triggers 4-phase flow | Unit | UT-AF-1189-033 (v1.0 prompt test) | Done |
| BR-AF-1189 | AC8 | Prompt has autonomous vs interactive detection | Unit | UT-AF-1189-033 (v1.0 merged) | Done |
| BR-AF-1189 | AC9 | AF does not block on approval | N/A | Out of scope (RAR sole authority) | N/A |
| BR-AF-1189 | AC10 | External agent receives progressive status updates | Unit | UT-AF-1189-111..120 | **This plan** |
| BR-AF-1189 | AC11 | `af_create_rr` audit includes `a2a_task_id` | Unit | UT-AF-1189-040..043 (v1.0 merged) | Done |
| BR-AF-1189 | AC12 | `EventA2ATaskCompleted` includes `rr_name`/`namespace` | Unit | UT-AF-1189-040..043 (v1.0 merged) | Done |
| BR-AF-1189 | AC13 | Data Store bidirectional correlation query | N/A | Deferred (requires DS changes) | Deferred |

---

## 8. Test Case Specifications

### 8.1 UT-AF-1189-100: start_investigation FunctionCall -> status text with context

**BR**: BR-AF-1189 AC5
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**: Converter function instantiated via `buildPartConverter()`

**Steps**:
1. **Given**: A `genai.Part` with `FunctionCall{Name: "kubernaut_start_investigation", Args: {"namespace":"prod","kind":"Deployment","name":"api-server"}}`
2. **When**: Converter is called with the part
3. **Then**: Returns `TextPart` with status message

**Expected Result**:
- Returned part is non-nil
- Part text contains "Starting investigation"
- Part text contains "prod" and "api-server"

### 8.2 UT-AF-1189-111: stream_investigation FunctionResponse -> summary extraction

**BR**: BR-AF-1189 AC10
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**: Converter function instantiated

**Steps**:
1. **Given**: A `genai.Part` with `FunctionResponse{Name: "kubernaut_stream_investigation", Response: {"status":"completed","summary":"Root cause: PostgreSQL uses emptyDir with 8Gi..."}}`
2. **When**: Converter is called with the part
3. **Then**: Returns `TextPart` with the summary

**Expected Result**:
- Returned part is non-nil
- Part text contains "Root cause: PostgreSQL uses emptyDir"
- Part text does NOT contain raw JSON structure

### 8.3 UT-AF-1189-115: Non-key tool FunctionResponse -> nil (dropped)

**BR**: BR-AF-1189 AC10
**Type**: Unit
**Category**: Drop
**Priority**: P0

**Preconditions**: Converter function instantiated

**Steps**:
1. **Given**: A `genai.Part` with `FunctionResponse{Name: "kubernaut_list_remediations", Response: {"remediations":[...]}}`
2. **When**: Converter is called with the part
3. **Then**: Returns `nil`

**Expected Result**:
- Returned part is nil (ADK drops nil parts per `GenAIPartConverter` contract)

### 8.4 UT-AF-1189-130: Text part passthrough

**BR**: BR-AF-1189 AC5
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**: Converter function instantiated

**Steps**:
1. **Given**: A `genai.Part` with `Text: "Node shows DiskPressure, emptyDir usage at 72%..."`
2. **When**: Converter is called with the part
3. **Then**: Returns TextPart with identical text

**Expected Result**:
- Returned part text equals original text exactly

### 8.5 UT-AF-1189-107: FunctionCall with nil Args

**BR**: BR-AF-1189 AC5
**Type**: Unit
**Category**: Nil/Zero
**Priority**: P1

**Preconditions**: Converter function instantiated

**Steps**:
1. **Given**: A `genai.Part` with `FunctionCall{Name: "kubernaut_watch", Args: nil}`
2. **When**: Converter is called
3. **Then**: Returns TextPart with base status message (no context)

**Expected Result**:
- No panic
- Part text contains "Watching remediation progress..."
- Part text does NOT contain nil/empty context fragments

---

## 9. Checkpoint Specifications

### CHECKPOINT 1 — After TDD RED Phase

**Gate criteria**: All tests written and verified to FAIL (compile but red).

#### 9-Category Audit

| # | Category | Tests That Satisfy | Notes |
|---|----------|--------------------|-------|
| 1 | **Observability wiring** | N/A for converter (pure function). Audit emission tested in v1.0: UT-AF-1189-040..043 | No new audit events in converter |
| 2 | **Adversarial inputs** | UT-AF-1189-107 (nil args), UT-AF-1189-108 (malformed JSON), UT-AF-1189-150 (10KB args), UT-AF-1189-151 (100KB response), UT-AF-1189-152 (special chars in tool name) | All external inputs covered |
| 3 | **Resource bounds** | UT-AF-1189-150, UT-AF-1189-151 | Large input handling verified; converter is per-call, no growing state |
| 4 | **Concurrency** | R5 risk assessment: converter is stateless pure function | No shared mutable state; no goroutines |
| 5 | **Nil/zero edge cases** | UT-AF-1189-107 (nil args), UT-AF-1189-119 (nil Response), UT-AF-1189-132 (empty text) | All nil/zero paths covered |
| 6 | **Error-path observability** | UT-AF-1189-108 (malformed JSON), UT-AF-1189-112 (unknown structure) | Errors degrade to fallback, no panic |
| 7 | **Cross-phase integration** | UT-AF-1189-140, UT-AF-1189-141 | Converter wired into ExecutorConfig |
| 8 | **Spec compliance** | UT-AF-1189-111..120 verify output matches golden transcript expectations | Summary text matches real investigation patterns |
| 9 | **API surface hygiene** | `buildPartConverter` unexported; converter returned as `adka2a.GenAIPartConverter` function value | No test helpers exported from production packages |

#### Preflight Check
- [ ] All 30 test specs compile
- [ ] All 30 test specs FAIL (red)
- [ ] No `Skip()` or pending tests
- [ ] Test descriptions include test IDs
- [ ] Test file uses Ginkgo/Gomega BDD framework
- [ ] Confidence >= 95%? If not, escalate findings before proceeding

### CHECKPOINT 2 — After TDD GREEN Phase

**Gate criteria**: All tests PASS. `go build ./...` succeeds. `go vet ./...` clean.

#### 9-Category Audit

| # | Category | Verification |
|---|----------|-------------|
| 1 | **Observability wiring** | Verify no accidental audit emission from converter (pure function) |
| 2 | **Adversarial inputs** | Run UT-AF-1189-107, -108, -150..152: all return valid output without panic |
| 3 | **Resource bounds** | Code review: no maps/slices that grow across calls. Converter is per-invocation |
| 4 | **Concurrency** | Run `go test -race ./pkg/apifrontend/launcher/...`: zero races |
| 5 | **Nil/zero edge cases** | Run UT-AF-1189-107, -119, -132: all pass with correct fallback behavior |
| 6 | **Error-path observability** | Run UT-AF-1189-108, -112: malformed input produces fallback text, no panic |
| 7 | **Cross-phase integration** | Run UT-AF-1189-140, -141: ExecutorConfig has converter and OutputMode wired |
| 8 | **Spec compliance** | Run all tests: summaries match golden transcript patterns |
| 9 | **API surface hygiene** | Verify `buildPartConverter` is unexported. No exported types beyond what's needed |

#### Preflight Check
- [ ] All 30 tests PASS
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` clean
- [ ] `go test -race ./pkg/apifrontend/launcher/...` — zero races
- [ ] Existing launcher tests (UT-AF-210-*) still pass (regression check)
- [ ] Existing enrichRRDetail tests (UT-AF-1189-040..043) still pass
- [ ] Confidence >= 95%? If not, escalate findings before proceeding

### CHECKPOINT 3 — After TDD REFACTOR Phase

**Gate criteria**: All tests still PASS. Code quality validated against 100 Go Mistakes.

#### 100 Go Mistakes Audit

| Mistake # | Title | Check | Status |
|-----------|-------|-------|--------|
| #1 | Unintended variable shadowing | No `err :=` inside `if` blocks that shadow outer `err` | Pending |
| #2 | Unnecessary nested code | Use early returns for nil/unknown checks | Pending |
| #3 | Misusing init functions | No init() in converter | Pending |
| #9 | Being confused about when to use generics | Converter uses concrete types, not generics | Pending |
| #10 | Not being aware of possible side effects in type embedding | No embedding in converter | Pending |
| #16 | Not using linters effectively | `golangci-lint run --timeout=5m` | Pending |
| #21 | Inefficient slice initialization | Pre-size status message map with `make(map, N)` | Pending |
| #26 | Slices and memory leaks | Converter returns new parts per call; no slice retention | Pending |
| #36 | Not understanding the concept of a rune | Tool names are ASCII; no rune issues | Pending |
| #39 | Under-optimized string concatenation | Use `fmt.Sprintf` for status messages, not `+` chains | Pending |
| #41 | Not using `io.Reader` and `io.Writer` abstractions | N/A (no I/O in converter) | Pending |
| #48 | Forgetting about `context.Context` | Converter receives `ctx` from ADK and passes through | Pending |
| #49 | Not using `errgroup` for goroutine error handling | N/A (no goroutines) | Pending |
| #53 | Not handling defer errors | N/A (no defers) | Pending |
| #54 | Not closing resources | N/A (no resources) | Pending |
| #77 | JSON handling mistakes | Use `json.Unmarshal` with target struct, not `interface{}` chains | Pending |
| #84 | Not using testing utility packages | Tests use Ginkgo/Gomega (project standard) | Pending |
| #91 | Not using `httptest` | N/A (no HTTP in converter) | Pending |
| #100 | Not understanding Go diagnostics tooling | `go vet`, `golangci-lint` pass | Pending |

#### 9-Category Re-Audit (Refactored Code)

All 9 categories re-verified against refactored code. Full test suite:
```bash
go test -race -count=1 ./pkg/apifrontend/launcher/... -ginkgo.v
```

#### Preflight Check
- [ ] All 30 tests still PASS
- [ ] `golangci-lint run --timeout=5m` — zero new errors
- [ ] `go vet ./...` — clean
- [ ] All `Expect` assertions include business-outcome context
- [ ] No duplicated code patterns (extract helpers if >15 lines shared)
- [ ] Status message map is package-level `var` (not rebuilt per call)
- [ ] Confidence >= 95%? If not, escalate findings before proceeding

---

## 10. Implementation Phases (TDD)

### Phase 1: TDD RED — Write Failing Tests

**Files to create**:
1. `pkg/apifrontend/launcher/part_converter_test.go` — 30 test specs (UT-AF-1189-100..152)

**Expected state**: All tests compile but FAIL (no implementation yet).

**CHECKPOINT 1 gate**: 9-category audit + preflight before proceeding.

### Phase 2: TDD GREEN — Minimal Implementation

**Files to create/modify**:
1. `pkg/apifrontend/launcher/part_converter.go` (new, ~150 lines)
   - `toolStatusMessages` map
   - `keyToolSummarizers` map
   - `buildPartConverter()` factory
   - `extractContext()` helper
   - `summarizeResponse()` helper
2. `pkg/apifrontend/launcher/launcher.go` (modify, +2 lines in ExecutorConfig)

**Expected state**: All tests PASS. `go build ./...` succeeds.

**CHECKPOINT 2 gate**: 9-category audit + regression check + preflight before proceeding.

### Phase 3: TDD REFACTOR — Code Quality

**Activities**:
1. 100 Go Mistakes audit (table in Section 9)
2. Extract shared patterns if duplicated
3. `golangci-lint run --timeout=5m`
4. `go vet ./...`
5. Ensure status map is package-level (not per-call allocation)
6. Review JSON unmarshaling for type safety (#77)

**Expected state**: All tests still PASS. No new lint errors. Code quality improved.

**CHECKPOINT 3 gate**: 9-category re-audit + 100 Go Mistakes verification + preflight.

---

## 11. Coverage Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Unit test coverage (part_converter.go) | >=80% | Pending |
| BR-AF-1189 AC5/AC10 test coverage | 100% (30/30 specs) | Pending |
| Race detector | 0 races | Pending |
| Lint compliance | 0 new errors | Pending |
| Regression (existing launcher tests) | 100% pass | Pending |

---

## 12. Execution Commands

```bash
# All v2.0 tests (this plan)
go test -race -count=1 ./pkg/apifrontend/launcher/... -ginkgo.v -ginkgo.focus="1189-1"

# Full launcher suite (regression)
go test -race -count=1 ./pkg/apifrontend/launcher/... -ginkgo.v

# Full AF package suite
go test -race -count=1 ./pkg/apifrontend/... -ginkgo.v

# Coverage
go test -coverprofile=coverage.out ./pkg/apifrontend/launcher/... && go tool cover -func=coverage.out

# Build + lint
go build ./...
go vet ./...
golangci-lint run --timeout=5m
```

---

## 13. Dependencies

| Dependency | Version | Usage |
|------------|---------|-------|
| `google.golang.org/adk` | v1.2.0 | `adka2a.GenAIPartConverter`, `OutputArtifactPerEvent`, `adka2a.ToA2APart` |
| `google.golang.org/genai` | (transitive via ADK) | `genai.Part`, `genai.FunctionCall`, `genai.FunctionResponse` |
| `github.com/onsi/ginkgo/v2` | latest | BDD test framework |
| `github.com/onsi/gomega` | latest | Assertion library |
| No new external dependencies | — | Converter uses existing ADK types |

---

## 14. Sign-off

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Assistant | 2026-05-19 | Draft |
| Technical Review | | | Pending |
| QE Review | | | Pending |

---

## 15. Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-19 | AI Assistant | Initial v1.0: SSE parser, CorrelationID, prompt, enrichRRDetail (completed and merged) |
| 2.0 | 2026-05-19 | AI Assistant | v2.0: A2A progressive status updates via GenAIPartConverter, grounded in golden transcripts |

---

## Appendix A: Golden Transcript Reference

**Source**: `kubernaut-demo-scenarios/golden-transcripts/disk-pressure-emptydir.json`

**Scenario**: DiskPressure from PostgreSQL emptyDir on constrained node
**KA turns**: 17 LLM turns, 23 tool calls
**Tokens**: 276,409
**Tool sequence** (representative):
1. `kubectl_get_by_kind_in_namespace` (turn 1)
2. `get_metric_names` (turn 2)
3. `kubectl_describe` (turn 0)
4. `kubectl_events` (turn 1)
5. `execute_prometheus_instant_query` (turns 2-3)
6. `execute_prometheus_range_query` (turns 2-3)
7. `get_cluster_resource_context` (turn 0)
8. `list_available_actions` (turn 0)
9. `list_workflows` (turn 0)
10. `get_workflow` (turn 0)

**KA SSE event pattern**:
```
reasoning_delta -> tool_call_start (x N) -> tool_result (x N) -> reasoning_delta -> ...
```

The converter transforms AF's equivalent pattern:
```
Text (LLM reasoning) -> FunctionCall (tool invocation) -> FunctionResponse (tool result) -> Text (LLM reasoning) -> ...
```

## Appendix B: Tool-to-Status Message Mapping

| Tool Name | Status Message | Context Fields |
|-----------|---------------|----------------|
| `kubernaut_start_investigation` | "Starting investigation for {namespace}/{name}..." | namespace, kind, name |
| `kubernaut_stream_investigation` | "Streaming live investigation events..." | session_id |
| `kubernaut_poll_investigation` | "Polling investigation status..." | session_id |
| `kubernaut_discover_workflows` | "Discovering available remediation workflows..." | rr_id |
| `kubernaut_select_workflow` | "Selecting remediation workflow {workflow_id}..." | workflow_id |
| `kubernaut_watch` | "Watching remediation progress..." | rr_id |
| `af_create_rr` | "Creating remediation request..." | namespace, kind, name |
| `af_check_existing_rr` | "Checking for existing remediation..." | namespace, kind, name |
| `af_list_events` | "Fetching cluster events..." | namespace |
| `af_get_pods` | "Fetching pod status..." | namespace |
| `af_get_workloads` | "Fetching workload details..." | namespace |
| `af_resolve_owner` | "Resolving resource ownership..." | namespace, name |
| `kubernaut_list_remediations` | "Listing remediations..." | namespace |
| `kubernaut_get_remediation` | "Getting remediation details..." | name |
| `kubernaut_approve` | "Approving remediation..." | name |
| `kubernaut_cancel_remediation` | "Cancelling remediation..." | name |
| `present_decision` | "Presenting decision to user..." | — |
| `kubernaut_list_workflows` | "Listing available workflows..." | — |
| `kubernaut_get_remediation_history` | "Getting remediation history..." | — |
| `kubernaut_get_effectiveness` | "Getting effectiveness data..." | — |
| `kubernaut_get_audit_trail` | "Getting audit trail..." | — |
| (unknown) | "Processing..." | — |

## Appendix C: Key Tool Response Summarizers

| Tool Name | Summary Extraction |
|-----------|-------------------|
| `kubernaut_stream_investigation` | `response["summary"]` -> full text |
| `kubernaut_start_investigation` | `"Investigation started: " + response["session_id"]` |
| `kubernaut_discover_workflows` | List workflow names from `response["workflows"]` array with confidence scores |
| `kubernaut_select_workflow` | `response["status"] + ": " + response["message"]` |
| `kubernaut_watch` | Extract phase from `response["phase"]` or `response["status"]` |
| `af_create_rr` | `"Remediation request created: " + response["rr_id"]` |
| All others | Return `nil` (dropped; LLM reasoning covers the content) |
