# DD-CONV-001: Conversation Tool-Call Architecture

**Date**: 2026-03-04
**Status**: Approved
**Parent ADR**: [ADR-034: Unified Audit Table Design](./ADR-034-unified-audit-table-design.md)
**Business Requirement**: #592 (Conversational API Backend)
**Deciders**: Architecture Team

---

## Context

The Kubernaut Agent conversation API (#592) currently operates as a text-only chat interface. When a user discusses a RemediationApprovalRequest (RAR), the LLM cannot verify pod state, check logs, or look up workflows during conversation turns.

**Problems identified**:

1. **No tool execution in conversation mode** — Guardrails infrastructure (`readOnlyTools`, `ValidateToolCall`) exists but has no runtime consumer. The LLM can only generate text responses.
2. **Global `todo_write` singleton** — The `investigation.NewTodoWriteTool()` is registered once in the global registry, causing cross-session and cross-investigation state leakage.
3. **No conversation memory** — All LLM APIs (OpenAI, Anthropic, etc.) are stateless. LangChainGo is used as a thin, stateless model driver (`GenerateContent` per call). Without caller-managed history, each user message feels like a new conversation.
4. **User-driven interaction** — KA must be a transparent mediator, not an autonomous agent. The user drives the conversation; the LLM assists with read-only tool access.

---

## Decision 1: Tool-Call Loop in LLMAdapter

`LLMAdapter.Respond` runs a bounded multi-turn loop, following the same pattern as the investigator `runLLMLoop` (`internal/kubernautagent/investigator/investigator.go:341`) but conversation-scoped.

**Loop structure**:

1. Build `[]llm.ToolDefinition` from `session.Guardrails.FilterTools(registry.All())` + session `todoWrite` (each via `toolToDefinition()` helper using `Name()`/`Description()`/`Parameters()`)
2. `llm.Client.Chat(ctx, ChatRequest{Messages, Tools})` per iteration
3. If `resp.ToolCalls` returned: for each `llm.ToolCall`, convert `tc.Arguments` (string) to `json.RawMessage`, validate via `session.Guardrails.ValidateToolCall`, execute via `registry.Execute` or `session.todoWrite.Execute`, emit SSE events, append `role: "tool"` result with `ToolCallID`, loop
4. If no `ToolCalls`: return final text via SSE `message` event

**Turn limit**: `maxToolTurns` defaults to same value as `InvestigatorConfig.MaxTurns` (15), configurable via `ConversationConfig.MaxToolTurns`. Existing per-user and per-session rate limiters control message volume (not tool depth).

---

## Decision 2: ConversationLLM Interface with Streaming Callback

```go
type ConversationEvent struct {
    Type string          // "tool_call", "tool_result", "tool_error", "message", "error"
    Data json.RawMessage
}

type ConversationLLM interface {
    Respond(ctx context.Context, sessionID, message string, emit func(ConversationEvent)) error
}
```

- `emit` callback streams events to the handler, which writes them as SSE frames
- `defaultLLM` emits one `message` event (for tests that don't need tools)
- `failingLLM` returns an error (existing test pattern preserved)

---

## Decision 3: Cross-Message Conversation History

All LLM APIs are stateless — there are no server-side sessions. Conversation history is 100% caller-owned.

- `Session.Messages []llm.Message` stores all user, assistant, and tool messages across turns
- `Session.mu sync.RWMutex` protects `Messages` (design-first for future concurrency)
- `GetMessages()` returns a copy under read lock; `AppendMessages(...)` appends under write lock
- Each `Respond` call builds: `[system_prompt] + session.GetMessages() + [new_user_msg]`, runs tool loop, then persists new messages (user + tool exchanges + assistant) back via `session.AppendMessages`
- System prompt is rebuilt from template each turn (not stored) to reflect runtime state changes
- Memory bounded by `maxConversationTurns` (~15 turns x ~4KB avg = ~60KB per session, evicted on TTL)
- On session reconnect after eviction: reconstruct from audit trail (deferred to post-v1.4)

---

## Decision 4: Per-Session `todo_write`

- `SessionManager.Create` calls `investigation.NewTodoWriteTool()` per session
- Session struct gets `todoWrite tools.Tool` field (implements `Name()`, `Description()`, `Parameters()`, `Execute()`)
- Adapter routes `todo_write` calls to `session.todoWrite.Execute(ctx, json.RawMessage(tc.Arguments))` (not global registry)
- Each `todo_write` call audited as `aiagent.llm.tool_call` with full args/result
- On session reconnect: reconstruct todo state from audit events via `AuditChainFetcher` pattern (replay `tool_call` events where `tool_name == "todo_write"`)

---

## Decision 5: SSE Event Types

| SSE Event | Payload | When |
|-----------|---------|------|
| `tool_call` | `{"name": "...", "args": {...}}` | LLM requests a tool |
| `tool_result` | `{"name": "...", "result": "..."}` | Tool execution complete |
| `tool_error` | `{"name": "...", "error": "..."}` | Guardrail rejection or execution error |
| `message` | `{"content": "..."}` | LLM's final text response |
| `error` | `{"error": "..."}` | Fatal error (ends turn) |

---

## Decision 6: Guardrails Runtime Enforcement

- **Per-turn**: adapter builds `toolDefs` from `readOnlyTools` + session `todo_write`
- **Per-call**: `session.Guardrails.ValidateToolCall(name string, args map[string]interface{})` before every execution
- Namespace scoping enforced on all namespaced tool args
- Rejected tool calls: error result appended to messages (LLM sees rejection reason), `tool_error` SSE event emitted

---

## Decision 7: Audit Parity (Using Existing Infrastructure)

Uses existing `audit.AuditStore` interface, `audit.NewEvent()`, `audit.StoreBestEffort()` — same helpers as investigator `runLLMLoop`. No new abstraction needed.

- Every `Chat` call: `aiagent.llm.request` + `aiagent.llm.response` (same event constants as investigator)
- Every tool execution: `aiagent.llm.tool_call` (same schema as investigator)
- End of turn: `aiagent.conversation.turn` summary event
- All keyed by `session.CorrelationID`
- Full forensic reconstruction possible from audit trail
- Test: `capturingAuditStore` in integration tests already satisfies `audit.AuditStore`

---

## Consequences

**Positive**:
- Conversation mode becomes a full tool-augmented assistant. Users can ask the LLM to investigate, verify, and plan.
- Guardrails infrastructure gets a runtime consumer (was dead code before).
- `todo_write` becomes a meaningful per-session planning tool.
- Full audit parity with investigator mode enables forensic reconstruction.

**Negative**:
- `ConversationLLM` interface change requires updating all test mocks (`defaultLLM`, `failingLLM`, integration test `failingLLM`).
- Per-session `todo_write` increases memory per session (mitigated by TTL eviction).

**Risks**:
- Phase 5 implementation complexity. Mitigated by decomposition into 4 sub-phases (5A-5D) and the proven investigator `runLLMLoop` pattern.

---

## Cross-references

- [ADR-034: Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) — audit event types/categories
- [DD-AUDIT-003: Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md) — service audit requirements
- [DD-HAPI-019](./DD-HAPI-019-go-rewrite-design/DD-HAPI-019-go-rewrite-design.md) — investigator tool-call loop pattern
- #592 — conversational API backend
- #594 — workflow override

## Implementation pointers

- **Tool-call loop**: [`internal/kubernautagent/conversation/llm_adapter.go`](../../../internal/kubernautagent/conversation/llm_adapter.go) — `LLMAdapter.Respond()`
- **SSE streaming**: [`internal/kubernautagent/conversation/handler.go`](../../../internal/kubernautagent/conversation/handler.go) — `HandlePostMessage()`
- **Guardrails**: [`internal/kubernautagent/conversation/guardrails.go`](../../../internal/kubernautagent/conversation/guardrails.go) — `FilterTools()`, `ValidateToolCall()`
- **Session management**: [`internal/kubernautagent/conversation/session.go`](../../../internal/kubernautagent/conversation/session.go) — `SessionManager`
- **Conversation prompt**: [`internal/kubernautagent/prompt/templates/conversation.tmpl`](../../../internal/kubernautagent/prompt/templates/conversation.tmpl)
- **Audit payload**: [`api/openapi/data-storage-v1.yaml`](../../../api/openapi/data-storage-v1.yaml) — `ConversationTurnPayload` schema
