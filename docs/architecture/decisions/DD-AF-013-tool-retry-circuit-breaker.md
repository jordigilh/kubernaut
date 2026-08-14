# DD-AF-013: AF-Side Tool-Call Retry Circuit Breaker

**Status**: Accepted
**Date**: 2026-08-13
**Issue**: [#2078](https://github.com/jordigilh/kubernaut/issues/2078)
**Related**: [DD-AF-011](DD-AF-011-phase-transition-consent-guard.md) (`reinvokingRunner.Run` — the same
wrapping iterator this decision extends), KA's `AnomalyDetector.RecordFailure`
(`internal/kubernautagent/investigator/anomaly.go` — the repeated-tool-failure precedent this
design mirrors for AF)

## Context

`google.golang.org/adk`'s model-generation loop (`internal/llminternal/base_flow.go`'s `Flow.Run`)
has no retry cap when a tool call repeatedly fails ADK's own JSON-schema validation of the LLM's
function-call arguments. If the LLM gets stuck emitting a `kubernaut_present_decision` (or any AF
tool) call whose arguments fail schema validation on every attempt, the loop retries indefinitely
until the outer session timeout, rather than failing fast. Flagged by QE while validating #2073;
confirmed present at `google.golang.org/adk@v1.5.1`. The #2073 fix removed the specific trigger QE
observed, so this is a latent gap, not a live failure — but it is a real, separate risk for any
future tool-schema regression, and is explicitly targeted for `v1.6`/`main` (not backported to
`v1.5.6`, since it is new functionality, not a defect fix).

### Why `AfterToolCallback` cannot implement this

KA already has an analogous defense (`AnomalyDetector.RecordFailure`, gating via its own
`AfterToolCallback`-equivalent in the investigator loop). AF's own gates
(`pkg/apifrontend/agent/phase_guard.go`'s `newPhaseGuard`) use the same `AfterToolCallback` seam.
That seam was investigated first for this fix and rejected: traced through the vendored ADK source
(`internal/llminternal/base_flow.go`'s `callTool`), an `AfterToolCallback`'s returned error is
**always** folded back into the tool's *result* map (`map[string]any{"error": err.Error()}`) before
being handed back to the LLM as a `FunctionResponse` — it is never surfaced as a Go `error` that
could stop `Flow.Run`'s loop. A callback can shape what the LLM sees; it cannot make the loop stop
retrying.

### Why schema-validation-specific detection was rejected

The natural refinement — trip only on a genuine ADK schema-validation failure, not an ordinary
business-logic tool error — was investigated and rejected. `github.com/google/jsonschema-go@v0.4.3`
(`jsonschema/validate.go`'s `Resolved.Validate`, which ADK calls before ever invoking the tool's Go
handler) returns plain `fmt.Errorf`-wrapped errors (`jsonschema/util.go`'s `wrapf`) with **no**
exported, distinguishable error type — nothing for `errors.As`/`errors.Is` to key off. The only
alternatives were fragile message string-matching (would silently break on a future
ADK/jsonschema-go wording change, with no compile-time signal) or adding a "did the real handler
run" marker inside `kubernaut_present_decision`'s own business logic (touches business code for a
test/observability-only concern, and does not generalize to "any AF tool" per the issue's own
scope). Both were rejected as worse trade-offs than accepting a coarser trigger.

## Decision

### Trip on N consecutive same-tool-name failures, detected at the one seam that can genuinely stop the loop

`reinvokingRunner.Run` (`pkg/apifrontend/launcher/reinvoking_runner.go`) already wraps the full
per-turn event stream from `r.inner.Run(...)`, which is — transitively, through
`runner.Runner.Run` → `Agent.Run` → `Flow.Run` — the *same* iterator ADK's own model-generation loop
drives, using Go 1.23 range-over-func semantics throughout (`if !yield(ev, nil) { return }` at every
layer, confirmed by reading `base_flow.go`, `runner.go`). This means the wrapping consumer stopping
early (`break`/`return` instead of continuing to pull) is a fully supported, already-idiomatic way
to terminate ADK's internal loop — no vendored-code change required.

A new `toolRetryCircuitBreaker` tracks, per tool name, a consecutive-failure count: incremented each
time a yielded event's `FunctionResponse.Response["error"]` is non-empty for that tool, reset to
zero on that tool's next success. (Different tools' counters are independent — a failing
`kubernaut_present_decision` interleaved with successful `kubernaut_list_workflows` calls still
trips on the former.) This does not distinguish schema-validation failures from business-logic tool
errors (see rejected refinement above) — either kind of persistent single-tool failure means the
LLM is stuck on that tool, which is the actual condition worth breaking on, consistent with KA's
own `MaxRepeatedFailures` precedent (also failure-count-based, not failure-cause-based).

On trip (`DefaultToolRetryCircuitBreakerThreshold = 5` consecutive failures — slightly more lenient
than KA's `MaxRepeatedFailures: 3`, since a schema-validation self-correction may reasonably take an
extra attempt or two): `reinvokingRunner.Run` stops consuming `r.inner.Run(...)`'s iterator and
yields a synthetic terminal error (`fmt.Errorf("tool %q failed %d consecutive times, aborting to
avoid an unbounded retry loop (#2078)", ...)`). This reuses the *already-tested* `hadError` path —
"any error from a turn stops the loop immediately, no further reinvocation attempts" — rather than
introducing a new control-flow branch. An `audit.EventCircuitBreakerTrip` event is emitted (existing
event type, reused rather than adding a new one to the audit catalog), with
`Detail: {"circuit_name": <tool name>, "failure_count": "<N>"}`. This key choice deliberately follows
`audit.buildCircuitBreakerTripPayload` (`pkg/apifrontend/audit/store_adapter.go`) — the function that
actually serializes this event type into the OpenAPI-typed payload sent to the audit backend — rather
than `pkg/apifrontend/resilience/circuitbreaker.go`'s own `Detail: {"dependency": ...}` convention for
the *same* event type: that HTTP-transport-level breaker's Detail keys are not read by
`buildCircuitBreakerTripPayload` at all (a pre-existing inconsistency, out of scope to fix here), so
matching it would have produced an event that logs but never audits under the intended schema — the
opposite of AU-3's goal.

### Threshold is a Go constant, not a Helm/config value

`DD-PLATFORM-006`'s entire thesis is reducing operator-facing config surface to only what genuinely
needs per-deployment tuning. This threshold is a safety-net default, not something an operator would
reasonably need to tune per cluster — it is `DefaultToolRetryCircuitBreakerThreshold`, an exported
Go constant in `pkg/apifrontend/launcher`, not a new YAML/Helm field.

## Alternatives Considered

### A: `AfterToolCallback`-based circuit breaker (rejected)

Investigated first, since it is the seam KA and AF's own existing gates (`phase_guard.go`) already
use. Rejected because it is mechanically incapable of stopping the loop — see "Why
`AfterToolCallback` cannot implement this" above. Confirmed by reading, not assumed.

### B: Schema-validation-specific detection via `errors.As` (rejected)

Would have let the breaker trip only on genuine ADK schema mismatches, not ordinary business-logic
tool errors — more precise in principle. Rejected because `jsonschema-go` exposes no distinguishable
error type to key off; the only fallbacks (string-matching, or a handler-invocation marker specific
to one tool) were judged worse trade-offs than accepting the coarser, cause-agnostic trigger. See
"Why schema-validation-specific detection was rejected" above.

### C: Defer — leave as a documented latent risk (rejected for this cycle)

The #2073 fix already removed the concrete trigger QE observed, so this gap is not currently causing
failures. Rejected because the issue is explicitly milestoned for `v1.6`, and an unbounded retry
until session timeout (rather than a fast, auditable failure) is a worse operator experience for any
future regression that does trigger it — the fix is cheap (one wrapper, no vendored-code change) once
the mechanism is understood.

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `toolRetryCircuitBreaker` | Every event yielded by `r.inner.Run(...)` inside `reinvokingRunner.Run` | `pkg/apifrontend/launcher/reinvoking_runner.go` | IT-AF-2078-001..004 |
| Audit emission on trip | `reinvokingRunner.Run`'s trip path | `pkg/apifrontend/launcher/reinvoking_runner.go` (new `auditor audit.Emitter` field, wired from `newReinvokingRunnerProvider`) | IT-AF-2078-003 |
| Wiring: `cfg.Auditor` → `reinvokingRunner` | `NewA2AHandler` | `pkg/apifrontend/launcher/launcher.go` (`newReinvokingRunnerProvider(runnerCfg, log, cfg.Auditor)`) | IT-AF-2078-003 |

## Consequences

### Positive

- Closes the unbounded-retry gap with zero vendored-code changes, reusing Go 1.23's own
  range-over-func "stop pulling to stop the producer" idiom, which ADK's own code already relies on
  throughout.
- Reuses the already-tested "error stops the turn, no reinvocation" contract instead of adding a new
  control-flow branch to `reinvokingRunner.Run`.
- Reuses the existing `audit.EventCircuitBreakerTrip` event type and Detail-key convention rather
  than growing the audit catalog.
- Threshold is a plain Go constant — no new operator-facing config surface, consistent with
  DD-PLATFORM-006.

### Negative

- Cannot distinguish "ADK schema validation failure" from "ordinary business-logic tool error" — a
  tool that legitimately, repeatedly rejects bad-but-schema-valid input from a confused LLM will also
  trip this breaker. Judged acceptable: KA's own precedent makes the same trade-off, and either case
  represents an LLM stuck on the same tool, which is the actual condition this breaker exists to catch.
- A tool call retried with genuinely different (but still failing) arguments each time still
  increments the same per-tool-name counter (by design — it does not key on argument hash, unlike
  KA's `AnomalyDetector`, precisely because schema-validation retries are expected to vary their
  arguments as the LLM tries to self-correct).
