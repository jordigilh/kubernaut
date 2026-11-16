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

### Post-validation fixes for run 43937713539 (head 5fd6e87f7)

Three source-attributed causes, proven by CI logs and must-gather:

1. **AA E2E controller crash loop** — the `aianalysis-controller` container
   exited with status 1 roughly every 120 s (containerd events, pod
   `aianalysis-controller-6f8d76c47-zdmzv` CrashLoopBackOff). The deployed
   RBAC already granted `remediationrequests get/list/watch`
   (`clusterrole/aianalysis-controller created` in the job log), but
   `cmd/aianalysis` never registered `remediationv1` in its production
   scheme, so the new parent watch can never sync. Same defect class as the
   `cmd/workflowexecution` fix. RED: IT-AA-2588-010 resolves RR/RRList from
   the actual production scheme and fails. GREEN: one `AddToScheme` line.
   SignalProcessing and EffectivenessMonitor already register the type.
2. **AWX label launch rejected** — every Ansible-engine failure returned
   HTTP 400 `{"labels":["Incorrect type. Expected pk value, received str."]}`:
   the launch payload sent label names, but AWX requires label primary keys.
   RED: UT-WE-2588-011 (resolve-existing and create-missing paths) fails.
   GREEN: `resolveLabelIDs` resolves each label name through the AWX labels
   API and creates a missing label inside the template's organization; the
   launch payload now carries integer PKs. UT-WE-2588-013 was updated to the
   proven PK contract.
3. **E2E-WE-2588-002 fixture** — `fmt.Sprintf("%064x", uuid.New())`
   hex-encodes the UUID's 36-character dashed `String()` (Stringer applies
   to `%x`), producing 72 characters and failing the CRD's 64-byte ceiling.
   Replaced with a SHA-256 hex digest of the test name.

| Component | Production entry | Wiring location | Proving test |
|-----------|------------------|-----------------|--------------|
| Parent RR type registration | AIAnalysis process startup | cmd/aianalysis/main.go init | IT-AA-2588-010 |
| AWX label PK resolution | Ansible executor launch | pkg/workflowexecution/executor/awx_client.go | UT-WE-2588-011/013 |

Review follow-up hardening (round 1): label matching and creation are scoped
to the template's organization — AWX labels are keyed by (organization,
name), so a same-named foreign-organization label is never reused; a missing
label is created inside the template organization only. Transient failures
on the label path (template fetch, label search, label create) that are 5xx
or transport-level are classified `ErrAWXLaunchRecoveryPending` so the
controller retries instead of terminalizing; deterministic 4xx responses
surface as `AWXAPIError`. A template without an organization is rejected
rather than guessed. UT-WE-2588-011 grew to 11 scenarios covering
organization scoping, cross-organization duplicates, both transient classes,
both deterministic 4xx classes, the no-organization guard, and the
invalid-create-id ambiguity; UT-WE-2588-013 asserts the organization-scoped
PK contract. Scoped changed AWX logic coverage, measured by merging both
test packages' profiles over the four changed functions
(`resolveLabelIDs`, `templateOrganization`, `resolveLabelID`,
`createLaunchLabel`): 59/66 statements (89.4%).

Known follow-up trade-off (review round 2, not a regression): the launch
label is persisted before the AWX launch POST, so a recovery-pending label
failure requeues into discovery-only recovery; with zero matching jobs this
retains the WFE in Pending until retry exhaustion rather than retrying the
full launch. This extends the pre-existing double-launch-protection design
(a lost launch POST already behaved this way) to pre-POST failures where a
full retry would be safe; hoisting label resolution before
`ensureAWXLaunchLabel` is the follow-up. The same template GET is also
classified differently at `validateAWXJobTemplate` (pre-existing,
terminalizing) versus `templateOrganization` (recovery-pending); unifying
them by passing the fetched template through is a follow-up.

Validation: RED observed for IT-AA-2588-010 and UT-WE-2588-011 before the
fixes; after GREEN, `cmd/aianalysis`, `cmd/workflowexecution`,
`pkg/workflowexecution/...`, both controller unit suites, E2E/IT compile
checks, full lint and diff checks pass. Deployed AA/WE E2E must be re-proven
by the next CI run.

### CI operator builder-version compatibility follow-up

Run 43884842808 at 9bedb2f5f failed both operator architecture jobs before
Buildx: operator commit 48d9d91df942c7e851c937cca02bf06ce856d23d updates the
builder to go-toolset:10.3, but the CI-only native-builder rewrite and its guard
match only 10.2. Preserve the selected operator's image reference instead of
pinning, downgrading or modifying that repository.

BR-PLATFORM-008; RED UT-INFRA-CI-2643-001 executes the actual workflow run block
with 10.2, 10.3, digest-pinned and unexpected builder fixtures. Assert exact output
with literal `$BUILDPLATFORM`, unchanged source and fail-closed rejection of an
unexpected builder. GREEN uses one capture-preserving rewrite plus its guard.
REFACTOR N/A for this localized configuration fix. Validate the shell fixtures,
infrastructure unit suite, lint and diff checks without building images. Both CI
operator architectures and downstream E2E must pass on the next pushed commit.

### Production WorkflowExecution parent-watch scheme registration

Run 43801432628 deployed WE E2E is blocked before reconciliation: must-gather
records `no kind is registered for the type v1alpha1.RemediationRequest`, then
parent-watch cache sync timeout. Before the fix, `cmd/workflowexecution.init`
registered WFE, workflow catalog and Tekton types, but not RR. Integration setup separately
registers RR, so a passing controller integration suite cannot detect this.

RED: IT-WE-2588-026 resolves both RR and RRList using the actual production
scheme initialized by `cmd/workflowexecution.init`; neither test registers types.
GREEN: add the existing RR AddToScheme call alongside the existing registrations.
REFACTOR: N/A (one existing registration pattern, no new component).
BR-ORCH-029; no audit schema, permissions or lifecycle semantics change.

| Component | Production entry | Wiring location | Proving test |
|-----------|------------------|-----------------|--------------|
| Parent RR type registration | WE process startup | cmd/workflowexecution/main.go init | IT-WE-2588-026 |

Validate RED/GREEN, cmd and controller unit suites, build/lint and affected
compile checks. CI WE E2E must prove deployed cache sync and dispatch after push.

### WorkflowExecution normal-execution fixture follow-up

Test-only scope (BR-ORCH-029, BR-WE-014): create real active parent RRs before
normal-execution WFE creation and bind the actual API-server UID, namespace,
API version and same-namespace controller owner. Do not change production
cancellation behavior or the ownerless/owned missing-parent unit tests
UT-WE-2588-022/023. No new production components or wiring points.

- RED (5 minutes): IT-WE-2588-024 exercises Job, Tekton and minimal fixtures
  through the running controller and requires a persisted Executing parent,
  matching reference/owner UID and Running WFE with an execution reference.
- GREEN (15 minutes): an explicit test creation helper provisions the parent
  before WFE creation; normal suite call sites use it, while raw client creation
  remains available for missing-parent scenarios.
- REFACTOR: remove fabricated Job-parent UIDs and centralize parent setup and
  cleanup without changing target, workflow, audit-correlation or phase assertions.
- Verification: focused fixture IT, full WE integration, unchanged missing-parent
  unit specs, compile/build/lint and diff checks. Use the already loaded CI ARM64
  DataStorage artifact only; no local image builds. Estimate 10–15 minutes for
  the full integration suite. Report infrastructure blockers separately.

Success: all three fixture variants prove the active-parent dispatch contract;
both missing-parent contracts remain green. A failed unrelated baseline does not
authorize unrelated production changes or relaxed assertions. Fixture rollback
is test-only; the separate startup registration fix is described above.

Implementation: normal suite creation sites now call
`createWorkflowExecutionWithActiveParent`; it persists an RR, sets Executing
through the status subresource, and binds the API-server-assigned UID before
creating the WFE. Existing same-name active parents are reused rather than
replaced. Same-namespace controller ownership is set explicitly; cross-namespace
spec references retain the real namespace/UID without an invalid owner reference.
The Job builder no longer fabricates a parent UUID. IT-WE-2588-025 deliberately
uses raw client creation to prove owned missing-parent cancellation remains
available. UT-WE-2588-022/023 and production cancellation semantics are unchanged;
the production scheme registration is the only production change in this follow-up.
IT-WE-2588-027 additionally proves same-namespace defaulting, cross-namespace
reference persistence without invalid ownership, and sequential reuse of an
active parent's UID/resourceVersion. It does not promise concurrent creation of
the same parent by independent helper calls.

Validation: full WE controller unit suite, the two focused missing-parent specs,
integration-package compilation, repository build/compile and full lint pass.
The normal integration invocation is blocked in BeforeSuite: the CI ARM64
DataStorage image is used successfully, but Podman cannot unpack PostgreSQL
(`lchown /etc/shadow: invalid argument`); no behavior specs execute, and teardown
also encounters its pre-existing nil-manager panic. No image was built or Podman
configuration changed to bypass this blocker.

Reproducible checks use tracked tests, not temporary diagnostic scripts:

```bash
go test ./cmd/workflowexecution ./internal/controller/workflowexecution -count=1
go test ./test/integration/workflowexecution -run='^$' -count=1
golangci-lint run --timeout=5m
```

In CI with preloaded operand artifacts, run the focused suite with
`-ginkgo.focus='IT-WE-2588-02[457]'`, then the full
`make test-integration-workflowexecution` and `make test-e2e-workflowexecution`.
Do not run image-building targets locally. The tracked tests are
`cmd/workflowexecution/scheme_2588_test.go` and
`test/integration/workflowexecution/parent_fixture_test.go`; CI logs/artifacts
must be tied to the pushed commit. Local temporary admission diagnostics are
supplementary only and are not a replacement for controller/dispatch or
DataStorage/auth proof. Full IT-WE-2588-024/025/027 and deployed WE E2E remain
unverified for this diff until that CI run passes.

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
| UT-AI-2588-008 | An ownerless AIAnalysis with a missing parent is not cancelled; only a persisted controller owner proves an orphaned production child (mirrors UT-WE-2588-022) | PASS — `go test ./internal/controller/aianalysis` |
| UT-AI-2588-009 | An owned AIAnalysis whose parent RemediationRequest is missing still fails closed to ParentCancelled cancellation (mirrors UT-WE-2588-023) | PASS — `go test ./internal/controller/aianalysis` |
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
| UT-ORCH-2588-028 | A normally pending child stop does not consume the RemediationRequest aggregate retry budget | PASS — `go test ./internal/controller/remediationorchestrator` |
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
| IT-AA-2588-003 | Generated controller RBAC grants required RemediationRequest parent-read verbs, while the direct AIAnalysis E2E ClusterRole is exactly read-only so its parent-cancellation informer can synchronize (E2E crash-loop fix, run 43913953987) | PASS — `go test ./test/infrastructure` |

The focused AIAnalysis envtest suites `cascadecancel` and `schemarejection`
also register `remediationv1alpha1` because the reconciler now performs the
parent RemediationRequest read before those scenarios exercise their existing
behavior. This is test-scheme wiring only; no production cancellation contract
is relaxed.

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

## Audit 409 regression follow-up (PR #2643)

Scope: BR-AUDIT-005 / DD-AUDIT-010, SOC2 CC7.2 and FedRAMP AU-2/AU-3/AU-9.
This follow-up supersedes the earlier source-level audit-clear statement above:
stage/outcome-only cancellation IDs can conflict when the logical observation changes.

- **RED (~5 minutes):** UT-AUDIT-2588-010 covers phase progress after a failed
  cancellation status update and changed actor attribution; UT-AUDIT-2588-011
  covers changed failure details. UT-AUDIT-2588-012 exercises production RO
  initialization on pre/post-progress snapshots. Identical observations must retain their ID.
  IT-AUDIT-2588-002 proves distinct observations and identical retries survive
  the real buffered DataStorage ingestion/query path.
- **GREEN (~5 minutes):** bind the existing deterministic cancellation identity
  to canonical event content, excluding the observation timestamp. Change only
  the audit builder; preserve request/UID identity, all-child acknowledgement,
  persisted retry budgets, finalizers, and notification behavior.
- **REFACTOR:** N/A if the minimal builder change leaves no separate cleanup.
- **Validation (~10 minutes):** focused RED/GREEN, audit/controller unit suites,
  real PostgreSQL/HTTP-handler replay, affected integration test, build and lint.
  No new component or wiring point is introduced. The existing audit wiring row
  remains authoritative; IT-AUDIT-2588-002 adds ingestion/idempotency proof.
- **Rollback/risk:** revert only this follow-up commit if necessary. Content-bound
  IDs differ from earlier stage-only IDs; replay across versions can therefore
  add one observation rather than deduplicate against the old key. Do not weaken
  DataStorage conflict detection or drop retained audit events to mask a failure.

Diagnostic evidence at starting HEAD `2c9b0c3d3`: production RO initialization
on snapshots of the archived RR identity (namespace `kubernaut-system-p2`,
name `rr-rar-schema-7e82e144`, UID `226e9720-7803-43e9-931e-af93da4ae4dc`)
emits ID `c5e30c66-1c8c-57ae-8ed0-9135ea288962` for both `Pending -> Cancelling`
and `Processing -> Cancelling` request observations. A real PostgreSQL 16 database
with production migrations 001–020 and the production DataStorage batch HTTP
handler accepts identical replay (201), then rejects the changed payload (409).
This reconstructs a collision using archived identity; the original CI request
bodies and conflicting ID were not captured, so attribution of that exact CI
failure remains evidence-supported rather than directly observed.

The initial focused integration replay ran zero specs: DataStorage image startup
failed during `dnf update` (`/usr/bin/write`: `cpio: chown failed - No data available`).
Do not attribute that environment failure to the audit change.

### Follow-up validation results

- RED: all four new unit scenarios failed against the stage/outcome-only IDs;
  PostgreSQL/HTTP-handler replay captured the conflict detail above.
- GREEN: audit manager (62 specs) and RO controller (433 specs) pass. The broader
  affected race run passes for `./pkg/remediationorchestrator/...`,
  `./internal/controller/remediationorchestrator`, `./cmd/remediationorchestrator`,
  `./pkg/audit`, and `./pkg/datastorage/repository`.
- Native integration GREEN: production RO initialization, BufferedAuditStore,
  generated OpenAPI HTTP client, production DataStorage batch handler, and real
  PostgreSQL 16 with migrations 001–020 persist two distinct observations while
  deduplicating identical replay. This diagnostic bypasses auth middleware; it
  is not a substitute for envtest TokenReview/SAR or deployed E2E qualification.
- `go build ./...`, scoped `golangci-lint run --timeout=5m` (zero issues), integration
  package compile-only, and `git diff --check` pass. No new component, lifecycle
  transition, dependency, or retry behavior was introduced. Anti-pattern review:
  no added interface, context field, goroutine, ignored error, or deep nesting.
- IT-AUDIT-2588-002 is implemented and compiles, but its full infrastructure run
  remains blocked by the local image-build failure. No tests were disabled or
  weakened. Scoped new unit scenarios: 4/4 passing, including critical integrity
  contracts; existing UID/deterministic-replay tests remain green.
- Starting-HEAD CI run `43791520735` subsequently passed RO integration and E2E
  `#1111`, confirming nondeterminism rather than disproving the reproduced race.
  Its RO E2E job finished 36 passed / 3 failed, all in `approval_e2e_test.go`
  (lines 234, 322, and BeforeEach 586). These are not certified resolved by this
  audit-only fix; their attribution is outside this follow-up's evidence.

Artifacts (local, not committed):
`/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/audit409-diagnostic/`
contains `events.json`, `problem.json`, `events-green.json`, RED/GREEN/race logs,
temporary Go overlays and the failed infrastructure replay log.

### Remaining RAR audit failures and lint closure

Run `43791520735` must-gather records 745 RO batch HTTP 409 responses. The
retained approval RARs `rar-e2e-rar-audit-1ebbc625` and
`rar-e2e-rar-persist-35229e98` have `AuditRecorded=True` with explicit
"accepted by audit buffer" messages at 02:47:13Z and 02:47:15Z, after the
retained 409 log begins at 02:47:08Z. Approval and cancellation controllers
share the production audit store. This supports shared-buffer blockage for the
missing orchestration events, not a failure to make or enqueue the decisions.
The rejection RAR is absent from the final snapshot; its exact enqueue state
cannot be reconstructed from this archive alone.

Extend IT-AUDIT-2588-002 (BR-AUDIT-005/006, CC7.2, AU-2/AU-3) before further
production changes: enqueue both approval and rejection events behind changed
cancellation observations in the same buffer, and prove each persists under
its own correlation ID while identical cancellation replay still deduplicates.
Keep E2E exact event-count assertions and timeouts unchanged. RED uses the
old builder's conflicting IDs; GREEN uses the content-bound IDs. No additional
production change or component is planned. REFACTOR: N/A for assertion-only
extension. Post-fix CI must still prove the three deployed RAR scenarios.

The two lint findings in UT-CRD-2588-001 are test style only: avoid shadowing
the predeclared `copy` identifier and use the promoted `Add` method. Preserve
the existing DeepCopy isolation assertions; validate the API unit suite and
full repository lint.

Closure validation: the API, audit-manager, and RO-controller unit suites pass;
the extended authenticated integration package compiles; full repository lint
reports zero issues. A dedicated native PostgreSQL replay with production
buffer/client/HTTP-handler wiring persists both RAR decisions behind cancellation
replay, retains exactly two cancellation observations, and still rejects changed
content forced under an existing ID with RFC7807 HTTP 409. Evidence:
`/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/audit-409-rar-fallout-proof.log`.
This diagnostic bypasses auth middleware; post-fix deployed CI remains the gate
for declaring all three original RAR E2E failures resolved. No production RAR
logic or E2E assertion/timeout was changed.
