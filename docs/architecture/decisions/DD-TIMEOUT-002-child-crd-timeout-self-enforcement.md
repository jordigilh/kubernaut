# DD-TIMEOUT-002: Child-CRD Timeout Self-Enforcement via Propagated Absolute Deadline

**Status**: ✅ Approved & Implemented
**Version**: 1.0
**Date**: 2026-08
**Confidence**: 92%
**Extends**: [DD-TIMEOUT-001: Global Remediation Timeout Strategy](DD-TIMEOUT-001-global-remediation-timeout.md)
**Implements**: [Issue #2176](https://github.com/jordigilh/kubernaut/issues/2176)
**Follow-up**: [Issue #2181](https://github.com/jordigilh/kubernaut/issues/2181) (EffectivenessAssessment / NotificationRequest)

---

## Context

`RemediationRequest.Status.TimeoutConfig` (`Global`/`Processing`/`Analyzing`/`Executing`) is
RemediationOrchestrator's (RO's) single authoritative, operator-overridable (`AC-028-5`) timeout
source. It is computed once on first reconcile (`populateTimeoutDefaults`,
`internal/controller/remediationorchestrator/timeout_management.go`), validated, and enforced by
RO itself (`checkPhaseTimeouts` -> `handlePhaseTimeout`) as an external, RR-level backstop.

Before this decision, that authoritative value was never propagated into any child CRD's own
`Spec`, and no subcontroller (SignalProcessing, AIAnalysis, WorkflowExecution) read or
self-enforced a timeout from its own Spec:

- `AIAnalysisSpec.TimeoutConfig *AIAnalysisTimeoutConfig` existed with a doc comment claiming it
  was "passed through from RR.Status.TimeoutConfig... by RO," but RO's creator
  (`pkg/remediationorchestrator/creator/aianalysis.go`) never set it, and `pkg/aianalysis` never
  read it.
- `SignalProcessingSpec` and `WorkflowExecutionSpec` had the same dead-schema pattern
  (`WorkflowExecutionSpec.ExecutionConfig.Timeout`): defined, never populated, never read.
- AIAnalysis's `InvestigatingHandler.maxInvestigationDuration` (default 25m, hardcoded) was a
  real, working self-enforced timeout — but disconnected from RO's own `Analyzing` phase timeout
  (default 10m) for the exact same phase. An operator overriding RO's `Analyzing` timeout via
  `AC-028-5` got no corresponding behavior change from AIAnalysis.

## Problem

**Question**: How do we make RO's `Status.TimeoutConfig` the single source of truth end-to-end,
with each child CRD self-enforcing its own authoritative deadline rather than relying solely on
RO's outer backstop?

**Requirements**:
1. Each of SignalProcessing, AIAnalysis, and WorkflowExecution must receive RO's authoritative
   per-phase timeout value at CRD-creation time.
2. Each subcontroller must self-enforce that value in its own reconcile loop — a first-hand "I
   know I'm over budget" self-termination, rather than waiting to be caught only by RO's outer
   `checkPhaseTimeouts`.
3. RO's existing outer backstop must remain unchanged — it stays the defense-in-depth layer for
   the case where a subcontroller is truly hung and can't even run its own self-check.
4. The propagated value must be uniformly referencable across all three CRDs (same field name,
   same type), not three independently-shaped per-CRD structs.

## Decision

**Propagate RO's authoritative per-phase timeout as a single, uniform, absolute-deadline field —
`Spec.TimesOutAt *metav1.Time` — on each child CRD's Spec, computed at CRD-creation time and
self-enforced by that CRD's own subcontroller.**

```go
// Identical shape on AIAnalysisSpec, SignalProcessingSpec, and WorkflowExecutionSpec.
// TimesOutAt is the absolute deadline propagated verbatim from RO's
// Status.TimeoutConfig at CRD-creation time (DD-TIMEOUT-002). Nil when RO
// has no authoritative timeout for that phase, in which case the child
// relies on its own configured default and/or RO's outer backstop.
TimesOutAt *metav1.Time `json:"timesOutAt,omitempty"`
```

| Child CRD | Sourced from | Self-enforced by |
|-----------|-------------|-------------------|
| SignalProcessing | `Status.TimeoutConfig.Processing` | `internal/controller/signalprocessing/signalprocessing_timeout.go` (`hasTimedOut`/`failOnTimeout`, checked once per `Reconcile` ahead of phase dispatch) |
| AIAnalysis | `Status.TimeoutConfig.Analyzing` | `pkg/aianalysis/handlers/investigating.go` (`checkInvestigationTimeout`, takes precedence over the hardcoded `DefaultMaxInvestigationDuration` fallback) |
| WorkflowExecution | `Status.TimeoutConfig.Executing` | `pkg/workflowexecution/executor/{job,ansible}.go` (`ActiveDeadlineSeconds` / TokenRequest TTL derived from `time.Until(Spec.TimesOutAt)` via the shared `remainingUntilDeadline` helper) |

RO's creators compute the deadline at the moment each child CRD is built, via a shared helper
(`pkg/remediationorchestrator/creator/timeout.go`):

```go
func computeTimesOutAt(d *metav1.Duration) *metav1.Time {
	if d == nil || d.Duration <= 0 {
		return nil
	}
	deadline := metav1.NewTime(metav1.Now().Add(d.Duration))
	return &deadline
}
```

Because `populateTimeoutDefaults` guarantees `Status.TimeoutConfig` is non-nil before any child
CRD is ever created, every child Spec gets a definite, always-populated deadline in normal
operation — the nil case exists only for defensive/back-compat handling (e.g. CRDs created before
this field existed).

## Alternatives Considered

### Alternative A: Relative duration (`Spec.Timeout *metav1.Duration`)

Propagate the phase duration itself (e.g. `5m`) rather than an absolute timestamp.

- **Rejected**: introduces clock-skew / anchor ambiguity. RO computes the duration once, at
  CRD-creation time, in its own reconcile loop. The child controller's self-check runs later, in
  its *own* reconcile loop(s) — a relative duration would need to be re-anchored against some
  other timestamp (`CreationTimestamp`? `Status.StartTime`?), and those two clocks are not
  guaranteed to agree on what "elapsed" means, especially across a child CRD that has its own
  multi-phase lifecycle (e.g. SignalProcessing's Enriching -> Classifying -> Categorizing). An
  absolute deadline sidesteps the question entirely: the child just checks `metav1.Now().After(*TimesOutAt)`.

### Alternative B: Nested per-CRD timeout structs (`AIAnalysisTimeoutConfig`, `ExecutionConfig{Timeout}`, ...)

Keep (or newly add) a per-CRD-shaped struct, as AIAnalysis's pre-existing (but dead) schema did.

- **Rejected**: not uniformly referencable. Every consumer (RO's creators, each subcontroller,
  future tooling/dashboards) would need CRD-specific field paths
  (`Spec.TimeoutConfig.InvestigatingTimeout` vs. `Spec.ExecutionConfig.Timeout` vs. a
  yet-to-be-invented SignalProcessing shape). A single `Spec.TimesOutAt *metav1.Time` field name,
  identical across all three CRDs, lets shared tooling and mental models generalize immediately.
  It also collapses AIAnalysis's `AIAnalysisTimeoutConfig.AnalyzingTimeout` sub-field, which was
  unused dead schema with no corresponding RO status source, keeping the new field lean.

### Alternative C: RO enforces phase timeout by deleting/failing the child CRD directly

Rather than having the child self-enforce, RO's own outer backstop transitions the child CRD to
`Failed` itself when its phase timeout fires.

- **Rejected**: violates the existing "the supervisor keeps its own clock even when the child has
  one" defense-in-depth principle already proven for RO -> {SP, AA, WE, EA, NT}. RO's backstop is
  intentionally coarse and independent; a truly-hung subcontroller (e.g. blocked on a slow K8s API
  call) may not even be reconciling to notice an externally-imposed failure promptly. Self-enforcement
  gives the *first* and *fastest* failure signal, from the actor most likely to detect it quickly, while
  RO's unchanged outer check remains the safety net.

## Scope Exclusion: EffectivenessAssessment (EA) and NotificationRequest (NT)

EA (Verifying phase) and NT (Notifying phase) are **not** covered by this decision.
`RemediationRequestStatus.TimeoutConfig` has only four fields (`Global`/`Processing`/`Analyzing`/`Executing`)
— there is no authoritative `Verifying`/`Notifying` duration to propagate, and RO's own
`checkPhaseTimeouts` doesn't even backstop those two phases today. Extending `TimeoutConfig` with
two new fields, extending RO's own backstop to cover them, and adding the same
propagate-and-self-enforce pattern to EA/NT is a separate, larger feature, tracked as a follow-up:
[Issue #2181](https://github.com/jordigilh/kubernaut/issues/2181).

## Consequences

### Positive

- ✅ **Single source of truth, end-to-end**: an operator's `AC-028-5` override of
  `Status.TimeoutConfig.{Processing,Analyzing,Executing}` now has a real, verifiable effect on the
  corresponding child CRD's actual self-enforced behavior, not just on RO's own outer check.
- ✅ **Faster failure signal**: a stuck child CRD self-fails on its own next reconcile past its
  deadline, rather than waiting for RO's typically coarser outer backstop interval.
- ✅ **Uniform mental model**: `Spec.TimesOutAt` means the same thing on every child CRD.
- ✅ **No clock-skew ambiguity**: an absolute deadline needs no re-anchoring by the consumer.
- ✅ **Dead schema removed**: `AIAnalysisTimeoutConfig` and `WorkflowExecutionSpec.ExecutionConfig`
  (never-populated, never-read fields) are deleted outright rather than left to rot alongside the
  new field.

### Negative

- ⚠️ **Two timeout layers to reason about** (child self-enforcement + RO's outer backstop) for any
  given phase.
  - **Mitigation**: the two layers are intentionally decoupled and independently testable; the
    child layer is documented as the fast path, RO's backstop as the defense-in-depth fallback.
- ⚠️ **EA/NT remain backstop-only** until the follow-up issue lands, so this decision does not yet
  give full five-CRD coverage.
  - **Mitigation**: explicitly tracked, not silently dropped — see [Issue #2181](https://github.com/jordigilh/kubernaut/issues/2181).

## Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-TIMEOUT-001](DD-TIMEOUT-001-global-remediation-timeout.md) | Extended by this decision — DD-TIMEOUT-001 stops at RO's own boundary and never specified child-CRD propagation |
| [BR-ORCH-027-028](../../requirements/BR-ORCH-027-028-timeout-management.md) | AC-028-2 (phase timeout before global) and AC-028-5 (per-remediation override) are now enforced end-to-end via this decision |
| [DD-AIANALYSIS-001](DD-AIANALYSIS-001-spec-structure.md) | Notes `AIAnalysisTimeoutConfig` as superseded by `TimesOutAt` |
| [Issue #2176](https://github.com/jordigilh/kubernaut/issues/2176) | Parent issue implementing this decision |
| [Issue #2181](https://github.com/jordigilh/kubernaut/issues/2181) | Follow-up: extend to EffectivenessAssessment / NotificationRequest |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08 | Initial decision: propagate `Status.TimeoutConfig` into SignalProcessing/AIAnalysis/WorkflowExecution as `Spec.TimesOutAt`; self-enforcement in each subcontroller; EA/NT explicitly excluded (follow-up #2181) |
