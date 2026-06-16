# A2A Streaming Protocol: Hybrid Event Model

**Version**: 1.2
**Last Updated**: 2026-06-13
**Status**: Active
**Applies to**: API Frontend (AF) A2A streaming endpoint (`/a2a/invoke`)

---

## Overview

The Kubernaut API Frontend uses a **hybrid event model** for A2A streaming:

- **`TaskArtifactUpdateEvent`** carries structured results (DataPart + TextPart) for
  rich payloads like investigation decisions and workflow cards.
- **`TaskStatusUpdateEvent`** carries ephemeral progress: orchestration status,
  reasoning deltas, investigation events, execution progress, and keepalives.
  Each event includes a `metadata.type` field for semantic classification.
- **Text routing** uses event-aware logic to route intermediate LLM text to the
  reasoning channel (ThinkingPanel) while only emitting definitive responses as
  user-facing artifacts.

This hybrid approach ensures compatibility with all A2A clients:
- **Standard clients** (e.g., Kagenti TUI/backend) render artifacts as the main
  response and status events as streaming progress — no changes needed.
- **Metadata-aware clients** can inspect `metadata.type` on status events to
  provide Claude Code/Cursor-style UX (collapsible reasoning, ephemeral status).

---

## Wire Format

### LLM Output: `TaskArtifactUpdateEvent`

The LLM's final response text is delivered as standard A2A artifact events.
The ADK executor manages the `lastChunk` lifecycle automatically.

```json
{
  "kind": "artifact-update",
  "taskId": "...",
  "contextId": "...",
  "artifact": {
    "artifactId": "...",
    "parts": [
      { "kind": "text", "text": "Here's a snapshot of `demo-mesh`:\n\n### Resources\n..." }
    ]
  },
  "lastChunk": false
}
```

### Structured Artifacts: Multi-Part DataPart + TextPart (#1399, #1403)

Structured tool results (investigation decisions, workflow cards, execution progress)
use multi-part artifacts with both machine-readable and human-readable content:

```json
{
  "kind": "artifact-update",
  "taskId": "...",
  "contextId": "...",
  "artifact": {
    "artifactId": "...",
    "parts": [
      {
        "kind": "data",
        "data": { "summary": "Pod OOMKilled", "options": [...] },
        "metadata": { "mediaType": "application/json" }
      },
      { "kind": "text", "text": "Decision: Pod OOMKilled\n\n" }
    ],
    "metadata": {
      "type": "decision",
      "schema": "investigation_summary",
      "schema_version": "1.0"
    }
  },
  "lastChunk": true
}
```

**Design rationale**: The DataPart carries structured JSON for machine parsing (Console
renders rich cards). The TextPart carries a human-readable fallback for standard A2A
clients that only handle text. The `metadata.schema` field enables schema-based routing
without content inspection (#1411).

### Progress Events: `TaskStatusUpdateEvent` with `metadata.type`

All ephemeral/progress content uses `TaskStatusUpdateEvent`:

```json
{
  "kind": "status-update",
  "taskId": "...",
  "contextId": "...",
  "status": {
    "state": "working",
    "timestamp": "2026-06-05T12:00:00Z",
    "message": {
      "role": "agent",
      "parts": [
        { "kind": "text", "text": "..." }
      ]
    }
  },
  "metadata": {
    "type": "<event-type>"
  }
}
```

---

## Event Types

### Artifact Events

| Event Kind | Payload | Description | Rendering Guidance |
|------|---------|-------------|--------------------|
| `artifact-update` (text-only) | TextPart | LLM final response (markdown) | Full markdown render; accumulate until `lastChunk: true` |
| `artifact-update` (multi-part) | DataPart + TextPart | Structured decision/workflow card | Parse DataPart for rich card; fall back to TextPart |

### Status Events (`metadata.type`)

| Type | Description | Rendering Guidance |
|------|-------------|--------------------|
| `reasoning` | LLM inner thoughts, investigation reasoning deltas | Dimmed/collapsible, like Claude Code's "thinking" |
| `status` | Orchestration progress ("Analyzing...", tool call summaries) | Ephemeral/italic, substatus indicator |
| `output` | Execution progress (JSON steps from kubernaut_watch) | Progress bar or step list |
| `decision` | Legacy structured decision (pre-#1399 path, StatusUpdateEvent) | Rich workflow card |
| `investigation` | KA investigation events (tool calls, completions, errors) | Ephemeral, may include structured data |
| `keepalive` | Idle timeout prevention (no `status.message`) | Spinner/pulse indicator, or ignore |
| `approval_request` | RAR created, awaiting user action | Approval card with Approve/Reject buttons |
| `approval_request_resolved` | RAR approved/rejected | Status update (green/red indicator) |

---

## Event Routing Pipeline

### Part Converter Architecture

The `buildStreamingPartConverter()` function routes GenAI parts through the following logic:

```
GenAI Part
├── FunctionResponse + name == "kubernaut_investigate"
│   └── SUPPRESS (reasoning already streamed progressively via EventBridge)
├── FunctionResponse + name == "kubernaut_present_decision"
│   └── SUPPRESS (emitted as ArtifactUpdateEvent via emitDecisionEvent)
├── FunctionResponse + outputMetaTools["kubernaut_watch"]
│   └── emitStructuredOutput → StatusUpdate(type="output", JSON steps)
├── FunctionCall + name == "kubernaut_present_decision"
│   └── emitDecisionEvent → ArtifactUpdateEvent(DataPart + TextPart)
├── FunctionCall (other tools)
│   └── EmitReasoning → StatusUpdate(type="reasoning")
├── FunctionResponse (other tools with summarizer)
│   └── EmitReasoning or EmitStructuredMeta → StatusUpdate
├── Thought
│   └── EmitReasoning → StatusUpdate(type="reasoning")
├── Text + shouldRouteTextToReasoning(event)
│   └── EmitReasoning → StatusUpdate(type="reasoning")
└── Text (final, definitive)
    └── Return as TextPart artifact (emoji-stripped)
```

### FunctionResponse Suppression (#1408)

The `kubernaut_investigate` FunctionResponse is suppressed in streaming mode because
the EventBridge has already delivered the investigation results progressively as
reasoning events. Without suppression, the user would see duplicate content: once
during streaming and again when the FunctionResponse is converted to a text part.

Similarly, `kubernaut_present_decision` FunctionResponse is suppressed because the
decision is emitted as a structured ArtifactUpdateEvent at FunctionCall time (the
response merely confirms `{presented: true}`).

### Event-Aware Text Routing (#1410)

`shouldRouteTextToReasoning(event)` determines whether plain text should go to the
reasoning channel (ThinkingPanel) instead of becoming a user-facing artifact:

```go
func shouldRouteTextToReasoning(event *session.Event) bool {
    if event == nil { return false }
    if event.Partial { return true }       // Streaming intermediate chunk
    return eventHasFunctionCall(event)     // Preamble narration before tool call
}
```

**Rationale**: LLMs produce intermediate text ("Let me check the pod status...")
before calling tools. Without this routing, that narration becomes a visible artifact.
By routing to reasoning when the event is partial or contains a FunctionCall, only
the final definitive response becomes the user-facing artifact.

### Metadata-Only Schema Identification (#1411)

Artifact metadata carries a `schema` field that identifies the data contract without
requiring content inspection:

```json
"metadata": {
  "type": "decision",
  "schema": "investigation_summary",
  "schema_version": "1.0"
}
```

This enables:
- Forward-compatible rendering: clients route artifacts to UI components by schema name
- No content parsing needed for type detection
- Schema evolution tracked by `schema_version`
- JSON Schema validation against `pkg/apifrontend/launcher/schemas/{schema}.v{version}.schema.json`

### Execution Progress Artifacts (#1403)

The `kubernaut_watch` FunctionResponse emits structured execution progress:

```json
{
  "steps": [
    { "id": "s1", "label": "Deploying fix", "state": "done" },
    { "id": "s2", "label": "Validating rollout", "state": "running" }
  ],
  "completed": false
}
```

This is emitted via `EmitStatusWithMeta(type="output")` so enhanced clients can
render a progress stepper while standard clients show it as text.

---

## Event Examples

### Multi-Part Artifact (Structured Decision)

```json
{
  "kind": "artifact-update",
  "taskId": "...",
  "artifact": {
    "artifactId": "...",
    "parts": [
      {
        "kind": "data",
        "data": {
          "summary": "Pod crash-looping due to OOMKill",
          "options": [
            { "id": "restart", "name": "Rolling restart", "confidence": 0.85 },
            { "id": "scale-up", "name": "Increase memory limits", "confidence": 0.72 }
          ]
        },
        "metadata": { "mediaType": "application/json" }
      },
      { "kind": "text", "text": "Decision: Pod crash-looping due to OOMKill\n\n" }
    ],
    "metadata": { "type": "decision", "schema": "investigation_summary", "schema_version": "1.0" }
  },
  "lastChunk": true
}
```

### Reasoning Event (LLM inner thought)

```json
{
  "kind": "status-update",
  "status": {
    "state": "working",
    "timestamp": "2026-06-05T12:00:01Z",
    "message": {
      "role": "agent",
      "parts": [{"kind": "text", "text": "The pod is in CrashLoopBackOff, checking OOMKill events..."}]
    }
  },
  "metadata": {"type": "reasoning"}
}
```

### Execution Progress Event

```json
{
  "kind": "status-update",
  "status": {
    "state": "working",
    "timestamp": "2026-06-05T12:00:05Z",
    "message": {
      "role": "agent",
      "parts": [{"kind": "text", "text": "{\"steps\":[{\"id\":\"s1\",\"label\":\"Deploying fix\",\"state\":\"done\"},{\"id\":\"s2\",\"label\":\"Validating\",\"state\":\"running\"}],\"completed\":false}"}]
    }
  },
  "metadata": {"type": "output"}
}
```

### Keepalive Event (metadata-only, no message)

```json
{
  "kind": "status-update",
  "status": {
    "state": "working",
    "timestamp": "2026-06-05T12:00:10Z"
  },
  "metadata": {"type": "keepalive", "dot": "."}
}
```

---

## Client Integration Guide

### Standard A2A Client (No Metadata Awareness)

Any A2A client that handles artifacts and status events works out of the box:

- **Artifacts** (`artifact-update`): Accumulate text parts as the main response.
  On `lastChunk: true`, finalize the message. Multi-part artifacts: use the TextPart.
- **Status events** (`status-update`): Show `status.message` text as streaming
  progress. Discard when the stream ends.

```javascript
for await (const event of a2aStream) {
  if (event.kind === "artifact-update") {
    const text = event.artifact.parts
      .filter(p => p.kind === "text")
      .map(p => p.text)
      .join("");
    appendToResponse(text);
    if (event.lastChunk) finalizeResponse();
  }
  if (event.kind === "status-update" && event.status?.message) {
    const text = event.status.message.parts
      .filter(p => p.kind === "text")
      .map(p => p.text)
      .join("");
    showProgress(text);
  }
}
```

### Enhanced Client (Metadata-Aware)

A metadata-aware client inspects `metadata.type` on status events and artifact
metadata for schema-based rendering:

```typescript
for await (const event of a2aStream) {
  if (event.kind === "artifact-update") {
    const schema = event.artifact?.metadata?.schema;
    if (schema) {
      // Structured artifact — use DataPart for rich rendering
      const dataPart = event.artifact.parts.find(p => p.kind === "data");
      renderStructuredCard(schema, dataPart?.data);
    } else {
      // Plain text artifact
      appendToResponse(extractArtifactText(event));
    }
    if (event.lastChunk) finalizeResponse();
    continue;
  }

  if (event.kind === "status-update") {
    if (event.metadata?.type === "keepalive") {
      showSpinner();
      continue;
    }
    if (!event.status?.message) continue;

    const text = extractText(event.status.message);
    const type = event.metadata?.type ?? "status";

    switch (type) {
      case "reasoning":
        appendToCollapsible("Thinking...", text);
        break;
      case "output":
        renderProgressStepper(JSON.parse(text));
        break;
      case "decision":
        renderDecisionCard(JSON.parse(text));
        break;
      case "approval_request":
        showApprovalCard(JSON.parse(text));
        break;
      case "approval_request_resolved":
        updateApprovalStatus(JSON.parse(text));
        break;
      case "status":
      case "investigation":
        appendSubstatus(text);
        break;
      default:
        appendSubstatus(text);
        break;
    }
  }
}
```

---

## Design Decisions

### Why a Hybrid Model?

The hybrid approach balances two requirements:

1. **Standard A2A client compatibility**: Clients like Kagenti render
   `TaskArtifactUpdateEvent` as the main chat response and
   `TaskStatusUpdateEvent` as ephemeral progress. Sending all content via
   status events caused Kagenti to show "No response from agent" because its
   backend only forwards status `content` on terminal states (`final`/`COMPLETED`).

2. **Rich semantic classification**: The `metadata.type` field on status events
   enables enhanced clients to provide Claude Code/Cursor-style UX with
   collapsible reasoning and ephemeral substatus — without breaking clients
   that ignore metadata.

### Why Multi-Part Artifacts? (#1399)

Using DataPart + TextPart in artifacts provides:

1. **Machine-readable**: DataPart carries structured JSON for rich card rendering
2. **Human-readable fallback**: TextPart ensures standard clients still display something
3. **A2A spec compliant**: DataPart is a standard part kind in A2A v1.0
4. **No protocol violations**: No custom event types or non-standard fields

### Why FunctionResponse Suppression? (#1408)

The `kubernaut_investigate` tool delivers results progressively via the EventBridge
during execution. Without suppression:
- User sees investigation results twice (progressive + final FunctionResponse)
- The "no narration" prompt directive reduces LLM commentary, making the
  FunctionResponse the only duplicate content source

### Why Event-Aware Text Routing? (#1410)

LLMs produce intermediate text before tool calls ("Let me check..."). Without
routing, this narration becomes a visible artifact alongside the final answer:
- `event.Partial`: Streaming chunk, not yet a complete thought
- `eventHasFunctionCall(event)`: Text is preamble to tool invocation, not a response

Both are routed to reasoning (ThinkingPanel) for enhanced clients.

### Why Metadata-Only Schema Identification? (#1411)

Without a schema field, clients must parse artifact content to determine its type:
- Fragile: content shape can change between versions
- Expensive: JSON parsing for type detection on every artifact
- Ambiguous: multiple schemas might share similar structures

`metadata.schema` + `schema_version` enables declarative routing without content inspection.

### Kagenti Backend Behavior (Reference)

The Kagenti backend (`backend/app/routers/chat.py`) translates A2A SSE events:

- **Artifacts**: Always extracts text parts and forwards as `content` to the TUI
- **Status events**: Only forwards `content` when `is_final=True` or
  `state` is `COMPLETED`/`FAILED`; non-final status events carry `event` metadata
  but no `content`

This means LLM output **must** use artifacts for the TUI to display it.

---

## Changelog

| Version | Date | Changes | Issues |
|---------|------|---------|--------|
| 1.0 | 2026-06-01 | Initial hybrid model: artifacts for LLM text, status for progress | — |
| 1.1 | 2026-06-05 | Added metadata.type classification, keepalive events | #1399 |
| 1.2 | 2026-06-13 | Multi-part DataPart/TextPart artifacts, FunctionResponse suppression, event-aware text routing, metadata-only schema identification, execution progress, approval events | #1399, #1403, #1408, #1410, #1411 |

---

## References

- [A2A Protocol Specification](https://a2aprotocol.ai/) — `TaskStatusUpdateEvent`, `metadata`
- [A2A Deep Dive: Real-Time Updates](https://medium.com/google-cloud/a2a-deep-dive-getting-real-time-updates-from-ai-agents-a28d60317332)
- [Agent Stack A2A Client Integration](https://agentstack.beeai.dev/stable/custom-ui/a2a-client)
- [Kagenti Integration](./kagenti.md) — Kagenti-specific discovery and deployment
- [DD-AF-005](../../architecture/decisions/DD-AF-005-alert-prioritization-algorithm.md) — Alert prioritization algorithm
- [DD-AF-006](../../architecture/decisions/DD-AF-006-approval-consent-guard.md) — Approval consent guard
