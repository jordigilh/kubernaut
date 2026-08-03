# ADR-019: AIAnalysis Error Classification & Retry Strategy (KA Calls)

**Status**: ✅ **APPROVED** (implemented, Go — with corrections; see Version History)
**Date**: 2025-10-17
**Related**: ADR-018 (Approval Notification Integration), [ADR-045](ADR-045-aianalysis-ka-service-contract.md) (AIAnalysis ↔ KA contract)
**Confidence**: 90% (original proposal) → rewritten against actual implementation, 2026-08-01

**Last Updated**: 2026-08-01 — Rewritten against the Go implementation
([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). The original document
proposed a detailed retry/circuit-breaker/manual-fallback design for the (then-hypothetical)
Python HolmesGPT-API integration. What was actually built for the Go implementation differs
substantially: no circuit breaker exists on this path, the retry parameters are different, the
proposed `AIApprovalRequest` CRD was never built under that name, and none of the proposed
`holmesgpt_*` status fields or metrics exist. See "Known Corrections vs. Original Design" below.

---

## Context & Problem

Kubernaut Agent (KA) is an **internal but network-separated** dependency for AI-powered root
cause analysis — AIAnalysis calls it over HTTP. If KA becomes unavailable (pod down, LLM
provider unavailable, network partition), the AIAnalysis controller cannot proceed with that
investigation.

**Critical Questions** (unchanged from the original ADR — still the right questions):
1. **What happens when KA is down?** Block all remediations? Fail immediately? Retry?
2. **How long should we retry?** Indefinitely? Fixed duration? Exponential backoff?
3. **What should operators see?** Clear status? Actionable error messages?
4. **What is the fallback?** Manual approval? Queue for later? Fail fast?

**Impact**:
- **KA unavailability blocks the affected AIAnalysis** without a retry strategy
- **No clear operator communication** during transient failures
- **Complete remediation blockage** for extended outages, without a fallback

---

## Decision (As Implemented)

**IMPLEMENTED: Classified Exponential-Backoff Retry (5 attempts, ~10 minutes worst case) + Escalation via RemediationOrchestrator, No Circuit Breaker**

**Strategy** (`pkg/aianalysis/handlers/error_classifier.go`, `investigating.go`):
1. Classify every KA-call error by cause (HTTP status / network / timeout / context-canceled)
2. **Retry with exponential backoff + jitter** for retryable errors, up to a fixed **attempt
   count** (5), not a fixed wall-clock timeout
3. **Update `AIAnalysis.Status`** (`Message`, `Reason`, `SubReason`, `ConsecutiveFailures`) each
   attempt so operators can see retry state via `kubectl get aianalysis` / `describe`
4. **After 5 attempts (or immediately for non-retryable errors)**: transition `AIAnalysis` to
   `Phase=Failed`
5. **Escalation fallback**: RemediationOrchestrator's manual-review notification path
   ([BR-ORCH-036](../../requirements/BR-ORCH-036-manual-review-notification.md)) detects the
   failure reason and creates a `NotificationRequest` (`type=manual-review`) — there is no
   dedicated approval CRD created directly by AIAnalysis for this case

**Rationale** (updated to match what was actually built):
- ✅ **Resilient to transient failures**: rate limits, 5xx, timeouts, network errors all retry
- ✅ **Clear observability**: `Status.Message`/`Reason`/`SubReason` reflect retry state; `aianalysis_failures_total` metric records outcomes
- ✅ **Bounded retry**: capped at 5 attempts (not indefinite)
- ✅ **Escalation path exists**: RO's `BR-ORCH-036` notification flow ensures a human is alerted on unrecoverable failure
- ⚠️ **No circuit breaker**: unlike the original proposal, there is no breaker on this path — see "Circuit Breaker Applicability" below for why this turned out not to matter in practice

---

## Design Details (As Implemented)

### Error Classification

`ErrorClassifier.ClassifyError()` (`pkg/aianalysis/handlers/error_classifier.go`) classifies
every error from a KA call into one of:

| Error Type | Trigger | Retryable | Alerts |
|------------|---------|-----------|--------|
| `Authentication` | HTTP 401 | ❌ No | ✅ Yes |
| `Authorization` | HTTP 403 | ❌ No | ✅ Yes |
| `Configuration` | HTTP 404 | ❌ No | ✅ Yes |
| `Permanent` | HTTP 400, 422 | ❌ No | ✅ Yes |
| `RateLimit` | HTTP 429 | ✅ Yes (2× base delay) | ❌ No (expected) |
| `Transient` | HTTP 5xx, unknown status | ✅ Yes | 5xx: No; unknown: Yes |
| `Timeout` | `context.DeadlineExceeded`, `net.Error.Timeout()` | ✅ Yes | ❌ No |
| `Network` | DNS failure, connection refused, generic `net.Error` | ✅ Yes | DNS/refused: Yes; generic: No |
| `SessionLost` | 404 on session poll (BR-AA-KA-064.5) | Session regeneration, not standard retry | — |
| `Permanent` (caller cancel) | `context.Canceled` | ❌ No | ❌ No |

### Retry Schedule (Exponential Backoff, Actual)

Per `NewErrorClassifier()` defaults (`pkg/aianalysis/handlers/error_classifier.go`) and `MaxRetries = 5` (`pkg/aianalysis/handlers/constants.go`):

| Attempt | Delay (base, before jitter) | Cumulative |
|---------|------------------------------|------------|
| 1 | ~1s | ~1s |
| 2 | ~2s | ~3s |
| 3 | ~4s | ~7s |
| 4 | ~8s | ~15s |
| 5 | ~16s | ~31s |
| 6th failure | — (max retries exceeded) | `Phase=Failed` |

**Backoff formula** (`pkg/shared/backoff`, [DD-SHARED-001](DD-SHARED-001-shared-backoff-library.md)):
```go
delay := min(basePeriod * (multiplier ^ attempt), maxPeriod) ± jitterPercent
// basePeriod = 1s, multiplier = 2.0, maxPeriod = 5m, jitterPercent = 10
```

**Total attempts**: exactly 5 (not "~12-13 over 5 minutes" as originally proposed). Worst-case
wall-clock time to permanent failure is ~31 seconds of backoff delay, not 5 minutes — because the
implementation bounds retries by **attempt count**, not by an elapsed-time budget. RateLimit
errors additionally double the base delay (2× `BasePeriod`) per classification.

### AIAnalysis Status Updates (Actual Fields)

Retry state is **not** tracked with dedicated `holmesGPTRetryAttempts`/`holmesGPTLastError`
fields as originally proposed. It reuses existing generic fields:

```yaml
# During retry
status:
  phase: "Investigating"
  message: "Transient error (attempt 3/5): <error>"
  reason: "TransientError"          # aianalysisv1.ReasonTransientError
  subReason: "Timeout"              # mapped from ErrorClassification.ErrorType
  consecutiveFailures: 3            # Status.ConsecutiveFailures, not a HolmesGPT-specific counter

# After max retries exceeded
status:
  phase: "Failed"
  message: "Transient error exceeded max retries (5 attempts): <error>"
  reason: "APIError"                 # aianalysisv1.ReasonAPIError
  subReason: "MaxRetriesExceeded"    # aianalysisv1.SubReasonMaxRetriesExceeded
  completedAt: "2026-08-01T10:35:20Z"
```

There is no `HolmesGPTAvailable` condition type, no `holmesGPTNextRetryTime`/
`holmesGPTTotalRetryDuration` field, and no `requiresApproval`/`approvalContext` populated
directly by this failure path (that machinery is used for the *analysis result* approval
decision — [ka-approval.md](../../services/crd-controllers/02-aianalysis/ka-approval.md) — not
for infrastructure failures).

### Configuration (Actual)

Retry parameters are **not** externally configurable via ConfigMap or environment variables as
originally proposed — they are compile-time constants:

- `MaxRetries = 5` — `pkg/aianalysis/handlers/constants.go`
- Backoff config (`BasePeriod=1s`, `MaxPeriod=5m`, `Multiplier=2.0`, `JitterPercent=10`) —
  `NewErrorClassifier()` in `pkg/aianalysis/handlers/error_classifier.go`

There is no `kubernaut-aianalysis-config` ConfigMap and no
`HOLMESGPT_RETRY_TIMEOUT`/`HOLMESGPT_INITIAL_DELAY`/etc. environment variables in the codebase.

---

## Implementation (Actual)

**Files**:
- `pkg/aianalysis/handlers/error_classifier.go` — classification + backoff calculation
- `pkg/aianalysis/handlers/investigating.go` — `handleError()`, `retryTransientError()`, `failMaxRetriesExceeded()`, `failPermanentError()`

```go
// pkg/aianalysis/handlers/investigating.go (actual, simplified)
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
    classification := h.errorClassifier.ClassifyError(err)
    analysis.Status.ConsecutiveFailures++

    if h.errorClassifier.ShouldRetry(classification, int(analysis.Status.ConsecutiveFailures)) {
        return h.retryTransientError(analysis, err, classification) // ctrl.Result{RequeueAfter: backoff}
    }
    if classification.IsRetryable {
        return h.failMaxRetriesExceeded(ctx, analysis, err, classification) // Phase=Failed, SubReason=MaxRetriesExceeded
    }
    return h.failPermanentError(ctx, analysis, err, classification) // Phase=Failed, immediate
}
```

Retry is driven by controller-runtime's standard `ctrl.Result{RequeueAfter: backoffDuration}`
mechanism — there is no bespoke in-process sleep/retry loop, and no separate "HolmesGPT
availability" reconcile branch as the original pseudocode proposed.

On terminal failure, both failure paths call `h.auditClient.RecordAnalysisFailed(ctx, analysis, err)`
(BR-AUDIT-005 Gap #7) and `aianalysis.SetInvestigationComplete(analysis, false, ...)`. **No
`AIApprovalRequest` (or any other CRD) is created directly by AIAnalysis** — see "Known
Corrections" below for the actual escalation path.

---

## Prometheus Metrics (Actual)

**Real metric** (`pkg/aianalysis/metrics/metrics.go`):

```go
const MetricNameFailuresTotal = "aianalysis_failures_total" // labels: reason, subReason
```

`h.metrics.RecordFailure(reason, subReason)` is called on every terminal failure (transient
retry, max-retries-exceeded, and permanent). There are **no** `holmesgpt_requests_total`,
`holmesgpt_retry_attempts_total`, `holmesgpt_retry_duration_seconds_bucket`,
`holmesgpt_unavailability_incidents_total`, or `holmesgpt_manual_fallback_total` metrics in the
codebase — none of the originally proposed metric names or the example PromQL queries below them
were implemented.

---

## Alerting Rules

No `holmesgpt_availability` Prometheus alert group exists in the codebase. Alerting on repeated
KA-call failures today would need to be built on `aianalysis_failures_total{reason="APIError",
subReason="MaxRetriesExceeded"}` — this is a documented gap, not an implemented alert.

---

## Business Requirements

**Real BRs governing this behavior** — the original document's proposed `BR-AI-061` through
`BR-AI-065` (see "Known Corrections" below) were never formalized as standalone requirements and
`BR-AI-065` collides with an unrelated, already-implemented requirement (Action Selection
Algorithm Logic, `docs/services/stateless/ai-ml/BR_MAPPING.md`). The actual governing
requirements are:

| BR | Description |
|----|-------------|
| **BR-AI-009** | Error classification and handling |
| **BR-AI-010** | Retry logic for transient failures; fail immediately on permanent errors |
| **BR-AUDIT-005** (Gap #7) | Record failure audit with standardized error details on every terminal AIAnalysis failure |
| **[BR-ORCH-036](../../requirements/BR-ORCH-036-manual-review-notification.md)** v3.0 | RO escalates unrecoverable AIAnalysis failures (`APIError`/`MaxRetriesExceeded`/`TransientError`/`PermanentError`) via `NotificationRequest` |

---

## Known Corrections vs. Original Design

1. **No circuit breaker.** Confirmed by code search (no `gobreaker` or equivalent wraps the
   AIAnalysis→KA call path) — retry-with-backoff-and-a-bounded-attempt-count is the sole
   resilience mechanism. A real circuit breaker (`sony/gobreaker/v2`) does exist elsewhere in the
   codebase, for Notification's external delivery channels
   (`pkg/notification/delivery/circuit_breaker.go`) — so the "Circuit Breaker Applicability
   Matrix" below is directionally still useful, but its "AIAnalysis Controller → KA: circuit
   breaker REQUIRED" row was never actually built.
2. **No `AIApprovalRequest` CRD**, under that name or any other, created directly by AIAnalysis
   on KA-call failure. `AIApprovalRequest` was renamed to `RemediationApprovalRequest`
   ([ADR-040](ADR-040-remediation-approval-request-architecture.md)) — but that CRD is created by
   RO in response to an AIAnalysis's *approval-required analysis result*
   ([ka-approval.md](../../services/crd-controllers/02-aianalysis/ka-approval.md)), not in
   response to KA being unreachable. The actual escalation path for KA-unreachable failures is
   RO's `NotificationRequest`-based manual-review flow (BR-ORCH-036 v3.0).
3. **Retry is bounded by attempt count (5), not wall-clock time (5 minutes).** Worst-case backoff
   delay is ~31 seconds, not 305 seconds.
4. **No dedicated `HolmesGPT*` status fields, conditions, or ConfigMap/env-var configuration.**
   Retry state reuses `Status.ConsecutiveFailures`/`Message`/`Reason`/`SubReason`; retry
   parameters are Go constants, not runtime-configurable.
5. **`BR-AI-061`–`BR-AI-065` were never implemented as proposed.** `V1_0_VS_V1_1_SCOPE_DECISION.md`
   deferred "HolmesGPT retry" to a hypothetical v1.1 scope that itself was superseded by the
   simpler `ErrorClassifier` design actually shipped under BR-AI-009/BR-AI-010. `BR-AI-065` is
   now in use for an unrelated requirement (Action Selection Algorithm Logic) — do not reuse it.

---

## Alternatives Considered (Original — Still Valid as Historical Rationale)

The alternatives analysis below was part of the original decision-making process and remains
useful context for *why* an attempt-bounded retry-with-escalation approach was chosen over the
alternatives, even though the specific parameters changed on implementation.

### Alternative 1: Exponential Backoff with Escalation Fallback (APPROVED, as adapted)

**Pros**: resilient to transient failures; observable via status + metrics; bounded retry;
escalation path exists so the system remains usable even if KA is down; simple (constants, not
external config).

**Cons**: adds retry-classification complexity (~250 lines); requires manual intervention (via
RO's notification flow) after max retries.

### Alternative 2: Fail Fast (No Retry) — REJECTED

No resilience to transient failures (network blips, rate limits); high false-failure rate.

### Alternative 3: Infinite Retry — REJECTED

Remediation blocks indefinitely; no escalation; resource risk if many AIAnalyses retry forever.

### Alternative 4: Queue for Later — REJECTED

Delayed response; queue-management complexity; misses MTTR targets.

---

## Testing Strategy

Real coverage lives in:
- `pkg/aianalysis/error_classifier_test.go` — classification + backoff unit tests
- `pkg/aianalysis/investigating_handler_test.go`, `investigating_handler_recovery_test.go` — retry/failure integration tests
- `pkg/aianalysis/investigation_timeout_test.go` — timeout classification and terminal-failure assertions

The original document's proposed "Chaos Tests" (kill KA pod, network partition, high latency)
are not implemented as a distinct chaos-testing tier; equivalent behavior is covered by the
integration tests above using injected/simulated errors rather than live infrastructure faults.

---

## Circuit Breaker Applicability Matrix

**Purpose**: Clarify when a circuit breaker IS vs IS NOT needed across Kubernaut services. This
decision framework remains valid; only the AIAnalysis→KA row's outcome changed (breaker was
evaluated as unnecessary in practice — attempt-bounded retry was judged sufficient given KA's
in-cluster, not-truly-external nature).

### Applicability Decision Tree

```
Is the dependency EXTERNAL (outside the Kubernaut cluster / outside the trust boundary)?
├── YES → Circuit breaker likely warranted
│         (Slack API, Email SMTP, external webhooks — see Notification's real gobreaker usage)
└── NO (internal, in-cluster) → Is the dependency rate-limited by Kubernetes?
    ├── YES → Circuit breaker NOT NEEDED
    │         (K8s API returns 429, controller-runtime handles backpressure)
    └── NO → Is the call fire-and-forget (ADR-038)?
        ├── YES → Circuit breaker NOT NEEDED
        │         (async audit writes don't block business operations)
        └── NO → Circuit breaker OPTIONAL
                  (evaluate based on failure impact; attempt-bounded retry may suffice — see AIAnalysis→KA)
```

### Service-by-Service Matrix (Corrected)

| Service | Primary Dependencies | External? | Circuit Breaker | Rationale |
|---------|---------------------|-----------|-----------------|-----------|
| **AIAnalysis Controller** | Kubernaut Agent (KA) | ⚠️ In-cluster, not truly external | ❌ **Not implemented** | Attempt-bounded retry + backoff (this ADR) judged sufficient; KA runs in-cluster, unlike a third-party SaaS API |
| **Notification Controller** | Slack API, Email SMTP | ✅ Yes | ✅ **Implemented** (`sony/gobreaker/v2`, `pkg/notification/delivery/circuit_breaker.go`) | Genuinely external, rate-limited third-party APIs |
| **Gateway** | External webhooks | ⚠️ Varies | ⚠️ Evaluate per-provider | Depends on webhook provider reliability |
| **Signal Processing** | K8s API, Data Storage API | ❌ No | ❌ **Not needed** | Internal services, K8s rate limits apply |
| **Remediation Orchestrator** | K8s API (CRD creation) | ❌ No | ❌ **Not needed** | K8s handles backpressure via 429 |
| **Remediation Execution** | K8s API (Jobs/Tekton) | ❌ No | ❌ **Not needed** | K8s handles backpressure via 429 |
| **Data Storage** | PostgreSQL | ❌ No | ❌ **Not needed** | Connection pool handles; internal DB |

### Why Most CRD Controllers Don't Need a Circuit Breaker

1. K8s API has built-in rate limiting — 429 responses handled via controller requeue
2. controller-runtime manages the work queue — natural backpressure mechanism
3. Data Storage audit writes use fire-and-forget (ADR-038) — don't block reconciliation
4. No third-party external service to protect from cascading overload

### Cross-References

- **ADR-038**: Async Buffered Audit Ingestion (fire-and-forget pattern)
- **DD-007**: Kubernetes-Aware Graceful Shutdown
- **[ADR-045](ADR-045-aianalysis-ka-service-contract.md)**: AIAnalysis ↔ KA service contract (retry/timeout details)
- **`pkg/notification/delivery/circuit_breaker.go`**: The one real circuit breaker in the codebase, for genuinely external notification channels

---

## References

1. **ADR-018**: Approval Notification Integration
2. **ADR-038**: Async Buffered Audit Ingestion (fire-and-forget audit pattern)
3. **[ADR-045](ADR-045-aianalysis-ka-service-contract.md)**: AIAnalysis ↔ Kubernaut Agent service contract
4. **[ADR-040](ADR-040-remediation-approval-request-architecture.md)**: `RemediationApprovalRequest` architecture (actual approval CRD)
5. **[BR-ORCH-036](../../requirements/BR-ORCH-036-manual-review-notification.md)**: Manual Review & Escalation Notification (actual escalation path)
6. **Business Requirements**: BR-AI-009, BR-AI-010, BR-AUDIT-005 (Gap #7)
7. `pkg/aianalysis/handlers/error_classifier.go`, `investigating.go` — implementation source of truth

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2026-08-01 (full rewrite against Go implementation, Issue #1806)
**Version**: v2.0
**Original Version**: v1.1 (2025-11-28) — proposed design for a Python HolmesGPT-API integration never built as specified
