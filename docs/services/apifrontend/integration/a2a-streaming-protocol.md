# A2A Streaming Protocol: Hybrid Event Model

**Version**: 1.1
**Last Updated**: 2026-06-05
**Status**: Active
**Applies to**: API Frontend (AF) A2A streaming endpoint (`/a2a/invoke`)

---

## Overview

The Kubernaut API Frontend uses a **hybrid event model** for A2A streaming:

- **`TaskArtifactUpdateEvent`** carries the LLM's final response text (markdown).
  The ADK executor manages the artifact lifecycle (`lastChunk`) so standard A2A
  clients render it as the persistent chat message.
- **`TaskStatusUpdateEvent`** carries ephemeral progress: orchestration status,
  reasoning deltas, investigation events, and keepalives. Each event includes a
  `metadata.type` field for semantic classification so enhanced clients can render
  them with appropriate visual treatment (dimmed reasoning, ephemeral substatus).

This hybrid approach ensures compatibility with all A2A clients:
- **Standard clients** (e.g., Kagenti TUI/backend) render artifacts as the main
  response and status events as streaming progress -- no changes needed.
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

### Artifact Events (LLM Output)

| Event Kind | Description | Rendering Guidance |
|------|-------------|--------------------|
| `artifact-update` | LLM response text (markdown), streamed as artifact chunks | Full render with markdown formatting; accumulate chunks until `lastChunk: true` |

### Status Events (`metadata.type`)

| Type | Description | Rendering Guidance |
|------|-------------|--------------------|
| `reasoning` | LLM inner thoughts, investigation reasoning deltas | Dimmed/collapsible, like Claude Code's "thinking" |
| `status` | Orchestration progress ("Analyzing...", tool call summaries) | Ephemeral/italic, substatus indicator |
| `investigation` | KA investigation events (tool calls, completions, errors) | Ephemeral, may include structured data |
| `keepalive` | Idle timeout prevention (no `status.message`) | Spinner/pulse indicator, or ignore |

---

## Event Examples

### Artifact Event (LLM response text)

```json
{
  "kind": "artifact-update",
  "taskId": "...",
  "artifact": {
    "artifactId": "...",
    "parts": [{"kind": "text", "text": "Here's a snapshot of `demo-mesh`:\n\n### Resources\n..."}]
  },
  "lastChunk": false
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

### Status Event (orchestration progress)

```json
{
  "kind": "status-update",
  "status": {
    "state": "working",
    "timestamp": "2026-06-05T12:00:02Z",
    "message": {
      "role": "agent",
      "parts": [{"kind": "text", "text": "Investigating demo-mesh/api-server...\n\n"}]
    }
  },
  "metadata": {"type": "status"}
}
```

### Keepalive Event (no message, metadata-only)

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
  On `lastChunk: true`, finalize the message.
- **Status events** (`status-update`): Show `status.message` text as streaming
  progress. Discard when the stream ends.

This is exactly how Kagenti, Agent Stack, and standard A2A clients behave.

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

A metadata-aware client inspects `metadata.type` on status events for
Claude Code/Cursor-style UX while keeping artifacts as the main response:

```typescript
for await (const event of a2aStream) {
  if (event.kind === "artifact-update") {
    appendToResponse(extractArtifactText(event));
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

### React Component Example

```tsx
function StreamEvent({ event }: { event: A2AEvent }) {
  if (event.kind === "artifact-update") {
    const text = event.artifact.parts
      .filter((p: any) => p.kind === "text")
      .map((p: any) => p.text)
      .join("");
    return <MarkdownRenderer>{text}</MarkdownRenderer>;
  }

  const type = event.metadata?.type ?? "status";
  const text = extractText(event.status?.message);

  switch (type) {
    case "reasoning":
      return <CollapsibleBlock title="Thinking..." className="text-gray-400 text-sm">{text}</CollapsibleBlock>;
    case "status":
    case "investigation":
      return <SubstatusLine className="text-gray-500 italic">{text}</SubstatusLine>;
    case "keepalive":
      return <PulseIndicator />;
    default:
      return <SubstatusLine>{text}</SubstatusLine>;
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
   collapsible reasoning and ephemeral substatus -- without breaking clients
   that ignore metadata.

### Why Metadata on Status Events?

Using `metadata.type` provides:

1. **Graceful degradation**: Clients that ignore metadata still render progress text
2. **Progressive enhancement**: Rich clients get semantic classification for free
3. **A2A spec compliance**: `metadata` is the designated extension point
4. **No protocol violations**: No custom event types or non-standard fields

### Kagenti Backend Behavior (Reference)

The Kagenti backend (`backend/app/routers/chat.py`) translates A2A SSE events:

- **Artifacts**: Always extracts text parts and forwards as `content` to the TUI
- **Status events**: Only forwards `content` when `is_final=True` or
  `state` is `COMPLETED`/`FAILED`; non-final status events carry `event` metadata
  but no `content`

This means LLM output **must** use artifacts for the TUI to display it.

---

## References

- [A2A Protocol Specification](https://a2aprotocol.ai/) -- `TaskStatusUpdateEvent`, `metadata`
- [A2A Deep Dive: Real-Time Updates](https://medium.com/google-cloud/a2a-deep-dive-getting-real-time-updates-from-ai-agents-a28d60317332)
- [Agent Stack A2A Client Integration](https://agentstack.beeai.dev/stable/custom-ui/a2a-client)
- [Kagenti Integration](./kagenti.md) -- Kagenti-specific discovery and deployment
