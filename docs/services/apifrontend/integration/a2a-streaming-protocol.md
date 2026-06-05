# A2A Streaming Protocol: Event Classification via Metadata

**Version**: 1.0
**Last Updated**: 2026-06-05
**Status**: Active
**Applies to**: API Frontend (AF) A2A streaming endpoint (`/a2a/invoke`)

---

## Overview

The Kubernaut API Frontend streams all progressive content to A2A clients via
`TaskStatusUpdateEvent` messages on the SSE stream. Each event carries a
`metadata.type` field that classifies the content semantically, enabling clients
to render different content types with appropriate visual treatment (e.g., dimmed
reasoning, ephemeral status indicators, full-render output).

This approach follows the A2A ecosystem convention where streaming text is
delivered via `status-update` events (not `artifact-update` events), ensuring
compatibility with clients like [Kagenti](https://github.com/kagenti/kagenti),
[Agent Stack](https://agentstack.beeai.dev/), and custom React/web UIs.

---

## Wire Format

All streaming events use `TaskStatusUpdateEvent` with the following structure:

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

## Event Types (`metadata.type`)

| Type | Description | Rendering Guidance |
|------|-------------|--------------------|
| `output` | Final LLM response text (markdown) | Full render with markdown formatting |
| `reasoning` | LLM inner thoughts, investigation reasoning deltas | Dimmed/collapsible, like Claude Code's "thinking" |
| `status` | Orchestration progress ("Analyzing...", "Listing cluster resources...") | Ephemeral/italic, substatus indicator |
| `investigation` | KA investigation events (tool calls, completions, errors) | Ephemeral, may include structured data |
| `keepalive` | Idle timeout prevention (no `status.message`) | Spinner/pulse indicator, or ignore |

---

## Event Examples

### Output Event (LLM response)

```json
{
  "kind": "status-update",
  "status": {
    "state": "working",
    "timestamp": "2026-06-05T12:00:05Z",
    "message": {
      "role": "agent",
      "parts": [{"kind": "text", "text": "Here's a snapshot of `demo-mesh`:\n\n### Resources\n..."}]
    }
  },
  "metadata": {"type": "output"}
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

### Minimal Client (No Metadata Awareness)

A client that ignores `metadata` entirely still works: every event with
`status.message` renders as streaming text. The UX is flat (no visual
separation) but no content is lost.

```javascript
for await (const event of a2aStream) {
  if (event.kind === "status-update" && event.status?.message) {
    const text = event.status.message.parts
      .filter(p => p.kind === "text")
      .map(p => p.text)
      .join("");
    appendText(text);
  }
}
```

### Enhanced Client (Metadata-Aware)

A metadata-aware client can provide Claude Code/Cursor-style UX:

```typescript
for await (const event of a2aStream) {
  if (event.kind === "status-update" && event.status?.message) {
    const text = extractText(event.status.message);
    const type = event.metadata?.type ?? "output";

    switch (type) {
      case "reasoning":
        appendToCollapsible("Thinking...", text);
        break;
      case "status":
      case "investigation":
        appendSubstatus(text);
        break;
      case "output":
      default:
        appendMessage(text);
        break;
    }
  }

  if (event.kind === "status-update" && event.metadata?.type === "keepalive") {
    showSpinner();
  }
}
```

### React Component Example

```tsx
function StreamEvent({ event }: { event: A2AStatusUpdate }) {
  const type = event.metadata?.type ?? "output";
  const text = extractText(event.status.message);

  switch (type) {
    case "reasoning":
      return <CollapsibleBlock title="Thinking..." className="text-gray-400 text-sm">{text}</CollapsibleBlock>;
    case "status":
    case "investigation":
      return <SubstatusLine className="text-gray-500 italic">{text}</SubstatusLine>;
    case "keepalive":
      return <PulseIndicator />;
    case "output":
    default:
      return <MarkdownRenderer>{text}</MarkdownRenderer>;
  }
}
```

---

## Design Decisions

### Why Status Events, Not Artifacts?

The A2A ecosystem convention (Kagenti, Agent Stack, A2A reference
implementations) uses `TaskStatusUpdateEvent` for streaming text and reserves
`TaskArtifactUpdateEvent` for structured outputs (files, canvases, assembled
results). Clients that render only status events during streaming -- like
Kagenti -- would display raw JSON for artifact-based content.

### Why Metadata, Not Separate Event Types?

Using `metadata.type` on a single event type provides:

1. **Graceful degradation**: Clients that ignore metadata still render all text
2. **Progressive enhancement**: Rich clients get semantic classification for free
3. **A2A spec compliance**: `metadata` is the designated extension point
4. **No protocol violations**: No custom event types or non-standard fields

### Relationship to `TaskArtifactUpdateEvent`

The AF still uses `TaskArtifactUpdateEvent` for the ADK executor's terminal
lifecycle events (`lastChunk`, task completion artifacts). The streaming content
during execution flows through status events exclusively.

---

## References

- [A2A Protocol Specification](https://a2aprotocol.ai/) -- `TaskStatusUpdateEvent`, `metadata`
- [A2A Deep Dive: Real-Time Updates](https://medium.com/google-cloud/a2a-deep-dive-getting-real-time-updates-from-ai-agents-a28d60317332)
- [Agent Stack A2A Client Integration](https://agentstack.beeai.dev/stable/custom-ui/a2a-client)
- [Kagenti Integration](./kagenti.md) -- Kagenti-specific discovery and deployment
