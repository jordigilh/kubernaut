# CP-4: Tool Completeness Gate — Test Case Specifications

**Checkpoint**: CP-4
**Gate Type**: Unit Tests
**Total Checks**: 12
**Merge Criteria**: All 12 tests pass, all 3 MCP tools functional
**PR**: PR4 (kubernaut_enrich) + PR5 (kubernaut_select_workflow)

---

## Overview

CP-4 validates that all three MCP tools (`kubernaut_investigate`, `kubernaut_enrich`, `kubernaut_select_workflow`) handle adversarial inputs correctly and maintain cross-tool consistency (shared session, audit, identity).

---

## Test Environment

- **Package**: `test/unit/kubernautagent/mcp/tools/`
- **Framework**: Ginkgo/Gomega BDD
- **Key Imports**:
  ```go
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/session"
  "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
  ```
- **Mocks**:
  - `MockEnrichmentPipeline`: Returns configurable enrichment results
  - `MockWorkflowCatalog`: Returns configurable workflow list
  - `MockDSClient`: Returns audit events for context
  - `audit.InMemoryAuditStore`: Captures events
- **Helpers**:
  - `newToolContext(user, sessionID, rrID)`: Creates authenticated tool execution context
  - `assertToolResult(result, expectedStatus, expectedFields)`: Validates MCP tool response

---

## TOOL: Adversarial Tool Scenarios

### UT-KA-TOOL-001: kubernaut_investigate with invalid rr_id

**BR**: BR-INTERACTIVE-001
**Type**: Unit
**Category**: Error Handling
**Check ID**: TOOL-01

**Description**: `kubernaut_investigate` called with non-existent `rr_id`. Must return clear MCP error (not crash).

**Preconditions**:
- MCP tool handler for `kubernaut_investigate` instantiated
- No RR with ID "nonexistent-rr" exists

**Steps**:
1. Create authenticated tool context for user-a
2. Call `kubernaut_investigate` with `{"rr_id": "nonexistent-rr", "action": "takeover"}`
3. Assert: MCP tool error returned (not HTTP error)
4. Assert: error code is `session_not_found` or similar
5. Assert: human message: "Remediation request not found"
6. Assert: no session created, no Lease acquired
7. Assert: no panic or nil-pointer dereference

**Acceptance Criteria**:
- Clear MCP JSON-RPC error (not 500)
- No side effects (no session, no Lease)
- Error guides user to correct action
- Non-existent RR is a user error, not server error

---

### UT-KA-TOOL-002: kubernaut_investigate without action field (observe mode)

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Happy Path
**Check ID**: TOOL-02

**Description**: `kubernaut_investigate` called with `rr_id` but no `action` field. User enters Observer mode (sees status, receives NotificationBus events, does NOT disrupt autonomous).

**Preconditions**:
- Autonomous investigation running on "rr-123"
- User authenticated as Observer (no Lease needed)

**Steps**:
1. Call `kubernaut_investigate` with `{"rr_id": "rr-123"}` (no action field)
2. Assert: response includes current investigation status (phase, findings so far)
3. Assert: user subscribed to NotificationBus for "rr-123"
4. Assert: autonomous mode NOT interrupted
5. Assert: no Lease acquired
6. Assert: response clearly indicates "observing" mode

**Acceptance Criteria**:
- Observer mode is default (no action = observe)
- Status response includes useful information
- Autonomous continues uninterrupted
- No Lease acquired for Observer

---

### UT-KA-TOOL-003: kubernaut_enrich with empty enrichment request

**BR**: BR-INTERACTIVE-001
**Type**: Unit
**Category**: Error Handling
**Check ID**: TOOL-03

**Description**: `kubernaut_enrich` called with empty or minimal arguments. Must validate required fields and return helpful error.

**Preconditions**:
- Active interactive session (user is Driver)
- `kubernaut_enrich` tool handler

**Steps**:
1. Call `kubernaut_enrich` with `{}` (empty arguments)
2. Assert: validation error returned listing required fields
3. Call with `{"rr_id": "rr-123"}` (missing enrichment type)
4. Assert: validation error: "enrichment_type is required"
5. Call with `{"rr_id": "rr-123", "enrichment_type": "invalid_type"}`
6. Assert: validation error: "unknown enrichment_type"

**Acceptance Criteria**:
- Input validation before execution
- Helpful error messages listing what's missing/wrong
- No partial execution on invalid input
- Valid enrichment types documented in error response

---

### UT-KA-TOOL-004: kubernaut_enrich on investigation user doesn't own

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security
**Check ID**: TOOL-04

**Description**: User-A is driving "rr-123". User-B (not the driver) attempts to call `kubernaut_enrich` on the same RR. Must be rejected.

**Preconditions**:
- User-A is active Driver on "rr-123"
- User-B authenticated but not the Driver

**Steps**:
1. User-A has active session on "rr-123"
2. User-B calls `kubernaut_enrich` with `{"rr_id": "rr-123", "enrichment_type": "logs"}`
3. Assert: rejected with `lease_held` error
4. Assert: user-A's session not affected
5. Assert: no enrichment executed

**Acceptance Criteria**:
- Non-driver cannot execute tools on active session
- Error includes Driver identity for coordination
- No side effects from rejected call

---

### UT-KA-TOOL-005: kubernaut_select_workflow with non-existent workflow

**BR**: BR-INTERACTIVE-001
**Type**: Unit
**Category**: Error Handling
**Check ID**: TOOL-05

**Description**: User selects a workflow that doesn't exist in the catalog. Clear error returned.

**Preconditions**:
- Active interactive session
- Mock workflow catalog with known workflows: ["restart-pod", "scale-deployment"]

**Steps**:
1. Call `kubernaut_select_workflow` with `{"rr_id": "rr-123", "workflow_id": "delete-cluster"}`
2. Assert: error "workflow not found: delete-cluster"
3. Assert: error includes available workflows (helps user pick correct one)
4. Assert: no workflow execution triggered
5. Assert: audit event captures the failed selection attempt

**Acceptance Criteria**:
- Clear error for non-existent workflow
- Response includes available alternatives
- No partial execution
- Audit captures attempt (including what was requested)

---

### UT-KA-TOOL-006: kubernaut_select_workflow without active session

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Error Handling
**Check ID**: TOOL-06

**Description**: User calls `kubernaut_select_workflow` without having taken over first (no active session). Must guide user to takeover.

**Preconditions**:
- User authenticated but NOT the Driver (no active session)
- RR exists but user is Observer

**Steps**:
1. Call `kubernaut_select_workflow` with `{"rr_id": "rr-123", "workflow_id": "restart-pod"}`
2. Assert: error code indicating no active session
3. Assert: message guides user: "You must take over the investigation first (action: takeover)"
4. Assert: no workflow selection occurs

**Acceptance Criteria**:
- Clear error for missing session
- Actionable guidance in error message
- No workflow triggered without Driver session

---

### UT-KA-TOOL-007: kubernaut_investigate with SQL injection in rr_id

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Input Validation
**Check ID**: TOOL-07

**Description**: User passes malicious input in `rr_id` field (SQL injection, path traversal). Must be sanitized or rejected.

**Preconditions**:
- Tool handler with input validation

**Steps**:
1. Call with `rr_id: "'; DROP TABLE audit_events; --"`
2. Assert: rejected with validation error (invalid characters)
3. Call with `rr_id: "../../etc/passwd"`
4. Assert: rejected
5. Call with `rr_id: "<script>alert(1)</script>"`
6. Assert: rejected
7. Call with valid K8s name format `rr_id: "my-remediation-123"`
8. Assert: accepted (K8s DNS-1123 subdomain validation)

**Acceptance Criteria**:
- Input validated against K8s naming rules (DNS-1123)
- Special characters rejected before any processing
- No injection possible in downstream queries
- Valid K8s names pass validation

---

## CONSIST: Cross-Tool Consistency

### UT-KA-TOOL-008: All tools use same session context

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Consistency
**Check ID**: CONSIST-01

**Description**: All three tools (`investigate`, `enrich`, `select_workflow`) operate within the same session and share session state (identity, RR reference, audit session_id).

**Preconditions**:
- Active interactive session with specific session_id and acting_user

**Steps**:
1. Take over → session created (session_id: "sess-01", user: "user-a")
2. Call `kubernaut_investigate` → assert session context has sess-01, user-a
3. Call `kubernaut_enrich` → assert same session context (sess-01, user-a)
4. Call `kubernaut_select_workflow` → assert same session context
5. Assert: all audit events from all tools have same session_id

**Acceptance Criteria**:
- Session context shared across all tools
- Single session_id for entire interactive period
- Single acting_user across all tool calls
- No session re-creation between tool calls

---

### UT-KA-TOOL-009: All tools emit audit events with consistent schema

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Consistency
**Check ID**: CONSIST-02

**Description**: Audit events from all tools follow the same schema: `session_id`, `acting_user`, `correlation_id`, `event_timestamp`, `event_type`.

**Preconditions**:
- Audit store capturing events from all tools
- Active session executing multiple tools

**Steps**:
1. Execute one call per tool (investigate, enrich, select_workflow)
2. Collect all audit events
3. For each event, assert mandatory fields present:
   - `session_id`: non-empty, matches session
   - `acting_user`: matches authenticated user
   - `correlation_id`: matches RR UID
   - `event_timestamp`: valid RFC3339Nano
   - `event_type`: follows `aiagent.*` naming pattern
4. Assert: no event is missing any mandatory field

**Acceptance Criteria**:
- Uniform schema across all tools
- No tool emits events with missing mandatory fields
- Naming convention consistent (`aiagent.interactive.*`, `aiagent.tool.*`)

---

### UT-KA-TOOL-010: All tools respect session timeout (rejected after timeout)

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Consistency
**Check ID**: CONSIST-03

**Description**: After session timeout, ALL tools reject calls with consistent error (not just investigate).

**Preconditions**:
- Session that has timed out (completed due to timeout)

**Steps**:
1. Create session, wait for timeout (accelerated)
2. Call `kubernaut_investigate` → assert: rejected with `session_timeout`
3. Call `kubernaut_enrich` → assert: rejected with `session_timeout`
4. Call `kubernaut_select_workflow` → assert: rejected with `session_timeout`
5. Assert: all three return same error code and similar message

**Acceptance Criteria**:
- All tools reject consistently after timeout
- Same error code (`session_timeout`) from all tools
- No tool allows execution after session death

---

### UT-KA-TOOL-011: All tools use impersonated identity for K8s calls

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Consistency
**Check ID**: CONSIST-04

**Description**: Any K8s API call made by any tool during interactive mode uses the user's impersonated identity, not KA SA.

**Preconditions**:
- Active session with user "user-a@corp"
- Mock K8s client that records impersonation config

**Steps**:
1. `kubernaut_investigate` triggers K8s call (e.g., get pod) → assert impersonation = user-a
2. `kubernaut_enrich` triggers K8s call (e.g., get events) → assert impersonation = user-a
3. `kubernaut_select_workflow` triggers K8s call (e.g., get WFE) → assert impersonation = user-a
4. Assert: no tool uses KA SA for K8s calls during interactive session

**Acceptance Criteria**:
- All K8s calls impersonated as user
- No tool bypasses impersonation
- Recorded ImpersonationConfig matches session user
- Non-K8s calls (Prometheus, DS) still use KA SA (documented exception)

---

### UT-KA-TOOL-012: Tool error does not kill session (partial failure resilient)

**BR**: BR-INTERACTIVE-005
**Type**: Unit
**Category**: Resilience
**Check ID**: CONSIST-05

**Description**: If one tool call fails (e.g., K8s API returns 403), the session remains active. User can retry or try a different tool.

**Preconditions**:
- Active interactive session
- Mock K8s client that returns 403 for specific call

**Steps**:
1. Active session as user-a
2. Call `kubernaut_enrich` with a resource user doesn't have access to → K8s returns 403
3. Assert: tool returns error to user ("Insufficient permissions for {resource}")
4. Assert: session is STILL active (not killed)
5. Call `kubernaut_investigate` with a query → assert: succeeds
6. Assert: audit captures both the failed and successful calls
7. Assert: session can be used for many more calls after failure

**Acceptance Criteria**:
- Tool-level failure does not kill session
- User gets clear error and can retry
- Session remains usable after tool error
- Audit captures both successes and failures
