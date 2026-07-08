# DD-LLM-009: Shared Retry Helper with Error Classification for Streaming and Non-Streaming LLM Calls

**Status**: ✅ Approved & Implemented
**Priority**: P1
**Owner**: KubernautAgent Team
**Scope**: `pkg/kubernautagent/llm/chat_helpers.go`, `pkg/kubernautagent/llm/anthropicfamily/client.go`, `pkg/kubernautagent/llm/openai/client.go`, `pkg/shared/llm/openaicompat/client.go`, `internal/kubernautagent/investigator/investigator_loop.go`
**Related**: [DD-HAPI-019](./DD-HAPI-019-go-rewrite-design/DD-HAPI-019-go-rewrite-design.md) (Framework Isolation), [DD-LLM-008](./DD-LLM-008-restart-required-llm-identity-lock.md), Issue #1612, Issue #1585

---

## Context & Problem

Two related defects in Kubernaut Agent's (KA) LLM call path were investigated and fixed together because fixing either one alone in isolation would have left the other's bug latent in the code the first one touches.

**#1612 — the streaming LLM call path has no retry at all.** `chatOrStream` (`internal/kubernautagent/investigator/investigator_loop.go`) is the call path used whenever a session event sink is present in `ctx` — i.e. every real interactive/production investigation, not just an autonomous fallback. It called `client.StreamChat` exactly once, with no retry loop whatsoever. `RuntimeParams.MaxRetries`, a real hot-reloadable operator setting, was silently dead code on this path even though the non-streaming `ChatWithParams` path honored it. Additionally, `callLLMTurn`'s cancellation check (`errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)`) misclassified a per-attempt backend timeout as an operator/system cancellation: `chatOrStream` builds a *child* context via `context.WithTimeout(ctx, ...)` for each call, and when that child expires on its own, the *parent* `ctx.Err()` is still `nil` — this is "the backend was too slow," not "someone cancelled the investigation," but the code treated them identically.

**#1585 — `ChatWithParams` retries every error identically.** The existing non-streaming retry loop in `ChatWithParams` (`pkg/kubernautagent/llm/chat_helpers.go`) retries on any non-nil error for the full `MaxRetries` budget, including permanent 400/401/403/404-class failures (bad request, invalid API key, wrong model name) that will never succeed no matter how many times they're retried. This wastes the entire backoff budget on a call that was doomed from the first attempt, delays surfacing a real configuration error to the operator, and pollutes logs with misleading "retrying" noise for what is actually a permanent failure.

Fixing #1612 by adding a second, independent retry loop to the streaming path — without also fixing #1585 — would have reproduced the exact same blind-retry bug in a second place on day one. The two were designed and implemented together.

## Alternatives Considered

### Part A — Shared retry helper

**Alternative 1: Inline retry loop duplicated in `chatOrStream` (rejected).** Duplicates the entire backoff-loop implementation that already exists in `ChatWithParams`, and would additionally require duplicating the error-classification wiring from Part B in two places instead of one. `ChatWithParams` already has DescribeTable regression coverage (`chat_with_params_test.go`) proving the backoff-loop behavior; a from-scratch duplicate would need its own equivalent coverage with no shared benefit.

**Alternative 2 (chosen): extract a shared generic `llm.RetryWithBackoff[T any]` helper.** A generic helper (`AttemptResult[T]{Value, Err, SafeToRetry}` contract) used by both `ChatWithParams` and `chatOrStream`. Existing `chat_with_params_test.go` coverage acts as the regression safety net for refactoring `ChatWithParams` onto the shared helper — if the refactor changes behavior, those tests fail. Generics already have production precedent in this codebase (`pkg/cache/redis/cache.go`'s `Cache[T]`), so this isn't introducing a new pattern.

**Alternative 3: push retry into each `llm.Client` implementation (rejected).** Would require every current and future provider adapter to reimplement backoff/retry independently, and violates DD-HAPI-019 Framework Isolation by pushing a business-logic concern (how many times to retry, what counts as retryable) down into the provider-specific translation layer, which should only translate wire formats.

`SafeToRetry` is computed per call site as the AND of every concern that matters there:
- `ChatWithParams`: `SafeToRetry: llm.IsRetryable(err)` — `Chat` is atomic (no partial delivery), so error classification is the only concern.
- `chatOrStream`: `SafeToRetry: !eventAlreadySentThisAttempt && llm.IsRetryable(err)` — combines the #1612 side-effect concern (a stream callback has already forwarded output to the operator this attempt; retrying from scratch would duplicate/interleave events) with the #1585 classification concern.

### Part B — Error classification

**Constraint**: `pkg/kubernautagent/llm` is the framework-isolation boundary that `anthropicfamily`/`openai` depend *on* (DD-HAPI-019). It must never import a provider SDK or provider-specific error shape — that would invert the dependency and force the generic retry machinery to know about every current and future provider's error format.

**Alternative 1: centralize classification in `chat_helpers.go`, importing `anthropic-sdk-go` and `openaicompat` directly to type-switch on errors (rejected).** Violates DD-HAPI-019 directly.

**Alternative 2 (chosen): small provider-agnostic primitives in `pkg/kubernautagent/llm`, classification applied by each provider adapter at the boundary it already owns.**
- `IsNonRetryableHTTPStatus(code int) bool` — a blocklist (400, 401, 403, 404 are non-retryable; everything else, including 429 and 5xx, defaults retryable).
- `MarkNonRetryable(err error) error` — wraps an error so it's recognizable as permanent, preserving `Error()` text and `errors.Is`/`errors.As` unwrapping so it composes with an adapter's own `fmt.Errorf("...: %w", ...)` context wrap.
- `IsRetryable(err error) bool` — unwraps and checks; defaults to `true` for any error not explicitly marked, and `false` for a `nil` error (nothing to retry). The `true` default is a deliberate fail-safe: an unrecognized error shape is assumed transient rather than silently killing retries for a provider this classification logic doesn't yet understand.
- Each provider adapter classifies its *own* errors at the translation boundary it already owns: `anthropicfamily.Client.Chat`/`StreamChat` type-asserts the Anthropic SDK's `*anthropic.Error` (which has a public `StatusCode int`) via `errors.As`; `openai.Client.Chat`/`StreamChat` does the same for `*openaicompat.APIError` (see prerequisite below). The generic retry helper only ever calls `llm.IsRetryable(err)` — it has zero provider knowledge, exactly matching the existing `llm.Client` interface's abstraction boundary.

**Alternative 3: have `openaicompat`/`anthropicfamily` return a shared `llm`-level error type directly instead of wrap-at-boundary (rejected).** Marginal difference from Alternative 2, but `MarkNonRetryable(err)` composes better with the adapter's existing `fmt.Errorf("...: %w", err)` wrapping — it wraps the already-formatted error rather than replacing it, so the original message text always survives unchanged.

**Prerequisite discovered during preflight**: issue #1585 assumed the OpenAI-compatible client already exposed a typed/structured error. In fact `pkg/shared/llm/openaicompat/client.go`'s `do()` returned a bare `fmt.Errorf("openaicompat: API error (HTTP %d): %s", ...)` with no programmatically accessible status code — there is no `openai-go` SDK dependency in `go.mod`; `openaicompat` is a home-grown client. This DD adds:

```go
type APIError struct {
    StatusCode int
    Body       string
}
func (e *APIError) Error() string {
    return fmt.Sprintf("openaicompat: API error (HTTP %d): %s", e.StatusCode, e.Body)
}
```

`do()` now returns `&APIError{...}` instead of the bare `fmt.Errorf`. The `.Error()` string format is unchanged, so no existing caller that only inspects the message text is affected.

### Decision

**Alternative 2 for both parts.** Approved after preflight confirmed: (a) `anthropic.Error` (`internal/apierror.Error` aliased in `anthropic-sdk-go@v1.56.0`) genuinely has a public `StatusCode int`, already preserved through `anthropicfamily`'s existing `%w` wrapping; (b) `pkg/shared/transport/retry.go` is a different, intentionally allowlist-based retry policy for a different concern and must not be unified with this blocklist-based classification; (c) no import-cycle risk from adding the classification primitives to `pkg/kubernautagent/llm`.

## Design

### Shared retry helper

```go
// AttemptResult is the outcome of a single attempt.
type AttemptResult[T any] struct {
    Value       T
    Err         error
    SafeToRetry bool // ignored when Err is nil
}

func RetryWithBackoff[T any](ctx context.Context, maxAttempts int, bo backoff.Config,
    attempt func(attemptIndex int) AttemptResult[T]) (T, error)
```

`RetryWithBackoff` calls `attempt` up to `maxAttempts` times, sleeping with exponential backoff between attempts, respecting parent context cancellation during the sleep (returns `ctx.Err()` immediately if the context is done while waiting), and stopping early — without consuming the remaining budget — the moment an attempt reports `SafeToRetry: false`.

`ResolveRetryBackoff(params RuntimeParams) backoff.Config` and `ResolveMaxAttempts(params RuntimeParams) int` centralize the "use `params.RetryBackoff`/`params.MaxRetries` if set, else the package default" logic that both `ChatWithParams` and `chatOrStream` now share, instead of each duplicating the same default-resolution literal.

### Error classification primitives

```go
func IsNonRetryableHTTPStatus(code int) bool // 400/401/403/404 => true
func MarkNonRetryable(err error) error       // wraps err; nil => nil
func IsRetryable(err error) bool             // false only if MarkNonRetryable-wrapped or err==nil
```

### Cancellation-classification fix (#1612)

`callLLMTurn`'s check changed from:

```go
errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
```

to:

```go
errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil)
```

A literal `context.Canceled` is unconditionally treated as cancellation (matches existing test mocks that call the real `cancelFn` and return `ctx.Err()` verbatim). A `context.DeadlineExceeded` only counts as cancellation when the *outer* (parent) context passed into `callLLMTurn` is actually done — otherwise it falls through to the pre-existing generic-error path (`ResponseFailed` audit event, wrapped hard error return), which was already structurally distinguishable from `CancelledResult`. See [Addendum: gap remediation](#addendum-gap-remediation) below for the follow-up that also surfaces this path as a live `session.EventTypeError` SSE event.

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test Coverage |
|-----------|------------------------|-----------------------|----------------|
| `llm.RetryWithBackoff[T]` / `AttemptResult[T]` / `ResolveRetryBackoff` / `ResolveMaxAttempts` | `ChatWithParams` (callers: `chatOrStream`'s sink-nil fallback, `RunRCAExtractionFromConversation`) and `chatOrStream`'s streaming branch | `pkg/kubernautagent/llm/chat_helpers.go`, `internal/kubernautagent/investigator/investigator_loop.go` | `retry_backoff_1612_test.go`, `chat_with_params_test.go` (unmodified regression), `streaming_retry_1612_test.go` |
| `llm.IsRetryable` / `MarkNonRetryable` / `IsNonRetryableHTTPStatus` | Consumed by `ChatWithParams` and `chatOrStream`'s `AttemptResult.SafeToRetry`; populated by `anthropicfamily.Client.Chat/StreamChat` and `openai.Client.Chat/StreamChat` | `pkg/kubernautagent/llm/chat_helpers.go`, `pkg/kubernautagent/llm/anthropicfamily/client.go`, `pkg/kubernautagent/llm/openai/client.go` | `classify_1585_test.go`, `anthropicfamily/classify_1585_test.go`, `openai/classify_1585_test.go` |
| `openaicompat.APIError` | Returned from `Client.do()` on any non-200 response; consumed by `openai.Client`'s classification | `pkg/shared/llm/openaicompat/client.go` | `openaicompat/apierror_1585_test.go` |
| Cancellation classification fix | `callLLMTurn`, invoked on every LLM turn via `runLoopTurn` from `Investigate()`/`RunInteractiveTurn()` | `internal/kubernautagent/investigator/investigator_loop.go` | `streaming_retry_1612_test.go` (UT-KA-1612-006/007), pre-existing `cancel_test.go`/`token_streaming_test.go` (regression) |

## Consequences

### Positive
- The streaming LLM path (used by every real interactive/production investigation) now retries transient failures with backoff, matching the non-streaming path's behavior and honoring `RuntimeParams.MaxRetries`, which was previously dead on this path.
- Neither retry path wastes budget on a permanent 400/401/403/404-class failure — such errors surface immediately, with the original error message intact, instead of being retried for the full backoff budget and only then surfacing.
- A backend timeout on one attempt (child context deadline) is no longer misreported as an operator cancellation; it now correctly follows the generic-error/audit path.
- Streaming retries never duplicate or interleave already-emitted token-delta events: once any callback fires for an attempt, that attempt's failure is unconditionally non-retryable regardless of error classification.
- One shared, tested backoff-loop implementation instead of two independent ones.

### Negative
- Two provider adapter files (`anthropicfamily/client.go`, `openai/client.go`) now each carry a small classification function, in addition to the two originally-in-scope files (`chat_helpers.go`, `investigator_loop.go`) — a modest increase in surface area versus a #1612-only fix.
- `openaicompat.APIError` is a new exported type; any external code that pattern-matched on the old error's exact string via unusual means (none found in this codebase) would need to switch to `errors.As`, though the `.Error()` string itself is unchanged.

## Addendum: gap remediation

A post-implementation review of #1612 identified three scoped gaps, all closed in this addendum:

1. **Distinguishable "backend unavailable" outcome.** `callLLMTurn`'s generic (non-cancellation) LLM-failure path now also emits a `session.EventTypeError` SSE event (`Data: {"error": "<message>"}`) alongside the pre-existing `ResponseFailed` audit event. This is *not* a new wire-contract addition — `session.EventTypeError` was already fully specified (`internal/kubernautagent/session/types.go`) and consumed end-to-end by API Frontend's SSE bridge (`pkg/apifrontend/tools/ka_investigate_bridge.go`'s `FormatEventForUser`/`isStatusEvent`, with matching redaction and priority tests), but KA's investigator never actually produced it. This addendum wires the existing consumer contract to its missing producer, giving the operator a real-time signal instead of silence until the whole investigation fails.
2. **Retry-attempt telemetry.** A new `aiagent_llm_call_retries_total{phase,outcome}` counter (`internal/kubernautagent/metrics/metrics.go`, `RecordLLMCallRetry`) is incremented from `chatOrStream` whenever a streaming LLM call needed more than one attempt, labeled by final `outcome` (`succeeded`, `exhausted`, `non_retryable`). Scoped to the streaming path only, matching #1612; `ChatWithParams` (in `pkg/kubernautagent/llm`) does not have access to `internal/kubernautagent/metrics` without violating the `pkg/` → `internal/` layering direction, so it was left out of scope here.
3. **Operator-facing documentation.** `docs/services/kubernaut-agent/configuration-reference.md` §5's `maxRetries` row now states that retries apply uniformly to both the streaming and non-streaming call paths and documents the two early-exit conditions (non-retryable classification, streaming side-effect-safety).

### Wiring Manifest (addendum)

| Component | Production Entry Point | Wiring Code Location | Test Coverage |
|-----------|------------------------|-----------------------|----------------|
| `session.EventTypeError` emission on generic LLM turn failure | `callLLMTurn`, invoked on every LLM turn | `internal/kubernautagent/investigator/investigator_loop.go` | `streaming_retry_1612_test.go` (UT-KA-1612-006/009) |
| `metrics.LLMCallRetriesTotal` / `RecordLLMCallRetry` | `chatOrStream`, after `llm.RetryWithBackoff` returns | `internal/kubernautagent/metrics/metrics.go`, `internal/kubernautagent/investigator/investigator_loop.go` | `streaming_retry_1612_test.go` (UT-KA-1612-010) |

---

**Document Control**:
- **Created**: 2026-07-07
- **Version**: 1.1 (addendum: gap remediation)
- **Status**: Approved & Implemented
