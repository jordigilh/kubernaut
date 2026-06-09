# IEEE 829 Test Plan — Fix #1384: Session Handoff Broken at Workflow Discovery

| Field                | Value                                                    |
|----------------------|----------------------------------------------------------|
| **Test Plan ID**     | TP-KA-1384                                               |
| **Revision**         | 1.0                                                      |
| **Author**           | Kubernaut AI Agent                                       |
| **Date**             | 2026-06-09                                               |
| **Status**           | Active                                                   |
| **Business Req**     | BR-INTERACTIVE-010, BR-AUDIT-005, BR-RBAC-005            |
| **FedRAMP Controls** | AU-2, AU-3, AU-6, SI-10, CP-10                           |

## 1. Introduction

This test plan covers the verification of fixes for GitHub issue #1384, which
describes two bugs in Kubernaut Agent (KA) interactive session management:

- **Bug A**: `handleDiscoverWorkflows` passes an un-enriched MCP context to
  `RunWorkflowDiscovery`, causing workflow discovery to run with an empty
  `session_id` and nil event sink. Results are detached from the investigation
  session.

- **Bug B**: `turnsToReconMessages` copies all conversation turns—including
  those with empty `Content` from tool-call-only LLM responses—into the
  reconstruction prompt. The Vertex AI client creates `NewTextBlock("")` for
  these, triggering a 400 error.

## 2. Scope

### In Scope

| Area                           | Description                                         |
|--------------------------------|-----------------------------------------------------|
| Session context propagation    | `session_id` + `LazySink` wired into workflow disc. |
| Reconstruction empty filter    | `turnsToReconMessages` skips empty `Content` turns  |
| Vertex client defense-in-depth | `buildParams` skips empty text blocks for assistant  |
| Session result bridge          | `CompleteHTTPSession` receives workflow result       |

### Out of Scope

- MCP transport layer (WebSocket/SSE framing)
- AA polling and scheduling
- AF session management (upstream consumer)
- E2E tests (existing E2E suite covers the full interactive flow)

## 3. Business Requirements Traceability

| BR                         | Description                                                     | Violated By |
|----------------------------|-----------------------------------------------------------------|-------------|
| BR-INTERACTIVE-010 SC-2   | Session tracks investigation through all phases                 | Bug A       |
| BR-INTERACTIVE-010 SC-3   | Context reconstruction from audit trail                         | Bug B       |
| BR-AUDIT-005 v2.0 (AU-2)  | All phases emit audit events with correlation IDs               | Bug A       |
| BR-RBAC-005               | Session state preserved across investigation phases             | Bug A       |

## 4. FedRAMP / SOC2 Control Verification

Tests verify **business-level behavior** tied to each control objective.

### AU-2 / AU-3 — Audit Generation / Content of Audit Records

**Business behavior**: An auditor querying DS audit events by `session_id` MUST
trace the full investigation lifecycle (RCA → workflow_discovery → selection).
Bug A breaks this by emitting audit events with empty `session_id`.

**Tests**: UT-KA-1384-A01, IT-KA-1384-001, IT-KA-1384-004

### AU-6 — Audit Review, Analysis, and Reporting

**Business behavior**: After MCP disconnect, KA reconstructs session context from
DS audit events. The reconstructed prompt MUST be valid for the LLM API.
Bug B sends empty text blocks, causing 400 errors and total context loss.

**Tests**: UT-KA-1384-B01, UT-KA-1384-B02, IT-KA-1384-003

### SI-10 — Information Input Validation

**Business behavior**: KA MUST NOT send malformed content to external LLM
providers. Empty text blocks waste API quota and prevent investigation continuity.

**Tests**: UT-KA-1384-B03, UT-KA-1384-B04

### CP-10 — Information System Recovery

**Business behavior**: After MCP transport failure, the system recovers session
context and allows investigation continuation without losing prior work.

**Tests**: IT-KA-1384-003, IT-KA-1384-004

## 5. Test Tiers and Coverage Targets

| Tier              | Testable Code                                                | Target |
|-------------------|--------------------------------------------------------------|--------|
| Unit Tests        | `turnsToReconMessages`, `buildParams` empty-block handling   | ≥80%   |
| Integration Tests | `handleDiscoverWorkflows` wiring, `SpawnReconstruct` chain   | ≥80%   |
| E2E               | Full interactive flow (covered by existing suite)            | N/A    |

## 6. Wiring Manifest (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Component                    | Production Entry Point                      | Wiring Location          | UT (logic)            | IT (wiring)       |
|------------------------------|--------------------------------------------|--------------------------|-----------------------|-------------------|
| Session context propagation  | `handleDiscoverWorkflows` → `RunWorkflow`  | `investigate.go:~866`    | UT-KA-1384-A01..A04  | IT-KA-1384-001    |
| Empty turn filter            | `SpawnReconstruct` → `turnsToReconMessages`| `reconstruct.go:~107`    | UT-KA-1384-B01,B02,B05,B06 | IT-KA-1384-003 |
| Empty text block guard       | `vertexanthropic.Chat/StreamChat`          | `client.go:~232`         | UT-KA-1384-B03,B04   | IT-KA-1384-005    |
| Session result bridge        | `select_workflow` → `CompleteHTTPSession`  | `session_teardown.go`    | UT-KA-1384-A05        | IT-KA-1384-002    |

## 7. Unit Test Scenarios

### Bug A — AU-2/AU-3, BR-INTERACTIVE-010 SC-2

| ID              | Business Behavior Verified                                                              | Acceptance Criteria                                                  |
|-----------------|-----------------------------------------------------------------------------------------|----------------------------------------------------------------------|
| UT-KA-1384-A01  | Session context propagated → workflow_discovery streams events (session continuity)     | `chatOrStream` uses streaming; `session_id` in log is non-empty      |
| UT-KA-1384-A02  | Without session context → graceful degradation (non-streaming fallback)                 | `chatOrStream` falls back to Chat; session_id logged as empty        |
| UT-KA-1384-A03  | Investigation events reach SSE subscriber during workflow_discovery                     | Event delivered to sink channel within 100ms                         |
| UT-KA-1384-A04  | Missing subscriber does not block or crash the investigation loop                       | No panic, no channel send, event silently dropped                    |
| UT-KA-1384-A05  | `buildFinalResult` merges RCA + workflow discovery into single result for AA            | Output has both `RCASummary` and `WorkflowID` populated              |

### Bug B — AU-6, SI-10, CP-10

| ID              | Business Behavior Verified                                                              | Acceptance Criteria                                                  |
|-----------------|-----------------------------------------------------------------------------------------|----------------------------------------------------------------------|
| UT-KA-1384-B01  | Reconstruction produces valid LLM prompt with no empty content blocks (SI-10)          | Zero messages with `Content == ""`                                   |
| UT-KA-1384-B02  | Reconstruction preserves all meaningful conversation context (CP-10)                    | All non-empty turns retained in chronological order                  |
| UT-KA-1384-B03  | LLM client never sends empty text blocks to Vertex AI (SI-10 defense-in-depth)         | Empty Content + no ToolCalls → NO text block                         |
| UT-KA-1384-B04  | LLM client correctly sends non-empty assistant text (no over-filtering)                 | Text block present with correct content                              |
| UT-KA-1384-B05  | All-empty audit history → nil reconstruction (CP-10 graceful degradation)               | Returns nil, not empty slice                                         |
| UT-KA-1384-B06  | Nil input to reconstruction handled safely (defensive coding)                           | Returns nil, no panic                                                |

## 8. Integration Test Scenarios

| ID              | Business Behavior Verified                                          | FedRAMP    | Acceptance Criteria                                                                  |
|-----------------|---------------------------------------------------------------------|------------|--------------------------------------------------------------------------------------|
| IT-KA-1384-001  | Workflow discovery audit events traceable to session                | AU-2/AU-3  | `chatOrStream` logs non-empty `session_id`; audit events have `correlation_id`       |
| IT-KA-1384-002  | Workflow result flows to HTTP session for AA polling                | BR-INT-010 | After discover+select, `CompleteUserDriving` receives result with `WorkflowID != ""` |
| IT-KA-1384-003  | Reconstruction succeeds despite tool-call-only audit turns          | AU-6, CP-10| `SpawnReconstruct` delivers ONLY non-empty messages; no LLM 400                      |
| IT-KA-1384-004  | End-to-end: discover → select → HTTP complete chain                | CP-10      | Full chain; AA poll returns result with workflow ID + parameters                     |
| IT-KA-1384-005  | Reconstruction path never sends empty text blocks to LLM client    | SI-10      | Mock LLM captures request; zero empty text blocks after full path                    |

## 9. TDD Execution Phases

| Phase | Description                                                       | Tests                          |
|-------|-------------------------------------------------------------------|--------------------------------|
| 1     | RED (Bug B): Write failing UTs + ITs for reconstruction           | B01-B06, IT-003, IT-005        |
| 2     | GREEN (Bug B): Implement empty-content filter + defense-in-depth  | All B* + IT-003/005 pass       |
| 3     | RED (Bug A): Write failing UTs + ITs for session propagation      | A01-A05, IT-001, IT-002        |
| 4     | GREEN (Bug A): Wire session_id + LazySink                        | All A* + IT-001/002 pass       |
| 5     | RED (Wiring Chain): Write IT-004 end-to-end                      | IT-004 fails                   |
| 6     | GREEN (Wiring Chain): Remaining wiring for IT-004                 | IT-004 passes                  |
| 7     | REFACTOR: 100-go-mistakes review, extract shared filter           | All tests remain green         |

## 10. Checkpoints

| Checkpoint | Gate                                                                         |
|------------|------------------------------------------------------------------------------|
| CP-1       | Bug B GA audit: build clean, lint clean, Bug B UTs + ITs pass, no regressions|
| CP-2       | Bug A GA audit: all A + B tests pass, session_id propagation verified        |
| CP-3       | Final GA audit: ≥80% per-tier coverage, FedRAMP, wiring manifest, lint       |

## 11. Risk Assessment

| Risk                                          | Severity | Mitigation                              |
|-----------------------------------------------|----------|-----------------------------------------|
| `FindUserDrivingByRemediationID` missing      | Medium   | Verify interface before Phase 4         |
| `GetLazySink` not exposed on session manager  | Medium   | Add accessor if needed                  |
| Reconstruction test depends on DS mock fidelity| Low     | Use existing `mockAuditQuerier` pattern |
