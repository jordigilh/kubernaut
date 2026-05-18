# ADR-014: Hybrid REST + MCP Client for AF-to-KA Communication

**Status:** Accepted
**Date:** 2026-05-03
**Deciders:** AF team, kubernaut team (kubernaut#1020)
**Supersedes:** ADR-006

## Context

ADR-006 chose "REST polling + CRD watch" based on the premise that KA did not implement SSE or interactive session support. This premise is no longer true. KA on `development/v1.5` now provides:

1. **REST API** (6 endpoints): analyze, status, result, cancel, snapshot, stream
2. **MCP endpoint** (`/api/v1/mcp/`): full interactive session via `kubernaut_investigate` (6 actions) and `kubernaut_select_workflow` tools, with streaming via MCP notifications

The interactive operations AF needs (takeover, message, complete, cancel) are already implemented as MCP tool actions — not as REST endpoints. Building duplicate REST endpoints would require KA-side work with no benefit.

## Decision

**Hybrid approach**: REST for autonomous operations, MCP client for interactive operations.

### Autonomous flow (REST)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/incident/analyze` | POST | Start investigation |
| `/api/v1/incident/session/{id}` | GET | Poll status |
| `/api/v1/incident/session/{id}/result` | GET | Get RCA result |
| `/api/v1/incident/session/{id}/cancel` | POST | Cancel investigation |
| `/api/v1/incident/session/{id}/snapshot` | GET | Reconnection snapshot |
| `/api/v1/incident/session/{id}/stream` | GET | SSE event stream |

Used when: AF starts an investigation and observes autonomous progress. Simple HTTP client, circuit breaker on failures.

### Interactive flow (MCP client)

AF connects to KA's MCP endpoint (`/api/v1/mcp/`) as an MCP client with the user's JWT for authentication.

**`kubernaut_investigate` tool actions:**

| Action | Purpose | Output status |
|--------|---------|---------------|
| `start` | Begin new interactive investigation | `session_started` |
| `takeover` | Take over running autonomous investigation | `takeover_started` |
| `message` | Send user follow-up to LLM | `message_sent` |
| `complete` | End session, trigger workflow execution | `completed` |
| `cancel` | Abort without remediation | `cancelled` |
| `status` | Query session state | current status |

**`kubernaut_select_workflow` tool:**
- Input: `{rr_id, workflow_id, kind, name, namespace}`
- Enrichment runs internally before workflow selection

**Streaming**: LLM responses, tool calls, and session events delivered via MCP `ServerSession.Log` notifications on the same SSE connection — single connection for both request and response streaming.

Used when: User takes over an investigation, sends follow-up messages, or drives workflow selection interactively.

### CRD watches (unchanged)

AF watches RR/AA/SP CRDs for pipeline state transitions (SP complete, AA created, RR phase changes). These provide context that neither KA REST nor MCP surfaces — the pipeline orchestration state.

## Consequences

- **No KA-side work needed**: Interactive surface already exists on MCP endpoint (39 unit tests + E2E suite)
- **Single protocol for interactive**: MCP transport handles streaming, session lifecycle, and tool calls in one connection
- **New dependency**: AF adds MCP Go SDK (`mcp-go`) as a client dependency
- **Auth works the same**: JWT forwarded per ADR-013 on both REST (header) and MCP (connection auth)
- **REST stays for autonomous**: Simple, stateless, well-tested — no reason to change
- **Reduces AF streaming complexity**: No need to build custom SSE relay for interactive mode; MCP transport handles it
- **Closes kubernaut#1020**: No new KA REST endpoints needed
- **kubernaut#874 dependency resolved**: Interactive session support is available via MCP (not blocked)

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| New REST endpoints for takeover/message (original #1020 ask) | Requires KA-side work; duplicates existing MCP surface; two protocols for the same operations |
| MCP-only (drop REST entirely) | REST is simpler for autonomous fire-and-forget operations; circuit breaker pattern maps better to REST |
| Keep ADR-006 as-is (REST polling only) | Premise is outdated; would require building custom session management that MCP already provides |
| gRPC streaming | KA doesn't expose gRPC; MCP provides equivalent streaming capability |
