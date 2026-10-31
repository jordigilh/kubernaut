# Test Plan: Issue #2582 — Fail-Closed RO Child Lifecycle

**Test Plan Identifier**: TP-2582-v1
**Feature**: Provenance-safe child creation, parent-phase gates, and verified cancellation across the remediation pipeline.
**Version**: 1.0
**Created**: 2026-10-31
**Author**: Kubernaut maintainers
**Status**: Active
**Branch**: `issue-2582-preflight-origin-main`

## 1. Purpose and success criteria

This plan proves that an unexpected or concurrently-created child is never
adopted, that a child cannot perform consequential work before its persisted
parent phase authorizes it, and that parent failure stops active work without
erasing terminal history. It also protects the explicit NotificationRequest
idempotency exception and the AF InvestigationSession-before-RR contract.

Pass criteria:

1. All P0 scenarios pass; no pending/skipped BDD examples.
2. All wiring-manifest rows have production-path integration proof.
3. Critical safety, identity, cancellation, and audit scenarios are 100% covered;
   at least 90% of meaningful in-scope unit scenarios pass.
4. Generated CRDs/RBAC/schemes match API source and least-privilege assumptions.
5. Existing affected service suites have no regression.

## 2. Authority and scope

- Issue #2582 and the approved OpenChamber plan `ee869748-7219-468e-8012-02349a0393bf`.
- DD-2582: `docs/architecture/decisions/DD-2582-fail-closed-ro-child-lifecycle.md`.
- BR-ORCH-025/026/029/030/036/045.2; BR-AI-007/009/010/023; BR-WE-007/014/015;
  BR-INTERACTIVE-010; BR-AUDIT-005; BR-COMMON-001; applicable SP/EM requirements.
- FedRAMP AC-4, AC-6, SI-10, AU-2, AU-3, AU-9, AU-11 and SOC2 CC7.2/CC8.1.

In scope: RO, SP, AIA/AgentSession/KA, RAR, WE Job/Tekton/AWX, EA, Notification,
AF IS creation/closure, generated artifacts, RBAC, indexes/watches, and audit
reconstruction. Out of scope: admission/central authorization transactions and
unrelated #2570 changes.

## 3. Risk-to-test traceability

| Risk | Impact | Tests | Mitigation |
|---|---|---|---|
| Deterministic child is adopted | Unauthorized work or data corruption | UT-RO-2582-001..004, IT-RO-2582-001 | CreateOnly plus typed collision handling |
| Stale/missed parent watch authorizes work | Premature KA/backend side effects | UT-RO-2582-005..007, IT-AI-2582-001, IT-WE-2582-001 | Fresh API-reader parent read and bounded requeue |
| RR failure leaves active work running | Continued remediation after failure | UT-AI-2582-004..005, UT-WE-2582-002..006, E2E-2582-003/004 | Child-owned cancellation and backend proof |
| Cancellation is claimed without stopping backend | False safety/audit state | UT-WE-2582-003..006, IT-WE-2582-002 | Cancelling plus Job/Pods, Tekton dependents, AWX verification |
| Notification exception is broken | Operators miss failure/completion | UT-NOT-2582-001..003, IT-NOT-2582-001, E2E-2582-001/002 | Exact identity reuse only for approved notification cases |
| IS collision or failure is hidden | Interactive request joins wrong session | UT-AF-2582-001, IT-AF-2582-001 | New start is direct CreateOnly; join/takeover stays explicit |

## 4. Wiring manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| RO collision and child mapping | `RemediationOrchestratorReconciler.SetupWithManager` | `internal/controller/remediationorchestrator/reconcile_loop.go`, `cmd/remediationorchestrator/main.go` | IT-RO-2582-001 |
| SP parent gate/cancellation | SP reconcile before enrichment | `internal/controller/signalprocessing/`, `cmd/signalprocessing/main.go` | IT-SP-2582-001 |
| AIA parent gate/cancellation | Pending/Investigating dispatch | `internal/controller/aianalysis/`, `cmd/aianalysis/main.go` | IT-AI-2582-001 |
| AgentSession/KA cancellation | AgentSession reconcile and dispatcher | `internal/kubernautagent/agentsession/`, `cmd/kubernautagent/agentsession_wiring.go` | IT-KA-2582-001 |
| WE gate and backend stop | Pending and cancellation lifecycle | `internal/controller/workflowexecution/`, `pkg/workflowexecution/executor/`, `cmd/workflowexecution/main.go` | IT-WE-2582-001/002 |
| EA parent gate/cancellation | EM reconcile before checks | `internal/controller/effectivenessmonitor/`, `cmd/effectivenessmonitor/main.go` | IT-EM-2582-001 |
| Notification exception | Notification reconcile/delivery | `internal/controller/notification/`, `cmd/notification/main.go` | IT-NOT-2582-001 |
| RAR cancellation | RO approval phase and RAR validation | `internal/controller/remediationorchestrator/awaiting_approval_handler.go`, RAR API | IT-RO-2582-002 |
| AF IS collision/close | AF create hook/session manager | `pkg/apifrontend/session/`, `pkg/apifrontend/tools/af_create_rr.go` | IT-AF-2582-001 |
| Generated RBAC/schemes | controller startup/manifests | `config/crd`, `config/rbac`, service `main.go` files | IT-SEC-2582-001 |

## 5. Unit scenarios (RED → GREEN → REFACTOR)

| ID | BR/control | Business outcome |
|---|---|---|
| UT-RO-2582-001 | BR-ORCH-025, AC-4 | Pre-existing SP/AIA/RAR/WE/EA is reported as a typed collision; no ref or phase advance occurs. |
| UT-RO-2582-002 | BR-ORCH-025, AC-4 | Concurrent `AlreadyExists` is a collision, never adoption. |
| UT-RO-2582-003 | BR-ORCH-036, AU-2 | Collision fails RR, sets manual review/failure phase, and creates an escalation notification without reopening a terminal RR. |
| UT-RO-2582-004 | SI-10 | Malformed/missing parent links fail closed. |
| UT-RO-2582-005 | BR-ORCH-025 | Legitimate create-before-parent-phase window waits and does not call side effects. |
| UT-RO-2582-006 | BR-ORCH-025 | Handoff timeout becomes observable fail-closed/cancelled state. |
| UT-RO-2582-007 | AC-4, AC-6 | Fresh parent read errors/restarts requeue without authorizing work; NotFound cancels the child. |
| UT-AI-2582-001 | BR-AI-007, AC-4 | AIA does not dispatch KA until RR is durably `Analyzing` and AIA is `Investigating`. |
| UT-AI-2582-002 | BR-AI-009, SI-10 | First AgentSession CreateOnly collision fails AIA/manual review; no adoption. |
| UT-AI-2582-003 | BR-AI-009 | Persisted AgentSession recovery identity requires matching UID, owner AIA, incident, and RR refs. |
| UT-AI-2582-004 | BR-AI-023, CC7.2 | Parent failure reaches the real session manager and AgentSession ends `Cancelled` only after acknowledgement. |
| UT-AI-2582-005 | BR-AI-010 | AIA terminal failure preserves existing terminal AgentSession history. |
| UT-WE-2582-001 | BR-WE-007, AC-4 | WE waits for persisted RR `Executing`; pre-created WE cannot dispatch while RR is `Investigating`. |
| UT-WE-2582-002 | BR-WE-014 | WE enters `Cancelling` on RR failure and does not claim `Cancelled` before proof. |
| UT-WE-2582-003 | BR-WE-014 | Job cancellation verifies Job and all owned Pods are stopped. |
| UT-WE-2582-004 | BR-WE-014 | Tekton cancellation verifies PipelineRun, TaskRuns, and Pods. |
| UT-WE-2582-005 | BR-WE-015 | AWX cancellation calls cancel, polls terminal status, and exposes failure/unavailability. |
| UT-WE-2582-006 | AC-6, CC7.2 | Permission/backend failures leave a non-success observable condition and retry/escalate. |
| UT-CHILD-2582-001 | BR-SP-*, AC-4 | SP `Cancelled` stops enrichment/classification and preserves Completed/Failed history. |
| UT-CHILD-2582-002 | BR-AI-010 | AIA `Cancelled` stops analysis and preserves terminal result history. |
| UT-CHILD-2582-003 | BR-ORCH-026 | RAR `ApprovalDecisionCancelled` cannot later become Approved or advance RR. |
| UT-CHILD-2582-004 | BR-EM-005 | EA `Cancelled` stops checks and preserves terminal outcomes. |
| UT-CHILD-2582-005 | BR-INTERACTIVE-010 | IS failure maps to RR failure by `spec.remediationRequestRef.name`; owner refs are not required. |
| UT-CHILD-2582-006 | BR-INTERACTIVE-010 | Explicit join/takeover semantics remain idempotent and separate from new-session collision. |
| UT-NOT-2582-001 | BR-ORCH-045.2 | Completion notification reuses exact same-parent/type identity. |
| UT-NOT-2582-002 | BR-ORCH-036, CC7.2 | Failure/manual-review notification remains deliverable after RR failure. |
| UT-NOT-2582-003 | SI-10 | Mismatched notification identity is a collision, not reusable. |
| UT-SEC-2582-001 | AC-6 | Generated RBAC contains only required parent reads and backend-stop verbs; SSAR assumptions are testable. |

## 6. Integration and E2E scenarios

| ID | Outcome |
|---|---|
| IT-RO-2582-001 | Production RO dispatch maps child status/failure through `spec.remediationRequestRef.name`; dropped/reconnected watches do not authorize premature work. |
| IT-RO-2582-002 | RAR cancellation and approval-vs-failure optimistic concurrency preserve failed RR history. |
| IT-SP-2582-001 | SP production reconcile reads RR directly and stops before enrichment when RR fails. |
| IT-AI-2582-001 | AIA/KA production path gates dispatch and cancels active KA work on RR failure. |
| IT-KA-2582-001 | Dispatcher invokes the real session manager/terminal hook and acknowledges cancellation. |
| IT-WE-2582-001 | Pre-created WE remains pending before `Executing`; normal create-before-phase path dispatches only after authorization. |
| IT-WE-2582-002 | Job, Tekton, and AWX cancellation paths expose verified termination or retry/manual-review failure. |
| IT-EM-2582-001 | EA stops checks after RR failure and maps IS/parent link failures correctly. |
| IT-NOT-2582-001 | Failure/manual-review notification delivers after RR enters `Failed`; completion idempotency remains intact. |
| IT-AF-2582-001 | AF creates IS before RR; a new IS collision fails the request, while explicit takeover joins safely. |
| IT-SEC-2582-001 | Generated manifests and SSAR assumptions validate least privilege. |
| E2E-2582-001 | Pre-created WE cannot dispatch, RR fails/escalates, failure notification delivers, and cancellation outcome is visible. |
| E2E-2582-002 | Normal SP→AIA/AgentSession→RAR/WE→EA flow preserves ordering and completion notification idempotency. |
| E2E-2582-003 | RR failure during active AI cancels KA work; AIA/AgentSession/IS converge and no later WE dispatch occurs. |
| E2E-2582-004 | RR failure during Job, Tekton, and AWX execution stops/observes backend or remains visibly unverified with retry/manual review. |
| E2E-2582-005 | Restart, stale/missed watch, transient parent API error, and least-privilege denial fail closed without false parent failure. |

## 7. Test approach and execution

- Unit tests use Ginkgo/Gomega and the controller-runtime fake client only for
  external Kubernetes API behavior; business logic remains real.
- Integration tests use envtest/KIND and traverse production controller entry
  points. No mocks for internal business logic.
- E2E tests use a real cluster and validate Job, Tekton, and AWX boundaries.
- Every scenario carries a BR/control mapping. No `testing.T`, `Skip`, `XIt`, or
  `PIt` is permitted.

Recommended commands:

```bash
go test ./pkg/remediationorchestrator/... ./internal/controller/remediationorchestrator/... -count=1
go test ./pkg/aianalysis/... ./internal/controller/aianalysis/... ./internal/kubernautagent/... -count=1
go test ./pkg/workflowexecution/... ./internal/controller/workflowexecution/... -count=1
go test ./pkg/apifrontend/session/... ./pkg/apifrontend/tools/... -count=1
go test ./test/integration/remediationorchestrator/... ./test/integration/workflowexecution/... -count=1
go build ./...
golangci-lint run --timeout=5m
```

## 8. Rollout, rollback, and residual races

Generated CRDs are applied before controllers that write the new enum values;
child/KA/AF controllers understand gates and cancellation before RO collision
enforcement. Rollback retains new enum values and pauses affected reconciliation
rather than reverting only RO. Watches remain asynchronous; a gate-to-dispatch
race with a privileged actor is tested as residual risk, not claimed atomic.
