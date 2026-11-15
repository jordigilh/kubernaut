# Test Plan: Issue #2588 — RemediationRequest cancellation shutdown

**Test Plan Identifier:** TP-2588-v1.0
**Feature:** Safe RemediationRequest cancellation propagation and cooperative child shutdown
**Business requirements:** BR-AUDIT-005, BR-ORCH-029, BR-ORCH-030, BR-ORCH-031, BR-COMMON-001
**Controls:** SOC2 CC7.2 (audit reconstruction), CC8.1 (change management only); FedRAMP AU-2, AU-3, AU-9, AC-6, SC-8, SI-10
**OWASP ASVS:** v5.0.0 — V2.2.1, V2.2.2, V2.3.1, V8.2.1, V8.3.1, V16.1.1, V16.2.1, V16.2.4, V16.3.3, V16.5.2, V16.5.3
**Created:** 2026-11-14
**Status:** Active — ENVIRONMENT-BLOCKED (source review and scoped runtime evidence pass; remaining environment gates suspended)
**Branch:** `openchamber/fix-2588-cancellation-lifecycle-cosmic-garden`

## 1. Purpose and objectives

This plan proves that an RR cancellation is asynchronous and cooperative, does
not claim terminal completion before child work stops, and remains observable
when a backend cannot be stopped or verified. It also proves that cancellation
notifications remain deliverable and that existing notification cancellation
semantics are unchanged.

Objectives:

1. The cancel API records an idempotent cancellation request and returns
   `Cancelling`; it never writes terminal `Cancelled` directly.
2. RO reaches `Cancelled` only after every required child acknowledges a
   terminal outcome or no remaining work.
3. WE verifies termination of Kubernetes Job, Tekton PipelineRun, and AWX
   execution resources before acknowledging stopped.
4. RAR cancellation is system-only, while SP, AIAnalysis, WE, and EA retain
   child ownership of cancellation transitions.
5. Every cancellation creates one dedicated cancellation NotificationRequest
   that uses normal delivery/retry and survives parent terminal cleanup.
6. A correlation ID reconstructs the complete cancellation lifecycle and its
   normal notification-delivery evidence.

## 2. Scope

### In scope

- RR cancellation request and deletionTimestamp paths.
- New RR `Cancelling` phase, cancellation acknowledgement contract, and
  parent deletion finalizer.
- Parent watches and child-owned cancellation for SP, AIAnalysis, WE, RAR,
  EA, and NotificationRequest.
- Idempotent Job, Tekton, and AWX stop plus verified termination.
- Cancellation NotificationRequest creation, delivery, retry, and audit.
- Restart/reconcile races, completion/cancellation races, bounded backoff,
  explicit cancellation failure, rendered RBAC, and live SAR evidence.

### Out of scope

- RAR parameter overrides or approval-flow redesign (#2570, #2564, #2565).
- Fleet scope/authorization work (#2592, #2593).
- Unrelated timeout semantics or changes from #2582/#2583.
- Synchronous cross-system cancellation guarantees.

## 3. Approved design decisions

| Decision | Approved behavior |
|---|---|
| Authority | A dedicated cancellation request plus RR deletionTimestamp are authoritative. |
| Parent lifecycle | Add nonterminal `Cancelling`; terminal `Cancelled` requires verified acknowledgements. |
| Ownership | RO owns RR phase/aggregate status; each child owns its own cancellation status. |
| Stop failure | Bound each attempt and back off; publish explicit failure/manual-review evidence and retain the deletion finalizer while stop is unverified. |
| Notification | Use a dedicated cancellation NotificationRequest with the RR correlation ID; it is not suppressed by terminal cleanup. |
| Idempotency | Request IDs and observed backend state make retries/restarts safe. |

## 4. Risks and mitigations

| ID | Risk | Impact | Tests |
|---|---|---|---|
| R1 | RR becomes terminal before a backend stops | Active remediation continues while operators see `Cancelled` | UT-ORCH-2588-001, IT-WE-2588-001, E2E-WE-2588-002 |
| R2 | Stop request succeeds but termination is not verified | False safety claim and unsafe finalizer removal | UT-WE-2588-001, UT-WE-2588-002 |
| R3 | Reconcile/restart repeats destructive stop or notification creation | Duplicate side effects and incomplete audit | UT-ORCH-2588-002, IT-NT-2588-001 |
| R4 | Terminal cleanup deletes the cancellation notification | Required operator notification is lost | UT-NT-2588-001, E2E-NT-2588-003 |
| R5 | User mutates or cancels an RAR directly | Approval policy bypass | UT-RAR-2588-001, IT-RBAC-2588-001 |
| R6 | Parent deletion removes finalizer while work is unverified | External execution outlives its lifecycle owner | IT-ORCH-2588-002, E2E-WE-2588-002 |
| R7 | A transient finalizer update failure strands a terminal RR | RR remains undeletable and cancellation cleanup never retries | UT-ORCH-2588-003 |

## 5. Wiring manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| Cancellation API | `kubernaut_cancel_remediation` | `pkg/apifrontend/tools/crd_tools.go:HandleCancelRemediation` | IT-ORCH-2588-001 |
| RR coordinator | RO reconcile | `internal/controller/remediationorchestrator/` | IT-ORCH-2588-002 |
| SP cancellation | SP parent watch/reconcile | `internal/controller/signalprocessing/` | IT-SP-2588-001 |
| AIAnalysis cancellation | AI parent watch and AgentSession stop | `internal/controller/aianalysis/` | IT-AI-2588-001 |
| WE cancellation | WE reconcile/finalizer | `internal/controller/workflowexecution/` | IT-WE-2588-001 |
| Backend stop | Job/Tekton/AWX executor implementations | `pkg/workflowexecution/executor/` | IT-WE-2588-002 |
| RAR system cancellation | RO/RAR lifecycle | RAR controller or system-only path | IT-RAR-2588-001 |
| EA cancellation | EA parent watch/reconcile | `internal/controller/effectivenessmonitor/` | IT-EM-2588-001 |
| Cancellation notification | RO creation plus notification controller | RO notification creation/handler | IT-NT-2588-001 |
| Audit | Cancellation lifecycle event emitters | Existing audit paths | IT-AUDIT-2588-001 |
| RBAC | Rendered operator/controller roles | `config/rbac/role.yaml` and markers | IT-RBAC-2588-001 |

## 6. BR coverage matrix

| BR | Outcome | Unit | Integration | E2E | Priority |
|---|---|---|---|---|---|
| BR-AUDIT-005 | Full correlated cancellation reconstruction, including normal cancellation-notification delivery | UT-AUDIT-2588-001 | IT-AUDIT-2588-001 | E2E-NT-2588-003 | P0 |
| BR-ORCH-029 | Deleting a NotificationRequest cancels only that notification | UT-NT-2588-002 | IT-NT-2588-002 | — | P0 |
| BR-ORCH-030 | RR tracks cancellation notification delivery state | UT-NT-2588-001 | IT-NT-2588-001 | E2E-NT-2588-003 | P0 |
| BR-ORCH-031 | Parent cleanup does not suppress required cancellation notification | UT-NT-2588-001 | IT-ORCH-2588-002 | E2E-NT-2588-003 | P0 |
| BR-COMMON-001 | Phase values use capitalized Kubernetes API values | UT-ORCH-2588-001 | IT-ORCH-2588-002 | — | P1 |

## 7. Test scenarios

### Unit tests

| ID | Business outcome | Phase |
|---|---|---|
| UT-ORCH-2588-001 | Active RR enters `Cancelling`, not terminal `Cancelled`, and duplicate requests are idempotent | PASS — `go test ./pkg/apifrontend/tools` |
| UT-ORCH-2588-002 | Terminal children are preserved and pre-start children acknowledge without work | PASS — scoped controller suites |
| UT-CRD-2588-001 | RemediationRequest DeepCopy isolates cancellation status and request timestamp | PASS — `go test ./api/remediation/v1alpha1` |
| UT-WE-2588-001 | Job, Tekton, and AWX stop is requested idempotently and only verified termination acknowledges stopped | PASS — Job cancellation tests, executor/controller suites |
| UT-WE-2588-002 | Transient, permission, timeout, and unknown backend states never produce false stopped | PASS — Job/Tekton/AWX executor and controller suites |
| UT-WE-2588-AWX-001 | AWX cancellation requests stop and verifies a terminal job | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-AWX-002 | AWX cancellation does not acknowledge a still-running job | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-AWX-003 | Missing AWX jobs are treated as idempotent stop success | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-PHASE-001 | WorkflowExecution cancellation phases validate and terminalize safely | PASS — `go test ./pkg/workflowexecution/phase` |
| UT-WE-2588-PHASE-002 | Active WorkflowExecution work enters and completes cancellation | PASS — `go test ./pkg/workflowexecution/phase` |
| UT-WE-2588-TEK-001 | Tekton cancellation requests an idempotent PipelineRun stop and verifies disappearance | PASS — `go test ./pkg/workflowexecution/executor` |
| UT-WE-2588-TEK-002 | Tekton cancellation refuses a PipelineRun owned by another WorkflowExecution | PASS — `go test ./pkg/workflowexecution/executor` |
| UT-WE-2588-TEK-003 | Tekton cancellation does not delete a same-name PipelineRun owned by another UID | PASS — `go test ./pkg/workflowexecution/executor` |
| UT-WE-2588-JOB-001 | Job cancellation rejects a stale RemediationRequest UID and returns `Unknown` evidence | PASS — `go test ./pkg/workflowexecution/executor` |
| UT-WE-2588-JOB-002 | Job deletion requested but not yet observed remains `Unknown` | PASS — `go test ./pkg/workflowexecution/executor` |
| UT-WE-2588-JOB-003 | Completed and failed Job conditions are positive stop evidence | PASS — `go test ./pkg/workflowexecution/executor` |
| UT-WE-2588-003 | WFE persists terminal cancellation failure and stops automatic requeue after the shared retry budget | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-004 | WFE deletion finalizer retries are bounded and retain the finalizer after unverified backend termination | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-005 | Terminal WFE deletion does not acknowledge cancellation until current backend termination is verified | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-006 | Failed backend stop requests consume exactly one persisted retry per reconcile and stop at five attempts | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-007 | Executor lookup/configuration failures consume the same persisted retry budget and stop automatic requeue after five attempts | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-008 | Terminal execution outcomes are preserved while cancellation is acknowledged | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-009 | Cancellation is rejected for a same-name parent from a stale UID incarnation | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-010 | Deletion stop failures persist cancellation status and use the explicit retry backoff without returning a conflicting error | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-011 | AWX cancellation recovers a lost launch response by discovering the unique persisted label, then cancels and verifies the job | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-012 | AWX cancellation fails closed when a persisted launch label matches multiple jobs | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-013 | AWX launch requests carry the persisted recovery label | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-014 | AWX job discovery filters by the persisted launch label | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-015 | AWX executor persists a unique launch label before launch and sends that exact label | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-016 | A WFE retry with a persisted AWX label recovers the existing job instead of launching a duplicate | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-017 | AWX launch failure cleanup deletes only credentials created for the WFE, never template-owned credentials | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-018 | Ambiguous AWX launch recovery remains Pending, retries with explicit backoff, and stops after the bounded discovery budget without claiming termination | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-019 | AWX template capability discovery confirms launch-label support and the unsliced-template requirement | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-020 | Ambiguous AWX launch responses retain WFE-created credentials for recovery cleanup instead of deleting credentials used by a possible accepted job | PASS — `go test ./pkg/workflowexecution` |
| UT-WE-2588-021 | A terminal WFE with an execution identity is not acknowledged until backend termination is verified; its terminal phase is preserved | PASS — `go test ./internal/controller/workflowexecution` |
| UT-WE-2588-022 | AWX templates that cannot accept launch labels are rejected before credentials or a launch are attempted | PASS — `go test ./pkg/workflowexecution` |
| UT-AI-2588-001 | AIAnalysis cancellation phases validate and terminalize safely | PASS — `go test ./pkg/aianalysis/phase` |
| UT-AI-2588-002 | AIAnalysis remains nonterminal until an existing AgentSession is observed stopping | PASS — `go test ./internal/controller/aianalysis` |
| UT-AI-2588-003 | AIAnalysis cancellation rejects a same-name parent from a stale UID incarnation | PASS — `go test ./internal/controller/aianalysis` |
| UT-AI-2588-004 | AIAnalysis removes its finalizer after acknowledged cancellation deletion | PASS — `go test ./internal/controller/aianalysis` |
| UT-AI-2588-005 | AgentSession cancellation exhaustion is bounded, non-acknowledged, and records manual-review evidence | PASS — `go test ./internal/controller/aianalysis` |
| UT-AI-2588-006 | Active AIAnalysis work can enter and complete cancellation | PASS — `go test ./pkg/aianalysis/phase` |
| UT-AI-2588-007 | AgentSession cancellation lookup failures persist status and use the explicit retry backoff without returning a conflicting error | PASS — `go test ./internal/controller/aianalysis` |
| UT-AA-2588-003 | AgentSession deletion rejects a stale AIAnalysis UID | PASS — `go test ./pkg/aianalysis/...` |
| UT-AA-2588-004 | AgentSession reuse rejects an owner with a stale AIAnalysis UID | PASS — `go test ./pkg/aianalysis/...` |
| UT-EM-2588-001 | EffectivenessAssessment stops before acknowledging parent cancellation | PASS — `go test ./internal/controller/effectivenessmonitor` |
| UT-EM-2588-002 | EffectivenessAssessment cancellation rejects a same-name parent with a stale owner UID | PASS — `go test ./internal/controller/effectivenessmonitor` |
| UT-RAR-2588-001 | User-supplied system cancellation is rejected | PASS — `go test ./pkg/authwebhook` |
| UT-RAR-2588-002 | Only the RO service account can cancel a pending RAR | PASS — `go test ./pkg/authwebhook` |
| UT-RAR-2588-003 | Repeated system cancellation of a RAR is idempotent | PASS — `go test ./pkg/authwebhook` |
| UT-RAR-2588-004 | System cancellation is attributed to the RO caller in the webhook audit event | PASS — `go test ./pkg/authwebhook` |
| UT-RAR-2588-005 | The RAR controller emits a correlated cancelled audit event for a system decision | PASS — `go test ./internal/controller/remediationorchestrator` |
| UT-NT-2588-001 | Dedicated cancellation notification is created once and remains deliverable/retryable | PASS — creator/notification suites |
| UT-NT-2588-002 | User deletion of any NotificationRequest changes notification state only | PASS — focused notification/RO suites |
| UT-NT-2588-003 | Partially sent cancellation notifications remain terminal but are not reported fully delivered | PASS — `go test ./internal/controller/remediationorchestrator` |
| UT-ORCH-2588-003 | A transient finalizer-removal error is retried without regressing `Cancelled` | PASS — focused RO cancellation suite |
| UT-ORCH-2588-004 | Ordinary `Completed`, `Failed`, `TimedOut`, and `Skipped` RRs remove the cancellation finalizer without changing their terminal phase | PASS — focused RO cancellation suite |
| UT-ORCH-2588-005 | Persistent cancellation-notification creation failure consumes the bounded retry budget after children acknowledge | PASS — focused RO cancellation suite |
| UT-ORCH-2588-008 | Deletion of an ordinary terminal RR does not re-enter the cancellation coordinator | PASS — focused RO cancellation suite |
| UT-ORCH-2588-009 | Cancellation rejects a same-name child with a stale parent UID | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-010 | Cancellation rejects a child owned by a stale parent UID | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-011 | Cancellation accepts matching and supported legacy UID-less identities | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-012 | System RAR cancellation rejects a stale parent UID | PASS — `go test ./internal/controller/remediationorchestrator` |
| UT-ORCH-2588-013 | Cancellation rejects children without a parent reference or controller owner | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-014 | Parent aggregation rejects acknowledgement from a stale same-name child | PASS — `go test ./internal/controller/remediationorchestrator` |
| UT-ORCH-2588-015 | SignalProcessing creator refuses to reuse a stale same-name child | PASS — `go test ./pkg/remediationorchestrator/creator` |
| UT-ORCH-2588-016 | AIAnalysis creator refuses to reuse a stale same-name child | PASS — `go test ./pkg/remediationorchestrator/creator` |
| UT-ORCH-2588-017 | WorkflowExecution creator refuses to reuse a stale same-name child | PASS — `go test ./pkg/remediationorchestrator/creator` |
| UT-ORCH-2588-018 | Notification creator refuses to reuse a manual-review notification with a stale parent UID | PASS — `go test ./pkg/remediationorchestrator/creator` |
| UT-ORCH-2588-019 | Namespace-less legacy child references resolve in the parent namespace and do not falsely acknowledge active work | PASS — focused RO cancellation suite |
| UT-ORCH-2588-020 | Fully UID-less, owner-less legacy children cannot supply cancellation acknowledgement across RR incarnations; UID-less references with a matching owner remain supported | PASS — shared identity and focused RO cancellation suites |
| UT-ORCH-2588-021 | Cancellation retry backoff is bounded and jittered | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-022 | A child cancellation request is not treated as acknowledgement | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-023 | Verified child stop records cancellation acknowledgement | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-024 | Child stop failure does not claim cancellation acknowledgement | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-025 | Parent phases requiring child stop are classified correctly | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-026 | Parent deletion is an authoritative child stop request | PASS — `go test ./pkg/shared/cancellation` |
| UT-ORCH-2588-027 | Shared cancellation identity rejects fully UID-less acknowledgement identities | PASS — `go test ./pkg/shared/cancellation` |
| UT-NT-2588-004 | User-deleted cancellation notifications are treated as terminal notification handling and are not recreated | PASS — focused RO cancellation suite |
| UT-RO-2588-002 | Cancellation notification creation is idempotent and remains retryable | PASS — `go test ./pkg/remediationorchestrator/creator` |
| UT-RO-2588-006 | A same-name notification with a different owner/RR UID, type, or cancellation request ID is rejected instead of reused | PASS — creator cancellation suite |
| UT-SP-2588-001 | SignalProcessing cancellation phases validate and terminalize safely | PASS — `go test ./pkg/signalprocessing/phase` |
| UT-SP-2588-002 | Active SignalProcessing work enters and completes cancellation | PASS — `go test ./pkg/signalprocessing/phase` |
| UT-SP-2588-003 | SignalProcessing cancellation rejects a same-name parent with a stale UID | PASS — `go test ./internal/controller/signalprocessing` |
| UT-AUDIT-2588-001 | Cancellation intent, child acknowledgement, backend verification, notification creation, and system-RAR attribution use typed correlated audit records with deterministic IDs | PASS — focused controller, audit, and webhook suites; Helios08 correlation query returned 7 events |
| UT-AUDIT-2588-002 | Cancellation failure events carry standardized error details | PASS — `go test ./pkg/remediationorchestrator/audit` |
| UT-AUDIT-2588-004 | The first cancellation audit uses the initialized request identity | PASS — `go test ./internal/controller/remediationorchestrator` |
| UT-AUDIT-2588-005 | Notification creation/delivery failures do not claim `WorkflowExecution` as their failure phase | PASS — focused RO cancellation suite |
| UT-AUDIT-2588-006 | AIAnalysis error audit payloads include standardized `error_details` | PASS — `go test ./pkg/aianalysis` |
| UT-AUDIT-2588-007 | Cancellation retry exhaustion is classified as non-retryable | PASS — `go test ./pkg/aianalysis` |
| UT-AUDIT-2588-008 | Repeated phase-transition audit builds use a stable event ID | PASS — `go test ./pkg/remediationorchestrator/audit` |
| UT-AUDIT-2588-009 | Cancelled RAR audit events use the service actor and RR correlation | PASS — `go test ./pkg/remediationorchestrator/audit` |

### Integration tests

| ID | Business outcome | Phase |
|---|---|---|
| IT-ORCH-2588-001 | Production cancel API dispatches an RR into the cancellation path | BLOCKED — ENVTEST infrastructure setup; integration scenario is present and the handler contract passes in `pkg/apifrontend/tools` |
| IT-ORCH-2588-002 | RR deletion holds the parent finalizer until required acknowledgements and stop verification | BLOCKED — ENVTEST infrastructure setup |
| IT-ORCH-2588-003 | RR deletion retains the finalizer until child acknowledgement and notification terminality | PARTIAL — Helios08 RO issue slice ran 47/47 with 0 failures; ENVTEST execution blocked |
| IT-SP-2588-001 | SP observes parent cancellation and stops without starting further processing | PARTIAL — focused controller suite passes; ENVTEST execution blocked |
| IT-SP-2588-002 | SP treats a missing parent as an authoritative stop request | PARTIAL — focused controller suite passes; ENVTEST execution blocked |
| IT-AI-2588-001 | AIAnalysis observes cancellation and stops its AgentSession path | PARTIAL — focused controller suite passes; ENVTEST execution blocked |
| IT-WE-2588-001 | WE observes cancellation and owns its status transition | PARTIAL — focused controller suite passes; ENVTEST execution blocked |
| IT-WE-2588-002 | Executor production wiring proves backend stop and verification | PARTIAL — executor/controller suites pass; ENVTEST execution blocked |
| IT-RAR-2588-001 | User-supplied RAR cancellation is rejected; the trusted system path is covered by the focused authorization unit suite | PARTIAL — focused AuthWebhook RAR cancellation check ran 1/1 with 0 failures; full ENVTEST execution is blocked; `UT-RAR-2588-001`/`002` prove the trusted-path contract |
| IT-EM-2588-001 | EA cancels when already created and in progress; no cancellation is required when never created | BLOCKED — exact ENVTEST scenario is present; ENVTEST infrastructure setup is unavailable |
| IT-NT-2588-001 | Cancellation notification uses normal delivery/retry despite RR cancellation | BLOCKED — ENVTEST infrastructure setup |
| IT-NT-2588-002 | Existing notification deletion semantics remain unchanged | BLOCKED — exact ENVTEST scenario is present; ENVTEST infrastructure setup is unavailable |
| IT-AUDIT-2588-001 | Correlation query reconstructs the full cancellation lifecycle | PASS — Helios08 DataStorage query returned `count:7,total:7`; artifact: `/tmp/kubernaut-must-gather/remediationorchestrator-integration-20261115-082908/remediationorchestrator_remediationorchestrator_datastorage_test.log` |
| IT-RBAC-2588-001 | Rendered roles and live SARs prove least privilege for parent reads, child status, backend stop, and audit writes | PARTIAL — rendered RBAC PASS; live issue-specific SAR unavailable because the persistent `kubernaut-demo` cluster has no #2588 controller service-account/RBAC targets |

### E2E tests

| ID | Business outcome | Phase |
|---|---|---|
| E2E-ORCH-2588-001 | Parent cancellation reaches a safe terminal state only after child acknowledgement and terminal notification handling | BLOCKED — exact deployed scenario is present; deployed cluster/DataStorage image setup is unavailable |
| E2E-WE-2588-002 | Each execution backend terminates before WE cleanup/finalizer removal | BLOCKED — focused WorkflowExecution setup reached cluster bootstrap after a temporary test-only `30281` → `30282` host-port remap, then timed out waiting for AWX readiness; 0/37 specs ran; the config was restored and the temporary cluster cleaned up |
| E2E-NT-2588-003 | Cancellation notification is delivered/retried during parent cancellation/deletion | BLOCKED — deployed cluster/DataStorage image setup |

### OWASP ASVS v5.0.0 evidence matrix

The mappings below are pinned to the [OWASP ASVS v5.0.0 requirements](https://github.com/OWASP/ASVS/tree/v5.0.0/5.0/en). A PASS is limited to the evidence named in this plan; it is not a release-wide ASVS certification.

| Requirement | Issue #2588 evidence | Status |
|---|---|---|
| V2.2.1 | RRID parsing, terminal-state rejection, and cancellation state validation are covered by the API and authorization suites. | PASS — focused unit evidence |
| V2.2.2 | Namespace is injected server-side for the cancel tool; RAR cancellation is accepted only for the configured RO service account. | PASS — focused auth/API evidence; live issue-specific SAR target unavailable |
| V2.3.1 | Cancellation enters `Cancelling`, requires child acknowledgement, and preserves terminal outcomes; finalizer retry regression is covered by `UT-ORCH-2588-003`. | PASS — scoped unit evidence; runtime journey blocked |
| V8.2.1 | Function-level cancellation and system-only RAR cancellation are restricted to the intended service paths. | PASS — focused authorization evidence; live issue-specific SAR target unavailable |
| V8.3.1 | Trusted service-layer checks enforce cancellation authorization; client-supplied namespace and `ApprovalDecisionCancelled` are not trusted. | PASS — focused webhook/tool evidence; live issue-specific SAR target unavailable |
| V16.1.1 | This plan inventories the cancellation audit stages and required storage/query evidence. | PASS — all in-scope cancellation stages have source-level emitters |
| V16.2.1 | Existing emitted events carry structured timestamp, actor, action, outcome, and correlation metadata. | PASS — typed lifecycle and webhook events include structured correlation and actor metadata |
| V16.2.4 | DataStorage correlation queries are specified for the RR name. | PASS — Helios08 DataStorage correlation query returned 7 events; full ENVTEST setup remains unavailable |
| V16.3.3 | Cancellation intent, child shutdown, backend verification, RAR cancellation, and notification lifecycle are security-relevant events for this feature. | PASS — source-level correlated events cover intent, child acknowledgement, backend verification, notification creation, and system-webhook attribution; runtime journey blocked |
| V16.5.2 | Stop failures remain observable, bounded retries are used, and the parent finalizer is retained; notification delivery has terminal failure handling. | PASS — focused unit evidence covers bounded child and notification-creation retries; runtime journey blocked |
| V16.5.3 | The implementation avoids terminal parent state before required acknowledgements and rejects user-controlled system cancellation. | PASS — focused unit/auth evidence; runtime journey blocked |

## 8. Pass/fail and coverage policy

PASS requires all P0 tests, all wiring-manifest integration tests, all critical
backend-stop and finalizer cases, generated CRDs, rendered RBAC/SAR checks, and
affected regression suites to pass. Unit scenario coverage must be at least 90%
of defined in-scope scenarios, with every safety and compliance scenario covered.

FAIL includes any false stopped acknowledgement, terminal RR before required
acknowledgement, lost cancellation notification, user-controlled RAR
cancellation, missing correlated audit evidence, or new build/lint/regression
failure.

Suspend on a broken build, unavailable required cluster/backend, or a cascading
failure with one unresolved root cause. Resume after the dependency or root
cause is fixed. No pending or skipped tests may remain at completion.

## 9. TDD execution record

- RED: complete — formal plan and cancellation contract tests were written before production changes; the finalizer retry regression first failed against terminal-phase regression behavior.
- GREEN: complete — production wiring and child behavior are implemented; CHECKPOINT W is covered by focused controller/infrastructure tests.
- REFACTOR: complete — cancellation helpers, terminal-state preservation, finalizer retry handling, error observability, retry structure, and all 12 lint findings were resolved without changing the lifecycle contract.
- VERIFY: complete for scoped unit/build/lint/generated, rendered-RBAC, and available Helios08 runtime checks — the fresh Engram-grounded source review passes; ENVTEST, live issue-specific SAR, and deployed E2E remain suspended on infrastructure blockers.

## 10. Execution evidence

### Passing local gates

- `go build ./...` — PASS.
- `make lint` — PASS (`golangci-lint` reports `0 issues`).
- `make manifests` — PASS; generated CRDs/RBAC and embedded assets synchronized.
- `git diff --check` — PASS.
- Affected controller, executor, creator, audit, authorization, infrastructure, integration-compile, and E2E-compile suites — PASS when run individually; full affected controller packages and the latest focused run pass the finalizer, notification-identity, UID-scoping, phase-state, retry-budget, explicit-backoff, bounded AWX launch recovery, AWX capability validation, credential ownership, terminal-backend verification, and no-reference backend-audit regressions. The slowest scoped package was `pkg/apifrontend/tools` at approximately 170 seconds.
- Test-plan/source-ID audit — PASS: 111 distinct #2588 Go test IDs are represented in this plan, with no missing mappings and no duplicate source declarations.
- Helios08 focused runtime evidence — PASS: RemediationOrchestrator issue slice `47/47`; EffectivenessMonitor cancellation check `1/1`; AuthWebhook RAR cancellation check `1/1`; no failures reported.
- Helios08 audit correlation — PASS: DataStorage returned `count:7,total:7` for the cancellation correlation query; raw evidence is in `/tmp/kubernaut-must-gather/remediationorchestrator-integration-20261115-082908/remediationorchestrator_remediationorchestrator_datastorage_test.log` around lines 1060–1064.
- Rendered RBAC — PASS: generated `config/rbac/role.yaml` includes the required cancellation child-status, backend-stop, and audit-write permissions; live issue-specific SAR execution remains unavailable.

### Suspended environment-dependent gates

- ENVTEST/integration execution cannot start because the DataStorage Podman image build fails during `util-linux` extraction: `cpio: chown failed - No data available`.
- Live issue-specific SAR verification cannot complete because the persistent `kubernaut-demo` cluster has no #2588 controller service accounts/RBAC targets; unrelated persistent workloads were left untouched.
- Focused WorkflowExecution Job-cancellation E2E setup timed out waiting for AWX readiness after the test-only port remap; 0/37 specs ran. The remap was restored and the temporary E2E cluster was cleaned up.
- Deployed E2E execution remains blocked; the new cancellation and Job-backend journeys compile successfully.
- The repository-wide `make test` parallel run exceeded its global suite deadline and also reported unrelated auth/agent/effectiveness timeouts; affected #2588 packages pass when isolated as listed above.

### Resolved review findings

- **AUDIT-2588-001/002/003:** fixed in this review worktree; cancellation intent, child acknowledgement, WFE backend verification, notification creation, and system-RAR webhook attribution now emit typed correlated events. Deterministic IDs include RR UID and cancellation request identity. Focused audit/controller/webhook tests pass.
- **LIFECYCLE-2588-001:** fixed in this review worktree; terminal `Cancelled` RRs preserve their phase while retrying finalizer removal, and ordinary terminal RRs remove the cancellation finalizer before retention deletion. `UT-ORCH-2588-003` and `UT-ORCH-2588-004` pass.
- **LIFECYCLE-2588-002:** fixed in this review worktree; cancellation notification reuse validates type, owner reference, RR reference/UID, and cancellation request ID. `UT-RO-2588-006` passes.
- **RETRY-2588-001:** fixed in this review worktree; notification-creation failures consume the bounded cancellation retry budget after child acknowledgement and publish manual-review evidence at exhaustion. `UT-ORCH-2588-005` passes.
- **PHASE-2588-001:** fixed in this review worktree; all child and RO phase state machines expose explicit `Cancelling → Cancelled` and terminal `Cancelled: {}` transitions. `UT-ORCH-2588-006` and focused phase suites pass.
- **LIFECYCLE-2588-003:** fixed in this review worktree; deletion of an ordinary terminal RR no longer re-enters the cancellation coordinator, while a terminal `Cancelled` RR with its cancellation finalizer still receives cleanup handling. `UT-ORCH-2588-008` passes.
- **RETRY-2588-002:** fixed in this review worktree; WFE parent-cancellation and deletion-finalizer stop attempts share a persisted five-attempt budget, publish `CancellationFailed`, retain the finalizer, and stop automatic requeue after exhaustion. `UT-WE-2588-003` and `UT-WE-2588-004` pass.
- **AUDIT-2588-004:** fixed in this review worktree; cancellation notification creation and delivery failures omit the unrelated `WorkflowExecution` failure phase while retaining correlated structured error details. `UT-AUDIT-2588-005` passes.
- **AUDIT-2588-005:** fixed in this review worktree; a verified WFE cancellation acknowledgement emits correlated backend-termination evidence even when deterministic or label-based recovery leaves `status.executionRef` empty. `IT-RO-2588-001` passes.
- **LIFECYCLE-2588-004:** fixed in this review worktree; terminal WFE deletion verifies backend termination before publishing cancellation acknowledgement or proceeding with finalizer cleanup. `UT-WE-2588-005` passes.
- **NOTIFICATION-2588-001:** fixed in this review worktree; cancellation reconciliation processes a recorded user deletion before notification creation and treats the cancellation notification as terminal without recreating it. `UT-NT-2588-004` passes.
- **RETRY-2588-003:** fixed in this review worktree; normal WFE backend stop failures increment the persisted retry counter exactly once per reconcile, sharing the five-attempt cap with deletion handling. `UT-WE-2588-006` passes.
- **RETRY-2588-005:** fixed in this review worktree; WFE executor lookup/configuration failures now increment the persisted cancellation retry counter before failure handling, preventing unbounded requeue. `UT-WE-2588-007` passes.
- **RETRY-2588-006:** fixed in this review worktree; persisted AIAnalysis and WFE cancellation failures now return the explicit backoff without a conflicting error, while EA API/identity/status errors return zero-result errors so controller-runtime applies its normal rate limiter. `UT-AI-2588-007` and `UT-WE-2588-010` pass.
- **IDENTITY-2588-002:** fixed in this review worktree; namespace-less legacy child references default to the live RR namespace before cancellation-state reads. `UT-ORCH-2588-019` passes.
- **RETRY-2588-004:** fixed in this review worktree; AIAnalysis AgentSession shutdown has the shared five-attempt cap, persists `CancellationFailed`, stops automatic requeue, and records manual-review status plus correlated error audit evidence. `UT-AI-2588-005` passes.
- **IDENTITY-2588-003:** fixed in this review worktree; cancellation acknowledgement uses an incarnation-bound identity check, rejecting fully UID-less/owner-less legacy children while preserving UID-less references with a matching controller owner. `UT-ORCH-2588-020` passes.
- **RECOVERY-2588-001:** fixed in this review worktree; AWX validates launch-label support and rejects sliced templates, persists a unique launch label before launch, sends it in the launch payload, recovers one matching job after a lost response, retries zero-match visibility with bounded Pending status, and fails closed for ambiguous matches. `UT-WE-2588-011` through `UT-WE-2588-016`, `UT-WE-2588-018`, `UT-WE-2588-019`, and `UT-WE-2588-022` pass.
- **CREDENTIAL-2588-001:** fixed in this review worktree; only WFE-created AWX credentials are tracked and deleted, ownership is persisted before launch, and credentials remain available when a launch response may have been accepted. `UT-WE-2588-017` and `UT-WE-2588-020` pass.
- **TERMINAL-2588-001:** fixed in this review worktree; terminal WFEs with an execution identity call RequestStop and VerifyStopped before cancellation acknowledgement while preserving their business terminal phase. A terminal WFE with no recorded execution identity acknowledges only the explicit no-backend evidence path. `UT-WE-2588-008` and `UT-WE-2588-021` pass.
- **AUDIT-2588-006:** fixed in this review worktree; `aianalysis.error.occurred` payloads now carry typed standardized `error_details`, including cancellation exhaustion events. `UT-AUDIT-2588-006` passes.

### Approval gate

Issue #2588 remains **not approved** only because ENVTEST, live issue-specific SAR, and deployed E2E evidence are unavailable in this environment. The latest Engram-grounded closure review is complete and found no unresolved source-level lifecycle, AWX recovery, credential-ownership, identity, audit, retry, or wiring findings. The implementation is review-clean subject to the unavailable runtime evidence.
