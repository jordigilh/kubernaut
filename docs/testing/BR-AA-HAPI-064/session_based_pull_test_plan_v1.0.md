# V1.0 Test Plan: BR-AA-HAPI-064 Session-Based Pull Design

**Version**: 1.0.0
**Created**: 2026-02-09
**Status**: APPROVED
**Purpose**: TDD test plan for session-based asynchronous AA-HAPI communication

---

## Overview

This test plan covers the full TDD implementation of BR-AA-HAPI-064 (Session-Based Pull Design),
which replaces the synchronous blocking HTTP call between AA and HAPI with an asynchronous
submit/poll pattern using session IDs.

**Reference Documents**:
- [BR-AA-HAPI-064](../../../docs/requirements/BR-AA-HAPI-064-session-based-pull-design.md)
- [DD-AA-HAPI-064](../../../docs/architecture/decisions/DD-AA-HAPI-064-session-based-pull-design.md)
- [DD-EVENT-001](../../../docs/architecture/decisions/DD-EVENT-001-controller-event-registry.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## Test Plan: AIAnalysis + HolmesGPT-API

**Service Type**: [x] CRD Controller (AA) | [x] Stateless HTTP API (HAPI)
**Team**: Kubernaut Core
**Date**: 2026-02-09
**Tester**: AI Agent + Jordi Gil

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-064-{SEQUENCE}`

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) and
[V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md):

- `UT-AA-064-xxx` -- AA unit tests (Go, Ginkgo/Gomega)
- `UT-HAPI-064-xxx` -- HAPI unit tests (Python, pytest)
- `IT-AA-064-xxx` -- AA integration tests (Go, envtest + real HAPI + Mock LLM)
- `IT-HAPI-064-xxx` -- HAPI integration tests (Python, direct business logic calls)
- `E2E-AA-064-xxx` -- AA E2E tests (Go, Kind cluster)
- `E2E-HAPI-064-xxx` -- HAPI E2E tests (Go, Kind cluster)

---

## Scope

This test plan covers:

- AA controller submit/poll/result handler logic (incident + recovery)
- HAPI session manager lifecycle and async endpoints (incident + recovery)
- `InvestigationSession` CRD tracking, `InvestigationSessionReady` Condition, K8s Events
- Session regeneration (404 detection, Generation counter, cap at 5)
- Audit trace validation with **exact** counts at integration tier
- Mock LLM session endpoint support for CI/CD
- Service-specific E2E with async AA-HAPI communication

**Out of Scope**:

- RO handling of session-related failures (covered by RO test plan)
- DataStorage audit persistence internals (DataStorage's responsibility)
- Performance tier (out of scope for v1.0 per TESTING_GUIDELINES v2.5.2)

### Design Decision: Async-First Endpoints (No Backward Compatibility)

**Decision date**: 2026-02-11
**Context**: BR-AA-HAPI-064 replaces synchronous blocking HTTP calls between AA and HAPI with an
asynchronous submit/poll/result pattern. Both HAPI endpoints (`POST /api/v1/incident/analyze` and
`POST /api/v1/recovery/analyze`) are modified to return HTTP 202 Accepted with a `session_id`
instead of HTTP 200 with the full synchronous response.

**Decision**: **No backward compatibility is maintained.** The existing synchronous behavior is fully
replaced by the async session pattern. There is no header-based toggle or parallel endpoint path.

**Rationale**:
- The AA controller (the sole consumer of these endpoints) is simultaneously being updated to use
  `SubmitInvestigation()` and `PollSession()` instead of the synchronous `Investigate()` method.
- Maintaining two code paths adds complexity without benefit -- there are no external consumers.
- A single async code path is simpler to test, maintain, and reason about.

**Impact on existing HAPI unit tests**:
The following existing test files call `POST /api/v1/recovery/analyze` and assert `status_code == 200`
with synchronous response body validation. They must be refactored to the async-aware pattern
(submit → get result) as part of the GREEN phase:

| File | Tests affected | Refactoring pattern |
| ---- | -------------- | ------------------- |
| `holmesgpt-api/tests/unit/test_recovery.py` | 11 endpoint calls | Submit (202) → GET result (200) → assert body |
| `holmesgpt-api/tests/unit/test_sdk_availability.py` | 1 endpoint call | Submit (202) → GET result (200) → assert body |

Files with string references to endpoint paths but no HTTP calls (`test_health.py`, `test_rfc7807_errors.py`)
are **not affected** and require no changes.

**Note**: FastAPI's `TestClient` (Starlette) executes `BackgroundTasks` synchronously within the
request lifecycle, so the submit → get-result pattern works without polling or sleep in unit tests.

---

## Triage Principles Applied

1. **Business outcome focus**: Every scenario validates an observable outcome (CRD status, Phase, Condition, ctrl.Result, HTTP response) -- not internal implementation details.
2. **Audit as side effect**: At unit level, audit recording is verified as a side-effect assertion (via `auditClientSpy`) within the flow scenario that triggers it -- not as standalone "verify mock was called" scenarios. Audit trace **count correctness** is validated at the integration tier with exact match assertions.
3. **No approximate counts**: All audit event counts in integration tests use `Equal(N)`, never `BeNumerically(">", 0)` or `BeNumerically(">=", N)`.
4. **Scenarios eliminated by triage**:
   - Standalone Condition scenario (merged into submit scenario as assertion)
   - Standalone K8s Event scenario (merged into cap-exceeded scenario)
   - ErrorTypeSessionLost classification (internal detail; behavior covered by regeneration scenario)
   - 3 standalone audit spy scenarios (folded into flow scenarios)
   - 30s timeout wait scenario (reframed as config correctness -- runs in milliseconds)

---

## Defense-in-Depth Coverage

| Tier        | BR Coverage                          | Code Coverage Target                     | Scenarios            | Focus                                                       |
| ----------- | ------------------------------------ | ---------------------------------------- | -------------------- | ----------------------------------------------------------- |
| Unit        | 100% of unit-testable code           | 100% of unit-testable code               | 36 (17 AA + 19 HAPI) | Handler behavior, SessionManager, error handling            |
| Integration | 100% of integration-testable code    | 100% of integration-testable code        | 14 (8 AA + 6 HAPI)   | CRD operations, exact audit trace counts, real HAPI+MockLLM |
| E2E         | Essential journeys only              | Essential journeys only                  | 5 (3 AA + 2 HAPI)    | Deployed controller, service-specific Kind cluster          |
| **Total**   | -           | -                    | **55**               | -                                                           |

---

## 1. Unit Tests -- AA Controller (Go)

**Location**: `test/unit/aianalysis/investigating_handler_session_test.go`
**Framework**: Ginkgo/Gomega BDD
**Mock**: `MockHolmesGPTClient` (extended with async methods) from `test/shared/mocks/holmesgpt.go`
**Audit validation**: Uses `auditClientSpy` (existing pattern in `test/unit/aianalysis/investigating_handler_test.go`). Audit events are validated as side effects of business operations, not as standalone scenarios.
**Existing pattern reference**: `test/unit/aianalysis/investigating_handler_test.go`

### 1.1 Incident Submit Flow

- **UT-AA-064-001**: Submit investigation when InvestigationSession is nil
  - BR: BR-AA-HAPI-064.1, BR-AA-HAPI-064.4, BR-AA-HAPI-064.7
  - Business Outcome: Controller creates a HAPI session and records it in CRD status
  - Given: AIAnalysis with nil InvestigationSession, Phase=Investigating
  - When: `Handle()` called
  - Then:
    - CRD status: `InvestigationSession{ID: "uuid", Generation: 0, CreatedAt: now}` populated
    - Condition: `InvestigationSessionReady=True`, Reason=`SessionCreated`
    - Result: `RequeueAfter: 10s` (non-blocking return)
    - Audit side effect: `auditClientSpy` recorded exactly 1 `holmesgpt.submit` event with sessionID
- **UT-AA-064-002**: Submit investigation after session regeneration (ID cleared, Generation preserved)
  - BR: BR-AA-HAPI-064.5
  - Business Outcome: After a session loss, AA resubmits while preserving the regeneration count
  - Given: AIAnalysis with `InvestigationSession{ID: "", Generation: 2}`
  - When: `Handle()` called
  - Then:
    - CRD status: ID populated with new UUID, Generation preserved at 2, CreatedAt updated
    - Condition: `InvestigationSessionReady=True`, Reason=`SessionRegenerated`

### 1.2 Incident Poll Flow

- **UT-AA-064-003**: Poll session -- status "pending", controller requeues
  - BR: BR-AA-HAPI-064.2, BR-AA-HAPI-064.8
  - Business Outcome: AA waits for HAPI to complete investigation without blocking
  - Given: InvestigationSession with valid ID
  - When: `PollSession()` returns `{status: "pending"}`
  - Then:
    - CRD status: `LastPolled` updated to now
    - Condition: Reason=`SessionActive`
    - Result: `RequeueAfter: 10s` (first poll interval)
- **UT-AA-064-004**: Poll session -- status "investigating", backoff increases
  - BR: BR-AA-HAPI-064.2, BR-AA-HAPI-064.8
  - Business Outcome: AA uses increasing backoff to avoid overloading HAPI
  - Given: InvestigationSession with valid ID, second consecutive poll
  - When: `PollSession()` returns `{status: "investigating"}`
  - Then:
    - CRD status: `LastPolled` updated
    - Result: `RequeueAfter: 20s` (second poll backoff)
- **UT-AA-064-005**: Poll session -- status "completed", result fetched and processed
  - BR: BR-AA-HAPI-064.3
  - Business Outcome: AA retrieves the completed investigation and advances to policy analysis
  - Given: InvestigationSession with valid ID
  - When: `PollSession()` returns `{status: "completed"}`, `GetSessionResult()` returns `IncidentResponse` with workflow
  - Then:
    - CRD status: Phase transitions to `Analyzing`, SelectedWorkflow populated
    - Audit side effect: `auditClientSpy` recorded exactly 1 `holmesgpt.result` event with `investigationTime > 0`
- **UT-AA-064-006**: Poll session -- status "failed", investigation terminates
  - BR: BR-AA-HAPI-064.2
  - Business Outcome: HAPI-side failure is surfaced to operators via CRD status
  - Given: InvestigationSession with valid ID
  - When: `PollSession()` returns `{status: "failed", error: "LLM provider error"}`
  - Then:
    - CRD status: Phase=`Failed`, error details in Message/Reason
- **UT-AA-064-007**: Polling backoff caps at 30s
  - BR: BR-AA-HAPI-064.8
  - Business Outcome: Polling frequency stabilizes to avoid unnecessary API load
  - Given: Consecutive polls (1st, 2nd, 3rd, 4th)
  - Then: RequeueAfter values are 10s, 20s, 30s, 30s respectively (capped)

### 1.3 Session Lost and Regeneration

- **UT-AA-064-008**: Session lost (404) -- first regeneration
  - BR: BR-AA-HAPI-064.5
  - Business Outcome: AA self-heals after HAPI restart by resubmitting investigation
  - Given: `InvestigationSession{ID: "session-1", Generation: 0}`
  - When: `PollSession()` returns 404
  - Then:
    - CRD status: Generation=1, ID cleared, CreatedAt preserved
    - Condition: `InvestigationSessionReady=False`, Reason=`SessionLost`
    - Result: `RequeueAfter: 0` (immediate resubmit)
    - Audit side effect: `auditClientSpy` recorded exactly 1 `holmesgpt.session_lost` event with `generation=1`
- **UT-AA-064-009**: Session lost -- multiple regenerations under cap
  - BR: BR-AA-HAPI-064.5
  - Business Outcome: AA continues self-healing up to the regeneration limit
  - Given: `InvestigationSession{Generation: 3}`
  - When: `PollSession()` returns 404
  - Then:
    - CRD status: Generation=4, ID cleared
    - Result: `RequeueAfter: 0` (immediate resubmit, not failed)
- **UT-AA-064-010**: Regeneration cap exceeded -- investigation fails with escalation
  - BR: BR-AA-HAPI-064.6, DD-EVENT-001
  - Business Outcome: After 5 failed session regenerations, AA escalates to operators via CRD status, K8s Warning Event, and escalation notification
  - Given: `InvestigationSession{Generation: 4}`
  - When: `PollSession()` returns 404 (incrementing Generation to 5)
  - Then:
    - CRD status: Phase=`Failed`, SubReason=`"SessionRegenerationExceeded"`
    - Condition: `InvestigationSessionReady=False`, Reason=`SessionRegenerationExceeded`
    - K8s Event: Warning Event emitted with reason=`EventReasonSessionRegenerationExceeded` (verified via fake EventRecorder)
    - Escalation: notification triggered for operator intervention

### 1.4 Error Handling

- **UT-AA-064-011**: Submit transient error (503) -- AA retries with backoff
  - Business Outcome: Temporary HAPI unavailability does not fail the investigation
  - Given: `SubmitInvestigation()` returns 503 Service Unavailable
  - When: `Handle()` called
  - Then:
    - CRD status: Phase stays `Investigating`, ConsecutiveFailures incremented
    - Result: `RequeueAfter` with exponential backoff
- **UT-AA-064-012**: Submit permanent error (401) -- investigation fails immediately
  - Business Outcome: Authentication failure is surfaced as permanent failure, no retry
  - Given: `SubmitInvestigation()` returns 401 Unauthorized
  - When: `Handle()` called
  - Then:
    - CRD status: Phase=`Failed`, SubReason=`PermanentError`
- **UT-AA-064-013**: GetSessionResult returns 409 -- AA re-polls
  - Business Outcome: Race condition (poll said completed but result not ready) is handled gracefully
  - Given: `PollSession()` returned "completed" but `GetSessionResult()` returns 409 Conflict
  - When: `Handle()` called
  - Then:
    - Result: requeue for re-poll (treated as transient)

### 1.5 Client Configuration Correctness

- **UT-AA-064-014**: Async client constructor sets 30s timeout (not 10m workaround)
  - BR: BR-AA-HAPI-064.10
  - Business Outcome: All AA-HAPI HTTP calls are short-lived; the 10-minute blocking workaround is removed
  - Given: `NewHolmesGPTClient()` constructed with async config
  - Then: `client.HTTPClient.Timeout == 30 * time.Second` (config value assertion, runs in milliseconds)

### 1.6 Recovery Submit/Poll Flow (Dedicated)

- **UT-AA-064-015**: Recovery submit routes to recovery endpoint
  - BR: BR-AA-HAPI-064.9
  - Business Outcome: Recovery investigations use the dedicated recovery endpoint, not the incident endpoint
  - Given: AIAnalysis with `IsRecoveryAttempt=true`, nil InvestigationSession
  - When: `Handle()` called
  - Then:
    - `SubmitRecoveryInvestigation()` called (NOT `SubmitInvestigation`)
    - CRD status: InvestigationSession populated with session ID
- **UT-AA-064-016**: Recovery poll completed -- recovery result fetched and processed
  - BR: BR-AA-HAPI-064.9, BR-AA-HAPI-064.3
  - Business Outcome: Recovery investigation results are correctly processed through the recovery response path
  - Given: Recovery session with status=completed
  - When: `GetRecoverySessionResult()` returns RecoveryResponse
  - Then:
    - CRD status: Phase transitions correctly, RecoveryStatus populated
- **UT-AA-064-017**: Recovery session lost -- same regeneration cap applies
  - BR: BR-AA-HAPI-064.9, BR-AA-HAPI-064.5
  - Business Outcome: Recovery investigations have the same resilience guarantees as incident investigations
  - Given: Recovery session, `PollSession()` returns 404
  - Then: Same regeneration flow (Generation increment, cap at 5, escalation on exceed)

---

## 2. Unit Tests -- HAPI (Python)

**Location**: `holmesgpt-api/tests/unit/test_session_manager.py`, `holmesgpt-api/tests/unit/test_session_endpoints.py`
**Framework**: pytest
**Existing pattern reference**: `holmesgpt-api/tests/unit/test_health.py`

### 2.1 SessionManager Core

- **UT-HAPI-064-001**: `create_session()` returns UUID and stores session with status "pending"
  - Business Outcome: HAPI can accept an investigation request and return a session handle immediately
  - Given: Valid IncidentRequest
  - Then: UUID returned, session stored with `status="pending"`, `created_at` set
- **UT-HAPI-064-002**: `get_session()` returns session status for existing session
  - Business Outcome: AA can query the progress of an ongoing investigation
  - Given: Session exists with status "investigating"
  - Then: Returns `{status: "investigating"}`
- **UT-HAPI-064-003**: `get_session()` returns None for unknown session_id
  - Business Outcome: AA detects a lost session (HAPI restart) and can trigger regeneration
  - Given: Non-existent session_id
  - Then: Returns None (caller maps to 404)
- **UT-HAPI-064-004**: `get_result()` returns IncidentResponse when status=completed
  - Business Outcome: AA retrieves a complete investigation result to advance the pipeline
  - Given: Completed session with stored result
  - Then: Returns full IncidentResponse
- **UT-HAPI-064-005**: `get_result()` raises error when status != completed
  - Business Outcome: AA is prevented from reading partial results during an active investigation
  - Given: Session with status="pending"
  - Then: Raises appropriate error (caller maps to 409)

### 2.2 TTL Cleanup

- **UT-HAPI-064-006**: Expired completed sessions are removed by cleanup
  - Business Outcome: HAPI memory does not grow unbounded from completed sessions
  - Given: Session completed 31 minutes ago, TTL=30 min
  - Then: Session removed after cleanup sweep
- **UT-HAPI-064-007**: Active investigating sessions are preserved by cleanup
  - Business Outcome: Long-running LLM investigations are not prematurely garbage-collected
  - Given: Session investigating for 5 minutes
  - Then: Session preserved after cleanup sweep
- **UT-HAPI-064-008**: Failed sessions expire like completed sessions
  - Business Outcome: Failed sessions don't leak memory either
  - Given: Session failed 31 minutes ago
  - Then: Session removed after cleanup sweep

### 2.3 Background Execution

- **UT-HAPI-064-009**: Background task transitions session pending -> investigating -> completed
  - Business Outcome: Investigation runs to completion without blocking the HTTP response
  - Given: Session created (pending)
  - When: Background task runs `investigate_issues()`
  - Then: Status transitions: pending -> investigating -> completed, result stored
- **UT-HAPI-064-010**: Background task handles investigate_issues() exception
  - Business Outcome: LLM failures are captured in the session, not lost silently
  - Given: `investigate_issues()` raises RuntimeError
  - When: Background task runs
  - Then: Status transitions to "failed", error message stored in session

### 2.4 HTTP Endpoints (Incident)

- **UT-HAPI-064-011**: `POST /api/v1/incident/analyze` returns 202 with session_id
  - Business Outcome: AA receives immediate acknowledgment and a handle to poll
  - Given: Valid IncidentRequest body
  - Then: HTTP 202 Accepted, `{"session_id": "uuid"}`
- **UT-HAPI-064-012**: `GET /api/v1/incident/session/{id}` returns status
  - Business Outcome: AA can observe investigation progress
  - Given: Existing session
  - Then: HTTP 200, `{"status": "investigating", "progress": "..."}`
- **UT-HAPI-064-013**: `GET /api/v1/incident/session/{id}` returns 404 for unknown
  - Business Outcome: AA detects HAPI restart (lost sessions) via standard HTTP semantics
  - Given: Non-existent session_id
  - Then: HTTP 404 Not Found
- **UT-HAPI-064-014**: `GET /api/v1/incident/session/{id}/result` returns IncidentResponse
  - Business Outcome: AA retrieves the full investigation result after completion
  - Given: Completed session
  - Then: HTTP 200, full IncidentResponse body
- **UT-HAPI-064-015**: `GET /api/v1/incident/session/{id}/result` returns 409 when not completed
  - Business Outcome: AA is told to keep polling if result is not ready yet
  - Given: Pending session
  - Then: HTTP 409 Conflict

### 2.5 HTTP Endpoints (Recovery -- Dedicated)

- **UT-HAPI-064-016**: `POST /api/v1/recovery/analyze` returns 202 with session_id
  - BR: BR-AA-HAPI-064.9
  - Business Outcome: Recovery investigations use the same async pattern as incident investigations
  - Given: Valid RecoveryRequest body
  - Then: HTTP 202 Accepted, `{"session_id": "uuid"}`
- **UT-HAPI-064-017**: `GET /api/v1/recovery/session/{id}` returns status
  - Given: Existing recovery session
  - Then: HTTP 200, `{"status": "investigating"}`
- **UT-HAPI-064-018**: `GET /api/v1/recovery/session/{id}/result` returns RecoveryResponse
  - Given: Completed recovery session
  - Then: HTTP 200, full RecoveryResponse body
- **UT-HAPI-064-019**: `GET /api/v1/recovery/session/{id}/result` returns 409 when not completed
  - Given: Pending recovery session
  - Then: HTTP 409 Conflict

---

## 3. Integration Tests -- AA Controller (Go)

**Location**: `test/integration/aianalysis/session_flow_integration_test.go`
**Framework**: Ginkgo/Gomega BDD
**Infrastructure**: envtest for K8s API + real DS (PostgreSQL, Redis) + real HAPI + Mock LLM -- all started programmatically in Go (per TESTING_GUIDELINES v2.5.2). No other CRD controllers.
**Existing pattern reference**: `test/integration/aianalysis/audit_flow_integration_test.go`, `test/integration/aianalysis/suite_test.go`

**Critical**: Audit tests follow the CORRECT pattern -- trigger business logic (create CRD), wait for controller to process, then validate audit events as side effects via OpenAPI client. NOT direct audit store calls. All audit counts use **exact** `Equal(N)` assertions.

### 3.1 Incident Flow + Audit Trace Validation

- **IT-AA-064-001**: Full submit/poll/result happy path with real HAPI
  - BR: BR-AA-HAPI-064.1, .2, .3, .4
  - Business Outcome: AA controller drives a complete async investigation lifecycle via CRD reconciliation
  - Given: envtest cluster, real HAPI (Mock LLM), AA controller running
  - When: AIAnalysis CRD created with Phase=Investigating
  - Then:
    - AA submits to HAPI (202), polls until completed, fetches result
    - `InvestigationSession` fully populated (ID set, Generation=0, LastPolled set, CreatedAt set)
    - Phase transitions: Investigating -> Analyzing -> Completed
    - `InvestigationSessionReady` Condition=True, Reason=SessionActive at end
- **IT-AA-064-002**: Audit trace exact counts -- happy path submit/poll/result
  - BR: BR-AUDIT-005
  - Business Outcome: Complete audit trail emitted for compliance -- every step of the async flow is traceable
  - Given: Full submit/poll/result cycle completes (from IT-AA-064-001)
  - Then: Query DataStorage via OpenAPI client with `correlationID=aa.UID`, `countEventsByType` with **exact** assertions:
    - `aianalysis.holmesgpt.submit`: exactly 1
    - `aianalysis.holmesgpt.result`: exactly 1 (with `investigationTime > 0` in event_data)
    - `aianalysis.phase.transition`: exactly 3 (Pending->Investigating, Investigating->Analyzing, Analyzing->Completed)
    - `aianalysis.analysis.completed`: exactly 1
    - **Total AA events**: exactly 6
    - Each event validated: `correlationId`, `eventCategory="analysis"`, `eventAction`, `eventOutcome` per ADR-034
- **IT-AA-064-003**: Session regeneration via simulated HAPI restart
  - BR: BR-AA-HAPI-064.5, .7
  - Business Outcome: HAPI restart is transparent to the remediation pipeline -- controller self-heals and completes
  - Given: Session created, then Mock HAPI returns 404 for first poll (simulating restart), then normal on resubmit
  - Then:
    - AA detects 404, increments Generation to 1, clears ID
    - Condition transitions: True/SessionCreated -> False/SessionLost -> True/SessionRegenerated
    - AA resubmits, eventually completes with Phase=Completed
- **IT-AA-064-004**: Audit trace exact counts -- session lost and regeneration
  - BR: BR-AUDIT-005
  - Business Outcome: Session loss and recovery are fully auditable
  - Given: One session lost, one regeneration, then success
  - Then: `countEventsByType` **exact** assertions:
    - `aianalysis.holmesgpt.submit`: exactly 2 (original + resubmit)
    - `aianalysis.holmesgpt.session_lost`: exactly 1 (with `generation=1` in event_data)
    - `aianalysis.holmesgpt.result`: exactly 1
    - `aianalysis.phase.transition`: exactly 3
    - `aianalysis.analysis.completed`: exactly 1
    - **Total AA events**: exactly 8
- **IT-AA-064-005**: Audit trace exact counts -- regeneration cap exceeded (5 losses)
  - BR: BR-AA-HAPI-064.6, BR-AUDIT-005
  - Business Outcome: Persistent HAPI instability failure path is fully auditable
  - Given: Mock HAPI returns 404 for every poll (5 consecutive losses)
  - Then: `countEventsByType` **exact** assertions:
    - `aianalysis.holmesgpt.submit`: exactly 5 (5 attempts)
    - `aianalysis.holmesgpt.session_lost`: exactly 5 (each with incrementing generation 1-5)
    - `aianalysis.holmesgpt.result`: exactly 0 (never completed)
    - `aianalysis.error.occurred`: exactly 1 (SessionRegenerationExceeded)
    - **Total AA events**: exactly 11
    - CRD: Phase=Failed, SubReason="SessionRegenerationExceeded"
- **IT-AA-064-006**: InvestigationSessionReady Condition lifecycle
  - BR: BR-AA-HAPI-064.7
  - Business Outcome: Operators can observe session health via standard K8s Conditions
  - Given: Full cycle with one session loss and regeneration
  - Then: Condition transitions verified at each stage:
    - After submit: Status=True, Reason=SessionCreated
    - During polling: Status=True, Reason=SessionActive
    - After 404: Status=False, Reason=SessionLost
    - After resubmit: Status=True, Reason=SessionRegenerated

### 3.2 Recovery Flow + Audit Trace Validation (Dedicated)

- **IT-AA-064-007**: Recovery submit/poll/result happy path
  - BR: BR-AA-HAPI-064.9
  - Business Outcome: Recovery investigations complete successfully using the async pattern
  - Given: AIAnalysis CRD with `IsRecoveryAttempt=true`
  - When: Controller reconciles
  - Then: Calls recovery endpoints (`/api/v1/recovery/analyze`, `/recovery/session/{id}`, `/recovery/session/{id}/result`), Phase reaches Completed
- **IT-AA-064-008**: Recovery audit trace exact counts
  - BR: BR-AA-HAPI-064.9, BR-AUDIT-005
  - Business Outcome: Recovery investigations produce the same audit completeness as incident investigations
  - Given: Full recovery submit/poll/result cycle
  - Then: `countEventsByType` **exact** assertions:
    - `aianalysis.holmesgpt.submit`: exactly 1 (recovery submit)
    - `aianalysis.holmesgpt.result`: exactly 1 (recovery result)
    - All events carry `isRecoveryAttempt=true` in event_data

### 3.3 K8s Event Observability (DD-EVENT-001 Handoff)

CRD Events team completed issues #71-#73. These integration tests validate session lifecycle K8s events emitted by the InvestigatingHandler (via `WithRecorder`).

**Location**: `test/integration/aianalysis/events_test.go` (within BR-AA-HAPI-064 session context)

- **IT-AA-064-01a**: SessionCreated on happy path
  - BR: BR-AA-HAPI-064, DD-EVENT-001
  - Business Outcome: Operators can observe that a HAPI session was created via K8s Events
  - Given: AIAnalysis CRD created, controller reconciles with `WithSessionMode()`
  - Then: Normal event with reason=`SessionCreated` emitted, message contains "session created"
- **IT-AA-064-01b**: SessionLost on stale session (404)
  - BR: BR-AA-HAPI-064.5, DD-EVENT-001
  - Business Outcome: HAPI restart is visible to operators via Warning K8s Event
  - Given: AIAnalysis has a session, then `InvestigationSession.ID` is set to a fabricated UUID (HAPI returns 404)
  - Then: Warning event `SessionLost` emitted, followed by regeneration producing a new `SessionCreated` event
  - At least 2 `SessionCreated` events (initial + post-regeneration)
- **IT-AA-064-01c**: SessionRegenerationExceeded
  - BR: BR-AA-HAPI-064.6, DD-EVENT-001
  - Business Outcome: Persistent HAPI instability is escalated via Warning K8s Event before AA fails
  - Given: `InvestigationSession.Generation` set to 4 (MaxSessionRegenerations - 1), stale session ID injected
  - Then: Warning events `SessionLost` + `SessionRegenerationExceeded` emitted, Phase=Failed, SubReason="SessionRegenerationExceeded"

---

## 4. Integration Tests -- HAPI (Python)

**Location**: `holmesgpt-api/tests/integration/test_session_manager_integration.py`
**Framework**: pytest
**Infrastructure**: Mock LLM + DataStorage (per existing HAPI integration pattern)
**Existing pattern reference**: `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`

**Critical**: Audit flow follows existing pattern: call business logic directly -> `audit_store.flush()` -> query via `query_audit_events_with_retry()` -> assert **exact** counts.

### 4.1 Incident Session Flow

- **IT-HAPI-064-001**: Submit + poll + result via business logic
  - BR: BR-AA-HAPI-064.1, .2, .3
  - Business Outcome: HAPI processes an investigation asynchronously and produces a complete result
  - Given: Running HAPI with Mock LLM
  - When: Call `analyze_incident()` async (new session-based flow)
  - Then: Session created, background task completes, result retrievable with full IncidentResponse
- **IT-HAPI-064-002**: Session status transitions observable via polling
  - Business Outcome: AA (or any client) can track investigation progress in real time
  - Given: Submit investigation
  - Then: Polling shows `pending -> investigating -> completed` (all three states observed sequentially)
- **IT-HAPI-064-003**: Session audit events emitted with exact counts
  - BR: BR-AUDIT-005, ADR-034
  - Business Outcome: HAPI's async session flow produces the same audit trail as the old sync flow -- no events lost in background execution
  - Given: Full session lifecycle using `oomkilled` Mock LLM scenario (deterministic: 1 tool call, 1 validation attempt)
  - Then: After `audit_store.flush()`, query with retry, **exact** assertions:
    - `aiagent.llm.request`: exactly 1
    - `aiagent.llm.response`: exactly 1
    - `aiagent.llm.tool_call`: exactly 1 (search_workflow_catalog for oomkilled scenario)
    - `aiagent.workflow.validation_attempt`: exactly 1
    - **Total HAPI events**: exactly 4
    - All events share `correlation_id=remediation_id`
    - All events pass ADR-034 schema validation (envelope fields present and typed)
- **IT-HAPI-064-004**: Concurrent sessions complete independently
  - Business Outcome: Multiple AA controllers (or retries) don't interfere with each other
  - Given: Two investigations submitted simultaneously (different remediation_ids)
  - Then: Both complete independently, each with correct results and correct audit events per session

### 4.2 Recovery Session Flow (Dedicated)

- **IT-HAPI-064-005**: Recovery submit + poll + result
  - BR: BR-AA-HAPI-064.9
  - Business Outcome: Recovery investigations work end-to-end through the async session pattern
  - Given: RecoveryRequest submitted
  - Then: Session created, background task calls recovery logic, result retrievable via recovery endpoint
- **IT-HAPI-064-006**: Recovery session audit events emitted with exact counts
  - BR: BR-AA-HAPI-064.9, BR-AUDIT-005
  - Business Outcome: Recovery audit trail is as complete as incident audit trail
  - Given: Full recovery session lifecycle
  - Then: After `audit_store.flush()`, **exact** assertions:
    - `aiagent.llm.request`: exactly 1
    - `aiagent.llm.response`: exactly 1
    - **Total HAPI events**: exactly 2
    - Correlation ID matches recovery remediation_id

---

## 5. E2E Tests

**Strategy**: Per TESTING_GUIDELINES, E2E tests deploy ONLY the controller under test. Other CRD controllers are NOT deployed -- their CRDs are created/simulated directly by the test in Go. The only real dependencies are DS (PostgreSQL, Redis) and, for AA/HAPI, Mock LLM as the sole allowed mocked dependency in CI/CD.

### 5.1 AA E2E

**Location**: `test/e2e/aianalysis/08_session_async_flow_test.go`
**Infrastructure**: Kind cluster with AA controller + DS (PostgreSQL, Redis) + HAPI + Mock LLM. NO other CRD controllers (RO, SP, WE, Notification, Gateway).
**Existing pattern reference**: `test/e2e/aianalysis/03_full_flow_test.go`, `test/e2e/aianalysis/suite_test.go`

- **E2E-AA-064-001**: AA async submit/poll/result flow (incident)
  - BR: BR-AA-HAPI-064.1 through .8
  - Business Outcome: AA controller completes an async investigation in a real K8s environment
  - Given: Kind cluster with AA controller, DS, HAPI, Mock LLM deployed. No other controllers.
  - When: Test creates AIAnalysis CRD directly (no RO/Gateway)
  - Then:
    - AA controller reconciles: submits to HAPI (202), polls session, fetches result
    - `InvestigationSession` populated in CRD status (Generation=0 in happy path)
    - Phase transitions: Pending -> Investigating -> Analyzing -> Completed
    - SelectedWorkflow populated, CompletedAt set
    - Staging auto-approve (ApprovalRequired=false)
  - **Status**: Implemented

### 5.2 HAPI E2E

**Location**: `test/e2e/holmesgpt-api/session_endpoints_test.go`
**Infrastructure**: Kind cluster with HAPI + DS (PostgreSQL, Redis) + Mock LLM. NO CRD controllers.
**Existing pattern reference**: `test/e2e/holmesgpt-api/incident_analysis_test.go`, `test/e2e/holmesgpt-api/audit_pipeline_test.go`

#### Incident Session Endpoints (6 scenarios)

- **E2E-HAPI-064-001**: Incident submit/poll/result for CrashLoopBackOff (happy path)
  - BR: BR-AA-HAPI-064.1, .2, .3
  - Business Outcome: Standard CrashLoopBackOff signal produces confident recommendation via async session
  - Endpoints: POST /incident/analyze (202), GET /incident/session/{id}, GET /incident/session/{id}/result
  - Assertions: session_id non-empty, status=completed, confidence ~0.88, needs_human_review=false
  - **Status**: Implemented
- **E2E-HAPI-064-002**: Incident submit/poll/result for OOMKilled (happy path)
  - BR: BR-AA-HAPI-064.1, .2, .3
  - Business Outcome: OOMKilled signal produces confident recommendation via async session
  - Assertions: confidence ~0.88, needs_human_review=false
  - **Status**: Implemented
- **E2E-HAPI-064-003**: No workflow found via session (MOCK_NO_WORKFLOW_FOUND)
  - BR: BR-AA-HAPI-064.1, BR-HAPI-197
  - Business Outcome: Escalation to human review with clear reason via session flow
  - Assertions: needs_human_review=true, human_review_reason=NoMatchingWorkflows, confidence=0, selected_workflow=nil
  - **Status**: Implemented
- **E2E-HAPI-064-004**: Low confidence via session (MOCK_LOW_CONFIDENCE)
  - BR: BR-AA-HAPI-064.1, BR-HAPI-197
  - Business Outcome: Low-confidence recommendation returned for AA threshold evaluation
  - Assertions: needs_human_review=false (HAPI doesn't enforce thresholds), confidence <0.5, alternative_workflows present
  - **Status**: Implemented
- **E2E-HAPI-064-005**: Max retries exhausted via session (MOCK_MAX_RETRIES_EXHAUSTED)
  - BR: BR-AA-HAPI-064.1, BR-HAPI-197
  - Business Outcome: Complete validation history for debugging after max retries
  - Assertions: needs_human_review=true, human_review_reason=LlmParsingError, 3 validation attempts, sequential attempt numbers
  - **Status**: Implemented
- **E2E-HAPI-064-006**: Session status transitions observable during investigation
  - BR: BR-AA-HAPI-064.2
  - Business Outcome: Session status is queryable and reaches terminal state
  - Assertions: "completed" status observed (intermediate states may not be observable with Mock LLM speed)
  - **Status**: Implemented

#### Recovery Session Endpoints (3 scenarios)

- **E2E-HAPI-064-007**: Recovery submit/poll/result happy path
  - BR: BR-AA-HAPI-064.9
  - Business Outcome: Recovery session endpoints work identically to incident endpoints
  - Endpoints: POST /recovery/analyze (202), GET /incident/session/{id}, GET /recovery/session/{id}/result
  - Assertions: can_recover=true, selected_workflow present, confidence ~0.85
  - **Status**: Implemented
- **E2E-HAPI-064-008**: Recovery not reproducible via session (MOCK_NOT_REPRODUCIBLE)
  - BR: BR-AA-HAPI-064.9, BR-HAPI-212
  - Business Outcome: When issue self-resolved, recovery indicates no action needed
  - Assertions: can_recover=false, needs_human_review=false, selected_workflow=null, confidence ~0.85
  - **Status**: Implemented
- **E2E-HAPI-064-009**: No recovery workflow found via session (MOCK_NO_WORKFLOW_FOUND)
  - BR: BR-AA-HAPI-064.9, BR-HAPI-197
  - Business Outcome: Escalation to human review when no recovery workflow available
  - Assertions: can_recover=true, needs_human_review=true, human_review_reason=NoMatchingWorkflows
  - **Status**: Implemented

#### Complete Lifecycle (1 scenario)

- **E2E-HAPI-064-010**: Full incident then recovery via session endpoints
  - BR: BR-AA-HAPI-064.1, .9
  - Business Outcome: End-to-end incident → recovery lifecycle using session endpoints
  - Flow: Submit incident → poll → result → simulate failure → submit recovery → poll → result
  - Assertions: both sessions complete, session IDs distinct, recovery selects workflow
  - **Status**: Implemented

#### Session Error Handling (2 scenarios)

- **E2E-HAPI-064-011**: Poll non-existent session returns 404
  - BR: BR-AA-HAPI-064.2, BR-AA-HAPI-064.5
  - Business Outcome: Invalid session IDs return clear HTTP 404 errors
  - Assertions: APIError with StatusCode=404
  - **Status**: Implemented
- **E2E-HAPI-064-012**: Result for non-existent session returns 404
  - BR: BR-AA-HAPI-064.3
  - Business Outcome: Result retrieval for invalid sessions returns clear HTTP 404 errors
  - Assertions: APIError with StatusCode=404
  - **Status**: Implemented

---

## 6. Mock LLM Session Support

**Location**: `test/services/mock-llm/src/server.py` (extend existing)
**Purpose**: Enable integration and E2E tests to work with session-based HAPI

Mock LLM itself does not need session endpoints (HAPI manages sessions internally). However, Mock LLM must remain compatible with the background execution model:

- `investigate_issues()` runs in `asyncio.to_thread()` -- Mock LLM's `/v1/chat/completions` must respond normally (no session awareness needed at LLM level)
- No changes to Mock LLM required unless HAPI's background task changes the call pattern

---

## Test File Locations

| Tier        | Service | File Path                                                             |
| ----------- | ------- | --------------------------------------------------------------------- |
| Unit        | AA      | `test/unit/aianalysis/investigating_handler_session_test.go`          |
| Unit        | HAPI    | `holmesgpt-api/tests/unit/test_session_manager.py`                    |
| Unit        | HAPI    | `holmesgpt-api/tests/unit/test_session_endpoints.py`                  |
| Integration | AA      | `test/integration/aianalysis/session_flow_integration_test.go`        |
| Integration | HAPI    | `holmesgpt-api/tests/integration/test_session_manager_integration.py` |
| E2E         | AA      | `test/e2e/aianalysis/08_session_async_flow_test.go`                   |
| E2E         | HAPI    | `test/e2e/holmesgpt-api/session_endpoints_test.go`                    |

---

## Acceptance Criteria

- All 17 AA unit tests implemented and passing (UT-AA-064-001 through 017)
- All 19 HAPI unit tests implemented and passing (UT-HAPI-064-001 through 019)
- All 8 AA integration tests passing with **exact** audit count validation (IT-AA-064-001 through 008)
- All 6 HAPI integration tests passing with **exact** audit count validation (IT-HAPI-064-001 through 006)
- All 13 E2E tests passing (E2E-AA-064-001, E2E-HAPI-064-001 through 012)
- **Existing HAPI unit tests refactored** to async-aware pattern per "Async-First Endpoints" decision (12 tests in 2 files)
- **Full existing HAPI unit test suite passes** after async migration (`make test-unit-holmesgpt-api`)
- No new lint errors introduced
- No `time.Sleep()` in any test (use `Eventually()`)
- No `Skip()` or `XIt()` in any test
- Audit tests trigger business logic, NOT direct audit store calls (TESTING_GUIDELINES anti-pattern)
- All audit event counts validated with **exact** `Equal(N)` assertions -- no approximate matchers
- Test execution SLA: Unit <5s, Integration <3min, E2E <10min

---

## Dependencies

- `MockHolmesGPTClient` must be extended with `SubmitInvestigation`, `SubmitRecoveryInvestigation`, `PollSession`, `GetSessionResult`, `GetRecoverySessionResult` methods
- `AuditClientInterface` must be extended with `RecordHolmesGPTSubmit`, `RecordHolmesGPTResult`, `RecordHolmesGPTSessionLost` methods
- HAPI `SessionManager` must be implemented before integration tests can run
- Mock LLM must support session-based HAPI (no changes expected -- LLM layer is session-unaware)
- **Existing HAPI unit tests** (`test_recovery.py`, `test_sdk_availability.py`) must be refactored to async-aware pattern (submit 202 → GET result 200) -- see "Async-First Endpoints" design decision in Scope section

---

## BR Mapping Summary

| BR Requirement                              | Unit Scenarios                  | Integration Scenarios                | E2E Scenarios |
| ------------------------------------------- | ------------------------------- | ------------------------------------ | ------------- |
| BR-AA-HAPI-064.1 (Async Submit)             | AA-001, 002, 015                | AA-001, 007                          | AA-001        |
| BR-AA-HAPI-064.2 (Session Polling)          | AA-003, 004, 005, 006, 007      | AA-001, 006                          | AA-001        |
| BR-AA-HAPI-064.3 (Result Retrieval)         | AA-005, 016                     | AA-001, 007                          | AA-001        |
| BR-AA-HAPI-064.4 (InvestigationSession CRD) | AA-001, 002                     | AA-001                               | AA-001        |
| BR-AA-HAPI-064.5 (Regeneration)             | AA-002, 008, 009, 017           | AA-003, 004                          | -             |
| BR-AA-HAPI-064.6 (Regeneration Cap)         | AA-010                          | AA-005                               | -             |
| BR-AA-HAPI-064.7 (Condition)                | AA-001                          | AA-006                               | -             |
| BR-AA-HAPI-064.8 (Requeue Backoff)          | AA-003, 004, 007                | AA-001                               | AA-001        |
| BR-AA-HAPI-064.9 (Recovery)                 | AA-015, 016, 017                | AA-007, 008                          | AA-003        |
| BR-AA-HAPI-064.10 (Timeout Removal)         | AA-014                          | -                                    | AA-001        |
| BR-AUDIT-005 (Audit Traces)                 | AA-001, 005, 008 (side effects) | AA-002, 004, 005, 008; HAPI-003, 006 | AA-002        |

---

## Compliance Sign-Off

### Test Execution Summary

| Test Category       | Tests Passed | Tests Failed | Coverage |
| ------------------- | ------------ | ------------ | -------- |
| AA Unit             | 0 / 17       | 0            | 0%       |
| HAPI Unit           | 0 / 19       | 0            | 0%       |
| AA Integration      | 0 / 8        | 0            | 0%       |
| HAPI Integration    | 0 / 6        | 0            | 0%       |
| AA E2E              | 0 / 3        | 0            | 0%       |
| HAPI E2E            | 0 / 2        | 0            | 0%       |
| **Total**           | **0 / 55**   | **0**        | **0%**   |

### Approval

| Role      | Name | Date | Signature |
| --------- | ---- | ---- | --------- |
| Developer |      |      |           |
| Reviewer  |      |      |           |
| Team Lead |      |      |           |

---

## References

- [BR-AA-HAPI-064](../../../docs/requirements/BR-AA-HAPI-064-session-based-pull-design.md)
- [DD-AA-HAPI-064](../../../docs/architecture/decisions/DD-AA-HAPI-064-session-based-pull-design.md)
- [DD-EVENT-001](../../../docs/architecture/decisions/DD-EVENT-001-controller-event-registry.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
