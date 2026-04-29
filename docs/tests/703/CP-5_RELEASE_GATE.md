# CP-5: Release Gate — Test Case Specifications

**Checkpoint**: CP-5
**Gate Type**: E2E + Unit (Helm) + Document Review (CRITICAL)
**Total Checks**: 29
**Merge Criteria**: All 29 checks pass, full E2E flow in Kind, Helm templates valid, documentation complete
**PR**: PR6 (Integration + Helm + E2E + Runbook + Monitoring)

---

## Overview

CP-5 is the final release gate. It validates the complete interactive flow end-to-end in a real Kind cluster, ensures Helm chart correctness, verifies backward compatibility with autonomous mode, validates UX requirements, and confirms documentation/runbook completeness.

---

## Test Environment

- **E2E Package**: `test/e2e/kubernautagent/`
- **Helm Package**: `test/unit/helm/` (helm template tests)
- **Framework**: Ginkgo/Gomega BDD with `Label("interactive")`
- **Infrastructure**:
  - Kind cluster (1 control plane + 1 worker)
  - Full Helm deployment (KA, DS, RO, mock-LLM)
  - Real MCP client (go-sdk v1.5.0)
  - Real K8s Lease
  - Real DS (PostgreSQL)
- **Timeout**: 20m (separate CI job from 15m autonomous E2E)
- **Key Imports**:
  ```go
  "github.com/modelcontextprotocol/go-sdk/mcp"
  "github.com/jordigilh/kubernaut/pkg/agentclient"
  dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
  coordinationv1 "k8s.io/api/coordination/v1"
  ```

---

## HARM: Holistic Adversarial Regression & Misuse Scenarios

### E2E-KA-HARM-001: Autonomous investigation unaffected by interactive code

**BR**: BR-INTERACTIVE-008
**Type**: E2E
**Category**: Regression
**Check ID**: HARM-01

**Description**: A standard autonomous remediation (no interactive user) runs identically to v1.4 behavior. Interactive code paths are not triggered.

**Preconditions**:
- Kind cluster with `interactive.enabled: true`
- No MCP client connected
- Standard RR created (no annotation, no spec field)

**Steps**:
1. Create RemediationRequest "rr-regression-01" (standard autonomous)
2. Assert: RR progresses through phases: Pending → Investigating → Analyzing → Completed
3. Assert: AIAnalysis created and reaches Completed
4. Assert: `AIAnalysis.Status.InteractiveSession` is nil (never populated)
5. Assert: audit events have acting_user = KA SA (never user)
6. Assert: no Lease created for this RR (interactive not activated)
7. Assert: total time similar to v1.4 baseline (no interactive overhead)

**Acceptance Criteria**:
- Full autonomous pipeline works unchanged
- InteractiveSession never populated
- No Lease artifacts
- No performance regression (within 10% of v1.4 baseline)

---

### E2E-KA-HARM-002: Feature gate disabled — MCP endpoint absent

**BR**: BR-INTERACTIVE-008
**Type**: E2E
**Category**: Security
**Check ID**: HARM-02

**Description**: With `interactive.enabled: false` (Helm override), the MCP endpoint does not exist. Full autonomous pipeline works.

**Preconditions**:
- Kind cluster deployed with `kubernautAgent.interactive.enabled=false`

**Steps**:
1. Deploy with interactive disabled via Helm
2. Port-forward to KA service
3. Attempt HTTP request to `/api/v1/mcp`
4. Assert: 404 Not Found (endpoint not registered)
5. Create standard RR → assert: autonomous pipeline completes normally
6. Assert: no MCP-related pods, containers, or sidecars

**Acceptance Criteria**:
- 404 on MCP endpoint (not 403)
- Autonomous pipeline unaffected
- No interactive resources deployed

---

### E2E-KA-HARM-003: Invalid token to MCP endpoint

**BR**: BR-INTERACTIVE-002
**Type**: E2E
**Category**: Security
**Check ID**: HARM-03

**Description**: Real HTTP request with invalid Bearer token to MCP endpoint in Kind cluster. Must be rejected by real TokenReview.

**Preconditions**:
- Kind cluster with interactive enabled
- Invalid/expired token

**Steps**:
1. Port-forward to KA MCP endpoint
2. Send MCP initialize with `Authorization: Bearer invalid-token-xyz`
3. Assert: 401 Unauthorized
4. Assert: response is RFC 7807 Problem JSON
5. Assert: no session created in KA
6. Assert: K8s audit log shows TokenReview rejection

**Acceptance Criteria**:
- Real TokenReview rejection (not mocked)
- RFC 7807 response format
- K8s audit trail captures attempt

---

### E2E-KA-HARM-004: Takeover of non-existent remediation

**BR**: BR-INTERACTIVE-004
**Type**: E2E
**Category**: Error Handling
**Check ID**: HARM-04

**Description**: User attempts takeover of an RR that doesn't exist in the cluster. Clear error.

**Preconditions**:
- Kind cluster, user authenticated via SA token
- No RR named "ghost-rr"

**Steps**:
1. Connect to MCP endpoint with valid SA token
2. Call `kubernaut_investigate` with `{"rr_id": "ghost-rr", "action": "takeover"}`
3. Assert: MCP tool error returned
4. Assert: error guides user (RR not found, check name/namespace)
5. Assert: no Lease created, no session created

**Acceptance Criteria**:
- Graceful error (not 500)
- Helpful message
- No orphaned resources

---

### E2E-KA-HARM-005: User K8s RBAC prevents resource access

**BR**: BR-INTERACTIVE-002
**Type**: E2E
**Category**: Security
**Check ID**: HARM-05

**Description**: User takes over but their K8s RBAC doesn't allow access to the resources the investigation needs. Impersonated calls fail with 403 from real API server.

**Preconditions**:
- Kind cluster
- SA token for user with limited RBAC (can only read pods in "default" namespace)
- RR is investigating resources in "production" namespace

**Steps**:
1. Create RR targeting "production" namespace resources
2. Wait for autonomous to start investigating
3. User (limited RBAC) takes over
4. User's impersonated call to "production" namespace → 403 from K8s API
5. Assert: tool returns error "Insufficient permissions for pods in namespace production"
6. Assert: session remains active (failure doesn't kill session)
7. Assert: audit event captures the 403 with details

**Acceptance Criteria**:
- Real K8s RBAC enforcement via impersonation
- Clear error to user (not generic 500)
- Session survives tool-level RBAC failure
- Audit captures the attempted access

---

### E2E-KA-HARM-006: Concurrent users — second driver rejected

**BR**: BR-INTERACTIVE-004
**Type**: E2E
**Category**: Concurrency
**Check ID**: HARM-06

**Description**: Two users attempt takeover of the same RR in a real Kind cluster. K8s Lease ensures only one wins.

**Preconditions**:
- Kind cluster
- Two distinct SA tokens (user-a-sa, user-b-sa)
- RR in autonomous investigation

**Steps**:
1. Create RR, wait for autonomous to start
2. User-A sends `action: takeover` → assert: succeeds (Lease acquired)
3. User-B sends `action: takeover` → assert: rejected with `lease_held`
4. Assert: Lease object exists in cluster with holder = user-A identity
5. Assert: user-B error includes user-A identity
6. User-A disconnects → Lease released
7. User-B retries takeover → assert: succeeds now

**Acceptance Criteria**:
- Real K8s Lease enforces single-driver
- Second user gets informative rejection
- After first user releases, second can acquire
- No split-brain scenario

---

## E2E: Full Interactive Flow

### E2E-KA-INT-001: Complete interactive lifecycle (connect → observe → takeover → drive → disconnect → resume)

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-004
**Type**: E2E
**Category**: Happy Path — Full Journey
**Check ID**: E2E-01

**Description**: The complete interactive user journey in a real Kind cluster. This is the golden-path E2E test.

**Preconditions**:
- Kind cluster with full Helm deployment (KA, DS, RO, mock-LLM)
- `interactive.enabled: true`
- Valid SA token with Driver RBAC

**Steps**:
1. Create RR "rr-e2e-interactive"
2. Wait for autonomous investigation to start (mock LLM produces findings)
3. Connect MCP client with valid SA token
4. Call `kubernaut_investigate` with `{"rr_id": "rr-e2e-interactive"}` (observe mode)
5. Assert: receive current status (autonomous running)
6. Call `kubernaut_investigate` with `{"rr_id": "rr-e2e-interactive", "action": "takeover"}`
7. Assert: takeover succeeds, autonomous cancelled
8. Assert: user receives auto-injected context from prior autonomous work
9. Call `kubernaut_enrich` (user-driven enrichment)
10. Assert: enrichment executes under user identity (impersonated)
11. Call `kubernaut_select_workflow` with valid workflow
12. Assert: workflow selection recorded
13. Disconnect MCP client
14. Assert: Lease released, `interactive.completed` audit event
15. Wait for autonomous reconstruction
16. Assert: new autonomous session picks up where user left off
17. Assert: autonomous completes the remediation
18. Query DS for full audit trail → assert complete sequence

**Acceptance Criteria**:
- All 17 steps complete without error
- Full audit trail in DS (query by correlation_id)
- Identity transitions visible (KA SA → user → KA SA)
- Total E2E time < 5 minutes
- Remediation reaches terminal state

---

### E2E-KA-INT-002: Graceful degradation — DS unavailable during auto-inject

**BR**: BR-INTERACTIVE-008
**Type**: E2E
**Category**: Fault Tolerance
**Check ID**: E2E-02

**Description**: DS is temporarily down when user takes over. Takeover succeeds (best-effort auto-inject).

**Preconditions**:
- Kind cluster
- Ability to temporarily kill DS pod

**Steps**:
1. Create RR, wait for autonomous to start
2. Kill DS pod (simulate outage)
3. User sends `action: takeover`
4. Assert: takeover succeeds (session created)
5. Assert: auto-inject returns empty context (DS unavailable)
6. Assert: warning logged and audit event notes degraded state
7. Restore DS pod
8. User can still send tool calls normally
9. After DS restores: new tool calls produce audit events normally

**Acceptance Criteria**:
- Takeover NOT blocked by DS failure
- Session functional without prior context
- DS recovery restores audit emission
- User notified of degraded state

---

### E2E-KA-INT-003: Observer mode (no takeover) — passive monitoring

**BR**: BR-INTERACTIVE-001
**Type**: E2E
**Category**: Happy Path
**Check ID**: E2E-03

**Description**: User connects and observes without taking over. Receives real-time NotificationBus events.

**Preconditions**:
- Kind cluster with active investigation
- User connected as Observer (no `action: takeover`)

**Steps**:
1. Create RR, wait for autonomous to start
2. Connect MCP client as Observer
3. Subscribe to notifications for this RR
4. Assert: receive audit events as autonomous progresses (LLM turns)
5. Assert: autonomous NOT interrupted
6. After 3 events received: disconnect
7. Assert: autonomous continues normally

**Acceptance Criteria**:
- Observer receives real-time events
- Autonomous not disrupted
- No Lease acquired for Observer
- Clean disconnect

---

### E2E-KA-INT-004: Impersonated K8s call in real cluster

**BR**: BR-INTERACTIVE-002
**Type**: E2E
**Category**: Security
**Check ID**: E2E-04

**Description**: During interactive mode, a K8s API call executes as the impersonated user (verified by K8s audit or RBAC behavior).

**Preconditions**:
- Kind cluster
- SA with RBAC: can read pods in "default", cannot read secrets

**Steps**:
1. User takes over investigation
2. Investigation tool makes K8s call: `GET pods/default` → succeeds
3. Investigation tool makes K8s call: `GET secrets/default` → 403
4. Assert: the 403 is because of USER's RBAC (not KA SA's RBAC)
5. Assert: KA SA CAN read secrets (proving impersonation is enforced)

**Acceptance Criteria**:
- Real RBAC enforcement via impersonation
- User's restrictions honored (not KA SA's permissions)
- Differential access proves impersonation is active

---

### E2E-KA-INT-005: Audit trail complete in DS after full flow

**BR**: BR-INTERACTIVE-003
**Type**: E2E
**Category**: Audit
**Check ID**: E2E-05

**Description**: After a complete interactive lifecycle, DS contains a full, ordered audit trail with correct session_id and acting_user attribution.

**Preconditions**:
- Completed interactive flow (E2E-KA-INT-001 or equivalent)
- DS query access

**Steps**:
1. After full flow completes (autonomous → takeover → interactive → disconnect → resume → complete)
2. Query DS: `correlation_id = RR-UID, order by event_timestamp`
3. Assert: events present from all 3 sessions (autonomous, interactive, reconstructed-autonomous)
4. Assert: identity transitions visible:
   - session.suspended (KA SA)
   - interactive.started (user)
   - interactive.completed (user)
   - session.resumed (KA SA)
5. Assert: all events have valid session_id (non-empty)
6. Assert: chronological ordering correct
7. Assert: no duplicate events

**Acceptance Criteria**:
- Complete audit trail reconstructable from single DS query
- Identity attribution correct throughout
- No missing events (all lifecycle transitions captured)
- Events support forensic reconstruction

---

### E2E-KA-INT-006: MCP reconnection after pod restart

**BR**: BR-INTERACTIVE-005, BR-INTERACTIVE-008
**Type**: E2E
**Category**: Fault Tolerance
**Check ID**: E2E-06

**Description**: KA pod restarts mid-session. Lease expires, user reconnects, session reconstructed from DS.

**Preconditions**:
- Kind cluster with active interactive session
- Ability to delete KA pod

**Steps**:
1. User takes over (active session)
2. Delete KA pod (simulates crash)
3. Wait for pod restart (K8s recreates)
4. Assert: old Lease expires within 30s
5. User reconnects with same token
6. User sends `action: takeover` again
7. Assert: new session created (cancel+reconstruct)
8. Assert: auto-inject retrieves prior audit events from DS
9. Assert: user can continue working

**Acceptance Criteria**:
- Recovery within Lease duration (30s) + pod start time
- User can re-takeover after restart
- Prior context available via DS
- No orphaned Lease blocking recovery

---

### E2E-KA-INT-007: Multi-step interactive investigation with enrichment

**BR**: BR-INTERACTIVE-001
**Type**: E2E
**Category**: Happy Path
**Check ID**: E2E-07

**Description**: User takes over, asks questions, enriches with additional data, selects workflow — full multi-step interaction.

**Preconditions**:
- Kind cluster with mock-LLM supporting interactive responses
- Workflow catalog with test workflow

**Steps**:
1. User takes over investigation
2. Send investigative query → receive LLM response
3. Call `kubernaut_enrich` to gather additional logs
4. Assert: enrichment result added to context
5. Send follow-up query referencing enrichment
6. Call `kubernaut_select_workflow` with chosen workflow
7. Assert: workflow recorded in AA status
8. Disconnect → autonomous resumes with all context

**Acceptance Criteria**:
- Multi-step flow works naturally
- Context accumulates across tool calls
- Workflow selection persists
- Autonomous reconstruction includes all user work

---

## HELM: Helm Chart Validation

### UT-HELM-001: interactive.enabled controls RBAC resources

**BR**: BR-INTERACTIVE-001
**Type**: Unit (helm template)
**Category**: Configuration
**Check ID**: HELM-01

**Description**: When `interactive.enabled: true`, Helm renders ClusterRole with `impersonate` verb. When false, no interactive RBAC.

**Preconditions**:
- Helm chart available
- `helm template` command

**Steps**:
1. Run `helm template` with `kubernautAgent.interactive.enabled=true`
2. Assert: ClusterRole includes `impersonate` verb on `users`, `groups`
3. Assert: ClusterRoleBinding binds KA SA
4. Run `helm template` with `kubernautAgent.interactive.enabled=false`
5. Assert: No impersonate ClusterRole rendered
6. Assert: No interactive-specific RBAC resources

**Acceptance Criteria**:
- RBAC conditional on feature gate
- impersonate verb only present when enabled
- Clean removal when disabled

---

### UT-HELM-002: interactive config in ConfigMap

**BR**: BR-INTERACTIVE-001
**Type**: Unit (helm template)
**Category**: Configuration
**Check ID**: HELM-02

**Description**: Interactive configuration (timeout, inactivity, rate limits) rendered in KA ConfigMap when enabled.

**Preconditions**:
- Helm chart with interactive values schema

**Steps**:
1. Run `helm template` with interactive config:
   ```yaml
   kubernautAgent:
     interactive:
       enabled: true
       inactivityTimeout: 15m
       globalTimeout: 1h
       rateLimitPerUser: 20
   ```
2. Assert: ConfigMap contains interactive section with values
3. Assert: defaults applied when values omitted (10m, 1h, 10)

**Acceptance Criteria**:
- Config values rendered correctly
- Defaults applied for omitted values
- No interactive config when disabled

---

### UT-HELM-003: Lease RBAC scoped correctly

**BR**: BR-INTERACTIVE-005
**Type**: Unit (helm template)
**Category**: Security
**Check ID**: HELM-03

**Description**: Lease RBAC allows KA SA to create/get/update Leases only in its own namespace (not cluster-wide).

**Preconditions**:
- Helm chart renders Lease Role (not ClusterRole)

**Steps**:
1. Run `helm template` with interactive enabled
2. Find Role (not ClusterRole) for Lease management
3. Assert: resources = `["leases"]`, verbs = `["get", "create", "update", "delete"]`
4. Assert: namespace-scoped (Role, not ClusterRole)
5. Assert: RoleBinding binds to KA SA in same namespace

**Acceptance Criteria**:
- Lease access is namespace-scoped (not cluster-wide)
- Minimum verbs needed (get, create, update, delete)
- Bound to KA SA only

---

### UT-HELM-004: Values schema validates interactive config

**BR**: BR-INTERACTIVE-001
**Type**: Unit (helm template)
**Category**: Configuration
**Check ID**: HELM-04

**Description**: Invalid interactive config values are caught by Helm schema validation.

**Preconditions**:
- Helm values.schema.json with interactive section

**Steps**:
1. Attempt `helm template` with `interactive.inactivityTimeout: "not-a-duration"` → assert: schema error
2. Attempt with `interactive.globalTimeout: -1` → assert: schema error
3. Attempt with `interactive.rateLimitPerUser: 0` → assert: schema error or warning
4. Attempt with valid values → assert: success

**Acceptance Criteria**:
- Invalid durations rejected
- Negative values rejected
- Zero rate limit rejected (or documented minimum)
- Schema covers all interactive config fields

---

### UT-HELM-005: Upgrade path from v1.4 chart (no interactive) to v1.5

**BR**: BR-INTERACTIVE-008
**Type**: Unit (helm template)
**Category**: Backward Compatibility
**Check ID**: HELM-05

**Description**: Upgrading from v1.4 chart (no interactive section in values) to v1.5 works with defaults. No breaking changes.

**Preconditions**:
- v1.4 values file (no `interactive` key)

**Steps**:
1. Run `helm template` with v1.4 values (no interactive section)
2. Assert: template succeeds (interactive defaults to disabled)
3. Assert: no interactive resources rendered
4. Assert: all v1.4 resources unchanged
5. Assert: `helm diff` shows only additive changes (never removes existing resources)

**Acceptance Criteria**:
- v1.4 values file works in v1.5 chart
- Interactive defaults to disabled
- No breaking changes for existing deployments
- Upgrade is purely additive

---

## DOC: Documentation Review

### DOC-01: User guide exists

**BR**: BR-INTERACTIVE-001
**Type**: Document Review
**Category**: Documentation
**Check ID**: DOC-01

**Description**: `docs/user-guide/interactive-mode.md` exists and covers: how to enable, how to connect, available tools, error handling, disconnect behavior.

**Preconditions**: None

**Review Steps**:
1. File exists at documented path
2. Sections present: Enable, Connect, Tools, Errors, Disconnect/Reconnect
3. MCP client examples (at least one client library shown)
4. Troubleshooting section covers common errors

**Acceptance Criteria**:
- [ ] File exists
- [ ] All 5 sections present
- [ ] At least one working MCP client example
- [ ] Troubleshooting covers: auth failures, Lease contention, timeout

---

### DOC-02: Support runbook exists

**BR**: BR-INTERACTIVE-005
**Type**: Document Review
**Category**: Documentation
**Check ID**: DOC-02

**Description**: Operational runbook for support teams covers: Lease stuck, session orphaned, audit gap, timeout tuning.

**Preconditions**: None

**Review Steps**:
1. Runbook exists (e.g., `docs/operations/interactive-mode-runbook.md`)
2. Covers: Lease stuck recovery, session cleanup, audit gap diagnosis
3. Includes `kubectl` commands for each scenario
4. References relevant Prometheus alerts

**Acceptance Criteria**:
- [ ] Runbook exists
- [ ] >=4 operational scenarios covered
- [ ] Each scenario has step-by-step resolution
- [ ] References monitoring alerts

---

### DOC-03: Architecture diagram updated

**BR**: BR-INTERACTIVE-001
**Type**: Document Review
**Category**: Documentation
**Check ID**: DOC-03

**Description**: System architecture diagrams include the MCP endpoint, apifrontend relationship, and interactive data flow.

**Preconditions**: None

**Review Steps**:
1. Architecture diagram shows MCP endpoint on KA
2. Shows apifrontend as external gateway
3. Shows Lease for session management
4. Shows audit flow to DS

**Acceptance Criteria**:
- [ ] MCP endpoint visible in architecture
- [ ] apifrontend relationship clear
- [ ] Interactive data flow distinguishable from autonomous

---

## COMPAT: Backward Compatibility

### E2E-KA-COMPAT-001: Existing autonomous E2E suite passes unchanged

**BR**: BR-INTERACTIVE-008
**Type**: E2E
**Category**: Regression
**Check ID**: COMPAT-01

**Description**: The existing autonomous E2E test suite (`make test-e2e-kubernautagent`) passes with zero modifications after all #703 PRs merged.

**Preconditions**:
- All #703 PRs merged
- Kind cluster deployed with interactive enabled

**Steps**:
1. Run `make test-e2e-kubernautagent` (existing autonomous suite)
2. Assert: 100% pass rate
3. Assert: no test file modified by #703 (git diff against pre-703 baseline)
4. Assert: execution time within 10% of baseline

**Acceptance Criteria**:
- Zero failures in existing suite
- Zero test modifications required
- No performance regression

---

### E2E-KA-COMPAT-002: v1.4 RR (without interactive fields) processed correctly

**BR**: BR-INTERACTIVE-007, BR-INTERACTIVE-008
**Type**: E2E
**Category**: Backward Compatibility
**Check ID**: COMPAT-02

**Description**: A RR created by v1.4 code (no interactive annotation/spec) is processed by v1.5 controllers without error.

**Preconditions**:
- Kind cluster with v1.5 code
- RR YAML matching v1.4 schema (no interactive fields)

**Steps**:
1. Apply v1.4-format RR YAML (no interactive fields)
2. Assert: RR accepted without validation error
3. Assert: processing proceeds through all phases
4. Assert: AIAnalysis.Status.InteractiveSession remains nil
5. Assert: no warning logs about missing interactive fields

**Acceptance Criteria**:
- v1.4 RR format works in v1.5
- No validation errors
- No interactive artifacts for v1.4-style RRs

---

### E2E-KA-COMPAT-003: CRD upgrade preserves existing AIAnalysis data

**BR**: BR-INTERACTIVE-007
**Type**: E2E
**Category**: Backward Compatibility
**Check ID**: COMPAT-03

**Description**: After CRD upgrade (adding InteractiveSessionInfo), existing AIAnalysis resources are readable and functional.

**Preconditions**:
- Kind cluster with existing AIAnalysis resources (created before CRD update)

**Steps**:
1. Create AIAnalysis with v1.4 schema (no interactiveSession)
2. Apply v1.5 CRD (adds interactiveSession to status)
3. Read existing AIAnalysis → assert: readable without error
4. Assert: `interactiveSession` is absent/nil (not empty)
5. Update status of existing AIAnalysis (normal processing)
6. Assert: update succeeds, existing fields preserved

**Acceptance Criteria**:
- CRD upgrade is non-breaking
- Existing resources readable
- Status updates work on pre-upgrade resources
- No data migration required

---

## UX: UX and Operational Readiness

### E2E-KA-UX-001: Time-remaining visible during interactive session

**BR**: BR-INTERACTIVE-005
**Type**: E2E
**Category**: UX
**Check ID**: UX-01

**Description**: During active interactive session, user can query remaining time. Warnings delivered before timeout.

**Preconditions**:
- Active interactive session in Kind cluster
- MCP notification channel

**Steps**:
1. User takes over
2. Query session status → assert: remaining time visible in response
3. Wait for T-10m equivalent (accelerated) → assert: warning notification received
4. Wait for T-2m equivalent → assert: critical notification received
5. Assert: notifications include recommended actions ("save your work")

**Acceptance Criteria**:
- Remaining time queryable
- Warnings at correct times
- Warnings include actionable guidance

---

### E2E-KA-UX-002: Inactivity warning before cutoff

**BR**: BR-INTERACTIVE-005
**Type**: E2E
**Category**: UX
**Check ID**: UX-02

**Description**: User idle for extended period receives inactivity warning before session is terminated.

**Preconditions**:
- Active session with short inactivity timeout (accelerated)

**Steps**:
1. User takes over
2. Do not send any tool calls
3. Assert: inactivity warning received at T-2m before cutoff
4. Assert: warning says "No activity detected. Session will end in 2 minutes."
5. Send a tool call after warning → assert: timer resets, session continues
6. Go idle again → assert: new warning cycle starts

**Acceptance Criteria**:
- Warning before cutoff (not after)
- Tool call resets timer
- Warning message is clear and actionable
- Timer reset is immediate

---

### E2E-KA-UX-003: Second driver rejection includes holder info

**BR**: BR-INTERACTIVE-004
**Type**: E2E
**Category**: UX
**Check ID**: UX-03

**Description**: When a second user is rejected (Lease held), the error message includes who holds the session and since when. Enables human coordination.

**Preconditions**:
- Two users, first holds session

**Steps**:
1. User-A takes over (Lease held)
2. User-B attempts takeover
3. Assert: error includes: `"Session controlled by user-a since 2026-04-29T17:00:00Z"`
4. Assert: message suggests: "Wait for release or observe"

**Acceptance Criteria**:
- Holder identity in error (not just "session busy")
- Timestamp of when holder started
- Suggested next action for rejected user

---

### E2E-KA-UX-004: Prometheus metrics emitted for interactive sessions

**BR**: BR-INTERACTIVE-001
**Type**: E2E
**Category**: Operational
**Check ID**: UX-04

**Description**: Prometheus metrics for interactive mode are emitted and scrapeable during an interactive session.

**Preconditions**:
- Kind cluster with Prometheus scraping KA

**Steps**:
1. User takes over → assert: `kubernaut_interactive_sessions_active` = 1
2. Execute tool call → assert: `kubernaut_interactive_command_duration_seconds` histogram updated
3. Assert: `kubernaut_interactive_takeover_total` incremented
4. User disconnects → assert: `kubernaut_interactive_sessions_active` = 0
5. Attempt concurrent takeover → assert: `kubernaut_interactive_lease_contention_total` incremented

**Acceptance Criteria**:
- All 4 documented metrics emitted
- Correct values (gauge goes up/down, counters increment)
- Scrapeable from /metrics endpoint
- Labels correct (rr_id, user, etc.)

---

### E2E-KA-UX-005: Audit drop-rate alerting fires on backpressure

**BR**: BR-INTERACTIVE-003
**Type**: E2E
**Category**: Operational
**Check ID**: UX-05

**Description**: When audit event emission is under backpressure (DS slow), the drop-rate metric increases and would trigger alerts.

**Preconditions**:
- Kind cluster
- DS artificially slowed (e.g., network delay)
- NotificationBus with bounded buffer

**Steps**:
1. Slow down DS responses (add 2s latency to audit ingestion)
2. Generate rapid audit events (multiple tool calls in quick succession)
3. Assert: `kubernaut_interactive_notifications_dropped_total` increases (if NotificationBus drops)
4. Assert: audit events NOT lost (buffered and eventually delivered to DS)
5. Assert: if drops occur, they are for NotificationBus observers only (not DS persistence)

**Acceptance Criteria**:
- Drop metric captures NotificationBus drops
- DS audit persistence is NOT affected by backpressure (buffered)
- Observer (NotificationBus) drops are acceptable and metered
- Alert threshold documentable
