# CP-3: Session & Audit Gate — Test Case Specifications

**Checkpoint**: CP-3
**Gate Type**: Unit + Integration Tests (CRITICAL)
**Total Checks**: 34
**Merge Criteria**: All 34 tests pass, session/audit code coverage >=80%
**PR**: PR3 (kubernaut_investigate tool, cancel+reconstruct, NotificationBus, takeover)

---

## Overview

CP-3 validates the core interactive session mechanics: session hijacking prevention, dynamic takeover with cancel+reconstruct, NotificationBus delivery, timeout management, and audit completeness. This is the functional heart of #703.

---

## Test Environment

- **Unit Package**: `test/unit/kubernautagent/mcp/session/`
- **Integration Package**: `test/integration/kubernautagent/mcp/`
- **Framework**: Ginkgo/Gomega BDD
- **Key Imports**:
  ```go
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/session"
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/notifications"
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
  "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
  "k8s.io/client-go/kubernetes/fake"
  coordinationv1 "k8s.io/api/coordination/v1"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  ```
- **Mocks**:
  - `MockLLMClient`: Configurable turn duration, cancellable
  - `MockDSClient`: Returns configurable audit event history
  - `MockLeaseClient`: Simulates K8s Lease acquire/release/contention
  - `audit.InMemoryAuditStore`: Captures emitted events for assertion
- **Helpers**:
  - `createSessionWithUser(user, rrID)`: Creates an authenticated interactive session
  - `simulateLLMTurn(ctx, duration)`: Simulates an autonomous LLM turn with cancellation support
  - `waitForAuditEvent(store, eventType, timeout)`: Waits for specific audit event

---

## SESS: Session Lifecycle Scenarios

### UT-KA-SESS-001: Session hijacking — different user attempts to control session

**BR**: BR-INTERACTIVE-004, BR-INTERACTIVE-005
**Type**: Unit
**Category**: Security
**Check ID**: SESS-01

**Description**: After user-A takes over an investigation, user-B attempts to send a tool call to the same session. The system must reject user-B.

**Preconditions**:
- Interactive session created by user-A (Lease held by user-A)
- user-B is authenticated but is a different identity

**Steps**:
1. Create interactive session: `session = createSessionWithUser("user-a@corp", "rr-123")`
2. Assert session.ActingUser == "user-a@corp"
3. Attempt tool call with context of `"user-b@corp"` to the same session
4. Assert: rejection with error code `lease_held`
5. Assert: error message includes who holds the session: "Session controlled by user-a@corp since {time}"
6. Assert: user-B's request is NOT processed
7. Assert: audit event emitted for rejected access

**Acceptance Criteria**:
- Different user rejected (not just hidden)
- Error message includes current holder identity (intentional for UX)
- Original session unaffected
- Audit trail captures attempted hijack

---

### UT-KA-SESS-002: Empty identity cannot create or control sessions

**BR**: BR-INTERACTIVE-002, BR-INTERACTIVE-005
**Type**: Unit
**Category**: Security
**Check ID**: SESS-02

**Description**: A request with empty/nil user identity (middleware bug) cannot create or interact with sessions.

**Preconditions**:
- Session manager available
- Context with no user identity set

**Steps**:
1. Create context with empty user: `ctx = context.WithValue(ctx, auth.UserContextKey, "")`
2. Attempt session creation: `session, err = manager.CreateInteractiveSession(ctx, "rr-123")`
3. Assert: error returned ("user identity required")
4. Attempt with nil context value: `ctx = context.Background()` (no user key)
5. Assert: error returned
6. Assert: no session created in store

**Acceptance Criteria**:
- Empty string user rejected
- Nil/missing user context rejected
- No session with empty ActingUser can exist
- Error is explicit (not nil-pointer panic)

---

### UT-KA-SESS-003: Concurrent Lease acquisition contention

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Concurrency
**Check ID**: SESS-03

**Description**: Two users attempt to takeover the same investigation simultaneously. Only one succeeds (K8s Lease guarantees).

**Preconditions**:
- MockLeaseClient that simulates contention (first acquirer wins)
- Two goroutines attempting simultaneous takeover

**Steps**:
1. Start autonomous investigation on "rr-123"
2. Launch goroutine-A: `takeover(ctx-user-a, "rr-123", "action: takeover")`
3. Launch goroutine-B: `takeover(ctx-user-b, "rr-123", "action: takeover")` (simultaneously)
4. Wait for both to complete
5. Assert: exactly ONE succeeds (Lease acquired)
6. Assert: other gets `lease_held` error with winner's identity
7. Assert: no partial state (no half-created sessions)
8. Assert: audit events show one success + one contention

**Acceptance Criteria**:
- Exactly one winner (deterministic, not both succeed)
- Loser gets clear error with winner identity
- No deadlock or hang
- Lease state consistent after contention

---

### IT-KA-SESS-001: Lease orphan recovery after simulated crash

**BR**: BR-INTERACTIVE-005
**Type**: Integration
**Category**: Fault Tolerance
**Check ID**: SESS-04

**Description**: After a KA pod "crash" (Lease holder disappears without releasing), the Lease expires naturally and another user can acquire it.

**Preconditions**:
- Real fake K8s clientset (not mock) with Lease support
- Lease configured with 30s duration, 15s renew

**Steps**:
1. User-A acquires Lease for "rr-123" (creates Coordination/v1 Lease)
2. Assert Lease exists with holder "user-a"
3. Simulate crash: stop Lease renewal (don't call renewLease)
4. Wait 35 seconds (Lease duration + grace)
5. User-B attempts to acquire same Lease
6. Assert: user-B succeeds (Lease expired, can be re-acquired)
7. Assert: user-B's session is independent (no stale state from user-A)

**Acceptance Criteria**:
- Lease expires after duration without renewal
- New user can acquire expired Lease
- No stale session state from crashed holder
- Total recovery time <= Lease duration + 5s grace

---

### UT-KA-SESS-004: Inactivity timeout releases session

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Timeout
**Check ID**: SESS-05

**Description**: If no tool call arrives within the inactivity window (default 10m), the session is automatically released and autonomous mode resumes.

**Preconditions**:
- Interactive session active with inactivity timeout = 10m (configurable, use 100ms for test)
- Timer/ticker mechanism for inactivity detection

**Steps**:
1. Create interactive session with inactivity timeout = 100ms (test acceleration)
2. Assert session is active
3. Do NOT send any tool calls
4. Wait 150ms (past inactivity timeout)
5. Assert session is completed with reason "inactivity_timeout"
6. Assert Lease released
7. Assert audit event: `aiagent.interactive.timeout` with reason "inactivity"
8. Assert autonomous reconstruction triggered (or flag set for reconstruction)

**Acceptance Criteria**:
- Session auto-completes after inactivity timeout
- Lease released (not orphaned)
- Audit event emitted with timeout reason
- Autonomous resume triggered

---

### UT-KA-SESS-005: Absolute timeout warning at T-10m and T-2m

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: UX
**Check ID**: SESS-06

**Description**: MCP `notifications/progress` messages sent at T-10m and T-2m before global timeout expires.

**Preconditions**:
- Interactive session with global timeout = 1h (use 1s for test)
- NotificationBus or MCP notification channel available
- Time-based trigger mechanism

**Steps**:
1. Create interactive session (global timeout remaining = 1000ms for test)
2. Subscribe to MCP notifications
3. Wait until T-100ms (simulating T-10m in real time)
4. Assert: notification received with `"type": "timeout_warning", "remaining": "10m"`
5. Wait until T-20ms (simulating T-2m)
6. Assert: notification received with `"type": "timeout_warning", "remaining": "2m"`
7. Wait until timeout
8. Assert: session ended with reason "global_timeout"

**Acceptance Criteria**:
- Two distinct warning notifications at correct times
- Notifications include remaining time
- Notification format matches MCP `notifications/progress` spec
- Final timeout ends session as expected

---

### UT-KA-SESS-006: MCP disconnect triggers cleanup

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Lifecycle
**Check ID**: SESS-07

**Description**: When the MCP transport detects client disconnect (TCP close), session cleanup is triggered: Lease released, audit event emitted, autonomous reconstruction signaled.

**Preconditions**:
- Active interactive session
- Disconnect detection mechanism (context cancellation or connection close callback)

**Steps**:
1. Create active interactive session
2. Assert: session active, Lease held
3. Trigger disconnect (cancel session context or call disconnect handler)
4. Assert: session marked as completed (CompletedAt set)
5. Assert: Lease released
6. Assert: audit event `aiagent.interactive.completed` with reason "disconnect"
7. Assert: reconstruction signal sent (autonomous resume)

**Acceptance Criteria**:
- Cleanup is immediate (not waiting for next tick)
- Lease released (idempotent if already released)
- Audit event captures disconnect
- Reconstruction triggered

---

### UT-KA-SESS-007: Double-complete is idempotent (no panic)

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Defensive
**Check ID**: SESS-08

**Description**: Calling session.Complete() twice (race between timeout and disconnect) does not panic or produce duplicate audit events.

**Preconditions**:
- Active interactive session
- Audit store capturing events

**Steps**:
1. Create active interactive session
2. Call `session.Complete("disconnect")` → assert success
3. Call `session.Complete("timeout")` → assert no error (idempotent)
4. Assert: only ONE `aiagent.interactive.completed` audit event emitted
5. Assert: no panic, no nil pointer dereference
6. Assert: session state is "completed" (not corrupted)

**Acceptance Criteria**:
- Second Complete() is no-op (idempotent)
- Exactly one audit event (not duplicated)
- No panic under any ordering
- First reason wins (second reason discarded)

---

### UT-KA-SESS-008: Tool call on completed session rejected

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Error Handling
**Check ID**: SESS-09

**Description**: After session is completed (timeout/disconnect), subsequent tool calls are rejected with clear error.

**Preconditions**:
- Session in completed state

**Steps**:
1. Create and complete a session (simulate disconnect)
2. Attempt tool call on the completed session
3. Assert: error with code `session_not_found` or `investigation_ended`
4. Assert: no state mutation on the completed session
5. Assert: no audit event from the rejected call (session is already done)

**Acceptance Criteria**:
- Clear error code (not generic 500)
- Human message guides user: "Session not found or expired"
- No state corruption on dead session
- Client can reconnect with fresh session

---

## TAKE: Dynamic Takeover Scenarios

### IT-KA-TAKE-001: Takeover mid-LLM-turn (autonomous completes turn before cancel)

**BR**: BR-INTERACTIVE-004
**Type**: Integration
**Category**: Correctness — Race Condition
**Check ID**: TAKE-01

**Description**: User sends `action: takeover` while the autonomous LLM is mid-turn. The current turn must complete (not be aborted mid-response), then autonomous is cancelled.

**Preconditions**:
- Autonomous investigation running with mock LLM (takes 500ms per turn)
- User authenticated and authorized for takeover

**Steps**:
1. Start autonomous investigation (mock LLM takes 500ms per turn)
2. Wait until LLM turn is in-progress (100ms into turn)
3. User sends `action: takeover`
4. Assert: takeover acknowledged (response to user)
5. Wait for current LLM turn to complete (remaining ~400ms)
6. Assert: autonomous turn result saved to audit store (not lost)
7. Assert: autonomous cancelled after turn completes (context.Done)
8. Assert: user is now driver (session.ActingUser changed)
9. Assert: audit sequence: `llm.response` → `session.suspended` → `interactive.started`

**Acceptance Criteria**:
- Current LLM turn completes fully (no truncation)
- Turn result saved to audit (not lost work)
- Cancellation happens AFTER turn, not during
- Audit sequence is ordered and complete

---

### UT-KA-TAKE-001: Takeover timing — request arrives between turns

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Correctness
**Check ID**: TAKE-02

**Description**: Takeover request arrives in the gap between autonomous turns (not during LLM call). Cancellation is immediate.

**Preconditions**:
- Autonomous investigation in "thinking" state between turns
- No active LLM call

**Steps**:
1. Start autonomous investigation, let it complete one turn
2. During inter-turn gap: user sends `action: takeover`
3. Assert: autonomous cancelled immediately (no waiting)
4. Assert: user takes over within <50ms
5. Assert: no extra LLM turn started after takeover received

**Acceptance Criteria**:
- Immediate cancellation between turns
- No unnecessary LLM turn started
- Takeover latency < 50ms when between turns

---

### IT-KA-TAKE-002: Rapid connect/disconnect stability

**BR**: BR-INTERACTIVE-004
**Type**: Integration
**Category**: Stability
**Check ID**: TAKE-03

**Description**: User connects, takes over, immediately disconnects, repeats 5 times. System must be stable (no goroutine leaks, no Lease orphans, reconstruction works each time).

**Preconditions**:
- Full session manager with Lease integration
- Mock DS client for reconstruction

**Steps**:
1. Start autonomous investigation
2. Repeat 5 times:
   a. User connects and sends `action: takeover`
   b. Assert: takeover succeeds
   c. Immediately disconnect (within 10ms)
   d. Assert: session cleaned up, Lease released
   e. Assert: autonomous reconstruction triggered
   f. Wait for reconstruction to complete
3. After all 5 cycles: assert investigation is in autonomous mode
4. Assert: goroutine count stable (no leaks)
5. Assert: 5 pairs of `interactive.started` + `interactive.completed` events

**Acceptance Criteria**:
- All 5 cycles complete without error
- Zero goroutine leaks (count before == count after)
- Zero Lease orphans
- Each reconstruction creates independent session
- Audit trail has complete 5-cycle history

---

### UT-KA-TAKE-002: Resume identity is always KA SA (not user)

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Security
**Check ID**: TAKE-04

**Description**: When autonomous mode resumes after user disconnect, the new session runs as KA SA — the user's identity is NOT retained.

**Preconditions**:
- Interactive session completed (user-a disconnected)
- Reconstruction logic available

**Steps**:
1. Interactive session exists with `ActingUser: "user-a@corp"`
2. User disconnects → reconstruction triggered
3. Assert: new autonomous session's `ActingUser` is `"system:serviceaccount:kubernaut:kubernaut-agent"` (KA SA)
4. Assert: new session ID is different from interactive session ID
5. Assert: K8s API calls use KA SA identity (not user-a)

**Acceptance Criteria**:
- Resumed session uses KA SA identity (not lingering user identity)
- New session ID (cancel+reconstruct, not resume)
- K8s ImpersonationConfig uses KA SA
- Audit events show identity transition

---

### IT-KA-TAKE-003: Combined conversation coherence after takeover

**BR**: BR-INTERACTIVE-006
**Type**: Integration
**Category**: Correctness
**Check ID**: TAKE-05

**Description**: After takeover, the user's LLM context includes the autonomous findings. The combined conversation is coherent (no gaps, no duplicates).

**Preconditions**:
- Autonomous investigation has completed 3 turns (findings stored in DS)
- Mock DS client returns those 3 turns as audit events
- User takes over

**Steps**:
1. Start autonomous investigation → 3 LLM turns complete (stored in DS audit)
2. User sends `action: takeover`
3. Auto-inject queries DS for correlation_id audit events
4. Assert: user's LLM context includes summary of all 3 autonomous turns
5. Assert: no duplicate content (auto-inject de-duplicates)
6. Assert: conversation order preserved (turn 1, 2, 3, then user's message)
7. User sends a tool call referencing autonomous finding → LLM can access it

**Acceptance Criteria**:
- All autonomous findings available to user's LLM
- No duplicates in injected context
- Chronological ordering preserved
- LLM can reference prior autonomous work

---

### IT-KA-TAKE-004: Auto-inject correctness (DS query + context building)

**BR**: BR-INTERACTIVE-006
**Type**: Integration
**Category**: Correctness
**Check ID**: TAKE-06

**Description**: Auto-inject queries DS by `correlation_id`, excludes own `session_id`, and builds LLM context correctly.

**Preconditions**:
- DS contains audit events from 2 prior sessions (autonomous sess-01, user-b sess-02)
- New user-a is taking over (sess-03)
- DS client configured to return events by correlation_id

**Steps**:
1. Seed mock DS with:
   - 3 events from sess-01 (autonomous, correlation: "rr-123")
   - 2 events from sess-02 (user-b, correlation: "rr-123")
2. User-A takes over (sess-03, correlation: "rr-123")
3. Auto-inject queries DS: `correlation_id=rr-123`
4. Assert: receives all 5 events (from sess-01 + sess-02)
5. Assert: excludes own session (sess-03) via client-side filter
6. Assert: context built includes sess-01 and sess-02 findings
7. Assert: events ordered by timestamp

**Acceptance Criteria**:
- DS query uses correlation_id (not session_id)
- Client-side exclusion of own session_id
- All prior sessions' events included
- Chronological ordering in built context

---

### UT-KA-TAKE-003: DS query failure during auto-inject (graceful degradation)

**BR**: BR-INTERACTIVE-008
**Type**: Unit
**Category**: Fault Tolerance
**Check ID**: TAKE-07

**Description**: If DS is unavailable during auto-inject, takeover proceeds with empty prior context (best-effort). User can still drive the investigation.

**Preconditions**:
- Mock DS client configured to return error (connection refused)
- User sends takeover

**Steps**:
1. Configure mock DS to return error on query
2. User sends `action: takeover`
3. Assert: takeover SUCCEEDS (not blocked by DS failure)
4. Assert: user's LLM context has NO prior findings (empty inject)
5. Assert: warning logged: "auto-inject failed: DS unavailable"
6. Assert: audit event includes `"auto_inject_failed": true`
7. User can send tool calls normally (session is functional)

**Acceptance Criteria**:
- Takeover not blocked by DS failure (best-effort)
- Session functional without prior context
- Warning logged (not silently swallowed)
- Audit captures the degraded state

---

### UT-KA-TAKE-004: Concurrent takeover rejection (second driver)

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Concurrency
**Check ID**: TAKE-08

**Description**: While user-A is driving, user-C attempts `action: takeover`. Rejected because Lease is already held.

**Preconditions**:
- User-A is active driver (Lease held)
- User-C is authenticated with Driver RBAC

**Steps**:
1. User-A has active session on "rr-123" (Lease held)
2. User-C sends `action: takeover` for "rr-123"
3. Assert: rejected with error code `lease_held`
4. Assert: error includes holder: "Session controlled by user-a@corp since {time}"
5. Assert: user-A's session unaffected
6. Assert: user-C can still connect as Observer (read-only NotificationBus)

**Acceptance Criteria**:
- Second driver rejected (single-driver guarantee)
- Error includes current holder identity
- First driver unaffected
- Observer role still available to rejected user

---

### IT-KA-TAKE-005: Timeout extension on takeover (bounded by global)

**BR**: BR-INTERACTIVE-005
**Type**: Integration
**Category**: Correctness
**Check ID**: TAKE-09

**Description**: When a user takes over, the investigation timeout is extended (within global bounds). Late takeovers get less time.

**Preconditions**:
- RR created 40 minutes ago (global timeout 1h)
- User takes over (20m remaining)
- Timeout extension logic integrated with RO

**Steps**:
1. Create RR with global timeout 1h, elapsed: 40m
2. User sends `action: takeover`
3. Assert: session timeout = 20m (remaining global, not a fresh 1h)
4. Assert: `InteractiveSessionInfo` on AA status reflects remaining time
5. Assert: timeout warnings scheduled at T-10m and T-2m from the 20m remaining
6. Repeat with elapsed: 55m → assert only 5m remaining for session

**Acceptance Criteria**:
- Timeout bounded by remaining global time
- No fresh timeout window on takeover
- Warnings adjusted to remaining time
- CRD status reflects actual remaining

---

### UT-KA-TAKE-005: Explicit takeover required (first message is not takeover)

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Security
**Check ID**: TAKE-10

**Description**: A user's first regular message (without `action: takeover`) does NOT trigger takeover. User observes but does not disrupt autonomous mode.

**Preconditions**:
- Autonomous investigation running
- User connected (Observer role)
- User sends regular tool call WITHOUT `action: takeover`

**Steps**:
1. Autonomous investigation running on "rr-123"
2. User connects as Observer (gets NotificationBus events)
3. User sends: `{"tool": "kubernaut_investigate", "arguments": {"rr_id": "rr-123", "message": "what's the status?"}}`
4. Assert: autonomous mode NOT interrupted
5. Assert: user gets informational response about current status (Observer mode)
6. Assert: no Lease acquired, no session created for user as Driver
7. User then sends: `{"tool": "kubernaut_investigate", "arguments": {"rr_id": "rr-123", "action": "takeover"}}`
8. Assert: NOW takeover occurs

**Acceptance Criteria**:
- Regular message ≠ takeover (explicit action required)
- Observer can query status without disruption
- Autonomous mode continues until explicit `action: takeover`
- Prevents accidental takeover from stray messages

---

### UT-KA-TAKE-006: Timeout visibility — T-10m and T-2m notifications

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: UX
**Check ID**: TAKE-11

**Description**: Active interactive user receives MCP progress notifications at T-10m and T-2m before session timeout.

**Preconditions**:
- Active interactive session with remaining time = 200ms (test-accelerated)
- Notification channel available

**Steps**:
1. Create session with remaining global time = 200ms
2. Subscribe to session notifications
3. At T-130ms (simulating T-10m): assert notification: `{"type": "timeout_warning", "remaining_minutes": 10, "message": "Session will end in 10 minutes"}`
4. At T-160ms (simulating T-2m): assert notification: `{"type": "timeout_critical", "remaining_minutes": 2, "message": "Session will end in 2 minutes"}`
5. At T-200ms: session ends

**Acceptance Criteria**:
- Two distinct notifications sent at correct relative times
- Notification includes remaining time in human-readable form
- Notifications don't block or delay session operations
- MCP notifications/progress format compliant

---

### UT-KA-TAKE-007: Inactivity warning at T-2m before cutoff

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: UX
**Check ID**: TAKE-12

**Description**: User receives inactivity warning 2 minutes before the 10m inactivity cutoff (at 8m of inactivity).

**Preconditions**:
- Active session with inactivity timeout = 100ms (test-accelerated)
- No tool calls being made

**Steps**:
1. Create active session (inactivity timeout = 100ms)
2. Do not send any tool calls
3. At 80ms (simulating 8m / T-2m before cutoff): assert inactivity warning notification
4. Assert message: "No activity for 8 minutes. Session will end in 2 minutes if no tool call received."
5. At 100ms: session ends due to inactivity

**Acceptance Criteria**:
- Inactivity warning at 80% of timeout (T-2m before 10m cutoff)
- Warning includes elapsed idle time and remaining time
- If user sends tool call after warning, timer resets (session continues)
- Warning does not reset the inactivity timer itself

---

### UT-KA-TAKE-008: NotificationBus message ordering preserved

**BR**: BR-INTERACTIVE-006
**Type**: Unit
**Category**: Correctness
**Check ID**: TAKE-13

**Description**: Events published to NotificationBus are received by subscribers in publication order (FIFO).

**Preconditions**:
- NotificationBus instantiated
- Subscriber connected

**Steps**:
1. Create NotificationBus
2. Subscribe to correlation "rr-123"
3. Publish 100 events in sequence (numbered 1-100)
4. Read all 100 events from subscriber channel
5. Assert: received order matches publication order (1, 2, 3, ..., 100)
6. Assert: no events lost (count == 100)
7. Assert: no duplicates

**Acceptance Criteria**:
- FIFO ordering guaranteed
- Zero message loss under normal conditions
- Zero duplicates
- Works with single subscriber

---

### UT-KA-TAKE-009: NotificationBus slow consumer doesn't block publisher

**BR**: BR-INTERACTIVE-006
**Type**: Unit
**Category**: Performance
**Check ID**: TAKE-14

**Description**: A slow subscriber (not reading from channel) does not block the publisher or other subscribers. Bounded buffer + drop policy applies.

**Preconditions**:
- NotificationBus with buffer size = 10
- Slow subscriber (never reads)
- Fast subscriber (reads immediately)

**Steps**:
1. Create NotificationBus (buffer = 10)
2. Subscribe slow-consumer (never reads from channel)
3. Subscribe fast-consumer (reads immediately)
4. Publish 20 events
5. Assert: publisher does NOT block (completes within 10ms)
6. Assert: fast-consumer received all 20 events
7. Assert: slow-consumer's channel has 10 events (buffer full, 10 dropped)
8. Assert: metric `kubernaut_interactive_notifications_dropped_total` incremented by 10

**Acceptance Criteria**:
- Publisher never blocks (bounded channel, non-blocking send)
- Fast consumers unaffected by slow consumers
- Drop count tracked via metric
- Buffer overflow drops oldest (or newest, document which)

---

## AUD: Audit Completeness

### UT-KA-AUD-001: session_id present on ALL audit events

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-01

**Description**: Every `AuditEvent` emitted by KA has `session_id` as a mandatory top-level field. No event can be emitted without it.

**Preconditions**:
- `audit.NewEvent()` function (the breaking change from PR2)
- Multiple event types exercised

**Steps**:
1. Call `audit.NewEvent(EventTypeLLMRequest, sessionID, correlationID, data)` with valid session_id
2. Assert: event has `SessionID` field set
3. Attempt `audit.NewEvent(EventTypeLLMRequest, "", correlationID, data)` (empty session_id)
4. Assert: error returned OR panic (session_id is mandatory)
5. Emit 5 different event types → assert ALL have session_id populated

**Acceptance Criteria**:
- `session_id` is mandatory parameter in `NewEvent` signature
- Empty session_id is compile-time or runtime error
- All event types carry session_id
- No code path can emit event without session_id

---

### UT-KA-AUD-002: acting_user set on interactive events

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-02

**Description**: All audit events emitted during interactive sessions have `acting_user` set to the authenticated user (not KA SA).

**Preconditions**:
- Interactive session active with user "user-a@corp"
- Audit emitter available

**Steps**:
1. Create interactive session (ActingUser: "user-a@corp")
2. Emit `EventTypeInteractiveStarted` → assert `acting_user == "user-a@corp"`
3. Emit `EventTypeLLMRequest` during interactive → assert `acting_user == "user-a@corp"`
4. Emit `EventTypeInteractiveK8sCall` → assert `acting_user == "user-a@corp"`
5. After disconnect, emit autonomous event → assert `acting_user == "system:serviceaccount:kubernaut:kubernaut-agent"`

**Acceptance Criteria**:
- Interactive events: user identity
- Autonomous events: KA SA identity
- Identity transition visible in audit trail
- No event with wrong identity attribution

---

### UT-KA-AUD-003: EventTypeInteractiveK8sCall emitted for every impersonated call

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-03

**Description**: Every K8s API call made during interactive mode (impersonated under user identity) emits `aiagent.interactive.k8s_call` with resource, verb, namespace, and result.

**Preconditions**:
- Interactive session active
- Impersonated K8s client making calls
- Audit store capturing

**Steps**:
1. Interactive user makes K8s call: `GET pods in namespace "production"`
2. Assert: audit event emitted with:
   - `event_type: "aiagent.interactive.k8s_call"`
   - `acting_user: "user-a@corp"`
   - `data.resource: "pods"`
   - `data.verb: "get"`
   - `data.namespace: "production"`
   - `data.result: "success"` (or error details)
3. User makes another call: `LIST deployments` (cluster-scoped)
4. Assert: second audit event with `namespace: ""` (cluster-scoped)
5. User call fails (403 from K8s) → assert event with `data.result: "forbidden"`

**Acceptance Criteria**:
- One audit event per K8s API call
- All fields populated (resource, verb, namespace, result)
- Both success and failure captured
- Cluster-scoped calls have empty namespace

---

### UT-KA-AUD-004: Identity transition events in correct sequence

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-04

**Description**: Takeover and resume produce the correct audit event sequence: `session.suspended` → `interactive.started` → ... → `interactive.completed` → `session.resumed`.

**Preconditions**:
- Full takeover/resume cycle
- Audit store capturing ordered events

**Steps**:
1. Autonomous running (events with acting_user = KA SA)
2. User takes over
3. Assert event sequence:
   - `aiagent.session.suspended` (acting_user: KA SA, reason: "takeover")
   - `aiagent.interactive.started` (acting_user: user-a, session_id: new)
4. User works (interactive events)
5. User disconnects
6. Assert event sequence:
   - `aiagent.interactive.completed` (acting_user: user-a, reason: "disconnect")
   - `aiagent.session.resumed` (acting_user: KA SA, session_id: new)

**Acceptance Criteria**:
- Strict ordering: suspended → started → completed → resumed
- No gaps in sequence
- Each event has correct identity attribution
- Session IDs are distinct for each phase

---

### UT-KA-AUD-005: Full conversation reconstructable from DS query

**BR**: BR-INTERACTIVE-003, BR-INTERACTIVE-006
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-05

**Description**: Querying DS by `correlation_id` ordered by `event_timestamp` produces a complete, chronologically correct conversation history.

**Preconditions**:
- Mock DS with seeded audit events from multiple sessions
- Query function available

**Steps**:
1. Seed DS with events from 3 sessions (in time order):
   - sess-01 (autonomous): events at T+1s, T+5s, T+10s
   - sess-02 (user-a): events at T+12s, T+15s, T+18s
   - sess-03 (autonomous): events at T+20s, T+25s
2. Query by `correlation_id` ordered by `event_timestamp`
3. Assert: all 8 events returned in timestamp order
4. Assert: session transitions visible (sess-01 → sess-02 → sess-03)
5. Assert: no gaps (all events present)

**Acceptance Criteria**:
- Complete reconstruction from correlation_id query
- Correct chronological ordering
- Cross-session events interleaved correctly by time
- Session boundaries identifiable via session_id changes

---

### UT-KA-AUD-006: Audit event includes correlation_id (RR UID)

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-06

**Description**: All events use the RemediationRequest UID as `correlation_id`, enabling full investigation history queries.

**Preconditions**:
- Investigation tied to RR with UID "rr-uid-12345"

**Steps**:
1. Start investigation for RR UID "rr-uid-12345"
2. Emit various events (LLM request, tool call, session start)
3. Assert: ALL events have `correlation_id == "rr-uid-12345"`
4. Assert: correlation_id is immutable across session transitions

**Acceptance Criteria**:
- correlation_id = RR UID on all events
- Immutable across autonomous/interactive transitions
- Enables single-query reconstruction of full investigation

---

### UT-KA-AUD-007: Audit events include timestamps with sub-second precision

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Audit
**Check ID**: AUD-07

**Description**: All audit events have `event_timestamp` with sub-second precision (for correct ordering of rapid events).

**Preconditions**:
- Rapid event emission (multiple events within same second)

**Steps**:
1. Emit 5 events in rapid succession (within 10ms)
2. Assert: all have distinct `event_timestamp` values
3. Assert: timestamps have sub-second precision (nanoseconds or microseconds)
4. Assert: ordering by timestamp matches emission order

**Acceptance Criteria**:
- Sub-second precision (not truncated to seconds)
- Distinct timestamps for rapid events
- Timestamp ordering matches emission ordering
- Format: RFC3339Nano or equivalent

---

## PROP-SESS: Session Security Properties

### UT-KA-SESS-009: Session ID entropy (crypto-random, >=128 bits)

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Security Property
**Check ID**: PROP-SESS-01

**Description**: Interactive session IDs have sufficient entropy to prevent guessing.

**Preconditions**:
- Session ID generator function

**Steps**:
1. Generate 1000 session IDs
2. Assert: all unique
3. Assert: each has >=22 characters (128 bits in base64url)
4. Assert: no sequential pattern
5. Assert: source is `crypto/rand` (code review or signature verification)

**Acceptance Criteria**:
- >=128 bits entropy
- `crypto/rand` source
- No predictable patterns
- URL-safe characters (base64url or hex)

---

### UT-KA-SESS-010: Lease name derived from RR (not user-controlled)

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Security Property
**Check ID**: PROP-SESS-02

**Description**: K8s Lease name is derived from the investigation/RR identifier (server-controlled), not from user input. Prevents Lease name injection.

**Preconditions**:
- Lease creation function

**Steps**:
1. Create session for RR "rr-12345"
2. Assert: Lease name is deterministic from RR ID (e.g., `"interactive-rr-12345"`)
3. Assert: user cannot influence Lease name via request parameters
4. Attempt to inject special characters in RR ID reference → assert sanitized

**Acceptance Criteria**:
- Lease name = predictable function of RR ID
- No user-controlled input in Lease name
- Special characters sanitized (K8s name constraints enforced)

---

### UT-KA-SESS-011: Session store bounded (max concurrent sessions)

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Security Property
**Check ID**: PROP-SESS-03

**Description**: Session store has a maximum capacity. Prevents memory exhaustion from excessive session creation.

**Preconditions**:
- Session store with configurable max capacity

**Steps**:
1. Configure session store with max = 10
2. Create 10 sessions successfully
3. Attempt to create 11th session
4. Assert: rejected with appropriate error (resource exhaustion protection)
5. Complete one session
6. Attempt again → assert: succeeds (slot freed)

**Acceptance Criteria**:
- Maximum capacity enforced
- Clear error on capacity exceeded
- Capacity freed on session completion
- Configurable limit (not hardcoded)

---

### UT-KA-SESS-012: Session metadata immutable after creation

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Security Property
**Check ID**: PROP-SESS-04

**Description**: Session metadata (ActingUser, RR reference, StartedAt) cannot be mutated after session creation. Only CompletedAt can be set (once).

**Preconditions**:
- Active session

**Steps**:
1. Create session with ActingUser "user-a", RR "rr-123"
2. Attempt to change ActingUser → assert: rejected or no-op
3. Attempt to change RR reference → assert: rejected or no-op
4. Attempt to change StartedAt → assert: rejected or no-op
5. Set CompletedAt (valid lifecycle transition) → assert: succeeds
6. Attempt to change CompletedAt again → assert: rejected (already set)

**Acceptance Criteria**:
- Creation-time fields are immutable
- Only CompletedAt is settable (once)
- No mutation API exposed for security-critical fields
- Immutability enforced at type level (not just convention)
