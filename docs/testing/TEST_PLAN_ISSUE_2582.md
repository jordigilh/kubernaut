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
| RR failure leaves active work running | Continued remediation after failure | UT-AI-2582-004..005, UT-WE-2582-002..005, IT-AI-2582-001, IT-WE-2582-003, E2E-2582-003/004 | Child-owned cancellation and backend proof |
| Cancellation is claimed without stopping backend | False safety/audit state | UT-WE-2582-003..005, IT-WE-2582-003 | Cancelling plus Job/Pods, Tekton dependents, AWX verification |
| Notification exception is broken | Operators miss failure/completion | UT-NOT-2582-003, IT-NOT-2582-001, E2E-2582-001/002 | Exact identity reuse only for approved notification cases |
| IS collision or failure is hidden | Interactive request joins wrong session | UT-AF-2582-001, IT-AF-2582-001 | New start is direct CreateOnly; join/takeover stays explicit |

## 4. Wiring manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| RO collision and child mapping | `RemediationOrchestratorReconciler.SetupWithManager` | `internal/controller/remediationorchestrator/reconcile_loop.go`, `cmd/remediationorchestrator/main.go` | IT-RO-2582-001 |
| SP parent gate/cancellation | SP reconcile before enrichment | `internal/controller/signalprocessing/`, `cmd/signalprocessing/main.go` | IT-SP-2582-001 |
| AIA parent gate/cancellation | Pending/Investigating dispatch | `internal/controller/aianalysis/`, `cmd/aianalysis/main.go` | IT-AI-2582-001 |
| AgentSession/KA cancellation | AgentSession reconcile and dispatcher | `internal/kubernautagent/agentsession/`, `cmd/kubernautagent/agentsession_wiring.go` | IT-KA-2582-001; IT-AI-2582-001; UT-KA-2582-002 |
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
| UT-RO-2582-002 | BR-ORCH-025, AC-4 | `CreateWithResult` reports an existing deterministic WFE as a collision without a creation side effect. |
| UT-RO-2582-003 | BR-ORCH-036, AU-2 | RO does not claim active child cancellation or overwrite child-owned terminal status. |
| UT-RO-2582-004 | AC-6 | RO leaves active child status ownership to the child controllers during parent cancellation. |
| UT-RO-2582-005 | BR-ORCH-025 | Legitimate create-before-parent-phase window waits and does not call side effects. |
| UT-RO-2582-006 | BR-ORCH-025 | The bounded handoff-expiry predicate identifies the create-before-phase timeout boundary. |
| UT-RO-2582-007 | AC-4, AC-6 | Fresh parent read errors/restarts requeue without authorizing work; NotFound cancels the child. |
| UT-AI-2582-001 | BR-AI-007, AC-4 | AIA does not dispatch KA until RR is durably `Analyzing` and AIA is `Investigating`. |
| UT-AI-2582-002 | BR-AI-009, SI-10 | First AgentSession CreateOnly collision fails AIA/manual review; no adoption. |
| UT-AI-2582-004 | BR-AI-009, AC-4 | A missing AIAnalysis parent is treated as a cancellation signal and never authorizes KA work. |
| UT-AI-2582-005 | BR-AI-010 | AIA terminal failure preserves existing terminal AgentSession history. |
| UT-WE-2582-001 | BR-WE-007, AC-4 | WE waits for persisted RR `Executing`; pre-created WE cannot dispatch while RR is `Investigating`. |
| UT-WE-2582-002 | BR-WE-014 | WE enters `Cancelling` on RR failure and does not claim `Cancelled` before proof. |
| UT-WE-2582-003 | BR-WE-014 | Job cancellation verifies Job and all owned Pods are stopped. |
| UT-WE-2582-004 | BR-WE-014 | Tekton cancellation verifies PipelineRun, TaskRuns, and Pods. |
| UT-WE-2582-005 | BR-WE-015 | AWX cancellation calls cancel, polls terminal status, and exposes failure/unavailability. |
| UT-WE-2582-006 | AC-4, SI-10 | A malformed parent reference enters verified cancellation instead of bypassing authorization. |
| UT-WE-2582-007 | BR-WE-015, AC-4 | AWX recovers a missing execution reference from an exact WFE identity before cancellation. |
| UT-WE-2582-008 | BR-WE-015, SI-10 | Ambiguous AWX identity matches fail closed rather than adopting one job. |
| UT-WE-2582-009 | BR-WE-015, AC-4 | AWX recovery follows pagination and ignores unrelated malformed extra-vars records. |
| UT-WE-2582-010 | SC-8, SI-10 | AWX recovery rejects cross-origin or scheme-changing pagination links before sending credentials. |
| UT-WE-2582-011 | SC-8, SI-10 | AWX HTTP redirects are origin-locked before bearer-token forwarding. |
| UT-WE-2582-012 | BR-AUDIT-005, CC7.2 | Verified cancellation emits `workflowexecution.workflow.cancelled`, not `workflow.failed`. |
| UT-NOT-2582-003 | SI-10 | Mismatched notification identity is a collision, not reusable. |

## 6. Integration and E2E scenarios

| ID | Outcome |
|---|---|
| IT-RO-2582-001 | Production RO leaves active child status ownership to child controllers after RR cancellation; no false child failure is written. |
| IT-RO-2582-002 | RAR `Cancelled` is terminal and cannot be reopened as `Approved` by a later status update. |
| IT-SP-2582-001 | SP production reconcile reads RR directly and stops before enrichment when RR fails. |
| IT-AI-2582-001 | AIA/KA production path gates dispatch and cancels active KA work on RR failure. |
| IT-KA-2582-001 | Dispatcher invokes the real session manager/terminal hook and acknowledges cancellation. |
| IT-WE-2582-001 | Pre-created WE remains pending before `Executing`; normal create-before-phase path dispatches only after authorization. |
| IT-WE-2582-002 | Production WFE reconciliation enters child-owned cancellation before claiming backend termination. |
| IT-WE-2582-003 | Job-backed WFE observes backend Job termination before every observed `Cancelled` state. |
| IT-EM-2582-001 | EA stops checks after RR failure and maps IS/parent link failures correctly. |
| IT-NOT-2582-001 | Failure/manual-review notification delivers after RR enters `Failed`; completion idempotency remains intact. |
| IT-AF-2582-001 | Production AF session service writes the InvestigationSession CRD used by the handoff path; collision/takeover semantics are covered by the mapped unit scenarios. |
| IT-SEC-2582-001 | Generated manager RBAC contains the required lifecycle parent reads, child status writes, RAR creation, and InvestigationSession reads. |
| E2E-2582-001 | Successful SP→AIA→WE completion produces an idempotent completion notification. |
| E2E-2582-002 | Normal remediation child ordering and owner references remain intact through the lifecycle. |
| E2E-2582-003 | RR cancellation during active AI leaves child cancellation ownership with AIA and prevents later WFE dispatch. |
| E2E-2582-004 | WE failure produces explicit failure-phase/reason/consecutive-failure attribution in the real Kind journey. |
| E2E-2582-005 | A failed parent remains fail-closed across an RO controller restart and creates no children afterward. |

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

## 9. Scoped evidence register

The following is the requirements-based acceptance matrix for the #2582 change;
raw package coverage is diagnostic and is not used as a substitute for these
business scenarios.

For the **parent-gate and pure cancellation-policy unit subset** (excluding
cross-component UID/lease/backend journeys, which are measured in IT/E2E rows),
the measured scenario denominator is **17** and the passing numerator is **17**:
CreateOnly collision refusal (`UT-RO-2582-001`
and `UT-RO-2582-002`), parent authorization/wait/terminal/read-error handling
(`UT-RO-2582-005..007`), terminal AgentSession-history preservation
(`UT-AI-2582-005`), Job/Tekton/AWX cancellation verification
(`UT-WE-2582-003..012`), and terminal RAR decision immutability
(`UT-AW-2582-001`). All 17 are critical policy scenarios; no pending/skipped
examples are included in the numerator. UID replacement, acknowledgement
ordering, and real backend termination are intentionally not counted in this
unit denominator because their proving evidence requires the production-path
integration tests listed below.

| Critical scenario | Proving evidence | Local status |
|---|---|---|
| Authorized parent phase is required | `UT-RO-2582-005`, `UT-AI-2582-001` | Pass |
| Create-before-phase handoff waits and expires fail-closed | `UT-RO-2582-005..006`, `UT-AI-2582-001`, `IT-WE-2582-001` | Pass/CI confirmation pending |
| Missing, failed, cancelled, timed-out, or unreadable parent never authorizes work | `UT-RO-2582-007`, `UT-AI-2582-004`, `IT-AI-2582-001`, `IT-SP-2582-001`, `IT-EM-2582-001` | Pass/CI confirmation pending |
| AgentSession owner UID replacement is rejected | `IT-KA-2582-001` | Pass locally and in focused envtest |
| Active KA cancellation retains lease until acknowledgement | `UT-KA-2582-002`, `IT-AI-2582-001` | Pass/CI confirmation pending |
| Terminal AgentSession history is preserved | `UT-AI-2582-005` | Pass |
| Verified cancellation has distinct audit attribution | `UT-WE-2582-012` | Pass |
| RAR terminal cancellation cannot be reopened | `UT-AW-2582-001`, `IT-RO-2582-002` | Pass/CI confirmation pending |
| RO does not claim child/backend cancellation | `IT-RO-2582-001`, `IT-RO-1421-003`, `IT-WE-2582-002..003` | Pass/CI confirmation pending |
| Generated lifecycle RBAC entries are present | `IT-SEC-2582-001`, `make manifests` | Pass |
| Real-Kind normal, cancellation, failure, and restart journeys | `E2E-2582-001..005` | CI confirmation pending |

The local AIA/EM/SP/WE integration matrix is environment-blocked when Podman
cannot allocate image storage; those scenarios are not counted as locally
passed and remain merge-blocking until the current-head CI jobs pass.

### Changed-production-line diagnostic

As a structural diagnostic only, a fresh `go test -coverprofile` run over the
changed internal/pkg production packages maps **363/1,722 added production lines
(21.1%)** to unit execution. Generated/API files, command wiring, test helpers,
and test files are excluded. This is not the requirements-based acceptance
metric: controller reconciliation branches are intentionally proven by the
IT/E2E rows above. The 16/16 unit result therefore must not be presented as
full-diff line coverage, and the 21.1% diagnostic must not be presented as a
business-scenario failure.
