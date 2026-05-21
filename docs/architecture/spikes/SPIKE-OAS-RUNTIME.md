# Spike: OAS Runtime with ACP Server

**Date**: May 20, 2026
**Status**: COMPLETED
**Duration**: 1 session
**Relates to**: PROPOSAL-EXT-003, [Agent Runtime Evaluation Plan](../../../.cursor/plans/agent_runtime_evaluation_fec263e8.plan.md)

---

## Objective

Validate that `open-agent-sdk-go` (v0.5.0) can serve as the execution engine for a thin Go binary ("OAS Runtime") that exposes an ACP-compliant HTTP/SSE server, replacing `goose-server` in Kubernaut's agent execution architecture.

## What Was Built

A complete proof-of-concept binary in `spikes/oas-runtime/` consisting of:

### 1. ACP Server (`internal/acp/`)

- **types.go**: Full ACP v0.2.0 type model transcribed from the OpenAPI 3.1.1 spec:
  - `AgentManifest`, `Run`, `RunCreateRequest`, `RunResumeRequest`
  - `Message`, `MessagePart`, `TrajectoryMetadata`
  - `Event` (11 SSE event types per ACP spec)
  - `RunMode` (sync/async/stream), `RunStatus` (7 states)

- **server.go**: HTTP server implementing 7 endpoints:
  - `GET /agents/{name}` -- agent manifest
  - `POST /runs` -- create run (sync/async/stream modes)
  - `GET /runs/{run_id}` -- get run status
  - `POST /runs/{run_id}` -- resume awaiting run
  - `DELETE /runs/{run_id}` -- cancel run
  - `GET /healthz`, `GET /readyz` -- probes

  Uses Go 1.22+ `http.ServeMux` with method-aware routing (no external router dependency). SSE streaming uses `text/event-stream` with typed events.

### 2. Runtime Executor (`internal/runtime/`)

- **agent.go**: Implements `acp.AgentExecutor` interface bridging ACP to `open-agent-sdk-go`:
  - `ExecuteSync()` -- blocking execution via `agent.Prompt()`
  - `ExecuteStream()` -- goroutine-based streaming via `agent.Query()`, converts `SDKMessage` events to ACP `Event` SSE stream
  - `Resume()` -- run state management for `awaiting` -> `in-progress` transitions
  - `Cancel()` -- context cancellation for in-flight runs
  - Hook wiring: `PermissionRequest` hook and `PreToolUse` audit hook

### 3. Main Binary (`main.go`)

- CLI flags for model, API key, base URL, MCP endpoints, max turns, port
- `--llm-endpoint` flag for `inference.local` zero-secret LLM access
- `--mcp-endpoints` comma-separated `name=url` format
- Graceful shutdown with SIGINT/SIGTERM handling
- JSON structured logging via `slog`

### 4. Dockerfile

- Multi-stage build (golang:1.23 builder -> ubuntu:24.04 runtime)
- OpenShell BYOC compatible: `sandbox` user (uid 1000), `iproute2` installed
- Supervisor-compatible: no CMD/ENTRYPOINT (OpenShell replaces with supervisor)

## Key Findings

### SDK API Surface Validation

| Capability | SDK API | Status |
|---|---|---|
| Agent creation | `agent.New(Options{})` | Works -- zero dependencies beyond Go stdlib + uuid |
| Blocking execution | `agent.Prompt(ctx, prompt)` -> `*QueryResult` | Works -- returns `Text`, `Usage`, `Cost`, `NumTurns`, `Duration` |
| Streaming execution | `agent.Query(ctx, prompt)` -> `<-chan SDKMessage, <-chan error` | Works -- yields `assistant`, `progress`, `result` message types |
| MCP server config | `Options.MCPServers` (map of `MCPServerConfig`) | Works -- HTTP, SSE, stdio transports supported |
| MCP init | `agent.Init(ctx)` | Required before first query to establish MCP connections |
| Hook system | `Options.Hooks` with `hooks.HookConfig` | Works -- 11 hook types, matcher-based routing |
| PermissionRequest hook | `hooks.HookRule{Matcher: "*", Hooks: []HookFn{...}}` | Works -- return empty string to allow, non-empty to deny |
| PreToolUse hook | Same pattern | Works -- fires before every tool invocation |
| Multi-provider | `Options.BaseURL` for OpenAI-compatible | Works -- auto-detects Anthropic vs OpenAI from URL/key/model |
| System prompt | `Options.SystemPrompt` | Works -- prepended to every conversation |
| Max turns | `Options.MaxTurns` | Works -- limits reasoning loop depth |
| Cost tracking | `QueryResult.Cost`, `agent.CostTracker()` | Built-in per-model tracking |
| Subagents | `Options.Agents` map | Available -- defines child agents with isolated tools/permissions |
| Custom tools | `Options.CustomTools` (implements `types.Tool`) | Available -- full interface for custom tool implementation |

### Streaming Event Model

The SDK's `SDKMessage` type has a `Type` field with values:
- `assistant` -- contains `Message` with `ContentBlock` array
- `progress` -- intermediate text updates
- `result` -- final result with `Text`, `Usage`, `Cost`

Tool calls and results are embedded as `ContentBlock` entries within `assistant` messages:
- `ContentBlockToolUse` -- tool name + input
- `ContentBlockToolResult` -- tool output + error flag
- `ContentBlockText` -- LLM text response
- `ContentBlockThinking` -- extended thinking content

This maps cleanly to ACP's `message.part` events with `TrajectoryMetadata`.

### Binary Size and Dependencies

| Metric | Value |
|---|---|
| Binary size (darwin/arm64) | 10 MB |
| Go dependencies | 1 (google/uuid) |
| Build time | ~1.2 seconds |

The SDK is genuinely lightweight. Compare to goose-server (Rust binary, ~50-100 MB).

### ACP Mapping Validation

| ACP Concept | Implementation | Notes |
|---|---|---|
| `POST /runs` (stream mode) | `ExecuteStream()` -> goroutine + channel -> SSE | Clean mapping |
| `POST /runs` (sync mode) | `ExecuteSync()` -> `agent.Prompt()` | Direct |
| `POST /runs` (async mode) | Background goroutine + status polling | Works via `GET /runs/{id}` |
| `POST /runs/{id}` (resume) | State machine: `awaiting` -> `in-progress` | State tracking in memory; production needs persistence |
| `DELETE /runs/{id}` (cancel) | Context cancellation | Clean teardown |
| `run.awaiting` state | Stub ready; needs SDK hook-to-ACP wiring | See Open Items |
| SSE event types | All 11 types defined; `message.part` and run lifecycle events implemented | Complete |
| `TrajectoryMetadata` | Populated from `ContentBlockToolUse`/`ContentBlockToolResult` | Structured audit trail |

### Hook-to-ACP Wiring for HITL

The `PermissionRequest` hook fires before tool execution and blocks. To bridge to ACP's `awaiting` state:

1. Hook fires -> runtime transitions run to `awaiting` with `AwaitRequest` containing tool name and input
2. ACP server emits `run.awaiting` SSE event to KA
3. KA evaluates permission ceiling, calls `POST /runs/{id}` to resume
4. Hook returns (allow or deny) based on the resume payload

This requires a synchronization primitive (channel or condition variable) between the hook goroutine and the ACP resume handler. Not implemented in this spike but architecturally straightforward.

## Open Items (For Production Implementation)

1. **Hook-to-ACP awaiting bridge**: Wire `PermissionRequest` hook to ACP `awaiting` state with channel-based synchronization between the hook goroutine and the HTTP resume handler.

2. **Run state persistence**: The spike stores runs in memory. Production needs durable state (or accept that the OAS Runtime is ephemeral and KA owns all state).

3. **Output schema enforcement**: The SDK does not enforce output schemas natively. KA validates output post-run, which matches the current proposal.

4. **Concurrent runs**: The spike supports concurrent runs via separate agent instances. Production may want a run pool with resource limits.

5. **Error taxonomy mapping**: ACP errors map cleanly but need extension for Kubernaut-specific categories (budget exceeded, infrastructure failure). Consider custom error codes in the `Error.data` field.

## Conclusion

**Validated**: The `open-agent-sdk-go` SDK provides everything needed for the OAS Runtime binary:
- Streaming agentic loop with tool execution
- MCP client with HTTP/SSE/stdio transports
- Hook system that maps to ACP's `awaiting` state for HITL
- Multi-provider LLM support (Anthropic + OpenAI-compatible)
- Cost tracking and token usage reporting

The ACP server is a thin HTTP layer (~250 lines) on top of the SDK. The total binary is 10MB with a single external dependency. This validates the architecture: **OAS Runtime + ACP + OpenShell is a viable replacement for goose-server**.

## File Inventory

```
spikes/oas-runtime/
  go.mod, go.sum              -- Isolated Go module (does not pollute main project)
  main.go                     -- Entry point with CLI flags
  Dockerfile                  -- OpenShell BYOC compatible image
  internal/
    acp/
      types.go                -- ACP v0.2.0 types from OpenAPI spec
      server.go               -- ACP HTTP server (7 endpoints)
    runtime/
      agent.go                -- AgentExecutor bridging ACP <-> open-agent-sdk-go
```
