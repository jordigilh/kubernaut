# Test Plan: #2080 recurrence — Durable session-lost regeneration backoff

## 1. Purpose

[#2080](https://github.com/jordigilh/kubernaut/issues/2080) (originally fixed
alongside #2079) added exponential backoff to `handleSessionLost`'s KA
session regeneration path so a transient session-hand-off race wouldn't burn
through the 5-regeneration cap before the legitimate multi-hop hand-off had
time to settle. That fix computed a `RequeueAfter` delay from
`errorClassifier.GetRetryDelay(session.Generation)` and returned it from the
handler.

CI runs [31454349115](https://github.com/jordigilh/kubernaut/actions/runs/31454349115)
(2026-08-11, validating the original fix's own backport PR #2087) and
[31695817033](https://github.com/jordigilh/kubernaut/actions/runs/31695817033)
(2026-08-13, an unrelated #2117 merge) both show `E2E-FP-1189-005` failing
with `Session regeneration cap exceeded (5 regenerations)`, with all 5
generations completing within ~1-2 wall-clock seconds instead of the intended
~1s/2s/4s/8s/16s (~31s) backoff sequence. `RequeueAfter` alone is not a
durable deadline: this controller's own self-watch predicate
(`aiAnalysisUpdatePredicate`) wakes the reconciler immediately whenever
`Status.KASession.ID` changes -- which `handleSessionLost`'s own regeneration
write does on every single attempt (clears `ID` on loss, sets a fresh UUID on
resubmit) -- bypassing the computed `RequeueAfter` entirely and re-running
`reconcileInvestigating` far sooner than the backoff intended.

### Root Cause Detail

`RequeueAfter` is a *hint* to the controller-runtime work queue, not an
exclusion lock. Any other event that satisfies a watch predicate and
re-enqueues the same object supersedes it -- the queue simply processes
whichever enqueue happens first. Because this controller's own status write
(clearing/resetting `KASession.ID`) triggers its own update predicate, the
very act of attempting the backoff produces a second, near-immediate
enqueue that races the intended delay.

### Chosen Fix

Add a durable `KASession.BackoffUntil *metav1.Time` field, set by
`handleSessionLost` to `now + backoffDuration` at the same time it computes
`RequeueAfter`. `runInvestigatingHandler` (the single production entry point
into `InvestigatingHandler.Handle`) checks this deadline *before* invoking
the handler, regardless of what woke the reconciler: if still in the future,
it skips the handler entirely and re-returns the remaining duration as
`RequeueAfter`, without mutating status (so the resulting no-op status write
cannot itself re-trigger the self-watch predicate). This makes the backoff
survive any early wake-up -- self-watch, a stray requeue, or a controller
restart -- instead of only holding when nothing else disturbs the queue.

## 2. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-11** | Error Handling | A session-lifecycle race must not surface as a permanent investigation failure when the underlying investigation already succeeded -- the same continuity guarantee #2080's original fix established, now made durable against any wake-up source, not just the specific timing it was tested against. |
| **SI-4 / AU-2 / AU-3** | System Monitoring / Auditable Events / Content of Audit Records | `RecordAIAgentSessionLost` must fire at most once per genuine regeneration, not once per spurious early wake-up -- an inflated `SessionLost` audit trail would misrepresent how many times the KA session was actually lost. |

## 3. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AA-2080-006 | Unit | 12 immediate, back-to-back `reconcileInvestigating` calls with no elapsed time between them (modeling the worst-case self-watch bypass) must not exhaust the 5-regeneration cap | SI-11 | `internal/controller/aianalysis/session_lost_backoff_bypass_test.go` |
| UT-AA-2080-007 | Unit | `reconcileInvestigating` skips `InvestigatingHandler.Handle` entirely (no new session submitted, no generation increment) while `BackoffUntil` is still in the future, and requeues for the remaining duration | SI-11 | same file |
| UT-AA-2080-008 | Unit | `reconcileInvestigating` resumes normal processing (resubmits) once `BackoffUntil` has elapsed | SI-11 | same file |

### Tier Coverage Rationale

- **UT** exercises the actual production entry point (`reconcileInvestigating`
  -> `runInvestigatingHandler`) directly against a fake client, mirroring the
  existing white-box pattern in `schema_rejection_retry_test.go` for this
  same controller -- no new wiring point is introduced, so no new Wiring
  Manifest row applies (per `.cursor/rules/10-wiring-verification.mdc`, this
  hardens an existing, already-wired path rather than introducing a new
  component).
- **E2E**: `E2E-FP-1189-005` (`test/e2e/fullpipeline`) already exercises the
  full session-lost regeneration journey through the real controller and
  KA client; it is the test that caught this recurrence in CI and is
  expected to pass once this fix lands, with generations spaced according to
  the intended backoff instead of completing in ~1-2 seconds.
- **IT**: not added net-new. The regeneration path's wiring into the
  `AIAnalysisReconciler`'s watch/reconcile loop already exists and predates
  this fix; this change only makes an existing backoff computation durable
  against early wake-ups.

## 4. Validation Results

- `go test ./internal/controller/aianalysis/...` -- pass (including the 3 new
  specs above; UT-AA-2080-006/007 confirmed RED against the pre-fix code
  before the fix was applied).
- `go test ./pkg/aianalysis/...` -- pass, including the pre-existing
  `UT-AA-2080-001..005` handler-level specs (`investigating_session_lost_adoption_test.go`).
- `go build ./...` -- clean.
- `golangci-lint run internal/controller/aianalysis/... pkg/aianalysis/handlers/... api/aianalysis/...` -- 0 issues.
- `gofmt -l` -- clean.
