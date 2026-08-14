# Test Plan: #2078 — AF-Side Tool-Call Retry Circuit Breaker

## 1. Business Requirement

`BR-SAFETY` (fail-fast on stuck LLM tool retries). See
[DD-AF-013](../../architecture/decisions/DD-AF-013-tool-retry-circuit-breaker.md) for the full
design rationale, rejected alternatives, and why the fix lives in `reinvokingRunner.Run` rather than
an `AfterToolCallback`.

## 2. Design Summary

`toolRetryCircuitBreaker` (`pkg/apifrontend/launcher/reinvoking_runner.go`) tracks, per tool name, a
consecutive-failure count across the event stream yielded by a single `r.inner.Run(...)` call.
`DefaultToolRetryCircuitBreakerThreshold` (5) consecutive failures for the same tool name trips the
breaker: `reinvokingRunner.Run` stops consuming the inner iterator, emits an
`audit.EventCircuitBreakerTrip` event, and yields a synthetic terminal error — reusing the
already-tested "any error stops the turn, no reinvocation" contract.

## 3. FedRAMP / SOC2 Control Mapping

- **SI-4** (system monitoring / fail-fast on anomalous behavior): an LLM stuck retrying the same
  failing tool call is exactly the kind of anomalous internal-loop behavior this control expects the
  system to detect and stop, rather than silently exhausting the session timeout.
- **AU-3** (content of audit records): the trip is recorded via `audit.EventCircuitBreakerTrip` with
  the failing tool name and failure count, giving operators a queryable forensic record of why a
  session ended abnormally instead of a silent timeout with no signal.

## 4. Pyramid Invariant — Test Scenario Inventory

| Tier | Test ID(s) | File | Proves |
|---|---|---|---|
| UT | `UT-AF-2078-001..004` | `pkg/apifrontend/launcher/toolretry_circuitbreaker_test.go` | `toolRetryCircuitBreaker.observe()` in isolation: N consecutive failures for one tool trips; a success resets that tool's count; a different tool's failures/successes don't cross-contaminate; a non-function-response event is a no-op |
| IT | `IT-AF-2078-001` | `pkg/apifrontend/launcher/reinvoking_runner_test.go` | `reinvokingRunner.Run()` wired end-to-end: a fake inner `Runner` that would otherwise yield unboundedly many consecutive failures for the same tool within one `Run()` call is stopped after exactly the threshold, yields a terminal error, and does not reinvoke |
| IT | `IT-AF-2078-002` | same file | Failures below the threshold, followed by a genuine success, do NOT trip — the turn completes normally |
| IT | `IT-AF-2078-003` | same file | On trip, the configured `audit.Emitter` receives exactly one `EventCircuitBreakerTrip` event with `Detail["circuit_name"]`/`Detail["failure_count"]` set (matching `audit.buildCircuitBreakerTripPayload`'s expected schema, not just a log line) |
| IT | `IT-AF-2078-004` | same file | A failing tool interleaved with a *different*, successful tool does not trip (per-tool-name isolation proven at the wiring level, not just in the UT) |
| Wiring | n/a | `cmd/apifrontend` (wherever `launcher.NewA2AHandler` is constructed) | `cfg.Auditor` reaches `reinvokingRunner` unchanged — mechanical signature threading, covered by existing `NewA2AHandler` construction tests plus IT-AF-2078-003 above (a nil-auditor path is not distinguishable from a wiring bug, so IT-AF-2078-003 is the actual proof) |

E2E: not added — this is an internal-loop safety net for a currently-dormant failure mode (the #2073
fix already removed the concrete trigger QE observed); the existing `fullpipeline` E2E suite already
proves normal tool-call turns complete correctly, which is the only behavior this change could
regress.

## 5. Confidence

92% — the mechanism (Go 1.23 range-over-func early-stop propagating through
`Flow.Run`/`runner.Runner.Run`) was verified by reading the vendored ADK source at each layer, not
assumed. The residual 8% is: (a) this is a coarser trigger than "genuine schema-validation failure
only" (accepted trade-off, see DD-AF-013), and (b) the exact threshold (5) is a judgment call, not
empirically tuned against a live-LLM spike (unlike #2118/#2120's 10/10-trial spikes) — justified
because the currently-dormant nature of this gap (per #2078's own text) makes a live-LLM spike lower
value than for an actively-reproducing bug.
