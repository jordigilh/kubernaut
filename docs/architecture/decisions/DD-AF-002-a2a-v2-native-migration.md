# DD-AF-002: Deferred Migration to A2A-Go v2 Native Executor

**Status**: DEFERRED
**Decision Date**: 2026-06-05
**Version**: 1.0
**Confidence**: 92%
**Applies To**: API Frontend (AF) -- A2A Streaming Stack
**Gate Condition**: Kagenti v1.0 with A2A protocol v2 wire format support

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-05 | Architecture Team | Initial decision: defer v2 migration, document scope and design. |

---

## Context & Problem

### Current State

AF uses the **deprecated** `google.golang.org/adk/server/adka2a` package (the v0 shim)
to expose ADK agents via the A2A protocol. This shim:

1. Accepts v0 types (`github.com/a2aproject/a2a-go/a2a`) from our callbacks
2. Internally delegates to the v2 executor (`google.golang.org/adk/server/adka2a/v2`)
3. Converts every event and part between v0 and v2 via `a2acompat/a2av0`

The v2 native package (`google.golang.org/adk/server/adka2a/v2`) uses
`github.com/a2aproject/a2a-go/v2/a2a` types directly, eliminating the conversion layer.

### Root Cause of the `{"value": {...}}` Bug

The shim's `GenAIPartConverter` wrapper calls `a2av0.ToV1Part(legacyPart)` to convert
our returned `a2a.Part` to the v2 `*a2a.Part` struct. `ToV1Part` uses a Go type switch:

```go
switch c := p.(type) {
case a2alegacy.TextPart:   // matches VALUE type only
    return &a2a.Part{Content: a2a.Text(c.Text), ...}
default:
    return &a2a.Part{Content: a2a.Data{Value: c}, ...}  // wraps unknown types
}
```

Our code returned `*a2a.TextPart` (pointer). The type switch matched the value type
`a2alegacy.TextPart`, not the pointer, so it fell through to `default` and wrapped the
entire struct as `a2a.Data{Value: c}`. When serialized, this produced
`{"value": {"kind": "text", "text": "..."}}`.

**Fix applied**: Changed all `&a2a.TextPart{...}` to `a2a.TextPart{...}` (value type)
and all `*a2a.TextPart` type assertions to `a2a.TextPart`. See commit `2e5f803e6`.

---

## Decision

**Defer migration to v2 native** until Kagenti supports A2A protocol v2 wire format.

### Why Not Now

Migrating to v2 native changes the SSE wire format in ways that break Kagenti v0.6.0:

| Concept | v0 Wire Value | v2 Wire Value | Kagenti v0.6.0 Expects |
|---------|--------------|---------------|------------------------|
| Task state (completed) | `"completed"` | `"TASK_STATE_COMPLETED"` | `"COMPLETED"` |
| Task state (failed) | `"failed"` | `"TASK_STATE_FAILED"` | `"FAILED"` |
| Task state (working) | `"working"` | `"TASK_STATE_WORKING"` | `"WORKING"` |
| Message role (agent) | `"agent"` | `"ROLE_AGENT"` | N/A |
| Protocol version | `"0.3.0"` | `"1.0"` | N/A |
| `TaskStatusUpdateEvent.Final` | `true`/`false` | **removed** | `result.get("final", False)` |

Kagenti v0.6.0's `_stream_from_response` uses `is_final = result.get("final", False)`
as the primary guard for terminal state detection. The state string checks
(`state in ["COMPLETED", "FAILED"]`) do not match either v0 or v2 format but are
compensated by the `is_final` fallback. With v2, `final` is removed entirely, breaking
terminal state detection.

Migrating to v2 while supporting Kagenti v0.6.0 would require building a custom
compat layer on the SSE output -- replacing one shim with another.

### Gate Condition

Proceed with v2 migration when **all** of these are met:

1. Kagenti v1.0 is released with A2A protocol v2 support
2. Kagenti v1.0 handles `TASK_STATE_*` prefixed states
3. Kagenti v1.0 infers terminality from state (not `final` field)
4. All other A2A clients in the ecosystem are v2-compatible

---

## Migration Scope (Future Reference)

### Production Files (6)

| File | Key Changes |
|------|-------------|
| `pkg/apifrontend/launcher/launcher.go` | `adka2a` -> `adka2a/v2`, `a2asrv.RequestContext` -> `a2asrv.ExecutorContext`, callback signatures, handler options |
| `pkg/apifrontend/launcher/part_converter.go` | `GenAIPartConverter` returns `*a2a.Part` (struct) instead of `a2a.Part` (interface); `a2a.TextPart{...}` -> `a2a.NewTextPart(...)` |
| `pkg/apifrontend/launcher/event_bridge.go` | `eventqueue.Queue.Write(ctx, event)` -> `eventqueue.Writer.Write(ctx, *Message)`; `a2a.TextPart` -> `a2a.NewTextPart()` |
| `pkg/apifrontend/launcher/streaming_executor.go` | `Execute(ctx, *RequestContext, Queue) error` -> `Execute(ctx, *ExecutorContext) iter.Seq2[Event, error]` |
| `pkg/apifrontend/launcher/session_interceptor.go` | `CallInterceptor.Before` gains early-return (`any`); `MessageSendParams` -> `SendMessageRequest` |
| `pkg/apifrontend/handler/agentcard.go` | `a2a.Version` changes from `"0.3.0"` to `"1.0"` |

### Test Files (9)

`event_bridge_test.go`, `part_converter_test.go`, `streaming_executor_test.go`,
`session_interceptor_test.go`, `audit_emission_test.go`, `export_test.go`,
`test_helpers_test.go`, `ka_investigate_mcp_test.go`, `testhelpers_test.go`

### Key Design Decision: StreamingExecutor + EventBridge

The v2 `AgentExecutor.Execute` returns `iter.Seq2[a2a.Event, error]` instead of
accepting an `eventqueue.Queue`. The server drains the iterator internally via
an `eventpipe.Writer`.

The `EventBridge` currently writes side-channel events (keepalives, reasoning,
status updates) directly to the queue. With v2, the queue is not exposed to the
executor. Options:

1. **Channel-merge pattern**: Run the inner executor's iterator in a goroutine
   sending to a channel; `EventBridge` also sends to the same channel;
   `StreamingExecutor.Execute` yields from the merged channel.
2. **ExecutorContextInterceptor**: Use the v2 `WithExecutorContextInterceptor`
   hook to inject the bridge into context and use a shared writer registry.
3. **AfterEventCallback enrichment**: Emit bridge events as additional yields
   after each ADK event via the callback chain.

Recommended: Option 1 (channel-merge) for simplicity and testability.

### Import Path Changes

| v0 | v2 |
|----|-----|
| `github.com/a2aproject/a2a-go/a2a` | `github.com/a2aproject/a2a-go/v2/a2a` |
| `github.com/a2aproject/a2a-go/a2asrv` | `github.com/a2aproject/a2a-go/v2/a2asrv` |
| `github.com/a2aproject/a2a-go/a2asrv/eventqueue` | `github.com/a2aproject/a2a-go/v2/a2asrv/eventqueue` |
| `google.golang.org/adk/server/adka2a` | `google.golang.org/adk/server/adka2a/v2` |

### go.mod Changes

- Promote `github.com/a2aproject/a2a-go/v2 v2.3.1` from indirect to direct
- Remove `github.com/a2aproject/a2a-go v0.3.15` once all imports are migrated

---

## Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Wire format breaks existing clients | High | Gate on Kagenti v1.0; verify all clients before migration |
| StreamingExecutor rewrite introduces concurrency bugs | Medium | Channel-merge pattern with comprehensive race-detector testing |
| v0 shim introduces new conversion bugs | Low | Value-type TextPart fix prevents the known class of bugs; monitor for others |

---

## References

- ADK v1.4.0 deprecation notice: `google.golang.org/adk/server/adka2a/executor.go` line 17
- a2a-go v2 core types: `github.com/a2aproject/a2a-go/v2@v2.3.1/a2a/core.go`
- a2av0 compat layer: `github.com/a2aproject/a2a-go/v2@v2.3.1/a2acompat/a2av0/conversions.go`
- Kagenti upstream: `github.com/kagenti/kagenti/kagenti/backend/app/routers/chat.py`
- TextPart fix commit: `2e5f803e6`
