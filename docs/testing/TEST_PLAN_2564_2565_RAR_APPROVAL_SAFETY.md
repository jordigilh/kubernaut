# Test Plan: #2564/#2565 RAR approval-safety baseline

**Test Plan Identifier**: TP-2564-2565-v1
**Feature**: Fail-closed RemediationApprovalRequest lifecycle, conditional decisions, expiry safety, and bounded parent evidence
**Version**: 1.0
**Created**: 2026-11-02
**Author**: Kubernaut development agent
**Status**: Active — RED complete; focused gates pass; real-API focused scenarios pass
**Branch**: `triage/issue-2564-2565`
**Base revision**: `16bb28c6ecf9dacb10d356d6125a18f1b076ecec` (`origin/main`)

> The approved implementation plan is saved in the OpenChamber project plan for
> issues #2564/#2565. Runtime implementation and RED tests begin only after this
> test plan is available. Preserve the pre-existing uncommitted
> `docs/architecture/decisions/DD-AF-017-standalone-rar-parameter-override-contract.md`.

## 1. Purpose and objectives

This plan proves that a RemediationApprovalRequest (RAR) cannot be reused,
approved while terminating, expired over a winning decision, or mutated after a
terminal decision. It also proves that RAR deletion cannot silently orphan or
continue an approval workflow, that approval evidence is bounded and
non-authoritative, and that audit bookkeeping means asynchronous enqueue
acceptance rather than durable persistence.

### Objectives

1. Track all 20 requirements-based scenarios, including all 18 critical scenarios;
   this baseline records open residual scenarios instead of claiming unrun coverage.
2. Prove every production wiring row W-01 through W-09 through an integration path;
   W-10 is an optional normal-delivery observation and is not a release gate.
3. Prove resourceVersion first-writer-wins behavior without adding reviewer UID,
   content digest, opaque review token, or selection revision binding.
4. Prove no parent failure occurs unless expiry is authoritatively confirmed.
5. Prove audit events are accepted by the shared asynchronous buffer without
   asserting flush or durability.

### Success metrics

| Metric | Target | Measurement |
|---|---:|---|
| Requirements-based scenario coverage | 16/20 currently passed; 4 explicitly open/conditional | This plan's scenario matrix |
| Critical scenario coverage | 15/18 currently passed; 3 open critical scenarios | Critical rows in the scenario matrix |
| Required wiring coverage | 3/9 fully evidenced; 6 rows remain open or unverified | W-01 through W-09 with production-path IT evidence |
| Optional normal-delivery observation | 0/1 or 1/1 | W-10, only where an existing harness supports it |
| Affected build | Pass | `go build ./...` |
| Affected lint | Pass with no new findings | `golangci-lint run --timeout=5m` |
| Generated artifacts | In sync | `make manifests`, `make validate-embed`, `make generate`, scoped `make gen-diff` |

## 2. Authority and scope decisions

### Governing documents

- `BR-ORCH-025` and `BR-ORCH-026`: `docs/requirements/BR-ORCH-025-026-workflow-approval-orchestration.md`
- `BR-AUDIT-005`: `docs/requirements/11_SECURITY_ACCESS_CONTROL.md`
- `ADR-040`: RemediationApprovalRequest CRD architecture and immutable spec
- `ADR-076`: RAR terminal decision contract at the approved base
- `DD-AF-017`: standalone RAR parameter/approval contract; its pre-existing uncommitted diff must remain unchanged
- `DD-WEBHOOK-001`: CRD webhook requirements and AdmissionReview identity boundary
- `DD-WEBHOOK-003`: webhook-complete audit pattern
- `ADR-038` / `DD-AUDIT-002`: shared asynchronous in-memory audit buffer
- Issue #2588: deferred RR cancellation propagation and cooperative child shutdown

### Approved boundaries

- Human users may approve or reject only. `ApprovalDecisionCancelled` and
  `ApprovalDecisionExpired` are system-only outcomes.
- Kubernetes `resourceVersion` is the concurrency guard. Reviewer UID, opaque
  review token, content digest, selection revision, and full replacement-race
  prevention are explicitly deferred residual risks.
- RR cancellation fan-out, child shutdown, cancellation notification delivery,
  and cancellation parent finalizers belong to #2588, not this baseline.
- `AuditRecorded` proves acceptance/enqueue into the shared buffer only. No test
  calls a synchronous flush or claims durable persistence.

### In scope

- RAR creator finalizer placement, deterministic collision classification, and
  typed post-create recovery in `pkg/remediationorchestrator/creator/approval.go`.
- RO collision transition, RAR deletion/finalizer handling, parent-missing
  lookup, expiry revalidation, and restart convergence in
  `internal/controller/remediationorchestrator/`.
- AuthWebhook authentication order, system-only outcomes, terminal tuple
  immutability, deadline validation, and attribution in `pkg/authwebhook/`.
- AF conditional approval write and bounded parent projection in
  `pkg/apifrontend/tools/crd_tools.go` and production MCP registration.
- Generated/manual RO RBAC and the inline RO E2E manifest.
- Shared asynchronous audit enqueue bookkeeping and correlation.

### Out of scope

- Reviewer UID/content/digest/revision binding or universal delete/recreate
  protection; these remain explicit residual risks under the approved contract.
- RR cancellation propagation, cooperative child shutdown, cancellation
  notification delivery, and cancellation finalizer orchestration; owned by #2588.
- #2570 parameter-only override expansion, workflow editing, or selection changes.
- Valkey-backed audit buffering, synchronous shutdown flush, audit durability,
  crash recovery, or broad RBAC/tenant/namespace redesign.
- Browser logout/session-generation proof owned by console work.

## 3. Risks and test design

| ID | Risk | Priority | Mitigating scenarios |
|---|---|---:|---|
| R-01 | Any deterministic RAR collision fails closed; ambiguous writes never replay the create | P0 | UT-RO-2564-001, IT-RO-2564-019 |
| R-02 | RAR deletion releases finalizer before safe parent handling or without exact owner/reference identity | P0 | IT-RO-2564-002, 003, 013, 017, 018 |
| R-03 | Competing approval writers both mutate terminal fields | P0 | IT-AF-2564-006, 010, IT-AW-2564-015 |
| R-04 | Expiry wins over a timely decision because of stale reads | P0 | IT-RO-2564-007, UT-AW-2564-011, UT-RO-2564-012 |
| R-05 | A system-only decision is spoofed by a human or AF | P0 | IT-AW-2564-005, 015, 016 |
| R-06 | Parent evidence leaks unbounded/raw data or gates a valid decision | P1 | IT-AF-2565-008 |
| R-07 | Audit condition claims durability that was not established | P0 | IT-RO-2564-009, 020 |
| R-08 | Production RBAC differs from E2E fixture or generated output | P0 | IT-RO-2564-004 |

### TDD phases

1. **RED**: Add failing Ginkgo/Gomega UT/IT scenarios from the matrix and record
   the failing output before production implementation.
2. **GREEN**: Wire production entry points and implement the smallest behavior
   that passes UT and IT. CHECKPOINT W must pass for W-01 through W-09.
3. **REFACTOR**: Improve shared validation, projection mapping, error handling,
   and observability without introducing new components. Re-run scoped gates.

### Pass/fail criteria

The full plan pass requires 20/20 scenarios, 18/18 critical scenarios, W-01
through W-09 with production-path evidence, no decision replay after conflict or
ambiguous write, no parent failure without `ExpiredConfirmed`, no unsupported
durability claim, and passing build/lint/generated-artifact/affected Make gates.
This PR does not claim that full certification: its current behavior evidence is
16/20 passed rows, while only 3/9 required wiring rows are fully evidenced.
IT-RO-2564-013, IT-RO-2564-014, and IT-RO-2564-018 are explicitly open residual
scenarios; W-05, W-06, and W-08 remain unverified for production wiring; and
IT-RO-2564-020 is conditional. Any executed scenario failure, affected
regression, generated RBAC mismatch, or unsupported audit durability claim fails
the baseline.

## 4. Wiring manifest

| ID | Component | Production entry point | Wiring location | IT scenario |
|---|---|---|---|---|
| W-01 | RAR creator and typed creation outcomes | `ApprovalCreator.Create` from analyzing handler | `pkg/remediationorchestrator/creator/approval.go`, `internal/controller/remediationorchestrator/analyzing_handler.go` | IT-RO-2564-019 |
| W-02 | RAR deletion/finalizer handling | RO reconcile loop and callbacks | `internal/controller/remediationorchestrator/reconcile_loop.go`, `reconciler_callbacks.go` | IT-RO-2564-002, 017, 018 |
| W-03 | Missing-parent safe lookup | RAR deletion callback | `internal/controller/remediationorchestrator/reconciler_callbacks.go` | IT-RO-2564-003, 014 |
| W-04 | RO RAR RBAC | controller markers and generated role | RO controller setup, `config/rbac/role.yaml`, inline E2E manifest | IT-RO-2564-004 |
| W-05 | AuthWebhook RAR admission | `cmd/authwebhook/main.go` to RAR handler | `pkg/authwebhook/remediationapprovalrequest_handler.go` | IT-AW-2564-005, 015, 016 |
| W-06 | AF conditional decision | `registerCRDTools` to approval handler | `pkg/apifrontend/tools/crd_tools.go` | IT-AF-2564-006, 010 |
| W-07 | Deadline and atomic expiry | creator, webhook, awaiting handler, callbacks | `pkg/remediationorchestrator/creator`, `pkg/authwebhook`, `internal/controller/remediationorchestrator` | IT-RO-2564-007, 013; UT-RO-2564-012 |
| W-08 | Bounded parent projection | `kubernaut_get_approval_request` | `pkg/apifrontend/tools/crd_tools.go` | IT-AF-2565-008 |
| W-09 | Decision audit enqueue | RAR reconciler audit callback | `internal/controller/remediationorchestrator` and shared audit store | IT-RO-2564-009 |
| W-10 | Optional normal audit delivery observation | existing downstream audit harness | existing integration/E2E harness only | IT-RO-2564-020 |

Required RAR RBAC rules use `kubernaut.ai`:

- `remediationapprovalrequests`: `get/list/watch/create/update/patch`
- `remediationapprovalrequests/status`: `get/update/patch`
- `remediationapprovalrequests/finalizers`: `update`

No `delete` verb is added unless a production client call proves it is required.

## 5. BR and control coverage

| BR/control | Business outcome | Scenarios |
|---|---|---|
| BR-ORCH-026 | RAR creation, approval, rejection, expiry, and terminal immutability | 001, 002, 005–007, 010–019 |
| BR-ORCH-025 | Bounded RAR/RR review projection | 008 |
| BR-AUDIT-005 | Correlated attempted/accepted approval audit | 009, 020 |
| AC-3 / AC-6 | Authenticated human/intermediary/system attribution and least privilege | 004–006, 010, 015, 016, 019 |
| AU-2 / AU-3 | Structured decision, denial, expiry, collision, and deletion audit content | 005, 007, 009, 019, 020 |
| SI-10 | Strict decision inputs, timestamps, and bounded projection output | 006–008, 011, 012 |
| SOC 2 CC7.2 | Correlation/reconstruction under normal audit delivery | 002, 003, 007, 009, 013, 014, 017–020 |
| SOC 2 CC8.1 | Change-management evidence for this tested change | All plan and gate evidence |

## 6. Requirements-based scenario matrix

Initial coverage was **0/20 overall** and **0/18 critical** before RED. Current
execution evidence is recorded in §8; the full scenario target remains open for
the explicitly listed residual and conditional rows.

| ID | Tier | Wiring | Critical | Outcome to prove | Status |
|---|---|---|---:|---|---|
| UT-RO-2564-001 | UT | W-01 | Yes | Finalizer exists before create; any `AlreadyExists` collision fails closed without reusing the existing RAR | Pass — focused package test |
| IT-RO-2564-002 | IT | W-02 | Yes | RAR deletion with awaiting/terminal parent follows the exact parent matrix before finalizer release | Pass — focused real-API integration (2 table entries) |
| IT-RO-2564-003 | IT | W-03 | Yes | RR `NotFound` performs the direct RAR lookup before returning and releases only after confirmed cleanup | Pass — focused real-API integration |
| IT-RO-2564-004 | IT | W-04 | Yes | Generated/deployed and inline E2E RBAC contain only required `kubernaut.ai` RAR rules | Pass — parity test |
| IT-AW-2564-005 | IT | W-05 | Yes | Terminating decision is rejected after authentication and denial audit is attributed/enqueued | Behavior pass — focused package test; production wiring unverified |
| IT-AF-2564-006 | IT | W-06 | Yes | Conditional write and full terminal tuple are immutable; exact same tuple is a no-op | Behavior pass — focused package test; production wiring unverified |
| IT-RO-2564-007 | IT | W-07 | Yes | Strict deadline and atomic expiry yield only `ExpiredConfirmed`, `DecisionWon`, or `Unavailable` | Pass — focused real-API integration |
| IT-AF-2565-008 | IT | W-08 | No | Projection honors owner validation, exact schema, false flag, nulls, and bounds | Behavior pass — focused package test; production wiring unverified |
| IT-RO-2564-009 | IT | W-09 | Yes | Decision/denial/expiry/deletion audit payloads correlate and enqueue successfully | Pass — focused real-API integration |
| IT-AF-2564-010 | IT | W-06 | Yes | Opposite writers have one winner; conflict/ambiguous writes never replay | Pass — focused package test |
| UT-AW-2564-011 | UT | W-07 | Yes | Before/equal/after deadline decisions use the injected clock | Pass — focused package test |
| UT-RO-2564-012 | UT | W-07 | Yes | Approved/Rejected/Expired/system-only Cancelled and invalid timestamp states converge safely | Pass — focused package test |
| IT-RO-2564-013 | IT | W-07 | Yes | Restart/watch replay and finalizer conflicts are idempotent and observable | Open residual — no focused integration spec |
| IT-RO-2564-014 | IT | W-03 | Yes | Missing RAR after persisted decision does not replay or mutate parent incorrectly | Open residual — no focused integration spec |
| IT-AW-2564-015 | IT | W-05 | Yes | Authenticates before terminal response and rejects every terminal-field mutation | Pass — focused package test |
| IT-AW-2564-016 | IT | W-05 | Yes | Client Cancelled/Expired is rejected; trusted RO system outcome is read-only/terminal | Pass — focused package test |
| IT-RO-2564-017 | IT | W-02 | Yes | Transient deletion-parent/API failure retains finalizer and retries observably | Pass — focused real-API integration |
| IT-RO-2564-018 | IT | W-02 | Yes | RR deletion with an existing RAR is distinct and does not orphan pending decision handling | Open residual — no focused integration spec |
| IT-RO-2564-019 | IT | W-01 | Yes | `Created`, any deterministic `AlreadyExists` `Collision`, `StatusPersistenceFailed`, and `PersistenceUnknown` have exact outcomes; unknown performs one read/no replay and manual review/audit | Pass — focused real-API integration plus creator unit evidence |
| IT-RO-2564-020 | IT | W-10 | No | Existing harness observes normal downstream audit delivery with bounded polling, if available | Conditional |

## 7. Test environment and data

- Use unique namespaces and deterministic RAR/RR fixtures per test.
- Use envtest/real API-server integration for resourceVersion conflicts, status
  subresources, owner references, finalizers, CRD validation, and AdmissionReview wiring.
- Mock only external DataStorage/audit delivery and unrelated external APIs.
- Use injected clocks for before/equal/after deadline and expiry restart cases.
- Use `httptest.NewServer` or existing repository test infrastructure for network
  boundaries; do not share fixed ports across tests.
- Use the real `kubernaut.ai` CRD group and rendered RBAC in integration/E2E.

## 8. Execution and evidence

Required commands are `make manifests`, `make validate-embed`, `make generate`,
scoped `make gen-diff`, `git diff --check`, `go build ./...`,
`golangci-lint run --timeout=5m`, and the affected integration Make targets for
remediationorchestrator, authwebhook, and apifrontend. Record exact output; do
not claim unrun suites passed.

`gen-diff` must be scoped to generated artifacts before it is used as a gate;
the pre-existing DD-AF-017 diff must be recorded and preserved, not reset or
excluded by deleting the file. Container-backed E2E uses the repository Make
target after checking Podman disk capacity.

Record for each scenario: revision, command, permissions/RBAC evidence, object
states, resourceVersions where relevant, audit enqueue result, optional normal
delivery result, logs/events, and generated-artifact diff. Keep enqueue evidence
separate from downstream delivery and durability claims.

### Current execution evidence (2026-11-02)

Passed:

- `go test ./internal/controller/remediationorchestrator -count=1`
- `go test ./internal/controller/remediationorchestrator -run '^TestController$' -ginkgo.focus='RAR approval safety terminal transition races' -count=1`
- `go test ./pkg/authwebhook -count=1`
- `go test ./pkg/remediationorchestrator -count=1`
- `go test ./pkg/remediationorchestrator/creator -count=1`
- `go test ./pkg/apifrontend/tools -count=1`
- `go test ./test/infrastructure -run TestInfrastructure -ginkgo.focus='RO RAR RBAC parity' -count=1`
- `go test ./test/infrastructure -run TestInfrastructure -ginkgo.focus='signs AuthWebhook with both service DNS identities' -count=1`
- `go build ./...`
- `golangci-lint run --timeout=5m`
- `make manifests`
- `make validate-embed`
- `make generate`
- `gofmt` on changed Go files and `git diff --check`

The terminal-transition regression proves a resourceVersion conflict cannot
overwrite a concurrent `Completed` outcome or emit a losing escalation
notification. The deletion table also exercises exact RAR parent identity
(name, reference UID, and controller-owner UID); these are additional safety
regressions and do not close the intentionally open restart/deletion-wiring
rows above.

Compile-only evidence passed for the new real-API suite:

- `go test ./test/integration/remediationorchestrator -run '^$' -count=1`

Repository-wide unit gate:

- `make test` — all suites completed, but the target exited 2 because the
  unrelated data-storage performance spec
  `pkg/datastorage/metrics_test.go:98` failed its first flaky attempt before
  passing on retry. No RAR-related unit failure was observed.

Focused real-API integration gate:

- An earlier run of `make test-integration-remediationorchestrator
  GINKGO_FOCUS='RAR approval safety' TEST_PROCS=1` passed **7/7 focused
  specs** at the prior local head.
- The latest post-fix rerun did not reach any specs: Podman image creation
  failed with `no space left on device` while the Podman VM reported 366
  images and 63.69 GB reclaimable. The suite is therefore environment-blocked
  for the current unpushed head; no image pruning was performed.
- The focused `pkg/authwebhook` and `pkg/apifrontend/tools` tests establish
  behavior only; they do not exercise the production entry points. W-05, W-06,
  and W-08 therefore remain unverified for wiring until production-path
  integration scenarios are added and pass.

## 9. Residual risks and handoff

Privileged finalizer removal, force deletion, arbitrary same-name replacement
after uncertain writes, reviewer/content binding, audit-buffer crash/drop
durability, broad authorization redesign, and RR cancellation propagation remain
explicit residuals or separate work. The bounded parent projection contract is
handed to console #163. Parameter/override expansion remains with #2570.
